package projections

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"spuri/internal/db"
	"time"

	"github.com/google/uuid"
)

type InscricoesProjection struct {
	client *db.Client
	ctx    context.Context
}

func NewInscricoesProjection(client *db.Client) *InscricoesProjection {
	return &InscricoesProjection{
		client: client,
		ctx:    context.Background(),
	}
}

func (p *InscricoesProjection) Name() string {
	return "inscricoes"
}

func (p *InscricoesProjection) Handle(event db.Event) error {
	switch event.EventType {
	case "EstudanteInscrito":
		return p.handleEstudanteInscrito(event)
	case "InscricaoAprovada":
		return p.handleInscricaoAprovada(event)
	case "InscricaoReprovada":
		return p.handleInscricaoReprovada(event)
	case "EstudanteVinculado":
		return p.handleEstudanteVinculado(event)
	default:
		return nil
	}
}

func (p *InscricoesProjection) Rebuild() error {
	if err := p.clear(); err != nil {
		return err
	}

	ctx := context.Background()
	rows, err := p.client.DB().QueryContext(ctx, `
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE event_type IN ('EstudanteInscrito', 'InscricaoAprovada', 'InscricaoReprovada', 'EstudanteVinculado')
		ORDER BY id ASC
	`)
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

func (p *InscricoesProjection) GetLastProcessedEventID() (int64, error) {
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

func (p *InscricoesProjection) UpdateCheckpoint(eventID int64) error {
	ctx := context.Background()
	_, err := p.client.DB().ExecContext(ctx, `
		INSERT INTO projection_checkpoints (
			projection_name, last_processed_event_id, last_processed_at, events_processed
		) VALUES ($1, $2, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) 
		DO UPDATE SET
			last_processed_event_id = $2,
			last_processed_at = CURRENT_TIMESTAMP,
			events_processed = projection_checkpoints.events_processed + 1
	`, p.Name(), eventID)
	return err
}

func (p *InscricoesProjection) clear() error {
	ctx := context.Background()
	_, err := p.client.DB().ExecContext(ctx, `TRUNCATE TABLE projection_inscricoes CASCADE`)
	return err
}

func (p *InscricoesProjection) handleEstudanteInscrito(event db.Event) error {
	log.Printf("[INSCRICOES_PROJECTION] Processando EstudanteInscrito (event_id: %s)", event.EventID)

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

	ctx := context.Background()
	var codigoEstudante string
	err := p.client.DB().QueryRowContext(ctx, `
		SELECT codigo_estudante FROM projection_estudantes WHERE id = $1
	`, estudanteID).Scan(&codigoEstudante)
	if err != nil {
		log.Printf("[INSCRICOES_PROJECTION] Estudante não encontrado (event_id: %s)", event.EventID)
		return fmt.Errorf("estudante não encontrado: %w", err)
	}

	var academiaID uuid.UUID
	err = p.client.DB().QueryRowContext(ctx, `
		SELECT id FROM projection_academias WHERE codigo_academia = $1
	`, payload.CodigoAcademia).Scan(&academiaID)
	if err != nil {
		log.Printf("[INSCRICOES_PROJECTION] Academia não encontrada (event_id: %s)", event.EventID)
		return fmt.Errorf("academia não encontrada: %w", err)
	}

	_, err = p.client.DB().ExecContext(ctx, `
		INSERT INTO projection_inscricoes (
			id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso, status, status_usado, created_at, updated_at, 
			event_id, version
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'espera', FALSE, $9, CURRENT_TIMESTAMP, $10, $11)
	`, payload.InscricaoID, estudanteID, codigoEstudante, academiaID, payload.CodigoAcademia,
		payload.Tipo, payload.AnoInscricao, payload.Curso, payload.CreatedAt, event.EventID, event.EventVersion)

	if err != nil {
		log.Printf("[INSCRICOES_PROJECTION] Erro ao inserir inscrição (event_id: %s)", event.EventID)
		return err
	}

	p.client.DB().ExecContext(ctx, `UPDATE projection_academias SET total_inscricoes_pendentes = total_inscricoes_pendentes + 1 WHERE id = $1`, academiaID)
	p.client.DB().ExecContext(ctx, `UPDATE projection_estudantes SET total_inscricoes = total_inscricoes + 1 WHERE id = $1`, estudanteID)

	return nil
}

func (p *InscricoesProjection) handleInscricaoAprovada(event db.Event) error {
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

	ctx := context.Background()
	var academiaID uuid.UUID
	err := p.client.DB().QueryRowContext(ctx, `SELECT id FROM projection_academias WHERE codigo_academia = $1`, payload.CodigoAcademia).Scan(&academiaID)
	if err != nil {
		return nil
	}

	var inscricaoID uuid.UUID
	err = p.client.DB().QueryRowContext(ctx, `
		UPDATE projection_inscricoes
		SET status = 'aprovado', updated_at = CURRENT_TIMESTAMP
		WHERE estudante_id = $1 AND academia_id = $2 AND status = 'espera' AND tipo = $3
		RETURNING id
	`, estudanteID, academiaID, payload.Tipo).Scan(&inscricaoID)

	return nil
}

func (p *InscricoesProjection) handleInscricaoReprovada(event db.Event) error {
	var payload struct {
		EstudanteID    uuid.UUID `json:"EstudanteID"`
		CodigoAcademia string    `json:"CodigoAcademia"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	estudanteID := payload.EstudanteID
	if estudanteID == uuid.Nil {
		return fmt.Errorf("EstudanteID ausente no payload")
	}

	ctx := context.Background()
	var academiaID uuid.UUID
	err := p.client.DB().QueryRowContext(ctx, `SELECT id FROM projection_academias WHERE codigo_academia = $1`, payload.CodigoAcademia).Scan(&academiaID)
	if err != nil {
		return nil
	}

	var inscricaoID uuid.UUID
	err = p.client.DB().QueryRowContext(ctx, `
		UPDATE projection_inscricoes
		SET status = 'reprovado', updated_at = CURRENT_TIMESTAMP
		WHERE estudante_id = $1 AND academia_id = $2 AND status = 'espera'
		RETURNING id
	`, estudanteID, academiaID).Scan(&inscricaoID)

	return nil
}

func (p *InscricoesProjection) handleEstudanteVinculado(event db.Event) error {
	var payload struct {
		InscricaoID uuid.UUID `json:"InscricaoID"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	ctx := context.Background()
	_, err := p.client.DB().ExecContext(ctx, `
		UPDATE projection_inscricoes SET status_usado = TRUE, updated_at = CURRENT_TIMESTAMP WHERE id = $1
	`, payload.InscricaoID)

	if err != nil {
		log.Printf("[INSCRICOES_PROJECTION] Erro ao marcar inscrição (event_id: %s)", event.EventID)
		return err
	}

	return nil
}

func (p *InscricoesProjection) GetByEstudante(estudanteID uuid.UUID) ([]InscricaoDTO, error) {
	ctx := context.Background()
	rows, err := p.client.DB().QueryContext(ctx, `
		SELECT id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso, status, status_usado, created_at, updated_at, 
			event_id, version
		FROM projection_inscricoes WHERE estudante_id = $1 ORDER BY created_at DESC
	`, estudanteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []InscricaoDTO
	for rows.Next() {
		var dto InscricaoDTO
		err := rows.Scan(
			&dto.ID, &dto.EstudanteID, &dto.CodigoEstudante,
			&dto.AcademiaID, &dto.CodigoAcademia, &dto.Tipo,
			&dto.AnoInscricao, &dto.Curso, &dto.Status, &dto.StatusUsado,
			&dto.CreatedAt, &dto.UpdatedAt, &dto.EventID, &dto.Version,
		)
		if err != nil {
			return nil, err
		}
		result = append(result, dto)
	}

	return result, rows.Err()
}

func (p *InscricoesProjection) GetByAcademia(academiaID uuid.UUID, status string) ([]InscricaoDTO, error) {
	ctx := context.Background()
	rows, err := p.client.DB().QueryContext(ctx, `
		SELECT id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso, status, status_usado, created_at, updated_at, 
			event_id, version
		FROM projection_inscricoes WHERE academia_id = $1 AND status = $2 ORDER BY created_at DESC
	`, academiaID, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []InscricaoDTO
	for rows.Next() {
		var dto InscricaoDTO
		err := rows.Scan(
			&dto.ID, &dto.EstudanteID, &dto.CodigoEstudante,
			&dto.AcademiaID, &dto.CodigoAcademia, &dto.Tipo,
			&dto.AnoInscricao, &dto.Curso, &dto.Status, &dto.StatusUsado,
			&dto.CreatedAt, &dto.UpdatedAt, &dto.EventID, &dto.Version,
		)
		if err != nil {
			return nil, err
		}
		result = append(result, dto)
	}

	return result, rows.Err()
}

func (p *InscricoesProjection) GetByID(id uuid.UUID) (*InscricaoDTO, error) {
	ctx := context.Background()
	var dto InscricaoDTO
	err := p.client.DB().QueryRowContext(ctx, `
		SELECT id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso, status, status_usado, created_at, updated_at, 
			event_id, version
		FROM projection_inscricoes WHERE id = $1
	`, id).Scan(
		&dto.ID, &dto.EstudanteID, &dto.CodigoEstudante,
		&dto.AcademiaID, &dto.CodigoAcademia, &dto.Tipo,
		&dto.AnoInscricao, &dto.Curso, &dto.Status, &dto.StatusUsado,
		&dto.CreatedAt, &dto.UpdatedAt, &dto.EventID, &dto.Version,
	)
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