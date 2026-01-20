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

	rows, err := p.client.DB().Query(`
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
	var lastID int64
	err := p.client.DB().QueryRow(`
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
	_, err := p.client.DB().Exec(`
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
	_, err := p.client.DB().Exec(`TRUNCATE TABLE projection_inscricoes CASCADE`)
	return err
}

func (p *InscricoesProjection) handleEstudanteInscrito(event db.Event) error {
	log.Printf("📘 [INSCRICAO] Processando EstudanteInscrito - EventID: %s, AggregateID: %s",
		event.EventID.String(), event.AggregateID.String())

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
	err := p.client.DB().QueryRow(`
		SELECT codigo_estudante FROM projection_estudantes WHERE id = $1
	`, estudanteID).Scan(&codigoEstudante)
	if err != nil {
		log.Printf("❌ [INSCRICAO] Estudante não encontrado: %s - Erro: %v", estudanteID.String(), err)
		return fmt.Errorf("estudante não encontrado: %w", err)
	}

	var academiaID uuid.UUID
	err = p.client.DB().QueryRow(`
		SELECT id FROM projection_academias WHERE codigo_academia = $1
	`, payload.CodigoAcademia).Scan(&academiaID)
	if err != nil {
		log.Printf("❌ [INSCRICAO] Academia não encontrada: %s - Erro: %v", payload.CodigoAcademia, err)
		return fmt.Errorf("academia não encontrada: %w", err)
	}

	_, err = p.client.DB().Exec(`
		INSERT INTO projection_inscricoes (
			id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso, status, status_usado, created_at, updated_at, 
			event_id, version
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'espera', FALSE, $9, CURRENT_TIMESTAMP, $10, $11)
	`, payload.InscricaoID, estudanteID, codigoEstudante, academiaID, payload.CodigoAcademia,
		payload.Tipo, payload.AnoInscricao, payload.Curso, payload.CreatedAt, event.EventID, event.EventVersion)

	if err != nil {
		log.Printf("❌ [INSCRICAO] Erro ao inserir: %v", err)
		return err
	}

	p.client.DB().Exec(`UPDATE projection_academias SET total_inscricoes_pendentes = total_inscricoes_pendentes + 1 WHERE id = $1`, academiaID)
	p.client.DB().Exec(`UPDATE projection_estudantes SET total_inscricoes = total_inscricoes + 1 WHERE id = $1`, estudanteID)

	log.Printf("✅ [INSCRICAO] Processamento completo - Inscrição criada com status 'espera'")
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
	err := p.client.DB().QueryRow(`SELECT id FROM projection_academias WHERE codigo_academia = $1`, payload.CodigoAcademia).Scan(&academiaID)
	if err != nil {
		return nil
	}

	var inscricaoID uuid.UUID
	err = p.client.DB().QueryRow(`
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

	var academiaID uuid.UUID
	err := p.client.DB().QueryRow(`SELECT id FROM projection_academias WHERE codigo_academia = $1`, payload.CodigoAcademia).Scan(&academiaID)
	if err != nil {
		return nil
	}

	var inscricaoID uuid.UUID
	err = p.client.DB().QueryRow(`
		UPDATE projection_inscricoes
		SET status = 'reprovado', updated_at = CURRENT_TIMESTAMP
		WHERE estudante_id = $1 AND academia_id = $2 AND status = 'espera'
		RETURNING id
	`, estudanteID, academiaID).Scan(&inscricaoID)

	return nil
}

func (p *InscricoesProjection) handleEstudanteVinculado(event db.Event) error {
	log.Printf("🔗 [INSCRICAO] Marcando inscrição como usada")

	var payload struct {
		InscricaoID uuid.UUID `json:"InscricaoID"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	_, err := p.client.DB().Exec(`
		UPDATE projection_inscricoes SET status_usado = TRUE, updated_at = CURRENT_TIMESTAMP WHERE id = $1
	`, payload.InscricaoID)

	if err != nil {
		log.Printf("❌ [INSCRICAO] Erro ao marcar como usada: %v", err)
		return err
	}

	log.Printf("✅ [INSCRICAO] Inscrição marcada como usada!")
	return nil
}

func (p *InscricoesProjection) GetByEstudante(estudanteID uuid.UUID) ([]InscricaoDTO, error) {
	rows, err := p.client.DB().Query(`
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
	rows, err := p.client.DB().Query(`
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
	var dto InscricaoDTO
	err := p.client.DB().QueryRow(`
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