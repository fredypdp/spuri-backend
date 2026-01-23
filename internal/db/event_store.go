package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
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

// ✅ SAFE: Mantém QueryRowContext pois usa RETURNING (necessário)
func (es *EventStore) Append(ctx context.Context, event *Event) error {
	query := `
		INSERT INTO spuri_ledger (
			event_id, aggregate_id, aggregate_type, event_type, 
			event_version, payload, metadata, occurred_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8) 
		RETURNING id, recorded_at, ledger_hash, previous_hash`

	row := es.client.db.QueryRowContext(ctx, query,
		event.EventID,
		event.AggregateID,
		event.AggregateType,
		event.EventType,
		event.EventVersion,
		event.Payload,
		event.Metadata,
		event.OccurredAt,
	)

	err := row.Scan(&event.ID, &event.RecordedAt, &event.LedgerHash, &event.PreviousHash)
	if err != nil {
		return fmt.Errorf("erro ao adicionar evento: %w", err)
	}

	return nil
}

// ✅ CORRIGIDO: Usar Query() padrão ao invés de Queryx()
func (es *EventStore) LoadEventStream(ctx context.Context, aggregateID uuid.UUID) ([]Event, error) {
	if aggregateID == uuid.Nil {
		return nil, fmt.Errorf("UUID inválido")
	}

	query := `
		SELECT 
			id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE aggregate_id = $1
		ORDER BY event_version ASC, recorded_at ASC`

	rows, err := es.client.db.Query(query, aggregateID)
	if err != nil {
		return nil, fmt.Errorf("erro ao carregar events: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var event Event
		err := rows.Scan(
			&event.ID, &event.EventID, &event.AggregateID, &event.AggregateType,
			&event.EventType, &event.EventVersion, &event.Payload, &event.Metadata,
			&event.OccurredAt, &event.RecordedAt, &event.LedgerHash, &event.PreviousHash,
		)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}

	return events, rows.Err()
}

// ✅ CORRIGIDO: Usar Query() padrão
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

	query := `
		SELECT 
			id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE aggregate_id = $1 AND event_version >= $2
		ORDER BY event_version ASC, recorded_at ASC`

	rows, err := es.client.db.Query(query, aggregateID, fromVersion)
	if err != nil {
		return nil, fmt.Errorf("erro ao carregar events: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var event Event
		err := rows.Scan(
			&event.ID, &event.EventID, &event.AggregateID, &event.AggregateType,
			&event.EventType, &event.EventVersion, &event.Payload, &event.Metadata,
			&event.OccurredAt, &event.RecordedAt, &event.LedgerHash, &event.PreviousHash,
		)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}

	return events, rows.Err()
}

// ✅ CORRIGIDO: Usar Query() padrão
func (es *EventStore) GetEventsByType(
	ctx context.Context, 
	eventType string, 
	limit int,
) ([]Event, error) {
	if err := ValidateEventType(eventType); err != nil {
		return nil, err
	}

	limit = ValidateLimit(limit)

	query := `
		SELECT 
			id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE event_type = $1
		ORDER BY recorded_at DESC
		LIMIT $2`

	rows, err := es.client.db.Query(query, eventType, limit)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar eventos: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var event Event
		err := rows.Scan(
			&event.ID, &event.EventID, &event.AggregateID, &event.AggregateType,
			&event.EventType, &event.EventVersion, &event.Payload, &event.Metadata,
			&event.OccurredAt, &event.RecordedAt, &event.LedgerHash, &event.PreviousHash,
		)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}

	return events, rows.Err()
}

// ✅ CORRIGIDO: Usar Query() padrão
func (es *EventStore) GetAllEvents(
	ctx context.Context, 
	offset, limit int,
) ([]Event, error) {
	offset = ValidateOffset(offset)
	limit = ValidateLimit(limit)

	query := `
		SELECT 
			id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		ORDER BY recorded_at DESC
		LIMIT $1 OFFSET $2`

	rows, err := es.client.db.Query(query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar eventos: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var event Event
		err := rows.Scan(
			&event.ID, &event.EventID, &event.AggregateID, &event.AggregateType,
			&event.EventType, &event.EventVersion, &event.Payload, &event.Metadata,
			&event.OccurredAt, &event.RecordedAt, &event.LedgerHash, &event.PreviousHash,
		)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}

	return events, rows.Err()
}

