// ============================================================================
// ARQUIVO: internal/projections/cursos_projection.go
// ============================================================================

package projections

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"spuri/internal/db"
	"time"

	"github.com/google/uuid"
)

type CursosProjection struct {
	client *db.Client
	ctx    context.Context
}

func NewCursosProjection(client *db.Client) *CursosProjection {
	return &CursosProjection{
		client: client,
		ctx:    context.Background(),
	}
}

func (p *CursosProjection) Name() string {
	return "cursos"
}

func (p *CursosProjection) Handle(event db.Event) error {
	if event.AggregateType != "Curso" {
		return nil
	}

	switch event.EventType {
	case "CursoCriado":
		return p.handleCursoCriado(event)
	case "CursoAtivado":
		return p.handleCursoAtivado(event)
	case "CursoDesativado":
		return p.handleCursoDesativado(event)
	default:
		return nil
	}
}

func (p *CursosProjection) Rebuild() error {
	if err := p.clear(); err != nil {
		return err
	}

	query := `
		SELECT 
			id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE aggregate_type = 'Curso'
		ORDER BY id ASC
	`

	rows, err := p.client.DB().Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var event db.Event
		err := rows.Scan(
			&event.ID, &event.EventID, &event.AggregateID, &event.AggregateType,
			&event.EventType, &event.EventVersion, &event.Payload, &event.Metadata,
			&event.OccurredAt, &event.RecordedAt, &event.LedgerHash, &event.PreviousHash,
		)
		if err != nil {
			return err
		}

		if err := p.Handle(event); err != nil {
			return fmt.Errorf("erro ao processar evento %d: %w", event.ID, err)
		}
	}

	return rows.Err()
}

func (p *CursosProjection) GetLastProcessedEventID() (int64, error) {
	query := fmt.Sprintf(`
		SELECT last_processed_event_id 
		FROM projection_checkpoints 
		WHERE projection_name = '%s'
	`, p.Name())

	var lastID int64
	err := p.client.DB().QueryRow(query).Scan(&lastID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return lastID, err
}

func (p *CursosProjection) UpdateCheckpoint(eventID int64) error {
	query := fmt.Sprintf(`
		INSERT INTO projection_checkpoints (
			projection_name, last_processed_event_id, last_processed_at, events_processed
		) VALUES ('%s', %d, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) 
		DO UPDATE SET
			last_processed_event_id = %d,
			last_processed_at = CURRENT_TIMESTAMP,
			events_processed = projection_checkpoints.events_processed + 1
	`, p.Name(), eventID, eventID)

	_, err := p.client.DB().Exec(query)
	return err
}

func (p *CursosProjection) clear() error {
	_, err := p.client.DB().Exec(`TRUNCATE TABLE projection_cursos CASCADE`)
	return err
}

// Event Handlers

func (p *CursosProjection) handleCursoCriado(event db.Event) error {
	var payload struct {
		Nome           string    `json:"Nome"`
		Type           string    `json:"Type"`
		Nivel          []string  `json:"Nivel"`
		CodigoAcademia string    `json:"CodigoAcademia"`
		CreatedAt      time.Time `json:"CreatedAt"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}

	nivelJSON, _ := json.Marshal(payload.Nivel)

	query := fmt.Sprintf(`
		INSERT INTO projection_cursos (
			id, nome, type, nivel, codigo_academia, status, 
			created_at, updated_at, version, last_event_id
		) VALUES (
			'%s', '%s', '%s', '%s', '%s', 'ativo',
			'%s', CURRENT_TIMESTAMP, %d, '%s'
		)
	`,
		event.AggregateID.String(),
		escapeStringCurso(payload.Nome),
		payload.Type,
		escapeStringCurso(string(nivelJSON)),
		payload.CodigoAcademia,
		payload.CreatedAt.Format(time.RFC3339),
		event.EventVersion,
		event.EventID.String(),
	)

	_, err := p.client.DB().Exec(query)
	return err
}

func (p *CursosProjection) handleCursoAtivado(event db.Event) error {
	query := fmt.Sprintf(`
		UPDATE projection_cursos
		SET status = 'ativo', version = %d, updated_at = CURRENT_TIMESTAMP
		WHERE id = '%s'
	`, event.EventVersion, event.AggregateID.String())

	_, err := p.client.DB().Exec(query)
	return err
}

func (p *CursosProjection) handleCursoDesativado(event db.Event) error {
	query := fmt.Sprintf(`
		UPDATE projection_cursos
		SET status = 'inativo', version = %d, updated_at = CURRENT_TIMESTAMP
		WHERE id = '%s'
	`, event.EventVersion, event.AggregateID.String())

	_, err := p.client.DB().Exec(query)
	return err
}

// Query Methods

func (p *CursosProjection) GetByID(id uuid.UUID) (*CursoDTO, error) {
	query := fmt.Sprintf(`
		SELECT id, nome, type, nivel, codigo_academia, status, created_at, updated_at, version
		FROM projection_cursos WHERE id = '%s'
	`, id.String())

	var dto CursoDTO
	var nivelJSON []byte
	err := p.client.DB().QueryRow(query).Scan(
		&dto.ID, &dto.Nome, &dto.Type, &nivelJSON, &dto.CodigoAcademia,
		&dto.Status, &dto.CreatedAt, &dto.UpdatedAt, &dto.Version,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	json.Unmarshal(nivelJSON, &dto.Nivel)
	return &dto, nil
}

func (p *CursosProjection) GetByAcademia(codigoAcademia string) ([]CursoDTO, error) {
	query := fmt.Sprintf(`
		SELECT id, nome, type, nivel, codigo_academia, status, created_at, updated_at, version
		FROM projection_cursos WHERE codigo_academia = '%s' ORDER BY created_at DESC
	`, codigoAcademia)

	rows, err := p.client.DB().Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cursos []CursoDTO
	for rows.Next() {
		var dto CursoDTO
		var nivelJSON []byte
		err := rows.Scan(
			&dto.ID, &dto.Nome, &dto.Type, &nivelJSON, &dto.CodigoAcademia,
			&dto.Status, &dto.CreatedAt, &dto.UpdatedAt, &dto.Version,
		)
		if err != nil {
			return nil, err
		}
		json.Unmarshal(nivelJSON, &dto.Nivel)
		cursos = append(cursos, dto)
	}

	return cursos, rows.Err()
}

type CursoDTO struct {
	ID             uuid.UUID `json:"id"`
	Nome           string    `json:"nome"`
	Type           string    `json:"type"`
	Nivel          []string  `json:"nivel"`
	CodigoAcademia string    `json:"codigo_academia"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	Version        int       `json:"version"`
}

func escapeStringCurso(s string) string {
	result := ""
	for _, char := range s {
		if char == '\'' {
			result += "''"
		} else if char == '\\' {
			result += "\\\\"
		} else {
			result += string(char)
		}
	}
	return result
}