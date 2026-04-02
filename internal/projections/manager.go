package projections

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sort"
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

// TransactionalProjection é uma interface opcional que projeções não-idempotentes
// devem implementar para garantir atomicidade real entre Handle e checkpoint.
type TransactionalProjection interface {
	Projection
	HandleTx(tx *sql.Tx, event db.Event) error
}

// Manager gerencia o ciclo de vida e o processamento de projeções.
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
	snapshot := make(map[string]Projection, len(m.projections))
	for name, p := range m.projections {
		snapshot[name] = p
	}
	m.mu.Unlock()

	for name, projection := range snapshot {
		if err := m.processProjection(name, projection); err != nil {
			log.Printf("[ERROR] Erro ao processar projeção %s: %v", name, err)
		}
	}
	return nil
}

func (m *Manager) processProjection(name string, projection Projection) error {
	lastID, err := projection.GetLastProcessedEventID()
	if err != nil {
		return fmt.Errorf("erro ao obter checkpoint de %s: %w", name, err)
	}

	events, err := m.getNewEvents(lastID)
	if err != nil {
		return fmt.Errorf("erro ao buscar eventos para %s: %w", name, err)
	}

	if len(events) == 0 {
		return nil
	}

	txProjection, isTransactional := projection.(TransactionalProjection)

	for _, event := range events {
		if isTransactional {
			if err := m.processEventTransactional(name, txProjection, event); err != nil {
				m.logProjectionError(name, err.Error())
				log.Printf("[ERROR] %s: falha permanente no evento %d: %v", name, event.ID, err)
				return err
			}
		} else {
			if err := m.processEventWithRetry(name, projection, event); err != nil {
				m.logProjectionError(name, err.Error())
				log.Printf("[ERROR] %s: falha permanente no evento %d: %v", name, event.ID, err)
				return err
			}
			if err := m.commitCheckpoint(projection, event.ID); err != nil {
				log.Printf("[WARN] %s: erro ao gravar checkpoint para evento %d: %v", name, event.ID, err)
			}
		}
	}

	return nil
}

func (m *Manager) processEventTransactional(name string, projection TransactionalProjection, event db.Event) error {
	tx, err := m.client.DB().Begin()
	if err != nil {
		return fmt.Errorf("erro ao iniciar transação: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if err = projection.HandleTx(tx, event); err != nil {
		return fmt.Errorf("HandleTx falhou: %w", err)
	}

	if _, err = tx.Exec(`
		INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at, events_processed)
		VALUES ($1, $2, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) DO UPDATE SET
			last_processed_event_id = $2,
			last_processed_at       = CURRENT_TIMESTAMP,
			events_processed        = projection_checkpoints.events_processed + 1
	`, projection.Name(), event.ID); err != nil {
		return fmt.Errorf("erro ao gravar checkpoint transacional: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("erro ao commitar transação: %w", err)
	}

	committed = true
	return nil
}

func (m *Manager) processEventWithRetry(name string, projection Projection, event db.Event) error {
	maxRetries := 3
	backoff := 1 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := projection.Handle(event)
		if err == nil {
			return nil
		}
		log.Printf("[WARN] %s: tentativa %d/%d falhou para evento %d: %v", name, attempt, maxRetries, event.ID, err)
		if attempt < maxRetries {
			time.Sleep(backoff)
			backoff *= 3
		}
	}
	return fmt.Errorf("falha após %d tentativas no evento %d", maxRetries, event.ID)
}

func (m *Manager) commitCheckpoint(projection Projection, eventID int64) error {
	return projection.UpdateCheckpoint(eventID)
}

// ============================================================================
// Rebuild
// ============================================================================

func (m *Manager) RebuildProjection(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	projection, ok := m.projections[name]
	if !ok {
		return fmt.Errorf("projeção não encontrada: %s", name)
	}

	return m.rebuildProjectionInternal(name, projection)
}

// RebuildAllProjections reconstrói todas as projeções em ordem determinística.
func (m *Manager) RebuildAllProjections() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	log.Printf("[SECURITY] RebuildAll: verificando integridade do ledger antes de iniciar")
	if err := m.verifyFullLedgerIntegrity(); err != nil {
		return fmt.Errorf("rebuild abortado: integridade do ledger comprometida: %w", err)
	}
	log.Printf("[SECURITY] RebuildAll: ledger íntegro — iniciando reconstrução")

	// Ordem respeita dependências de FK entre projeções.
	rebuildOrder := []string{
		// Tier 1 — sem dependências externas
		"admins",
		"academias",
		"cursos",
		"materias",
		"categorias_nota",
		"telefones_extra",
		// Tier 2 — dependem de academias/cursos
		"estudantes",
		"turmas",
		// Tier 3 — dependem de estudantes e materias
		"notas",
		"faltas",
		// Tier 4 — avaliação final (depende de estudantes)
		"avaliacao_final",
	}

	processed := make(map[string]bool)

	for _, name := range rebuildOrder {
		projection, ok := m.projections[name]
		if !ok {
			log.Printf("[DEBUG] RebuildAll: projeção %q não registrada, pulando", name)
			continue
		}
		log.Printf("[DEBUG] RebuildAll: reconstruindo %s (tier ordenado)", name)
		if err := m.rebuildProjectionInternal(name, projection); err != nil {
			return fmt.Errorf("falha ao reconstruir %s: %w", name, err)
		}
		processed[name] = true
	}

	remaining := make([]string, 0)
	for name := range m.projections {
		if !processed[name] {
			remaining = append(remaining, name)
		}
	}
	sort.Strings(remaining)

	for _, name := range remaining {
		projection := m.projections[name]
		log.Printf("[DEBUG] RebuildAll: reconstruindo %s (ordem alfabética)", name)
		if err := m.rebuildProjectionInternal(name, projection); err != nil {
			return fmt.Errorf("falha ao reconstruir %s: %w", name, err)
		}
	}

	return nil
}

