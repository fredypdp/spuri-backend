// ============================================================================
// ARQUIVO: internal/projections/manager.go
// Gerenciador de todas as projeções do sistema
// VERSÃO: 2.1.1 - CORRIGIDO: Erro de prepared statement
// ============================================================================

package projections

import (
	"context"
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
	
	// Configuração
	pollInterval time.Duration
	batchSize    int
}

// NewManager cria um novo gerenciador de projeções
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

// RegisterProjection registra uma nova projeção
func (m *Manager) RegisterProjection(name string, projection Projection) {
	m.projections[name] = projection
	log.Printf("📊 Projeção registrada: %s", name)
}

// StartProcessing inicia o processamento contínuo de eventos
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

// Stop para o processamento
func (m *Manager) Stop() {
	m.cancel()
}

// processNewEvents processa novos eventos para todas as projeções
func (m *Manager) processNewEvents() error {
	for name, projection := range m.projections {
		if err := m.processProjection(name, projection); err != nil {
			log.Printf("❌ Erro ao processar projeção %s: %v", name, err)
			// Continuar com outras projeções
			continue
		}
	}
	return nil
}

// processProjection processa eventos pendentes para uma projeção
func (m *Manager) processProjection(name string, projection Projection) error {
	// Obter último evento processado
	lastProcessedID, err := projection.GetLastProcessedEventID()
	if err != nil {
		return fmt.Errorf("erro ao obter checkpoint: %w", err)
	}

	// 🔥 CORRIGIDO: Buscar novos eventos
	events, err := m.getNewEvents(lastProcessedID, m.batchSize)
	if err != nil {
		return fmt.Errorf("erro ao buscar eventos: %w", err)
	}

	if len(events) == 0 {
		return nil // Nenhum evento novo
	}

	// Processar eventos
	processedCount := 0
	for _, event := range events {
		// Processar evento
		if err := projection.Handle(event); err != nil {
			log.Printf("❌ [%s] Erro ao processar evento %d: %v", name, event.ID, err)
			
			// Registrar erro
			m.logProjectionError(name, err.Error())
			
			// Atualizar checkpoint mesmo com erro para não reprocessar infinitamente
			if err := projection.UpdateCheckpoint(event.ID); err != nil {
				log.Printf("❌ [%s] Erro ao atualizar checkpoint: %v", name, err)
			}
			continue
		}

		// Atualizar checkpoint após processar com sucesso
		if err := projection.UpdateCheckpoint(event.ID); err != nil {
			log.Printf("❌ [%s] Erro ao atualizar checkpoint: %v", name, err)
			return fmt.Errorf("falha crítica ao salvar checkpoint: %w", err)
		}
		
		processedCount++
	}

	// Log apenas se processou eventos
	if processedCount > 0 {
		log.Printf("✅ [%s] Processados %d eventos (último: %d)", 
			name, processedCount, events[len(events)-1].ID)
	}

	return nil
}

