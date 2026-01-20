// ============================================================================
// ARQUIVO: internal/middleware/rate_limit.go
// Rate limiting configurável via variáveis de ambiente
// ============================================================================

package middleware

import (
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

// Configuração dinâmica baseada em ENV
var (
	GlobalRateLimiter *RateLimiter
	LoginRateLimiter  *RateLimiter
	EmailRateLimiter  *RateLimiter
)

func init() {
	// Global Rate Limiter
	globalLimit := getEnvInt("RATE_LIMIT_GLOBAL", 100)
	GlobalRateLimiter = NewRateLimiter(
		rate.Every(time.Minute/time.Duration(globalLimit)),
		globalLimit/10,
		5*time.Minute,
	)

	// Login Rate Limiter
	loginLimit := getEnvInt("RATE_LIMIT_LOGIN", 5)
	LoginRateLimiter = NewRateLimiter(
		rate.Every(time.Minute/time.Duration(loginLimit)),
		loginLimit/2,
		10*time.Minute,
	)

	// Email Rate Limiter
	emailLimit := getEnvInt("RATE_LIMIT_EMAIL", 2)
	EmailRateLimiter = NewRateLimiter(
		rate.Every(time.Hour/time.Duration(emailLimit)),
		1,
		time.Hour,
	)
}

func getEnvInt(key string, defaultValue int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
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