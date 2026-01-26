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
	if event.EventType != "FaltasRegistradas" {
		return nil
	}
	log.Printf("[DEBUG] Processando FaltasRegistradas: %s", event.EventID)
	return p.handleFaltasRegistradas(event)
}

func (p *FaltasProjection) Rebuild() error {
	log.Printf("[DEBUG] Rebuild iniciado")
	
	if err := p.clear(); err != nil {
		return fmt.Errorf("falha ao limpar: %w", err)
	}

	rows, err := p.client.DB().Query(`
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger WHERE event_type = 'FaltasRegistradas' ORDER BY id ASC
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

func (p *FaltasProjection) GetLastProcessedEventID() (int64, error) {
	var lastID int64
	query := fmt.Sprintf(`SELECT last_processed_event_id FROM projection_checkpoints WHERE projection_name = '%s'`,
		db.SafeString(p.Name()))
	
	err := p.client.DB().QueryRow(query).Scan(&lastID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return lastID, err
}

func (p *FaltasProjection) UpdateCheckpoint(eventID int64) error {
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

func (p *FaltasProjection) clear() error {
	_, err := p.client.DB().Exec(`TRUNCATE TABLE projection_faltas CASCADE`)
	return err
}

func (p *FaltasProjection) handleFaltasRegistradas(event db.Event) error {
	var payload struct {
		CodigoEstudante, CodigoAcademia, AnoLectivo, MateriaDisciplinarID string
		Data, RegisteredAt                                                time.Time
		Quantidade                                                        int
		Observacao                                                        *string
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error: %w", err)
	}

	log.Printf("[DEBUG] Registrando %d faltas para estudante %s", 
		payload.Quantidade, payload.CodigoEstudante)

	query := fmt.Sprintf(`
		INSERT INTO projection_faltas (
			codigo_estudante, codigo_academia, ano_lectivo, data,
			materia_disciplinar_id, quantidade, observacao, registered_at, event_id, version
		) VALUES ('%s', '%s', '%s', '%s', '%s', %d, %s, '%s', '%s', %d)
		ON CONFLICT (codigo_estudante, codigo_academia, data, materia_disciplinar_id)
		DO UPDATE SET quantidade = EXCLUDED.quantidade, observacao = EXCLUDED.observacao,
			registered_at = EXCLUDED.registered_at, event_id = EXCLUDED.event_id, version = EXCLUDED.version
	`, db.SafeString(payload.CodigoEstudante), db.SafeString(payload.CodigoAcademia),
		db.SafeString(payload.AnoLectivo), payload.Data.Format(time.RFC3339),
		db.SafeString(payload.MateriaDisciplinarID), payload.Quantidade,
		nullOrString(payload.Observacao), payload.RegisteredAt.Format(time.RFC3339),
		event.EventID, event.EventVersion)

	if _, err := p.client.DB().Exec(query); err != nil {
		return err
	}

	updateQuery := fmt.Sprintf(`
		UPDATE projection_estudantes
		SET total_faltas = (SELECT COALESCE(SUM(quantidade), 0) FROM projection_faltas WHERE codigo_estudante = '%s')
		WHERE codigo_estudante = '%s'
	`, db.SafeString(payload.CodigoEstudante), db.SafeString(payload.CodigoEstudante))
	
	p.client.DB().Exec(updateQuery)
	return nil
}

func (p *FaltasProjection) GetByEstudante(codigoEstudante string) ([]FaltaDTO, error) {
	return p.queryFaltas(fmt.Sprintf(
		`f.codigo_estudante = '%s' ORDER BY f.data DESC`,
		db.SafeString(codigoEstudante)))
}

func (p *FaltasProjection) GetByPeriodo(codigoEstudante, anoLectivo string, dataInicio, dataFim time.Time) ([]FaltaDTO, error) {
	return p.queryFaltas(fmt.Sprintf(
		`f.codigo_estudante = '%s' AND f.ano_lectivo = '%s' AND f.data BETWEEN '%s' AND '%s' ORDER BY f.data DESC`,
		db.SafeString(codigoEstudante), db.SafeString(anoLectivo),
		dataInicio.Format(time.RFC3339), dataFim.Format(time.RFC3339)))
}

func (p *FaltasProjection) queryFaltas(whereClause string) ([]FaltaDTO, error) {
	query := fmt.Sprintf(`
		SELECT f.id, f.codigo_estudante, f.codigo_academia, f.ano_lectivo, f.data,
			f.materia_disciplinar_id, COALESCE(m.nome, '') as materia_nome, f.quantidade,
			f.observacao, f.registered_at, f.event_id, f.version
		FROM projection_faltas f
		LEFT JOIN projection_materias m ON f.materia_disciplinar_id::uuid = m.id
		WHERE %s
	`, whereClause)
	
	rows, err := p.client.DB().Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []FaltaDTO
	for rows.Next() {
		var dto FaltaDTO
		if err := rows.Scan(&dto.ID, &dto.CodigoEstudante, &dto.CodigoAcademia, &dto.AnoLectivo,
			&dto.Data, &dto.MateriaDisciplinarID, &dto.MateriaNome, &dto.Quantidade,
			&dto.Observacao, &dto.RegisteredAt, &dto.EventID, &dto.Version); err != nil {
			continue
		}
		result = append(result, dto)
	}

	log.Printf("[DEBUG] %d faltas encontradas", len(result))
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