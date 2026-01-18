// ============================================================================
// ARQUIVO: internal/projections/inscricoes_projection.go
// 🔥 COMPLETO COM EVENTO EstudanteVinculado
// ============================================================================

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
	case "EstudanteVinculado": // 🔥 NOVO
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
		SELECT 
			id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE event_type IN ('EstudanteInscrito', 'InscricaoAprovada', 'InscricaoReprovada', 'EstudanteVinculado')
		ORDER BY id ASC
	`

	rows, err := p.client.DB().Query(query)
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
	query := fmt.Sprintf(`
		SELECT last_processed_event_id 
		FROM projection_checkpoints 
		WHERE projection_name = '%s'
	`, p.Name())

	var lastID int64
	err := p.client.DB().QueryRow(query).Scan(&lastID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	return lastID, nil
}

func (p *InscricoesProjection) UpdateCheckpoint(eventID int64) error {
	query := fmt.Sprintf(`
		INSERT INTO projection_checkpoints (
			projection_name, 
			last_processed_event_id, 
			last_processed_at,
			events_processed
		) VALUES ('%s', %d, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) 
		DO UPDATE SET
			last_processed_event_id = %d,
			last_processed_at = CURRENT_TIMESTAMP,
			events_processed = projection_checkpoints.events_processed + 1
	`, p.Name(), eventID, eventID)

	_, err := p.client.DB().Exec(query)
	return err
}

func (p *InscricoesProjection) clear() error {
	_, err := p.client.DB().Exec(`TRUNCATE TABLE projection_inscricoes CASCADE`)
	return err
}

// 🔥 ATUALIZADO: Incluir InscricaoID e StatusUsado
func (p *InscricoesProjection) handleEstudanteInscrito(event db.Event) error {
	log.Printf("📘 [INSCRICAO] Processando EstudanteInscrito - EventID: %s, AggregateID: %s",
		event.EventID.String(), event.AggregateID.String())

	var payload struct {
		InscricaoID    uuid.UUID `json:"InscricaoID"` // 🔥 NOVO
		CodigoAcademia string    `json:"CodigoAcademia"`
		Tipo           string    `json:"Tipo"`
		AnoInscricao   string    `json:"AnoInscricao"`
		Curso          *string   `json:"Curso"`
		CreatedAt      time.Time `json:"CreatedAt"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	log.Printf("📋 [INSCRICAO] Payload - InscricaoID: %s, CodigoAcademia: %s, Tipo: %s",
		payload.InscricaoID.String(), payload.CodigoAcademia, payload.Tipo)

	estudanteID := event.AggregateID

	// 1. Buscar codigo_estudante
	var codigoEstudante string
	queryEstudante := fmt.Sprintf(`
		SELECT codigo_estudante 
		FROM projection_estudantes 
		WHERE id = '%s'
	`, estudanteID.String())

	err := p.client.DB().QueryRow(queryEstudante).Scan(&codigoEstudante)
	if err != nil {
		log.Printf("❌ [INSCRICAO] Estudante não encontrado: %s - Erro: %v",
			estudanteID.String(), err)
		return fmt.Errorf("estudante não encontrado: %w", err)
	}

	log.Printf("✅ [INSCRICAO] Estudante encontrado: %s (Código: %s)",
		estudanteID.String(), codigoEstudante)

	// 2. Buscar UUID da academia
	var academiaID uuid.UUID
	queryAcademiaID := fmt.Sprintf(`
		SELECT id 
		FROM projection_academias 
		WHERE codigo_academia = '%s'
	`, payload.CodigoAcademia)

	err = p.client.DB().QueryRow(queryAcademiaID).Scan(&academiaID)
	if err != nil {
		log.Printf("❌ [INSCRICAO] Academia não encontrada: %s - Erro: %v",
			payload.CodigoAcademia, err)
		return fmt.Errorf("academia não encontrada: %w", err)
	}

	log.Printf("✅ [INSCRICAO] Academia encontrada: %s (ID: %s)",
		payload.CodigoAcademia, academiaID.String())

	// 3. Inserir inscrição
	cursoStr := "NULL"
	if payload.Curso != nil {
		cursoStr = fmt.Sprintf("'%s'", *payload.Curso)
	}

	query := fmt.Sprintf(`
		INSERT INTO projection_inscricoes (
			id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso, status, status_usado, created_at, updated_at, 
			event_id, version
		) VALUES (
			'%s', '%s', '%s', '%s', '%s',
			'%s', '%s', %s, 'espera', FALSE,
			'%s', CURRENT_TIMESTAMP,
			'%s', %d
		)
	`,
		payload.InscricaoID.String(), // 🔥 Usar ID do payload
		estudanteID.String(),
		codigoEstudante,
		academiaID.String(),
		payload.CodigoAcademia,
		payload.Tipo,
		payload.AnoInscricao,
		cursoStr,
		payload.CreatedAt.Format(time.RFC3339),
		event.EventID.String(),
		event.EventVersion,
	)

	log.Printf("🔍 [INSCRICAO] Executando INSERT...")
	result, err := p.client.DB().Exec(query)
	if err != nil {
		log.Printf("❌ [INSCRICAO] Erro ao inserir: %v", err)
		return err
	}

	rows, _ := result.RowsAffected()
	log.Printf("✅ [INSCRICAO] Inserção OK! Rows: %d", rows)

	// 4. Atualizar contadores
	updateAcademia := fmt.Sprintf(`
		UPDATE projection_academias
		SET total_inscricoes_pendentes = total_inscricoes_pendentes + 1
		WHERE id = '%s'
	`, academiaID.String())
	p.client.DB().Exec(updateAcademia)

	updateEstudante := fmt.Sprintf(`
		UPDATE projection_estudantes
		SET total_inscricoes = total_inscricoes + 1
		WHERE id = '%s'
	`, estudanteID.String())
	p.client.DB().Exec(updateEstudante)

	log.Printf("✅ [INSCRICAO] Processamento completo - Inscrição criada com status 'espera'")
	return nil
}

