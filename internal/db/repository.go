package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"spuri/internal/domain/aggregates"
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

// NewAggregateRepository cria um repositório com context.Background() por padrão.
// Para operações em handlers HTTP, use r.WithContext(c.Request.Context()) para
// propagar o deadline e cancelamento da requisição — evita que queries bloqueiem
// indefinidamente se o banco travar ou o cliente desconectar.
func NewAggregateRepository(client *Client) *AggregateRepository {
	return &AggregateRepository{
		eventStore: NewEventStore(client),
		factory:    &aggregates.DefaultAggregateFactory{},
		ctx:        context.Background(),
	}
}

// WithContext retorna uma shallow-copy do repositório usando o context fornecido.
// A instância original não é modificada — seguro compartilhá-la entre requests.
//
// FIX DB-04: context.Background() fixo no construtor ignora o deadline do handler
// HTTP. Queries de Load (aggregates com muitos eventos) ficam sem timeout e podem
// bloquear indefinidamente se o banco travar.
//
// Uso recomendado nos handlers:
//
//	repo := getRepository(c).WithContext(c.Request.Context())
//	agg, err := repo.Load(id, "Estudante")
func (r *AggregateRepository) WithContext(ctx context.Context) *AggregateRepository {
	if ctx == nil {
		ctx = context.Background()
	}
	return &AggregateRepository{
		eventStore: r.eventStore,
		factory:    r.factory,
		ctx:        ctx,
	}
}

// idSetter é uma interface local para injetar o ID no agregado antes
// de aplicar os eventos. Satisfeita por todos os tipos que embarcam *BaseAggregate.
type idSetter interface {
	SetID(uuid.UUID)
}

// Load reconstrói um aggregate a partir dos eventos do ledger.
//
// FIX-REPO-02: valida consistência de aggregate_type em todos os eventos.
//
// FIX-REPO-03: após criar o aggregate via factory (que gera um uuid.New()
// aleatório em NewAcademia/NewEstudante/etc.), o ID é sobrescrito com o `id`
// real passado como parâmetro — ANTES de aplicar qualquer evento.
// Sem este SetID, todo evento levantado por comandos pós-Load usaria um UUID
// aleatório como AggregateID, fazendo SaveWithAudit gravar no ledger sob um
// aggregate inexistente e a projeção nunca atualizar (UPDATE WHERE id = UUID
// errado → 0 linhas afetadas, silencioso).
func (r *AggregateRepository) Load(id uuid.UUID, aggregateType string) (aggregates.Aggregate, error) {
	dbEvents, err := r.eventStore.LoadEventStream(r.ctx, id)
	if err != nil {
		return nil, fmt.Errorf("erro ao carregar eventos: %w", err)
	}

	if len(dbEvents) == 0 {
		return nil, fmt.Errorf("agregado não encontrado: %s", id)
	}

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

	// FIX-REPO-03: a factory gera um UUID aleatório (uuid.New()) que nunca
	// é sobrescrito pelos apply handlers (applyAcademiaCriada, applyEstudanteCriado,
	// etc. não setam a.ID). Sem este SetID, qualquer evento levantado sobre o
	// aggregate reconstruído usaria o UUID errado como AggregateID, corrompendo
	// silenciosamente o ledger e impossibilitando a atualização da projeção.
	if setter, ok := aggregate.(idSetter); ok {
		setter.SetID(id)
	}

	for _, event := range domainEvents {
		if err := aggregate.Apply(event); err != nil {
			return nil, fmt.Errorf("erro ao aplicar evento: %w", err)
		}
	}

	return aggregate, nil
}

