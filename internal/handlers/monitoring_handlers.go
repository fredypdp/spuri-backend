// ============================================================================
// ARQUIVO: internal/handlers/monitoring_handlers.go
// Handlers para endpoints de monitoramento
// ============================================================================

package handlers

import (
	"log"
	"net/http"
	"spuri/internal/monitoring"

	"github.com/gin-gonic/gin"
)

// HealthCheckBasic endpoint básico (público)
func HealthCheckBasic(c *gin.Context) {
	log.Printf("💊 [HEALTH-CHECK-BASIC] Verificando saúde básica do sistema")
	
	client := getDbClient(c)
	
	status := "ok"
	dbStatus := "ok"
	
	log.Printf("🔍 [HEALTH-CHECK-BASIC-DEBUG] Verificando conexão com database...")
	if err := client.Health(); err != nil {
		log.Printf("❌ [HEALTH-CHECK-BASIC-DEBUG] Database com problemas: %v", err)
		status = "degraded"
		dbStatus = "error"
	} else {
		log.Printf("✅ [HEALTH-CHECK-BASIC-DEBUG] Database OK")
	}
	
	log.Printf("✅ [HEALTH-CHECK-BASIC] Status: %s, Database: %s", status, dbStatus)
	
	c.JSON(http.StatusOK, gin.H{
		"status":   status,
		"database": dbStatus,
		"service":  "Spuri Event Sourcing API",
		"version":  "3.0.0",
	})
}

// HealthCheckDetailed endpoint detalhado (apenas admin)
func HealthCheckDetailed(c *gin.Context) {
	log.Printf("💊 [HEALTH-CHECK-DETAILED] Iniciando verificação detalhada")
	
	client := getDbClient(c)
	checker := monitoring.NewHealthChecker(client.DB())
	
	log.Printf("🔍 [HEALTH-CHECK-DETAILED-DEBUG] Executando checagem completa...")
	health := checker.CheckAll()
	
	log.Printf("📊 [HEALTH-CHECK-DETAILED-DEBUG] Status: %s", health.Status)
	log.Printf("📊 [HEALTH-CHECK-DETAILED-DEBUG] Components: %v", health.Components)
	
	statusCode := http.StatusOK
	if health.Status == monitoring.StatusUnhealthy {
		log.Printf("❌ [HEALTH-CHECK-DETAILED] Sistema UNHEALTHY")
		statusCode = http.StatusServiceUnavailable
	} else if health.Status == monitoring.StatusDegraded {
		log.Printf("⚠️ [HEALTH-CHECK-DETAILED] Sistema DEGRADED")
		statusCode = http.StatusOK // Ainda funcional
	} else {
		log.Printf("✅ [HEALTH-CHECK-DETAILED] Sistema HEALTHY")
	}
	
	c.JSON(statusCode, health)
}

// GetMetrics retorna métricas do sistema (apenas admin)
func GetMetrics(c *gin.Context) {
	log.Printf("📊 [GET-METRICS] Coletando métricas do sistema")
	
	metrics := monitoring.GetMetrics()
	snapshot := metrics.GetSnapshot()
	
	log.Printf("✅ [GET-METRICS-DEBUG] Snapshot coletado - Commands: %d, Events: %d, Errors: %d",
		snapshot["commands_total"], snapshot["events_total"], snapshot["errors_total"])
	
	c.JSON(http.StatusOK, gin.H{
		"metrics": snapshot,
	})
}

// GetSystemStats retorna estatísticas do sistema
func GetSystemStats(c *gin.Context) {
	log.Printf("📊 [SYSTEM-STATS] Coletando estatísticas do sistema")
	
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
	
	log.Printf("🔍 [SYSTEM-STATS-DEBUG] Executando query de estatísticas gerais...")
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
		log.Printf("❌ [SYSTEM-STATS-DEBUG] Erro ao buscar estatísticas: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "erro ao buscar estatísticas",
		})
		return
	}
	
	log.Printf("✅ [SYSTEM-STATS-DEBUG] Estatísticas gerais coletadas - Events: %d, Aggregates: %d",
		stats.TotalEvents, stats.TotalAggregates)
	
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

	log.Printf("🔍 [SYSTEM-STATS-DEBUG] Executando query de tipos de eventos...")
	rows, err := client.DB().Query(queryTypes)
	if err != nil {
		log.Printf("❌ [SYSTEM-STATS-DEBUG] Erro ao buscar tipos de eventos: %v", err)
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
			log.Printf("⚠️ [SYSTEM-STATS-DEBUG] Erro ao ler tipo de evento: %v", err)
			continue
		}
		eventTypes = append(eventTypes, et)
	}
	
	log.Printf("✅ [SYSTEM-STATS-DEBUG] Tipos de eventos coletados - Total tipos: %d", len(eventTypes))
	
	dbStats := client.Stats()
	log.Printf("📊 [SYSTEM-STATS-DEBUG] DB Pool Stats - OpenConns: %d, InUse: %d, Idle: %d",
		dbStats.OpenConnections, dbStats.InUse, dbStats.Idle)
	
	log.Printf("✅ [SYSTEM-STATS] Estatísticas completas coletadas")
	
	c.JSON(http.StatusOK, gin.H{
		"database":    stats,
		"event_types": eventTypes,
		"db_stats":    dbStats,
	})
}

// ResetMetrics reseta métricas (apenas admin FPP)
func ResetMetrics(c *gin.Context) {
	log.Printf("🔄 [RESET-METRICS] Resetando métricas do sistema")
	
	metrics := monitoring.GetMetrics()
	metrics.Reset()
	
	log.Printf("✅ [RESET-METRICS] Métricas resetadas com sucesso")
	
	c.JSON(http.StatusOK, gin.H{
		"message": "métricas resetadas com sucesso",
	})
}