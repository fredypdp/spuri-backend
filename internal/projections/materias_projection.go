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
	return &MateriasProjection{client: client, ctx: context.Background()}
}

func (p *MateriasProjection) Name() string { return "materias" }

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
	case "MateriaDadosAtualizados":
		return p.handleMateriaDadosAtualizados(event)
	}
	return nil
}

func (p *MateriasProjection) Rebuild() error {
	if err := p.clear(); err != nil {
		return err
	}

	ctx := context.Background()
	rows, err := p.client.DB().QueryContext(ctx, `
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger WHERE aggregate_type = 'MateriaDisciplinar' ORDER BY id ASC
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var event db.Event
		err := rows.Scan(&event.ID, &event.EventID, &event.AggregateID, &event.AggregateType,
			&event.EventType, &event.EventVersion, &event.Payload, &event.Metadata,
			&event.OccurredAt, &event.RecordedAt, &event.LedgerHash, &event.PreviousHash)
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
	ctx := context.Background()
	var lastID int64
	err := p.client.DB().QueryRowContext(ctx, `
		SELECT last_processed_event_id 
		FROM projection_checkpoints 
		WHERE projection_name = $1
	`, p.Name()).Scan(&lastID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return lastID, err
}

func (p *MateriasProjection) UpdateCheckpoint(eventID int64) error {
	ctx := context.Background()
	_, err := p.client.DB().ExecContext(ctx, `
		INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at, events_processed)
		VALUES ($1, $2, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) DO UPDATE SET
			last_processed_event_id = $2, last_processed_at = CURRENT_TIMESTAMP,
			events_processed = projection_checkpoints.events_processed + 1
	`, p.Name(), eventID)
	return err
}

func (p *MateriasProjection) clear() error {
	ctx := context.Background()
	_, err := p.client.DB().ExecContext(ctx, `TRUNCATE TABLE projection_materias CASCADE`)
	return err
}

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

	var nivelJSON []byte
	if len(payload.Nivel) > 0 {
		nivelJSON, _ = json.Marshal(payload.Nivel)
	}

	ctx := context.Background()
	_, err := p.client.DB().ExecContext(ctx, `
		INSERT INTO projection_materias (id, nome, type, nivel, codigo_academia, curso_id, status, created_at, updated_at, version, last_event_id)
		VALUES ($1, $2, $3, $4, $5, $6, 'ativo', $7, CURRENT_TIMESTAMP, $8, $9)
	`, event.AggregateID, payload.Nome, payload.Type, nivelJSON, payload.CodigoAcademia,
		payload.CursoID, payload.CreatedAt, event.EventVersion, event.EventID)
	return err
}

func (p *MateriasProjection) handleMateriaAtivada(event db.Event) error {
	ctx := context.Background()
	_, err := p.client.DB().ExecContext(ctx, `
		UPDATE projection_materias SET status = 'ativo', version = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2
	`, event.EventVersion, event.AggregateID)
	return err
}

func (p *MateriasProjection) handleMateriaDesativada(event db.Event) error {
	ctx := context.Background()
	_, err := p.client.DB().ExecContext(ctx, `
		UPDATE projection_materias SET status = 'inativo', version = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2
	`, event.EventVersion, event.AggregateID)
	return err
}

func (p *MateriasProjection) handleMateriaDadosAtualizados(event db.Event) error {
	var payload struct {
		Nome *string `json:"Nome"`
		Type *string `json:"Type"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}

	ctx := context.Background()
	
	if payload.Nome != nil {
		p.client.DB().ExecContext(ctx, `UPDATE projection_materias SET nome = $1 WHERE id = $2`, *payload.Nome, event.AggregateID)
	}
	if payload.Type != nil {
		p.client.DB().ExecContext(ctx, `UPDATE projection_materias SET type = $1 WHERE id = $2`, *payload.Type, event.AggregateID)
	}

	_, err := p.client.DB().ExecContext(ctx, `
		UPDATE projection_materias SET version = $1, updated_at = CURRENT_TIMESTAMP, last_event_id = $2 WHERE id = $3
	`, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

func (p *MateriasProjection) GetByID(id uuid.UUID) (*MateriaDTO, error) {
	ctx := context.Background()
	var dto MateriaDTO
	var nivelJSON []byte
	var cursoID sql.NullString

	err := p.client.DB().QueryRowContext(ctx, `
		SELECT id, nome, type, nivel, codigo_academia, curso_id, status, created_at, updated_at, version
		FROM projection_materias WHERE id = $1
	`, id).Scan(&dto.ID, &dto.Nome, &dto.Type, &nivelJSON, &dto.CodigoAcademia,
		&cursoID, &dto.Status, &dto.CreatedAt, &dto.UpdatedAt, &dto.Version)

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
	ctx := context.Background()
	rows, err := p.client.DB().QueryContext(ctx, `
		SELECT id, nome, type, nivel, codigo_academia, curso_id, status, created_at, updated_at, version
		FROM projection_materias WHERE codigo_academia = $1 ORDER BY created_at DESC
	`, codigoAcademia)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var materias []MateriaDTO
	for rows.Next() {
		var dto MateriaDTO
		var nivelJSON []byte
		var cursoID sql.NullString

		err := rows.Scan(&dto.ID, &dto.Nome, &dto.Type, &nivelJSON, &dto.CodigoAcademia,
			&cursoID, &dto.Status, &dto.CreatedAt, &dto.UpdatedAt, &dto.Version)
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