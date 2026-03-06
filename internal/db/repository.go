package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"spuri/internal/domain/aggregates"
)

type AggregateRepository struct {
	eventStore *EventStore
	factory    aggregates.AggregateFactory
}

// AuditContext carrega informações do usuário que está realizando a operação.
// Passado via SaveWithAudit para enriquecer o metadata de cada evento.
type AuditContext struct {
	UserID   string // UUID do usuário (estudante, academia ou admin)
	UserType string // "estudante" | "academia" | "admin"
	IP       string // IP da requisição (opcional)
}

// NewAggregateRepository cria um repositório com a factory padrão.
//
// DB-04 FIX: ctx foi removido do struct. Cada operação recebe agora o contexto
// da chamada (vindo do handler HTTP), permitindo timeout e cancelamento por
// requisição. context.Background() não é mais atribuído no construtor.
func NewAggregateRepository(client *Client) *AggregateRepository {
	return &AggregateRepository{
		eventStore: NewEventStore(client),
		factory:    &aggregates.DefaultAggregateFactory{},
	}
}

// Load reconstrói um aggregate a partir dos eventos do ledger.
//
// DB-04 FIX: recebe ctx da chamada (handler HTTP) em vez de usar context.Background() fixo.
// DB-02/DB-03 FIX (via Save): GetAggregateVersion movido para dentro da transação (ver Save).
func (r *AggregateRepository) Load(ctx context.Context, id uuid.UUID, aggregateType string) (aggregates.Aggregate, error) {
	dbEvents, err := r.eventStore.LoadEventStream(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("erro ao carregar eventos: %w", err)
	}

	if len(dbEvents) == 0 {
		return nil, fmt.Errorf("agregado não encontrado: %s", id)
	}

	// Verifica consistência de aggregate_type no ledger.
	for _, ge := range dbEvents {
		if ge.AggregateType != aggregateType {
			return nil, fmt.Errorf(
				"inconsistência de tipo no ledger: evento %q (id=%d) pertence ao aggregate %q, esperado %q — verifique o UUID utilizado",
				ge.EventType, ge.ID, ge.AggregateType, aggregateType,
			)
		}
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

// Save persiste os eventos não-commitados de um aggregate no ledger.
//
// DB-02 FIX: GetAggregateVersion é chamado DENTRO da transação com FOR UPDATE,
// eliminando a race condition entre a leitura da versão e o INSERT.
// O banco retorna erro de constraint UNIQUE(aggregate_id, event_version) se
// duas transações concorrentes tentarem gravar a mesma versão — o erro é
// propagado claramente ao caller para retry ou rejeição.
//
// DB-03 FIX: GetAggregateVersion usa SELECT COALESCE(MAX, 0) que nunca
// retorna sql.ErrNoRows. O tratamento correto é: qualquer erro != nil
// deve ser retornado imediatamente. O branch "else if" que silenciava
// erros reais foi removido.
//
// DB-04 FIX: recebe ctx da chamada.
func (r *AggregateRepository) Save(ctx context.Context, aggregate aggregates.Aggregate) error {
	uncommittedEvents := aggregate.GetUncommittedEvents()
	if len(uncommittedEvents) == 0 {
		return nil
	}

	tx, err := r.eventStore.client.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("erro ao iniciar transação: %w", err)
	}
	defer tx.Rollback()

	// DB-02 FIX: leitura da versão DENTRO da transação com lock de linha.
	// SELECT ... FOR UPDATE garante que nenhuma outra transação insira eventos
	// para o mesmo aggregate_id entre esta leitura e o INSERT abaixo.
	var currentVersion int
	err = tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(event_version), 0)
		FROM spuri_ledger
		WHERE aggregate_id = $1
		FOR UPDATE`,
		aggregate.GetID(),
	).Scan(&currentVersion)
	// DB-03 FIX: COALESCE nunca retorna ErrNoRows. Qualquer erro aqui é real
	// (timeout, conexão morta, etc.) e deve ser propagado.
	if err != nil {
		return fmt.Errorf("erro ao obter versão do aggregate: %w", err)
	}

	for i, domainEvent := range uncommittedEvents {
		dbEvent, err := r.dbEvent(domainEvent, aggregate.GetType(), currentVersion+i+1)
		if err != nil {
			return fmt.Errorf("erro ao converter evento: %w", err)
		}

		if err := r.eventStore.AppendTx(ctx, tx, dbEvent); err != nil {
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
//
// DB-02 FIX: GetAggregateVersion dentro da transação com FOR UPDATE.
// DB-03 FIX: erro propagado corretamente.
// DB-04 FIX: recebe ctx da chamada.
func (r *AggregateRepository) SaveWithAudit(ctx context.Context, aggregate aggregates.Aggregate, audit AuditContext) error {
	uncommittedEvents := aggregate.GetUncommittedEvents()
	if len(uncommittedEvents) == 0 {
		return nil
	}

	tx, err := r.eventStore.client.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("erro ao iniciar transação: %w", err)
	}
	defer tx.Rollback()

	// DB-02 FIX: versão lida dentro da transação com lock.
	var currentVersion int
	err = tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(event_version), 0)
		FROM spuri_ledger
		WHERE aggregate_id = $1
		FOR UPDATE`,
		aggregate.GetID(),
	).Scan(&currentVersion)
	if err != nil {
		return fmt.Errorf("erro ao obter versão do aggregate: %w", err)
	}

	for i, domainEvent := range uncommittedEvents {
		dbEvent, err := r.dbEventWithAudit(domainEvent, aggregate.GetType(), currentVersion+i+1, audit)
		if err != nil {
			return fmt.Errorf("erro ao converter evento: %w", err)
		}

		if err := r.eventStore.AppendTx(ctx, tx, dbEvent); err != nil {
			return fmt.Errorf("erro ao salvar evento: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("erro ao confirmar transação: %w", err)
	}

	aggregate.ClearUncommittedEvents()
	return nil
}

// Exists verifica se um aggregate com o ID fornecido existe no ledger.
// DB-04 FIX: recebe ctx da chamada.
func (r *AggregateRepository) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	count, err := r.eventStore.CountEventsByAggregate(ctx, id)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// LoadFromVersion reconstrói um aggregate a partir de uma versão específica.
// DB-04 FIX: recebe ctx da chamada.
func (r *AggregateRepository) LoadFromVersion(
	ctx context.Context,
	id uuid.UUID,
	aggregateType string,
	fromVersion int,
) (aggregates.Aggregate, error) {
	dbEvents, err := r.eventStore.LoadEventStreamFromVersion(ctx, id, fromVersion)
	if err != nil {
		return nil, fmt.Errorf("erro ao carregar eventos: %w", err)
	}

	if len(dbEvents) == 0 {
		return nil, fmt.Errorf("nenhum evento encontrado")
	}

	for _, ge := range dbEvents {
		if ge.AggregateType != aggregateType {
			return nil, fmt.Errorf(
				"inconsistência de tipo no ledger: evento %q (id=%d) pertence ao aggregate %q, esperado %q",
				ge.EventType, ge.ID, ge.AggregateType, aggregateType,
			)
		}
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

// GetEventHistory retorna o histórico de eventos de um aggregate.
// DB-04 FIX: recebe ctx da chamada.
func (r *AggregateRepository) GetEventHistory(ctx context.Context, id uuid.UUID) ([]Event, error) {
	return r.eventStore.LoadEventStream(ctx, id)
}

// VerifyIntegrity verifica a integridade da hash chain de um aggregate.
// DB-04 FIX: recebe ctx da chamada.
func (r *AggregateRepository) VerifyIntegrity(ctx context.Context, id uuid.UUID) (bool, error) {
	return r.eventStore.VerifyLedgerIntegrity(ctx, id)
}

// SaveSnapshot persiste o estado atual do aggregate como snapshot.
// DB-04 FIX: recebe ctx da chamada.
func (r *AggregateRepository) SaveSnapshot(ctx context.Context, aggregate aggregates.Aggregate) error {
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

	_, err = r.eventStore.client.db.ExecContext(ctx, `
		INSERT INTO aggregate_snapshots (aggregate_id, aggregate_type, version, state)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (aggregate_id)
		DO UPDATE SET
			version    = EXCLUDED.version,
			state      = EXCLUDED.state,
			updated_at = CURRENT_TIMESTAMP`,
		aggID, aggType, version, stateJSON,
	)
	return err
}

// LoadSnapshot carrega o snapshot mais recente de um aggregate.
// DB-04 FIX: recebe ctx da chamada.
func (r *AggregateRepository) LoadSnapshot(ctx context.Context, id uuid.UUID) (*Snapshot, error) {
	if id == uuid.Nil {
		return nil, fmt.Errorf("UUID inválido")
	}

	var snapshot Snapshot
	err := r.eventStore.client.db.QueryRowContext(ctx, `
		SELECT aggregate_id, aggregate_type, version, state, created_at
		FROM aggregate_snapshots
		WHERE aggregate_id = $1`,
		id,
	).Scan(
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

// Snapshot representa um estado salvo de um aggregate.
type Snapshot struct {
	AggregateID   uuid.UUID       `db:"aggregate_id"`
	AggregateType string          `db:"aggregate_type"`
	Version       int             `db:"version"`
	State         json.RawMessage `db:"state"`
	CreatedAt     time.Time       `db:"created_at"`
}

// ============================================================================
// Helpers internos
// ============================================================================

func (r *AggregateRepository) dbEvent(
	domainEvent aggregates.DomainEvent,
	aggregateType string,
	version int,
) (*Event, error) {
	payload := domainEvent.GetPayload()

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar payload do evento %s: %w", domainEvent.GetEventType(), err)
	}

	metadata := map[string]interface{}{
		"timestamp": time.Now().Unix(),
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar metadata do evento %s: %w", domainEvent.GetEventType(), err)
	}

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

func (r *AggregateRepository) dbEventWithAudit(
	domainEvent aggregates.DomainEvent,
	aggregateType string,
	version int,
	audit AuditContext,
) (*Event, error) {
	payload := domainEvent.GetPayload()

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar payload do evento %s: %w", domainEvent.GetEventType(), err)
	}

	metadata := map[string]interface{}{
		"timestamp": time.Now().Unix(),
		"user_id":   audit.UserID,
		"user_type": audit.UserType,
		"ip":        audit.IP,
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar metadata de auditoria do evento %s: %w", domainEvent.GetEventType(), err)
	}

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

// convertToDomainEvents converte eventos do banco para DomainEvents.
// Passa ge.Payload (json.RawMessage) diretamente como Payload do BaseEvent,
// sem deserializar para map[string]interface{} intermediário.
func (r *AggregateRepository) convertToDomainEvents(dbEvents []Event) ([]aggregates.DomainEvent, error) {
	domainEvents := make([]aggregates.DomainEvent, 0, len(dbEvents))

	for _, ge := range dbEvents {
		if !json.Valid(ge.Payload) {
			return nil, fmt.Errorf("payload inválido para evento %s (ledger id=%d)", ge.EventType, ge.ID)
		}

		domainEvent := &aggregates.BaseEvent{
			EventType:   ge.EventType,
			AggregateID: ge.AggregateID,
			Payload:     ge.Payload,
		}

		domainEvents = append(domainEvents, domainEvent)
	}

	return domainEvents, nil
}