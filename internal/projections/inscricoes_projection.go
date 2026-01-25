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

func (p *InscricoesProjection) Handle(event db.Event) error {
	handlers := map[string]func(db.Event) error{
		"EstudanteInscrito":   p.handleEstudanteInscrito,
		"InscricaoAprovada":   p.handleInscricaoAprovada,
		"InscricaoReprovada":  p.handleInscricaoReprovada,
		"EstudanteVinculado":  p.handleEstudanteVinculado,
	}

	if handler, ok := handlers[event.EventType]; ok {
		log.Printf("[DEBUG] Processando %s: %s", event.EventType, event.EventID)
		return handler(event)
	}
	return nil
}

func (p *InscricoesProjection) Rebuild() error {
	log.Printf("[DEBUG] Rebuild iniciado")
	
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
		if err := rows.Scan(&event.ID, &event.EventID, &event.AggregateID, &event.AggregateType,
			&event.EventType, &event.EventVersion, &event.Payload, &event.Metadata,
			&event.OccurredAt, &event.RecordedAt, &event.LedgerHash, &event.PreviousHash); err != nil {
			return err
		}

		if err := p.Handle(event); err != nil {
			return fmt.Errorf("erro no evento %d: %w", event.ID, err)
		}
		count++
	}

	log.Printf("[DEBUG] Rebuild concluído: %d eventos processados", count)
	return rows.Err()
}

func (p *InscricoesProjection) GetLastProcessedEventID() (int64, error) {
	var lastID int64
	query := fmt.Sprintf(`SELECT last_processed_event_id FROM projection_checkpoints WHERE projection_name = '%s'`,
		db.SafeString(p.Name()))
	
	err := p.client.DB().QueryRow(query).Scan(&lastID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return lastID, err
}

func (p *InscricoesProjection) UpdateCheckpoint(eventID int64) error {
	eventID = int64(db.ValidateOffset(int(eventID)))
	query := fmt.Sprintf(`
		INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at, events_processed)
		VALUES ('%s', %d, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) DO UPDATE SET
			last_processed_event_id = %d, last_processed_at = CURRENT_TIMESTAMP,
			events_processed = projection_checkpoints.events_processed + 1
	`, db.SafeString(p.Name()), eventID, eventID)
	
	_, err := p.client.DB().Exec(query)
	return err
}

func (p *InscricoesProjection) clear() error {
	_, err := p.client.DB().Exec(`TRUNCATE TABLE projection_inscricoes CASCADE`)
	return err
}

func (p *InscricoesProjection) handleEstudanteInscrito(event db.Event) error {
	var payload struct {
		InscricaoID, CodigoAcademia, Tipo, AnoInscricao string
		Curso                                            *string
		CreatedAt                                        time.Time
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error: %w", err)
	}

	codigoEstudante, err := p.getCodigoEstudante(event.AggregateID)
	if err != nil {
		return err
	}

	academiaID, err := p.getAcademiaID(payload.CodigoAcademia)
	if err != nil {
		return err
	}

	query := fmt.Sprintf(`
		INSERT INTO projection_inscricoes (
			id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso, status, status_usado, created_at, updated_at, event_id, version
		) VALUES ('%s', '%s', '%s', '%s', '%s', '%s', '%s', %s, 'espera', FALSE, '%s', CURRENT_TIMESTAMP, '%s', %d)`,
		payload.InscricaoID, event.AggregateID, db.SafeString(codigoEstudante),
		academiaID, db.SafeString(payload.CodigoAcademia),
		db.SafeString(payload.Tipo), db.SafeString(payload.AnoInscricao),
		nullOrString(payload.Curso), payload.CreatedAt.Format(time.RFC3339),
		event.EventID, event.EventVersion)

	if _, err := p.client.DB().Exec(query); err != nil {
		return err
	}

	p.client.DB().Exec(fmt.Sprintf(`UPDATE projection_academias SET total_inscricoes_pendentes = total_inscricoes_pendentes + 1 WHERE id = '%s'`, academiaID))
	p.client.DB().Exec(fmt.Sprintf(`UPDATE projection_estudantes SET total_inscricoes = total_inscricoes + 1 WHERE id = '%s'`, event.AggregateID))

	return nil
}

func (p *InscricoesProjection) handleInscricaoAprovada(event db.Event) error {
	var payload struct {
		EstudanteID    uuid.UUID
		InscricaoID    uuid.UUID
		CodigoAcademia, Tipo string
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}

	estudanteID := payload.EstudanteID
	if estudanteID == uuid.Nil {
		estudanteID = event.AggregateID
	}

	academiaID, err := p.getAcademiaID(payload.CodigoAcademia)
	if err != nil {
		return nil
	}

	query := fmt.Sprintf(`
		UPDATE projection_inscricoes
		SET status = 'aprovado', updated_at = CURRENT_TIMESTAMP
		WHERE estudante_id = '%s' AND academia_id = '%s' AND status = 'espera' AND tipo = '%s'`,
		estudanteID, academiaID, db.SafeString(payload.Tipo))

	p.client.DB().Exec(query)
	return nil
}

func (p *InscricoesProjection) handleInscricaoReprovada(event db.Event) error {
	var payload struct {
		EstudanteID    uuid.UUID
		CodigoAcademia string
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}

	academiaID, err := p.getAcademiaID(payload.CodigoAcademia)
	if err != nil {
		return nil
	}

	query := fmt.Sprintf(`
		UPDATE projection_inscricoes
		SET status = 'reprovado', updated_at = CURRENT_TIMESTAMP
		WHERE estudante_id = '%s' AND academia_id = '%s' AND status = 'espera'`,
		payload.EstudanteID, academiaID)

	p.client.DB().Exec(query)
	return nil
}

func (p *InscricoesProjection) handleEstudanteVinculado(event db.Event) error {
	var payload struct{ InscricaoID uuid.UUID }

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}

	query := fmt.Sprintf(`
		UPDATE projection_inscricoes SET status_usado = TRUE, updated_at = CURRENT_TIMESTAMP 
		WHERE id = '%s'`, payload.InscricaoID)

	_, err := p.client.DB().Exec(query)
	return err
}

