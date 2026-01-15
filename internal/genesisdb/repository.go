// ============================================================================
// ARQUIVO: internal/genesisdb/repository.go
// CORREÇÃO: Tratar agregados novos (primeira versão)
// ============================================================================

package genesisdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"spuri/internal/domain/aggregates"

	"github.com/google/uuid"
)

// AggregateRepository repositório de agregados usando Event Sourcing
type AggregateRepository struct {
	eventStore *EventStore
	factory    aggregates.AggregateFactory
	ctx        context.Context
}

// NewAggregateRepository cria um novo repositório
func NewAggregateRepository(client *Client) *AggregateRepository {
	return &AggregateRepository{
		eventStore: NewEventStore(client),
		factory:    &aggregates.DefaultAggregateFactory{},
		ctx:        context.Background(),
	}
}

// Load carrega um agregado reconstruindo-o a partir dos eventos
func (r *AggregateRepository) Load(id uuid.UUID, aggregateType string) (aggregates.Aggregate, error) {
	// 1. Carregar eventos do ledger
	genesisEvents, err := r.eventStore.LoadEventStream(r.ctx, id)
	if err != nil {
		return nil, fmt.Errorf("erro ao carregar eventos: %w", err)
	}

	if len(genesisEvents) == 0 {
		return nil, fmt.Errorf("agregado não encontrado: %s", id)
	}

	// 2. Converter eventos do GenesisDB para eventos de domínio
	domainEvents, err := r.convertToDomainEvents(genesisEvents)
	if err != nil {
		return nil, fmt.Errorf("erro ao converter eventos: %w", err)
	}

	// 3. Criar agregado vazio
	aggregate, err := r.factory.Create(aggregateType)
	if err != nil {
		return nil, err
	}

	// 4. Reconstruir estado aplicando eventos
	for _, event := range domainEvents {
		if err := aggregate.Apply(event); err != nil {
			return nil, fmt.Errorf("erro ao aplicar evento: %w", err)
		}
	}

	return aggregate, nil
}

// Save salva um agregado persistindo seus eventos não commitados
func (r *AggregateRepository) Save(aggregate aggregates.Aggregate) error {
	uncommittedEvents := aggregate.GetUncommittedEvents()
	if len(uncommittedEvents) == 0 {
		return nil // Nada para salvar
	}

	// 🔥 CORREÇÃO: Obter versão atual do agregado no ledger
	// Se o agregado não existir (sql.ErrNoRows), começar do zero
	currentVersion, err := r.eventStore.GetAggregateVersion(r.ctx, aggregate.GetID())
	if err != nil && err != sql.ErrNoRows {
		// Erro real, não apenas "não encontrado"
		return fmt.Errorf("erro ao obter versão: %w", err)
	}
	// Se err == sql.ErrNoRows, currentVersion já é 0 (valor padrão)

	// Salvar cada evento
	for i, domainEvent := range uncommittedEvents {
		// Converter para evento do GenesisDB
		genesisEvent, err := r.convertToGenesisEvent(
			domainEvent,
			aggregate.GetType(),
			currentVersion+i+1,
		)
		if err != nil {
			return fmt.Errorf("erro ao converter evento: %w", err)
		}

		// Persistir no ledger
		if err := r.eventStore.Append(r.ctx, genesisEvent); err != nil {
			return fmt.Errorf("erro ao salvar evento: %w", err)
		}
	}

	// Limpar eventos não commitados
	aggregate.ClearUncommittedEvents()

	return nil
}

