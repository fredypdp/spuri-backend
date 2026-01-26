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
	if event.EventType != "NotasRegistradas" {
		return nil
	}
	log.Printf("[DEBUG] Processando NotasRegistradas: %s", event.EventID)
	return p.handleNotasRegistradas(event)
}

func (p *NotasProjection) Rebuild() error {
	log.Printf("[DEBUG] Rebuild iniciado")
	
	if err := p.clear(); err != nil {
		return fmt.Errorf("falha ao limpar: %w", err)
	}

	rows, err := p.client.DB().Query(`
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger WHERE event_type = 'NotasRegistradas' ORDER BY id ASC
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

func (p *NotasProjection) handleNotasRegistradas(event db.Event) error {
	var payload struct {
		CodigoEstudante, CodigoAcademia, AnoLectivo, Periodo, MateriaDisciplinarID string
		Nota                                                                        float64
		Observacao                                                                  *string
		RegisteredAt                                                                time.Time
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error: %w", err)
	}

	log.Printf("[DEBUG] Registrando nota %.2f para estudante %s", 
		payload.Nota, payload.CodigoEstudante)

	query := fmt.Sprintf(`
		INSERT INTO projection_notas (
			codigo_estudante, codigo_academia, ano_lectivo, periodo,
			materia_disciplinar_id, nota, observacao, registered_at, event_id, version
		) VALUES ('%s', '%s', '%s', '%s', '%s', %f, %s, '%s', '%s', %d)
		ON CONFLICT (codigo_estudante, codigo_academia, ano_lectivo, periodo, materia_disciplinar_id)
		DO UPDATE SET nota = EXCLUDED.nota, observacao = EXCLUDED.observacao,
			registered_at = EXCLUDED.registered_at, event_id = EXCLUDED.event_id, version = EXCLUDED.version
	`, db.SafeString(payload.CodigoEstudante), db.SafeString(payload.CodigoAcademia),
		db.SafeString(payload.AnoLectivo), db.SafeString(payload.Periodo),
		db.SafeString(payload.MateriaDisciplinarID), payload.Nota,
		nullOrString(payload.Observacao), payload.RegisteredAt.Format(time.RFC3339),
		event.EventID, event.EventVersion)

	if _, err := p.client.DB().Exec(query); err != nil {
		return err
	}

	updateQuery := fmt.Sprintf(`
		UPDATE projection_estudantes
		SET total_notas = (SELECT COUNT(*) FROM projection_notas WHERE codigo_estudante = '%s')
		WHERE codigo_estudante = '%s'
	`, db.SafeString(payload.CodigoEstudante), db.SafeString(payload.CodigoEstudante))
	
	p.client.DB().Exec(updateQuery)
	return nil
}

func (p *NotasProjection) GetByEstudante(codigoEstudante string) ([]NotaDTO, error) {
	return p.queryNotas(fmt.Sprintf(
		`n.codigo_estudante = '%s' ORDER BY n.registered_at DESC`,
		db.SafeString(codigoEstudante)))
}

func (p *NotasProjection) GetByPeriodo(codigoEstudante, anoLectivo, periodo string) ([]NotaDTO, error) {
	return p.queryNotas(fmt.Sprintf(
		`n.codigo_estudante = '%s' AND n.ano_lectivo = '%s' AND n.periodo = '%s' ORDER BY m.nome`,
		db.SafeString(codigoEstudante), db.SafeString(anoLectivo), db.SafeString(periodo)))
}

func (p *NotasProjection) queryNotas(whereClause string) ([]NotaDTO, error) {
	query := fmt.Sprintf(`
		SELECT n.id, n.codigo_estudante, n.codigo_academia, n.ano_lectivo, n.periodo,
			n.materia_disciplinar_id, COALESCE(m.nome, '') as materia_nome, n.nota,
			n.observacao, n.registered_at, n.event_id, n.version
		FROM projection_notas n
		LEFT JOIN projection_materias m ON n.materia_disciplinar_id::uuid = m.id
		WHERE %s
	`, whereClause)
	
	rows, err := p.client.DB().Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []NotaDTO
	for rows.Next() {
		var dto NotaDTO
		if err := rows.Scan(&dto.ID, &dto.CodigoEstudante, &dto.CodigoAcademia, &dto.AnoLectivo,
			&dto.Periodo, &dto.MateriaDisciplinarID, &dto.MateriaNome, &dto.Nota,
			&dto.Observacao, &dto.RegisteredAt, &dto.EventID, &dto.Version); err != nil {
			continue
		}
		result = append(result, dto)
	}
	
	log.Printf("[DEBUG] %d notas encontradas", len(result))
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