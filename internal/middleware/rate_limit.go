// ============================================================================
// ARQUIVO: internal/middleware/rate_limit.go
//
// CORREÇÕES APLICADAS:
//   FIX-RL1 — cleanup(): antes apagava TODOS os limiters a cada TTL tick,
//              incluindo IPs com tentativas recentes (ex: brute force ativo era
//              resetado a cada 10min para todos simultaneamente).
//              Agora: cada entry tem timestamp de último acesso, e o cleanup
//              remove apenas entries inativos por mais de TTL.
//   FIX-RL2 — getLimiter(): atualiza lastSeen a cada acesso para evitar remoção
//              prematura de IPs ativos.
// ============================================================================

package middleware

import (
	"log"
	"net/http"
	"os"
	"spuri/internal/monitoring"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// limiterEntry armazena o limiter e o timestamp do último acesso.
// FIX-RL1: lastSeen permite cleanup seletivo (remove apenas inativos).
type limiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type RateLimiter struct {
	mu       sync.RWMutex
	entries  map[string]*limiterEntry
	rate     rate.Limit
	burst    int
	ttl      time.Duration
}

func NewRateLimiter(r rate.Limit, b int, ttl time.Duration) *RateLimiter {
	log.Printf("🚦 [RateLimiter] Criando novo limiter - Rate: %v, Burst: %d, TTL: %v", r, b, ttl)

	rl := &RateLimiter{
		entries: make(map[string]*limiterEntry),
		rate:    r,
		burst:   b,
		ttl:     ttl,
	}

	go rl.cleanup()
	return rl
}

// getLimiter retorna o limiter para a chave informada, criando um novo se necessário.
// FIX-RL2: atualiza lastSeen a cada acesso.
func (rl *RateLimiter) getLimiter(key string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	entry, exists := rl.entries[key]
	if !exists {
		log.Printf("🆕 [RateLimiter] Novo limiter para: %s", key)
		entry = &limiterEntry{
			limiter:  rate.NewLimiter(rl.rate, rl.burst),
			lastSeen: time.Now(),
		}
		rl.entries[key] = entry
	} else {
		// FIX-RL2: atualiza lastSeen para evitar que IPs ativos sejam removidos
		entry.lastSeen = time.Now()
	}

	return entry.limiter
}

// cleanup remove apenas entries inativos por mais de TTL.
// FIX-RL1: antes removia TODOS — agora é seletivo.
func (rl *RateLimiter) cleanup() {
	// Intervalo de cleanup = metade do TTL para checar com frequência razoável
	ticker := time.NewTicker(rl.ttl / 2)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		removed := 0
		for key, entry := range rl.entries {
			if now.Sub(entry.lastSeen) > rl.ttl {
				delete(rl.entries, key)
				removed++
			}
		}
		rl.mu.Unlock()

		if removed > 0 {
			log.Printf("🧹 [RateLimiter] Cleanup: %d entries expirados removidos", removed)
		}
	}
}

// Configuração dinâmica baseada em variáveis de ambiente
var (
	GlobalRateLimiter *RateLimiter
	LoginRateLimiter  *RateLimiter
	EmailRateLimiter  *RateLimiter
)

func init() {
	log.Printf("⚙️  [RateLimit] Inicializando rate limiters...")

	globalLimit := getEnvInt("RATE_LIMIT_GLOBAL", 100)
	log.Printf("🌍 [RateLimit] Global: %d req/min", globalLimit)
	GlobalRateLimiter = NewRateLimiter(
		rate.Every(time.Minute/time.Duration(globalLimit)),
		globalLimit/10,
		5*time.Minute,
	)

	loginLimit := getEnvInt("RATE_LIMIT_LOGIN", 5)
	log.Printf("🔐 [RateLimit] Login: %d req/min", loginLimit)
	LoginRateLimiter = NewRateLimiter(
		rate.Every(time.Minute/time.Duration(loginLimit)),
		loginLimit/2,
		// FIX-RL1: TTL de 30min para login — bloqueio de brute force persiste
		// por 30min sem reset global acidental.
		30*time.Minute,
	)

	emailLimit := getEnvInt("RATE_LIMIT_EMAIL", 2)
	log.Printf("📧 [RateLimit] Email: %d req/hour", emailLimit)
	EmailRateLimiter = NewRateLimiter(
		rate.Every(time.Hour/time.Duration(emailLimit)),
		1,
		time.Hour,
	)

	log.Printf("✅ [RateLimit] Rate limiters inicializados com sucesso")
}

func getEnvInt(key string, defaultValue int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil && i > 0 {
			log.Printf("📝 [RateLimit] %s = %d (via ENV)", key, i)
			return i
		}
		log.Printf("⚠️  [RateLimit] %s inválido: %s (usando padrão: %d)", key, val, defaultValue)
	}
	log.Printf("📝 [RateLimit] %s = %d (padrão)", key, defaultValue)
	return defaultValue
}

func getClientIP(c *gin.Context) string {
	ip := c.GetHeader("X-Forwarded-For")
	if ip == "" {
		ip = c.GetHeader("X-Real-IP")
	}
	if ip == "" {
		ip = c.ClientIP()
	}
	return ip
}

func RateLimitMiddleware(limiter *RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := getClientIP(c)

		if !limiter.getLimiter(ip).Allow() {
			log.Printf("⛔ [RateLimit] BLOQUEADO - IP: %s - Path: %s", ip, c.Request.URL.Path)
			monitoring.GetMetrics().RecordRateLimit()

			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       "muitas requisições, tente novamente mais tarde",
				"retry_after": "60s",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

func GlobalRateLimit() gin.HandlerFunc {
	return RateLimitMiddleware(GlobalRateLimiter)
}

func LoginRateLimit() gin.HandlerFunc {
	return RateLimitMiddleware(LoginRateLimiter)
}

func EmailRateLimit() gin.HandlerFunc {
	return RateLimitMiddleware(EmailRateLimiter)
}