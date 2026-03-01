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

func (es *EventStore) Append(ctx context.Context, event *Event) error {
	if event.AggregateID == uuid.Nil {
		return fmt.Errorf("UUID inválido")
	}
	
	if err := ValidateAggregateType(event.AggregateType); err != nil {
		return err
	}
	
	if err := ValidateEventType(event.EventType); err != nil {
		return err
	}

	safePayload := SafeString(string(event.Payload))
	safeMetadata := SafeString(string(event.Metadata))
	
	query := fmt.Sprintf(`
		INSERT INTO spuri_ledger (
			event_id, aggregate_id, aggregate_type, event_type, 
			event_version, payload, metadata, occurred_at
		) VALUES ('%s', '%s', '%s', '%s', %d, '%s', '%s', '%s') 
		RETURNING id, recorded_at, ledger_hash, previous_hash`,
		event.EventID, event.AggregateID, event.AggregateType, event.EventType,
		event.EventVersion, safePayload, safeMetadata, event.OccurredAt.Format(time.RFC3339))

	row := es.client.db.QueryRow(query)

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

	safePayload := SafeString(string(event.Payload))
	safeMetadata := SafeString(string(event.Metadata))

	query := fmt.Sprintf(`
		INSERT INTO spuri_ledger (
			event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at
		) VALUES ('%s', '%s', '%s', '%s', %d, '%s', '%s', '%s')
		RETURNING id, recorded_at, ledger_hash, previous_hash`,
		event.EventID, event.AggregateID, event.AggregateType, event.EventType,
		event.EventVersion, safePayload, safeMetadata, event.OccurredAt.Format(time.RFC3339))

	row := tx.QueryRowContext(ctx, query)

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

	query := fmt.Sprintf(`
		SELECT 
			id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE aggregate_id = '%s'
		ORDER BY event_version ASC, recorded_at ASC`, aggregateID)

	rows, err := es.client.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("erro ao carregar events: %w", err)
	}
	defer rows.Close()

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

// ✅ CORRIGIDO: Query direta
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

	query := fmt.Sprintf(`
		SELECT 
			id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE aggregate_id = '%s' AND event_version >= %d
		ORDER BY event_version ASC, recorded_at ASC`, aggregateID, fromVersion)

	rows, err := es.client.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("erro ao carregar events: %w", err)
	}
	defer rows.Close()

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

// ✅ CORRIGIDO: Query direta
func (es *EventStore) GetEventsByType(
	ctx context.Context, 
	eventType string, 
	limit int,
) ([]Event, error) {
	if err := ValidateEventType(eventType); err != nil {
		return nil, err
	}

	limit = ValidateLimit(limit)
	safeType := SafeString(eventType)

	query := fmt.Sprintf(`
		SELECT 
			id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE event_type = '%s'
		ORDER BY recorded_at DESC
		LIMIT %d`, safeType, limit)

	rows, err := es.client.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar eventos: %w", err)
	}
	defer rows.Close()

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

// ✅ CORRIGIDO: Query direta
func (es *EventStore) GetAllEvents(
	ctx context.Context, 
	offset, limit int,
) ([]Event, error) {
	offset = ValidateOffset(offset)
	limit = ValidateLimit(limit)

	query := fmt.Sprintf(`
		SELECT 
			id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		ORDER BY recorded_at DESC
		LIMIT %d OFFSET %d`, limit, offset)

	rows, err := es.client.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar eventos: %w", err)
	}
	defer rows.Close()

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

// ✅ CORRIGIDO: Query direta
func (es *EventStore) GetEventByID(ctx context.Context, eventID uuid.UUID) (*Event, error) {
	if eventID == uuid.Nil {
		return nil, fmt.Errorf("UUID inválido")
	}

	query := fmt.Sprintf(`
		SELECT 
			id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE event_id = '%s'`, eventID)

	var event Event
	var prevHash sql.NullString
	err := es.client.db.QueryRow(query).Scan(
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

// ✅ CORRIGIDO: Query direta
func (es *EventStore) GetAggregateVersion(ctx context.Context, aggregateID uuid.UUID) (int, error) {
	if aggregateID == uuid.Nil {
		return 0, fmt.Errorf("UUID inválido")
	}

	query := fmt.Sprintf(`
		SELECT COALESCE(MAX(event_version), 0)
		FROM spuri_ledger
		WHERE aggregate_id = '%s'`, aggregateID)

	var version int
	err := es.client.db.QueryRow(query).Scan(&version)
	
	if err == sql.ErrNoRows {
		return 0, nil
	}
	
	if err != nil {
		return 0, fmt.Errorf("erro ao obter versão: %w", err)
	}

	return version, nil
}

// ✅ CORRIGIDO: Query direta
func (es *EventStore) VerifyLedgerIntegrity(ctx context.Context, aggregateID uuid.UUID) (bool, error) {
	if aggregateID == uuid.Nil {
		return false, fmt.Errorf("UUID inválido")
	}

	query := fmt.Sprintf(`
		SELECT ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE aggregate_id = '%s'
		ORDER BY event_version ASC`, aggregateID)

	type hashPair struct {
		LedgerHash   string
		PreviousHash sql.NullString
	}

	rows, err := es.client.db.Query(query)
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
		if !hashes[i].PreviousHash.Valid {
			return false, fmt.Errorf("hash anterior ausente no evento %d", i)
		}
		if hashes[i].PreviousHash.String != hashes[i-1].LedgerHash {
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

// ✅ CORRIGIDO: Query direta
func (es *EventStore) CountEventsByAggregate(ctx context.Context, aggregateID uuid.UUID) (int64, error) {
	if aggregateID == uuid.Nil {
		return 0, fmt.Errorf("UUID inválido")
	}

	query := fmt.Sprintf(`SELECT COUNT(*) FROM spuri_ledger WHERE aggregate_id = '%s'`, aggregateID)
	
	var count int64
	err := es.client.db.QueryRow(query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("erro ao contar eventos: %w", err)
	}

	return count, nil
}