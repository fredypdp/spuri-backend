package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"spuri/internal/domain/aggregates"
	"time"

	"github.com/google/uuid"
)

type AggregateRepository struct {
	eventStore *EventStore
	factory    aggregates.AggregateFactory
	ctx        context.Context
}

// AuditContext carrega informações do usuário que está realizando a operação.
// Passado via SaveWithAudit para enriquecer o metadata de cada evento.
type AuditContext struct {
	UserID   string // UUID do usuário (estudante, academia ou admin)
	UserType string // "estudante" | "academia" | "admin"
	IP       string // IP da requisição (opcional)
}

func NewAggregateRepository(client *Client) *AggregateRepository {
	return &AggregateRepository{
		eventStore: NewEventStore(client),
		factory:    &aggregates.DefaultAggregateFactory{},
		ctx:        context.Background(),
	}
}

func (r *AggregateRepository) Load(id uuid.UUID, aggregateType string) (aggregates.Aggregate, error) {
	dbEvents, err := r.eventStore.LoadEventStream(r.ctx, id)
	if err != nil {
		return nil, fmt.Errorf("erro ao carregar eventos: %w", err)
	}

	if len(dbEvents) == 0 {
		return nil, fmt.Errorf("agregado não encontrado: %s", id)
	}

	domainEvents, err := r.convertToDomainEvents(dbEvents)
	if err != nil {
		return nil, fmt.Errorf("erro ao converter eventos: %w", err)
	}

	aggregate, err := r.factory.Create(aggregateType)
	if err != nil {
		return nil, err
	}

	for _, event := range domainEvents {
		if err := aggregate.Apply(event); err != nil {
			return nil, fmt.Errorf("erro ao aplicar evento: %w", err)
		}
	}

	return aggregate, nil
}

