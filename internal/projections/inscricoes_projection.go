// ============================================================================
// ARQUIVO: internal/projections/inscricoes_projection.go
// CORRIGIDO: Remover Context de todas as queries
// ============================================================================

package projections

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"spuri/internal/genesisdb"
	"time"

	"github.com/google/uuid"
)

type InscricoesProjection struct {
	client *genesisdb.Client
	ctx    context.Context
}

func NewInscricoesProjection(client *genesisdb.Client) *InscricoesProjection {
	return &InscricoesProjection{
		client: client,
		ctx:    context.Background(),
	}
}

func (p *InscricoesProjection) Name() string {
	return "inscricoes"
}

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

func (p *InscricoesProjection) Rebuild() error {
	if err := p.clear(); err != nil {
		return err
	}

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

func (p *InscricoesProjection) GetLastProcessedEventID() (int64, error) {
	query := `
		SELECT last_processed_event_id 
		FROM projection_checkpoints 
		WHERE projection_name = $1
	`

	var lastID int64
	err := p.client.DB().Get(&lastID, query, p.Name())
	return lastID, err
}

func (p *InscricoesProjection) UpdateCheckpoint(eventID int64) error {
	query := `
		UPDATE projection_checkpoints
		SET 
			last_processed_event_id = $1,
			last_processed_at = CURRENT_TIMESTAMP,
			events_processed = events_processed + 1
		WHERE projection_name = $2
	`

	_, err := p.client.DB().Exec(query, eventID, p.Name())
	return err
}

func (p *InscricoesProjection) clear() error {
	_, err := p.client.DB().Exec(`TRUNCATE TABLE projection_inscricoes CASCADE`)
	return err
}

// Event Handlers

func (p *InscricoesProjection) handleEstudanteInscrito(event genesisdb.Event) error {
	log.Printf("🔘 [INSCRICAO] Processando EstudanteInscrito")
	
	var payload struct {
		CodigoAcademia string    `json:"CodigoAcademia"`
		Tipo           string    `json:"Tipo"`
		AnoInscricao   string    `json:"AnoInscricao"`
		Curso          *string   `json:"Curso"`
		CreatedAt      time.Time `json:"CreatedAt"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		log.Printf("❌ [INSCRICAO] Erro ao parsear payload: %v", err)
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	estudanteID := event.AggregateID

	// Buscar UUID da academia usando o código
	var academiaID uuid.UUID
	queryAcademiaID := `SELECT id FROM projection_academias WHERE codigo_academia = $1`
	err := p.client.DB().Get(&academiaID, queryAcademiaID, payload.CodigoAcademia)
	if err != nil {
		log.Printf("❌ [INSCRICAO] Academia não encontrada com código: %s", payload.CodigoAcademia)
		return fmt.Errorf("academia não encontrada: %w", err)
	}

	// Buscar código do estudante
	var codigoEstudante string
	queryEstudante := `SELECT codigo_estudante FROM projection_estudantes WHERE id = $1`
	err = p.client.DB().Get(&codigoEstudante, queryEstudante, estudanteID)
	if err != nil {
		log.Printf("❌ [INSCRICAO] Estudante não encontrado: %s", estudanteID)
		return fmt.Errorf("estudante não encontrado: %w", err)
	}

	// Inserir inscrição na projeção
	query := `
		INSERT INTO projection_inscricoes (
			estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso, status, created_at, updated_at, 
			event_id, version
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`

	result, err := p.client.DB().Exec(
		query,
		estudanteID,
		codigoEstudante,
		academiaID,
		payload.CodigoAcademia,
		payload.Tipo,
		payload.AnoInscricao,
		payload.Curso,
		"espera",
		payload.CreatedAt,
		time.Now(),
		event.EventID,
		event.EventVersion,
	)

	if err != nil {
		log.Printf("❌ [INSCRICAO] Erro ao inserir: %v", err)
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	log.Printf("✅ [INSCRICAO] Inscrição criada! (rows: %d)", rowsAffected)

	// Atualizar contador de inscrições pendentes na academia
	updateQuery := `
		UPDATE projection_academias
		SET total_inscricoes_pendentes = total_inscricoes_pendentes + 1
		WHERE id = $1
	`
	p.client.DB().Exec(updateQuery, academiaID)

	// Atualizar contador de inscrições no estudante
	updateEstudanteQuery := `
		UPDATE projection_estudantes
		SET total_inscricoes = total_inscricoes + 1
		WHERE id = $1
	`
	p.client.DB().Exec(updateEstudanteQuery, estudanteID)

	return nil
}

func (p *InscricoesProjection) handleInscricaoAprovada(event genesisdb.Event) error {
	var payload struct {
		EstudanteID    uuid.UUID `json:"EstudanteID"`
		InscricaoID    uuid.UUID `json:"InscricaoID"`
		CodigoAcademia string    `json:"CodigoAcademia"`
		Tipo           string    `json:"Tipo"`
		AnoInscricao   string    `json:"AnoInscricao"`
		Curso          *string   `json:"Curso"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	estudanteID := payload.EstudanteID
	if estudanteID == uuid.Nil {
		estudanteID = event.AggregateID
	}

	// Buscar UUID da academia
	var academiaID uuid.UUID
	queryAcademiaID := `SELECT id FROM projection_academias WHERE codigo_academia = $1`
	err := p.client.DB().Get(&academiaID, queryAcademiaID, payload.CodigoAcademia)
	if err != nil {
		log.Printf("⚠️ [INSCRICAO] Academia não encontrada: %s", payload.CodigoAcademia)
		return nil
	}

	// Atualizar apenas a inscrição em 'espera'
	query := `
		UPDATE projection_inscricoes
		SET 
			status = 'aprovado',
			updated_at = CURRENT_TIMESTAMP
		WHERE estudante_id = $1 
		  AND academia_id = $2 
		  AND status = 'espera'
		  AND tipo = $3
		RETURNING id
	`

	var inscricaoID uuid.UUID
	err = p.client.DB().QueryRow(
		query,
		estudanteID,
		academiaID,
		payload.Tipo,
	).Scan(&inscricaoID)

	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("⚠️ [INSCRICAO] Nenhuma inscrição pendente para aprovar")
			return nil
		}
		return err
	}

	log.Printf("✅ [INSCRICAO] Inscrição aprovada: %s", inscricaoID)
	return nil
}

func (p *InscricoesProjection) handleInscricaoReprovada(event genesisdb.Event) error {
	var payload struct {
		EstudanteID    uuid.UUID `json:"EstudanteID"`
		InscricaoID    uuid.UUID `json:"InscricaoID"`
		CodigoAcademia string    `json:"CodigoAcademia"`
		Motivo         string    `json:"Motivo"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	estudanteID := payload.EstudanteID
	if estudanteID == uuid.Nil {
		log.Printf("⚠️ [INSCRICAO] EstudanteID não encontrado no payload!")
		return fmt.Errorf("EstudanteID ausente no payload")
	}

	// Buscar UUID da academia
	var academiaID uuid.UUID
	queryAcademiaID := `SELECT id FROM projection_academias WHERE codigo_academia = $1`
	err := p.client.DB().Get(&academiaID, queryAcademiaID, payload.CodigoAcademia)
	if err != nil {
		log.Printf("⚠️ [INSCRICAO] Academia não encontrada: %s", payload.CodigoAcademia)
		return nil
	}

	query := `
		UPDATE projection_inscricoes
		SET 
			status = 'reprovado',
			updated_at = CURRENT_TIMESTAMP
		WHERE estudante_id = $1 
		  AND academia_id = $2 
		  AND status = 'espera'
		RETURNING id
	`

	var inscricaoID uuid.UUID
	err = p.client.DB().QueryRow(
		query,
		estudanteID,
		academiaID,
	).Scan(&inscricaoID)

	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("⚠️ [INSCRICAO] Nenhuma inscrição pendente para reprovar")
			return nil
		}
		return err
	}

	log.Printf("✅ [INSCRICAO] Inscrição reprovada: %s", inscricaoID)
	return nil
}

// Query methods

func (p *InscricoesProjection) GetByEstudante(estudanteID uuid.UUID) ([]InscricaoDTO, error) {
	query := `
		SELECT 
			id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso, status, created_at, updated_at, 
			event_id, version
		FROM projection_inscricoes
		WHERE estudante_id = $1
		ORDER BY created_at DESC
	`

	var result []InscricaoDTO
	err := p.client.DB().Select(&result, query, estudanteID)
	return result, err
}

func (p *InscricoesProjection) GetByAcademia(academiaID uuid.UUID, status string) ([]InscricaoDTO, error) {
	query := `
		SELECT 
			id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso, status, created_at, updated_at, 
			event_id, version
		FROM projection_inscricoes
		WHERE academia_id = $1 AND status = $2
		ORDER BY created_at DESC
	`

	var result []InscricaoDTO
	err := p.client.DB().Select(&result, query, academiaID, status)
	return result, err
}

func (p *InscricoesProjection) GetAll(limit, offset int) ([]InscricaoDTO, error) {
	query := `
		SELECT 
			id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso, status, created_at, updated_at, 
			event_id, version
		FROM projection_inscricoes
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	var result []InscricaoDTO
	err := p.client.DB().Select(&result, query, limit, offset)
	return result, err
}

func (p *InscricoesProjection) CountAll() (int, error) {
	query := `SELECT COUNT(*) FROM projection_inscricoes`
	
	var count int
	err := p.client.DB().Get(&count, query)
	return count, err
}

func (p *InscricoesProjection) GetByID(id uuid.UUID) (*InscricaoDTO, error) {
	query := `
		SELECT 
			id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso, status, created_at, updated_at, 
			event_id, version
		FROM projection_inscricoes
		WHERE id = $1
	`

	var dto InscricaoDTO
	err := p.client.DB().Get(&dto, query, id)
	if err != nil {
		return nil, err
	}
	return &dto, nil
}

type InscricaoDTO struct {
	ID              uuid.UUID  `db:"id" json:"id"`
	EstudanteID     uuid.UUID  `db:"estudante_id" json:"estudante_id"`
	CodigoEstudante string     `db:"codigo_estudante" json:"codigo_estudante"`
	AcademiaID      uuid.UUID  `db:"academia_id" json:"academia_id"`
	CodigoAcademia  string     `db:"codigo_academia" json:"codigo_academia"`
	Tipo            string     `db:"tipo" json:"tipo"`
	AnoInscricao    string     `db:"ano_inscricao" json:"ano_inscricao"`
	Curso           *string    `db:"curso" json:"curso,omitempty"`
	Status          string     `db:"status" json:"status"`
	CreatedAt       time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt       time.Time  `db:"updated_at" json:"updated_at"`
	EventID         uuid.UUID  `db:"event_id" json:"event_id"`
	Version         int        `db:"version" json:"version"`
}