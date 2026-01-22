package projections

import (
	"database/sql"
	"encoding/json"
	"fmt"
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
	if event.EventType != "NotasRegistradas" {
		return nil
	}
	return p.handleNotasRegistradas(event)
}

func (p *NotasProjection) Rebuild() error {
	if err := p.clear(); err != nil {
		return err
	}

	query := `
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger WHERE event_type = 'NotasRegistradas' ORDER BY id ASC
	`
	
	rows, err := p.client.DB().Queryx(query)
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
		if err := p.Handle(event); err != nil {
			return fmt.Errorf("erro ao processar evento %d: %w", event.ID, err)
		}
	}
	return rows.Err()
}

func (p *NotasProjection) GetLastProcessedEventID() (int64, error) {
	var lastID int64
	query := `
		SELECT last_processed_event_id 
		FROM projection_checkpoints 
		WHERE projection_name = $1
	`
	err := p.client.DB().Get(&lastID, query, p.Name())
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return lastID, err
}

func (p *NotasProjection) UpdateCheckpoint(eventID int64) error {
	query := `
		INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at, events_processed)
		VALUES ($1, $2, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) DO UPDATE SET
			last_processed_event_id = $2, last_processed_at = CURRENT_TIMESTAMP,
			events_processed = projection_checkpoints.events_processed + 1
	`
	_, err := p.client.DB().Exec(query, p.Name(), eventID)
	return err
}

func (p *NotasProjection) clear() error {
	_, err := p.client.DB().Exec(`TRUNCATE TABLE projection_notas CASCADE`)
	return err
}

func (p *NotasProjection) handleNotasRegistradas(event db.Event) error {
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
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	query := `
		INSERT INTO projection_notas (
			codigo_estudante, codigo_academia, ano_lectivo, periodo,
			materia_disciplinar_id, nota, observacao, registered_at, event_id, version
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (codigo_estudante, codigo_academia, ano_lectivo, periodo, materia_disciplinar_id)
		DO UPDATE SET nota = EXCLUDED.nota, observacao = EXCLUDED.observacao,
			registered_at = EXCLUDED.registered_at, event_id = EXCLUDED.event_id, version = EXCLUDED.version
	`
	
	_, err := p.client.DB().Exec(query,
		payload.CodigoEstudante, payload.CodigoAcademia, payload.AnoLectivo, payload.Periodo,
		payload.MateriaDisciplinarID, payload.Nota, payload.Observacao, payload.RegisteredAt,
		event.EventID, event.EventVersion)

	if err == nil {
		p.client.DB().Exec(`
			UPDATE projection_estudantes SET total_notas = (SELECT COUNT(*) FROM projection_notas WHERE codigo_estudante = $1)
			WHERE codigo_estudante = $1
		`, payload.CodigoEstudante)
	}
	return err
}

func (p *NotasProjection) GetByEstudante(codigoEstudante string) ([]NotaDTO, error) {
	query := `
		SELECT n.id, n.codigo_estudante, n.codigo_academia, n.ano_lectivo, n.periodo,
			n.materia_disciplinar_id, m.nome as materia_nome, n.nota, n.observacao,
			n.registered_at, n.event_id, n.version
		FROM projection_notas n
		LEFT JOIN projection_materias m ON n.materia_disciplinar_id = m.id
		WHERE n.codigo_estudante = $1 ORDER BY n.registered_at DESC
	`
	
	rows, err := p.client.DB().Queryx(query, codigoEstudante)
	if err != nil {
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
			return nil, err
		}
		result = append(result, dto)
	}
	return result, rows.Err()
}

func (p *NotasProjection) GetByPeriodo(codigoEstudante, anoLectivo, periodo string) ([]NotaDTO, error) {
	query := `
		SELECT n.id, n.codigo_estudante, n.codigo_academia, n.ano_lectivo, n.periodo,
			n.materia_disciplinar_id, m.nome as materia_nome, n.nota, n.observacao,
			n.registered_at, n.event_id, n.version
		FROM projection_notas n
		LEFT JOIN projection_materias m ON n.materia_disciplinar_id = m.id
		WHERE n.codigo_estudante = $1 AND n.ano_lectivo = $2 AND n.periodo = $3
		ORDER BY m.nome
	`
	
	rows, err := p.client.DB().Queryx(query, codigoEstudante, anoLectivo, periodo)
	if err != nil {
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
			return nil, err
		}
		result = append(result, dto)
	}
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