package projections

import (
	"context"
	"encoding/json"
	"fmt"
	"spuri/internal/genesisdb"
	"time"

	"github.com/google/uuid"
)

// InscricoesProjection projeção de inscrições
type InscricoesProjection struct {
	client *genesisdb.Client
	ctx    context.Context
}

// NewInscricoesProjection cria nova projeção de inscrições
func NewInscricoesProjection(client *genesisdb.Client) *InscricoesProjection {
	return &InscricoesProjection{
		client: client,
		ctx:    context.Background(),
	}
}

// Name implementa Projection
func (p *InscricoesProjection) Name() string {
	return "inscricoes"
}

// Handle processa um evento
func (p *InscricoesProjection) Handle(event genesisdb.Event) error {
	switch event.EventType {
	case "EstudanteInscrito":
		return p.handleEstudanteInscrito(event)
	case "InscricaoAprovada":
		return p.handleInscricaoAprovada(event)
	case "InscricaoReprovada":
		return p.handleInscricaoReprovada(event)
	default:
		return nil
	}
}

// Rebuild reconstrói a projeção do zero
func (p *InscricoesProjection) Rebuild() error {
	// Limpar projeção
	if err := p.clear(); err != nil {
		return err
	}

	// Buscar todos os eventos relevantes
	query := `
		SELECT 
			id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM genesis_ledger
		WHERE event_type IN ('EstudanteInscrito', 'InscricaoAprovada', 'InscricaoReprovada')
		ORDER BY id ASC
	`

	var events []genesisdb.Event
	if err := p.client.DB().Select(&events, query); err != nil {
		return err
	}

	for _, event := range events {
		if err := p.Handle(event); err != nil {
			return fmt.Errorf("erro ao processar evento %d: %w", event.ID, err)
		}
	}

	return nil
}

// GetLastProcessedEventID implementa Projection
func (p *InscricoesProjection) GetLastProcessedEventID() (int64, error) {
	query := `
		SELECT last_processed_event_id 
		FROM projection_checkpoints 
		WHERE projection_name = $1
	`

	var lastID int64
	err := p.client.DB().GetContext(p.ctx, &lastID, query, p.Name())
	return lastID, err
}

// UpdateCheckpoint implementa Projection
func (p *InscricoesProjection) UpdateCheckpoint(eventID int64) error {
	query := `
		UPDATE projection_checkpoints
		SET 
			last_processed_event_id = $1,
			last_processed_at = CURRENT_TIMESTAMP,
			events_processed = events_processed + 1
		WHERE projection_name = $2
	`

	_, err := p.client.DB().ExecContext(p.ctx, query, eventID, p.Name())
	return err
}

// clear limpa a projeção
func (p *InscricoesProjection) clear() error {
	_, err := p.client.DB().ExecContext(p.ctx, `TRUNCATE TABLE projection_inscricoes CASCADE`)
	return err
}

// Event Handlers

