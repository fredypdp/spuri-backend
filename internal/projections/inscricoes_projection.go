// ============================================================================
// ARQUIVO: internal/projections/inscricoes_projection.go
// 🔥 CORRIGIDO: GetLastProcessedEventID com tratamento de erro
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

	rows, err := p.client.DB().QueryContext(p.ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var event genesisdb.Event
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

// 🔥 CORRIGIDO: Tratamento de sql.ErrNoRows
func (p *InscricoesProjection) GetLastProcessedEventID() (int64, error) {
	query := `
		SELECT last_processed_event_id 
		FROM projection_checkpoints 
		WHERE projection_name = $1
	`

	var lastID int64
	err := p.client.DB().QueryRowContext(p.ctx, query, p.Name()).Scan(&lastID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	return lastID, nil
}

func (p *InscricoesProjection) UpdateCheckpoint(eventID int64) error {
	query := `
		INSERT INTO projection_checkpoints (
			projection_name, 
			last_processed_event_id, 
			last_processed_at,
			events_processed
		) VALUES ($1, $2, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) 
		DO UPDATE SET
			last_processed_event_id = $2,
			last_processed_at = CURRENT_TIMESTAMP,
			events_processed = projection_checkpoints.events_processed + 1
	`

	_, err := p.client.DB().ExecContext(p.ctx, query, p.Name(), eventID)
	return err
}

func (p *InscricoesProjection) clear() error {
	_, err := p.client.DB().ExecContext(p.ctx, `TRUNCATE TABLE projection_inscricoes CASCADE`)
	return err
}

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
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	estudanteID := event.AggregateID

	// Buscar UUID da academia
	var academiaID uuid.UUID
	queryAcademiaID := `SELECT id FROM projection_academias WHERE codigo_academia = $1`
	err := p.client.DB().QueryRowContext(p.ctx, queryAcademiaID, payload.CodigoAcademia).Scan(&academiaID)
	if err != nil {
		log.Printf("❌ [INSCRICAO] Academia não encontrada: %s", payload.CodigoAcademia)
		return fmt.Errorf("academia não encontrada: %w", err)
	}

	// Buscar código do estudante
	var codigoEstudante string
	queryEstudante := `SELECT codigo_estudante FROM projection_estudantes WHERE id = $1`
	err = p.client.DB().QueryRowContext(p.ctx, queryEstudante, estudanteID).Scan(&codigoEstudante)
	if err != nil {
		return fmt.Errorf("estudante não encontrado: %w", err)
	}

	query := `
		INSERT INTO projection_inscricoes (
			estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso, status, created_at, updated_at, 
			event_id, version
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`

	_, err = p.client.DB().ExecContext(
		p.ctx, query,
		estudanteID, codigoEstudante, academiaID, payload.CodigoAcademia,
		payload.Tipo, payload.AnoInscricao, payload.Curso, "espera",
		payload.CreatedAt, time.Now(), event.EventID, event.EventVersion,
	)

	if err == nil {
		// Atualizar contadores
		updateAcademia := `
			UPDATE projection_academias
			SET total_inscricoes_pendentes = total_inscricoes_pendentes + 1
			WHERE id = $1
		`
		p.client.DB().ExecContext(p.ctx, updateAcademia, academiaID)

		updateEstudante := `
			UPDATE projection_estudantes
			SET total_inscricoes = total_inscricoes + 1
			WHERE id = $1
		`
		p.client.DB().ExecContext(p.ctx, updateEstudante, estudanteID)
	}

	return err
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
	err := p.client.DB().QueryRowContext(p.ctx, queryAcademiaID, payload.CodigoAcademia).Scan(&academiaID)
	if err != nil {
		return nil
	}

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
		estudanteID, academiaID, payload.Tipo,
	).Scan(&inscricaoID)

	if err != nil && err != sql.ErrNoRows {
		return err
	}

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
		return fmt.Errorf("EstudanteID ausente no payload")
	}

	// Buscar UUID da academia
	var academiaID uuid.UUID
	queryAcademiaID := `SELECT id FROM projection_academias WHERE codigo_academia = $1`
	err := p.client.DB().QueryRowContext(p.ctx, queryAcademiaID, payload.CodigoAcademia).Scan(&academiaID)
	if err != nil {
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
	err = p.client.DB().QueryRowContext(
		p.ctx, query,
		estudanteID, academiaID,
	).Scan(&inscricaoID)

	if err != nil && err != sql.ErrNoRows {
		return err
	}

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

	rows, err := p.client.DB().QueryContext(p.ctx, query, estudanteID)
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
			&dto.AnoInscricao, &dto.Curso, &dto.Status,
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
	query := `
		SELECT 
			id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso, status, created_at, updated_at, 
			event_id, version
		FROM projection_inscricoes
		WHERE academia_id = $1 AND status = $2
		ORDER BY created_at DESC
	`

	rows, err := p.client.DB().QueryContext(p.ctx, query, academiaID, status)
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
			&dto.AnoInscricao, &dto.Curso, &dto.Status,
			&dto.CreatedAt, &dto.UpdatedAt, &dto.EventID, &dto.Version,
		)
		if err != nil {
			return nil, err
		}
		result = append(result, dto)
	}

	return result, rows.Err()
}

func (p *InscricoesProjection) GetAll(limit, offset int) ([]InscricaoDTO, error) {
	query := fmt.Sprintf(`
		SELECT 
			id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso, status, created_at, updated_at, 
			event_id, version
		FROM projection_inscricoes
		ORDER BY created_at DESC
		LIMIT %d OFFSET %d
	`, limit, offset)

	rows, err := p.client.DB().QueryContext(p.ctx, query)
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
			&dto.AnoInscricao, &dto.Curso, &dto.Status,
			&dto.CreatedAt, &dto.UpdatedAt, &dto.EventID, &dto.Version,
		)
		if err != nil {
			return nil, err
		}
		result = append(result, dto)
	}

	return result, rows.Err()
}

func (p *InscricoesProjection) CountAll() (int, error) {
	query := `SELECT COUNT(*) FROM projection_inscricoes`
	
	var count int
	err := p.client.DB().QueryRowContext(p.ctx, query).Scan(&count)
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
	err := p.client.DB().QueryRowContext(p.ctx, query, id).Scan(
		&dto.ID, &dto.EstudanteID, &dto.CodigoEstudante,
		&dto.AcademiaID, &dto.CodigoAcademia, &dto.Tipo,
		&dto.AnoInscricao, &dto.Curso, &dto.Status,
		&dto.CreatedAt, &dto.UpdatedAt, &dto.EventID, &dto.Version,
	)
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