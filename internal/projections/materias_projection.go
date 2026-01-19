// ============================================================================
// ARQUIVO: internal/projections/materias_projection.go
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

type MateriasProjection struct {
	client *db.Client
	ctx    context.Context
}

func NewMateriasProjection(client *db.Client) *MateriasProjection {
	return &MateriasProjection{
		client: client,
		ctx:    context.Background(),
	}
}

func (p *MateriasProjection) Name() string {
	return "materias"
}

func (p *MateriasProjection) Handle(event db.Event) error {
	if event.AggregateType != "MateriaDisciplinar" {
		return nil
	}

	switch event.EventType {
	case "MateriaCriada":
		return p.handleMateriaCriada(event)
	case "MateriaAtivada":
		return p.handleMateriaAtivada(event)
	case "MateriaDesativada":
		return p.handleMateriaDesativada(event)
	default:
		return nil
	}
}

func (p *MateriasProjection) Rebuild() error {
	if err := p.clear(); err != nil {
		return err
	}

	query := `
		SELECT 
			id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE aggregate_type = 'MateriaDisciplinar'
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

func (p *MateriasProjection) GetLastProcessedEventID() (int64, error) {
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

func (p *MateriasProjection) UpdateCheckpoint(eventID int64) error {
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

func (p *MateriasProjection) clear() error {
	_, err := p.client.DB().Exec(`TRUNCATE TABLE projection_materias CASCADE`)
	return err
}

// Event Handlers

func (p *MateriasProjection) handleMateriaCriada(event db.Event) error {
	var payload struct {
		Nome           string     `json:"Nome"`
		Type           string     `json:"Type"`
		Nivel          []string   `json:"Nivel"`
		CodigoAcademia string     `json:"CodigoAcademia"`
		CursoID        *uuid.UUID `json:"CursoID"`
		CreatedAt      time.Time  `json:"CreatedAt"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}

	nivelJSON := "NULL"
	if len(payload.Nivel) > 0 {
		nj, _ := json.Marshal(payload.Nivel)
		nivelJSON = fmt.Sprintf("'%s'", escapeStringMateria(string(nj)))
	}

	cursoIDStr := "NULL"
	if payload.CursoID != nil {
		cursoIDStr = fmt.Sprintf("'%s'", payload.CursoID.String())
	}

	query := fmt.Sprintf(`
		INSERT INTO projection_materias (
			id, nome, type, nivel, codigo_academia, curso_id, status,
			created_at, updated_at, version, last_event_id
		) VALUES (
			'%s', '%s', '%s', %s, '%s', %s, 'ativo',
			'%s', CURRENT_TIMESTAMP, %d, '%s'
		)
	`,
		event.AggregateID.String(),
		escapeStringMateria(payload.Nome),
		payload.Type,
		nivelJSON,
		payload.CodigoAcademia,
		cursoIDStr,
		payload.CreatedAt.Format(time.RFC3339),
		event.EventVersion,
		event.EventID.String(),
	)

	_, err := p.client.DB().Exec(query)
	return err
}

func (p *MateriasProjection) handleMateriaAtivada(event db.Event) error {
	query := fmt.Sprintf(`
		UPDATE projection_materias
		SET status = 'ativo', version = %d, updated_at = CURRENT_TIMESTAMP
		WHERE id = '%s'
	`, event.EventVersion, event.AggregateID.String())

	_, err := p.client.DB().Exec(query)
	return err
}

func (p *MateriasProjection) handleMateriaDesativada(event db.Event) error {
	query := fmt.Sprintf(`
		UPDATE projection_materias
		SET status = 'inativo', version = %d, updated_at = CURRENT_TIMESTAMP
		WHERE id = '%s'
	`, event.EventVersion, event.AggregateID.String())

	_, err := p.client.DB().Exec(query)
	return err
}

// Query Methods

func (p *MateriasProjection) GetByID(id uuid.UUID) (*MateriaDTO, error) {
	query := fmt.Sprintf(`
		SELECT id, nome, type, nivel, codigo_academia, curso_id, status, created_at, updated_at, version
		FROM projection_materias WHERE id = '%s'
	`, id.String())

	var dto MateriaDTO
	var nivelJSON []byte
	var cursoID sql.NullString
	
	err := p.client.DB().QueryRow(query).Scan(
		&dto.ID, &dto.Nome, &dto.Type, &nivelJSON, &dto.CodigoAcademia,
		&cursoID, &dto.Status, &dto.CreatedAt, &dto.UpdatedAt, &dto.Version,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if len(nivelJSON) > 0 {
		json.Unmarshal(nivelJSON, &dto.Nivel)
	}
	
	if cursoID.Valid {
		cid, _ := uuid.Parse(cursoID.String)
		dto.CursoID = &cid
	}

	return &dto, nil
}

func (p *MateriasProjection) GetByAcademia(codigoAcademia string) ([]MateriaDTO, error) {
	query := fmt.Sprintf(`
		SELECT id, nome, type, nivel, codigo_academia, curso_id, status, created_at, updated_at, version
		FROM projection_materias WHERE codigo_academia = '%s' ORDER BY created_at DESC
	`, codigoAcademia)

	rows, err := p.client.DB().Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var materias []MateriaDTO
	for rows.Next() {
		var dto MateriaDTO
		var nivelJSON []byte
		var cursoID sql.NullString
		
		err := rows.Scan(
			&dto.ID, &dto.Nome, &dto.Type, &nivelJSON, &dto.CodigoAcademia,
			&cursoID, &dto.Status, &dto.CreatedAt, &dto.UpdatedAt, &dto.Version,
		)
		if err != nil {
			return nil, err
		}

		if len(nivelJSON) > 0 {
			json.Unmarshal(nivelJSON, &dto.Nivel)
		}
		
		if cursoID.Valid {
			cid, _ := uuid.Parse(cursoID.String)
			dto.CursoID = &cid
		}

		materias = append(materias, dto)
	}

	return materias, rows.Err()
}

type MateriaDTO struct {
	ID             uuid.UUID  `json:"id"`
	Nome           string     `json:"nome"`
	Type           string     `json:"type"`
	Nivel          []string   `json:"nivel,omitempty"`
	CodigoAcademia string     `json:"codigo_academia"`
	CursoID        *uuid.UUID `json:"curso_id,omitempty"`
	Status         string     `json:"status"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	Version        int        `json:"version"`
}

func escapeStringMateria(s string) string {
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