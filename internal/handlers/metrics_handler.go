// ============================================================================
// ARQUIVO: internal/handlers/metrics_handler.go
//
// Handler para o endpoint de métricas do sistema.
// FIX-ERR: main.go referenciava handlers.GetMetrics que não existia.
// Criado GetSystemMetrics que delega para monitoring.GetMetrics().GetSnapshot().
// ============================================================================

package handlers

import (
	"net/http"
	"spuri/internal/monitoring"

	"github.com/gin-gonic/gin"
)

// GetSystemMetrics retorna snapshot das métricas do sistema.
// Endpoint: GET /admin/metrics
// Requer: RequireAdmin()
func GetSystemMetrics(c *gin.Context) {
	metrics := monitoring.GetMetrics()
	snapshot := metrics.GetSnapshot()

	c.JSON(http.StatusOK, gin.H{
		"metrics": snapshot,
	})
}