// ✅ CORRIGIDO: Usar QueryRow() padrão
func (es *EventStore) GetEventByID(ctx context.Context, eventID uuid.UUID) (*Event, error) {
	if eventID == uuid.Nil {
		return nil, fmt.Errorf("UUID inválido")
	}

	query := `
		SELECT 
			id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE event_id = $1`

	var event Event
	err := es.client.db.QueryRow(query, eventID).Scan(
		&event.ID, &event.EventID, &event.AggregateID, &event.AggregateType,
		&event.EventType, &event.EventVersion, &event.Payload, &event.Metadata,
		&event.OccurredAt, &event.RecordedAt, &event.LedgerHash, &event.PreviousHash,
	)
	
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar evento: %w", err)
	}

	return &event, nil
}

// ✅ CORRIGIDO: Usar QueryRow com placeholder
func (es *EventStore) GetAggregateVersion(ctx context.Context, aggregateID uuid.UUID) (int, error) {
	if aggregateID == uuid.Nil {
		return 0, fmt.Errorf("UUID inválido")
	}

	query := `
		SELECT COALESCE(MAX(event_version), 0)
		FROM spuri_ledger
		WHERE aggregate_id = $1`

	var version int
	err := es.client.db.QueryRow(query, aggregateID).Scan(&version)
	
	if err == sql.ErrNoRows {
		return 0, nil
	}
	
	if err != nil {
		return 0, fmt.Errorf("erro ao obter versão: %w", err)
	}

	return version, nil
}

// ✅ CORRIGIDO: Usar Query() padrão
func (es *EventStore) VerifyLedgerIntegrity(ctx context.Context, aggregateID uuid.UUID) (bool, error) {
	if aggregateID == uuid.Nil {
		return false, fmt.Errorf("UUID inválido")
	}

	query := `
		SELECT ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE aggregate_id = $1
		ORDER BY event_version ASC`

	type hashPair struct {
		LedgerHash   string
		PreviousHash *string
	}

	rows, err := es.client.db.Query(query, aggregateID)
	if err != nil {
		return false, fmt.Errorf("erro ao verificar integridade: %w", err)
	}
	defer rows.Close()

	var hashes []hashPair
	for rows.Next() {
		var hp hashPair
		err := rows.Scan(&hp.LedgerHash, &hp.PreviousHash)
		if err != nil {
			return false, err
		}
		hashes = append(hashes, hp)
	}

	for i := 1; i < len(hashes); i++ {
		if hashes[i].PreviousHash == nil {
			return false, fmt.Errorf("hash anterior ausente no evento %d", i)
		}
		if *hashes[i].PreviousHash != hashes[i-1].LedgerHash {
			return false, fmt.Errorf("cadeia de hashes quebrada no evento %d", i)
		}
	}

	return true, nil
}

func (es *EventStore) CountEvents(ctx context.Context) (int64, error) {
	query := `SELECT COUNT(*) FROM spuri_ledger`
	
	var count int64
	err := es.client.db.QueryRow(query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("erro ao contar eventos: %w", err)
	}

	return count, nil
}

// ✅ CORRIGIDO: Usar QueryRow com placeholder ao invés de fmt.Sprintf
func (es *EventStore) CountEventsByAggregate(ctx context.Context, aggregateID uuid.UUID) (int64, error) {
	if aggregateID == uuid.Nil {
		return 0, fmt.Errorf("UUID inválido")
	}

	query := `SELECT COUNT(*) FROM spuri_ledger WHERE aggregate_id = $1`
	
	var count int64
	err := es.client.db.QueryRow(query, aggregateID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("erro ao contar eventos: %w", err)
	}

	return count, nil
}