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

// appendDirect insere um evento no ledger imutável fora de uma transação.
// Método interno (unexported) — uso externo ao pacote db deve passar por
// AggregateRepository.Save / SaveWithAudit que usam AppendTx com tx Serializable.
//
// FIX DB-07: renomeado de Append (público) para appendDirect (interno) para
// evitar que código externo ao pacote grave eventos diretamente no ledger
// contornando a serialização de versão do repositório.
func (es *EventStore) appendDirect(ctx context.Context, event *Event) error {
	if event.AggregateID == uuid.Nil {
		return fmt.Errorf("UUID inválido")
	}

	if err := ValidateAggregateType(event.AggregateType); err != nil {
		return err
	}

	if err := ValidateEventType(event.EventType); err != nil {
		return err
	}

	row := es.client.db.QueryRowContext(ctx, `
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
		return fmt.Errorf("erro ao adicionar evento: %w", err)
	}

	if prevHash.Valid {
		event.PreviousHash = &prevHash.String
	}

	return nil
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
		ORDER BY event_version ASC, recorded_at ASC`,
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
		ORDER BY event_version ASC, recorded_at ASC`,
		aggregateID, fromVersion,
	)
	if err != nil {
		return nil, fmt.Errorf("erro ao carregar events: %w", err)
	}
	defer rows.Close()

	return scanEvents(rows)
}

// GetEventsByType retorna eventos de um tipo específico ordenados de forma determinística.
//
// FIX DB-05: adicionado id DESC como critério de desempate para eventos com
// o mesmo recorded_at (inseridos na mesma transação). Garante ordem estável
// e reprodutível entre execuções.
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

// GetAllEvents retorna todos os eventos do ledger ordenados de forma determinística.
//
// FIX DB-06: adicionado id DESC como critério de desempate para eventos com
// o mesmo recorded_at. Garante ordem estável e reprodutível entre execuções,
// crítico para auditoria forense.
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

	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("erro ao obter versão: %w", err)
	}

	return version, nil
}

func (es *EventStore) VerifyLedgerIntegrity(ctx context.Context, aggregateID uuid.UUID) (bool, error) {
	if aggregateID == uuid.Nil {
		return false, fmt.Errorf("UUID inválido")
	}

	// verify_hash_chain é uma função SQL definida no banco — recebe UUID como argumento.
	// Usamos $1 como placeholder para o UUID, evitando interpolação de string.
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

// scanEvents é um helper que percorre *sql.Rows e escaneia cada linha para Event.
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

// GetDistinctAggregateIDs retorna a lista de todos os aggregate_id únicos presentes no ledger.
//
// Usado por verifyFullLedgerIntegrity no Manager para verificar a integridade
// de todos os aggregates antes de um rebuild — sem precisar conhecer os tipos ou IDs antecipadamente.
//
// A query é simples e rápida graças ao índice idx_spuri_aggregate já existente em (aggregate_id, event_version).
func (es *EventStore) GetDistinctAggregateIDs(ctx context.Context) ([]uuid.UUID, error) {
	rows, err := es.client.db.QueryContext(ctx, `
		SELECT DISTINCT aggregate_id FROM spuri_ledger ORDER BY aggregate_id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("GetDistinctAggregateIDs: erro na query: %w", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("GetDistinctAggregateIDs: erro ao scan: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}