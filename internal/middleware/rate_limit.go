package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// RateLimiter é mantido apenas por compatibilidade de API.
type RateLimiter struct{}

func NewRateLimiter(_ rate.Limit, _ int, _ time.Duration) *RateLimiter {
	return &RateLimiter{}
}

// RateLimitMiddleware não aplica bloqueio algum.
func RateLimitMiddleware(_ *RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
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
