// ============================================================================
// ARQUIVO: internal/projections/inscricoes_projection.go
// CORRIGIDO: Ler CodigoAcademia do payload e buscar UUID da academia
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

// InscricoesProjection projeÃ§Ã£o de inscriÃ§Ãµes
type InscricoesProjection struct {
	client *genesisdb.Client
	ctx    context.Context
}

// NewInscricoesProjection cria nova projeÃ§Ã£o de inscriÃ§Ãµes
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

// Rebuild reconstrÃ³i a projeÃ§Ã£o do zero
func (p *InscricoesProjection) Rebuild() error {
	// Limpar projeÃ§Ã£o
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

// clear limpa a projeÃ§Ã£o
func (p *InscricoesProjection) clear() error {
	_, err := p.client.DB().ExecContext(p.ctx, `TRUNCATE TABLE projection_inscricoes CASCADE`)
	return err
}

// Event Handlers

// ðŸ”¥ CORRIGIDO: handleEstudanteInscrito
func (p *InscricoesProjection) handleEstudanteInscrito(event genesisdb.Event) error {
	log.Printf("ðŸ“˜ [INSCRICAO] Processando EstudanteInscrito")
	
	// ðŸ”¥ CORRIGIDO: Ler CodigoAcademia do payload
	var payload struct {
		CodigoAcademia string    `json:"CodigoAcademia"` // ðŸ”¥ STRING, nÃ£o UUID
		Tipo           string    `json:"Tipo"`
		AnoInscricao   string    `json:"AnoInscricao"`
		Curso          *string   `json:"Curso"`
		CreatedAt      time.Time `json:"CreatedAt"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		log.Printf("âŒ [INSCRICAO] Erro ao parsear payload: %v", err)
		log.Printf("   Payload: %s", string(event.Payload))
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	log.Printf("ðŸ“Š [INSCRICAO] Dados parseados:")
	log.Printf("   CodigoAcademia: %s", payload.CodigoAcademia)
	log.Printf("   Tipo: %s", payload.Tipo)
	log.Printf("   AnoInscricao: %s", payload.AnoInscricao)

	// event.AggregateID Ã© o ID do ESTUDANTE
	estudanteID := event.AggregateID

	// ðŸ”¥ BUSCAR UUID da academia usando o cÃ³digo
	var academiaID uuid.UUID
	queryAcademiaID := `SELECT id FROM projection_academias WHERE codigo_academia = $1`
	err := p.client.DB().GetContext(p.ctx, &academiaID, queryAcademiaID, payload.CodigoAcademia)
	if err != nil {
		log.Printf("âŒ [INSCRICAO] Academia nÃ£o encontrada com cÃ³digo: %s", payload.CodigoAcademia)
		return fmt.Errorf("academia nÃ£o encontrada: %w", err)
	}

	// Buscar cÃ³digo do estudante
	var codigoEstudante string
	queryEstudante := `SELECT codigo_estudante FROM projection_estudantes WHERE id = $1`
	err = p.client.DB().GetContext(p.ctx, &codigoEstudante, queryEstudante, estudanteID)
	if err != nil {
		log.Printf("âŒ [INSCRICAO] Estudante nÃ£o encontrado: %s", estudanteID)
		return fmt.Errorf("estudante nÃ£o encontrado: %w", err)
	}

	log.Printf("ðŸ” [INSCRICAO] IDs resolvidos:")
	log.Printf("   EstudanteID: %s (%s)", estudanteID, codigoEstudante)
	log.Printf("   AcademiaID: %s (%s)", academiaID, payload.CodigoAcademia)

	// ðŸ”¥ INSERIR inscriÃ§Ã£o na projeÃ§Ã£o
	query := `
		INSERT INTO projection_inscricoes (
			estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso, status, created_at, updated_at, 
			event_id, version
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`

	result, err := p.client.DB().ExecContext(
		p.ctx, query,
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
		log.Printf("âŒ [INSCRICAO] Erro ao inserir: %v", err)
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	log.Printf("âœ… [INSCRICAO] InscriÃ§Ã£o criada! (rows: %d)", rowsAffected)

	// Atualizar contador de inscriÃ§Ãµes pendentes na academia
	updateQuery := `
		UPDATE projection_academias
		SET total_inscricoes_pendentes = total_inscricoes_pendentes + 1
		WHERE id = $1
	`
	p.client.DB().ExecContext(p.ctx, updateQuery, academiaID)

	// Atualizar contador de inscriÃ§Ãµes no estudante
	updateEstudanteQuery := `
		UPDATE projection_estudantes
		SET total_inscricoes = total_inscricoes + 1
		WHERE id = $1
	`
	p.client.DB().ExecContext(p.ctx, updateEstudanteQuery, estudanteID)

	return nil
}

// ðŸ”¥ CORRIGIDO: handleInscricaoAprovada
func (p *InscricoesProjection) handleInscricaoAprovada(event genesisdb.Event) error {
	var payload struct {
		EstudanteID    uuid.UUID `json:"EstudanteID"`
		InscricaoID    uuid.UUID `json:"InscricaoID"`
		CodigoAcademia string    `json:"CodigoAcademia"` // ðŸ”¥ STRING
		Tipo           string    `json:"Tipo"`
		AnoInscricao   string    `json:"AnoInscricao"`
		Curso          *string   `json:"Curso"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	// Se EstudanteID nÃ£o vier no payload, usar aggregate ID
	estudanteID := payload.EstudanteID
	if estudanteID == uuid.Nil {
		estudanteID = event.AggregateID
	}

	// ðŸ”¥ BUSCAR UUID da academia
	var academiaID uuid.UUID
	queryAcademiaID := `SELECT id FROM projection_academias WHERE codigo_academia = $1`
	err := p.client.DB().GetContext(p.ctx, &academiaID, queryAcademiaID, payload.CodigoAcademia)
	if err != nil {
		log.Printf("âš ï¸ [INSCRICAO] Academia nÃ£o encontrada: %s", payload.CodigoAcademia)
		return nil // NÃ£o Ã© erro crÃ­tico
	}

	log.Printf("âœ… [INSCRICAO] Aprovando - Estudante: %s, Academia: %s", 
		estudanteID, academiaID)

	// ðŸ”¥ ATUALIZAR apenas a inscriÃ§Ã£o em 'espera'
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
	err = p.client.DB().QueryRowContext(
		p.ctx, query,
		estudanteID,
		academiaID,
		payload.Tipo,
	).Scan(&inscricaoID)

	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("âš ï¸ [INSCRICAO] Nenhuma inscriÃ§Ã£o pendente para aprovar")
			return nil
		}
		return err
	}

	log.Printf("âœ… [INSCRICAO] InscriÃ§Ã£o aprovada: %s", inscricaoID)
	return nil
}

// ðŸ”¥ CORRIGIDO: handleInscricaoReprovada
func (p *InscricoesProjection) handleInscricaoReprovada(event genesisdb.Event) error {
	var payload struct {
		EstudanteID    uuid.UUID `json:"EstudanteID"`
		InscricaoID    uuid.UUID `json:"InscricaoID"`
		CodigoAcademia string    `json:"CodigoAcademia"` // ðŸ”¥ STRING
		Motivo         string    `json:"Motivo"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	estudanteID := payload.EstudanteID
	if estudanteID == uuid.Nil {
		log.Printf("âš ï¸ [INSCRICAO] EstudanteID nÃ£o encontrado no payload!")
		return fmt.Errorf("EstudanteID ausente no payload")
	}

	// ðŸ”¥ BUSCAR UUID da academia
	var academiaID uuid.UUID
	queryAcademiaID := `SELECT id FROM projection_academias WHERE codigo_academia = $1`
	err := p.client.DB().GetContext(p.ctx, &academiaID, queryAcademiaID, payload.CodigoAcademia)
	if err != nil {
		log.Printf("âš ï¸ [INSCRICAO] Academia nÃ£o encontrada: %s", payload.CodigoAcademia)
		return nil
	}

	log.Printf("âŒ [INSCRICAO] Reprovando - Estudante: %s, Academia: %s", 
		estudanteID, academiaID)

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
	err = p.client.DB().QueryRowContext(
		p.ctx, query,
		estudanteID,
		academiaID,
	).Scan(&inscricaoID)

	if err != nil {
		if err == sql.ErrNoRows {
			log.Printf("âš ï¸ [INSCRICAO] Nenhuma inscriÃ§Ã£o pendente para reprovar")
			return nil
		}
		return err
	}

	log.Printf("âœ… [INSCRICAO] InscriÃ§Ã£o reprovada: %s", inscricaoID)
	return nil
}

// Query methods

// GetByEstudante busca inscriÃ§Ãµes de um estudante
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
	err := p.client.DB().SelectContext(p.ctx, &result, query, estudanteID)
	return result, err
}

// GetByAcademia busca inscriÃ§Ãµes de uma academia por status
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
	err := p.client.DB().SelectContext(p.ctx, &result, query, academiaID, status)
	return result, err
}

// GetAll retorna todas as inscriÃ§Ãµes
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
	err := p.client.DB().SelectContext(p.ctx, &result, query, limit, offset)
	return result, err
}

// CountAll conta total de inscriÃ§Ãµes
func (p *InscricoesProjection) CountAll() (int, error) {
	query := `SELECT COUNT(*) FROM projection_inscricoes`
	
	var count int
	err := p.client.DB().GetContext(p.ctx, &count, query)
	return count, err
}

// GetByID busca uma inscriÃ§Ã£o especÃ­fica
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
	err := p.client.DB().GetContext(p.ctx, &dto, query, id)
	if err != nil {
		return nil, err
	}
	return &dto, nil
}

// InscricaoDTO com cÃ³digos
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