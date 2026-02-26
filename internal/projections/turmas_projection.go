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
		"TurmaCriada":              p.handleTurmaCriada,
		"TurmaAtivada":             p.handleStatusChange("ativo"),
		"TurmaDesativada":          p.handleStatusChange("inativo"),
		"EstudanteAdicionadoATurma": p.handleEstudanteAdicionado,
		"EstudanteRemovidoDaTurma":  p.handleEstudanteRemovido,
		"TurmaDadosAtualizados":    p.handleTurmaDadosAtualizados,
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
	return nil
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

	var cursoID interface{} = nil
	if payload.CursoID != nil {
		cursoID = *payload.CursoID
	}

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
		nullOrUUID(cursoID),
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
		SET estudantes  = estudantes || '"%s"'::jsonb,
		    version     = %d,
		    updated_at  = CURRENT_TIMESTAMP,
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

	// Remove o elemento do array JSONB
	query := fmt.Sprintf(`
		UPDATE projection_turmas
		SET estudantes  = (
		        SELECT jsonb_agg(e)
		        FROM jsonb_array_elements_text(estudantes) e
		        WHERE e <> '%s'
		    ),
		    version     = %d,
		    updated_at  = CURRENT_TIMESTAMP,
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
	return scanTurmaDTO(row)
}

func (p *TurmasProjection) GetByCodigoTurma(codigoTurma, codigoAcademia string) (*TurmaDTO, error) {
	row := p.client.DB().QueryRow(`
		SELECT id, codigo_turma, codigo_academia, nivel, curso_id, turno,
		       estudantes, status, created_at, updated_at, version
		FROM projection_turmas
		WHERE codigo_turma = $1 AND codigo_academia = $2
	`, codigoTurma, codigoAcademia)
	return scanTurmaDTO(row)
}

func (p *TurmasProjection) ListByAcademia(codigoAcademia string) ([]*TurmaDTO, error) {
	rows, err := p.client.DB().Query(`
		SELECT id, codigo_turma, codigo_academia, nivel, curso_id, turno,
		       estudantes, status, created_at, updated_at, version
		FROM projection_turmas
		WHERE codigo_academia = $1
		ORDER BY nivel, turno
	`, codigoAcademia)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var turmas []*TurmaDTO
	for rows.Next() {
		t, err := scanTurmaDTORows(rows)
		if err != nil {
			return nil, err
		}
		turmas = append(turmas, t)
	}
	return turmas, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func scanTurmaDTO(row *sql.Row) (*TurmaDTO, error) {
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

func scanTurmaDTORows(rows *sql.Rows) (*TurmaDTO, error) {
	var t TurmaDTO
	var cursoID sql.NullString
	var estudantesJSON []byte

	err := rows.Scan(&t.ID, &t.CodigoTurma, &t.CodigoAcademia, &t.Nivel,
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

// nullOrUUID retorna 'uuid' ou NULL para uso em queries SQL
func nullOrUUID(v interface{}) string {
	if v == nil {
		return "NULL"
	}
	return fmt.Sprintf("'%v'", v)
}