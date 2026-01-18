// ============================================================================
// ARQUIVO: internal/db/event_store.go
// 🔥 CORRIGIDO: Todas as queries usando Exec/Query direto sem prepared statements
// ============================================================================

package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Event representa um evento no Banco de dados Ledger
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
	
	// Banco de dados specific - imutabilidade garantida
	LedgerHash    string          `json:"ledger_hash" db:"ledger_hash"`
	PreviousHash  *string         `json:"previous_hash,omitempty" db:"previous_hash"`
}

// EventMetadata metadados do evento
type EventMetadata struct {
	ActorID      uuid.UUID `json:"actor_id"`
	ActorType    string    `json:"actor_type"`
	IP           string    `json:"ip,omitempty"`
	UserAgent    string    `json:"user_agent,omitempty"`
	CorrelationID string   `json:"correlation_id,omitempty"`
	CausationID   string   `json:"causation_id,omitempty"`
}

// EventStore gerencia o armazenamento de eventos no Banco de dados
type EventStore struct {
	client *Client
}

// NewEventStore cria um novo Event Store
func NewEventStore(client *Client) *EventStore {
	return &EventStore{
		client: client,
	}
}

// Append adiciona um novo evento ao ledger (APPEND-ONLY)
// 🔥 CORRIGIDO: Usar QueryRow direto sem prepared statement
func (es *EventStore) Append(ctx context.Context, event *Event) error {
	query := fmt.Sprintf(`
		INSERT INTO spuri_ledger (
			event_id, aggregate_id, aggregate_type, event_type, 
			event_version, payload, metadata, occurred_at
		) VALUES (
			'%s', '%s', '%s', '%s', 
			%d, '%s', '%s', '%s'
		) 
		RETURNING id, recorded_at, ledger_hash, previous_hash
	`,
		event.EventID.String(),
		event.AggregateID.String(),
		event.AggregateType,
		event.EventType,
		event.EventVersion,
		escapeString(string(event.Payload)),
		escapeString(string(event.Metadata)),
		event.OccurredAt.Format(time.RFC3339),
	)

	row := es.client.db.QueryRow(query)

	err := row.Scan(&event.ID, &event.RecordedAt, &event.LedgerHash, &event.PreviousHash)
	if err != nil {
		return fmt.Errorf("erro ao adicionar evento ao ledger: %w", err)
	}

	return nil
}

// LoadEventStream carrega todos os eventos de um agregado
func (es *EventStore) LoadEventStream(ctx context.Context, aggregateID uuid.UUID) ([]Event, error) {
	query := fmt.Sprintf(`
		SELECT 
			id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE aggregate_id = '%s'
		ORDER BY event_version ASC, recorded_at ASC
	`, aggregateID.String())

	var events []Event
	err := es.client.db.Select(&events, query)
	if err != nil {
		return nil, fmt.Errorf("erro ao carregar event stream: %w", err)
	}

	return events, nil
}

// LoadEventStreamFromVersion carrega eventos a partir de uma versão
func (es *EventStore) LoadEventStreamFromVersion(
	ctx context.Context, 
	aggregateID uuid.UUID, 
	fromVersion int,
) ([]Event, error) {
	query := fmt.Sprintf(`
		SELECT 
			id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE aggregate_id = '%s' AND event_version >= %d
		ORDER BY event_version ASC, recorded_at ASC
	`, aggregateID.String(), fromVersion)

	var events []Event
	err := es.client.db.Select(&events, query)
	if err != nil {
		return nil, fmt.Errorf("erro ao carregar event stream: %w", err)
	}

	return events, nil
}

// GetEventsByType busca eventos por tipo
func (es *EventStore) GetEventsByType(
	ctx context.Context, 
	eventType string, 
	limit int,
) ([]Event, error) {
	query := fmt.Sprintf(`
		SELECT 
			id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE event_type = '%s'
		ORDER BY recorded_at DESC
		LIMIT %d
	`, eventType, limit)

	var events []Event
	err := es.client.db.Select(&events, query)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar eventos por tipo: %w", err)
	}

	return events, nil
}

// GetAllEvents busca todos os eventos (com paginação)
func (es *EventStore) GetAllEvents(
	ctx context.Context, 
	offset, limit int,
) ([]Event, error) {
	query := fmt.Sprintf(`
		SELECT 
			id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		ORDER BY recorded_at DESC
		LIMIT %d OFFSET %d
	`, limit, offset)

	var events []Event
	err := es.client.db.Select(&events, query)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar todos eventos: %w", err)
	}

	return events, nil
}

// GetEventByID busca um evento específico
func (es *EventStore) GetEventByID(ctx context.Context, eventID uuid.UUID) (*Event, error) {
	query := fmt.Sprintf(`
		SELECT 
			id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE event_id = '%s'
	`, eventID.String())

	var event Event
	err := es.client.db.Get(&event, query)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar evento: %w", err)
	}

	return &event, nil
}

// GetAggregateVersion retorna a versão atual de um agregado
func (es *EventStore) GetAggregateVersion(ctx context.Context, aggregateID uuid.UUID) (int, error) {
	query := fmt.Sprintf(`
		SELECT COALESCE(MAX(event_version), 0)
		FROM spuri_ledger
		WHERE aggregate_id = '%s'
	`, aggregateID.String())

	var version int
	err := es.client.db.QueryRow(query).Scan(&version)
	
	// Se não houver linhas, retornar 0 (agregado novo)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	
	if err != nil {
		return 0, fmt.Errorf("erro ao obter versão do agregado: %w", err)
	}

	return version, nil
}

// VerifyLedgerIntegrity verifica a integridade do ledger
func (es *EventStore) VerifyLedgerIntegrity(ctx context.Context, aggregateID uuid.UUID) (bool, error) {
	query := fmt.Sprintf(`
		SELECT 
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE aggregate_id = '%s'
		ORDER BY event_version ASC
	`, aggregateID.String())

	type hashPair struct {
		LedgerHash   string  `db:"ledger_hash"`
		PreviousHash *string `db:"previous_hash"`
	}

	var hashes []hashPair
	err := es.client.db.Select(&hashes, query)
	if err != nil {
		return false, fmt.Errorf("erro ao verificar integridade: %w", err)
	}

	// Verificar cadeia de hashes
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

// CountEvents retorna o total de eventos
func (es *EventStore) CountEvents(ctx context.Context) (int64, error) {
	query := `SELECT COUNT(*) FROM spuri_ledger`
	
	var count int64
	err := es.client.db.QueryRow(query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("erro ao contar eventos: %w", err)
	}

	return count, nil
}

// CountEventsByAggregate retorna total de eventos de um agregado
func (es *EventStore) CountEventsByAggregate(ctx context.Context, aggregateID uuid.UUID) (int64, error) {
	query := fmt.Sprintf(`SELECT COUNT(*) FROM spuri_ledger WHERE aggregate_id = '%s'`, aggregateID.String())
	
	var count int64
	err := es.client.db.QueryRow(query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("erro ao contar eventos: %w", err)
	}

	return count, nil
}

// 🔥 HELPER: Escapar strings para evitar SQL injection
func escapeString(s string) string {
	return EscapeString(s)
}