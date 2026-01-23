package projections

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"spuri/internal/db"
	"time"

	"github.com/google/uuid"
)

type InscricoesProjection struct {
	client *db.Client
}

func NewInscricoesProjection(client *db.Client) *InscricoesProjection {
	return &InscricoesProjection{client: client}
}

func (ip *InscricoesProjection) Name() string { return "inscricoes" }

func (ip *InscricoesProjection) Handle(event db.Event) error {
	switch event.EventType {
	case "EstudanteInscrito":
		return ip.handleEstudanteInscrito(event)
	case "InscricaoAprovada":
		return ip.handleInscricaoAprovada(event)
	case "InscricaoReprovada":
		return ip.handleInscricaoReprovada(event)
	case "EstudanteVinculado":
		return ip.handleEstudanteVinculado(event)
	}
	return nil
}

// ✅ CORRIGIDO: Query() + loop manual
func (ip *InscricoesProjection) Rebuild() error {
	if err := ip.clear(); err != nil {
		return err
	}

	query := `
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE event_type IN ('EstudanteInscrito', 'InscricaoAprovada', 'InscricaoReprovada', 'EstudanteVinculado')
		ORDER BY id ASC
	`
	
	rows, err := ip.client.DB().Query(query)
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
		if err := ip.Handle(event); err != nil {
			return fmt.Errorf("erro ao processar evento %d: %w", event.ID, err)
		}
	}
	return rows.Err()
}

