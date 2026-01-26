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

func (m *Manager) getNewEvents(fromID int64) ([]db.Event, error) {
	if fromID < 0 {
		fromID = 0
	}

	query := fmt.Sprintf(`
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE id > %d
		ORDER BY id ASC
		LIMIT %d
	`, fromID, db.ValidateLimit(m.batchSize))

	rows, err := m.client.DB().Query(query)
	if err != nil {
		return nil, fmt.Errorf("erro na query: %w", err)
	}
	defer rows.Close()

	var events []db.Event
	for rows.Next() {
		var event db.Event
		if err := rows.Scan(&event.ID, &event.EventID, &event.AggregateID, &event.AggregateType,
			&event.EventType, &event.EventVersion, &event.Payload, &event.Metadata,
			&event.OccurredAt, &event.RecordedAt, &event.LedgerHash, &event.PreviousHash); err != nil {
			return nil, fmt.Errorf("erro ao scan: %w", err)
		}
		events = append(events, event)
	}

	return events, rows.Err()
}

func (m *Manager) RebuildProjection(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
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

func (m *Manager) RebuildAllProjections() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	log.Println("[DEBUG] Reconstruindo TODAS as projeções")

	for name := range m.projections {
		m.mu.Unlock()
		err := m.RebuildProjection(name)
		m.mu.Lock()
		
		if err != nil {
			log.Printf("[ERROR] Erro ao reconstruir %s: %v", name, err)
		}
	}

	log.Println("[DEBUG] Todas as projeções reconstruídas")
	return nil
}

func (m *Manager) markRebuildStart(name string) error {
	query := fmt.Sprintf(`
		UPDATE projection_checkpoints
		SET is_rebuilding = true, rebuild_started_at = CURRENT_TIMESTAMP, last_processed_event_id = 0
		WHERE projection_name = '%s'
	`, db.SafeString(name))
	
	_, err := m.client.DB().Exec(query)
	return err
}

func (m *Manager) markRebuildComplete(name string) error {
	var lastEventID int64
	if err := m.client.DB().QueryRow(`SELECT COALESCE(MAX(id), 0) FROM spuri_ledger`).Scan(&lastEventID); err != nil {
		return err
	}

	query := fmt.Sprintf(`
		UPDATE projection_checkpoints
		SET is_rebuilding = false, rebuild_started_at = NULL,
			last_processed_event_id = %d, last_processed_at = CURRENT_TIMESTAMP
		WHERE projection_name = '%s'
	`, lastEventID, db.SafeString(name))
	
	_, err := m.client.DB().Exec(query)
	return err
}

func (m *Manager) logProjectionError(name, errorMsg string) {
	query := fmt.Sprintf(`
		UPDATE projection_checkpoints
		SET error_count = error_count + 1, last_error = '%s', last_error_at = CURRENT_TIMESTAMP
		WHERE projection_name = '%s'
	`, db.SafeString(errorMsg), db.SafeString(name))
	
	m.client.DB().Exec(query)
}

func (m *Manager) GetProjectionStatus(name string) (map[string]interface{}, error) {
	query := fmt.Sprintf(`
		SELECT projection_name, last_processed_event_id, last_processed_at,
			events_processed, is_rebuilding, rebuild_started_at,
			error_count, last_error, last_error_at
		FROM projection_checkpoints
		WHERE projection_name = '%s'
	`, db.SafeString(name))

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

	err := m.client.DB().QueryRow(query).Scan(
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