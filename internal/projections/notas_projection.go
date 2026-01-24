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

type NotasProjection struct {
	client *db.Client
}

func NewNotasProjection(client *db.Client) *NotasProjection {
	return &NotasProjection{client: client}
}

func (p *NotasProjection) Name() string { return "notas" }

func (p *NotasProjection) Handle(event db.Event) error {
	log.Printf("[DEBUG] NotasProjection.Handle - EventType: %s, EventID: %s", event.EventType, event.EventID)
	
	if event.EventType != "NotasRegistradas" {
		log.Printf("[DEBUG] NotasProjection.Handle - Evento ignorado, tipo: %s", event.EventType)
		return nil
	}
	return p.handleNotasRegistradas(event)
}

func (p *NotasProjection) Rebuild() error {
	log.Printf("[DEBUG] NotasProjection.Rebuild - Iniciando rebuild")
	
	if err := p.clear(); err != nil {
		log.Printf("[ERROR] NotasProjection.Rebuild - Erro ao limpar tabela: %v", err)
		return err
	}

	query := `
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger WHERE event_type = 'NotasRegistradas' ORDER BY id ASC
	`
	
	log.Printf("[DEBUG] NotasProjection.Rebuild - Query: %s", query)
	
	rows, err := p.client.DB().Query(query)
	if err != nil {
		log.Printf("[ERROR] NotasProjection.Rebuild - Erro ao buscar eventos: %v", err)
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
			log.Printf("[ERROR] NotasProjection.Rebuild - Erro ao fazer scan do evento: %v", err)
			return err
		}
		if err := p.Handle(event); err != nil {
			log.Printf("[ERROR] NotasProjection.Rebuild - Erro ao processar evento %d: %v", event.ID, err)
			return fmt.Errorf("erro ao processar evento %d: %w", event.ID, err)
		}
		count++
	}
	
	log.Printf("[DEBUG] NotasProjection.Rebuild - Rebuild concluído, %d eventos processados", count)
	return rows.Err()
}

func (p *NotasProjection) GetLastProcessedEventID() (int64, error) {
	safeName := db.SafeString(p.Name())
	
	query := fmt.Sprintf(`
		SELECT last_processed_event_id 
		FROM projection_checkpoints 
		WHERE projection_name = '%s'
	`, safeName)
	
	log.Printf("[DEBUG] NotasProjection.GetLastProcessedEventID - Query: %s", query)
	
	var lastID int64
	err := p.client.DB().QueryRow(query).Scan(&lastID)
	if err == sql.ErrNoRows {
		log.Printf("[DEBUG] NotasProjection.GetLastProcessedEventID - Nenhum checkpoint encontrado")
		return 0, nil
	}
	
	if err != nil {
		log.Printf("[ERROR] NotasProjection.GetLastProcessedEventID - Erro: %v", err)
	} else {
		log.Printf("[DEBUG] NotasProjection.GetLastProcessedEventID - LastID: %d", lastID)
	}
	
	return lastID, err
}

func (p *NotasProjection) UpdateCheckpoint(eventID int64) error {
	safeName := db.SafeString(p.Name())
	eventID = int64(db.ValidateOffset(int(eventID)))
	
	query := fmt.Sprintf(`
		INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at, events_processed)
		VALUES ('%s', %d, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) DO UPDATE SET
			last_processed_event_id = %d, last_processed_at = CURRENT_TIMESTAMP,
			events_processed = projection_checkpoints.events_processed + 1
	`, safeName, eventID, eventID)
	
	log.Printf("[DEBUG] NotasProjection.UpdateCheckpoint - Query: %s", query)
	
	_, err := p.client.DB().Exec(query)
	
	if err != nil {
		log.Printf("[ERROR] NotasProjection.UpdateCheckpoint - Erro: %v", err)
	} else {
		log.Printf("[DEBUG] NotasProjection.UpdateCheckpoint - Checkpoint atualizado para eventID: %d", eventID)
	}
	
	return err
}