func (p *InscricoesProjection) handleEstudanteInscrito(event genesisdb.Event) error {
	var payload struct {
		AcademiaID   uuid.UUID `json:"AcademiaID"`
		Tipo         string    `json:"Tipo"`
		AnoInscricao string    `json:"AnoInscricao"`
		Curso        *string   `json:"Curso"`
		CreatedAt    time.Time `json:"CreatedAt"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	query := `
		INSERT INTO projection_inscricoes (
			estudante_id, academia_id, tipo, ano_inscricao,
			curso, status, created_at, updated_at, event_id, version
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`

	_, err := p.client.DB().ExecContext(
		p.ctx, query,
		event.AggregateID,
		payload.AcademiaID,
		payload.Tipo,
		payload.AnoInscricao,
		payload.Curso,
		"espera",
		payload.CreatedAt,
		time.Now(),
		event.EventID,
		event.EventVersion,
	)

	// Atualizar contador de inscrições pendentes na academia
	if err == nil {
		updateQuery := `
			UPDATE projection_academias
			SET total_inscricoes_pendentes = total_inscricoes_pendentes + 1
			WHERE id = $1
		`
		p.client.DB().ExecContext(p.ctx, updateQuery, payload.AcademiaID)
	}

	return err
}

func (p *InscricoesProjection) handleInscricaoAprovada(event genesisdb.Event) error {
	var payload struct {
		InscricaoID  uuid.UUID `json:"InscricaoID"`
		AcademiaID   uuid.UUID `json:"AcademiaID"`
		Tipo         string    `json:"Tipo"`
		AnoInscricao string    `json:"AnoInscricao"`
		Curso        *string   `json:"Curso"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	// Atualizar inscrição para aprovada
	query := `
		UPDATE projection_inscricoes
		SET 
			status = 'aprovado',
			updated_at = CURRENT_TIMESTAMP
		WHERE estudante_id = $1 AND academia_id = $2 AND status = 'espera'
	`

	_, err := p.client.DB().ExecContext(
		p.ctx, query,
		event.AggregateID,
		payload.AcademiaID,
	)

	return err
}

func (p *InscricoesProjection) handleInscricaoReprovada(event genesisdb.Event) error {
	var payload struct {
		InscricaoID uuid.UUID `json:"InscricaoID"`
		AcademiaID  uuid.UUID `json:"AcademiaID"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	// Atualizar inscrição para reprovada
	query := `
		UPDATE projection_inscricoes
		SET 
			status = 'reprovado',
			updated_at = CURRENT_TIMESTAMP
		WHERE estudante_id = $1 AND academia_id = $2 AND status = 'espera'
	`

	_, err := p.client.DB().ExecContext(
		p.ctx, query,
		event.AggregateID,
		payload.AcademiaID,
	)

	return err
}

// Query methods

// GetByEstudante busca inscrições de um estudante
func (p *InscricoesProjection) GetByEstudante(estudanteID uuid.UUID) ([]InscricaoDTO, error) {
	query := `
		SELECT 
			id, estudante_id, academia_id, tipo, ano_inscricao,
			curso, status, created_at, updated_at, event_id, version
		FROM projection_inscricoes
		WHERE estudante_id = $1
		ORDER BY created_at DESC
	`

	var result []InscricaoDTO
	err := p.client.DB().SelectContext(p.ctx, &result, query, estudanteID)
	return result, err
}

// GetByAcademia busca inscrições de uma academia
func (p *InscricoesProjection) GetByAcademia(academiaID uuid.UUID, status string) ([]InscricaoDTO, error) {
	query := `
		SELECT 
			id, estudante_id, academia_id, tipo, ano_inscricao,
			curso, status, created_at, updated_at, event_id, version
		FROM projection_inscricoes
		WHERE academia_id = $1 AND status = $2
		ORDER BY created_at DESC
	`

	var result []InscricaoDTO
	err := p.client.DB().SelectContext(p.ctx, &result, query, academiaID, status)
	return result, err
}

// GetByID busca uma inscrição específica
func (p *InscricoesProjection) GetByID(id uuid.UUID) (*InscricaoDTO, error) {
	query := `
		SELECT 
			id, estudante_id, academia_id, tipo, ano_inscricao,
			curso, status, created_at, updated_at, event_id, version
		FROM projection_inscricoes
		WHERE id = $1
	`

	var dto InscricaoDTO
	err := p.client.DB().GetContext(p.ctx, &dto, query, id)
	if err != nil {
		return nil, err
	}
	return &dto, nil
}

// InscricaoDTO DTO da projeção
type InscricaoDTO struct {
	ID           uuid.UUID  `db:"id" json:"id"`
	EstudanteID  uuid.UUID  `db:"estudante_id" json:"estudante_id"`
	AcademiaID   uuid.UUID  `db:"academia_id" json:"academia_id"`
	Tipo         string     `db:"tipo" json:"tipo"`
	AnoInscricao string     `db:"ano_inscricao" json:"ano_inscricao"`
	Curso        *string    `db:"curso" json:"curso,omitempty"`
	Status       string     `db:"status" json:"status"`
	CreatedAt    time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time  `db:"updated_at" json:"updated_at"`
	EventID      uuid.UUID  `db:"event_id" json:"event_id"`
	Version      int        `db:"version" json:"version"`
}