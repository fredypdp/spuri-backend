package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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

	// Ler body bruto e validar que é um array JSON sem re-serializar.
	// Isso reduz CPU/memória em batches grandes (ex.: notas/faltas com 2000 itens)
	// e evita brechas de timeout no endpoint de enqueue.
	payloadBytes, totalItems, err := readAndValidateJSONArrayBody(c, maxItems)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if totalItems == 0 {
		utils.RespondWithValidationError(c, fmt.Errorf("array não pode ser vazio"))
		return
	}

	store := getJobStore(c)
	if store == nil {
		return
	}

	j, err := store.Enqueue(jobType, userID, userType, json.RawMessage(payloadBytes), totalItems)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	// Enfileirar no worker (não bloqueante)
	if w := getJobWorker(c); w != nil {
		w.Enqueue(j)
	}
	if n := getJobNotifier(c); n != nil {
		n.Publish(userID, jobs.Event{
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
		"message":     "job criado com sucesso — use GET /jobs/:id ou GET /jobs/stream para acompanhar o progresso",
		"job_id":      j.ID,
		"total_items": j.TotalItems,
		"status":      j.Status,
		"poll_url":    fmt.Sprintf("/jobs/%s", j.ID),
		"sse_url":     "/jobs/stream",
	})
}

func readAndValidateJSONArrayBody(c *gin.Context, maxItems int) ([]byte, int, error) {
	if c.Request == nil || c.Request.Body == nil {
		return nil, 0, fmt.Errorf("body deve ser um array JSON")
	}

	payloadBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("erro ao ler body")
	}
	if len(bytes.TrimSpace(payloadBytes)) == 0 {
		return nil, 0, fmt.Errorf("body deve ser um array JSON")
	}

	decoder := json.NewDecoder(bytes.NewReader(payloadBytes))
	tok, err := decoder.Token()
	if err != nil {
		return nil, 0, fmt.Errorf("body deve ser um array JSON")
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '[' {
		return nil, 0, fmt.Errorf("body deve ser um array JSON")
	}

	count := 0
	for decoder.More() {
		var item json.RawMessage
		if err := decoder.Decode(&item); err != nil {
			return nil, 0, fmt.Errorf("body deve ser um array JSON válido")
		}
		count++
		if count > maxItems {
			return nil, 0, fmt.Errorf("máximo de %d itens por batch assíncrono", maxItems)
		}
	}

	endTok, err := decoder.Token()
	if err != nil {
		return nil, 0, fmt.Errorf("body deve ser um array JSON")
	}
	endDelim, ok := endTok.(json.Delim)
	if !ok || endDelim != ']' {
		return nil, 0, fmt.Errorf("body deve ser um array JSON")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, 0, fmt.Errorf("body deve conter apenas um array JSON")
	}

	return payloadBytes, count, nil
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

func AtivarAdminBatchAsync(c *gin.Context) {
	enqueueAsyncBatch(c, jobs.JobTypeAtivarAdminBatch, 500)
}

func DesativarAdminBatchAsync(c *gin.Context) {
	enqueueAsyncBatch(c, jobs.JobTypeDesativarAdminBatch, 500)
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

func AtualizarDadosAcademiaBatchAsync(c *gin.Context) {
	enqueueAsyncBatch(c, jobs.JobTypeAtualizarDadosAcademiaBatch, 200)
}

func CriarCategoriaNotaBatchAsync(c *gin.Context) {
	enqueueAsyncBatch(c, jobs.JobTypeCriarCategoriaNotaBatch, 500)
}

func DeletarCategoriaNotaBatchAsync(c *gin.Context) {
	enqueueAsyncBatch(c, jobs.JobTypeDeletarCategoriaNotaBatch, 500)
}

func AtivarCursoBatchAsync(c *gin.Context) {
	enqueueAsyncBatch(c, jobs.JobTypeAtivarCursoBatch, 500)
}

func DesativarCursoBatchAsync(c *gin.Context) {
	enqueueAsyncBatch(c, jobs.JobTypeDesativarCursoBatch, 500)
}

func AtualizarDadosCursoBatchAsync(c *gin.Context) {
	enqueueAsyncBatch(c, jobs.JobTypeAtualizarDadosCursoBatch, 500)
}

func DeletarCursoBatchAsync(c *gin.Context) {
	enqueueAsyncBatch(c, jobs.JobTypeDeletarCursoBatch, 500)
}

func AtivarMateriaBatchAsync(c *gin.Context) {
	enqueueAsyncBatch(c, jobs.JobTypeAtivarMateriaBatch, 1000)
}

func DesativarMateriaBatchAsync(c *gin.Context) {
	enqueueAsyncBatch(c, jobs.JobTypeDesativarMateriaBatch, 1000)
}

func DefinirPeriodoMateriaBatchAsync(c *gin.Context) {
	enqueueAsyncBatch(c, jobs.JobTypeDefinirPeriodoMateriaBatch, 1000)
}

func AtualizarDadosMateriaBatchAsync(c *gin.Context) {
	enqueueAsyncBatch(c, jobs.JobTypeAtualizarDadosMateriaBatch, 1000)
}

func DeletarMateriaBatchAsync(c *gin.Context) {
	enqueueAsyncBatch(c, jobs.JobTypeDeletarMateriaBatch, 1000)
}

func AtivarTurmaBatchAsync(c *gin.Context) {
	enqueueAsyncBatch(c, jobs.JobTypeAtivarTurmaBatch, 500)
}

func DesativarTurmaBatchAsync(c *gin.Context) {
	enqueueAsyncBatch(c, jobs.JobTypeDesativarTurmaBatch, 500)
}

func AtualizarDadosTurmaBatchAsync(c *gin.Context) {
	enqueueAsyncBatch(c, jobs.JobTypeAtualizarDadosTurmaBatch, 500)
}

func DeletarTurmaBatchAsync(c *gin.Context) {
	enqueueAsyncBatch(c, jobs.JobTypeDeletarTurmaBatch, 500)
}

func RemoverEstudanteTurmaBatchAsync(c *gin.Context) {
	enqueueAsyncBatch(c, jobs.JobTypeRemoverEstudanteTurmaBatch, 1000)
}
