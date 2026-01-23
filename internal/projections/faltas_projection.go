package projections

import (
	"database/sql"
	"encoding/json"
	"fmt"
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
	return p.handleFaltasRegistradas(event)
}

// ✅ CORRIGIDO: Query() + loop manual
func (p *FaltasProjection) Rebuild() error {
	if err := p.clear(); err != nil {
		return err
	}

	query := `
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger WHERE event_type = 'FaltasRegistradas' ORDER BY id ASC
	`
	
	rows, err := p.client.DB().Query(query)
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

func (p *FaltasProjection) GetLastProcessedEventID() (int64, error) {
	query := `
		SELECT last_processed_event_id 
		FROM projection_checkpoints 
		WHERE projection_name = $1
	`
	
	var lastID int64
	err := p.client.DB().QueryRow(query, p.Name()).Scan(&lastID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return lastID, err
}

func (p *FaltasProjection) UpdateCheckpoint(eventID int64) error {
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

func (p *FaltasProjection) clear() error {
	_, err := p.client.DB().Exec(`TRUNCATE TABLE projection_faltas CASCADE`)
	return err
}

func (p *FaltasProjection) handleFaltasRegistradas(event db.Event) error {
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
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	query := `
		INSERT INTO projection_faltas (
			codigo_estudante, codigo_academia, ano_lectivo, data,
			materia_disciplinar_id, quantidade, observacao, registered_at, event_id, version
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (codigo_estudante, codigo_academia, data, materia_disciplinar_id)
		DO UPDATE SET quantidade = EXCLUDED.quantidade, observacao = EXCLUDED.observacao,
			registered_at = EXCLUDED.registered_at, event_id = EXCLUDED.event_id, version = EXCLUDED.version
	`

	_, err := p.client.DB().Exec(query,
		payload.CodigoEstudante, payload.CodigoAcademia, payload.AnoLectivo, payload.Data,
		payload.MateriaDisciplinarID, payload.Quantidade, payload.Observacao, 
		payload.RegisteredAt, event.EventID, event.EventVersion)

	if err == nil {
		updateQuery := `
			UPDATE projection_estudantes SET total_faltas = (SELECT COALESCE(SUM(quantidade), 0) FROM projection_faltas WHERE codigo_estudante = $1)
			WHERE codigo_estudante = $1
		`
		p.client.DB().Exec(updateQuery, payload.CodigoEstudante)
	}
	return err
}

// ✅ CORRIGIDO: Query() + loop manual
func (p *FaltasProjection) GetByEstudante(codigoEstudante string) ([]FaltaDTO, error) {
	query := `
		SELECT f.id, f.codigo_estudante, f.codigo_academia, f.ano_lectivo, f.data,
			f.materia_disciplinar_id, m.nome as materia_nome, f.quantidade, f.observacao,
			f.registered_at, f.event_id, f.version
		FROM projection_faltas f
		LEFT JOIN projection_materias m ON f.materia_disciplinar_id::uuid = m.id
		WHERE f.codigo_estudante = $1 ORDER BY f.data DESC
	`
	
	rows, err := p.client.DB().Query(query, codigoEstudante)
	if err != nil {
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
			continue
		}
		result = append(result, dto)
	}
	return result, rows.Err()
}

// ✅ CORRIGIDO: Query() + loop manual
func (p *FaltasProjection) GetByPeriodo(codigoEstudante, anoLectivo string, dataInicio, dataFim time.Time) ([]FaltaDTO, error) {
	query := `
		SELECT f.id, f.codigo_estudante, f.codigo_academia, f.ano_lectivo, f.data,
			f.materia_disciplinar_id, m.nome as materia_nome, f.quantidade, f.observacao,
			f.registered_at, f.event_id, f.version
		FROM projection_faltas f
		LEFT JOIN projection_materias m ON f.materia_disciplinar_id::uuid = m.id
		WHERE f.codigo_estudante = $1 AND f.ano_lectivo = $2 
			AND f.data BETWEEN $3 AND $4
		ORDER BY f.data DESC
	`
	
	rows, err := p.client.DB().Query(query, codigoEstudante, anoLectivo, dataInicio, dataFim)
	if err != nil {
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
			continue
		}
		result = append(result, dto)
	}
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