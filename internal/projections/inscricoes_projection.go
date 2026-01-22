package projections

import (
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
}

func NewInscricoesProjection(client *db.Client) *InscricoesProjection {
	return &InscricoesProjection{client: client}
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

	query := `
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE event_type IN ('EstudanteInscrito', 'InscricaoAprovada', 'InscricaoReprovada', 'EstudanteVinculado')
		ORDER BY id ASC
	`
	
	rows, err := p.client.DB().Queryx(query)
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
	var lastID int64
	query := `
		SELECT last_processed_event_id 
		FROM projection_checkpoints 
		WHERE projection_name = $1
	`
	
	err := p.client.DB().Get(&lastID, query, p.Name())
	
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return lastID, err
}

func (p *InscricoesProjection) UpdateCheckpoint(eventID int64) error {
	query := `
		INSERT INTO projection_checkpoints (
			projection_name, last_processed_event_id, last_processed_at, events_processed
		) VALUES ($1, $2, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) 
		DO UPDATE SET
			last_processed_event_id = $2,
			last_processed_at = CURRENT_TIMESTAMP,
			events_processed = projection_checkpoints.events_processed + 1
	`
	_, err := p.client.DB().Exec(query, p.Name(), eventID)
	return err
}

func (p *InscricoesProjection) clear() error {
	_, err := p.client.DB().Exec(`TRUNCATE TABLE projection_inscricoes CASCADE`)
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

	var codigoEstudante string
	err := p.client.DB().Get(&codigoEstudante, `
		SELECT codigo_estudante FROM projection_estudantes WHERE id = $1
	`, estudanteID)
	
	if err != nil {
		log.Printf("[INSCRICOES_PROJECTION] Estudante não encontrado (event_id: %s)", event.EventID)
		return fmt.Errorf("estudante não encontrado: %w", err)
	}

	var academiaID uuid.UUID
	err = p.client.DB().Get(&academiaID, `
		SELECT id FROM projection_academias WHERE codigo_academia = $1
	`, payload.CodigoAcademia)
	
	if err != nil {
		log.Printf("[INSCRICOES_PROJECTION] Academia não encontrada (event_id: %s)", event.EventID)
		return fmt.Errorf("academia não encontrada: %w", err)
	}

	query := `
		INSERT INTO projection_inscricoes (
			id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso, status, status_usado, created_at, updated_at, 
			event_id, version
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'espera', FALSE, $9, CURRENT_TIMESTAMP, $10, $11)
	`
	
	_, err = p.client.DB().Exec(query,
		payload.InscricaoID, estudanteID, codigoEstudante, academiaID, payload.CodigoAcademia,
		payload.Tipo, payload.AnoInscricao, payload.Curso, payload.CreatedAt, event.EventID, event.EventVersion)

	if err != nil {
		log.Printf("[INSCRICOES_PROJECTION] Erro ao inserir inscrição (event_id: %s)", event.EventID)
		return err
	}

	p.client.DB().Exec(`UPDATE projection_academias SET total_inscricoes_pendentes = total_inscricoes_pendentes + 1 WHERE id = $1`, academiaID)
	p.client.DB().Exec(`UPDATE projection_estudantes SET total_inscricoes = total_inscricoes + 1 WHERE id = $1`, estudanteID)

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

	var academiaID uuid.UUID
	err := p.client.DB().Get(&academiaID, `SELECT id FROM projection_academias WHERE codigo_academia = $1`, payload.CodigoAcademia)
	if err != nil {
		return nil
	}

	var inscricaoID uuid.UUID
	query := `
		UPDATE projection_inscricoes
		SET status = 'aprovado', updated_at = CURRENT_TIMESTAMP
		WHERE estudante_id = $1 AND academia_id = $2 AND status = 'espera' AND tipo = $3
		RETURNING id
	`
	
	err = p.client.DB().Get(&inscricaoID, query, estudanteID, academiaID, payload.Tipo)

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

	var academiaID uuid.UUID
	err := p.client.DB().Get(&academiaID, `SELECT id FROM projection_academias WHERE codigo_academia = $1`, payload.CodigoAcademia)
	if err != nil {
		return nil
	}

	var inscricaoID uuid.UUID
	query := `
		UPDATE projection_inscricoes
		SET status = 'reprovado', updated_at = CURRENT_TIMESTAMP
		WHERE estudante_id = $1 AND academia_id = $2 AND status = 'espera'
		RETURNING id
	`
	
	err = p.client.DB().Get(&inscricaoID, query, estudanteID, academiaID)

	return nil
}

func (p *InscricoesProjection) handleEstudanteVinculado(event db.Event) error {
	var payload struct {
		InscricaoID uuid.UUID `json:"InscricaoID"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	query := `
		UPDATE projection_inscricoes SET status_usado = TRUE, updated_at = CURRENT_TIMESTAMP WHERE id = $1
	`
	
	_, err := p.client.DB().Exec(query, payload.InscricaoID)

	if err != nil {
		log.Printf("[INSCRICOES_PROJECTION] Erro ao marcar inscrição (event_id: %s)", event.EventID)
		return err
	}

	return nil
}

func (p *InscricoesProjection) GetByEstudante(estudanteID uuid.UUID) ([]InscricaoDTO, error) {
	var result []InscricaoDTO
	query := `
		SELECT id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso, status, status_usado, created_at, updated_at, 
			event_id, version
		FROM projection_inscricoes WHERE estudante_id = $1 ORDER BY created_at DESC
	`
	
	err := p.client.DB().Select(&result, query, estudanteID)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (p *InscricoesProjection) GetByAcademia(academiaID uuid.UUID, status string) ([]InscricaoDTO, error) {
	var result []InscricaoDTO
	query := `
		SELECT id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso, status, status_usado, created_at, updated_at, 
			event_id, version
		FROM projection_inscricoes WHERE academia_id = $1 AND status = $2 ORDER BY created_at DESC
	`
	
	err := p.client.DB().Select(&result, query, academiaID, status)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (p *InscricoesProjection) GetByID(id uuid.UUID) (*InscricaoDTO, error) {
	var dto InscricaoDTO
	query := `
		SELECT id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso, status, status_usado, created_at, updated_at, 
			event_id, version
		FROM projection_inscricoes WHERE id = $1
	`
	
	err := p.client.DB().Get(&dto, query, id)
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