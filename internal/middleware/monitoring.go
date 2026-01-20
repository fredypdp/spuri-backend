// ============================================================================
// ARQUIVO: internal/middleware/monitoring.go
// Middleware para coleta de métricas
// ============================================================================

package middleware

import (
	"spuri/internal/monitoring"
	"time"

	"github.com/gin-gonic/gin"
)

// MonitoringMiddleware coleta métricas de cada requisição
func MonitoringMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path

		// Processar requisição
		c.Next()

		// Calcular latência
		latency := time.Since(start)

		// Verificar se houve erro
		hasError := len(c.Errors) > 0 || c.Writer.Status() >= 400

		// Registrar métricas
		monitoring.GetMetrics().RecordRequest(path, latency, hasError)

		// Registrar falhas de autenticação
		if c.Writer.Status() == 401 {
			monitoring.GetMetrics().RecordAuthFailure()
		}
	}
}