// 🔥 CORRIGIDO: getNewEvents agora recebe batchSize explicitamente
func (m *Manager) getNewEvents(fromID int64, limit int) ([]genesisdb.Event, error) {
	// Query sem prepared statement cacheado
	query := `
		SELECT 
			id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM genesis_ledger
		WHERE id > $1
		ORDER BY id ASC
		LIMIT $2
	`

	var events []genesisdb.Event
	
	// 🔥 SOLUÇÃO: Usar Query + Scan em vez de SelectContext
	// Isso evita problemas de prepared statement cache
	rows, err := m.client.DB().QueryContext(m.ctx, query, fromID, limit)
	if err != nil {
		return nil, fmt.Errorf("erro na query: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var event genesisdb.Event
		err := rows.Scan(
			&event.ID,
			&event.EventID,
			&event.AggregateID,
			&event.AggregateType,
			&event.EventType,
			&event.EventVersion,
			&event.Payload,
			&event.Metadata,
			&event.OccurredAt,
			&event.RecordedAt,
			&event.LedgerHash,
			&event.PreviousHash,
		)
		if err != nil {
			return nil, fmt.Errorf("erro ao scanear evento: %w", err)
		}
		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erro ao iterar eventos: %w", err)
	}

	return events, nil
}

// RebuildProjection reconstrói uma projeção do zero
func (m *Manager) RebuildProjection(name string) error {
	projection, exists := m.projections[name]
	if !exists {
		return fmt.Errorf("projeção não encontrada: %s", name)
	}

	log.Printf("🔨 Reconstruindo projeção: %s", name)

	// Marcar como em reconstrução
	if err := m.markRebuildStart(name); err != nil {
		return err
	}

	// Reconstruir
	if err := projection.Rebuild(); err != nil {
		log.Printf("❌ Erro ao reconstruir projeção %s: %v", name, err)
		return err
	}

	// Marcar como concluído
	if err := m.markRebuildComplete(name); err != nil {
		return err
	}

	log.Printf("✅ Projeção %s reconstruída com sucesso", name)
	return nil
}

// RebuildAllProjections reconstrói todas as projeções
func (m *Manager) RebuildAllProjections() error {
	log.Println("🔨 Reconstruindo TODAS as projeções...")

	for name := range m.projections {
		if err := m.RebuildProjection(name); err != nil {
			log.Printf("❌ Erro ao reconstruir %s: %v", name, err)
			// Continuar com outras projeções
			continue
		}
	}

	log.Println("✅ Todas as projeções reconstruídas")
	return nil
}

// markRebuildStart marca início de reconstrução
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

// markRebuildComplete marca conclusão de reconstrução
func (m *Manager) markRebuildComplete(name string) error {
	// Obter último evento
	var lastEventID int64
	query := `SELECT COALESCE(MAX(id), 0) FROM genesis_ledger`
	err := m.client.DB().GetContext(m.ctx, &lastEventID, query)
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

// logProjectionError registra erro em projeção
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

// GetProjectionStatus retorna status de uma projeção
func (m *Manager) GetProjectionStatus(name string) (map[string]interface{}, error) {
	query := `
		SELECT 
			projection_name,
			last_processed_event_id,
			last_processed_at,
			events_processed,
			is_rebuilding,
			rebuild_started_at,
			error_count,
			last_error,
			last_error_at
		FROM projection_checkpoints
		WHERE projection_name = $1
	`

	type Status struct {
		ProjectionName       string     `db:"projection_name"`
		LastProcessedEventID int64      `db:"last_processed_event_id"`
		LastProcessedAt      time.Time  `db:"last_processed_at"`
		EventsProcessed      int64      `db:"events_processed"`
		IsRebuilding         bool       `db:"is_rebuilding"`
		RebuildStartedAt     *time.Time `db:"rebuild_started_at"`
		ErrorCount           int        `db:"error_count"`
		LastError            *string    `db:"last_error"`
		LastErrorAt          *time.Time `db:"last_error_at"`
	}

	var status Status
	err := m.client.DB().GetContext(m.ctx, &status, query, name)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"name":                   status.ProjectionName,
		"last_processed_event":   status.LastProcessedEventID,
		"last_processed_at":      status.LastProcessedAt,
		"events_processed":       status.EventsProcessed,
		"is_rebuilding":          status.IsRebuilding,
		"rebuild_started_at":     status.RebuildStartedAt,
		"error_count":            status.ErrorCount,
		"last_error":             status.LastError,
		"last_error_at":          status.LastErrorAt,
	}, nil
}

// GetAllProjectionStatuses retorna status de todas as projeções
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

// GetRegisteredProjections retorna lista de projeções registradas
func (m *Manager) GetRegisteredProjections() []string {
	names := make([]string, 0, len(m.projections))
	for name := range m.projections {
		names = append(names, name)
	}
	return names
}

// IsProjectionRegistered verifica se uma projeção está registrada
func (m *Manager) IsProjectionRegistered(name string) bool {
	_, exists := m.projections[name]
	return exists
}