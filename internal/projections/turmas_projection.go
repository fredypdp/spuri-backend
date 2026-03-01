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

type TurmasProjection struct {
	client *db.Client
}

func NewTurmasProjection(client *db.Client) *TurmasProjection {
	return &TurmasProjection{client: client}
}

func (p *TurmasProjection) Name() string { return "turmas" }

func (p *TurmasProjection) Handle(event db.Event) error {
	if event.AggregateType != "Turma" {
		return nil
	}

	handlers := map[string]func(db.Event) error{
		"TurmaCriada":               p.handleTurmaCriada,
		"TurmaAtivada":              p.handleStatusChange("ativo"),
		"TurmaDesativada":           p.handleStatusChange("inativo"),
		"EstudanteAdicionadoATurma": p.handleEstudanteAdicionado,
		"EstudanteRemovidoDaTurma":  p.handleEstudanteRemovido,
		"TurmaDadosAtualizados":     p.handleTurmaDadosAtualizados,
		"TurmaDeletada": p.handleTurmaDeletada,
	}

	if handler, ok := handlers[event.EventType]; ok {
		log.Printf("[DEBUG] Processando %s para turma %s", event.EventType, event.AggregateID)
		return handler(event)
	}
	return nil
}

func (p *TurmasProjection) Rebuild() error {
	log.Printf("[DEBUG] [turmas] Rebuild iniciado")

	if err := p.clear(); err != nil {
		return fmt.Errorf("falha ao limpar: %w", err)
	}

	rows, err := p.client.DB().Query(`
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger WHERE aggregate_type = 'Turma' ORDER BY id ASC
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

	log.Printf("[DEBUG] [turmas] Rebuild concluído: %d eventos processados", count)
	return rows.Err()
}

func (p *TurmasProjection) GetLastProcessedEventID() (int64, error) {
	var lastID int64
	query := fmt.Sprintf(`SELECT last_processed_event_id FROM projection_checkpoints WHERE projection_name = '%s'`,
		db.SafeString(p.Name()))
	err := p.client.DB().QueryRow(query).Scan(&lastID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return lastID, err
}

func (p *TurmasProjection) UpdateCheckpoint(eventID int64) error {
	eventID = int64(db.ValidateOffset(int(eventID)))
	query := fmt.Sprintf(`
		INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at, events_processed)
		VALUES ('%s', %d, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) DO UPDATE SET
			last_processed_event_id = %d,
			last_processed_at = CURRENT_TIMESTAMP,
			events_processed = projection_checkpoints.events_processed + 1
	`, db.SafeString(p.Name()), eventID, eventID)
	_, err := p.client.DB().Exec(query)
	return err
}

// ── Handlers de evento ────────────────────────────────────────────────────────

func (p *TurmasProjection) handleTurmaCriada(event db.Event) error {
	var payload struct {
		CodigoTurma    string
		CodigoAcademia string
		Nivel          string
		CursoID        *uuid.UUID
		Turno          string
		CreatedAt      time.Time
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}

	estudantesJSON, _ := json.Marshal([]string{})

	query := fmt.Sprintf(`
		INSERT INTO projection_turmas
			(id, codigo_turma, codigo_academia, nivel, curso_id, turno,
			 estudantes, status, created_at, updated_at, version, last_event_id)
		VALUES ('%s','%s','%s','%s',%s,'%s','%s','ativo','%s',CURRENT_TIMESTAMP,%d,'%s')
		ON CONFLICT (id) DO NOTHING
	`,
		event.AggregateID,
		db.SafeString(payload.CodigoTurma),
		db.SafeString(payload.CodigoAcademia),
		db.SafeString(payload.Nivel),
		nullOrUUID(payload.CursoID),
		db.SafeString(payload.Turno),
		string(estudantesJSON),
		payload.CreatedAt.Format(time.RFC3339),
		event.EventVersion,
		event.EventID,
	)

	_, err := p.client.DB().Exec(query)
	return err
}

func (p *TurmasProjection) handleStatusChange(status string) func(db.Event) error {
	return func(event db.Event) error {
		query := fmt.Sprintf(`
			UPDATE projection_turmas
			SET status = '%s', version = %d, updated_at = CURRENT_TIMESTAMP, last_event_id = '%s'
			WHERE id = '%s'
		`, status, event.EventVersion, event.EventID, event.AggregateID)
		_, err := p.client.DB().Exec(query)
		return err
	}
}

func (p *TurmasProjection) handleEstudanteAdicionado(event db.Event) error {
	var payload struct{ CodigoEstudante string }
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}

	query := fmt.Sprintf(`
		UPDATE projection_turmas
		SET estudantes    = estudantes || '"%s"'::jsonb,
		    version       = %d,
		    updated_at    = CURRENT_TIMESTAMP,
		    last_event_id = '%s'
		WHERE id = '%s'
	`, db.SafeString(payload.CodigoEstudante), event.EventVersion, event.EventID, event.AggregateID)

	_, err := p.client.DB().Exec(query)
	return err
}

func (p *TurmasProjection) handleEstudanteRemovido(event db.Event) error {
	var payload struct{ CodigoEstudante string }
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}

	query := fmt.Sprintf(`
		UPDATE projection_turmas
		SET estudantes = (
		        SELECT COALESCE(jsonb_agg(e), '[]'::jsonb)
		        FROM jsonb_array_elements_text(estudantes) e
		        WHERE e <> '%s'
		    ),
		    version       = %d,
		    updated_at    = CURRENT_TIMESTAMP,
		    last_event_id = '%s'
		WHERE id = '%s'
	`, db.SafeString(payload.CodigoEstudante), event.EventVersion, event.EventID, event.AggregateID)

	_, err := p.client.DB().Exec(query)
	return err
}

func (p *TurmasProjection) handleTurmaDadosAtualizados(event db.Event) error {
	var payload struct {
		Nivel   *string
		CursoID *uuid.UUID
		Turno   *string
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}

	if payload.Nivel != nil {
		q := fmt.Sprintf(`UPDATE projection_turmas SET nivel = '%s' WHERE id = '%s'`,
			db.SafeString(*payload.Nivel), event.AggregateID)
		p.client.DB().Exec(q)
	}
	if payload.Turno != nil {
		q := fmt.Sprintf(`UPDATE projection_turmas SET turno = '%s' WHERE id = '%s'`,
			db.SafeString(*payload.Turno), event.AggregateID)
		p.client.DB().Exec(q)
	}
	if payload.CursoID != nil {
		q := fmt.Sprintf(`UPDATE projection_turmas SET curso_id = '%s' WHERE id = '%s'`,
			*payload.CursoID, event.AggregateID)
		p.client.DB().Exec(q)
	}

	q := fmt.Sprintf(`
		UPDATE projection_turmas
		SET version = %d, updated_at = CURRENT_TIMESTAMP, last_event_id = '%s'
		WHERE id = '%s'
	`, event.EventVersion, event.EventID, event.AggregateID)
	_, err := p.client.DB().Exec(q)
	return err
}

func (p *TurmasProjection) handleTurmaDeletada(event db.Event) error {
	var payload struct {
		DeletadoPor string    `json:"DeletadoPor"`
		Motivo      string    `json:"Motivo"`
		DeletedAt   time.Time `json:"DeletedAt"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}

	// Mantém o registro na projeção com status=deletado e deleted_at preenchido.
	// Isso garante auditabilidade via READ; filtre WHERE deleted_at IS NULL em queries normais.
	query := fmt.Sprintf(`
		UPDATE projection_turmas
		SET
			status     = 'deletado',
			deleted_at = '%s',
			updated_at = CURRENT_TIMESTAMP,
			version    = %d,
			last_event_id = '%s'
		WHERE id = '%s'
	`,
		payload.DeletedAt.UTC().Format("2006-01-02T15:04:05Z"),
		event.EventVersion,
		event.EventID,
		event.AggregateID,
	)

	_, err := p.client.DB().Exec(query)
	return err
}

