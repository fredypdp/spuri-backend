package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"spuri/internal/jobs"
	"spuri/internal/middleware"
	"spuri/internal/utils"

	"github.com/gin-gonic/gin"
)

// enqueueAsyncBatch é o helper central para todos os endpoints batch assíncronos.
// Valida o tamanho do array, cria o job no store e o enfileira no worker.
// Retorna 202 Accepted com o job_id para polling.
func enqueueAsyncBatch(c *gin.Context, jobType jobs.JobType, maxItems int) {
	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	// Ler body bruto como array JSON para preservar o payload sem dupla serialização
	var rawItems []json.RawMessage
	if err := c.ShouldBindJSON(&rawItems); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("body deve ser um array JSON"))
		return
	}
	if len(rawItems) == 0 {
		utils.RespondWithValidationError(c, fmt.Errorf("array não pode ser vazio"))
		return
	}
	if len(rawItems) > maxItems {
		utils.RespondWithValidationError(c, fmt.Errorf("máximo de %d itens por batch assíncrono", maxItems))
		return
	}

	// Re-serializar o array completo como payload do job
	payloadBytes, err := json.Marshal(rawItems)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	store := getJobStore(c)
	if store == nil {
		return
	}

	j, err := store.Enqueue(jobType, userID, userType, payloadBytes, len(rawItems))
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	// Enfileirar no worker (não bloqueante)
	if w := getJobWorker(c); w != nil {
		w.Enqueue(j)
	}

	c.JSON(http.StatusAccepted, gin.H{
		"message":     "job criado com sucesso — use GET /jobs/:id para acompanhar o progresso",
		"job_id":      j.ID,
		"total_items": j.TotalItems,
		"status":      j.Status,
		"poll_url":    fmt.Sprintf("/jobs/%s", j.ID),
	})
}

// ============================================================================
// Admin — academias
// ============================================================================

// RegisterAcademiaBatchAsync enfileira o cadastro de até 500 academias.
// A versão síncrona (/batch) tem limite de 5 por causa do bcrypt.
// A versão assíncrona não tem esse gargalo pois processa em background.
func RegisterAcademiaBatchAsync(c *gin.Context) {
	enqueueAsyncBatch(c, jobs.JobTypeRegisterAcademiaBatch, 500)
}

func AtivarAcademiaBatchAsync(c *gin.Context) {
	enqueueAsyncBatch(c, jobs.JobTypeAtivarAcademiaBatch, 500)
}

func DesativarAcademiaBatchAsync(c *gin.Context) {
	enqueueAsyncBatch(c, jobs.JobTypeDesativarAcademiaBatch, 500)
}

// ============================================================================
// Academia — estudantes
// ============================================================================

// RegisterEstudanteBatchAsync enfileira o cadastro de até 1000 estudantes.
func RegisterEstudanteBatchAsync(c *gin.Context) {
	enqueueAsyncBatch(c, jobs.JobTypeRegisterEstudanteBatch, 1000)
}

// ============================================================================
// Academia — notas
// ============================================================================

func RegistrarNotaBatchAsync(c *gin.Context) {
	enqueueAsyncBatch(c, jobs.JobTypeRegistrarNotaBatch, 2000)
}

func AtualizarNotaBatchAsync(c *gin.Context) {
	enqueueAsyncBatch(c, jobs.JobTypeAtualizarNotaBatch, 2000)
}

func DeletarNotaBatchAsync(c *gin.Context) {
	enqueueAsyncBatch(c, jobs.JobTypeDeletarNotaBatch, 2000)
}

// ============================================================================
// Academia — faltas
// ============================================================================

func RegistrarFaltasBatchAsync(c *gin.Context) {
	enqueueAsyncBatch(c, jobs.JobTypeRegistrarFaltasBatch, 2000)
}

func AtualizarFaltaBatchAsync(c *gin.Context) {
	enqueueAsyncBatch(c, jobs.JobTypeAtualizarFaltaBatch, 2000)
}

func DeletarFaltaBatchAsync(c *gin.Context) {
	enqueueAsyncBatch(c, jobs.JobTypeDeletarFaltaBatch, 2000)
}

// ============================================================================
// Academia — avaliação final
// ============================================================================

func RegistrarAvaliacaoFinalBatchAsync(c *gin.Context) {
	enqueueAsyncBatch(c, jobs.JobTypeRegistrarAvaliacaoFinalBatch, 1000)
}

// ============================================================================
// Academia — status escolar
// ============================================================================

func AtualizarStatusEscolarBatchAsync(c *gin.Context) {
	enqueueAsyncBatch(c, jobs.JobTypeAtualizarStatusEscolarBatch, 1000)
}

// ============================================================================
// Academia — cursos, matérias, turmas
// ============================================================================

func CriarCursoBatchAsync(c *gin.Context) {
	enqueueAsyncBatch(c, jobs.JobTypeCriarCursoBatch, 200)
}

func CriarMateriaBatchAsync(c *gin.Context) {
	enqueueAsyncBatch(c, jobs.JobTypeCriarMateriaBatch, 500)
}

func CriarTurmaBatchAsync(c *gin.Context) {
	enqueueAsyncBatch(c, jobs.JobTypeCriarTurmaBatch, 200)
}

func AdicionarEstudanteBatchAsync(c *gin.Context) {
	enqueueAsyncBatch(c, jobs.JobTypeAdicionarEstudanteBatch, 1000)
}