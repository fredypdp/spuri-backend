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
	switch event.EventType {
	case "NotasRegistradas":
		return p.handleNotasRegistradas(event)
	case "NotaAtualizada":
		return p.handleNotaAtualizada(event)
	}
	return nil
}

// ============================================================================
// Rebuild (reprocessa todos os eventos do ledger)
// ============================================================================

func (p *NotasProjection) Rebuild() error {
	if err := p.clear(); err != nil {
		return fmt.Errorf("falha ao limpar: %w", err)
	}

	rows, err := p.client.DB().Query(`
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE event_type IN ('NotasRegistradas', 'NotaAtualizada')
		ORDER BY id ASC
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var event db.Event
		if err := rows.Scan(
			&event.ID, &event.EventID, &event.AggregateID, &event.AggregateType,
			&event.EventType, &event.EventVersion, &event.Payload, &event.Metadata,
			&event.OccurredAt, &event.RecordedAt, &event.LedgerHash, &event.PreviousHash,
		); err != nil {
			return err
		}
		if err := p.Handle(event); err != nil {
			return fmt.Errorf("erro no evento %d: %w", event.ID, err)
		}
		count++
	}

	log.Printf("[notas] Rebuild concluído: %d eventos", count)
	return rows.Err()
}

// ============================================================================
// Handlers de eventos
// ============================================================================

func (p *NotasProjection) handleNotasRegistradas(event db.Event) error {
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
		return fmt.Errorf("parse error NotasRegistradas: %w", err)
	}

	query := fmt.Sprintf(`
		INSERT INTO projection_notas (
			codigo_estudante, codigo_academia, ano_lectivo, ano_academico, periodo,
			materia_disciplinar_id, tipo, categoria, nota, observacao,
			registered_at, event_id, version
		) VALUES (
			'%s', '%s', '%s', '%s', '%s',
			'%s', '%s', '%s', %f, %s,
			'%s', '%s', %d
		)
		ON CONFLICT (codigo_estudante, ano_lectivo, periodo, materia_disciplinar_id, tipo, categoria)
		DO NOTHING
	`,
		db.SafeString(payload.CodigoEstudante),
		db.SafeString(payload.CodigoAcademia),
		db.SafeString(payload.AnoLectivo),
		db.SafeString(payload.AnoAcademico),
		db.SafeString(payload.Periodo),
		db.SafeString(payload.MateriaDisciplinarID),
		db.SafeString(payload.Tipo),
		db.SafeString(payload.Categoria),
		payload.Nota,
		nullOrText(payload.Observacao),
		payload.RegisteredAt.Format("2006-01-02 15:04:05"),
		event.EventID,
		event.EventVersion,
	)

	_, err := p.client.DB().Exec(query)
	return err
}

func (p *NotasProjection) handleNotaAtualizada(event db.Event) error {
	var payload struct {
		CodigoEstudante      string    `json:"CodigoEstudante"`
		CodigoAcademia       string    `json:"CodigoAcademia"`
		AnoLectivo           string    `json:"AnoLectivo"`
		Periodo              string    `json:"Periodo"`
		MateriaDisciplinarID string    `json:"MateriaDisciplinarID"`
		Tipo                 string    `json:"Tipo"`
		Categoria            string    `json:"Categoria"`
		NotaNova             float64   `json:"NotaNova"`
		Observacao           string    `json:"Observacao"`
		UpdatedAt            time.Time `json:"UpdatedAt"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error NotaAtualizada: %w", err)
	}

	query := fmt.Sprintf(`
		UPDATE projection_notas SET
			nota        = %f,
			observacao  = '%s',
			event_id    = '%s',
			version     = %d
		WHERE
			codigo_estudante      = '%s'
			AND ano_lectivo       = '%s'
			AND periodo           = '%s'
			AND materia_disciplinar_id = '%s'
			AND tipo              = '%s'
			AND categoria         = '%s'
	`,
		payload.NotaNova,
		db.SafeString(payload.Observacao),
		event.EventID,
		event.EventVersion,
		db.SafeString(payload.CodigoEstudante),
		db.SafeString(payload.AnoLectivo),
		db.SafeString(payload.Periodo),
		db.SafeString(payload.MateriaDisciplinarID),
		db.SafeString(payload.Tipo),
		db.SafeString(payload.Categoria),
	)

	_, err := p.client.DB().Exec(query)
	return err
}

