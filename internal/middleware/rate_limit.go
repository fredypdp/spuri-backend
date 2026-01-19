// ============================================================================
// ARQUIVO: internal/middleware/rate_limit.go
// ✅ Rate limiting por IP e endpoint
// ============================================================================

package middleware

import (
	"net/http"
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
	rl := &RateLimiter{
		limiters: make(map[string]*rate.Limiter),
		rate:     r,
		burst:    b,
		ttl:      ttl,
	}
	
	// Limpeza periódica
	go rl.cleanup()
	
	return rl
}

func (rl *RateLimiter) getLimiter(key string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	limiter, exists := rl.limiters[key]
	if !exists {
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
		rl.limiters = make(map[string]*rate.Limiter)
		rl.mu.Unlock()
	}
}

// GlobalRateLimiter - 100 req/min por IP
var GlobalRateLimiter = NewRateLimiter(
	rate.Every(time.Minute/100), // 100 por minuto
	10,                           // burst de 10
	5*time.Minute,                // TTL
)

// LoginRateLimiter - 5 tentativas/min por IP
var LoginRateLimiter = NewRateLimiter(
	rate.Every(time.Minute/5), // 5 por minuto
	2,                          // burst de 2
	10*time.Minute,             // TTL
)

// EmailRateLimiter - 2 envios/hora por IP
var EmailRateLimiter = NewRateLimiter(
	rate.Every(time.Hour/2), // 2 por hora
	1,                        // burst de 1
	time.Hour,                // TTL
)

func getClientIP(c *gin.Context) string {
	// Verificar headers de proxy
	ip := c.GetHeader("X-Forwarded-For")
	if ip == "" {
		ip = c.GetHeader("X-Real-IP")
	}
	if ip == "" {
		ip = c.ClientIP()
	}
	return ip
}

// RateLimitMiddleware retorna middleware de rate limiting
func RateLimitMiddleware(limiter *RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := getClientIP(c)
		
		if !limiter.getLimiter(ip).Allow() {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "muitas requisições, tente novamente mais tarde",
				"retry_after": "60s",
			})
			c.Abort()
			return
		}
		
		c.Next()
	}
}

// GlobalRateLimit aplica limite global
func GlobalRateLimit() gin.HandlerFunc {
	return RateLimitMiddleware(GlobalRateLimiter)
}

// LoginRateLimit aplica limite de login
func LoginRateLimit() gin.HandlerFunc {
	return RateLimitMiddleware(LoginRateLimiter)
}

// EmailRateLimit aplica limite de email
func EmailRateLimit() gin.HandlerFunc {
	return RateLimitMiddleware(EmailRateLimiter)
}