func (p *NotasProjection) clear() error {
	log.Printf("[DEBUG] NotasProjection.clear - Limpando tabela projection_notas")
	_, err := p.client.DB().Exec(`TRUNCATE TABLE projection_notas CASCADE`)
	if err != nil {
		log.Printf("[ERROR] NotasProjection.clear - Erro: %v", err)
	}
	return err
}

func (p *NotasProjection) handleNotasRegistradas(event db.Event) error {
	log.Printf("[DEBUG] NotasProjection.handleNotasRegistradas - EventID: %s", event.EventID)
	
	var payload struct {
		CodigoEstudante      string    `json:"CodigoEstudante"`
		CodigoAcademia       string    `json:"CodigoAcademia"`
		AnoLectivo           string    `json:"AnoLectivo"`
		Periodo              string    `json:"Periodo"`
		MateriaDisciplinarID string    `json:"MateriaDisciplinarID"`
		Nota                 float64   `json:"Nota"`
		Observacao           *string   `json:"Observacao"`
		RegisteredAt         time.Time `json:"RegisteredAt"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		log.Printf("[ERROR] NotasProjection.handleNotasRegistradas - Erro ao parsear payload: %v", err)
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	log.Printf("[DEBUG] NotasProjection.handleNotasRegistradas - Payload: CodigoEstudante=%s, Nota=%.2f", 
		payload.CodigoEstudante, payload.Nota)

	safeCodEst := db.SafeString(payload.CodigoEstudante)
	safeCodAcad := db.SafeString(payload.CodigoAcademia)
	safeAno := db.SafeString(payload.AnoLectivo)
	safePer := db.SafeString(payload.Periodo)
	safeMatID := db.SafeString(payload.MateriaDisciplinarID)

	var obsStr string
	if payload.Observacao != nil {
		obsStr = fmt.Sprintf("'%s'", db.SafeString(*payload.Observacao))
	} else {
		obsStr = "NULL"
	}

	query := fmt.Sprintf(`
		INSERT INTO projection_notas (
			codigo_estudante, codigo_academia, ano_lectivo, periodo,
			materia_disciplinar_id, nota, observacao, registered_at, event_id, version
		) VALUES ('%s', '%s', '%s', '%s', '%s', %f, %s, '%s', '%s', %d)
		ON CONFLICT (codigo_estudante, codigo_academia, ano_lectivo, periodo, materia_disciplinar_id)
		DO UPDATE SET nota = EXCLUDED.nota, observacao = EXCLUDED.observacao,
			registered_at = EXCLUDED.registered_at, event_id = EXCLUDED.event_id, version = EXCLUDED.version
	`, safeCodEst, safeCodAcad, safeAno, safePer, safeMatID, payload.Nota, obsStr,
		payload.RegisteredAt.Format(time.RFC3339), event.EventID, event.EventVersion)

	log.Printf("[DEBUG] NotasProjection.handleNotasRegistradas - Insert query: %s", query)

	_, err := p.client.DB().Exec(query)

	if err != nil {
		log.Printf("[ERROR] NotasProjection.handleNotasRegistradas - Erro ao inserir nota: %v", err)
	} else {
		log.Printf("[DEBUG] NotasProjection.handleNotasRegistradas - Nota inserida com sucesso")
		
		updateQuery := fmt.Sprintf(`
			UPDATE projection_estudantes SET total_notas = (SELECT COUNT(*) FROM projection_notas WHERE codigo_estudante = '%s')
			WHERE codigo_estudante = '%s'
		`, safeCodEst, safeCodEst)
		
		log.Printf("[DEBUG] NotasProjection.handleNotasRegistradas - Update estudante query: %s", updateQuery)
		p.client.DB().Exec(updateQuery)
	}
	
	return err
}

func (p *NotasProjection) GetByEstudante(codigoEstudante string) ([]NotaDTO, error) {
	safeCodigo := db.SafeString(codigoEstudante)

	query := fmt.Sprintf(`
		SELECT n.id, n.codigo_estudante, n.codigo_academia, n.ano_lectivo, n.periodo,
			n.materia_disciplinar_id, m.nome as materia_nome, n.nota, n.observacao,
			n.registered_at, n.event_id, n.version
		FROM projection_notas n
		LEFT JOIN projection_materias m ON n.materia_disciplinar_id::uuid = m.id
		WHERE n.codigo_estudante = '%s' ORDER BY n.registered_at DESC
	`, safeCodigo)
	
	log.Printf("[DEBUG] NotasProjection.GetByEstudante - Query: %s", query)
	
	rows, err := p.client.DB().Query(query)
	if err != nil {
		log.Printf("[ERROR] NotasProjection.GetByEstudante - Erro: %v", err)
		return nil, err
	}
	defer rows.Close()

	var result []NotaDTO
	for rows.Next() {
		var dto NotaDTO
		err := rows.Scan(&dto.ID, &dto.CodigoEstudante, &dto.CodigoAcademia, &dto.AnoLectivo,
			&dto.Periodo, &dto.MateriaDisciplinarID, &dto.MateriaNome, &dto.Nota,
			&dto.Observacao, &dto.RegisteredAt, &dto.EventID, &dto.Version)
		if err != nil {
			log.Printf("[ERROR] NotasProjection.GetByEstudante - Erro ao fazer scan: %v", err)
			continue
		}
		result = append(result, dto)
	}
	
	log.Printf("[DEBUG] NotasProjection.GetByEstudante - %d notas encontradas", len(result))
	return result, rows.Err()
}

func (p *NotasProjection) GetByPeriodo(codigoEstudante, anoLectivo, periodo string) ([]NotaDTO, error) {
	safeCodigo := db.SafeString(codigoEstudante)
	safeAno := db.SafeString(anoLectivo)
	safePer := db.SafeString(periodo)

	query := fmt.Sprintf(`
		SELECT n.id, n.codigo_estudante, n.codigo_academia, n.ano_lectivo, n.periodo,
			n.materia_disciplinar_id, m.nome as materia_nome, n.nota, n.observacao,
			n.registered_at, n.event_id, n.version
		FROM projection_notas n
		LEFT JOIN projection_materias m ON n.materia_disciplinar_id::uuid = m.id
		WHERE n.codigo_estudante = '%s' AND n.ano_lectivo = '%s' AND n.periodo = '%s'
		ORDER BY m.nome
	`, safeCodigo, safeAno, safePer)
	
	log.Printf("[DEBUG] NotasProjection.GetByPeriodo - Query: %s", query)
	
	rows, err := p.client.DB().Query(query)
	if err != nil {
		log.Printf("[ERROR] NotasProjection.GetByPeriodo - Erro: %v", err)
		return nil, err
	}
	defer rows.Close()

	var result []NotaDTO
	for rows.Next() {
		var dto NotaDTO
		err := rows.Scan(&dto.ID, &dto.CodigoEstudante, &dto.CodigoAcademia, &dto.AnoLectivo,
			&dto.Periodo, &dto.MateriaDisciplinarID, &dto.MateriaNome, &dto.Nota,
			&dto.Observacao, &dto.RegisteredAt, &dto.EventID, &dto.Version)
		if err != nil {
			log.Printf("[ERROR] NotasProjection.GetByPeriodo - Erro ao fazer scan: %v", err)
			continue
		}
		result = append(result, dto)
	}
	
	log.Printf("[DEBUG] NotasProjection.GetByPeriodo - %d notas encontradas", len(result))
	return result, rows.Err()
}

type NotaDTO struct {
	ID                   uuid.UUID `json:"id"`
	CodigoEstudante      string    `json:"codigo_estudante"`
	CodigoAcademia       string    `json:"codigo_academia"`
	AnoLectivo           string    `json:"ano_lectivo"`
	Periodo              string    `json:"periodo"`
	MateriaDisciplinarID uuid.UUID `json:"materia_disciplinar_id"`
	MateriaNome          string    `json:"materia_nome"`
	Nota                 float64   `json:"nota"`
	Observacao           *string   `json:"observacao,omitempty"`
	RegisteredAt         time.Time `json:"registered_at"`
	EventID              uuid.UUID `json:"event_id"`
	Version              int       `json:"version"`
}