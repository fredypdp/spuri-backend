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
	
	// 🔒 NOVO: Mutex para evitar race conditions
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
	log.Printf("📊 Projeção registrada: %s", name)
}

func (m *Manager) StartProcessing() {
	log.Println("▶️ Iniciando processamento de projeções...")

	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			log.Println("⏹️ Parando processamento de projeções")
			return
		case <-ticker.C:
			if err := m.processNewEvents(); err != nil {
				log.Printf("❌ Erro ao processar eventos: %v", err)
			}
		}
	}
}

func (m *Manager) Stop() {
	m.cancel()
}

// 🔒 CORRIGIDO: Processar eventos sequencialmente com mutex
func (m *Manager) processNewEvents() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	for name, projection := range m.projections {
		if err := m.processProjection(name, projection); err != nil {
			log.Printf("❌ Erro ao processar projeção %s: %v", name, err)
			// Continue processando outras projeções mesmo com erro
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
		if err := projection.Handle(event); err != nil {
			log.Printf("❌ [%s] Erro ao processar evento %d: %v", name, event.ID, err)
			m.logProjectionError(name, err.Error())

			// Atualizar checkpoint mesmo com erro para não reprocessar
			if err := projection.UpdateCheckpoint(event.ID); err != nil {
				log.Printf("❌ [%s] Erro ao atualizar checkpoint: %v", name, err)
			}
			continue
		}

		if err := projection.UpdateCheckpoint(event.ID); err != nil {
			log.Printf("❌ [%s] Erro ao atualizar checkpoint: %v", name, err)
			return fmt.Errorf("falha crítica ao salvar checkpoint: %w", err)
		}

		processedCount++
	}

	if processedCount > 0 {
		log.Printf("✅ [%s] Processados %d eventos (último: %d)",
			name, processedCount, events[len(events)-1].ID)
	}

	return nil
}

func (m *Manager) getNewEvents(fromID int64) ([]db.Event, error) {
	rows, err := m.client.DB().Query(`
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE id > $1
		ORDER BY id ASC
		LIMIT $2
	`, fromID, m.batchSize)
	
	if err != nil {
		return nil, fmt.Errorf("erro na query: %w", err)
	}
	defer rows.Close()

	var events []db.Event
	for rows.Next() {
		var e db.Event
		err := rows.Scan(
			&e.ID, &e.EventID, &e.AggregateID, &e.AggregateType,
			&e.EventType, &e.EventVersion, &e.Payload, &e.Metadata,
			&e.OccurredAt, &e.RecordedAt, &e.LedgerHash, &e.PreviousHash,
		)
		if err != nil {
			return nil, fmt.Errorf("erro ao escanear evento: %w", err)
		}
		events = append(events, e)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erro ao iterar eventos: %w", err)
	}

	return events, nil
}

// 🔒 CORRIGIDO: Rebuild com mutex para evitar concorrência
func (m *Manager) RebuildProjection(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	projection, exists := m.projections[name]
	if !exists {
		return fmt.Errorf("projeção não encontrada: %s", name)
	}

	log.Printf("🔨 Reconstruindo projeção: %s", name)

	if err := m.markRebuildStart(name); err != nil {
		return err
	}

	if err := projection.Rebuild(); err != nil {
		log.Printf("❌ Erro ao reconstruir projeção %s: %v", name, err)
		return err
	}

	if err := m.markRebuildComplete(name); err != nil {
		return err
	}

	log.Printf("✅ Projeção %s reconstruída com sucesso", name)
	return nil
}

func (m *Manager) RebuildAllProjections() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	log.Println("🔨 Reconstruindo TODAS as projeções...")

	for name := range m.projections {
		// Liberar mutex temporariamente para cada rebuild
		m.mu.Unlock()
		err := m.RebuildProjection(name)
		m.mu.Lock()
		
		if err != nil {
			log.Printf("❌ Erro ao reconstruir %s: %v", name, err)
			continue
		}
	}

	log.Println("✅ Todas as projeções reconstruídas")
	return nil
}

func (m *Manager) markRebuildStart(name string) error {
	_, err := m.client.DB().Exec(`
		UPDATE projection_checkpoints
		SET is_rebuilding = $1, rebuild_started_at = CURRENT_TIMESTAMP, last_processed_event_id = $2
		WHERE projection_name = $3
	`, true, 0, name)
	return err
}

func (m *Manager) markRebuildComplete(name string) error {
	var lastEventID int64
	err := m.client.DB().QueryRow(`SELECT COALESCE(MAX(id), 0) FROM spuri_ledger`).Scan(&lastEventID)
	if err != nil {
		return err
	}

	_, err = m.client.DB().Exec(`
		UPDATE projection_checkpoints
		SET is_rebuilding = $1, rebuild_started_at = $2,
			last_processed_event_id = $3, last_processed_at = CURRENT_TIMESTAMP
		WHERE projection_name = $4
	`, false, nil, lastEventID, name)
	return err
}

func (m *Manager) logProjectionError(name, errorMsg string) {
	m.client.DB().Exec(`
		UPDATE projection_checkpoints
		SET error_count = error_count + 1, last_error = $1, last_error_at = CURRENT_TIMESTAMP
		WHERE projection_name = $2
	`, errorMsg, name)
}

func (m *Manager) GetProjectionStatus(name string) (map[string]interface{}, error) {
	row := m.client.DB().QueryRow(`
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
			log.Printf("Erro ao obter status de %s: %v", name, err)
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