func (m *Manager) rebuildProjectionInternal(name string, projection Projection) error {
	log.Printf("[DEBUG] Reconstruindo projeção: %s", name)

	if err := m.markRebuildStart(name); err != nil {
		log.Printf("[WARN] %s: erro ao marcar início de rebuild: %v", name, err)
	}

	log.Printf("[SECURITY] %s: verificando integridade do ledger antes do rebuild", name)
	if err := m.verifyFullLedgerIntegrity(); err != nil {
		if resetErr := m.markRebuildFailed(name); resetErr != nil {
			log.Printf("[WARN] %s: erro ao resetar is_rebuilding após falha de integridade: %v", name, resetErr)
		}
		return fmt.Errorf("%s: rebuild abortado por integridade comprometida: %w", name, err)
	}
	log.Printf("[SECURITY] %s: ledger íntegro — prosseguindo com rebuild", name)

	rebuildErr := projection.Rebuild()
	if rebuildErr != nil {
		if resetErr := m.markRebuildFailed(name); resetErr != nil {
			log.Printf("[WARN] %s: erro ao resetar is_rebuilding após falha: %v", name, resetErr)
		}
		return fmt.Errorf("erro no rebuild de %s: %w", name, rebuildErr)
	}

	if err := m.markRebuildComplete(name); err != nil {
		log.Printf("[WARN] %s: erro ao marcar fim de rebuild: %v", name, err)
	}

	log.Printf("[DEBUG] Projeção %s reconstruída com sucesso", name)
	return nil
}

func (m *Manager) verifyFullLedgerIntegrity() error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	aggregateIDs, err := m.eventStore.GetDistinctAggregateIDs(ctx)
	if err != nil {
		return fmt.Errorf("erro ao listar aggregates para verificação: %w", err)
	}

	if len(aggregateIDs) == 0 {
		log.Printf("[SECURITY] Ledger vazio — nenhuma verificação necessária")
		return nil
	}

	log.Printf("[SECURITY] Verificando integridade de %d aggregate(s)", len(aggregateIDs))

	for _, aggID := range aggregateIDs {
		valid, err := m.eventStore.VerifyLedgerIntegrity(ctx, aggID)
		if err != nil {
			log.Printf("[SECURITY] ALERTA: aggregate %s comprometido: %v", aggID, err)
			return fmt.Errorf("aggregate %s: %w", aggID, err)
		}
		if !valid {
			return fmt.Errorf("aggregate %s: integridade inválida sem detalhes", aggID)
		}
	}

	log.Printf("[SECURITY] Verificação concluída: %d aggregate(s) íntegros", len(aggregateIDs))
	return nil
}

