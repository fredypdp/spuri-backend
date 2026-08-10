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
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"spuri/internal/db"
)

// HandlerFunc é a assinatura de um handler Gin usado pelo worker.
type HandlerFunc func(c *gin.Context)

// ContextSetupFunc injeta dependências (dbClient, repository, etc.) no contexto
// sintético antes de executar um handler. O caller (main.go) fornece esta função.
type ContextSetupFunc func(c *gin.Context, userID uuid.UUID, userType string)

// Worker processa jobs de uma fila em background.
type Worker struct {
	store        *Store
	notifier     *Notifier
	setupCtx     ContextSetupFunc
	handlers     map[JobType]HandlerFunc
	queue        chan *Job
	concurrency  int
	stopCh       chan struct{}
	inFlightMu   sync.Mutex
	inFlightJobs map[uuid.UUID]struct{}
}

// NewWorker cria um worker com o número de goroutines especificado.
func NewWorker(store *Store, notifier *Notifier, setupCtx ContextSetupFunc, concurrency int) *Worker {
	if concurrency <= 0 {
		concurrency = 3
	}
	return &Worker{
		store:        store,
		notifier:     notifier,
		setupCtx:     setupCtx,
		handlers:     make(map[JobType]HandlerFunc),
		queue:        make(chan *Job, 500),
		concurrency:  concurrency,
		stopCh:       make(chan struct{}),
		inFlightJobs: make(map[uuid.UUID]struct{}),
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
	if !w.markInFlight(j.ID) {
		return
	}
	select {
	case w.queue <- j:
	default:
		w.unmarkInFlight(j.ID)
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
	minInterval := 30 * time.Second
	maxInterval := 30 * time.Minute
	currentInterval := minInterval

	for {
		timer := time.NewTimer(currentInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-w.stopCh:
			timer.Stop()
			return
		case <-timer.C:
		}

		active, err := w.store.ListActive(500)
		if err != nil {
			// Erro na varredura em si não é motivo para reagir depressa: o
			// despacho real de jobs criados em operação normal já acontece
			// via Enqueue() direto, não depende deste loop. Pular para o
			// teto evita insistir no banco em caso de instabilidade
			// transitória.
			log.Printf("[worker] WARN: erro na varredura de jobs ativos: %v", err)
			currentInterval = maxInterval
			continue
		}

		for _, j := range active {
			w.Enqueue(j)
		}

		if len(active) > 0 {
			// Ainda há jobs pending/processing — manter o piso rápido para
			// recuperar rapidamente um backlog real (ex.: muitos jobs após
			// reinício do servidor).
			currentInterval = minInterval
		} else {
			// Nenhum job ativo: pular direto para o teto em vez de uma
			// rampa gradual. Jobs criados em operação normal continuam
			// imediatos via Enqueue() direto nos handlers — este loop é
			// só a rede de segurança para reinício/fila cheia.
			currentInterval = maxInterval
		}

		log.Printf("[worker] varredura de jobs ativos — ativos=%d fila=%d próximo_intervalo=%s", len(active), len(w.queue), currentInterval)
	}
}

func (w *Worker) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(24 * time.Hour)
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
	defer w.unmarkInFlight(j.ID)

	latest, err := w.store.Get(j.ID)
	if err == nil && latest != nil {
		j = latest
	}
	if j.IsDone() {
		return
	}

	log.Printf("[worker] iniciando job %s type=%s items=%d", j.ID, j.Type, j.TotalItems)

	if err := w.store.UpdateStatus(j.ID, StatusProcessing, ""); err != nil {
		log.Printf("[worker] ERR: UpdateStatus processing %s: %v", j.ID, err)
	}
	j.Status = StatusProcessing
	w.publishProgress(j, EventJobProgress)

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
		if idx < (j.DoneItems + j.FailItems) {
			continue // retoma do ponto salvo
		}
		result := w.processItem(h, j, idx, rawItem)
		if err := w.store.AppendResult(j.ID, result); err != nil {
			log.Printf("[worker] WARN: AppendResult idx=%d job=%s: %v", idx, j.ID, err)
			if db.IsTransientConnectionError(err) {
				log.Printf("[worker] WARN: job %s pausado por indisponibilidade transitória do banco; progresso persistido será retomado", j.ID)
				return
			}
		}
		if latest, err := w.store.Get(j.ID); err == nil && latest != nil {
			j = latest
		}
		w.publishProgress(j, EventJobProgress)
	}

	// Se qualquer item falhar, o job inteiro deve refletir falha.
	// Antes, jobs com falha parcial ficavam como "done", o que fazia
	// a API reportar sucesso mesmo quando apenas parte do batch foi aplicada.
	finalStatus := StatusDone
	finalError := ""
	if j.FailItems > 0 {
		finalStatus = StatusFailed
		finalError = w.buildFailureReason(j)
	}

	if err := w.store.UpdateStatus(j.ID, finalStatus, finalError); err != nil {
		log.Printf("[worker] ERR: UpdateStatus final %s: %v", j.ID, err)
	}
	j.Status = finalStatus
	j.Error = finalError
	if finalStatus == StatusDone {
		w.publishProgress(j, EventJobDone)
	} else {
		w.publishProgress(j, EventJobFailed)
	}

	log.Printf("[worker] job %s concluído: ok=%d fail=%d", j.ID, j.DoneItems, j.FailItems)
}

