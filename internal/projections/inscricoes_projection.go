package projections

import (
	"database/sql"
	"encoding/json"
	"fmt"
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

func (ip *InscricoesProjection) Name() string { return "inscricoes" }

func (ip *InscricoesProjection) Handle(event db.Event) error {
	switch event.EventType {
	case "EstudanteInscrito":
		return ip.handleEstudanteInscrito(event)
	case "InscricaoAprovada":
		return ip.handleInscricaoAprovada(event)
	case "InscricaoReprovada":
		return ip.handleInscricaoReprovada(event)
	case "EstudanteVinculado":
		return ip.handleEstudanteVinculado(event)
	}
	return nil
}

func (ip *InscricoesProjection) Rebuild() error {
	if err := ip.clear(); err != nil {
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
	
	rows, err := ip.client.DB().Queryx(query)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var event db.Event
		err := rows.Scan(&event.ID, &event.EventID, &event.AggregateID, &event.AggregateType,
			&event.EventType, &event.EventVersion, &event.Payload, &event.Metadata,
			&event.OccurredAt, &event.RecordedAt, &event.LedgerHash, &event.PreviousHash)
		if err != nil {
			return err
		}
		if err := ip.Handle(event); err != nil {
			return fmt.Errorf("erro ao processar evento %d: %w", event.ID, err)
		}
	}
	return rows.Err()
}

func (ip *InscricoesProjection) GetLastProcessedEventID() (int64, error) {
	safeName := db.SafeString(ip.Name())
	query := fmt.Sprintf(`SELECT last_processed_event_id FROM projection_checkpoints WHERE projection_name = '%s'`, safeName)
	
	var lastID int64
	err := ip.client.DB().QueryRow(query).Scan(&lastID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return lastID, err
}

func (ip *InscricoesProjection) UpdateCheckpoint(eventID int64) error {
	safeName := db.SafeString(ip.Name())
	eventID = int64(db.ValidateOffset(int(eventID)))
	
	query := fmt.Sprintf(`
		INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at, events_processed)
		VALUES ('%s', %d, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) DO UPDATE SET
			last_processed_event_id = %d, last_processed_at = CURRENT_TIMESTAMP,
			events_processed = projection_checkpoints.events_processed + 1
	`, safeName, eventID, eventID)
	
	_, err := ip.client.DB().Exec(query)
	return err
}

func (ip *InscricoesProjection) clear() error {
	_, err := ip.client.DB().Exec(`TRUNCATE TABLE projection_inscricoes CASCADE`)
	return err
}

func (ip *InscricoesProjection) handleEstudanteInscrito(event db.Event) error {
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
	if estudanteID == uuid.Nil {
		return fmt.Errorf("UUID estudante inválido")
	}

	// Buscar codigo_estudante
	queryCodigo := fmt.Sprintf(`SELECT codigo_estudante FROM projection_estudantes WHERE id = '%s'`, estudanteID)
	var codigoEstudante string
	err := ip.client.DB().QueryRow(queryCodigo).Scan(&codigoEstudante)
	if err != nil {
		return fmt.Errorf("estudante não encontrado: %w", err)
	}

	// Buscar academia_id
	safeCodAcad := db.SafeString(payload.CodigoAcademia)
	queryAcad := fmt.Sprintf(`SELECT id FROM projection_academias WHERE codigo_academia = '%s'`, safeCodAcad)
	var academiaID uuid.UUID
	err = ip.client.DB().QueryRow(queryAcad).Scan(&academiaID)
	if err != nil {
		return fmt.Errorf("academia não encontrada: %w", err)
	}

	safeTipo := db.SafeString(payload.Tipo)
	safeAnoInsc := db.SafeString(payload.AnoInscricao)
	
	var cursoStr string
	if payload.Curso != nil {
		cursoStr = fmt.Sprintf("'%s'", db.SafeString(*payload.Curso))
	} else {
		cursoStr = "NULL"
	}

	query := fmt.Sprintf(`
		INSERT INTO projection_inscricoes (
			id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso, status, status_usado, created_at, updated_at, 
			event_id, version
		) VALUES ('%s', '%s', '%s', '%s', '%s', '%s', '%s', %s, 'espera', FALSE, '%s', CURRENT_TIMESTAMP, '%s', %d)
	`, payload.InscricaoID, estudanteID, codigoEstudante, academiaID, safeCodAcad,
		safeTipo, safeAnoInsc, cursoStr, payload.CreatedAt.Format(time.RFC3339), 
		event.EventID, event.EventVersion)

	_, err = ip.client.DB().Exec(query)
	if err != nil {
		return err
	}

	// Atualizar contadores
	updateAcad := fmt.Sprintf(`UPDATE projection_academias SET total_inscricoes_pendentes = total_inscricoes_pendentes + 1 WHERE id = '%s'`, academiaID)
	ip.client.DB().Exec(updateAcad)
	
	updateEst := fmt.Sprintf(`UPDATE projection_estudantes SET total_inscricoes = total_inscricoes + 1 WHERE id = '%s'`, estudanteID)
	ip.client.DB().Exec(updateEst)

	return nil
}

func (ip *InscricoesProjection) handleInscricaoAprovada(event db.Event) error {
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
	if estudanteID == uuid.Nil {
		return fmt.Errorf("UUID estudante inválido")
	}

	// Buscar academia_id
	safeCodAcad := db.SafeString(payload.CodigoAcademia)
	queryAcad := fmt.Sprintf(`SELECT id FROM projection_academias WHERE codigo_academia = '%s'`, safeCodAcad)
	
	var academiaID uuid.UUID
	err := ip.client.DB().QueryRow(queryAcad).Scan(&academiaID)
	if err != nil {
		return nil // Academia não encontrada - não é erro crítico
	}

	safeTipo := db.SafeString(payload.Tipo)

	query := fmt.Sprintf(`
		UPDATE projection_inscricoes
		SET status = 'aprovado', updated_at = CURRENT_TIMESTAMP
		WHERE estudante_id = '%s' AND academia_id = '%s' AND status = 'espera' AND tipo = '%s'
	`, estudanteID, academiaID, safeTipo)

	ip.client.DB().Exec(query)
	return nil
}

func (ip *InscricoesProjection) handleInscricaoReprovada(event db.Event) error {
	var payload struct {
		EstudanteID    uuid.UUID `json:"EstudanteID"`
		CodigoAcademia string    `json:"CodigoAcademia"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	estudanteID := payload.EstudanteID
	if estudanteID == uuid.Nil {
		return fmt.Errorf("UUID estudante inválido")
	}

	// Buscar academia_id
	safeCodAcad := db.SafeString(payload.CodigoAcademia)
	queryAcad := fmt.Sprintf(`SELECT id FROM projection_academias WHERE codigo_academia = '%s'`, safeCodAcad)
	
	var academiaID uuid.UUID
	err := ip.client.DB().QueryRow(queryAcad).Scan(&academiaID)
	if err != nil {
		return nil
	}

	query := fmt.Sprintf(`
		UPDATE projection_inscricoes
		SET status = 'reprovado', updated_at = CURRENT_TIMESTAMP
		WHERE estudante_id = '%s' AND academia_id = '%s' AND status = 'espera'
	`, estudanteID, academiaID)

	ip.client.DB().Exec(query)
	return nil
}

func (ip *InscricoesProjection) handleEstudanteVinculado(event db.Event) error {
	var payload struct {
		InscricaoID uuid.UUID `json:"InscricaoID"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	if payload.InscricaoID == uuid.Nil {
		return fmt.Errorf("UUID inscrição inválido")
	}

	query := fmt.Sprintf(`
		UPDATE projection_inscricoes SET status_usado = TRUE, updated_at = CURRENT_TIMESTAMP 
		WHERE id = '%s'
	`, payload.InscricaoID)

	_, err := ip.client.DB().Exec(query)
	return err
}

func (ip *InscricoesProjection) GetByEstudante(estudanteID uuid.UUID) ([]InscricaoDTO, error) {
	if estudanteID == uuid.Nil {
		return nil, fmt.Errorf("UUID inválido")
	}

	query := fmt.Sprintf(`
		SELECT id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso, status, status_usado, created_at, updated_at, 
			event_id, version
		FROM projection_inscricoes WHERE estudante_id = '%s' ORDER BY created_at DESC
	`, estudanteID)

	rows, err := ip.client.DB().Queryx(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []InscricaoDTO
	for rows.Next() {
		var dto InscricaoDTO
		err := rows.StructScan(&dto)
		if err != nil {
			return nil, err
		}
		result = append(result, dto)
	}
	return result, rows.Err()
}

func (ip *InscricoesProjection) GetByAcademia(academiaID uuid.UUID, status string) ([]InscricaoDTO, error) {
	if academiaID == uuid.Nil {
		return nil, fmt.Errorf("UUID inválido")
	}

	safeStatus := db.SafeString(status)

	query := fmt.Sprintf(`
		SELECT id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso, status, status_usado, created_at, updated_at, 
			event_id, version
		FROM projection_inscricoes WHERE academia_id = '%s' AND status = '%s' ORDER BY created_at DESC
	`, academiaID, safeStatus)

	rows, err := ip.client.DB().Queryx(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []InscricaoDTO
	for rows.Next() {
		var dto InscricaoDTO
		err := rows.StructScan(&dto)
		if err != nil {
			return nil, err
		}
		result = append(result, dto)
	}
	return result, rows.Err()
}

func (ip *InscricoesProjection) GetByID(id uuid.UUID) (*InscricaoDTO, error) {
	if id == uuid.Nil {
		return nil, fmt.Errorf("UUID inválido")
	}

	query := fmt.Sprintf(`
		SELECT id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso, status, status_usado, created_at, updated_at, 
			event_id, version
		FROM projection_inscricoes WHERE id = '%s'
	`, id)

	var dto InscricaoDTO
	err := ip.client.DB().QueryRowx(query).StructScan(&dto)
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