// ============================================================================
// ARQUIVO: internal/projections/manager.go
// 🔥 CORRIGIDO: getNewEvents com LIMIT fixo em vez de parametrizado
// ============================================================================

package projections

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"spuri/internal/genesisdb"
	"time"
)

// Manager gerencia todas as projeções
type Manager struct {
	client      *genesisdb.Client
	eventStore  *genesisdb.EventStore
	projections map[string]Projection
	ctx         context.Context
	cancel      context.CancelFunc
	
	pollInterval time.Duration
	batchSize    int
}

func NewManager(client *genesisdb.Client) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &Manager{
		client:       client,
		eventStore:   genesisdb.NewEventStore(client),
		projections:  make(map[string]Projection),
		ctx:          ctx,
		cancel:       cancel,
		pollInterval: 1 * time.Second,
		batchSize:    100,
	}
}

func (m *Manager) RegisterProjection(name string, projection Projection) {
	m.projections[name] = projection
	log.Printf("📊 Projeção registrada: %s", name)
}

func (m *Manager) StartProcessing() {
	log.Println("▶️  Iniciando processamento de projeções...")
	
	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-m.ctx.Done():
			log.Println("⏹️  Parando processamento de projeções")
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

func (m *Manager) processNewEvents() error {
	for name, projection := range m.projections {
		if err := m.processProjection(name, projection); err != nil {
			log.Printf("❌ Erro ao processar projeção %s: %v", name, err)
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

// 🔥 CORRIGIDO: LIMIT como valor fixo na string em vez de parâmetro
func (m *Manager) getNewEvents(fromID int64) ([]genesisdb.Event, error) {
	// 🔥 SOLUÇÃO: Usar fmt.Sprintf para inserir o LIMIT na query
	query := fmt.Sprintf(`
		SELECT 
			id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM genesis_ledger
		WHERE id > $1
		ORDER BY id ASC
		LIMIT %d
	`, m.batchSize)

	// 🔥 Agora só passa 1 parâmetro: fromID
	rows, err := m.client.DB().QueryContext(m.ctx, query, fromID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []genesisdb.Event
	for rows.Next() {
		var e genesisdb.Event
		err := rows.Scan(
			&e.ID, &e.EventID, &e.AggregateID, &e.AggregateType,
			&e.EventType, &e.EventVersion, &e.Payload, &e.Metadata,
			&e.OccurredAt, &e.RecordedAt, &e.LedgerHash, &e.PreviousHash,
		)
		if err != nil {
			return nil, err
		}
		events = append(events, e)
	}

	return events, rows.Err()
}

func (m *Manager) RebuildProjection(name string) error {
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
	log.Println("🔨 Reconstruindo TODAS as projeções...")

	for name := range m.projections {
		if err := m.RebuildProjection(name); err != nil {
			log.Printf("❌ Erro ao reconstruir %s: %v", name, err)
			continue
		}
	}

	log.Println("✅ Todas as projeções reconstruídas")
	return nil
}

func (m *Manager) markRebuildStart(name string) error {
	query := `
		UPDATE projection_checkpoints
		SET 
			is_rebuilding = TRUE,
			rebuild_started_at = CURRENT_TIMESTAMP,
			last_processed_event_id = 0
		WHERE projection_name = $1
	`
	_, err := m.client.DB().ExecContext(m.ctx, query, name)
	return err
}

func (m *Manager) markRebuildComplete(name string) error {
	var lastEventID int64
	query := `SELECT COALESCE(MAX(id), 0) FROM genesis_ledger`
	err := m.client.DB().QueryRowContext(m.ctx, query).Scan(&lastEventID)
	if err != nil {
		return err
	}

	query = `
		UPDATE projection_checkpoints
		SET 
			is_rebuilding = FALSE,
			rebuild_started_at = NULL,
			last_processed_event_id = $1,
			last_processed_at = CURRENT_TIMESTAMP
		WHERE projection_name = $2
	`
	_, err = m.client.DB().ExecContext(m.ctx, query, lastEventID, name)
	return err
}

func (m *Manager) logProjectionError(name, errorMsg string) {
	query := `
		UPDATE projection_checkpoints
		SET 
			error_count = error_count + 1,
			last_error = $1,
			last_error_at = CURRENT_TIMESTAMP
		WHERE projection_name = $2
	`
	m.client.DB().ExecContext(m.ctx, query, errorMsg, name)
}

func (m *Manager) GetProjectionStatus(name string) (map[string]interface{}, error) {
	query := `
		SELECT 
			projection_name, last_processed_event_id, last_processed_at,
			events_processed, is_rebuilding, rebuild_started_at,
			error_count, last_error, last_error_at
		FROM projection_checkpoints
		WHERE projection_name = $1
	`

	row := m.client.DB().QueryRowContext(m.ctx, query, name)
	
	var (
		projName       string
		lastEventID    int64
		lastProcessed  time.Time
		eventsProc     int64
		rebuilding     bool
		rebuildStart   sql.NullTime
		errCount       int
		lastErr        sql.NullString
		lastErrAt      sql.NullTime
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
	names := make([]string, 0, len(m.projections))
	for name := range m.projections {
		names = append(names, name)
	}
	return names
}

func (m *Manager) IsProjectionRegistered(name string) bool {
	_, exists := m.projections[name]
	return exists
}