// ============================================================================
// ARQUIVO: internal/projections/manager.go
// CORREÇÃO BUG #2: getNewEvents usa sql.NullString para previous_hash.
//   Antes: scan direto para *string causava erro quando campo era NULL no banco
//   (primeiro evento de todo aggregate não tem previous_hash). O lote inteiro
//   de eventos era abandonado silenciosamente.
// CORREÇÃO BUG #3: RebuildAllProjections refatorado com rebuildProjectionInternal.
//   Antes: Lock→Unlock→RebuildProjection(Lock interno)→Lock causava race condition
//   com a goroutine StartProcessing durante o intervalo sem lock.
// ============================================================================

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
			log.Printf("[ERROR] %s: evento %d falhou permanentemente", name, event.ID)
			m.logProjectionError(name, err.Error())
		}
		if err := projection.UpdateCheckpoint(event.ID); err != nil {
			log.Printf("[WARN] Erro ao atualizar checkpoint para evento %d: %v", event.ID, err)
		}
		processedCount++
	}
	if processedCount > 0 {
		log.Printf("[DEBUG] %s: processados %d eventos (último: %d)",
			name, processedCount, events[len(events)-1].ID)
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
// CORRIGIDO BUG #2: previous_hash lido como sql.NullString para suportar NULL.
// O primeiro evento de cada aggregate não tem previous_hash (é NULL no banco).
// Antes, o scan direto para *string falhava com erro e abandonava o lote inteiro.
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
		// CORRIGIDO: sql.NullString para previous_hash (pode ser NULL no banco)
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

// RebuildProjection reconstrói uma projeção específica.
// Adquire lock e delega para rebuildProjectionInternal.
func (m *Manager) RebuildProjection(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.rebuildProjectionInternal(name)
}

// rebuildProjectionInternal executa o rebuild sem adquirir o lock.
// DEVE ser chamado somente quando o lock já foi adquirido pelo chamador.
// Separado de RebuildProjection para que RebuildAllProjections possa
// reutilizá-lo sem risco de deadlock.
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

// RebuildAllProjections reconstrói todas as projeções registradas.
// CORRIGIDO BUG #3: adquire o lock uma única vez e chama rebuildProjectionInternal
// (sem lock interno) para cada projeção.
// O padrão anterior Lock→Unlock→RebuildProjection(Lock)→Lock era frágil:
// a goroutine StartProcessing podia adquirir o lock no intervalo sem lock,
// causando race condition entre rebuild e processamento normal de eventos.
func (m *Manager) RebuildAllProjections() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	log.Println("[DEBUG] Reconstruindo TODAS as projeções")
	var firstErr error
	for name := range m.projections {
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

func (m *Manager) markRebuildStart(name string) error {
	_, err := m.client.DB().Exec(`
		UPDATE projection_checkpoints
		SET is_rebuilding        = true,
		    rebuild_started_at   = CURRENT_TIMESTAMP,
		    last_processed_event_id = 0
		WHERE projection_name = $1`,
		name,
	)
	return err
}

func (m *Manager) markRebuildComplete(name string) error {
	var lastEventID int64
	if err := m.client.DB().QueryRow(
		`SELECT COALESCE(MAX(id), 0) FROM spuri_ledger`,
	).Scan(&lastEventID); err != nil {
		return err
	}
	_, err := m.client.DB().Exec(`
		UPDATE projection_checkpoints
		SET is_rebuilding             = false,
		    rebuild_started_at        = NULL,
		    last_processed_event_id   = $1,
		    last_processed_at         = CURRENT_TIMESTAMP
		WHERE projection_name = $2`,
		lastEventID, name,
	)
	return err
}

func (m *Manager) logProjectionError(name, errorMsg string) {
	_, err := m.client.DB().Exec(`
		UPDATE projection_checkpoints
		SET error_count  = error_count + 1,
		    last_error   = $1,
		    last_error_at = CURRENT_TIMESTAMP
		WHERE projection_name = $2`,
		errorMsg, name,
	)
	if err != nil {
		log.Printf("[WARN] logProjectionError: falha ao registrar erro para %s: %v", name, err)
	}
}

func (m *Manager) GetProjectionStatus(name string) (map[string]interface{}, error) {
	var (
		projName      string
		lastEventID   int64
		lastProcessed time.Time
		eventsProc    int64
		rebuilding    bool
		rebuildStart  sql.NullTime
		errCount      int
		lastErr       sql.NullString
		lastErrAt     sql.NullTime
	)
	err := m.client.DB().QueryRow(`
		SELECT projection_name, last_processed_event_id, last_processed_at,
			events_processed, is_rebuilding, rebuild_started_at,
			error_count, last_error, last_error_at
		FROM projection_checkpoints
		WHERE projection_name = $1`,
		name,
	).Scan(
		&projName, &lastEventID, &lastProcessed, &eventsProc,
		&rebuilding, &rebuildStart, &errCount, &lastErr, &lastErrAt,
	)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"name":                 projName,
		"last_processed_event": lastEventID,
		"last_processed_at":    lastProcessed,
		"events_processed":     eventsProc,
		"is_rebuilding":        rebuilding,
		"rebuild_started_at":   rebuildStart,
		"error_count":          errCount,
		"last_error":           lastErr,
		"last_error_at":        lastErrAt,
	}, nil
}

func (m *Manager) GetAllProjectionStatuses() ([]map[string]interface{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var statuses []map[string]interface{}
	for name := range m.projections {
		if status, err := m.GetProjectionStatus(name); err == nil {
			statuses = append(statuses, status)
		}
	}
	return statuses, nil
}

func (m *Manager) GetRegisteredProjections() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	names := make([]string, 0, len(m.projections))
	for name := range m.projections {
		names = append(names, name)
	}
	return names
}

func (m *Manager) IsProjectionRegistered(name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, exists := m.projections[name]
	return exists
}

func (m *Manager) GetProjection(name string) (Projection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.projections[name]
	if !ok {
		return nil, fmt.Errorf("projeção '%s' não registrada", name)
	}
	return p, nil
}