func (p *InscricoesProjection) getCodigoEstudante(estudanteID uuid.UUID) (string, error) {
	var codigo string
	query := fmt.Sprintf(`SELECT codigo_estudante FROM projection_estudantes WHERE id = '%s'`, estudanteID)
	err := p.client.DB().QueryRow(query).Scan(&codigo)
	if err != nil {
		return "", fmt.Errorf("estudante não encontrado: %w", err)
	}
	return codigo, nil
}

func (p *InscricoesProjection) getAcademiaID(codigoAcademia string) (uuid.UUID, error) {
	var id uuid.UUID
	query := fmt.Sprintf(`SELECT id FROM projection_academias WHERE codigo_academia = '%s'`, db.SafeString(codigoAcademia))
	err := p.client.DB().QueryRow(query).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("academia não encontrada: %w", err)
	}
	return id, nil
}

func (p *InscricoesProjection) GetByEstudante(estudanteID uuid.UUID) ([]InscricaoDTO, error) {
	if estudanteID == uuid.Nil {
		return nil, fmt.Errorf("UUID inválido")
	}
	return p.queryInscricoes(fmt.Sprintf("estudante_id = '%s' ORDER BY created_at DESC", estudanteID))
}

func (p *InscricoesProjection) GetByAcademia(academiaID uuid.UUID, status string) ([]InscricaoDTO, error) {
	if academiaID == uuid.Nil {
		return nil, fmt.Errorf("UUID inválido")
	}
	return p.queryInscricoes(fmt.Sprintf("academia_id = '%s' AND status = '%s' ORDER BY created_at DESC",
		academiaID, db.SafeString(status)))
}

func (p *InscricoesProjection) GetByID(id uuid.UUID) (*InscricaoDTO, error) {
	if id == uuid.Nil {
		return nil, fmt.Errorf("UUID inválido")
	}

	query := fmt.Sprintf(`
		SELECT id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso, status, status_usado, created_at, updated_at, event_id, version
		FROM projection_inscricoes WHERE id = '%s'`, id)

	var dto InscricaoDTO
	err := p.client.DB().QueryRow(query).Scan(
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

func (p *InscricoesProjection) queryInscricoes(whereClause string) ([]InscricaoDTO, error) {
	query := fmt.Sprintf(`
		SELECT id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso, status, status_usado, created_at, updated_at, event_id, version
		FROM projection_inscricoes WHERE %s
	`, whereClause)

	rows, err := p.client.DB().Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []InscricaoDTO
	for rows.Next() {
		var dto InscricaoDTO
		if err := rows.Scan(&dto.ID, &dto.EstudanteID, &dto.CodigoEstudante, &dto.AcademiaID,
			&dto.CodigoAcademia, &dto.Tipo, &dto.AnoInscricao, &dto.Curso, &dto.Status,
			&dto.StatusUsado, &dto.CreatedAt, &dto.UpdatedAt, &dto.EventID, &dto.Version); err != nil {
			continue
		}
		result = append(result, dto)
	}
	
	log.Printf("[DEBUG] %d inscrições encontradas", len(result))
	return result, rows.Err()
}

type InscricaoDTO struct {
	ID              uuid.UUID `json:"id"`
	EstudanteID     uuid.UUID `json:"estudante_id"`
	CodigoEstudante string    `json:"codigo_estudante"`
	AcademiaID      uuid.UUID `json:"academia_id"`
	CodigoAcademia  string    `json:"codigo_academia"`
	Tipo            string    `json:"tipo"`
	AnoInscricao    string    `json:"ano_inscricao"`
	Curso           *string   `json:"curso,omitempty"`
	Status          string    `json:"status"`
	StatusUsado     bool      `json:"status_usado"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	EventID         uuid.UUID `json:"event_id"`
	Version         int       `json:"version"`
}