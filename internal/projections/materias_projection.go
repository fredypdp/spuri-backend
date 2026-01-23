package projections

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"spuri/internal/db"
	"time"

	"github.com/google/uuid"
)

type MateriasProjection struct {
	client *db.Client
}

func NewMateriasProjection(client *db.Client) *MateriasProjection {
	return &MateriasProjection{client: client}
}

func (mp *MateriasProjection) Name() string { return "materias" }

func (mp *MateriasProjection) Handle(event db.Event) error {
	if event.AggregateType != "MateriaDisciplinar" {
		return nil
	}

	switch event.EventType {
	case "MateriaCriada":
		return mp.handleMateriaCriada(event)
	case "MateriaAtivada":
		return mp.handleMateriaAtivada(event)
	case "MateriaDesativada":
		return mp.handleMateriaDesativada(event)
	case "MateriaDadosAtualizados":
		return mp.handleMateriaDadosAtualizados(event)
	}
	return nil
}

// ✅ CORRIGIDO: Query() + loop manual
func (mp *MateriasProjection) Rebuild() error {
	if err := mp.clear(); err != nil {
		return err
	}

	query := `
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger WHERE aggregate_type = 'MateriaDisciplinar' ORDER BY id ASC
	`
	
	rows, err := mp.client.DB().Query(query)
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
		if err := mp.Handle(event); err != nil {
			return fmt.Errorf("erro ao processar evento %d: %w", event.ID, err)
		}
	}
	return rows.Err()
}

func (mp *MateriasProjection) GetLastProcessedEventID() (int64, error) {
	query := `
		SELECT last_processed_event_id 
		FROM projection_checkpoints 
		WHERE projection_name = $1
	`
	
	var lastID int64
	err := mp.client.DB().QueryRow(query, mp.Name()).Scan(&lastID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return lastID, err
}

func (mp *MateriasProjection) UpdateCheckpoint(eventID int64) error {
	query := `
		INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at, events_processed)
		VALUES ($1, $2, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) DO UPDATE SET
			last_processed_event_id = $2, last_processed_at = CURRENT_TIMESTAMP,
			events_processed = projection_checkpoints.events_processed + 1
	`
	
	_, err := mp.client.DB().Exec(query, mp.Name(), eventID)
	return err
}

func (mp *MateriasProjection) clear() error {
	_, err := mp.client.DB().Exec(`TRUNCATE TABLE projection_materias CASCADE`)
	return err
}

func (mp *MateriasProjection) handleMateriaCriada(event db.Event) error {
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

	query := `
		INSERT INTO projection_materias (id, nome, type, nivel, codigo_academia, curso_id, status, created_at, updated_at, version, last_event_id)
		VALUES ($1, $2, $3, $4, $5, $6, 'ativo', $7, CURRENT_TIMESTAMP, $8, $9)
	`

	_, err := mp.client.DB().Exec(query,
		event.AggregateID, payload.Nome, payload.Type, nivelJSON, payload.CodigoAcademia, payload.CursoID,
		payload.CreatedAt, event.EventVersion, event.EventID)
	return err
}

func (mp *MateriasProjection) handleMateriaAtivada(event db.Event) error {
	query := `
		UPDATE projection_materias SET status = 'ativo', version = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2
	`
	
	_, err := mp.client.DB().Exec(query, event.EventVersion, event.AggregateID)
	return err
}

func (mp *MateriasProjection) handleMateriaDesativada(event db.Event) error {
	query := `
		UPDATE projection_materias SET status = 'inativo', version = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2
	`
	
	_, err := mp.client.DB().Exec(query, event.EventVersion, event.AggregateID)
	return err
}

func (mp *MateriasProjection) handleMateriaDadosAtualizados(event db.Event) error {
	var payload struct {
		Nome *string `json:"Nome"`
		Type *string `json:"Type"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}

	if payload.Nome != nil {
		mp.client.DB().Exec(`UPDATE projection_materias SET nome = $1 WHERE id = $2`, *payload.Nome, event.AggregateID)
	}
	if payload.Type != nil {
		mp.client.DB().Exec(`UPDATE projection_materias SET type = $1 WHERE id = $2`, *payload.Type, event.AggregateID)
	}

	updateQuery := `
		UPDATE projection_materias SET version = $1, updated_at = CURRENT_TIMESTAMP, last_event_id = $2 WHERE id = $3
	`
	
	_, err := mp.client.DB().Exec(updateQuery, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

// ✅ CORRIGIDO: QueryRow().Scan() manual
func (mp *MateriasProjection) GetByID(id uuid.UUID) (*MateriaDTO, error) {
	query := `
		SELECT id, nome, type, nivel, codigo_academia, curso_id, status, created_at, updated_at, version
		FROM projection_materias WHERE id = $1
	`
	
	var dto MateriaDTO
	var nivelJSON sql.NullString
	var cursoID sql.NullString

	err := mp.client.DB().QueryRow(query, id).Scan(
		&dto.ID, &dto.Nome, &dto.Type, &nivelJSON, &dto.CodigoAcademia, 
		&cursoID, &dto.Status, &dto.CreatedAt, &dto.UpdatedAt, &dto.Version)
	
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if nivelJSON.Valid && nivelJSON.String != "" {
		json.Unmarshal([]byte(nivelJSON.String), &dto.Nivel)
	}

	if cursoID.Valid {
		cid, _ := uuid.Parse(cursoID.String)
		dto.CursoID = &cid
	}

	return &dto, nil
}

// ✅ CORRIGIDO: Query() + loop manual
func (mp *MateriasProjection) GetByAcademia(codigoAcademia string) ([]MateriaDTO, error) {
	query := `
		SELECT id, nome, type, nivel, codigo_academia, curso_id, status, created_at, updated_at, version
		FROM projection_materias WHERE codigo_academia = $1 ORDER BY created_at DESC
	`
	
	rows, err := mp.client.DB().Query(query, codigoAcademia)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var materias []MateriaDTO
	for rows.Next() {
		var dto MateriaDTO
		var nivelJSON sql.NullString
		var cursoID sql.NullString

		err := rows.Scan(&dto.ID, &dto.Nome, &dto.Type, &nivelJSON, &dto.CodigoAcademia,
			&cursoID, &dto.Status, &dto.CreatedAt, &dto.UpdatedAt, &dto.Version)
		if err != nil {
			continue
		}

		if nivelJSON.Valid && nivelJSON.String != "" {
			json.Unmarshal([]byte(nivelJSON.String), &dto.Nivel)
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
	ID             uuid.UUID  `json:"id" db:"id"`
	Nome           string     `json:"nome" db:"nome"`
	Type           string     `json:"type" db:"type"`
	Nivel          []string   `json:"nivel,omitempty" db:"nivel"`
	CodigoAcademia string     `json:"codigo_academia" db:"codigo_academia"`
	CursoID        *uuid.UUID `json:"curso_id,omitempty" db:"curso_id"`
	Status         string     `json:"status" db:"status"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at" db:"updated_at"`
	Version        int        `json:"version" db:"version"`
}