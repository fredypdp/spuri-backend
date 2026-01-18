package handlers

import (
	"net/http"
	"spuri/internal/projections"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid" // ← ADICIONAR ESTE IMPORT
)

// RebuildProjection reconstrói uma projeção específica
func RebuildProjection(c *gin.Context) {
	projectionName := c.Param("name")

	// Validar nome
	validNames := map[string]bool{
		"estudantes": true,
		"academias":  true,
		"notas":      true,
		"faltas":     true,
		"inscricoes": true,
		"all":        true,
	}

	if !validNames[projectionName] {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "projeção inválida",
			"valid_names": []string{
				"estudantes", "academias", "notas", "faltas", "inscricoes", "all",
			},
		})
		return
	}

	// Obter projection manager
	projManager := getProjectionManager(c)

	// Reconstruir
	if projectionName == "all" {
		if err := projManager.RebuildAllProjections(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "erro ao reconstruir projeções",
				"details": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message": "todas as projeções reconstruídas com sucesso",
		})
		return
	}

	// Reconstruir projeção específica
	if err := projManager.RebuildProjection(projectionName); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "erro ao reconstruir projeção",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "projeção reconstruída com sucesso",
		"projection": projectionName,
	})
}

// GetProjectionStatus retorna o status de uma projeção
func GetProjectionStatus(c *gin.Context) {
	projectionName := c.Param("name")

	projManager := getProjectionManager(c)
	status, err := projManager.GetProjectionStatus(projectionName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "projeção não encontrada",
		})
		return
	}

	c.JSON(http.StatusOK, status)
}

// GetAllProjectionStatuses retorna status de todas as projeções
func GetAllProjectionStatuses(c *gin.Context) {
	projManager := getProjectionManager(c)
	statuses, err := projManager.GetAllProjectionStatuses()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "erro ao buscar status",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"projections": statuses,
		"total": len(statuses),
	})
}

// GetLedgerStats retorna estatísticas do Banco de dados Ledger
func GetLedgerStats(c *gin.Context) {
	client := getDbClientFromContext(c)
	
	// Estatísticas do ledger via SQL
	type LedgerStats struct {
		TotalEvents        int64  `db:"total_events"`
		TotalAggregates    int64  `db:"total_aggregates"`
		OldestEvent        string `db:"oldest_event"`
		NewestEvent        string `db:"newest_event"`
		AvgEventsPerAgg    float64 `db:"avg_events_per_aggregate"`
	}

	query := `
		SELECT 
			COUNT(*)::BIGINT as total_events,
			COUNT(DISTINCT aggregate_id)::BIGINT as total_aggregates,
			TO_CHAR(MIN(occurred_at), 'YYYY-MM-DD HH24:MI:SS') as oldest_event,
			TO_CHAR(MAX(occurred_at), 'YYYY-MM-DD HH24:MI:SS') as newest_event,
			ROUND(COUNT(*)::NUMERIC / NULLIF(COUNT(DISTINCT aggregate_id), 0), 2) as avg_events_per_aggregate
		FROM spuri_ledger
	`

	var stats LedgerStats
	if err := client.DB().Get(&stats, query); err != nil {
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

	var eventTypes []EventTypeCount
	client.DB().Select(&eventTypes, queryTypes)

	c.JSON(http.StatusOK, gin.H{
		"ledger": stats,
		"event_types": eventTypes,
		"db_stats": client.Stats(),
	})
}

// VerifyAllIntegrity verifica integridade de todos os agregados
func VerifyAllIntegrity(c *gin.Context) {
	client := getDbClientFromContext(c)
	
	// Buscar todos os agregados distintos
	query := `
		SELECT DISTINCT aggregate_id, aggregate_type
		FROM spuri_ledger
		ORDER BY aggregate_type, aggregate_id
	`

	type Aggregate struct {
		ID   string `db:"aggregate_id"`
		Type string `db:"aggregate_type"`
	}

	var aggregates []Aggregate
	if err := client.DB().Select(&aggregates, query); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "erro ao buscar agregados",
		})
		return
	}

	// Verificar integridade de cada um
	repository := getRepository(c)
	results := make([]gin.H, 0)
	totalValid := 0
	totalInvalid := 0

	for _, agg := range aggregates {
		// Converter string para UUID
		id, err := uuid.Parse(agg.ID) // ← AGORA uuid.Parse está disponível
		if err != nil {
			continue
		}

		isValid, err := repository.VerifyIntegrity(id)
		if err != nil {
			results = append(results, gin.H{
				"aggregate_id": agg.ID,
				"type": agg.Type,
				"valid": false,
				"error": err.Error(),
			})
			totalInvalid++
			continue
		}

		if isValid {
			totalValid++
		} else {
			totalInvalid++
		}

		results = append(results, gin.H{
			"aggregate_id": agg.ID,
			"type": agg.Type,
			"valid": isValid,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"summary": gin.H{
			"total": len(aggregates),
			"valid": totalValid,
			"invalid": totalInvalid,
		},
		"aggregates": results,
	})
}

// Helper functions

func getProjectionManager(c *gin.Context) *projections.Manager {
	manager, _ := c.Get("projManager")
	return manager.(*projections.Manager)
}