// GetByEstudante retorna todas as notas de um estudante, ordenadas por período
func (p *NotasProjection) GetByEstudante(codigoEstudante string) ([]NotaDTO, error) {
	query := fmt.Sprintf(`
		SELECT id, codigo_estudante, codigo_academia, ano_lectivo, ano_academico, periodo,
			materia_disciplinar_id, tipo, categoria, nota, observacao,
			registered_at, event_id, version
		FROM projection_notas
		WHERE codigo_estudante = '%s'
		ORDER BY ano_lectivo DESC, periodo ASC, categoria ASC
	`, db.SafeString(codigoEstudante))

	rows, err := p.client.DB().Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notas []NotaDTO
	for rows.Next() {
		var n NotaDTO
		if err := rows.Scan(
			&n.ID, &n.CodigoEstudante, &n.CodigoAcademia, &n.AnoLectivo, &n.AnoAcademico, &n.Periodo,
			&n.MateriaDisciplinarID, &n.Tipo, &n.Categoria, &n.Nota, &n.Observacao,
			&n.RegisteredAt, &n.EventID, &n.Version,
		); err != nil {
			return nil, err
		}
		notas = append(notas, n)
	}
	return notas, rows.Err()
}

// ============================================================================
// Queries de leitura
// ============================================================================

type NotaDTO struct {
	ID                   string   `json:"id"`
	CodigoEstudante      string   `json:"codigo_estudante"`
	CodigoAcademia       string   `json:"codigo_academia"`
	AnoLectivo           string   `json:"ano_lectivo"`
	AnoAcademico string `json:"ano_academico"`
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

// GetNota busca uma nota específica pelo conjunto de chaves únicas
func (p *NotasProjection) GetNota(
	codigoEstudante, anoLectivo, periodo string,
	materiaID uuid.UUID,
	tipo, categoria string,
) (*NotaDTO, error) {
	query := fmt.Sprintf(`
		SELECT id, codigo_estudante, codigo_academia, ano_lectivo, ano_academico, periodo,
			materia_disciplinar_id, tipo, categoria, nota, observacao,
			registered_at, event_id, version
		FROM projection_notas
		WHERE codigo_estudante = '%s'
			AND ano_lectivo = '%s'
			AND periodo = '%s'
			AND materia_disciplinar_id = '%s'
			AND tipo = '%s'
			AND categoria = '%s'
		LIMIT 1
	`,
		db.SafeString(codigoEstudante),
		db.SafeString(anoLectivo),
		db.SafeString(periodo),
		materiaID.String(),
		db.SafeString(tipo),
		db.SafeString(categoria),
	)

	var n NotaDTO
	err := p.client.DB().QueryRow(query).Scan(
		&n.ID, &n.CodigoEstudante, &n.CodigoAcademia, &n.AnoLectivo, &n.AnoAcademico, &n.Periodo,
		&n.MateriaDisciplinarID, &n.Tipo, &n.Categoria, &n.Nota, &n.Observacao,
		&n.RegisteredAt, &n.EventID, &n.Version,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &n, err
}

// ============================================================================
// Checkpoint (inalterado)
// ============================================================================

func (p *NotasProjection) GetLastProcessedEventID() (int64, error) {
	var lastID int64
	query := fmt.Sprintf(`SELECT last_processed_event_id FROM projection_checkpoints WHERE projection_name = '%s'`,
		db.SafeString(p.Name()))
	err := p.client.DB().QueryRow(query).Scan(&lastID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return lastID, err
}

func (p *NotasProjection) UpdateCheckpoint(eventID int64) error {
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

func (p *NotasProjection) clear() error {
	_, err := p.client.DB().Exec(`TRUNCATE TABLE projection_notas CASCADE`)
	return err
}

func (p *NotasProjection) GetNotaByID(id string) (*NotaDTO, error) {
    query := fmt.Sprintf(`
        SELECT id, codigo_estudante, codigo_academia, ano_lectivo, ano_academico, periodo,
			materia_disciplinar_id, tipo, categoria, nota, observacao,
			registered_at, event_id, version
		FROM projection_notas
		WHERE id = '%s'
        LIMIT 1
    `, db.SafeString(id))

    var n NotaDTO
    err := p.client.DB().QueryRow(query).Scan(
        &n.ID, &n.CodigoEstudante, &n.CodigoAcademia, &n.AnoLectivo, &n.AnoAcademico, &n.Periodo,
        &n.MateriaDisciplinarID, &n.Tipo, &n.Categoria, &n.Nota, &n.Observacao,
        &n.RegisteredAt, &n.EventID, &n.Version,
    )
    if err == sql.ErrNoRows {
        return nil, nil
    }
    return &n, err
}

// ============================================================================
// Helper
// ============================================================================

func nullOrText(s *string) string {
	if s == nil {
		return "NULL"
	}
	return fmt.Sprintf("'%s'", db.SafeString(*s))
}