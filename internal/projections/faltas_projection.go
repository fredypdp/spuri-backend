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

type FaltasProjection struct {
	client *db.Client
}

func NewFaltasProjection(client *db.Client) *FaltasProjection {
	return &FaltasProjection{client: client}
}

func (p *FaltasProjection) Name() string { return "faltas" }

func (p *FaltasProjection) Handle(event db.Event) error {
	log.Printf("[DEBUG] FaltasProjection.Handle - EventType: %s, EventID: %s", event.EventType, event.EventID)
	
	if event.EventType != "FaltasRegistradas" {
		log.Printf("[DEBUG] FaltasProjection.Handle - Evento ignorado, tipo: %s", event.EventType)
		return nil
	}
	return p.handleFaltasRegistradas(event)
}

func (p *FaltasProjection) Rebuild() error {
	log.Printf("[DEBUG] FaltasProjection.Rebuild - Iniciando rebuild")
	
	if err := p.clear(); err != nil {
		log.Printf("[ERROR] FaltasProjection.Rebuild - Erro ao limpar tabela: %v", err)
		return err
	}

	query := `
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger WHERE event_type = 'FaltasRegistradas' ORDER BY id ASC
	`
	
	log.Printf("[DEBUG] FaltasProjection.Rebuild - Query: %s", query)
	
	rows, err := p.client.DB().Query(query)
	if err != nil {
		log.Printf("[ERROR] FaltasProjection.Rebuild - Erro ao buscar eventos: %v", err)
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
			log.Printf("[ERROR] FaltasProjection.Rebuild - Erro ao fazer scan do evento: %v", err)
			return err
		}
		if err := p.Handle(event); err != nil {
			log.Printf("[ERROR] FaltasProjection.Rebuild - Erro ao processar evento %d: %v", event.ID, err)
			return fmt.Errorf("erro ao processar evento %d: %w", event.ID, err)
		}
		count++
	}
	
	log.Printf("[DEBUG] FaltasProjection.Rebuild - Rebuild concluído, %d eventos processados", count)
	return rows.Err()
}

func (p *FaltasProjection) GetLastProcessedEventID() (int64, error) {
	safeName := db.SafeString(p.Name())
	
	query := fmt.Sprintf(`
		SELECT last_processed_event_id 
		FROM projection_checkpoints 
		WHERE projection_name = '%s'
	`, safeName)
	
	log.Printf("[DEBUG] FaltasProjection.GetLastProcessedEventID - Query: %s", query)
	
	var lastID int64
	err := p.client.DB().QueryRow(query).Scan(&lastID)
	if err == sql.ErrNoRows {
		log.Printf("[DEBUG] FaltasProjection.GetLastProcessedEventID - Nenhum checkpoint encontrado")
		return 0, nil
	}
	
	if err != nil {
		log.Printf("[ERROR] FaltasProjection.GetLastProcessedEventID - Erro: %v", err)
	} else {
		log.Printf("[DEBUG] FaltasProjection.GetLastProcessedEventID - LastID: %d", lastID)
	}
	
	return lastID, err
}

func (p *FaltasProjection) UpdateCheckpoint(eventID int64) error {
	safeName := db.SafeString(p.Name())
	eventID = int64(db.ValidateOffset(int(eventID)))
	
	query := fmt.Sprintf(`
		INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at, events_processed)
		VALUES ('%s', %d, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) DO UPDATE SET
			last_processed_event_id = %d, last_processed_at = CURRENT_TIMESTAMP,
			events_processed = projection_checkpoints.events_processed + 1
	`, safeName, eventID, eventID)
	
	log.Printf("[DEBUG] FaltasProjection.UpdateCheckpoint - Query: %s", query)
	
	_, err := p.client.DB().Exec(query)
	
	if err != nil {
		log.Printf("[ERROR] FaltasProjection.UpdateCheckpoint - Erro: %v", err)
	} else {
		log.Printf("[DEBUG] FaltasProjection.UpdateCheckpoint - Checkpoint atualizado para eventID: %d", eventID)
	}
	
	return err
}

