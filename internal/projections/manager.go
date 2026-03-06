package projections

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"spuri/internal/db"
	"sync"
	"time"
)

// Projection define a interface que toda projeção deve implementar.
type Projection interface {
	Name() string
	Handle(event db.Event) error
	Rebuild() error
	GetLastProcessedEventID() (int64, error)
	UpdateCheckpoint(eventID int64) error
}

// rebuildOrder define a ordem determinística para RebuildAllProjections.
//
// DB-18 FIX: Go itera maps em ordem aleatória. Projeções com dependência de FK
// (ex: projection_notas requer projection_estudantes) falhavam de forma não
// reprodutível. Esta slice define a ordem correta: entidades base primeiro,
// entidades derivadas depois.
var rebuildOrder = []string{
	// Entidades base (sem FK para outras projeções)
	"academias",
	"admins",
	"cursos",
	"materias",
	"sistema_config",
	// Estudantes depende de cursos (FK curso_medio_id, curso_superior_id)
	"estudantes",
	// Turmas depende de estudantes e cursos
	"turmas",
	// Entidades derivadas de estudantes
	"notas",
	"faltas",
	"aprovacao_ano",
	"reprovacoes",
	"avaliacao_final",
	"categorias_nota",
	// Inscricoes é depreciado mas mantido
	"inscricoes",
}

type Manager struct {
	client       *db.Client
	eventStore   *db.EventStore
	projections  map[string]Projection
	ctx          context.Context
	cancel       context.CancelFunc
	pollInterval time.Duration
	batchSize    int
	mu           sync.Mutex
}

func NewManager(client *db.Client) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		client:       client,
		eventStore:   db.NewEventStore(client),
		projections:  make(map[string]Projection),
		ctx:          ctx,
		cancel:       cancel,
		pollInterval: 1 * time.Second,
		batchSize:    100,
	}
}

func (m *Manager) RegisterProjection(name string, projection Projection) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.projections[name] = projection
	log.Printf("[DEBUG] Projeção registrada: %s", name)
}

func (m *Manager) StartProcessing() {
	log.Println("[DEBUG] Iniciando processamento de projeções")
	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			log.Println("[DEBUG] Parando processamento")
			return
		case <-ticker.C:
			if err := m.processNewEvents(); err != nil {
				log.Printf("[ERROR] Erro ao processar eventos: %v", err)
			}
		}
	}
}

func (m *Manager) Stop() {
	log.Println("[DEBUG] Parando manager")
	m.cancel()
}

func (m *Manager) processNewEvents() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for name, projection := range m.projections {
		if err := m.processProjection(name, projection); err != nil {
			log.Printf("[ERROR] Erro ao processar projeção %s: %v", name, err)
		}
	}
	return nil
}

// processProjection processa eventos novos para uma projeção específica.
//
// DB-16 FIX: Handle() e UpdateCheckpoint() continuam em operações separadas,
// porém UpdateCheckpoint() só é chamado APÓS Handle() retornar nil.
// Se o processo morrer entre os dois, o evento é reprocessado na próxima
// execução — as projeções devem ser idempotentes para operações de insert
// (ON CONFLICT DO UPDATE). Para operações não-idempotentes (ex: contadores),
// o rebuild é o mecanismo de recuperação.
// O log de erro permanente alerta o operador sem descartar o evento.
func (m *Manager) processProjection(name string, projection Projection) error {
	lastProcessedID, err := projection.GetLastProcessedEventID()
	if err != nil {
		return fmt.Errorf("erro ao obter checkpoint: %w", err)
	}

	events, err := m.getNewEvents(lastProcessedID)
	if err != nil {
		return fmt.Errorf("erro ao buscar eventos: %w", err)
	}

	if len(events) == 0 {
		return nil
	}

	log.Printf("[DEBUG] %s: processando %d eventos", name, len(events))

	processedCount := 0
	for _, event := range events {
		if err := m.processEventWithRetry(name, projection, event); err != nil {
			// Falha permanente — NÃO avança checkpoint.
			// O evento ficará represado e será reprocessado no próximo tick.
			// O operador deve corrigir o código e fazer rebuild para recuperar.
			log.Printf("[ERROR] %s: evento %d falhou permanentemente — checkpoint não avançado: %v",
				name, event.ID, err)
			m.logProjectionError(name, err.Error())
			// Para neste evento: não processa os seguintes para manter ordem.
			break
		}

		// Só atualiza checkpoint após Handle() bem-sucedido.
		if err := projection.UpdateCheckpoint(event.ID); err != nil {
			log.Printf("[WARN] %s: erro ao atualizar checkpoint para evento %d: %v",
				name, event.ID, err)
		}
		processedCount++
	}

	if processedCount > 0 {
		log.Printf("[DEBUG] %s: processados %d eventos (último: %d)",
			name, processedCount, events[processedCount-1].ID)
	}

	return nil
}