func (w *Worker) buildFailureReason(j *Job) string {
	if j.FailItems == 0 {
		return ""
	}
	samples := make([]string, 0, 3)
	for _, r := range j.Results {
		if r.Sucesso || r.Erro == "" {
			continue
		}
		samples = append(samples, fmt.Sprintf("item[%d]: %s", r.Index, r.Erro))
		if len(samples) == 3 {
			break
		}
	}
	if len(samples) == 0 {
		return fmt.Sprintf("job concluído com %d falha(s)", j.FailItems)
	}
	return fmt.Sprintf("job concluído com %d falha(s): %s", j.FailItems, strings.Join(samples, " | "))
}

func (w *Worker) markInFlight(jobID uuid.UUID) bool {
	w.inFlightMu.Lock()
	defer w.inFlightMu.Unlock()
	if _, exists := w.inFlightJobs[jobID]; exists {
		return false
	}
	w.inFlightJobs[jobID] = struct{}{}
	return true
}

func (w *Worker) unmarkInFlight(jobID uuid.UUID) {
	w.inFlightMu.Lock()
	defer w.inFlightMu.Unlock()
	delete(w.inFlightJobs, jobID)
}

func (w *Worker) publishProgress(j *Job, eventType EventType) {
	if w.notifier == nil {
		return
	}
	w.notifier.Publish(j.UserID, Event{
		Type:       eventType,
		JobID:      j.ID,
		JobType:    j.Type,
		Status:     j.Status,
		Progress:   j.Progress(),
		DoneItems:  j.DoneItems,
		FailItems:  j.FailItems,
		TotalItems: j.TotalItems,
		Error:      j.Error,
	})
}

// processItem executa o handler Gin em modo sintético para um único item.
func (w *Worker) processItem(h HandlerFunc, j *Job, idx int, rawItem json.RawMessage) ItemResult {
	// Criar ResponseRecorder
	rec := httptest.NewRecorder()
	rec.Code = http.StatusOK

	// Criar request sintético
	req, err := http.NewRequest(http.MethodPost, "/", io.NopCloser(bytes.NewReader(rawItem)))
	if err != nil {
		return ItemResult{Index: idx, Sucesso: false, Payload: rawItem, Erro: fmt.Sprintf("criar request: %v", err)}
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
		return ItemResult{Index: idx, Sucesso: true, Payload: rawItem, Dados: dados}
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

	return ItemResult{Index: idx, Sucesso: false, Payload: rawItem, Erro: msg}
}
