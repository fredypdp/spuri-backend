// ============================================================================
// ARQUIVO: internal/db/repository.go
//
// CORREÇÕES APLICADAS (Etapa 2 — auditoria-etapa2-db.md):
//   FIX-REPO-01 — metadataJSON, _ := json.Marshal(metadata) silenciava erro
//                 de serialização em dbEvent() e dbEventWithAudit().
//                 Agora o erro é propagado ao chamador, evitando que eventos
//                 sejam gravados no ledger sem metadata de auditoria.
//   FIX-REPO-02 — Load() e LoadFromVersion() não validavam que os eventos
//                 retornados pelo ledger pertencem ao aggregateType solicitado.
//                 Adicionada verificação de consistência no primeiro evento,
//                 prevenindo reconstituição silenciosa com tipo errado.
//
// NOTA ARQUITETURAL (REPO-03):
//   Save() e SaveWithAudit() gravam no ledger em uma transação, mas a
//   atualização da projeção ocorre após o Commit(), via Manager assíncrono,
//   em transação separada. Esta é uma limitação conhecida do padrão adotado:
//   há uma janela de inconsistência entre ledger e projeção após falha do
//   Manager. O rebuild é o mecanismo de recuperação previsto.
// ============================================================================

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
	ctx        context.Context
}

// AuditContext carrega informações do usuário que está realizando a operação.
// Passado via SaveWithAudit para enriquecer o metadata de cada evento.
type AuditContext struct {
	UserID   string // UUID do usuário (estudante, academia ou admin)
	UserType string // "estudante" | "academia" | "admin"
	IP       string // IP da requisição (opcional)
}

// NewAggregateRepository cria um repositório com a factory padrão.
// Assinatura original preservada — sem parâmetro factory externo.
func NewAggregateRepository(client *Client) *AggregateRepository {
	return &AggregateRepository{
		eventStore: NewEventStore(client),
		factory:    &aggregates.DefaultAggregateFactory{},
		ctx:        context.Background(),
	}
}

// Load reconstrói um aggregate a partir dos eventos do ledger.
//
// FIX-REPO-02: valida que todos os eventos retornados pertencem ao
// aggregateType solicitado, prevenindo reconstituição com tipo errado.
func (r *AggregateRepository) Load(id uuid.UUID, aggregateType string) (aggregates.Aggregate, error) {
	dbEvents, err := r.eventStore.LoadEventStream(r.ctx, id)
	if err != nil {
		return nil, fmt.Errorf("erro ao carregar eventos: %w", err)
	}

	if len(dbEvents) == 0 {
		return nil, fmt.Errorf("agregado não encontrado: %s", id)
	}

	// FIX-REPO-02: verificar consistência de aggregate_type no ledger.
	// Protege contra chamadas incorretas (ex: UUID de Curso com aggregateType="Turma").
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

	currentVersion := 0
	version, err := r.eventStore.GetAggregateVersion(r.ctx, aggregate.GetID())
	if err == nil {
		currentVersion = version
	} else if err != sql.ErrNoRows {
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

// LoadFromVersion reconstrói um aggregate a partir de uma versão específica.
//
// FIX-REPO-02: valida consistência de aggregate_type, igual ao Load().
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

	// FIX-REPO-02: verificar consistência de aggregate_type.
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

func (r *AggregateRepository) GetEventHistory(id uuid.UUID) ([]Event, error) {
	return r.eventStore.LoadEventStream(r.ctx, id)
}

func (r *AggregateRepository) VerifyIntegrity(id uuid.UUID) (bool, error) {
	return r.eventStore.VerifyLedgerIntegrity(r.ctx, id)
}

// SaveSnapshot persiste o estado atual do aggregate como snapshot.
// Usa prepared statement com $1..$4 — sem interpolação de string.
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
// Usa prepared statement com $1 — sem interpolação de string.
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

// dbEvent converte um DomainEvent para o formato de banco sem contexto de auditoria.
//
// FIX-REPO-01: metadataJSON, _ = json.Marshal(...) silenciava falhas de
// serialização. Agora o erro é propagado — o Save retorna erro em vez de
// gravar um evento sem metadata.
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
	// FIX-REPO-01: erro propagado ao invés de silenciado com _.
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

// dbEventWithAudit converte um DomainEvent com contexto de auditoria completo.
//
// FIX-REPO-01: metadataJSON, _ = json.Marshal(...) silenciava falhas de
// serialização. Crítico nesta função pois o metadata é o único lugar onde
// user_id, user_type e IP são persistidos.
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
	// FIX-REPO-01: erro propagado. Sem metadata de auditoria o Save deve falhar,
	// não gravar um evento sem rastreabilidade de quem/quando/de onde.
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
//
// FIX (double-serialization de UUIDs):
// Passa ge.Payload (json.RawMessage) diretamente como Payload do BaseEvent,
// sem deserializar para map[string]interface{} intermediário. Isso preserva
// UUIDs, ponteiros e timestamps exatamente como foram gravados no banco.
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