// ============================================================================
// Checkpoint helpers
// ============================================================================

func (m *Manager) markRebuildStart(name string) error {
	_, err := m.client.DB().Exec(`
		INSERT INTO projection_checkpoints
			(projection_name, last_processed_event_id, last_processed_at, events_processed, is_rebuilding, rebuild_started_at)
		VALUES ($1, 0, CURRENT_TIMESTAMP, 0, TRUE, CURRENT_TIMESTAMP)
		ON CONFLICT (projection_name) DO UPDATE SET
			last_processed_event_id = 0,
			last_processed_at       = CURRENT_TIMESTAMP,
			events_processed        = 0,
			is_rebuilding           = TRUE,
			rebuild_started_at      = CURRENT_TIMESTAMP
	`, name)
	return err
}

func (m *Manager) markRebuildComplete(name string) error {
	var maxID int64
	err := m.client.DB().QueryRow(`SELECT COALESCE(MAX(id), 0) FROM spuri_ledger`).Scan(&maxID)
	if err != nil {
		return err
	}

	_, err = m.client.DB().Exec(`
		INSERT INTO projection_checkpoints
			(projection_name, last_processed_event_id, last_processed_at, is_rebuilding)
		VALUES ($1, $2, CURRENT_TIMESTAMP, FALSE)
		ON CONFLICT (projection_name) DO UPDATE SET
			last_processed_event_id = $2,
			last_processed_at       = CURRENT_TIMESTAMP,
			is_rebuilding           = FALSE
	`, name, maxID)
	return err
}

func (m *Manager) markRebuildFailed(name string) error {
	_, err := m.client.DB().Exec(`
		INSERT INTO projection_checkpoints
			(projection_name, last_processed_event_id, last_processed_at, is_rebuilding)
		VALUES ($1, 0, CURRENT_TIMESTAMP, FALSE)
		ON CONFLICT (projection_name) DO UPDATE SET
			is_rebuilding     = FALSE,
			last_processed_at = CURRENT_TIMESTAMP
	`, name)
	return err
}

func (m *Manager) logProjectionError(name string, errMsg string) {
	_, err := m.client.DB().Exec(`
		INSERT INTO projection_errors (projection_name, error_message, occurred_at)
		VALUES ($1, $2, CURRENT_TIMESTAMP)
		ON CONFLICT DO NOTHING
	`, name, errMsg)
	if err != nil {
		log.Printf("[WARN] Erro ao registrar falha de projeção %s: %v", name, err)
	}
}

// ============================================================================
// Event fetch
// ============================================================================

func (m *Manager) getNewEvents(fromID int64) ([]db.Event, error) {
	if fromID < 0 {
		fromID = 0
	}
	limit := db.ValidateLimit(m.batchSize)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := m.client.DB().QueryContext(ctx, `
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

// ============================================================================
// Status / introspection
// ============================================================================

func (m *Manager) IsProjectionRegistered(name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.projections[name]
	return ok
}

func (m *Manager) GetProjectionStatus(name string) (map[string]interface{}, error) {
	m.mu.Lock()
	projection, ok := m.projections[name]
	m.mu.Unlock()

	if !ok {
		return nil, fmt.Errorf("projeção não encontrada: %s", name)
	}

	lastID, err := projection.GetLastProcessedEventID()
	if err != nil {
		return nil, fmt.Errorf("erro ao obter checkpoint: %w", err)
	}

	var maxID int64
	_ = m.client.DB().QueryRow(`SELECT COALESCE(MAX(id), 0) FROM spuri_ledger`).Scan(&maxID)

	_, isTransactional := projection.(TransactionalProjection)

	return map[string]interface{}{
		"name":              name,
		"last_processed_id": lastID,
		"ledger_max_id":     maxID,
		"lag":               maxID - lastID,
		"transactional":     isTransactional,
	}, nil
}