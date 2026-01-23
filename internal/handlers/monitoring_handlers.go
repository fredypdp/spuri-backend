// ============================================================================
// ARQUIVO: internal/handlers/monitoring_handlers.go
// Handlers para endpoints de monitoramento
// ============================================================================

package handlers

import (
	"net/http"
	"spuri/internal/monitoring"

	"github.com/gin-gonic/gin"
)

// HealthCheckBasic endpoint básico (público)
func HealthCheckBasic(c *gin.Context) {
	client := getDbClient(c)
	
	status := "ok"
	dbStatus := "ok"
	
	if err := client.Health(); err != nil {
		status = "degraded"
		dbStatus = "error"
	}
	
	c.JSON(http.StatusOK, gin.H{
		"status":   status,
		"database": dbStatus,
		"service":  "Spuri Event Sourcing API",
		"version":  "3.0.0",
	})
}

// HealthCheckDetailed endpoint detalhado (apenas admin)
func HealthCheckDetailed(c *gin.Context) {
	client := getDbClient(c)
	checker := monitoring.NewHealthChecker(client.DB())
	
	health := checker.CheckAll()
	
	statusCode := http.StatusOK
	if health.Status == monitoring.StatusUnhealthy {
		statusCode = http.StatusServiceUnavailable
	} else if health.Status == monitoring.StatusDegraded {
		statusCode = http.StatusOK // Ainda funcional
	}
	
	c.JSON(statusCode, health)
}

// GetMetrics retorna métricas do sistema (apenas admin)
func GetMetrics(c *gin.Context) {
	metrics := monitoring.GetMetrics()
	snapshot := metrics.GetSnapshot()
	
	c.JSON(http.StatusOK, gin.H{
		"metrics": snapshot,
	})
}

// ✅ CORRIGIDO: QueryRow().Scan() manual
func GetSystemStats(c *gin.Context) {
	client := getDbClient(c)
	
	type DBStats struct {
		TotalEvents      int64 `db:"total_events"`
		TotalAggregates  int64 `db:"total_aggregates"`
		TotalEstudantes  int64 `db:"total_estudantes"`
		TotalAcademias   int64 `db:"total_academias"`
		TotalAdmins      int64 `db:"total_admins"`
		TotalNotas       int64 `db:"total_notas"`
		TotalFaltas      int64 `db:"total_faltas"`
		TotalInscricoes  int64 `db:"total_inscricoes"`
	}
	
	var stats DBStats
	query := `
		SELECT 
			(SELECT COUNT(*) FROM spuri_ledger) as total_events,
			(SELECT COUNT(DISTINCT aggregate_id) FROM spuri_ledger) as total_aggregates,
			(SELECT COUNT(*) FROM projection_estudantes) as total_estudantes,
			(SELECT COUNT(*) FROM projection_academias) as total_academias,
			(SELECT COUNT(*) FROM projection_admins) as total_admins,
			(SELECT COUNT(*) FROM projection_notas) as total_notas,
			(SELECT COUNT(*) FROM projection_faltas) as total_faltas,
			(SELECT COUNT(*) FROM projection_inscricoes) as total_inscricoes
	`
	
	err := client.DB().QueryRow(query).Scan(
		&stats.TotalEvents,
		&stats.TotalAggregates,
		&stats.TotalEstudantes,
		&stats.TotalAcademias,
		&stats.TotalAdmins,
		&stats.TotalNotas,
		&stats.TotalFaltas,
		&stats.TotalInscricoes,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "erro ao buscar estatísticas",
		})
		return
	}
	
	// Estatísticas por tipo de evento
	type EventTypeCount struct {
		EventType string `db:"event_type"`
		Count     int64  `db:"count"`
	}

	queryTypes := `
		SELECT event_type, COUNT(*) as count
		FROM spuri_ledger
		GROUP BY event_type
		ORDER BY count DESC
	`

	rows, err := client.DB().Query(queryTypes)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "erro ao buscar tipos de eventos",
		})
		return
	}
	defer rows.Close()

	var eventTypes []EventTypeCount
	for rows.Next() {
		var et EventTypeCount
		if err := rows.Scan(&et.EventType, &et.Count); err != nil {
			continue
		}
		eventTypes = append(eventTypes, et)
	}
	
	c.JSON(http.StatusOK, gin.H{
		"database":    stats,
		"event_types": eventTypes,
		"db_stats":    client.Stats(),
	})
}

// ResetMetrics reseta métricas (apenas admin FPP)
func ResetMetrics(c *gin.Context) {
	metrics := monitoring.GetMetrics()
	metrics.Reset()
	
	c.JSON(http.StatusOK, gin.H{
		"message": "métricas resetadas com sucesso",
	})
}