func (r *AggregateRepository) Save(aggregate aggregates.Aggregate) error {
	uncommittedEvents := aggregate.GetUncommittedEvents()
	if len(uncommittedEvents) == 0 {
		return nil
	}

	// Iniciar transação — todos os eventos gravados atomicamente ou nenhum
	tx, err := r.eventStore.client.BeginTx(r.ctx)
	if err != nil {
		return fmt.Errorf("erro ao iniciar transação: %w", err)
	}
	defer tx.Rollback() // no-op se Commit já foi chamado

	currentVersion := 0
	version, err := r.eventStore.GetAggregateVersion(r.ctx, aggregate.GetID())
	if err == nil {
		currentVersion = version
	} else if err != sql.ErrNoRows {
		return fmt.Errorf("erro ao obter versão: %w", err)
	}

	for i, domainEvent := range uncommittedEvents {
		dbEvent, err := r.dbEvent(
			domainEvent,
			aggregate.GetType(),
			currentVersion+i+1,
		)
		if err != nil {
			return fmt.Errorf("erro ao converter evento: %w", err)
		}

		if err := r.eventStore.AppendTx(r.ctx, tx, dbEvent); err != nil {
			return fmt.Errorf("erro ao salvar evento: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("erro ao confirmar transação: %w", err)
	}

	aggregate.ClearUncommittedEvents()
	return nil
}

// SaveWithAudit salva os eventos do aggregate com metadata de auditoria completo.
// Usar este método em handlers onde o contexto do usuário está disponível.
func (r *AggregateRepository) SaveWithAudit(aggregate aggregates.Aggregate, audit AuditContext) error {
	uncommittedEvents := aggregate.GetUncommittedEvents()
	if len(uncommittedEvents) == 0 {
		return nil
	}

	tx, err := r.eventStore.client.BeginTx(r.ctx)
	if err != nil {
		return fmt.Errorf("erro ao iniciar transação: %w", err)
	}
	defer tx.Rollback()

	currentVersion := 0
	version, err := r.eventStore.GetAggregateVersion(r.ctx, aggregate.GetID())
	if err == nil {
		currentVersion = version
	} else if err != sql.ErrNoRows {
		return fmt.Errorf("erro ao obter versão: %w", err)
	}

	for i, domainEvent := range uncommittedEvents {
		dbEvent, err := r.dbEventWithAudit(domainEvent, aggregate.GetType(), currentVersion+i+1, audit)
		if err != nil {
			return fmt.Errorf("erro ao converter evento: %w", err)
		}

		if err := r.eventStore.AppendTx(r.ctx, tx, dbEvent); err != nil {
			return fmt.Errorf("erro ao salvar evento: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("erro ao confirmar transação: %w", err)
	}

	aggregate.ClearUncommittedEvents()
	return nil
}

func (r *AggregateRepository) Exists(id uuid.UUID) (bool, error) {
	count, err := r.eventStore.CountEventsByAggregate(r.ctx, id)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *AggregateRepository) LoadFromVersion(
	id uuid.UUID,
	aggregateType string,
	fromVersion int,
) (aggregates.Aggregate, error) {
	dbEvents, err := r.eventStore.LoadEventStreamFromVersion(r.ctx, id, fromVersion)
	if err != nil {
		return nil, fmt.Errorf("erro ao carregar eventos: %w", err)
	}

	if len(dbEvents) == 0 {
		return nil, fmt.Errorf("nenhum evento encontrado")
	}

	domainEvents, err := r.convertToDomainEvents(dbEvents)
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

func (r *AggregateRepository) GetEventHistory(id uuid.UUID) ([]Event, error) {
	return r.eventStore.LoadEventStream(r.ctx, id)
}

func (r *AggregateRepository) VerifyIntegrity(id uuid.UUID) (bool, error) {
	return r.eventStore.VerifyLedgerIntegrity(r.ctx, id)
}

func (r *AggregateRepository) dbEvent(
	domainEvent aggregates.DomainEvent,
	aggregateType string,
	version int,
) (*Event, error) {
	payload := domainEvent.GetPayload()
	
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar payload: %w", err)
	}

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

// dbEventWithAudit cria um db.Event com metadata de auditoria enriquecido.
func (r *AggregateRepository) dbEventWithAudit(
	domainEvent aggregates.DomainEvent,
	aggregateType string,
	version int,
	audit AuditContext,
) (*Event, error) {
	payload := domainEvent.GetPayload()

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar payload: %w", err)
	}

	metadata := map[string]interface{}{
		"timestamp":  time.Now().Unix(),
		"user_id":    audit.UserID,
		"user_type":  audit.UserType,
		"ip":         audit.IP,
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

func (r *AggregateRepository) convertToDomainEvents(dbEvents []Event) ([]aggregates.DomainEvent, error) {
	domainEvents := make([]aggregates.DomainEvent, 0, len(dbEvents))

	for _, ge := range dbEvents {
		var payload map[string]interface{}
		if err := json.Unmarshal(ge.Payload, &payload); err != nil {
			return nil, err
		}

		domainEvent := &aggregates.BaseEvent{
			EventType:   ge.EventType,
			AggregateID: ge.AggregateID,
			Payload:     payload,
		}

		domainEvents = append(domainEvents, domainEvent)
	}

	return domainEvents, nil
}

// ✅ ATUALIZADO: Usar ExecWithRetry
func (r *AggregateRepository) SaveSnapshot(aggregate aggregates.Aggregate) error {
	stateJSON, err := json.Marshal(aggregate)
	if err != nil {
		return err
	}

	aggID := aggregate.GetID()
	if aggID == uuid.Nil {
		return fmt.Errorf("UUID inválido")
	}

	aggType := aggregate.GetType()
	if err := ValidateAggregateType(aggType); err != nil {
		return err
	}

	version := aggregate.GetVersion()
	if version < 0 {
		version = 0
	}

	safeType := SafeString(aggType)
	safeState := SafeString(string(stateJSON))

	query := fmt.Sprintf(`
		INSERT INTO aggregate_snapshots (aggregate_id, aggregate_type, version, state)
		VALUES ('%s', '%s', %d, '%s')
		ON CONFLICT (aggregate_id) 
		DO UPDATE SET 
			version = EXCLUDED.version,
			state = EXCLUDED.state,
			updated_at = CURRENT_TIMESTAMP`, aggID, safeType, version, safeState)

	// ✅ USAR RETRY
	_, err = r.eventStore.client.ExecWithRetry(query)
	return err
}

// ✅ ATUALIZADO: Usar QueryRowWithRetry
func (r *AggregateRepository) LoadSnapshot(id uuid.UUID) (*Snapshot, error) {
	if id == uuid.Nil {
		return nil, fmt.Errorf("UUID inválido")
	}

	query := fmt.Sprintf(`
		SELECT aggregate_id, aggregate_type, version, state, created_at
		FROM aggregate_snapshots
		WHERE aggregate_id = '%s'`, id)

	var snapshot Snapshot
	
	// ✅ USAR RETRY
	err := r.eventStore.client.QueryRowWithRetry(query).Scan(
		&snapshot.AggregateID,
		&snapshot.AggregateType,
		&snapshot.Version,
		&snapshot.State,
		&snapshot.CreatedAt,
	)
	
	if err == sql.ErrNoRows {
		return nil, nil
	}
	
	if err != nil {
		return nil, err
	}

	return &snapshot, nil
}

type Snapshot struct {
	AggregateID   uuid.UUID       `db:"aggregate_id"`
	AggregateType string          `db:"aggregate_type"`
	Version       int             `db:"version"`
	State         json.RawMessage `db:"state"`
	CreatedAt     time.Time       `db:"created_at"`
}