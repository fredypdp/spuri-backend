package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"spuri/internal/jobs"
	"spuri/internal/middleware"
	"spuri/internal/utils"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// getJobStore retorna o *jobs.Store injetado no contexto.
func getJobStore(c *gin.Context) *jobs.Store {
	raw, exists := c.Get("jobStore")
	if !exists {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error": "erro interno: job store não disponível",
		})
		return nil
	}
	s, ok := raw.(*jobs.Store)
	if !ok || s == nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error": "erro interno: job store inválido",
		})
		return nil
	}
	return s
}

// getJobWorker retorna o *jobs.Worker injetado no contexto.
func getJobWorker(c *gin.Context) *jobs.Worker {
	raw, exists := c.Get("jobWorker")
	if !exists {
		return nil
	}
	w, _ := raw.(*jobs.Worker)
	return w
}

func getJobNotifier(c *gin.Context) *jobs.Notifier {
	raw, exists := c.Get("jobNotifier")
	if !exists {
		return nil
	}
	n, _ := raw.(*jobs.Notifier)
	return n
}

// ============================================================================
// GET /jobs/:id — polling de status
// ============================================================================

// GetJob retorna o status atual de um job assíncrono.
// Com ?results=true inclui os resultados parciais/finais.
// Sem esse parâmetro retorna apenas o summary (mais leve para polling frequente).
//
// Qualquer usuário autenticado pode consultar apenas seus próprios jobs.
func GetJob(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	jobID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	store := getJobStore(c)
	if store == nil {
		return
	}

	j, err := store.Get(jobID)
	if err != nil {
		utils.RespondWithNotFoundError(c, "job")
		return
	}

	// Usuário só pode ver seus próprios jobs
	if j.UserID != userID {
		utils.RespondWithForbiddenError(c, "acesso negado")
		return
	}

	withResults := c.Query("results") == "true"

	if withResults {
		c.JSON(http.StatusOK, gin.H{
			"job":     j.ToSummary(),
			"results": j.Results,
		})
		return
	}

	c.JSON(http.StatusOK, j.ToSummary())
}

// ============================================================================
// GET /jobs — listar jobs do usuário autenticado
// ============================================================================

// ListJobs lista os jobs mais recentes do usuário autenticado.
func ListJobs(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	store := getJobStore(c)
	if store == nil {
		return
	}

	jobList, err := store.GetByUser(userID)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	summaries := make([]jobs.Summary, 0, len(jobList))
	for _, j := range jobList {
		summaries = append(summaries, j.ToSummary())
	}

	c.JSON(http.StatusOK, gin.H{
		"jobs":  summaries,
		"total": len(summaries),
	})
}

// StreamJobs abre um canal SSE com notificações em tempo real de jobs do usuário.
// Endpoint: GET /jobs/stream
func StreamJobs(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	notifier := getJobNotifier(c)
	if notifier == nil {
		utils.RespondWithInternalError(c, fmt.Errorf("notifier de jobs não disponível"))
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	events := notifier.Subscribe(userID)
	defer notifier.Unsubscribe(userID, events)

	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()

	c.Stream(func(w io.Writer) bool {
		select {
		case <-c.Request.Context().Done():
			return false
		case <-heartbeat.C:
			c.Writer.WriteString(": ping\n\n")
			return true
		case ev, ok := <-events:
			if !ok {
				return false
			}
			payload, _ := json.Marshal(ev)
			c.Writer.WriteString("event: " + string(ev.Type) + "\n")
			c.Writer.WriteString("data: " + string(payload) + "\n\n")
			return true
		}
	})
}
