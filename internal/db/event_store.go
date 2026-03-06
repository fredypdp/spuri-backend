package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type Event struct {
	ID            int64           `json:"-" db:"id"`
	EventID       uuid.UUID       `json:"event_id" db:"event_id"`
	AggregateID   uuid.UUID       `json:"aggregate_id" db:"aggregate_id"`
	AggregateType string          `json:"aggregate_type" db:"aggregate_type"`
	EventType     string          `json:"event_type" db:"event_type"`
	EventVersion  int             `json:"event_version" db:"event_version"`
	Payload       json.RawMessage `json:"payload" db:"payload"`
	Metadata      json.RawMessage `json:"metadata" db:"metadata"`
	OccurredAt    time.Time       `json:"occurred_at" db:"occurred_at"`
	RecordedAt    time.Time       `json:"recorded_at" db:"recorded_at"`
	LedgerHash    string          `json:"ledger_hash" db:"ledger_hash"`
	PreviousHash  *string         `json:"previous_hash,omitempty" db:"previous_hash"`
}

type EventStore struct {
	client *Client
}

func NewEventStore(client *Client) *EventStore {
	return &EventStore{client: client}
}

// appendInternal é o helper compartilhado que realiza o INSERT no ledger.
// Centraliza a lógica comum entre Append e AppendTx.
func appendInternal(
	ctx context.Context,
	queryRow func(query string, args ...interface{}) *sql.Row,
	event *Event,
) error {
	if event.AggregateID == uuid.Nil {
		return fmt.Errorf("UUID inválido")
	}
	if err := ValidateAggregateType(event.AggregateType); err != nil {
		return err
	}
	if err := ValidateEventType(event.EventType); err != nil {
		return err
	}

	row := queryRow(`
		INSERT INTO spuri_ledger (
			event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, recorded_at, ledger_hash, previous_hash`,
		event.EventID, event.AggregateID, event.AggregateType, event.EventType,
		event.EventVersion, event.Payload, event.Metadata, event.OccurredAt,
	)

	var prevHash sql.NullString
	if err := row.Scan(&event.ID, &event.RecordedAt, &event.LedgerHash, &prevHash); err != nil {
		return fmt.Errorf("erro ao adicionar evento: %w", err)
	}
	if prevHash.Valid {
		event.PreviousHash = &prevHash.String
	}
	return nil
}

// append insere um evento no ledger fora de transação.
// DB-07 FIX: método renomeado para minúsculo (privado) para impedir que código
// externo ao pacote db contorne a serialização de versão do AggregateRepository.
// Todo código externo deve usar AggregateRepository.Save / SaveWithAudit.
// Mantido para uso interno (ex: testes de integração no pacote db).
func (es *EventStore) append(ctx context.Context, event *Event) error {
	return appendInternal(ctx, func(query string, args ...interface{}) *sql.Row {
		return es.client.db.QueryRowContext(ctx, query, args...)
	}, event)
}

// AppendTx insere um evento dentro de uma transação já iniciada.
// Usado por AggregateRepository.Save para garantir atomicidade
// quando um aggregate emite múltiplos eventos em uma única operação.
func (es *EventStore) AppendTx(ctx context.Context, tx *sqlx.Tx, event *Event) error {
	if event.AggregateID == uuid.Nil {
		return fmt.Errorf("UUID inválido")
	}
	if err := ValidateAggregateType(event.AggregateType); err != nil {
		return err
	}
	if err := ValidateEventType(event.EventType); err != nil {
		return err
	}

	row := tx.QueryRowContext(ctx, `
		INSERT INTO spuri_ledger (
			event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, recorded_at, ledger_hash, previous_hash`,
		event.EventID, event.AggregateID, event.AggregateType, event.EventType,
		event.EventVersion, event.Payload, event.Metadata, event.OccurredAt,
	)

	var prevHash sql.NullString
	err := row.Scan(&event.ID, &event.RecordedAt, &event.LedgerHash, &prevHash)
	if err != nil {
		return fmt.Errorf("erro ao adicionar evento na transação: %w", err)
	}
	if prevHash.Valid {
		event.PreviousHash = &prevHash.String
	}
	return nil
}

func (es *EventStore) LoadEventStream(ctx context.Context, aggregateID uuid.UUID) ([]Event, error) {
	if aggregateID == uuid.Nil {
		return nil, fmt.Errorf("UUID inválido")
	}

	rows, err := es.client.db.QueryContext(ctx, `
		SELECT
			id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE aggregate_id = $1
		ORDER BY event_version ASC, id ASC`,
		aggregateID,
	)
	if err != nil {
		return nil, fmt.Errorf("erro ao carregar events: %w", err)
	}
	defer rows.Close()

	return scanEvents(rows)
}

func (es *EventStore) LoadEventStreamFromVersion(
	ctx context.Context,
	aggregateID uuid.UUID,
	fromVersion int,
) ([]Event, error) {
	if aggregateID == uuid.Nil {
		return nil, fmt.Errorf("UUID inválido")
	}
	if fromVersion < 0 {
		fromVersion = 0
	}

	rows, err := es.client.db.QueryContext(ctx, `
		SELECT
			id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE aggregate_id = $1 AND event_version >= $2
		ORDER BY event_version ASC, id ASC`,
		aggregateID, fromVersion,
	)
	if err != nil {
		return nil, fmt.Errorf("erro ao carregar events: %w", err)
	}
	defer rows.Close()

	return scanEvents(rows)
}

