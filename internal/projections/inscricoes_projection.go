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

func (p *InscricoesProjection) Name() string { return "inscricoes" }

// ============================================================================
// Interface Projection
// ============================================================================

func (p *InscricoesProjection) GetLastProcessedEventID() (int64, error) {
	var lastID int64
	err := p.client.DB().QueryRow(
		`SELECT last_processed_event_id FROM projection_checkpoints WHERE projection_name = $1`,
		p.Name(),
	).Scan(&lastID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return lastID, err
}

func (p *InscricoesProjection) UpdateCheckpoint(eventID int64) error {
	eventID = int64(db.ValidateOffset(int(eventID)))
	_, err := p.client.DB().Exec(`
		INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at, events_processed)
		VALUES ($1, $2, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) DO UPDATE SET
			last_processed_event_id = $2,
			last_processed_at = CURRENT_TIMESTAMP,
			events_processed = projection_checkpoints.events_processed + 1
	`, p.Name(), eventID)
	return err
}

func (p *InscricoesProjection) Handle(event db.Event) error {
	handlers := map[string]func(db.Event) error{
		"EstudanteInscrito":  p.handleEstudanteInscrito,
		"InscricaoAprovada":  p.handleInscricaoAprovada,
		"InscricaoReprovada": p.handleInscricaoReprovada,
		"EstudanteVinculado": p.handleEstudanteVinculado,
	}
	if handler, ok := handlers[event.EventType]; ok {
		log.Printf("[DEBUG] [inscricoes] Processando %s: %s", event.EventType, event.EventID)
		return handler(event)
	}
	return nil
}

func (p *InscricoesProjection) Rebuild() error {
	log.Printf("[DEBUG] [inscricoes] Rebuild iniciado")
	if err := p.clear(); err != nil {
		return fmt.Errorf("falha ao limpar: %w", err)
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
	count := 0
	for rows.Next() {
		var event db.Event
		var prevHash sql.NullString
		if err := rows.Scan(
			&event.ID, &event.EventID, &event.AggregateID, &event.AggregateType,
			&event.EventType, &event.EventVersion, &event.Payload, &event.Metadata,
			&event.OccurredAt, &event.RecordedAt, &event.LedgerHash, &prevHash,
		); err != nil {
			return err
		}
		if prevHash.Valid {
			event.PreviousHash = &prevHash.String
		}
		if err := p.Handle(event); err != nil {
			return fmt.Errorf("erro no evento %d: %w", event.ID, err)
		}
		count++
	}
	log.Printf("[DEBUG] [inscricoes] Rebuild concluído: %d eventos", count)
	return rows.Err()
}

func (p *InscricoesProjection) clear() error {
	_, err := p.client.DB().Exec(`TRUNCATE TABLE projection_inscricoes CASCADE`)
	return err
}

// ============================================================================
// Handlers de evento
// ============================================================================

func (p *InscricoesProjection) handleEstudanteInscrito(event db.Event) error {
	var payload struct {
		EstudanteID     uuid.UUID  `json:"EstudanteID"`
		CodigoEstudante string     `json:"CodigoEstudante"`
		AcademiaID      uuid.UUID  `json:"AcademiaID"`
		CodigoAcademia  string     `json:"CodigoAcademia"`
		Tipo            string     `json:"Tipo"`
		AnoInscricao    string     `json:"AnoInscricao"`
		CursoID         *uuid.UUID `json:"CursoID"`
		Status          string     `json:"Status"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error EstudanteInscrito: %w", err)
	}

	var cursoID interface{}
	if payload.CursoID != nil {
		cursoID = payload.CursoID.String()
	}

	_, err := p.client.DB().Exec(`
		INSERT INTO projection_inscricoes (
			id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso_id, status, status_usado,
			created_at, updated_at, event_id, version
		) VALUES (
			uuid_generate_v4(), $1, $2, $3, $4,
			$5, $6, $7, $8, FALSE,
			CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, $9, $10
		)
	`,
		payload.EstudanteID, payload.CodigoEstudante, payload.AcademiaID, payload.CodigoAcademia,
		payload.Tipo, payload.AnoInscricao, cursoID, payload.Status,
		event.EventID, event.EventVersion,
	)
	return err
}

// handleInscricaoAprovada processa o evento "InscricaoAprovada" emitido pelo
// aggregate Academia quando uma inscrição pendente é aceita.
//
// CORREÇÃO: o status correto é 'aprovado' (sem o 'a' feminino).
// O schema da tabela tem CHECK (status IN ('espera', 'aprovado', 'reprovado')).
// O valor anterior 'aprovada' violava a constraint e causava erro silencioso.
func (p *InscricoesProjection) handleInscricaoAprovada(event db.Event) error {
	var payload struct {
		InscricaoID uuid.UUID `json:"InscricaoID"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error InscricaoAprovada: %w", err)
	}

	_, err := p.client.DB().Exec(`
		UPDATE projection_inscricoes
		SET status = 'aprovado',
			updated_at = CURRENT_TIMESTAMP,
			event_id = $1,
			version = $2
		WHERE id = $3
	`, event.EventID, event.EventVersion, payload.InscricaoID)
	return err
}

// handleInscricaoReprovada processa o evento "InscricaoReprovada" emitido pelo
// aggregate Academia quando uma inscrição pendente é rejeitada.
//
// CORREÇÃO: o status correto é 'reprovado' (sem o 'a' feminino).
// O schema da tabela tem CHECK (status IN ('espera', 'aprovado', 'reprovado')).
// O valor anterior 'reprovada' violava a constraint e causava erro silencioso.
func (p *InscricoesProjection) handleInscricaoReprovada(event db.Event) error {
	var payload struct {
		InscricaoID uuid.UUID `json:"InscricaoID"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error InscricaoReprovada: %w", err)
	}

	_, err := p.client.DB().Exec(`
		UPDATE projection_inscricoes
		SET status = 'reprovado',
			updated_at = CURRENT_TIMESTAMP,
			event_id = $1,
			version = $2
		WHERE id = $3
	`, event.EventID, event.EventVersion, payload.InscricaoID)
	return err
}

// handleEstudanteVinculado processa o evento "EstudanteVinculado" emitido pelo
// aggregate Estudante quando ele usa uma inscrição aprovada para se vincular
// efetivamente à academia.
//
// CORREÇÃO: o valor 'vinculado' não existe na CHECK constraint da tabela.
// A constraint aceita apenas ('espera', 'aprovado', 'reprovado').
// O comportamento correto é manter status = 'aprovado' e setar status_usado = TRUE,
// indicando que a inscrição já foi consumida pelo vínculo.
func (p *InscricoesProjection) handleEstudanteVinculado(event db.Event) error {
	var payload struct {
		InscricaoID uuid.UUID `json:"InscricaoID"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error EstudanteVinculado: %w", err)
	}

	_, err := p.client.DB().Exec(`
		UPDATE projection_inscricoes
		SET status_usado = TRUE,
			updated_at = CURRENT_TIMESTAMP,
			event_id = $1,
			version = $2
		WHERE id = $3
	`, event.EventID, event.EventVersion, payload.InscricaoID)
	return err
}

// ============================================================================
// Queries de leitura
// ============================================================================

type InscricaoDTO struct {
	ID              uuid.UUID  `json:"id"`
	EstudanteID     uuid.UUID  `json:"estudante_id"`
	CodigoEstudante string     `json:"codigo_estudante"`
	AcademiaID      uuid.UUID  `json:"academia_id"`
	CodigoAcademia  string     `json:"codigo_academia"`
	Tipo            string     `json:"tipo"`
	AnoInscricao    string     `json:"ano_inscricao"`
	CursoID         *uuid.UUID `json:"curso_id,omitempty"`
	Status          string     `json:"status"`
	StatusUsado     bool       `json:"status_usado"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	EventID         *uuid.UUID `json:"event_id,omitempty"`
	Version         *int       `json:"version,omitempty"`
}

func (p *InscricoesProjection) GetByID(id uuid.UUID) (*InscricaoDTO, error) {
	row := p.client.DB().QueryRow(`
		SELECT id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso_id, status, status_usado, created_at, updated_at, event_id, version
		FROM projection_inscricoes WHERE id = $1
	`, id)
	return scanInscricao(row)
}

func (p *InscricoesProjection) GetByEstudante(estudanteID uuid.UUID) ([]InscricaoDTO, error) {
	rows, err := p.client.DB().Query(`
		SELECT id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso_id, status, status_usado, created_at, updated_at, event_id, version
		FROM projection_inscricoes
		WHERE estudante_id = $1
		ORDER BY created_at DESC
	`, estudanteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanInscricoes(rows)
}

// GetByCodigoEstudante filtra pelo código string do estudante (ex: "EST-001").
func (p *InscricoesProjection) GetByCodigoEstudante(codigoEstudante string) ([]InscricaoDTO, error) {
	rows, err := p.client.DB().Query(`
		SELECT id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso_id, status, status_usado, created_at, updated_at, event_id, version
		FROM projection_inscricoes
		WHERE codigo_estudante = $1
		ORDER BY created_at DESC
	`, codigoEstudante)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanInscricoes(rows)
}

func (p *InscricoesProjection) GetByAcademia(codigoAcademia string, limit, offset int) ([]InscricaoDTO, int, error) {
	var total int
	p.client.DB().QueryRow(
		`SELECT COUNT(*) FROM projection_inscricoes WHERE codigo_academia = $1`,
		codigoAcademia,
	).Scan(&total)

	rows, err := p.client.DB().Query(`
		SELECT id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso_id, status, status_usado, created_at, updated_at, event_id, version
		FROM projection_inscricoes
		WHERE codigo_academia = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, codigoAcademia, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	inscricoes, err := scanInscricoes(rows)
	return inscricoes, total, err
}

func (p *InscricoesProjection) GetByAcademiaAndStatus(codigoAcademia, status string, limit, offset int) ([]InscricaoDTO, int, error) {
	var total int
	p.client.DB().QueryRow(
		`SELECT COUNT(*) FROM projection_inscricoes WHERE codigo_academia = $1 AND status = $2`,
		codigoAcademia, status,
	).Scan(&total)

	rows, err := p.client.DB().Query(`
		SELECT id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso_id, status, status_usado, created_at, updated_at, event_id, version
		FROM projection_inscricoes
		WHERE codigo_academia = $1 AND status = $2
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4
	`, codigoAcademia, status, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	inscricoes, err := scanInscricoes(rows)
	return inscricoes, total, err
}

func (p *InscricoesProjection) GetAll(limit, offset int) ([]InscricaoDTO, int, error) {
	var total int
	p.client.DB().QueryRow(`SELECT COUNT(*) FROM projection_inscricoes`).Scan(&total)

	rows, err := p.client.DB().Query(`
		SELECT id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso_id, status, status_usado, created_at, updated_at, event_id, version
		FROM projection_inscricoes
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	inscricoes, err := scanInscricoes(rows)
	return inscricoes, total, err
}

func (p *InscricoesProjection) GetAllByStatus(status string, limit, offset int) ([]InscricaoDTO, int, error) {
	var total int
	p.client.DB().QueryRow(
		`SELECT COUNT(*) FROM projection_inscricoes WHERE status = $1`,
		status,
	).Scan(&total)

	rows, err := p.client.DB().Query(`
		SELECT id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso_id, status, status_usado, created_at, updated_at, event_id, version
		FROM projection_inscricoes
		WHERE status = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, status, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	inscricoes, err := scanInscricoes(rows)
	return inscricoes, total, err
}

func scanInscricao(row *sql.Row) (*InscricaoDTO, error) {
	var dto InscricaoDTO
	err := row.Scan(
		&dto.ID, &dto.EstudanteID, &dto.CodigoEstudante,
		&dto.AcademiaID, &dto.CodigoAcademia,
		&dto.Tipo, &dto.AnoInscricao, &dto.CursoID,
		&dto.Status, &dto.StatusUsado,
		&dto.CreatedAt, &dto.UpdatedAt, &dto.EventID, &dto.Version,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &dto, err
}

func scanInscricoes(rows *sql.Rows) ([]InscricaoDTO, error) {
	var result []InscricaoDTO
	for rows.Next() {
		var dto InscricaoDTO
		if err := rows.Scan(
			&dto.ID, &dto.EstudanteID, &dto.CodigoEstudante,
			&dto.AcademiaID, &dto.CodigoAcademia,
			&dto.Tipo, &dto.AnoInscricao, &dto.CursoID,
			&dto.Status, &dto.StatusUsado,
			&dto.CreatedAt, &dto.UpdatedAt, &dto.EventID, &dto.Version,
		); err != nil {
			continue
		}
		result = append(result, dto)
	}
	return result, rows.Err()
}