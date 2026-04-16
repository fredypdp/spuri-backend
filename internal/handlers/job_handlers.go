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
	store := getJobStore(c)
	if notifier == nil {
		utils.RespondWithInternalError(c, fmt.Errorf("notifier de jobs não disponível"))
		return
	}
	if store == nil {
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
			hidden, err := store.IsHiddenFromSSE(userID, ev.JobID)
			if err != nil {
				utils.RespondWithInternalError(c, err)
				return false
			}
			if hidden {
				return true
			}
			payload, _ := json.Marshal(ev)
			c.Writer.WriteString("event: " + string(ev.Type) + "\n")
			c.Writer.WriteString("data: " + string(payload) + "\n\n")
			return true
		}
	})
}

// HideJobFromSSE permite que a academia oculte um job no stream SSE.
// Endpoint: DELETE /jobs/:id/sse
func HideJobFromSSE(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)
	if userType != "academia" {
		utils.RespondWithForbiddenError(c, "apenas academias podem ocultar jobs no SSE")
		return
	}

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
	if j.UserID != userID {
		utils.RespondWithForbiddenError(c, "acesso negado")
		return
	}

	if err := store.HideFromSSE(userID, jobID); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "job ocultado do stream SSE com sucesso",
		"job_id":  jobID,
	})
}

// RetryFailedJob reenvia apenas itens com falha em um novo job.
// Endpoint: POST /jobs/:id/retry-failed
func RetryFailedJob(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)
	if userType != "academia" {
		utils.RespondWithForbiddenError(c, "apenas academias podem reenviar itens falhados")
		return
	}

	jobID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	store := getJobStore(c)
	if store == nil {
		return
	}

	originalJob, err := store.Get(jobID)
	if err != nil {
		utils.RespondWithNotFoundError(c, "job")
		return
	}
	if originalJob.UserID != userID {
		utils.RespondWithForbiddenError(c, "acesso negado")
		return
	}
	if originalJob.FailItems == 0 {
		utils.RespondWithValidationError(c, fmt.Errorf("job não possui itens falhados para retentar"))
		return
	}

	retryPayload, err := buildRetryFailedPayload(originalJob)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	if len(retryPayload) == 0 {
		utils.RespondWithValidationError(c, fmt.Errorf("não foi possível extrair itens falhados para retentar"))
		return
	}

	retryJob, err := store.Enqueue(originalJob.Type, userID, userType, retryPayload, len(retryPayload))
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	if w := getJobWorker(c); w != nil {
		w.Enqueue(retryJob)
	}
	if n := getJobNotifier(c); n != nil {
		n.Publish(userID, jobs.Event{
			Type:       jobs.EventJobEnqueued,
			JobID:      retryJob.ID,
			JobType:    retryJob.Type,
			Status:     retryJob.Status,
			Progress:   retryJob.Progress(),
			DoneItems:  retryJob.DoneItems,
			FailItems:  retryJob.FailItems,
			TotalItems: retryJob.TotalItems,
		})
	}

	c.JSON(http.StatusAccepted, gin.H{
		"message":         "job de retry criado com sucesso",
		"original_job_id": originalJob.ID,
		"retry_job_id":    retryJob.ID,
		"retry_items":     retryJob.TotalItems,
		"status":          retryJob.Status,
		"poll_url":        fmt.Sprintf("/jobs/%s", retryJob.ID),
		"sse_url":         "/jobs/stream",
	})
}

func buildRetryFailedPayload(j *jobs.Job) (json.RawMessage, error) {
	var failedItems []json.RawMessage
	for _, r := range j.Results {
		if r.Sucesso {
			continue
		}
		if len(r.Payload) > 0 {
			failedItems = append(failedItems, r.Payload)
		}
	}

	// Fallback: em casos legados, tenta extrair do payload original usando índice.
	if len(failedItems) == 0 && len(j.Payload) > 0 {
		var originalItems []json.RawMessage
		if err := json.Unmarshal(j.Payload, &originalItems); err != nil {
			return nil, fmt.Errorf("falha ao decodificar payload original: %w", err)
		}
		for _, r := range j.Results {
			if r.Sucesso || r.Index < 0 || r.Index >= len(originalItems) {
				continue
			}
			failedItems = append(failedItems, originalItems[r.Index])
		}
	}

	retryPayload, err := json.Marshal(failedItems)
	if err != nil {
		return nil, fmt.Errorf("falha ao serializar payload de retry: %w", err)
	}
	return retryPayload, nil
}