// GetEventsByType retorna eventos filtrados por tipo, ordenados do mais recente ao mais antigo.
//
// DB-05 FIX: ORDER BY agora usa (recorded_at DESC, id DESC) como segundo critério de
// desempate, tornando a ordem determinística para eventos com mesmo timestamp.
func (es *EventStore) GetEventsByType(
	ctx context.Context,
	eventType string,
	limit int,
) ([]Event, error) {
	if err := ValidateEventType(eventType); err != nil {
		return nil, err
	}

	limit = ValidateLimit(limit)

	rows, err := es.client.db.QueryContext(ctx, `
		SELECT
			id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE event_type = $1
		ORDER BY recorded_at DESC, id DESC
		LIMIT $2`,
		eventType, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar eventos: %w", err)
	}
	defer rows.Close()

	return scanEvents(rows)
}

// GetAllEvents retorna todos os eventos do ledger paginados, do mais recente ao mais antigo.
//
// DB-06 FIX: ORDER BY agora usa (recorded_at DESC, id DESC) como segundo critério de
// desempate, tornando a listagem determinística para eventos com mesmo timestamp.
func (es *EventStore) GetAllEvents(
	ctx context.Context,
	offset, limit int,
) ([]Event, error) {
	offset = ValidateOffset(offset)
	limit = ValidateLimit(limit)

	rows, err := es.client.db.QueryContext(ctx, `
		SELECT
			id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		ORDER BY recorded_at DESC, id DESC
		LIMIT $1 OFFSET $2`,
		limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar eventos: %w", err)
	}
	defer rows.Close()

	return scanEvents(rows)
}

func (es *EventStore) GetEventByID(ctx context.Context, eventID uuid.UUID) (*Event, error) {
	if eventID == uuid.Nil {
		return nil, fmt.Errorf("UUID inválido")
	}

	var event Event
	var prevHash sql.NullString
	err := es.client.db.QueryRowContext(ctx, `
		SELECT
			id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE event_id = $1`,
		eventID,
	).Scan(
		&event.ID, &event.EventID, &event.AggregateID, &event.AggregateType,
		&event.EventType, &event.EventVersion, &event.Payload, &event.Metadata,
		&event.OccurredAt, &event.RecordedAt, &event.LedgerHash, &prevHash,
	)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar evento: %w", err)
	}
	if prevHash.Valid {
		event.PreviousHash = &prevHash.String
	}
	return &event, nil
}

// GetAggregateVersion retorna a versão máxima de um aggregate no ledger.
// Usa SELECT COALESCE(MAX(...), 0) que nunca retorna sql.ErrNoRows.
// Nota: esta função existe para uso diagnóstico. O Save/SaveWithAudit leem
// a versão DENTRO da transação com FOR UPDATE (ver repository.go).
func (es *EventStore) GetAggregateVersion(ctx context.Context, aggregateID uuid.UUID) (int, error) {
	if aggregateID == uuid.Nil {
		return 0, fmt.Errorf("UUID inválido")
	}

	var version int
	err := es.client.db.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(event_version), 0)
		FROM spuri_ledger
		WHERE aggregate_id = $1`,
		aggregateID,
	).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("erro ao obter versão: %w", err)
	}
	return version, nil
}

func (es *EventStore) VerifyLedgerIntegrity(ctx context.Context, aggregateID uuid.UUID) (bool, error) {
	if aggregateID == uuid.Nil {
		return false, fmt.Errorf("UUID inválido")
	}

	var (
		isValid  bool
		brokenAt *int
		message  string
	)

	err := es.client.db.QueryRowContext(ctx,
		`SELECT is_valid, broken_at_version, message FROM verify_hash_chain($1)`,
		aggregateID,
	).Scan(&isValid, &brokenAt, &message)
	if err != nil {
		return false, fmt.Errorf("erro ao verificar integridade via SQL: %w", err)
	}

	if !isValid {
		if brokenAt != nil {
			return false, fmt.Errorf("integridade comprometida na versão %d: %s", *brokenAt, message)
		}
		return false, fmt.Errorf("integridade comprometida: %s", message)
	}

	return true, nil
}

func (es *EventStore) CountEvents(ctx context.Context) (int64, error) {
	var count int64
	err := es.client.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM spuri_ledger`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("erro ao contar eventos: %w", err)
	}
	return count, nil
}

func (es *EventStore) CountEventsByAggregate(ctx context.Context, aggregateID uuid.UUID) (int64, error) {
	if aggregateID == uuid.Nil {
		return 0, fmt.Errorf("UUID inválido")
	}

	var count int64
	err := es.client.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM spuri_ledger WHERE aggregate_id = $1`,
		aggregateID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("erro ao contar eventos: %w", err)
	}
	return count, nil
}

// scanEvents percorre *sql.Rows e escaneia cada linha para Event.
func scanEvents(rows *sql.Rows) ([]Event, error) {
	var events []Event
	for rows.Next() {
		var event Event
		var prevHash sql.NullString
		err := rows.Scan(
			&event.ID, &event.EventID, &event.AggregateID, &event.AggregateType,
			&event.EventType, &event.EventVersion, &event.Payload, &event.Metadata,
			&event.OccurredAt, &event.RecordedAt, &event.LedgerHash, &prevHash,
		)
		if err != nil {
			return nil, err
		}
		if prevHash.Valid {
			event.PreviousHash = &prevHash.String
		}
		events = append(events, event)
	}
	return events, rows.Err()
}