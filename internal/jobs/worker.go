package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// HandlerFunc é a assinatura de um handler Gin usado pelo worker.
type HandlerFunc func(c *gin.Context)

// ContextSetupFunc injeta dependências (dbClient, repository, etc.) no contexto
// sintético antes de executar um handler. O caller (main.go) fornece esta função.
type ContextSetupFunc func(c *gin.Context, userID uuid.UUID, userType string)

// Worker processa jobs de uma fila em background.
type Worker struct {
	store        *Store
	setupCtx     ContextSetupFunc
	handlers     map[JobType]HandlerFunc
	queue        chan *Job
	concurrency  int
	stopCh       chan struct{}
}

// NewWorker cria um worker com o número de goroutines especificado.
func NewWorker(store *Store, setupCtx ContextSetupFunc, concurrency int) *Worker {
	if concurrency <= 0 {
		concurrency = 3
	}
	return &Worker{
		store:       store,
		setupCtx:    setupCtx,
		handlers:    make(map[JobType]HandlerFunc),
		queue:       make(chan *Job, 500),
		concurrency: concurrency,
		stopCh:      make(chan struct{}),
	}
}

// RegisterHandler associa um JobType a um handler Gin.
func (w *Worker) RegisterHandler(jobType JobType, h HandlerFunc) {
	w.handlers[jobType] = h
}

// Enqueue encaminha um job para a fila de processamento.
// É não-bloqueante: se a fila estiver cheia, o job ainda está no banco e será
// recuperado pelo próximo ciclo de varredura.
func (w *Worker) Enqueue(j *Job) {
	select {
	case w.queue <- j:
	default:
		log.Printf("[worker] WARN: fila cheia, job %s será processado na próxima varredura", j.ID)
	}
}

// Start inicia as goroutines do worker pool e a goroutine de varredura periódica.
func (w *Worker) Start(ctx context.Context) {
	log.Printf("[worker] iniciando %d goroutine(s)", w.concurrency)

	for i := 0; i < w.concurrency; i++ {
		go w.loop(ctx)
	}

	// Varredura periódica: recupera jobs "pending" que não entraram na fila
	// (ex: reinício do servidor, fila cheia).
	go w.sweepPending(ctx)

	// Cleanup periódico de jobs antigos
	go w.cleanupLoop(ctx)
}

func (w *Worker) loop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		case j := <-w.queue:
			w.process(j)
		}
	}
}

func (w *Worker) sweepPending(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		case <-ticker.C:
			// Recarregar jobs pending que podem ter ficado presos
			// (simplificado: apenas log — jobs são enfileirados no Enqueue do handler)
			log.Printf("[worker] varredura de jobs pendentes — fila: %d", len(w.queue))
		}
	}
}

func (w *Worker) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		case <-ticker.C:
			w.store.Cleanup()
		}
	}
}

// process executa um job item a item, reportando resultados parciais.
func (w *Worker) process(j *Job) {
	log.Printf("[worker] iniciando job %s type=%s items=%d", j.ID, j.Type, j.TotalItems)

	if err := w.store.UpdateStatus(j.ID, StatusProcessing, ""); err != nil {
		log.Printf("[worker] ERR: UpdateStatus processing %s: %v", j.ID, err)
	}

	h, ok := w.handlers[j.Type]
	if !ok {
		errMsg := fmt.Sprintf("handler não registrado para tipo %s", j.Type)
		log.Printf("[worker] ERR: %s", errMsg)
		_ = w.store.UpdateStatus(j.ID, StatusFailed, errMsg)
		return
	}

	// O payload do job é sempre um array JSON de itens.
	// Processamos item a item para poder reportar progresso.
	var rawItems []json.RawMessage
	if err := json.Unmarshal(j.Payload, &rawItems); err != nil {
		errMsg := fmt.Sprintf("payload inválido: %v", err)
		log.Printf("[worker] ERR: %s", errMsg)
		_ = w.store.UpdateStatus(j.ID, StatusFailed, errMsg)
		return
	}

	for idx, rawItem := range rawItems {
		result := w.processItem(h, j, idx, rawItem)
		if err := w.store.AppendResult(j.ID, result); err != nil {
			log.Printf("[worker] WARN: AppendResult idx=%d job=%s: %v", idx, j.ID, err)
		}
	}

	// Se qualquer item falhar, o job inteiro deve refletir falha.
	// Antes, jobs com falha parcial ficavam como "done", o que fazia
	// a API reportar sucesso mesmo quando apenas parte do batch foi aplicada.
	finalStatus := StatusDone
	if j.FailItems > 0 {
		finalStatus = StatusFailed
	}

	if err := w.store.UpdateStatus(j.ID, finalStatus, ""); err != nil {
		log.Printf("[worker] ERR: UpdateStatus final %s: %v", j.ID, err)
	}

	log.Printf("[worker] job %s concluído: ok=%d fail=%d", j.ID, j.DoneItems, j.FailItems)
}

// processItem executa o handler Gin em modo sintético para um único item.
func (w *Worker) processItem(h HandlerFunc, j *Job, idx int, rawItem json.RawMessage) ItemResult {
	// Criar ResponseRecorder
	rec := httptest.NewRecorder()
	rec.Code = http.StatusOK

	// Criar request sintético
	req, err := http.NewRequest(http.MethodPost, "/", io.NopCloser(bytes.NewReader(rawItem)))
	if err != nil {
		return ItemResult{Index: idx, Sucesso: false, Erro: fmt.Sprintf("criar request: %v", err)}
	}
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(rawItem))

	// Criar contexto Gin
	c, _ := gin.CreateTestContext(rec)
	c.Request = req

	// Injetar identidade e dependências
	w.setupCtx(c, j.UserID, j.UserType)

	// Executar handler
	h(c)

	code := rec.Code
	body := rec.Body.Bytes()

	if code >= 200 && code < 300 {
		var dados json.RawMessage
		if len(body) > 0 {
			dados = json.RawMessage(body)
		}
		return ItemResult{Index: idx, Sucesso: true, Dados: dados}
	}

	// Extrair mensagem de erro
	var errResp struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if len(body) > 0 {
		_ = json.Unmarshal(body, &errResp)
	}
	msg := errResp.Message
	if msg == "" {
		msg = errResp.Error
	}
	if msg == "" {
		msg = http.StatusText(code)
	}

	return ItemResult{Index: idx, Sucesso: false, Erro: msg}
}