func (ip *InscricoesProjection) GetLastProcessedEventID() (int64, error) {
	query := `
		SELECT last_processed_event_id 
		FROM projection_checkpoints 
		WHERE projection_name = $1
	`
	
	var lastID int64
	err := ip.client.DB().QueryRow(query, ip.Name()).Scan(&lastID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return lastID, err
}

func (ip *InscricoesProjection) UpdateCheckpoint(eventID int64) error {
	query := `
		INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at, events_processed)
		VALUES ($1, $2, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) DO UPDATE SET
			last_processed_event_id = $2, last_processed_at = CURRENT_TIMESTAMP,
			events_processed = projection_checkpoints.events_processed + 1
	`
	
	_, err := ip.client.DB().Exec(query, ip.Name(), eventID)
	return err
}

func (ip *InscricoesProjection) clear() error {
	_, err := ip.client.DB().Exec(`TRUNCATE TABLE projection_inscricoes CASCADE`)
	return err
}

func (ip *InscricoesProjection) handleEstudanteInscrito(event db.Event) error {
	var payload struct {
		InscricaoID    uuid.UUID `json:"InscricaoID"`
		CodigoAcademia string    `json:"CodigoAcademia"`
		Tipo           string    `json:"Tipo"`
		AnoInscricao   string    `json:"AnoInscricao"`
		Curso          *string   `json:"Curso"`
		CreatedAt      time.Time `json:"CreatedAt"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	estudanteID := event.AggregateID

	// Buscar código do estudante
	var codigoEstudante string
	err := ip.client.DB().QueryRow(`SELECT codigo_estudante FROM projection_estudantes WHERE id = $1`, estudanteID).Scan(&codigoEstudante)
	if err != nil {
		return fmt.Errorf("estudante não encontrado: %w", err)
	}

	// Buscar ID da academia
	var academiaID uuid.UUID
	err = ip.client.DB().QueryRow(`SELECT id FROM projection_academias WHERE codigo_academia = $1`, payload.CodigoAcademia).Scan(&academiaID)
	if err != nil {
		return fmt.Errorf("academia não encontrada: %w", err)
	}

	query := `
		INSERT INTO projection_inscricoes (
			id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso, status, status_usado, created_at, updated_at, 
			event_id, version
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'espera', FALSE, $9, CURRENT_TIMESTAMP, $10, $11)
	`

	_, err = ip.client.DB().Exec(query, 
		payload.InscricaoID, estudanteID, codigoEstudante, academiaID, payload.CodigoAcademia,
		payload.Tipo, payload.AnoInscricao, payload.Curso, payload.CreatedAt, 
		event.EventID, event.EventVersion)

	if err != nil {
		return err
	}

	ip.client.DB().Exec(`UPDATE projection_academias SET total_inscricoes_pendentes = total_inscricoes_pendentes + 1 WHERE id = $1`, academiaID)
	ip.client.DB().Exec(`UPDATE projection_estudantes SET total_inscricoes = total_inscricoes + 1 WHERE id = $1`, estudanteID)

	return nil
}

func (ip *InscricoesProjection) handleInscricaoAprovada(event db.Event) error {
	var payload struct {
		EstudanteID    uuid.UUID `json:"EstudanteID"`
		InscricaoID    uuid.UUID `json:"InscricaoID"`
		CodigoAcademia string    `json:"CodigoAcademia"`
		Tipo           string    `json:"Tipo"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	estudanteID := payload.EstudanteID
	if estudanteID == uuid.Nil {
		estudanteID = event.AggregateID
	}

	var academiaID uuid.UUID
	err := ip.client.DB().QueryRow(`SELECT id FROM projection_academias WHERE codigo_academia = $1`, payload.CodigoAcademia).Scan(&academiaID)
	if err != nil {
		return nil
	}

	query := `
		UPDATE projection_inscricoes
		SET status = 'aprovado', updated_at = CURRENT_TIMESTAMP
		WHERE estudante_id = $1 AND academia_id = $2 AND status = 'espera' AND tipo = $3
	`

	ip.client.DB().Exec(query, estudanteID, academiaID, payload.Tipo)
	return nil
}

func (ip *InscricoesProjection) handleInscricaoReprovada(event db.Event) error {
	var payload struct {
		EstudanteID    uuid.UUID `json:"EstudanteID"`
		CodigoAcademia string    `json:"CodigoAcademia"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	var academiaID uuid.UUID
	err := ip.client.DB().QueryRow(`SELECT id FROM projection_academias WHERE codigo_academia = $1`, payload.CodigoAcademia).Scan(&academiaID)
	if err != nil {
		return nil
	}

	query := `
		UPDATE projection_inscricoes
		SET status = 'reprovado', updated_at = CURRENT_TIMESTAMP
		WHERE estudante_id = $1 AND academia_id = $2 AND status = 'espera'
	`

	ip.client.DB().Exec(query, payload.EstudanteID, academiaID)
	return nil
}

func (ip *InscricoesProjection) handleEstudanteVinculado(event db.Event) error {
	var payload struct {
		InscricaoID uuid.UUID `json:"InscricaoID"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	query := `
		UPDATE projection_inscricoes SET status_usado = TRUE, updated_at = CURRENT_TIMESTAMP 
		WHERE id = $1
	`

	_, err := ip.client.DB().Exec(query, payload.InscricaoID)
	return err
}

// ✅ CORRIGIDO: Query() + loop manual
func (ip *InscricoesProjection) GetByEstudante(estudanteID uuid.UUID) ([]InscricaoDTO, error) {
	query := `
		SELECT id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso, status, status_usado, created_at, updated_at, 
			event_id, version
		FROM projection_inscricoes WHERE estudante_id = $1 ORDER BY created_at DESC
	`

	rows, err := ip.client.DB().Query(query, estudanteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []InscricaoDTO
	for rows.Next() {
		var dto InscricaoDTO
		err := rows.Scan(&dto.ID, &dto.EstudanteID, &dto.CodigoEstudante, &dto.AcademiaID,
			&dto.CodigoAcademia, &dto.Tipo, &dto.AnoInscricao, &dto.Curso, &dto.Status,
			&dto.StatusUsado, &dto.CreatedAt, &dto.UpdatedAt, &dto.EventID, &dto.Version)
		if err != nil {
			continue
		}
		result = append(result, dto)
	}
	return result, rows.Err()
}

// ✅ CORRIGIDO: Query() + loop manual
func (ip *InscricoesProjection) GetByAcademia(academiaID uuid.UUID, status string) ([]InscricaoDTO, error) {
	query := `
		SELECT id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso, status, status_usado, created_at, updated_at, 
			event_id, version
		FROM projection_inscricoes WHERE academia_id = $1 AND status = $2 ORDER BY created_at DESC
	`

	rows, err := ip.client.DB().Query(query, academiaID, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []InscricaoDTO
	for rows.Next() {
		var dto InscricaoDTO
		err := rows.Scan(&dto.ID, &dto.EstudanteID, &dto.CodigoEstudante, &dto.AcademiaID,
			&dto.CodigoAcademia, &dto.Tipo, &dto.AnoInscricao, &dto.Curso, &dto.Status,
			&dto.StatusUsado, &dto.CreatedAt, &dto.UpdatedAt, &dto.EventID, &dto.Version)
		if err != nil {
			continue
		}
		result = append(result, dto)
	}
	return result, rows.Err()
}

// ✅ CORRIGIDO: QueryRow().Scan() manual
func (ip *InscricoesProjection) GetByID(id uuid.UUID) (*InscricaoDTO, error) {
	query := `
		SELECT id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso, status, status_usado, created_at, updated_at, 
			event_id, version
		FROM projection_inscricoes WHERE id = $1
	`

	var dto InscricaoDTO
	err := ip.client.DB().QueryRow(query, id).Scan(
		&dto.ID, &dto.EstudanteID, &dto.CodigoEstudante, &dto.AcademiaID,
		&dto.CodigoAcademia, &dto.Tipo, &dto.AnoInscricao, &dto.Curso, &dto.Status,
		&dto.StatusUsado, &dto.CreatedAt, &dto.UpdatedAt, &dto.EventID, &dto.Version,
	)
	
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &dto, nil
}

type InscricaoDTO struct {
	ID              uuid.UUID `db:"id" json:"id"`
	EstudanteID     uuid.UUID `db:"estudante_id" json:"estudante_id"`
	CodigoEstudante string    `db:"codigo_estudante" json:"codigo_estudante"`
	AcademiaID      uuid.UUID `db:"academia_id" json:"academia_id"`
	CodigoAcademia  string    `db:"codigo_academia" json:"codigo_academia"`
	Tipo            string    `db:"tipo" json:"tipo"`
	AnoInscricao    string    `db:"ano_inscricao" json:"ano_inscricao"`
	Curso           *string   `db:"curso" json:"curso,omitempty"`
	Status          string    `db:"status" json:"status"`
	StatusUsado     bool      `db:"status_usado" json:"status_usado"`
	CreatedAt       time.Time `db:"created_at" json:"created_at"`
	UpdatedAt       time.Time `db:"updated_at" json:"updated_at"`
	EventID         uuid.UUID `db:"event_id" json:"event_id"`
	Version         int       `db:"version" json:"version"`
}