// Save persiste os eventos uncommitted do aggregate no ledger.
//
// FIX DB-02: GetAggregateVersion executado DENTRO da transação Serializable
// (via getAggregateVersionTx), eliminando a race condition entre leitura de
// versão e INSERT.
// FIX DB-03: branch "else if err != sql.ErrNoRows" removido — era dead code
// pois COALESCE(MAX(...), 0) nunca retorna sql.ErrNoRows.
func (r *AggregateRepository) Save(aggregate aggregates.Aggregate) error {
	uncommittedEvents := aggregate.GetUncommittedEvents()
	if len(uncommittedEvents) == 0 {
		return nil
	}

	tx, err := r.eventStore.client.BeginTx(r.ctx)
	if err != nil {
		return fmt.Errorf("erro ao iniciar transação: %w", err)
	}
	defer tx.Rollback()

	currentVersion, err := r.getAggregateVersionTx(r.ctx, tx, aggregate.GetID())
	if err != nil {
		return fmt.Errorf("erro ao obter versão: %w", err)
	}

	for i, domainEvent := range uncommittedEvents {
		dbEvent, err := r.dbEvent(domainEvent, aggregate.GetType(), currentVersion+i+1)
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
	notifyLedgerWritten()
	return nil
}

// SaveWithAudit salva os eventos do aggregate com metadata de auditoria completo.
//
// FIX DB-02: versão lida dentro da tx Serializable via getAggregateVersionTx.
// FIX DB-03: branch ErrNoRows (dead code) removido.
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

	currentVersion, err := r.getAggregateVersionTx(r.ctx, tx, aggregate.GetID())
	if err != nil {
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
	notifyLedgerWritten()
	return nil
}

// getAggregateVersionTx retorna a versão máxima do aggregate dentro de uma
// transação em curso. Qualquer erro (timeout, conexão perdida) é propagado —
// não há branch ErrNoRows pois COALESCE nunca retorna zero rows.
func (r *AggregateRepository) getAggregateVersionTx(
	ctx context.Context,
	tx *sqlx.Tx,
	aggregateID uuid.UUID,
) (int, error) {
	if aggregateID == uuid.Nil {
		return 0, fmt.Errorf("UUID inválido")
	}

	var version int
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(event_version), 0)
		FROM spuri_ledger
		WHERE aggregate_id = $1`,
		aggregateID,
	).Scan(&version); err != nil {
		return 0, fmt.Errorf("erro ao obter versão na transação: %w", err)
	}

	return version, nil
}

func (r *AggregateRepository) Exists(id uuid.UUID) (bool, error) {
	count, err := r.eventStore.CountEventsByAggregate(r.ctx, id)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// LoadFromVersion reconstrói um aggregate a partir de uma versão específica.
//
// FIX-REPO-03 (mesma correção do Load): SetID injeta o ID real antes de aplicar
// eventos — a factory gera uuid.New() que causaria AggregateID errado nos eventos.
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

	// FIX-REPO-03: mesma correção do Load — SetID antes de aplicar eventos.
	if setter, ok := aggregate.(idSetter); ok {
		setter.SetID(id)
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

// SaveSnapshot persiste o estado atual do aggregate como snapshot.
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

	_, err = r.eventStore.client.db.ExecContext(r.ctx, `
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
func (r *AggregateRepository) LoadSnapshot(id uuid.UUID) (*Snapshot, error) {
	if id == uuid.Nil {
		return nil, fmt.Errorf("UUID inválido")
	}

	var snapshot Snapshot
	err := r.eventStore.client.db.QueryRowContext(r.ctx, `
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

// Snapshot representa o estado persistido de um aggregate.
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
	payloadJSON, err := json.Marshal(domainEvent.GetPayload())
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar payload do evento %s: %w", domainEvent.GetEventType(), err)
	}

	metadataJSON, err := json.Marshal(map[string]interface{}{
		"timestamp": time.Now().Unix(),
	})
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
	payloadJSON, err := json.Marshal(domainEvent.GetPayload())
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar payload do evento %s: %w", domainEvent.GetEventType(), err)
	}

	metadataJSON, err := json.Marshal(map[string]interface{}{
		"timestamp": time.Now().Unix(),
		"user_id":   audit.UserID,
		"user_type": audit.UserType,
		"ip":        audit.IP,
	})
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

// convertToDomainEvents converte eventos do banco para DomainEvents sem
// double-serialization — Payload (json.RawMessage) é passado diretamente
// como interface{} no BaseEvent. Os apply handlers de cada aggregate
// fazem json.Marshal+Unmarshal do Payload para o tipo concreto esperado.
func (r *AggregateRepository) convertToDomainEvents(dbEvents []Event) ([]aggregates.DomainEvent, error) {
	domainEvents := make([]aggregates.DomainEvent, 0, len(dbEvents))
	for _, ge := range dbEvents {
		event := &aggregates.BaseEvent{
			EventType:   ge.EventType,
			AggregateID: ge.AggregateID,
			Payload:     ge.Payload, // json.RawMessage — preserva exatamente o que foi gravado
		}
		domainEvents = append(domainEvents, event)
	}
	return domainEvents, nil
}

// GetEventByID exposes a single ledger event for authorized audit handlers.
func (r *AggregateRepository) GetEventByID(eventID uuid.UUID) (*Event, error) {
	return r.eventStore.GetEventByID(r.ctx, eventID)
}

// placeholder - wrong file, ignore
