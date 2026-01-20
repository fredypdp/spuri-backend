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

var (
	GlobalRateLimiter = NewRateLimiter(
		rate.Every(time.Minute/100),
		10,
		5*time.Minute,
	)

	LoginRateLimiter = NewRateLimiter(
		rate.Every(time.Minute/5),
		2,
		10*time.Minute,
	)

	EmailRateLimiter = NewRateLimiter(
		rate.Every(time.Hour/2),
		1,
		time.Hour,
	)
)

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

func GlobalRateLimit() gin.HandlerFunc {
	return RateLimitMiddleware(GlobalRateLimiter)
}

func LoginRateLimit() gin.HandlerFunc {
	return RateLimitMiddleware(LoginRateLimiter)
}

func EmailRateLimit() gin.HandlerFunc {
	return RateLimitMiddleware(EmailRateLimiter)
}