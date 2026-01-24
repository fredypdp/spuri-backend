// ============================================================================
// ARQUIVO: internal/middleware/rate_limit.go
// Rate limiting configurável via variáveis de ambiente
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

type RateLimiter struct {
	mu       sync.RWMutex
	limiters map[string]*rate.Limiter
	rate     rate.Limit
	burst    int
	ttl      time.Duration
}

func NewRateLimiter(r rate.Limit, b int, ttl time.Duration) *RateLimiter {
	log.Printf("🚦 [RateLimiter] Criando novo limiter - Rate: %v, Burst: %d, TTL: %v", r, b, ttl)
	
	rl := &RateLimiter{
		limiters: make(map[string]*rate.Limiter),
		rate:     r,
		burst:    b,
		ttl:      ttl,
	}
	
	go rl.cleanup()
	
	return rl
}

func (rl *RateLimiter) getLimiter(key string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	limiter, exists := rl.limiters[key]
	if !exists {
		log.Printf("🆕 [RateLimiter] Criando novo limiter para IP: %s", key)
		limiter = rate.NewLimiter(rl.rate, rl.burst)
		rl.limiters[key] = limiter
	}

	return limiter
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.ttl)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		count := len(rl.limiters)
		rl.limiters = make(map[string]*rate.Limiter)
		rl.mu.Unlock()
		
		log.Printf("🧹 [RateLimiter] Cleanup executado - %d limiters removidos", count)
	}
}

// Configuração dinâmica baseada em ENV
var (
	GlobalRateLimiter *RateLimiter
	LoginRateLimiter  *RateLimiter
	EmailRateLimiter  *RateLimiter
)

func init() {
	log.Printf("⚙️ [RateLimit] Inicializando rate limiters...")
	
	// Global Rate Limiter
	globalLimit := getEnvInt("RATE_LIMIT_GLOBAL", 100)
	log.Printf("🌍 [RateLimit] Global: %d req/min", globalLimit)
	GlobalRateLimiter = NewRateLimiter(
		rate.Every(time.Minute/time.Duration(globalLimit)),
		globalLimit/10,
		5*time.Minute,
	)

	// Login Rate Limiter
	loginLimit := getEnvInt("RATE_LIMIT_LOGIN", 5)
	log.Printf("🔐 [RateLimit] Login: %d req/min", loginLimit)
	LoginRateLimiter = NewRateLimiter(
		rate.Every(time.Minute/time.Duration(loginLimit)),
		loginLimit/2,
		10*time.Minute,
	)

	// Email Rate Limiter
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
		if i, err := strconv.Atoi(val); err == nil {
			log.Printf("📝 [RateLimit] %s = %d (via ENV)", key, i)
			return i
		}
		log.Printf("⚠️ [RateLimit] %s inválido: %s (usando padrão: %d)", key, val, defaultValue)
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
		path := c.Request.URL.Path
		
		log.Printf("🚦 [RateLimit] Verificando - IP: %s - Path: %s", ip, path)
		
		if !limiter.getLimiter(ip).Allow() {
			log.Printf("⛔ [RateLimit] BLOQUEADO - IP: %s - Path: %s", ip, path)
			monitoring.GetMetrics().RecordRateLimit()
			
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       "muitas requisições, tente novamente mais tarde",
				"retry_after": "60s",
			})
			c.Abort()
			return
		}
		
		log.Printf("✅ [RateLimit] Permitido - IP: %s - Path: %s", ip, path)
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