// ── Queries de leitura ────────────────────────────────────────────────────────

type TurmaDTO struct {
	ID             uuid.UUID  `json:"id"`
	CodigoTurma    string     `json:"codigo_turma"`
	CodigoAcademia string     `json:"codigo_academia"`
	Nivel          string     `json:"nivel"`
	CursoID        *uuid.UUID `json:"curso_id,omitempty"`
	Turno          string     `json:"turno"`
	Estudantes     []string   `json:"estudantes"`
	Status         string     `json:"status"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	Version        int        `json:"version"`
}

func (p *TurmasProjection) GetByID(id uuid.UUID) (*TurmaDTO, error) {
	row := p.client.DB().QueryRow(`
		SELECT id, codigo_turma, codigo_academia, nivel, curso_id, turno,
		       estudantes, status, created_at, updated_at, version
		FROM projection_turmas WHERE id = $1
	`, id)
	return scanTurmaRow(row)
}

func (p *TurmasProjection) GetByCodigoTurma(codigoTurma, codigoAcademia string) (*TurmaDTO, error) {
	row := p.client.DB().QueryRow(`
		SELECT id, codigo_turma, codigo_academia, nivel, curso_id, turno,
		       estudantes, status, created_at, updated_at, version
		FROM projection_turmas
		WHERE codigo_turma = $1 AND codigo_academia = $2
	`, codigoTurma, codigoAcademia)
	return scanTurmaRow(row)
}

func (p *TurmasProjection) ListByAcademia(codigoAcademia string) ([]*TurmaDTO, error) {
	rows, err := p.client.DB().Query(`
		SELECT id, codigo_turma, codigo_academia, nivel, curso_id, turno,
		       estudantes, status, created_at, updated_at, version
		FROM projection_turmas
		WHERE codigo_academia = $1
			AND deleted_at IS NULL
		ORDER BY nivel, turno
	`, codigoAcademia)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var turmas []*TurmaDTO
	for rows.Next() {
		var t TurmaDTO
		var cursoID sql.NullString
		var estudantesJSON []byte
		if err := rows.Scan(&t.ID, &t.CodigoTurma, &t.CodigoAcademia, &t.Nivel,
			&cursoID, &t.Turno, &estudantesJSON, &t.Status, &t.CreatedAt, &t.UpdatedAt, &t.Version); err != nil {
			return nil, err
		}
		if cursoID.Valid {
			id, _ := uuid.Parse(cursoID.String)
			t.CursoID = &id
		}
		json.Unmarshal(estudantesJSON, &t.Estudantes)
		if t.Estudantes == nil {
			t.Estudantes = []string{}
		}
		turmas = append(turmas, &t)
	}
	return turmas, rows.Err()
}

func (p *TurmasProjection) ListByCurso(cursoID uuid.UUID) ([]TurmaDTO, error) {
	query := fmt.Sprintf(`
		SELECT id, codigo_turma, codigo_academia, nivel, curso_id, turno,
		       estudantes, status, created_at, updated_at, version
		FROM projection_turmas
		WHERE curso_id = '%s'
		  AND (deleted_at IS NULL OR status != 'deletado')
	`, cursoID)

	rows, err := p.client.DB().Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var turmas []TurmaDTO
	for rows.Next() {
		var dto TurmaDTO
		var estudantesJSON sql.NullString
		var cursoIDStr sql.NullString
		err := rows.Scan(
			&dto.ID, &dto.CodigoTurma, &dto.CodigoAcademia, &dto.Nivel,
			&cursoIDStr, &dto.Turno, &estudantesJSON,
			&dto.Status, &dto.CreatedAt, &dto.UpdatedAt, &dto.Version,
		)
		if err != nil {
			return nil, err
		}
		if estudantesJSON.Valid && estudantesJSON.String != "" {
			json.Unmarshal([]byte(estudantesJSON.String), &dto.Estudantes)
		}
		if cursoIDStr.Valid {
			cid, _ := uuid.Parse(cursoIDStr.String)
			dto.CursoID = &cid
		}
		turmas = append(turmas, dto)
	}
	return turmas, rows.Err()
}

// ── Helpers internos ──────────────────────────────────────────────────────────

func scanTurmaRow(row *sql.Row) (*TurmaDTO, error) {
	var t TurmaDTO
	var cursoID sql.NullString
	var estudantesJSON []byte

	err := row.Scan(&t.ID, &t.CodigoTurma, &t.CodigoAcademia, &t.Nivel,
		&cursoID, &t.Turno, &estudantesJSON, &t.Status, &t.CreatedAt, &t.UpdatedAt, &t.Version)
	if err != nil {
		return nil, err
	}
	if cursoID.Valid {
		id, _ := uuid.Parse(cursoID.String)
		t.CursoID = &id
	}
	json.Unmarshal(estudantesJSON, &t.Estudantes)
	if t.Estudantes == nil {
		t.Estudantes = []string{}
	}
	return &t, nil
}

func (p *TurmasProjection) clear() error {
	_, err := p.client.DB().Exec(`DELETE FROM projection_turmas`)
	return err
}