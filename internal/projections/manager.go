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
	log.Printf("[INFO] Projecao registrada: %s", name)
}

func (m *Manager) StartProcessing() {
	log.Println("[INFO] Iniciando processamento de projecoes")

	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			log.Println("[INFO] Parando processamento de projecoes")
			return
		case <-ticker.C:
			if err := m.processNewEvents(); err != nil {
				log.Printf("[ERROR] Erro ao processar eventos: %v", err)
			}
		}
	}
}

func (m *Manager) Stop() {
	m.cancel()
}

func (m *Manager) processNewEvents() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	for name, projection := range m.projections {
		if err := m.processProjection(name, projection); err != nil {
			log.Printf("[ERROR] Erro ao processar projecao %s: %v", name, err)
			continue
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

	processedCount := 0
	
	for _, event := range events {
		if err := m.processEventWithRetry(name, projection, event); err != nil {
			log.Printf("[ERROR] Projecao %s: evento %d falhou permanentemente", name, event.ID)
			m.logProjectionError(name, err.Error())
			projection.UpdateCheckpoint(event.ID)
			continue
		}
		
		if err := projection.UpdateCheckpoint(event.ID); err != nil {
			log.Printf("[WARN] Erro ao atualizar checkpoint para evento %d: %v", event.ID, err)
		}
		processedCount++
	}

	if processedCount > 0 {
		log.Printf("[INFO] Projecao %s: processados %d eventos (ultimo: %d)",
			name, processedCount, events[len(events)-1].ID)
	}

	return nil
}

func (m *Manager) processEventWithRetry(name string, projection Projection, event db.Event) error {
	maxRetries := 3
	baseDelay := 1 * time.Second
	
	var lastErr error
	
	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := projection.Handle(event)
		
		if err == nil {
			if attempt > 1 {
				log.Printf("[INFO] Projecao %s: evento %d recuperado na tentativa %d", 
					name, event.ID, attempt)
			}
			return nil
		}
		
		lastErr = err
		
		if attempt < maxRetries {
			delay := time.Duration(attempt*attempt) * baseDelay
			log.Printf("[WARN] Projecao %s: evento %d falhou (tentativa %d/%d), retry em %v", 
				name, event.ID, attempt, maxRetries, delay)
			time.Sleep(delay)
		}
	}
	
	return fmt.Errorf("evento %d falhou apos %d tentativas: %w", event.ID, maxRetries, lastErr)
}

// ✅ SOLUÇÃO DEFINITIVA: Usar Select do sqlx
func (m *Manager) getNewEvents(fromID int64) ([]db.Event, error) {
	query := `
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE id > $1
		ORDER BY id ASC
		LIMIT $2
	`
	
	var events []db.Event
	
	// ✅ USAR Select que NÃO usa prepared statements
	err := m.client.DB().Select(&events, query, fromID, m.batchSize)
	if err != nil {
		return nil, fmt.Errorf("erro na query: %w", err)
	}

	return events, nil
}

func (m *Manager) RebuildProjection(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	projection, exists := m.projections[name]
	if !exists {
		return fmt.Errorf("projecao nao encontrada: %s", name)
	}

	log.Printf("[INFO] Reconstruindo projecao: %s", name)

	if err := m.markRebuildStart(name); err != nil {
		return err
	}

	if err := projection.Rebuild(); err != nil {
		log.Printf("[ERROR] Erro ao reconstruir projecao %s: %v", name, err)
		return err
	}

	if err := m.markRebuildComplete(name); err != nil {
		return err
	}

	log.Printf("[INFO] Projecao %s reconstruida com sucesso", name)
	return nil
}

func (m *Manager) RebuildAllProjections() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	log.Println("[INFO] Reconstruindo TODAS as projecoes")

	for name := range m.projections {
		m.mu.Unlock()
		err := m.RebuildProjection(name)
		m.mu.Lock()
		
		if err != nil {
			log.Printf("[ERROR] Erro ao reconstruir %s: %v", name, err)
			continue
		}
	}

	log.Println("[INFO] Todas as projecoes reconstruidas")
	return nil
}

func (m *Manager) markRebuildStart(name string) error {
	ctx := context.Background()
	_, err := m.client.DB().ExecContext(ctx, `
		UPDATE projection_checkpoints
		SET is_rebuilding = $1, rebuild_started_at = CURRENT_TIMESTAMP, last_processed_event_id = $2
		WHERE projection_name = $3
	`, true, 0, name)
	return err
}

func (m *Manager) markRebuildComplete(name string) error {
	ctx := context.Background()
	var lastEventID int64
	err := m.client.DB().QueryRowContext(ctx, `SELECT COALESCE(MAX(id), 0) FROM spuri_ledger`).Scan(&lastEventID)
	if err != nil {
		return err
	}

	_, err = m.client.DB().ExecContext(ctx, `
		UPDATE projection_checkpoints
		SET is_rebuilding = $1, rebuild_started_at = $2,
			last_processed_event_id = $3, last_processed_at = CURRENT_TIMESTAMP
		WHERE projection_name = $4
	`, false, nil, lastEventID, name)
	return err
}

func (m *Manager) logProjectionError(name, errorMsg string) {
	ctx := context.Background()
	m.client.DB().ExecContext(ctx, `
		UPDATE projection_checkpoints
		SET error_count = error_count + 1, last_error = $1, last_error_at = CURRENT_TIMESTAMP
		WHERE projection_name = $2
	`, errorMsg, name)
}

func (m *Manager) GetProjectionStatus(name string) (map[string]interface{}, error) {
	ctx := context.Background()
	row := m.client.DB().QueryRowContext(ctx, `
		SELECT projection_name, last_processed_event_id, last_processed_at,
			events_processed, is_rebuilding, rebuild_started_at,
			error_count, last_error, last_error_at
		FROM projection_checkpoints
		WHERE projection_name = $1
	`, name)

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

	err := row.Scan(
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
		status, err := m.GetProjectionStatus(name)
		if err != nil {
			log.Printf("[ERROR] Erro ao obter status de %s: %v", name, err)
			continue
		}
		statuses = append(statuses, status)
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