func (p *FaltasProjection) clear() error {
	log.Printf("[DEBUG] FaltasProjection.clear - Limpando tabela projection_faltas")
	_, err := p.client.DB().Exec(`TRUNCATE TABLE projection_faltas CASCADE`)
	if err != nil {
		log.Printf("[ERROR] FaltasProjection.clear - Erro: %v", err)
	}
	return err
}

func (p *FaltasProjection) handleFaltasRegistradas(event db.Event) error {
	log.Printf("[DEBUG] FaltasProjection.handleFaltasRegistradas - EventID: %s", event.EventID)
	
	var payload struct {
		CodigoEstudante      string    `json:"CodigoEstudante"`
		CodigoAcademia       string    `json:"CodigoAcademia"`
		AnoLectivo           string    `json:"AnoLectivo"`
		Data                 time.Time `json:"Data"`
		MateriaDisciplinarID string    `json:"MateriaDisciplinarID"`
		Quantidade           int       `json:"Quantidade"`
		Observacao           *string   `json:"Observacao"`
		RegisteredAt         time.Time `json:"RegisteredAt"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		log.Printf("[ERROR] FaltasProjection.handleFaltasRegistradas - Erro ao parsear payload: %v", err)
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	log.Printf("[DEBUG] FaltasProjection.handleFaltasRegistradas - Payload: CodigoEstudante=%s, Quantidade=%d", 
		payload.CodigoEstudante, payload.Quantidade)

	safeCodEst := db.SafeString(payload.CodigoEstudante)
	safeCodAcad := db.SafeString(payload.CodigoAcademia)
	safeAno := db.SafeString(payload.AnoLectivo)
	safeMatID := db.SafeString(payload.MateriaDisciplinarID)

	var obsStr string
	if payload.Observacao != nil {
		obsStr = fmt.Sprintf("'%s'", db.SafeString(*payload.Observacao))
	} else {
		obsStr = "NULL"
	}

	query := fmt.Sprintf(`
		INSERT INTO projection_faltas (
			codigo_estudante, codigo_academia, ano_lectivo, data,
			materia_disciplinar_id, quantidade, observacao, registered_at, event_id, version
		) VALUES ('%s', '%s', '%s', '%s', '%s', %d, %s, '%s', '%s', %d)
		ON CONFLICT (codigo_estudante, codigo_academia, data, materia_disciplinar_id)
		DO UPDATE SET quantidade = EXCLUDED.quantidade, observacao = EXCLUDED.observacao,
			registered_at = EXCLUDED.registered_at, event_id = EXCLUDED.event_id, version = EXCLUDED.version
	`, safeCodEst, safeCodAcad, safeAno, payload.Data.Format(time.RFC3339),
		safeMatID, payload.Quantidade, obsStr,
		payload.RegisteredAt.Format(time.RFC3339), event.EventID, event.EventVersion)

	log.Printf("[DEBUG] FaltasProjection.handleFaltasRegistradas - Insert query: %s", query)

	_, err := p.client.DB().Exec(query)

	if err != nil {
		log.Printf("[ERROR] FaltasProjection.handleFaltasRegistradas - Erro ao inserir falta: %v", err)
	} else {
		log.Printf("[DEBUG] FaltasProjection.handleFaltasRegistradas - Falta inserida com sucesso")
		
		updateQuery := fmt.Sprintf(`
			UPDATE projection_estudantes SET total_faltas = (SELECT COALESCE(SUM(quantidade), 0) FROM projection_faltas WHERE codigo_estudante = '%s')
			WHERE codigo_estudante = '%s'
		`, safeCodEst, safeCodEst)
		
		log.Printf("[DEBUG] FaltasProjection.handleFaltasRegistradas - Update estudante query: %s", updateQuery)
		p.client.DB().Exec(updateQuery)
	}
	
	return err
}

func (p *FaltasProjection) GetByEstudante(codigoEstudante string) ([]FaltaDTO, error) {
	safeCodigo := db.SafeString(codigoEstudante)

	query := fmt.Sprintf(`
		SELECT f.id, f.codigo_estudante, f.codigo_academia, f.ano_lectivo, f.data,
			f.materia_disciplinar_id, m.nome as materia_nome, f.quantidade, f.observacao,
			f.registered_at, f.event_id, f.version
		FROM projection_faltas f
		LEFT JOIN projection_materias m ON f.materia_disciplinar_id::uuid = m.id
		WHERE f.codigo_estudante = '%s' ORDER BY f.data DESC
	`, safeCodigo)
	
	log.Printf("[DEBUG] FaltasProjection.GetByEstudante - Query: %s", query)
	
	rows, err := p.client.DB().Query(query)
	if err != nil {
		log.Printf("[ERROR] FaltasProjection.GetByEstudante - Erro: %v", err)
		return nil, err
	}
	defer rows.Close()

	var result []FaltaDTO
	for rows.Next() {
		var dto FaltaDTO
		err := rows.Scan(&dto.ID, &dto.CodigoEstudante, &dto.CodigoAcademia, &dto.AnoLectivo,
			&dto.Data, &dto.MateriaDisciplinarID, &dto.MateriaNome, &dto.Quantidade,
			&dto.Observacao, &dto.RegisteredAt, &dto.EventID, &dto.Version)
		if err != nil {
			log.Printf("[ERROR] FaltasProjection.GetByEstudante - Erro ao fazer scan: %v", err)
			continue
		}
		result = append(result, dto)
	}
	
	log.Printf("[DEBUG] FaltasProjection.GetByEstudante - %d faltas encontradas", len(result))
	return result, rows.Err()
}

func (p *FaltasProjection) GetByPeriodo(codigoEstudante, anoLectivo string, dataInicio, dataFim time.Time) ([]FaltaDTO, error) {
	safeCodigo := db.SafeString(codigoEstudante)
	safeAno := db.SafeString(anoLectivo)

	query := fmt.Sprintf(`
		SELECT f.id, f.codigo_estudante, f.codigo_academia, f.ano_lectivo, f.data,
			f.materia_disciplinar_id, m.nome as materia_nome, f.quantidade, f.observacao,
			f.registered_at, f.event_id, f.version
		FROM projection_faltas f
		LEFT JOIN projection_materias m ON f.materia_disciplinar_id::uuid = m.id
		WHERE f.codigo_estudante = '%s' AND f.ano_lectivo = '%s' 
			AND f.data BETWEEN '%s' AND '%s'
		ORDER BY f.data DESC
	`, safeCodigo, safeAno, dataInicio.Format(time.RFC3339), dataFim.Format(time.RFC3339))
	
	log.Printf("[DEBUG] FaltasProjection.GetByPeriodo - Query: %s", query)
	
	rows, err := p.client.DB().Query(query)
	if err != nil {
		log.Printf("[ERROR] FaltasProjection.GetByPeriodo - Erro: %v", err)
		return nil, err
	}
	defer rows.Close()

	var result []FaltaDTO
	for rows.Next() {
		var dto FaltaDTO
		err := rows.Scan(&dto.ID, &dto.CodigoEstudante, &dto.CodigoAcademia, &dto.AnoLectivo,
			&dto.Data, &dto.MateriaDisciplinarID, &dto.MateriaNome, &dto.Quantidade,
			&dto.Observacao, &dto.RegisteredAt, &dto.EventID, &dto.Version)
		if err != nil {
			log.Printf("[ERROR] FaltasProjection.GetByPeriodo - Erro ao fazer scan: %v", err)
			continue
		}
		result = append(result, dto)
	}
	
	log.Printf("[DEBUG] FaltasProjection.GetByPeriodo - %d faltas encontradas", len(result))
	return result, rows.Err()
}

type FaltaDTO struct {
	ID                   uuid.UUID `json:"id"`
	CodigoEstudante      string    `json:"codigo_estudante"`
	CodigoAcademia       string    `json:"codigo_academia"`
	AnoLectivo           string    `json:"ano_lectivo"`
	Data                 time.Time `json:"data"`
	MateriaDisciplinarID uuid.UUID `json:"materia_disciplinar_id"`
	MateriaNome          string    `json:"materia_nome"`
	Quantidade           int       `json:"quantidade"`
	Observacao           *string   `json:"observacao,omitempty"`
	RegisteredAt         time.Time `json:"registered_at"`
	EventID              uuid.UUID `json:"event_id"`
	Version              int       `json:"version"`
}