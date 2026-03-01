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

// ============================================================================
// Interface Projection
// ============================================================================

func (p *NotasProjection) GetLastProcessedEventID() (int64, error) {
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

func (p *NotasProjection) UpdateCheckpoint(eventID int64) error {
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

func (p *NotasProjection) Handle(event db.Event) error {
	handlers := map[string]func(db.Event) error{
		"NotaRegistrada":   p.handleNotaRegistrada,
		"NotaCorrigida":    p.handleNotaCorrigida,
		"NotaEliminada":    p.handleNotaEliminada,
	}
	if handler, ok := handlers[event.EventType]; ok {
		log.Printf("[DEBUG] [notas] Processando %s: %s", event.EventType, event.EventID)
		return handler(event)
	}
	return nil
}

func (p *NotasProjection) Rebuild() error {
	log.Printf("[DEBUG] [notas] Rebuild iniciado")
	if err := p.clear(); err != nil {
		return fmt.Errorf("falha ao limpar: %w", err)
	}
	rows, err := p.client.DB().Query(`
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE event_type IN ('NotaRegistrada', 'NotaCorrigida', 'NotaEliminada')
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
	log.Printf("[DEBUG] [notas] Rebuild concluído: %d eventos", count)
	return rows.Err()
}

func (p *NotasProjection) clear() error {
	_, err := p.client.DB().Exec(`TRUNCATE TABLE projection_notas CASCADE`)
	return err
}

// ============================================================================
// Handlers de evento
// ============================================================================

func (p *NotasProjection) handleNotaRegistrada(event db.Event) error {
	var payload struct {
		CodigoEstudante      string    `json:"CodigoEstudante"`
		CodigoAcademia       string    `json:"CodigoAcademia"`
		AnoLectivo           string    `json:"AnoLectivo"`
		AnoAcademico         string    `json:"AnoAcademico"`
		Periodo              string    `json:"Periodo"`
		MateriaDisciplinarID string    `json:"MateriaDisciplinarID"`
		Tipo                 string    `json:"Tipo"`
		Categoria            string    `json:"Categoria"`
		Nota                 float64   `json:"Nota"`
		Observacao           *string   `json:"Observacao"`
		RegisteredAt         time.Time `json:"RegisteredAt"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error NotaRegistrada: %w", err)
	}

	_, err := p.client.DB().Exec(`
		INSERT INTO projection_notas (
			codigo_estudante, codigo_academia, ano_lectivo, ano_academico,
			periodo, materia_disciplinar_id, tipo, categoria, nota, observacao,
			registered_at, event_id, version
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT DO NOTHING
	`,
		payload.CodigoEstudante, payload.CodigoAcademia, payload.AnoLectivo, payload.AnoAcademico,
		payload.Periodo, payload.MateriaDisciplinarID, payload.Tipo, payload.Categoria,
		payload.Nota, payload.Observacao,
		payload.RegisteredAt, event.EventID, event.EventVersion,
	)
	return err
}

func (p *NotasProjection) handleNotaCorrigida(event db.Event) error {
	var payload struct {
		NotaID     string  `json:"NotaID"`
		NovaNota   float64 `json:"NovaNota"`
		Observacao *string `json:"Observacao"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error NotaCorrigida: %w", err)
	}

	_, err := p.client.DB().Exec(`
		UPDATE projection_notas
		SET nota = $1, observacao = $2, version = $3, event_id = $4
		WHERE id = $5
	`, payload.NovaNota, payload.Observacao, event.EventVersion, event.EventID, payload.NotaID)
	return err
}

func (p *NotasProjection) handleNotaEliminada(event db.Event) error {
	var payload struct {
		NotaID string `json:"NotaID"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error NotaEliminada: %w", err)
	}
	_, err := p.client.DB().Exec(`DELETE FROM projection_notas WHERE id = $1`, payload.NotaID)
	return err
}

// ============================================================================
// Queries de leitura
// ============================================================================

type NotaDTO struct {
	ID                   string   `json:"id"`
	CodigoEstudante      string   `json:"codigo_estudante"`
	CodigoAcademia       string   `json:"codigo_academia"`
	AnoLectivo           string   `json:"ano_lectivo"`
	AnoAcademico         string   `json:"ano_academico"`
	Periodo              string   `json:"periodo"`
	MateriaDisciplinarID string   `json:"materia_disciplinar_id"`
	Tipo                 string   `json:"tipo"`
	Categoria            string   `json:"categoria"`
	Nota                 float64  `json:"nota"`
	Observacao           *string  `json:"observacao,omitempty"`
	RegisteredAt         string   `json:"registered_at"`
	EventID              string   `json:"event_id"`
	Version              int      `json:"version"`
}

func (p *NotasProjection) GetNota(
	codigoEstudante, anoLectivo, periodo string,
	materiaID uuid.UUID,
	tipo, categoria string,
) (*NotaDTO, error) {
	var n NotaDTO
	err := p.client.DB().QueryRow(`
		SELECT id, codigo_estudante, codigo_academia, ano_lectivo, ano_academico, periodo,
			materia_disciplinar_id, tipo, categoria, nota, observacao,
			registered_at, event_id, version
		FROM projection_notas
		WHERE codigo_estudante = $1
			AND ano_lectivo = $2
			AND periodo = $3
			AND materia_disciplinar_id = $4
			AND tipo = $5
			AND categoria = $6
		LIMIT 1
	`, codigoEstudante, anoLectivo, periodo, materiaID.String(), tipo, categoria).Scan(
		&n.ID, &n.CodigoEstudante, &n.CodigoAcademia, &n.AnoLectivo, &n.AnoAcademico, &n.Periodo,
		&n.MateriaDisciplinarID, &n.Tipo, &n.Categoria, &n.Nota, &n.Observacao,
		&n.RegisteredAt, &n.EventID, &n.Version,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &n, err
}

func (p *NotasProjection) GetNotaByID(id string) (*NotaDTO, error) {
	var n NotaDTO
	err := p.client.DB().QueryRow(`
		SELECT id, codigo_estudante, codigo_academia, ano_lectivo, ano_academico, periodo,
			materia_disciplinar_id, tipo, categoria, nota, observacao,
			registered_at, event_id, version
		FROM projection_notas
		WHERE id = $1
		LIMIT 1
	`, id).Scan(
		&n.ID, &n.CodigoEstudante, &n.CodigoAcademia, &n.AnoLectivo, &n.AnoAcademico, &n.Periodo,
		&n.MateriaDisciplinarID, &n.Tipo, &n.Categoria, &n.Nota, &n.Observacao,
		&n.RegisteredAt, &n.EventID, &n.Version,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &n, err
}

func (p *NotasProjection) GetNotasByEstudante(codigoEstudante string) ([]NotaDTO, error) {
	rows, err := p.client.DB().Query(`
		SELECT id, codigo_estudante, codigo_academia, ano_lectivo, ano_academico, periodo,
			materia_disciplinar_id, tipo, categoria, nota, observacao,
			registered_at, event_id, version
		FROM projection_notas
		WHERE codigo_estudante = $1
		ORDER BY registered_at DESC
	`, codigoEstudante)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanNotas(rows)
}

// GetByEstudante é um alias de GetNotasByEstudante — usado nos handlers.
func (p *NotasProjection) GetByEstudante(codigoEstudante string) ([]NotaDTO, error) {
	return p.GetNotasByEstudante(codigoEstudante)
}

func (p *NotasProjection) GetNotasByAcademia(codigoAcademia string) ([]NotaDTO, error) {
	rows, err := p.client.DB().Query(`
		SELECT id, codigo_estudante, codigo_academia, ano_lectivo, ano_academico, periodo,
			materia_disciplinar_id, tipo, categoria, nota, observacao,
			registered_at, event_id, version
		FROM projection_notas
		WHERE codigo_academia = $1
		ORDER BY registered_at DESC
	`, codigoAcademia)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanNotas(rows)
}

func (p *NotasProjection) GetAll() ([]NotaDTO, error) {
	rows, err := p.client.DB().Query(`
		SELECT id, codigo_estudante, codigo_academia, ano_lectivo, ano_academico, periodo,
			materia_disciplinar_id, tipo, categoria, nota, observacao,
			registered_at, event_id, version
		FROM projection_notas
		ORDER BY registered_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanNotas(rows)
}

func scanNotas(rows *sql.Rows) ([]NotaDTO, error) {
	var notas []NotaDTO
	for rows.Next() {
		var n NotaDTO
		if err := rows.Scan(
			&n.ID, &n.CodigoEstudante, &n.CodigoAcademia, &n.AnoLectivo, &n.AnoAcademico, &n.Periodo,
			&n.MateriaDisciplinarID, &n.Tipo, &n.Categoria, &n.Nota, &n.Observacao,
			&n.RegisteredAt, &n.EventID, &n.Version,
		); err != nil {
			continue
		}
		notas = append(notas, n)
	}
	return notas, rows.Err()
}

// ============================================================================
// Helper
// ============================================================================

func nullOrText(s *string) interface{} {
	if s == nil {
		return nil
	}
	return *s
}