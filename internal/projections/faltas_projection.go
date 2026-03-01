package projections

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"spuri/internal/db"
	"time"
)

type FaltasProjection struct {
	client *db.Client
}

func NewFaltasProjection(client *db.Client) *FaltasProjection {
	return &FaltasProjection{client: client}
}

func (p *FaltasProjection) Name() string { return "faltas" }

// ============================================================================
// Interface Projection
// ============================================================================

func (p *FaltasProjection) GetLastProcessedEventID() (int64, error) {
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

func (p *FaltasProjection) UpdateCheckpoint(eventID int64) error {
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

func (p *FaltasProjection) Handle(event db.Event) error {
	handlers := map[string]func(db.Event) error{
		"FaltasRegistradas": p.handleFaltasRegistradas,
	}
	if handler, ok := handlers[event.EventType]; ok {
		log.Printf("[DEBUG] [faltas] Processando %s: %s", event.EventType, event.EventID)
		return handler(event)
	}
	return nil
}

func (p *FaltasProjection) Rebuild() error {
	log.Printf("[DEBUG] [faltas] Rebuild iniciado")
	if err := p.clear(); err != nil {
		return fmt.Errorf("falha ao limpar: %w", err)
	}
	rows, err := p.client.DB().Query(`
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE event_type IN ('FaltasRegistradas')
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
	log.Printf("[DEBUG] [faltas] Rebuild concluído: %d eventos", count)
	return rows.Err()
}

func (p *FaltasProjection) clear() error {
	_, err := p.client.DB().Exec(`TRUNCATE TABLE projection_faltas CASCADE`)
	return err
}

// ============================================================================
// Handler de evento
// ============================================================================

func (p *FaltasProjection) handleFaltasRegistradas(event db.Event) error {
	var payload struct {
		CodigoEstudante      string    `json:"CodigoEstudante"`
		CodigoAcademia       string    `json:"CodigoAcademia"`
		AnoLectivo           string    `json:"AnoLectivo"`
		AnoAcademico         string    `json:"AnoAcademico"`
		Data                 time.Time `json:"Data"`
		MateriaDisciplinarID string    `json:"MateriaDisciplinarID"`
		Quantidade           int       `json:"Quantidade"`
		Observacao           *string   `json:"Observacao"`
		RegisteredAt         time.Time `json:"RegisteredAt"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error FaltasRegistradas: %w", err)
	}

	_, err := p.client.DB().Exec(`
		INSERT INTO projection_faltas (
			codigo_estudante, codigo_academia, ano_lectivo, ano_academico,
			data, materia_disciplinar_id, quantidade, observacao,
			registered_at, event_id, version
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
	`,
		payload.CodigoEstudante, payload.CodigoAcademia, payload.AnoLectivo, payload.AnoAcademico,
		payload.Data.Format("2006-01-02"), payload.MateriaDisciplinarID, payload.Quantidade, payload.Observacao,
		payload.RegisteredAt, event.EventID, event.EventVersion,
	)
	return err
}

// ============================================================================
// Queries de leitura
// ============================================================================

type FaltaDTO struct {
	ID                   string  `json:"id"`
	CodigoEstudante      string  `json:"codigo_estudante"`
	CodigoAcademia       string  `json:"codigo_academia"`
	AnoLectivo           string  `json:"ano_lectivo"`
	AnoAcademico         string  `json:"ano_academico"`
	Data                 string  `json:"data"`
	MateriaDisciplinarID string  `json:"materia_disciplinar_id"`
	MateriaNome          *string `json:"materia_nome,omitempty"`
	Quantidade           int     `json:"quantidade"`
	Observacao           *string `json:"observacao,omitempty"`
	RegisteredAt         string  `json:"registered_at"`
	EventID              string  `json:"event_id"`
	Version              int     `json:"version"`
}

func (p *FaltasProjection) GetByEstudante(codigoEstudante string) ([]FaltaDTO, error) {
	rows, err := p.client.DB().Query(`
		SELECT f.id, f.codigo_estudante, f.codigo_academia, f.ano_lectivo, f.ano_academico,
			f.data, f.materia_disciplinar_id, m.nome, f.quantidade, f.observacao,
			f.registered_at, f.event_id, f.version
		FROM projection_faltas f
		LEFT JOIN projection_materias m ON m.id::text = f.materia_disciplinar_id
		WHERE f.codigo_estudante = $1
		ORDER BY f.data DESC
	`, codigoEstudante)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFaltas(rows)
}

func (p *FaltasProjection) GetByAcademia(codigoAcademia string) ([]FaltaDTO, error) {
	rows, err := p.client.DB().Query(`
		SELECT f.id, f.codigo_estudante, f.codigo_academia, f.ano_lectivo, f.ano_academico,
			f.data, f.materia_disciplinar_id, m.nome, f.quantidade, f.observacao,
			f.registered_at, f.event_id, f.version
		FROM projection_faltas f
		LEFT JOIN projection_materias m ON m.id::text = f.materia_disciplinar_id
		WHERE f.codigo_academia = $1
		ORDER BY f.data DESC
	`, codigoAcademia)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFaltas(rows)
}

func (p *FaltasProjection) GetByPeriodo(codigoEstudante, anoLectivo string, dataInicio, dataFim time.Time) ([]FaltaDTO, error) {
	rows, err := p.client.DB().Query(`
		SELECT f.id, f.codigo_estudante, f.codigo_academia, f.ano_lectivo, f.ano_academico,
			f.data, f.materia_disciplinar_id, m.nome, f.quantidade, f.observacao,
			f.registered_at, f.event_id, f.version
		FROM projection_faltas f
		LEFT JOIN projection_materias m ON m.id::text = f.materia_disciplinar_id
		WHERE f.codigo_estudante = $1
			AND f.ano_lectivo = $2
			AND f.data BETWEEN $3 AND $4
		ORDER BY f.data DESC
	`, codigoEstudante, anoLectivo, dataInicio.Format(time.RFC3339), dataFim.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFaltas(rows)
}

func (p *FaltasProjection) GetAll() ([]FaltaDTO, error) {
	rows, err := p.client.DB().Query(`
		SELECT f.id, f.codigo_estudante, f.codigo_academia, f.ano_lectivo, f.ano_academico,
			f.data, f.materia_disciplinar_id, m.nome, f.quantidade, f.observacao,
			f.registered_at, f.event_id, f.version
		FROM projection_faltas f
		LEFT JOIN projection_materias m ON m.id::text = f.materia_disciplinar_id
		ORDER BY f.data DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFaltas(rows)
}

func scanFaltas(rows *sql.Rows) ([]FaltaDTO, error) {
	var faltas []FaltaDTO
	for rows.Next() {
		var f FaltaDTO
		if err := rows.Scan(
			&f.ID, &f.CodigoEstudante, &f.CodigoAcademia, &f.AnoLectivo, &f.AnoAcademico,
			&f.Data, &f.MateriaDisciplinarID, &f.MateriaNome, &f.Quantidade, &f.Observacao,
			&f.RegisteredAt, &f.EventID, &f.Version,
		); err != nil {
			continue
		}
		faltas = append(faltas, f)
	}
	return faltas, rows.Err()
}