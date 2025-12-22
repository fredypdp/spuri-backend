package store

import (
	"database/sql"
	"fmt"
	"spuri/internal/domain"

	"github.com/google/uuid"
)

// SaveEvent salva um evento no Event Store
// Esta é a função central do Event Sourcing - NUNCA deleta ou atualiza eventos
func SaveEvent(event *domain.Event) error {
	query := `
		INSERT INTO event_store (
			event_id, aggregate_id, aggregate_type, event_type,
			payload, metadata, occurred_at, version
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id
	`

	err := DB.QueryRow(
		query,
		event.EventID,
		event.AggregateID,
		event.AggregateType,
		event.EventType,
		event.Payload,
		event.Metadata,
		event.OccurredAt,
		event.Version,
	).Scan(&event.ID)

	if err != nil {
		return fmt.Errorf("erro ao salvar evento: %w", err)
	}

	return nil
}

// GetEventsByAggregate obtém todos os eventos de um agregado
// Usado para reconstruir o estado completo de uma entidade
func GetEventsByAggregate(aggregateID uuid.UUID) ([]domain.Event, error) {
	query := `
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
		       payload, metadata, occurred_at, version
		FROM event_store
		WHERE aggregate_id = $1
		ORDER BY version ASC, occurred_at ASC
	`

	var events []domain.Event
	err := DB.Select(&events, query, aggregateID)
	if err != nil {
		if err == sql.ErrNoRows {
			return []domain.Event{}, nil
		}
		return nil, fmt.Errorf("erro ao buscar eventos: %w", err)
	}

	return events, nil
}

// GetEventsByType obtém eventos por tipo
// Útil para processar todos os eventos de um tipo específico
func GetEventsByType(eventType string, limit int) ([]domain.Event, error) {
	query := `
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
		       payload, metadata, occurred_at, version
		FROM event_store
		WHERE event_type = $1
		ORDER BY occurred_at DESC
		LIMIT $2
	`

	var events []domain.Event
	err := DB.Select(&events, query, eventType, limit)
	if err != nil {
		if err == sql.ErrNoRows {
			return []domain.Event{}, nil
		}
		return nil, fmt.Errorf("erro ao buscar eventos por tipo: %w", err)
	}

	return events, nil
}

// GetAllEvents obtém todos os eventos (com paginação)
// Útil para auditoria completa
func GetAllEvents(offset, limit int) ([]domain.Event, error) {
	query := `
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
		       payload, metadata, occurred_at, version
		FROM event_store
		ORDER BY occurred_at DESC
		LIMIT $1 OFFSET $2
	`

	var events []domain.Event
	err := DB.Select(&events, query, limit, offset)
	if err != nil {
		if err == sql.ErrNoRows {
			return []domain.Event{}, nil
		}
		return nil, fmt.Errorf("erro ao buscar todos os eventos: %w", err)
	}

	return events, nil
}

// SaveNotasWithEvent salva notas E cria o evento (transação atômica)
func SaveNotasWithEvent(notas *domain.RegistroNotas, event *domain.Event) error {
	tx, err := DB.Beginx()
	if err != nil {
		return fmt.Errorf("erro ao iniciar transação: %w", err)
	}
	defer tx.Rollback()

	// 1. Salvar o evento
	var eventID int64
	eventQuery := `
		INSERT INTO event_store (
			event_id, aggregate_id, aggregate_type, event_type,
			payload, metadata, occurred_at, version
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id
	`

	err = tx.QueryRow(
		eventQuery,
		event.EventID,
		event.AggregateID,
		event.AggregateType,
		event.EventType,
		event.Payload,
		event.Metadata,
		event.OccurredAt,
		event.Version,
	).Scan(&eventID)

	if err != nil {
		return fmt.Errorf("erro ao salvar evento: %w", err)
	}

	// 2. Salvar a projeção (read model)
	notasQuery := `
		INSERT INTO registro_notas (
			estudante_id, id_academia, ano_lectivo, periodo, materias, event_id
		) VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`

	err = tx.QueryRow(
		notasQuery,
		notas.EstudanteID,
		notas.IDAcademia,
		notas.AnoLectivo,
		notas.Periodo,
		notas.Materias,
		event.EventID,
	).Scan(&notas.ID)

	if err != nil {
		return fmt.Errorf("erro ao salvar notas: %w", err)
	}

	return tx.Commit()
}

// SaveFaltasWithEvent salva faltas E cria o evento (transação atômica)
func SaveFaltasWithEvent(faltas *domain.RegistroFaltas, event *domain.Event) error {
	tx, err := DB.Beginx()
	if err != nil {
		return fmt.Errorf("erro ao iniciar transação: %w", err)
	}
	defer tx.Rollback()

	// 1. Salvar o evento
	var eventID int64
	eventQuery := `
		INSERT INTO event_store (
			event_id, aggregate_id, aggregate_type, event_type,
			payload, metadata, occurred_at, version
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id
	`

	err = tx.QueryRow(
		eventQuery,
		event.EventID,
		event.AggregateID,
		event.AggregateType,
		event.EventType,
		event.Payload,
		event.Metadata,
		event.OccurredAt,
		event.Version,
	).Scan(&eventID)

	if err != nil {
		return fmt.Errorf("erro ao salvar evento: %w", err)
	}

	// 2. Salvar a projeção (read model)
	faltasQuery := `
		INSERT INTO registro_faltas (
			estudante_id, id_academia, ano_lectivo, periodo, materias, event_id
		) VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`

	err = tx.QueryRow(
		faltasQuery,
		faltas.EstudanteID,
		faltas.IDAcademia,
		faltas.AnoLectivo,
		faltas.Periodo,
		faltas.Materias,
		event.EventID,
	).Scan(&faltas.ID)

	if err != nil {
		return fmt.Errorf("erro ao salvar faltas: %w", err)
	}

	return tx.Commit()
}