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

func (ip *InscricoesProjection) Name() string { return "inscricoes" }

func (ip *InscricoesProjection) Handle(event db.Event) error {
	log.Printf("[DEBUG] InscricoesProjection.Handle - EventType: %s, EventID: %s", event.EventType, event.EventID)
	
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
	
	log.Printf("[DEBUG] InscricoesProjection.Handle - EventType não tratado: %s", event.EventType)
	return nil
}

func (ip *InscricoesProjection) Rebuild() error {
	log.Printf("[DEBUG] InscricoesProjection.Rebuild - Iniciando rebuild")
	
	if err := ip.clear(); err != nil {
		log.Printf("[ERROR] InscricoesProjection.Rebuild - Erro ao limpar tabela: %v", err)
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
	
	log.Printf("[DEBUG] InscricoesProjection.Rebuild - Query: %s", query)
	
	rows, err := ip.client.DB().Query(query)
	if err != nil {
		log.Printf("[ERROR] InscricoesProjection.Rebuild - Erro ao buscar eventos: %v", err)
		return err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var event db.Event
		err := rows.Scan(&event.ID, &event.EventID, &event.AggregateID, &event.AggregateType,
			&event.EventType, &event.EventVersion, &event.Payload, &event.Metadata,
			&event.OccurredAt, &event.RecordedAt, &event.LedgerHash, &event.PreviousHash)
		if err != nil {
			log.Printf("[ERROR] InscricoesProjection.Rebuild - Erro ao fazer scan do evento: %v", err)
			return err
		}
		if err := ip.Handle(event); err != nil {
			log.Printf("[ERROR] InscricoesProjection.Rebuild - Erro ao processar evento %d: %v", event.ID, err)
			return fmt.Errorf("erro ao processar evento %d: %w", event.ID, err)
		}
		count++
	}
	
	log.Printf("[DEBUG] InscricoesProjection.Rebuild - Rebuild concluído, %d eventos processados", count)
	return rows.Err()
}

func (ip *InscricoesProjection) GetLastProcessedEventID() (int64, error) {
	safeName := db.SafeString(ip.Name())
	
	query := fmt.Sprintf(`
		SELECT last_processed_event_id 
		FROM projection_checkpoints 
		WHERE projection_name = '%s'
	`, safeName)
	
	log.Printf("[DEBUG] InscricoesProjection.GetLastProcessedEventID - Query: %s", query)
	
	var lastID int64
	err := ip.client.DB().QueryRow(query).Scan(&lastID)
	if err == sql.ErrNoRows {
		log.Printf("[DEBUG] InscricoesProjection.GetLastProcessedEventID - Nenhum checkpoint encontrado")
		return 0, nil
	}
	
	if err != nil {
		log.Printf("[ERROR] InscricoesProjection.GetLastProcessedEventID - Erro: %v", err)
	} else {
		log.Printf("[DEBUG] InscricoesProjection.GetLastProcessedEventID - LastID: %d", lastID)
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
	
	log.Printf("[DEBUG] InscricoesProjection.UpdateCheckpoint - Query: %s", query)
	
	_, err := ip.client.DB().Exec(query)
	
	if err != nil {
		log.Printf("[ERROR] InscricoesProjection.UpdateCheckpoint - Erro: %v", err)
	} else {
		log.Printf("[DEBUG] InscricoesProjection.UpdateCheckpoint - Checkpoint atualizado para eventID: %d", eventID)
	}
	
	return err
}

func (ip *InscricoesProjection) clear() error {
	log.Printf("[DEBUG] InscricoesProjection.clear - Limpando tabela projection_inscricoes")
	_, err := ip.client.DB().Exec(`TRUNCATE TABLE projection_inscricoes CASCADE`)
	if err != nil {
		log.Printf("[ERROR] InscricoesProjection.clear - Erro: %v", err)
	}
	return err
}

func (ip *InscricoesProjection) handleEstudanteInscrito(event db.Event) error {
	log.Printf("[DEBUG] InscricoesProjection.handleEstudanteInscrito - EventID: %s", event.EventID)
	
	var payload struct {
		InscricaoID    uuid.UUID `json:"InscricaoID"`
		CodigoAcademia string    `json:"CodigoAcademia"`
		Tipo           string    `json:"Tipo"`
		AnoInscricao   string    `json:"AnoInscricao"`
		Curso          *string   `json:"Curso"`
		CreatedAt      time.Time `json:"CreatedAt"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		log.Printf("[ERROR] InscricoesProjection.handleEstudanteInscrito - Erro ao parsear payload: %v", err)
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	log.Printf("[DEBUG] InscricoesProjection.handleEstudanteInscrito - Payload: InscricaoID=%s, CodigoAcademia=%s", 
		payload.InscricaoID, payload.CodigoAcademia)

	estudanteID := event.AggregateID

	queryEst := fmt.Sprintf(`SELECT codigo_estudante FROM projection_estudantes WHERE id = '%s'`, estudanteID)
	log.Printf("[DEBUG] InscricoesProjection.handleEstudanteInscrito - Query estudante: %s", queryEst)
	
	var codigoEstudante string
	err := ip.client.DB().QueryRow(queryEst).Scan(&codigoEstudante)
	if err != nil {
		log.Printf("[ERROR] InscricoesProjection.handleEstudanteInscrito - Estudante não encontrado: %v", err)
		return fmt.Errorf("estudante não encontrado: %w", err)
	}

	safeCodigoAcad := db.SafeString(payload.CodigoAcademia)
	queryAcad := fmt.Sprintf(`SELECT id FROM projection_academias WHERE codigo_academia = '%s'`, safeCodigoAcad)
	log.Printf("[DEBUG] InscricoesProjection.handleEstudanteInscrito - Query academia: %s", queryAcad)
	
	var academiaID uuid.UUID
	err = ip.client.DB().QueryRow(queryAcad).Scan(&academiaID)
	if err != nil {
		log.Printf("[ERROR] InscricoesProjection.handleEstudanteInscrito - Academia não encontrada: %v", err)
		return fmt.Errorf("academia não encontrada: %w", err)
	}

	safeTipo := db.SafeString(payload.Tipo)
	safeAno := db.SafeString(payload.AnoInscricao)
	safeCodEst := db.SafeString(codigoEstudante)
	safeCodAcad := db.SafeString(payload.CodigoAcademia)
	
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
		) VALUES ('%s', '%s', '%s', '%s', '%s', '%s', '%s', %s, 'espera', FALSE, '%s', CURRENT_TIMESTAMP, '%s', %d)`,
		payload.InscricaoID, estudanteID, safeCodEst, academiaID, safeCodAcad,
		safeTipo, safeAno, cursoStr, payload.CreatedAt.Format(time.RFC3339), 
		event.EventID, event.EventVersion)

	log.Printf("[DEBUG] InscricoesProjection.handleEstudanteInscrito - Insert query: %s", query)

	_, err = ip.client.DB().Exec(query)
	if err != nil {
		log.Printf("[ERROR] InscricoesProjection.handleEstudanteInscrito - Erro ao inserir: %v", err)
		return err
	}

	log.Printf("[DEBUG] InscricoesProjection.handleEstudanteInscrito - Inscrição criada com sucesso")

	updateAcad := fmt.Sprintf(`UPDATE projection_academias SET total_inscricoes_pendentes = total_inscricoes_pendentes + 1 WHERE id = '%s'`, academiaID)
	log.Printf("[DEBUG] InscricoesProjection.handleEstudanteInscrito - Update academia: %s", updateAcad)
	ip.client.DB().Exec(updateAcad)
	
	updateEst := fmt.Sprintf(`UPDATE projection_estudantes SET total_inscricoes = total_inscricoes + 1 WHERE id = '%s'`, estudanteID)
	log.Printf("[DEBUG] InscricoesProjection.handleEstudanteInscrito - Update estudante: %s", updateEst)
	ip.client.DB().Exec(updateEst)

	return nil
}

func (ip *InscricoesProjection) handleInscricaoAprovada(event db.Event) error {
	log.Printf("[DEBUG] InscricoesProjection.handleInscricaoAprovada - EventID: %s", event.EventID)
	
	var payload struct {
		EstudanteID    uuid.UUID `json:"EstudanteID"`
		InscricaoID    uuid.UUID `json:"InscricaoID"`
		CodigoAcademia string    `json:"CodigoAcademia"`
		Tipo           string    `json:"Tipo"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		log.Printf("[ERROR] InscricoesProjection.handleInscricaoAprovada - Erro ao parsear payload: %v", err)
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	log.Printf("[DEBUG] InscricoesProjection.handleInscricaoAprovada - Payload: EstudanteID=%s, InscricaoID=%s", 
		payload.EstudanteID, payload.InscricaoID)

	estudanteID := payload.EstudanteID
	if estudanteID == uuid.Nil {
		estudanteID = event.AggregateID
	}

	safeCodAcad := db.SafeString(payload.CodigoAcademia)
	queryAcad := fmt.Sprintf(`SELECT id FROM projection_academias WHERE codigo_academia = '%s'`, safeCodAcad)
	log.Printf("[DEBUG] InscricoesProjection.handleInscricaoAprovada - Query academia: %s", queryAcad)
	
	var academiaID uuid.UUID
	err := ip.client.DB().QueryRow(queryAcad).Scan(&academiaID)
	if err != nil {
		log.Printf("[ERROR] InscricoesProjection.handleInscricaoAprovada - Academia não encontrada: %v", err)
		return nil
	}

	safeTipo := db.SafeString(payload.Tipo)
	query := fmt.Sprintf(`
		UPDATE projection_inscricoes
		SET status = 'aprovado', updated_at = CURRENT_TIMESTAMP
		WHERE estudante_id = '%s' AND academia_id = '%s' AND status = 'espera' AND tipo = '%s'`,
		estudanteID, academiaID, safeTipo)

	log.Printf("[DEBUG] InscricoesProjection.handleInscricaoAprovada - Update query: %s", query)

	ip.client.DB().Exec(query)
	log.Printf("[DEBUG] InscricoesProjection.handleInscricaoAprovada - Inscrição aprovada")
	return nil
}

func (ip *InscricoesProjection) handleInscricaoReprovada(event db.Event) error {
	log.Printf("[DEBUG] InscricoesProjection.handleInscricaoReprovada - EventID: %s", event.EventID)
	
	var payload struct {
		EstudanteID    uuid.UUID `json:"EstudanteID"`
		CodigoAcademia string    `json:"CodigoAcademia"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		log.Printf("[ERROR] InscricoesProjection.handleInscricaoReprovada - Erro ao parsear payload: %v", err)
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	log.Printf("[DEBUG] InscricoesProjection.handleInscricaoReprovada - Payload: EstudanteID=%s", payload.EstudanteID)

	safeCodAcad := db.SafeString(payload.CodigoAcademia)
	queryAcad := fmt.Sprintf(`SELECT id FROM projection_academias WHERE codigo_academia = '%s'`, safeCodAcad)
	log.Printf("[DEBUG] InscricoesProjection.handleInscricaoReprovada - Query academia: %s", queryAcad)
	
	var academiaID uuid.UUID
	err := ip.client.DB().QueryRow(queryAcad).Scan(&academiaID)
	if err != nil {
		log.Printf("[ERROR] InscricoesProjection.handleInscricaoReprovada - Academia não encontrada: %v", err)
		return nil
	}

	query := fmt.Sprintf(`
		UPDATE projection_inscricoes
		SET status = 'reprovado', updated_at = CURRENT_TIMESTAMP
		WHERE estudante_id = '%s' AND academia_id = '%s' AND status = 'espera'`,
		payload.EstudanteID, academiaID)

	log.Printf("[DEBUG] InscricoesProjection.handleInscricaoReprovada - Update query: %s", query)

	ip.client.DB().Exec(query)
	log.Printf("[DEBUG] InscricoesProjection.handleInscricaoReprovada - Inscrição reprovada")
	return nil
}

func (ip *InscricoesProjection) handleEstudanteVinculado(event db.Event) error {
	log.Printf("[DEBUG] InscricoesProjection.handleEstudanteVinculado - EventID: %s", event.EventID)
	
	var payload struct {
		InscricaoID uuid.UUID `json:"InscricaoID"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		log.Printf("[ERROR] InscricoesProjection.handleEstudanteVinculado - Erro ao parsear payload: %v", err)
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	log.Printf("[DEBUG] InscricoesProjection.handleEstudanteVinculado - Payload: InscricaoID=%s", payload.InscricaoID)

	query := fmt.Sprintf(`
		UPDATE projection_inscricoes SET status_usado = TRUE, updated_at = CURRENT_TIMESTAMP 
		WHERE id = '%s'`, payload.InscricaoID)

	log.Printf("[DEBUG] InscricoesProjection.handleEstudanteVinculado - Update query: %s", query)

	_, err := ip.client.DB().Exec(query)
	
	if err != nil {
		log.Printf("[ERROR] InscricoesProjection.handleEstudanteVinculado - Erro: %v", err)
	} else {
		log.Printf("[DEBUG] InscricoesProjection.handleEstudanteVinculado - Estudante vinculado")
	}
	
	return err
}

func (ip *InscricoesProjection) GetByEstudante(estudanteID uuid.UUID) ([]InscricaoDTO, error) {
	if estudanteID == uuid.Nil {
		log.Printf("[ERROR] InscricoesProjection.GetByEstudante - UUID inválido")
		return nil, fmt.Errorf("UUID inválido")
	}

	query := fmt.Sprintf(`
		SELECT id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso, status, status_usado, created_at, updated_at, 
			event_id, version
		FROM projection_inscricoes WHERE estudante_id = '%s' ORDER BY created_at DESC
	`, estudanteID)

	log.Printf("[DEBUG] InscricoesProjection.GetByEstudante - Query: %s", query)

	rows, err := ip.client.DB().Query(query)
	if err != nil {
		log.Printf("[ERROR] InscricoesProjection.GetByEstudante - Erro: %v", err)
		return nil, err
	}
	defer rows.Close()

	var result []InscricaoDTO
	for rows.Next() {
		var dto InscricaoDTO
		err := rows.Scan(&dto.ID, &dto.EstudanteID, &dto.CodigoEstudante, &dto.AcademiaID,
			&dto.CodigoAcademia, &dto.Tipo, &dto.AnoInscricao, &dto.Curso, &dto.Status,
			&dto.StatusUsado, &dto.CreatedAt, &dto.UpdatedAt, &dto.EventID, &dto.Version)
		if err != nil {
			log.Printf("[ERROR] InscricoesProjection.GetByEstudante - Erro ao fazer scan: %v", err)
			continue
		}
		result = append(result, dto)
	}
	
	log.Printf("[DEBUG] InscricoesProjection.GetByEstudante - %d inscrições encontradas", len(result))
	return result, rows.Err()
}

func (ip *InscricoesProjection) GetByAcademia(academiaID uuid.UUID, status string) ([]InscricaoDTO, error) {
	if academiaID == uuid.Nil {
		log.Printf("[ERROR] InscricoesProjection.GetByAcademia - UUID inválido")
		return nil, fmt.Errorf("UUID inválido")
	}

	safeStatus := db.SafeString(status)

	query := fmt.Sprintf(`
		SELECT id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso, status, status_usado, created_at, updated_at, 
			event_id, version
		FROM projection_inscricoes WHERE academia_id = '%s' AND status = '%s' ORDER BY created_at DESC
	`, academiaID, safeStatus)

	log.Printf("[DEBUG] InscricoesProjection.GetByAcademia - Query: %s", query)

	rows, err := ip.client.DB().Query(query)
	if err != nil {
		log.Printf("[ERROR] InscricoesProjection.GetByAcademia - Erro: %v", err)
		return nil, err
	}
	defer rows.Close()

	var result []InscricaoDTO
	for rows.Next() {
		var dto InscricaoDTO
		err := rows.Scan(&dto.ID, &dto.EstudanteID, &dto.CodigoEstudante, &dto.AcademiaID,
			&dto.CodigoAcademia, &dto.Tipo, &dto.AnoInscricao, &dto.Curso, &dto.Status,
			&dto.StatusUsado, &dto.CreatedAt, &dto.UpdatedAt, &dto.EventID, &dto.Version)
		if err != nil {
			log.Printf("[ERROR] InscricoesProjection.GetByAcademia - Erro ao fazer scan: %v", err)
			continue
		}
		result = append(result, dto)
	}
	
	log.Printf("[DEBUG] InscricoesProjection.GetByAcademia - %d inscrições encontradas", len(result))
	return result, rows.Err()
}

func (ip *InscricoesProjection) GetByID(id uuid.UUID) (*InscricaoDTO, error) {
	if id == uuid.Nil {
		log.Printf("[ERROR] InscricoesProjection.GetByID - UUID inválido")
		return nil, fmt.Errorf("UUID inválido")
	}

	query := fmt.Sprintf(`
		SELECT id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso, status, status_usado, created_at, updated_at, 
			event_id, version
		FROM projection_inscricoes WHERE id = '%s'`, id)

	log.Printf("[DEBUG] InscricoesProjection.GetByID - Query: %s", query)

	var dto InscricaoDTO
	err := ip.client.DB().QueryRow(query).Scan(
		&dto.ID, &dto.EstudanteID, &dto.CodigoEstudante, &dto.AcademiaID,
		&dto.CodigoAcademia, &dto.Tipo, &dto.AnoInscricao, &dto.Curso, &dto.Status,
		&dto.StatusUsado, &dto.CreatedAt, &dto.UpdatedAt, &dto.EventID, &dto.Version,
	)
	
	if err == sql.ErrNoRows {
		log.Printf("[DEBUG] InscricoesProjection.GetByID - Inscrição não encontrada")
		return nil, nil
	}
	if err != nil {
		log.Printf("[ERROR] InscricoesProjection.GetByID - Erro: %v", err)
		return nil, err
	}
	
	log.Printf("[DEBUG] InscricoesProjection.GetByID - Inscrição encontrada")
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