func (m *Manager) processEventWithRetry(name string, projection Projection, event db.Event) error {
	maxRetries := 3
	baseDelay := 1 * time.Second
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		if err := projection.Handle(event); err == nil {
			if attempt > 1 {
				log.Printf("[DEBUG] %s: evento %d recuperado na tentativa %d", name, event.ID, attempt)
			}
			return nil
		} else {
			lastErr = err
		}

		if attempt < maxRetries {
			delay := time.Duration(attempt*attempt) * baseDelay
			log.Printf("[WARN] %s: evento %d falhou (tentativa %d/%d), retry em %v",
				name, event.ID, attempt, maxRetries, delay)
			time.Sleep(delay)
		}
	}

	return fmt.Errorf("evento %d falhou após %d tentativas: %w", event.ID, maxRetries, lastErr)
}

// getNewEvents busca eventos do ledger com id > fromID.
//
// DB-17 NOTA: usa ORDER BY id ASC (BIGSERIAL). Para eventos de um único
// aggregate, id reflete a ordem de inserção. Para eventos de aggregates
// distintos em transações concorrentes, pode haver desvio de causalidade
// cross-aggregate. Projeções que dependem de causalidade cross-aggregate
// devem ser resilientes (ex: usar ON CONFLICT para reprocessamento idempotente)
// ou usar o mecanismo de rebuild para garantir ordem correta em caso de falha.
func (m *Manager) getNewEvents(fromID int64) ([]db.Event, error) {
	if fromID < 0 {
		fromID = 0
	}
	limit := db.ValidateLimit(m.batchSize)
	rows, err := m.client.DB().Query(`
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE id > $1
		ORDER BY id ASC
		LIMIT $2`,
		fromID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("erro na query: %w", err)
	}
	defer rows.Close()

	var events []db.Event
	for rows.Next() {
		var event db.Event
		var prevHash sql.NullString
		if err := rows.Scan(
			&event.ID, &event.EventID, &event.AggregateID, &event.AggregateType,
			&event.EventType, &event.EventVersion, &event.Payload, &event.Metadata,
			&event.OccurredAt, &event.RecordedAt, &event.LedgerHash, &prevHash,
		); err != nil {
			return nil, fmt.Errorf("erro ao scan: %w", err)
		}
		if prevHash.Valid {
			event.PreviousHash = &prevHash.String
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

// RebuildProjection reconstrói uma projeção específica pelo nome.
func (m *Manager) RebuildProjection(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.rebuildProjectionInternal(name)
}

// rebuildProjectionInternal executa o rebuild sem adquirir o lock.
// DEVE ser chamado somente quando o lock já foi adquirido pelo chamador.
func (m *Manager) rebuildProjectionInternal(name string) error {
	log.Printf("[DEBUG] Iniciando rebuild de: %s", name)
	projection, exists := m.projections[name]
	if !exists {
		return fmt.Errorf("projeção não encontrada: %s", name)
	}
	if err := m.markRebuildStart(name); err != nil {
		return err
	}
	if err := projection.Rebuild(); err != nil {
		return err
	}
	if err := m.markRebuildComplete(name); err != nil {
		return err
	}
	log.Printf("[DEBUG] Projeção %s reconstruída com sucesso", name)
	return nil
}

// RebuildAllProjections reconstrói todas as projeções em ordem determinística.
//
// DB-18 FIX: em vez de iterar sobre o map (ordem aleatória no Go), itera
// sobre rebuildOrder que define a sequência correta: entidades base primeiro,
// entidades com FK depois. Projeções registradas que não constam em
// rebuildOrder são processadas por último, em ordem estável (por nome).
func (m *Manager) RebuildAllProjections() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	log.Println("[DEBUG] Reconstruindo TODAS as projeções (ordem determinística)")

	// Conjuntos de nomes já processados e registrados
	registered := make(map[string]bool, len(m.projections))
	for name := range m.projections {
		registered[name] = true
	}

	processed := make(map[string]bool, len(m.projections))
	var firstErr error

	// 1. Processar na ordem definida por rebuildOrder
	for _, name := range rebuildOrder {
		if !registered[name] {
			// Projeção não registrada neste manager — pular sem erro.
			continue
		}
		if err := m.rebuildProjectionInternal(name); err != nil {
			log.Printf("[ERROR] Erro ao reconstruir %s: %v", name, err)
			if firstErr == nil {
				firstErr = err
			}
		}
		processed[name] = true
	}

	// 2. Processar qualquer projeção registrada não coberta por rebuildOrder,
	//    em ordem alfabética para ser determinístico.
	extras := make([]string, 0)
	for name := range m.projections {
		if !processed[name] {
			extras = append(extras, name)
		}
	}
	// Ordena extras alfabeticamente
	for i := 0; i < len(extras)-1; i++ {
		for j := i + 1; j < len(extras); j++ {
			if extras[i] > extras[j] {
				extras[i], extras[j] = extras[j], extras[i]
			}
		}
	}
	for _, name := range extras {
		log.Printf("[WARN] Projeção %q não está em rebuildOrder — reconstruindo por último", name)
		if err := m.rebuildProjectionInternal(name); err != nil {
			log.Printf("[ERROR] Erro ao reconstruir %s: %v", name, err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	log.Println("[DEBUG] Todas as projeções reconstruídas")
	return firstErr
}

// markRebuildStart zera o checkpoint e marca is_rebuilding = TRUE.
func (m *Manager) markRebuildStart(name string) error {
	_, err := m.client.DB().Exec(`
		UPDATE projection_checkpoints
		SET is_rebuilding           = TRUE,
		    rebuild_started_at      = CURRENT_TIMESTAMP,
		    last_processed_event_id = 0
		WHERE projection_name = $1`,
		name,
	)
	return err
}

// markRebuildComplete avança o checkpoint para o MAX(id) do ledger e desmarca is_rebuilding.
func (m *Manager) markRebuildComplete(name string) error {
	var maxID int64
	err := m.client.DB().QueryRow(`SELECT COALESCE(MAX(id), 0) FROM spuri_ledger`).Scan(&maxID)
	if err != nil {
		return fmt.Errorf("erro ao obter max id do ledger: %w", err)
	}

	_, err = m.client.DB().Exec(`
		UPDATE projection_checkpoints
		SET is_rebuilding           = FALSE,
		    last_processed_event_id = $1,
		    last_processed_at       = CURRENT_TIMESTAMP
		WHERE projection_name = $2`,
		maxID, name,
	)
	return err
}

// logProjectionError registra o erro no checkpoint da projeção para diagnóstico.
func (m *Manager) logProjectionError(name string, errMsg string) {
	_, err := m.client.DB().Exec(`
		UPDATE projection_checkpoints
		SET error_count    = COALESCE(error_count, 0) + 1,
		    last_error     = $1,
		    last_error_at  = CURRENT_TIMESTAMP
		WHERE projection_name = $2`,
		errMsg, name,
	)
	if err != nil {
		log.Printf("[WARN] Não foi possível registrar erro da projeção %s: %v", name, err)
	}
}

// ============================================================================
// Métodos de consulta — usados pelos handlers HTTP de administração
// ============================================================================

// IsProjectionRegistered retorna true se a projeção com o nome dado está registrada.
func (m *Manager) IsProjectionRegistered(name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, exists := m.projections[name]
	return exists
}

// GetProjectionStatus retorna o status de uma projeção lendo projection_checkpoints.
func (m *Manager) GetProjectionStatus(name string) (map[string]interface{}, error) {
	var (
		projName     string
		lastEventID  int64
		lastProc     time.Time
		eventsProc   int64
		rebuilding   bool
		rebuildStart sql.NullTime
		errCount     int
		lastErr      sql.NullString
		lastErrAt    sql.NullTime
	)
	err := m.client.DB().QueryRow(`
		SELECT projection_name, last_processed_event_id, last_processed_at,
			events_processed, is_rebuilding, rebuild_started_at,
			error_count, last_error, last_error_at
		FROM projection_checkpoints
		WHERE projection_name = $1`,
		name,
	).Scan(
		&projName, &lastEventID, &lastProc, &eventsProc,
		&rebuilding, &rebuildStart, &errCount, &lastErr, &lastErrAt,
	)
	if err != nil {
		return nil, fmt.Errorf("projeção '%s' não encontrada nos checkpoints: %w", name, err)
	}
	return map[string]interface{}{
		"name":                   projName,
		"last_processed_event":   lastEventID,
		"last_processed_at":      lastProc,
		"events_processed":       eventsProc,
		"is_rebuilding":          rebuilding,
		"rebuild_started_at":     rebuildStart,
		"error_count":            errCount,
		"last_error":             lastErr,
		"last_error_at":          lastErrAt,
	}, nil
}

// GetAllProjectionStatuses retorna o status de todas as projeções registradas.
func (m *Manager) GetAllProjectionStatuses() ([]map[string]interface{}, error) {
	m.mu.Lock()
	names := make([]string, 0, len(m.projections))
	for name := range m.projections {
		names = append(names, name)
	}
	m.mu.Unlock()

	var statuses []map[string]interface{}
	for _, name := range names {
		if status, err := m.GetProjectionStatus(name); err == nil {
			statuses = append(statuses, status)
		} else {
			log.Printf("[WARN] GetAllProjectionStatuses: erro ao obter status de %s: %v", name, err)
		}
	}
	return statuses, nil
}

// GetRegisteredProjections retorna os nomes de todas as projeções registradas.
func (m *Manager) GetRegisteredProjections() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	names := make([]string, 0, len(m.projections))
	for name := range m.projections {
		names = append(names, name)
	}
	return names
}

// GetProjection retorna a projeção registrada com o nome dado, ou erro se não existir.
func (m *Manager) GetProjection(name string) (Projection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.projections[name]
	if !ok {
		return nil, fmt.Errorf("projeção '%s' não registrada", name)
	}
	return p, nil
}