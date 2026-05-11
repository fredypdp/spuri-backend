package projections

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"spuri/internal/db"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

type TurmasProjection struct {
	client *db.Client
}

func NewTurmasProjection(client *db.Client) *TurmasProjection {
	return &TurmasProjection{client: client}
}

func (p *TurmasProjection) Name() string { return "turmas" }

// ============================================================================
// Interface Projection
// ============================================================================

func (p *TurmasProjection) GetLastProcessedEventID() (int64, error) {
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

func (p *TurmasProjection) UpdateCheckpoint(eventID int64) error {
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

func (p *TurmasProjection) Handle(event db.Event) error {
	handlers := map[string]func(db.Event) error{
		"TurmaCriada":                p.handleTurmaCriada,
		"TurmaAtivada":               p.handleTurmaAtivada,
		"TurmaDesativada":            p.handleTurmaDesativada,
		"EstudanteAdicionadoATurma":  p.handleEstudanteAdicionado,
		"EstudanteRemovidoDaTurma":   p.handleEstudanteRemovido,
		"AvaliacaoFinalEscolar":      p.handleAvaliacaoFinalAnoAcademico,
		"AvaliacaoFinalSuperior":     p.handleAvaliacaoFinalAnoAcademico,
		"TurmaDadosAtualizados":      p.handleTurmaAtualizada,
		"TurmaDeletada":              p.handleTurmaDeletada,
	}
	if handler, ok := handlers[event.EventType]; ok {
		log.Printf("[DEBUG] [turmas] Processando %s: %s", event.EventType, event.EventID)
		return handler(event)
	}
	return nil
}

func (p *TurmasProjection) Rebuild() error {
	log.Printf("[DEBUG] [turmas] Rebuild iniciado")
	if _, err := p.client.DB().Exec(`TRUNCATE TABLE projection_turmas CASCADE`); err != nil {
		return fmt.Errorf("falha ao limpar: %w", err)
	}
	rows, err := p.client.DB().Query(`
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE aggregate_type = 'Turma' OR event_type IN ('AvaliacaoFinalEscolar','AvaliacaoFinalSuperior')
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
	log.Printf("[DEBUG] [turmas] Rebuild concluído: %d eventos", count)
	return rows.Err()
}

// ============================================================================
// Handlers de evento
// ============================================================================

func (p *TurmasProjection) handleTurmaCriada(event db.Event) error {
	var payload struct {
		CodigoTurma    string     `json:"CodigoTurma"`
		CodigoAcademia string     `json:"CodigoAcademia"`
		Nivel          string     `json:"Nivel"`
		CursoID        *uuid.UUID `json:"CursoID"`
		Turno          string     `json:"Turno"`
		CreatedAt      time.Time  `json:"CreatedAt"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error TurmaCriada: %w", err)
	}

	estudantesJSON, _ := json.Marshal([]string{})
	var cursoID interface{}
	if payload.CursoID != nil {
		cursoID = payload.CursoID.String()
	}

	// FIX PROJ-TUR-02: usar created_at do payload para preservar timestamp real.
	createdAt := payload.CreatedAt
	if createdAt.IsZero() {
		createdAt = event.OccurredAt
	}

	_, err := p.client.DB().Exec(`
		INSERT INTO projection_turmas (
			id, codigo_turma, codigo_academia, nivel, curso_id, turno,
			estudantes, historico_estudantes_ano_letivo, status, created_at, updated_at, version, last_event_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, '{}'::jsonb, 'ativo', $8, CURRENT_TIMESTAMP, $9, $10)
		ON CONFLICT (id) DO UPDATE SET
			codigo_turma    = EXCLUDED.codigo_turma,
			codigo_academia = EXCLUDED.codigo_academia,
			nivel           = EXCLUDED.nivel,
			curso_id        = EXCLUDED.curso_id,
			turno           = EXCLUDED.turno,
			created_at      = EXCLUDED.created_at,
			version         = EXCLUDED.version,
			last_event_id   = EXCLUDED.last_event_id
	`,
		event.AggregateID, payload.CodigoTurma, payload.CodigoAcademia, payload.Nivel,
		cursoID, payload.Turno, string(estudantesJSON),
		createdAt.UTC(), event.EventVersion, event.EventID,
	)
	return err
}

func (p *TurmasProjection) handleTurmaAtivada(event db.Event) error {
	var payload struct {
		AlteradoPor uuid.UUID `json:"AlteradoPor"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error TurmaAtivada: %w", err)
	}

	_, err := p.client.DB().Exec(`
		UPDATE projection_turmas
		SET status              = 'ativo',
		    status_alterado_por = $1,
		    status_alterado_em  = $2,
		    version             = $3,
		    updated_at          = CURRENT_TIMESTAMP,
		    last_event_id       = $4
		WHERE id = $5
	`, payload.AlteradoPor, event.OccurredAt.UTC(), event.EventVersion, event.EventID, event.AggregateID)
	return err
}

func (p *TurmasProjection) handleTurmaDesativada(event db.Event) error {
	var payload struct {
		AlteradoPor uuid.UUID `json:"AlteradoPor"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error TurmaDesativada: %w", err)
	}

	_, err := p.client.DB().Exec(`
		UPDATE projection_turmas
		SET status              = 'inativo',
		    status_alterado_por = $1,
		    status_alterado_em  = $2,
		    version             = $3,
		    updated_at          = CURRENT_TIMESTAMP,
		    last_event_id       = $4
		WHERE id = $5
	`, payload.AlteradoPor, event.OccurredAt.UTC(), event.EventVersion, event.EventID, event.AggregateID)
	return err
}

// handleEstudanteAdicionado — BUG #1 FIX: event type corrigido para "EstudanteAdicionadoATurma".
func (p *TurmasProjection) handleEstudanteAdicionado(event db.Event) error {
	var payload struct {
		CodigoEstudante string `json:"CodigoEstudante"`
		AnoLectivo      string `json:"AnoLectivo"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error EstudanteAdicionadoATurma: %w", err)
	}

	_, err := p.client.DB().Exec(`
		UPDATE projection_turmas
		SET estudantes = (
			SELECT jsonb_agg(DISTINCT val)
			FROM (
				SELECT jsonb_array_elements_text(estudantes::jsonb) AS val
				UNION ALL SELECT $1::text
			) sub
		)::json,
		historico_estudantes_ano_letivo = CASE
			WHEN COALESCE(NULLIF($5, ''), '') = '' THEN historico_estudantes_ano_letivo
			ELSE jsonb_set(
				COALESCE(historico_estudantes_ano_letivo, '{}'::jsonb),
				ARRAY[$5]::text[],
				(
					SELECT to_jsonb(COALESCE(array_agg(DISTINCT v), ARRAY[]::text[]))
					FROM (
						SELECT jsonb_array_elements_text(
							COALESCE(historico_estudantes_ano_letivo -> $5, '[]'::jsonb)
						) AS v
						UNION ALL
						SELECT $1::text AS v
					) s
				),
				true
			)
		END,
		version = $2, updated_at = CURRENT_TIMESTAMP, last_event_id = $3
		WHERE id = $4
	`, payload.CodigoEstudante, event.EventVersion, event.EventID, event.AggregateID, payload.AnoLectivo)
	return err
}

func (p *TurmasProjection) handleEstudanteRemovido(event db.Event) error {
	var payload struct {
		CodigoEstudante string `json:"CodigoEstudante"`
		AnoLectivo      string `json:"AnoLectivo"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error EstudanteRemovidoDaTurma: %w", err)
	}

	_, err := p.client.DB().Exec(`
		UPDATE projection_turmas
		SET estudantes = COALESCE(
			(
				SELECT json_agg(val)
				FROM (
					SELECT jsonb_array_elements_text(estudantes::jsonb) AS val
				) sub
				WHERE val != $1
			),
			'[]'::json
		),
		version = $2, updated_at = CURRENT_TIMESTAMP, last_event_id = $3
		WHERE id = $4
	`, payload.CodigoEstudante, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

func (p *TurmasProjection) handleAvaliacaoFinalAnoAcademico(event db.Event) error {
	var payload struct {
		CodigoEstudante        string   `json:"codigo_estudante"`
		CodigoAcademia         string   `json:"codigo_academia"`
		AnoLectivo             string   `json:"ano_lectivo"`
		ProximoAnoAcademico    *string  `json:"proximo_ano_academico"`
		CodigosTurmasRemovidas []string `json:"codigos_turmas_removidas"`
		Aprovado               bool     `json:"aprovado"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error AvaliacaoFinalAnoAcademico em turmas: %w", err)
	}
	if payload.CodigoEstudante == "" {
		return nil
	}

	if event.EventType != "AvaliacaoFinalEscolar" {
		return nil
	}
	if !payload.Aprovado {
		return nil
	}
	if payload.ProximoAnoAcademico == nil || *payload.ProximoAnoAcademico == "" || len(payload.CodigosTurmasRemovidas) == 0 {
		return nil
	}

	tx, err := p.client.DB().Begin()
	if err != nil {
		return fmt.Errorf("handleAvaliacaoFinalAnoAcademico: erro ao iniciar transação: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec(`
		UPDATE projection_turmas
		SET estudantes = COALESCE(
			(
				SELECT json_agg(val)
				FROM (
					SELECT jsonb_array_elements_text(estudantes::jsonb) AS val
				) sub
				WHERE val != $1
			),
			'[]'::json
		),
		historico_estudantes_ano_letivo = CASE
			WHEN COALESCE(NULLIF($2, ''), '') = '' THEN historico_estudantes_ano_letivo
			ELSE jsonb_set(
				COALESCE(historico_estudantes_ano_letivo, '{}'::jsonb),
				ARRAY[$2]::text[],
				(
					SELECT to_jsonb(COALESCE(array_agg(DISTINCT v), ARRAY[]::text[]))
					FROM (
						SELECT jsonb_array_elements_text(
							COALESCE(historico_estudantes_ano_letivo -> $2, '[]'::jsonb)
						) AS v
						UNION ALL
						SELECT $1::text AS v
					) s
				),
				true
			)
		END,
		updated_at = CURRENT_TIMESTAMP
		WHERE codigo_turma = ANY($3) AND deleted_at IS NULL
	`, payload.CodigoEstudante, payload.AnoLectivo, pq.Array(payload.CodigosTurmasRemovidas)); err != nil {
		return err
	}

	if _, err := tx.Exec(`
		WITH origem AS (
			SELECT turno, curso_id
			FROM projection_turmas
			WHERE codigo_turma = ANY($4)
			  AND codigo_academia = $1
			  AND deleted_at IS NULL
			ORDER BY codigo_turma ASC
			LIMIT 1
		),
		destinos_nivel AS (
			SELECT
				t.id,
				t.codigo_turma,
				t.turno,
				t.curso_id
			FROM projection_turmas t
			WHERE t.codigo_academia = $1
			  AND t.nivel = $2
			  AND t.deleted_at IS NULL
		),
		destino_compat AS (
			SELECT d.id
			FROM destinos_nivel d
			JOIN origem o
			  ON d.turno IS NOT DISTINCT FROM o.turno
			 AND d.curso_id IS NOT DISTINCT FROM o.curso_id
			ORDER BY d.codigo_turma ASC
			LIMIT 1
		),
		destino_fallback AS (
			SELECT d.id
			FROM (
				SELECT
					id,
					codigo_turma,
					ROW_NUMBER() OVER (ORDER BY codigo_turma ASC) - 1 AS idx,
					COUNT(*) OVER () AS total
				FROM destinos_nivel
			) d
			WHERE d.total > 0
			  AND d.idx = (ABS(hashtext($3)) % d.total)
			LIMIT 1
		),
		destino AS (
			SELECT id FROM destino_compat
			UNION ALL
			SELECT id FROM destino_fallback
			WHERE NOT EXISTS (SELECT 1 FROM destino_compat)
			LIMIT 1
		)
		UPDATE projection_turmas t
		SET estudantes = CASE
				WHEN EXISTS (
					SELECT 1
					FROM jsonb_array_elements_text(COALESCE(t.estudantes::jsonb, '[]'::jsonb)) AS v(val)
					WHERE v.val = $3
				) THEN t.estudantes
				ELSE COALESCE(t.estudantes::jsonb, '[]'::jsonb) || to_jsonb($3::text)
			END,
			updated_at = CURRENT_TIMESTAMP
		FROM destino d
		WHERE t.id = d.id
	`, payload.CodigoAcademia, *payload.ProximoAnoAcademico, payload.CodigoEstudante, pq.Array(payload.CodigosTurmasRemovidas)); err != nil {
		return err
	}

	return tx.Commit()
}

func (p *TurmasProjection) handleTurmaAtualizada(event db.Event) error {
	var payload struct {
		Nivel   *string    `json:"Nivel"`
		CursoID *uuid.UUID `json:"CursoID"`
		Turno   *string    `json:"Turno"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error TurmaDadosAtualizados: %w", err)
	}

	// FIX PROJ-08: transação única para todas as atualizações parciais.
	tx, err := p.client.DB().Begin()
	if err != nil {
		return fmt.Errorf("handleTurmaAtualizada: erro ao iniciar transação: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if payload.Nivel != nil {
		if _, err := tx.Exec(
			`UPDATE projection_turmas SET nivel = $1 WHERE id = $2`,
			*payload.Nivel, event.AggregateID,
		); err != nil {
			return fmt.Errorf("handleTurmaAtualizada: erro ao atualizar nivel: %w", err)
		}
	}
	if payload.CursoID != nil {
		if _, err := tx.Exec(
			`UPDATE projection_turmas SET curso_id = $1 WHERE id = $2`,
			payload.CursoID.String(), event.AggregateID,
		); err != nil {
			return fmt.Errorf("handleTurmaAtualizada: erro ao atualizar curso_id: %w", err)
		}
	}
	if payload.Turno != nil {
		if _, err := tx.Exec(
			`UPDATE projection_turmas SET turno = $1 WHERE id = $2`,
			*payload.Turno, event.AggregateID,
		); err != nil {
			return fmt.Errorf("handleTurmaAtualizada: erro ao atualizar turno: %w", err)
		}
	}

	if _, err := tx.Exec(`
		UPDATE projection_turmas
		SET version = $1, updated_at = CURRENT_TIMESTAMP, last_event_id = $2
		WHERE id = $3
	`, event.EventVersion, event.EventID, event.AggregateID); err != nil {
		return fmt.Errorf("handleTurmaAtualizada: erro ao atualizar version: %w", err)
	}

	return tx.Commit()
}

func (p *TurmasProjection) handleTurmaDeletada(event db.Event) error {
	var payload struct {
		DeletedAt time.Time `json:"DeletedAt"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error TurmaDeletada: %w", err)
	}

	_, err := p.client.DB().Exec(`
		UPDATE projection_turmas
		SET status = 'deletado', deleted_at = $1,
			updated_at = CURRENT_TIMESTAMP, version = $2, last_event_id = $3
		WHERE id = $4
	`, payload.DeletedAt.UTC(), event.EventVersion, event.EventID, event.AggregateID)
	return err
}

// ============================================================================
// Queries de leitura
// ============================================================================

type TurmaDTO struct {
	ID                           uuid.UUID           `json:"id"`
	CodigoTurma                  string              `json:"codigo_turma"`
	CodigoAcademia               string              `json:"codigo_academia"`
	Nivel                        string              `json:"nivel"`
	CursoID                      *uuid.UUID          `json:"curso_id,omitempty"`
	Turno                        string              `json:"turno"`
	Estudantes                   []string            `json:"estudantes"`
	HistoricoEstudantesAnoLetivo map[string][]string `json:"historico_estudantes_ano_letivo"`
	Status                       string              `json:"status"`
	StatusAlteradoPor            *uuid.UUID          `json:"status_alterado_por,omitempty"`
	StatusAlteradoEm             *time.Time          `json:"status_alterado_em,omitempty"`
	CreatedAt                    time.Time           `json:"created_at"`
	UpdatedAt                    time.Time           `json:"updated_at"`
	Version                      int                 `json:"version"`
}

func (p *TurmasProjection) GetByID(id uuid.UUID) (*TurmaDTO, error) {
	row := p.client.DB().QueryRow(`
		SELECT id, codigo_turma, codigo_academia, nivel, curso_id, turno,
		       estudantes, historico_estudantes_ano_letivo, status, status_alterado_por, status_alterado_em,
		       created_at, updated_at, version
		FROM projection_turmas WHERE id = $1
	`, id)
	return scanTurmaRow(row)
}

func (p *TurmasProjection) GetByCodigoTurma(codigoTurma, codigoAcademia string) (*TurmaDTO, error) {
	row := p.client.DB().QueryRow(`
		SELECT id, codigo_turma, codigo_academia, nivel, curso_id, turno,
		       estudantes, historico_estudantes_ano_letivo, status, status_alterado_por, status_alterado_em,
		       created_at, updated_at, version
		FROM projection_turmas
		WHERE codigo_turma = $1 AND codigo_academia = $2
			AND deleted_at IS NULL
		LIMIT 1
	`, codigoTurma, codigoAcademia)
	return scanTurmaRow(row)
}

func (p *TurmasProjection) GetByAcademia(codigoAcademia string) ([]TurmaDTO, error) {
	rows, err := p.client.DB().Query(`
		SELECT id, codigo_turma, codigo_academia, nivel, curso_id, turno,
		       estudantes, historico_estudantes_ano_letivo, status, status_alterado_por, status_alterado_em,
		       created_at, updated_at, version
		FROM projection_turmas
		WHERE codigo_academia = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
	`, codigoAcademia)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTurmas(rows)
}

// ListByAcademia é um alias de GetByAcademia — retorna ponteiros para compatibilidade com handlers.
func (p *TurmasProjection) ListByAcademia(codigoAcademia string) ([]*TurmaDTO, error) {
	turmas, err := p.GetByAcademia(codigoAcademia)
	if err != nil {
		return nil, err
	}
	result := make([]*TurmaDTO, len(turmas))
	for i := range turmas {
		result[i] = &turmas[i]
	}
	return result, nil
}

// ListByCurso retorna turmas vinculadas a um curso específico.
func (p *TurmasProjection) ListByCurso(cursoID uuid.UUID) ([]*TurmaDTO, error) {
	rows, err := p.client.DB().Query(`
		SELECT id, codigo_turma, codigo_academia, nivel, curso_id, turno,
		       estudantes, historico_estudantes_ano_letivo, status, status_alterado_por, status_alterado_em,
		       created_at, updated_at, version
		FROM projection_turmas
		WHERE curso_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
	`, cursoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	turmas, err := scanTurmas(rows)
	if err != nil {
		return nil, err
	}
	result := make([]*TurmaDTO, len(turmas))
	for i := range turmas {
		result[i] = &turmas[i]
	}
	return result, nil
}

// ListByEstudante retorna turmas que contêm o código do estudante no array estudantes.
// Quando codigoAcademia != nil, limita o resultado à academia informada.
func (p *TurmasProjection) ListByEstudante(codigoEstudante string, codigoAcademia *string) ([]*TurmaDTO, error) {
	baseQuery := `
		SELECT id, codigo_turma, codigo_academia, nivel, curso_id, turno,
		       estudantes, historico_estudantes_ano_letivo, status, status_alterado_por, status_alterado_em,
		       created_at, updated_at, version
		FROM projection_turmas
		WHERE deleted_at IS NULL
		  AND EXISTS (
			  SELECT 1
			  FROM jsonb_array_elements_text(estudantes) AS e(codigo)
			  WHERE e.codigo = $1
		  )
	`

	var (
		rows *sql.Rows
		err  error
	)
	if codigoAcademia != nil {
		rows, err = p.client.DB().Query(baseQuery+` AND codigo_academia = $2 ORDER BY created_at DESC`, codigoEstudante, *codigoAcademia)
	} else {
		rows, err = p.client.DB().Query(baseQuery+` ORDER BY created_at DESC`, codigoEstudante)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	turmas, err := scanTurmas(rows)
	if err != nil {
		return nil, err
	}
	result := make([]*TurmaDTO, len(turmas))
	for i := range turmas {
		result[i] = &turmas[i]
	}
	return result, nil
}

func scanTurmaRow(row *sql.Row) (*TurmaDTO, error) {
	var dto TurmaDTO
	var cursoID sql.NullString
	var estudantesRaw []byte
	var historicoRaw []byte
	var statusAlteradoPor sql.NullString
	var statusAlteradoEm sql.NullTime

	err := row.Scan(
		&dto.ID, &dto.CodigoTurma, &dto.CodigoAcademia,
		&dto.Nivel, &cursoID, &dto.Turno,
		&estudantesRaw, &historicoRaw, &dto.Status,
		&statusAlteradoPor, &statusAlteradoEm,
		&dto.CreatedAt, &dto.UpdatedAt, &dto.Version,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if cursoID.Valid {
		uid, _ := uuid.Parse(cursoID.String)
		dto.CursoID = &uid
	}

	if statusAlteradoPor.Valid {
		uid, _ := uuid.Parse(statusAlteradoPor.String)
		dto.StatusAlteradoPor = &uid
	}

	if statusAlteradoEm.Valid {
		t := statusAlteradoEm.Time
		dto.StatusAlteradoEm = &t
	}

	if len(estudantesRaw) > 0 {
		_ = json.Unmarshal(estudantesRaw, &dto.Estudantes)
	}
	if dto.Estudantes == nil {
		dto.Estudantes = []string{}
	}
	if len(historicoRaw) > 0 {
		_ = json.Unmarshal(historicoRaw, &dto.HistoricoEstudantesAnoLetivo)
	}
	if dto.HistoricoEstudantesAnoLetivo == nil {
		dto.HistoricoEstudantesAnoLetivo = map[string][]string{}
	}

	return &dto, nil
}

func scanTurmas(rows *sql.Rows) ([]TurmaDTO, error) {
	var result []TurmaDTO
	for rows.Next() {
		var dto TurmaDTO
		var cursoID sql.NullString
		var estudantesRaw []byte
		var historicoRaw []byte
		var statusAlteradoPor sql.NullString
		var statusAlteradoEm sql.NullTime

		if err := rows.Scan(
			&dto.ID, &dto.CodigoTurma, &dto.CodigoAcademia,
			&dto.Nivel, &cursoID, &dto.Turno,
			&estudantesRaw, &historicoRaw, &dto.Status,
			&statusAlteradoPor, &statusAlteradoEm,
			&dto.CreatedAt, &dto.UpdatedAt, &dto.Version,
		); err != nil {
			continue
		}

		if cursoID.Valid {
			uid, _ := uuid.Parse(cursoID.String)
			dto.CursoID = &uid
		}

		if statusAlteradoPor.Valid {
			uid, _ := uuid.Parse(statusAlteradoPor.String)
			dto.StatusAlteradoPor = &uid
		}

		if statusAlteradoEm.Valid {
			t := statusAlteradoEm.Time
			dto.StatusAlteradoEm = &t
		}

		if len(estudantesRaw) > 0 {
			_ = json.Unmarshal(estudantesRaw, &dto.Estudantes)
		}
		if dto.Estudantes == nil {
			dto.Estudantes = []string{}
		}
		if len(historicoRaw) > 0 {
			_ = json.Unmarshal(historicoRaw, &dto.HistoricoEstudantesAnoLetivo)
		}
		if dto.HistoricoEstudantesAnoLetivo == nil {
			dto.HistoricoEstudantesAnoLetivo = map[string][]string{}
		}

		result = append(result, dto)
	}
	return result, rows.Err()
}
