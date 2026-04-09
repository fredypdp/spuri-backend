package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"spuri/internal/jobs"
	"spuri/internal/middleware"
	"spuri/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type rebuildProjectionRequest struct {
	Name string `json:"name"`
}

func runProjectionRebuild(c *gin.Context, adminID uuid.UUID, name string) error {
	if name == "" {
		return fmt.Errorf("nome da projeção é obrigatório")
	}

	manager := getProjManager(c)
	if manager == nil {
		return fmt.Errorf("projection manager não disponível")
	}

	if err := manager.RebuildProjection(name); err != nil {
		log.Printf("❌ [RebuildProjection] Falha ao reconstruir '%s' por admin %v: %v", name, adminID, err)
		registrarAcaoAdmin(c, adminID, "rebuild_projection", map[string]interface{}{
			"projection": name,
			"resultado":  "falha",
			"erro":       err.Error(),
		})
		return err
	}

	log.Printf("✅ [RebuildProjection] Projeção '%s' reconstruída com sucesso por admin %v", name, adminID)
	registrarAcaoAdmin(c, adminID, "rebuild_projection", map[string]interface{}{
		"projection": name,
		"resultado":  "sucesso",
	})

	return nil
}

// ============================================================================
// POST /admin/projections/rebuild/:name
// ============================================================================
func RebuildProjection(c *gin.Context) {
	adminID, _ := middleware.GetUserID(c)
	name := c.Param("name")

	if err := runProjectionRebuild(c, adminID, name); err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, err.Error(), err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "projeção reconstruída com sucesso",
		"projection": name,
	})
}

// RebuildProjectionAsync enfileira o rebuild de uma projeção para execução em background.
// Endpoint: POST /dominis/projections/rebuild/:name/async
func RebuildProjectionAsync(c *gin.Context) {
	adminID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)
	name := c.Param("name")
	if name == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("nome da projeção é obrigatório"))
		return
	}

	store := getJobStore(c)
	if store == nil {
		return
	}

	payloadItem, err := json.Marshal(rebuildProjectionRequest{Name: name})
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	payload, err := json.Marshal([]json.RawMessage{payloadItem})
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	j, err := store.Enqueue(jobs.JobTypeRebuildProjection, adminID, userType, payload, 1)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	if w := getJobWorker(c); w != nil {
		w.Enqueue(j)
	}
	if n := getJobNotifier(c); n != nil {
		n.Publish(adminID, jobs.Event{
			Type:       jobs.EventJobEnqueued,
			JobID:      j.ID,
			JobType:    j.Type,
			Status:     j.Status,
			Progress:   j.Progress(),
			DoneItems:  j.DoneItems,
			FailItems:  j.FailItems,
			TotalItems: j.TotalItems,
		})
	}

	c.JSON(http.StatusAccepted, gin.H{
		"message":     "rebuild enfileirado com sucesso — use GET /jobs/:id para acompanhar o progresso",
		"projection":  name,
		"job_id":      j.ID,
		"status":      j.Status,
		"total_items": j.TotalItems,
		"poll_url":    fmt.Sprintf("/jobs/%s", j.ID),
	})
}

// RebuildProjectionJobItem processa 1 item de rebuild no worker de jobs.
// Espera body JSON: {"name": "admins"}
func RebuildProjectionJobItem(c *gin.Context) {
	adminID, _ := middleware.GetUserID(c)

	var req rebuildProjectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("payload inválido: informe {name}"))
		return
	}

	if err := runProjectionRebuild(c, adminID, req.Name); err != nil {
		utils.RespondWithError(c, http.StatusInternalServerError, err.Error(), err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "projeção reconstruída com sucesso",
		"projection": req.Name,
	})
}
