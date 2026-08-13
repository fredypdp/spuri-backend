package middleware

import (
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// RateLimiter keeps independent token buckets per source address.  Public
// financial actions use dedicated instances rather than relying on the legacy
// disabled global limiter.
type RateLimiter struct {
	mu      sync.Mutex
	limit   rate.Limit
	burst   int
	clients map[string]*rate.Limiter
}

func NewRateLimiter(limit rate.Limit, burst int, _ time.Duration) *RateLimiter {
	return &RateLimiter{limit: limit, burst: burst, clients: map[string]*rate.Limiter{}}
}

func RateLimitMiddleware(l *RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		if l == nil {
			c.Next()
			return
		}
		key := c.ClientIP()
		l.mu.Lock()
		bucket := l.clients[key]
		if bucket == nil {
			bucket = rate.NewLimiter(l.limit, l.burst)
			l.clients[key] = bucket
		}
		allowed := bucket.Allow()
		l.mu.Unlock()
		if !allowed {
			c.AbortWithStatus(429)
			return
		}
		c.Next()
	}
}

// GlobalRateLimit desativado.
func GlobalRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}

// LoginRateLimit desativado.
func LoginRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}

// EmailRateLimit desativado.
func EmailRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}