// Exists verifica se um agregado existe
func (r *AggregateRepository) Exists(id uuid.UUID) (bool, error) {
	count, err := r.eventStore.CountEventsByAggregate(r.ctx, id)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// LoadFromVersion carrega agregado a partir de uma versão específica
func (r *AggregateRepository) LoadFromVersion(
	id uuid.UUID,
	aggregateType string,
	fromVersion int,
) (aggregates.Aggregate, error) {
	// Carregar eventos a partir da versão
	genesisEvents, err := r.eventStore.LoadEventStreamFromVersion(r.ctx, id, fromVersion)
	if err != nil {
		return nil, fmt.Errorf("erro ao carregar eventos: %w", err)
	}

	if len(genesisEvents) == 0 {
		return nil, fmt.Errorf("nenhum evento encontrado")
	}

	// Converter e reconstruir
	domainEvents, err := r.convertToDomainEvents(genesisEvents)
	if err != nil {
		return nil, err
	}

	aggregate, err := r.factory.Create(aggregateType)
	if err != nil {
		return nil, err
	}

	for _, event := range domainEvents {
		if err := aggregate.Apply(event); err != nil {
			return nil, err
		}
	}

	return aggregate, nil
}

// GetEventHistory retorna o histórico de eventos de um agregado
func (r *AggregateRepository) GetEventHistory(id uuid.UUID) ([]Event, error) {
	return r.eventStore.LoadEventStream(r.ctx, id)
}

// VerifyIntegrity verifica a integridade do ledger de um agregado
func (r *AggregateRepository) VerifyIntegrity(id uuid.UUID) (bool, error) {
	return r.eventStore.VerifyLedgerIntegrity(r.ctx, id)
}

// convertToGenesisEvent converte evento de domínio para evento do GenesisDB
func (r *AggregateRepository) convertToGenesisEvent(
	domainEvent aggregates.DomainEvent,
	aggregateType string,
	version int,
) (*Event, error) {
	// Obter o payload completo do evento
	payload := domainEvent.GetPayload()
	
	// Serializar payload corretamente
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar payload: %w", err)
	}

	// Criar metadata vazia por padrão
	metadata := map[string]interface{}{
		"timestamp": time.Now().Unix(),
	}
	metadataJSON, _ := json.Marshal(metadata)

	return &Event{
		EventID:       uuid.New(),
		AggregateID:   domainEvent.GetAggregateID(),
		AggregateType: aggregateType,
		EventType:     domainEvent.GetEventType(),
		EventVersion:  version,
		Payload:       payloadJSON,
		Metadata:      metadataJSON,
		OccurredAt:    time.Now(),
	}, nil
}

// convertToDomainEvents converte eventos do GenesisDB para eventos de domínio
func (r *AggregateRepository) convertToDomainEvents(genesisEvents []Event) ([]aggregates.DomainEvent, error) {
	domainEvents := make([]aggregates.DomainEvent, 0, len(genesisEvents))

	for _, ge := range genesisEvents {
		// Deserializar payload
		var payload map[string]interface{}
		if err := json.Unmarshal(ge.Payload, &payload); err != nil {
			return nil, err
		}

		// Criar evento de domínio base
		domainEvent := &aggregates.BaseEvent{
			EventType:   ge.EventType,
			AggregateID: ge.AggregateID,
			Payload:     payload,
		}

		domainEvents = append(domainEvents, domainEvent)
	}

	return domainEvents, nil
}

// Snapshot (opcional) - salva snapshot do estado atual
type Snapshot struct {
	AggregateID   uuid.UUID       `db:"aggregate_id"`
	AggregateType string          `db:"aggregate_type"`
	Version       int             `db:"version"`
	State         json.RawMessage `db:"state"`
	CreatedAt     time.Time       `db:"created_at"`
}

// SaveSnapshot salva um snapshot (otimização para agregados com muitos eventos)
func (r *AggregateRepository) SaveSnapshot(aggregate aggregates.Aggregate) error {
	// Serializar estado
	stateJSON, err := json.Marshal(aggregate)
	if err != nil {
		return err
	}

	query := `
		INSERT INTO aggregate_snapshots (aggregate_id, aggregate_type, version, state)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (aggregate_id) 
		DO UPDATE SET version = $3, state = $4, created_at = CURRENT_TIMESTAMP
	`

	_, err = r.eventStore.client.db.ExecContext(
		r.ctx, query,
		aggregate.GetID(),
		aggregate.GetType(),
		aggregate.GetVersion(),
		stateJSON,
	)

	return err
}

// LoadSnapshot carrega um snapshot
func (r *AggregateRepository) LoadSnapshot(id uuid.UUID) (*Snapshot, error) {
	query := `
		SELECT aggregate_id, aggregate_type, version, state, created_at
		FROM aggregate_snapshots
		WHERE aggregate_id = $1
	`

	var snapshot Snapshot
	err := r.eventStore.client.db.GetContext(r.ctx, &snapshot, query, id)
	if err != nil {
		return nil, err
	}

	return &snapshot, nil
}