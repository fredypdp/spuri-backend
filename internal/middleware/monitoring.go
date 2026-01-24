// ============================================================================
// ARQUIVO: internal/middleware/monitoring.go
// Middleware para coleta de métricas
// ============================================================================

package middleware

import (
	"log"
	"spuri/internal/monitoring"
	"time"

	"github.com/gin-gonic/gin"
)

// MonitoringMiddleware coleta métricas de cada requisição
func MonitoringMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method
		
		log.Printf("📊 [Monitoring] Iniciando - %s %s - IP: %s", method, path, c.ClientIP())

		// Processar requisição
		c.Next()

		// Calcular latência
		latency := time.Since(start)
		statusCode := c.Writer.Status()

		// Verificar se houve erro
		hasError := len(c.Errors) > 0 || statusCode >= 400

		log.Printf("📈 [Monitoring] Finalizado - %s %s - Status: %d - Latency: %v - HasError: %v",
			method, path, statusCode, latency, hasError)

		// Registrar métricas
		monitoring.GetMetrics().RecordRequest(path, latency, hasError)

		// Registrar falhas de autenticação
		if statusCode == 401 {
			log.Printf("🔒 [Monitoring] Falha de autenticação registrada - Path: %s - IP: %s", path, c.ClientIP())
			monitoring.GetMetrics().RecordAuthFailure()
		}

		// Log de erros HTTP
		if hasError {
			log.Printf("⚠️ [Monitoring] Erro HTTP - %s %s - Status: %d - Errors: %v", 
				method, path, statusCode, c.Errors)
		}
	}
}