func (p *InscricoesProjection) handleInscricaoAprovada(event db.Event) error {
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
	queryAcademiaID := fmt.Sprintf(`SELECT id FROM projection_academias WHERE codigo_academia = '%s'`, payload.CodigoAcademia)
	err := p.client.DB().QueryRow(queryAcademiaID).Scan(&academiaID)
	if err != nil {
		return nil
	}

	query := fmt.Sprintf(`
		UPDATE projection_inscricoes
		SET 
			status = 'aprovado',
			updated_at = CURRENT_TIMESTAMP
		WHERE estudante_id = '%s' 
		  AND academia_id = '%s' 
		  AND status = 'espera'
		  AND tipo = '%s'
		RETURNING id
	`, estudanteID.String(), academiaID.String(), payload.Tipo)

	var inscricaoID uuid.UUID
	err = p.client.DB().QueryRow(query).Scan(&inscricaoID)

	if err != nil && err != sql.ErrNoRows {
		return err
	}

	return nil
}

func (p *InscricoesProjection) handleInscricaoReprovada(event db.Event) error {
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
	queryAcademiaID := fmt.Sprintf(`SELECT id FROM projection_academias WHERE codigo_academia = '%s'`, payload.CodigoAcademia)
	err := p.client.DB().QueryRow(queryAcademiaID).Scan(&academiaID)
	if err != nil {
		return nil
	}

	query := fmt.Sprintf(`
		UPDATE projection_inscricoes
		SET 
			status = 'reprovado',
			updated_at = CURRENT_TIMESTAMP
		WHERE estudante_id = '%s' 
		  AND academia_id = '%s' 
		  AND status = 'espera'
		RETURNING id
	`, estudanteID.String(), academiaID.String())

	var inscricaoID uuid.UUID
	err = p.client.DB().QueryRow(query).Scan(&inscricaoID)

	if err != nil && err != sql.ErrNoRows {
		return err
	}

	return nil
}

// 🔥 NOVO: Marcar inscrição como usada quando estudante vincular
func (p *InscricoesProjection) handleEstudanteVinculado(event db.Event) error {
	log.Printf("🔗 [INSCRICAO] Marcando inscrição como usada")

	var payload struct {
		InscricaoID uuid.UUID `json:"InscricaoID"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	query := fmt.Sprintf(`
		UPDATE projection_inscricoes
		SET 
			status_usado = TRUE,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = '%s'
	`, payload.InscricaoID.String())

	result, err := p.client.DB().Exec(query)
	if err != nil {
		log.Printf("❌ [INSCRICAO] Erro ao marcar como usada: %v", err)
		return err
	}

	rows, _ := result.RowsAffected()
	log.Printf("✅ [INSCRICAO] Inscrição marcada como usada! (rows: %d)", rows)

	return nil
}

// Query methods

func (p *InscricoesProjection) GetByEstudante(estudanteID uuid.UUID) ([]InscricaoDTO, error) {
	query := fmt.Sprintf(`
		SELECT 
			id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso, status, status_usado, created_at, updated_at, 
			event_id, version
		FROM projection_inscricoes
		WHERE estudante_id = '%s'
		ORDER BY created_at DESC
	`, estudanteID.String())

	rows, err := p.client.DB().Query(query)
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
	query := fmt.Sprintf(`
		SELECT 
			id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso, status, status_usado, created_at, updated_at, 
			event_id, version
		FROM projection_inscricoes
		WHERE academia_id = '%s' AND status = '%s'
		ORDER BY created_at DESC
	`, academiaID.String(), status)

	rows, err := p.client.DB().Query(query)
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
	query := fmt.Sprintf(`
		SELECT 
			id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso, status, status_usado, created_at, updated_at, 
			event_id, version
		FROM projection_inscricoes
		WHERE id = '%s'
	`, id.String())

	var dto InscricaoDTO
	err := p.client.DB().QueryRow(query).Scan(
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
	StatusUsado     bool      `db:"status_usado" json:"status_usado"` // 🔥 NOVO
	CreatedAt       time.Time `db:"created_at" json:"created_at"`
	UpdatedAt       time.Time `db:"updated_at" json:"updated_at"`
	EventID         uuid.UUID `db:"event_id" json:"event_id"`
	Version         int       `db:"version" json:"version"`
}
