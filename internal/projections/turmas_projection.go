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

// Handle processa eventos do ledger e atualiza a projection_turmas.
//
// CORREÇÃO BUG #1: Os 3 nomes abaixo estavam errados e nunca faziam match
// com os eventos reais emitidos pelo aggregate Turma:
//   - "EstudanteAdicionadoTurma"  → corrigido para "EstudanteAdicionadoATurma"
//   - "EstudanteRemovidoTurma"    → corrigido para "EstudanteRemovidoDaTurma"
//   - "TurmaAtualizada"           → corrigido para "TurmaDadosAtualizados"
//
// CORREÇÃO BUG #2: "TurmaAtivada" e "TurmaDesativada" estavam completamente
// ausentes do map — adicionados com seus respectivos handlers.
func (p *TurmasProjection) Handle(event db.Event) error {
	handlers := map[string]func(db.Event) error{
		"TurmaCriada":               p.handleTurmaCriada,
		// BUG #2 FIX — eram ausentes:
		"TurmaAtivada":              p.handleTurmaAtivada,
		"TurmaDesativada":           p.handleTurmaDesativada,
		// BUG #1 FIX — eram "EstudanteAdicionadoTurma":
		"EstudanteAdicionadoATurma": p.handleEstudanteAdicionado,
		// BUG #1 FIX — era "EstudanteRemovidoTurma":
		"EstudanteRemovidoDaTurma":  p.handleEstudanteRemovido,
		// BUG #1 FIX — era "TurmaAtualizada":
		"TurmaDadosAtualizados":     p.handleTurmaAtualizada,
		"TurmaDeletada":             p.handleTurmaDeletada,
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
		WHERE aggregate_type = 'Turma'
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
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error TurmaCriada: %w", err)
	}

	estudantesJSON, _ := json.Marshal([]string{})
	var cursoID interface{}
	if payload.CursoID != nil {
		cursoID = payload.CursoID.String()
	}

	_, err := p.client.DB().Exec(`
		INSERT INTO projection_turmas (
			id, codigo_turma, codigo_academia, nivel, curso_id, turno,
			estudantes, status, created_at, updated_at, version, last_event_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, 'ativo', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, $8, $9)
		ON CONFLICT (id) DO NOTHING
	`,
		event.AggregateID, payload.CodigoTurma, payload.CodigoAcademia, payload.Nivel,
		cursoID, payload.Turno, string(estudantesJSON), event.EventVersion, event.EventID,
	)
	return err
}

// handleTurmaAtivada — NOVO (BUG #2 FIX).
// Atualiza status para 'ativo' quando o aggregate emite TurmaAtivada.
func (p *TurmasProjection) handleTurmaAtivada(event db.Event) error {
	_, err := p.client.DB().Exec(`
		UPDATE projection_turmas
		SET status = 'ativo',
			version = $1, updated_at = CURRENT_TIMESTAMP, last_event_id = $2
		WHERE id = $3
	`, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

// handleTurmaDesativada — NOVO (BUG #2 FIX).
// Atualiza status para 'inativo' quando o aggregate emite TurmaDesativada.
func (p *TurmasProjection) handleTurmaDesativada(event db.Event) error {
	_, err := p.client.DB().Exec(`
		UPDATE projection_turmas
		SET status = 'inativo',
			version = $1, updated_at = CURRENT_TIMESTAMP, last_event_id = $2
		WHERE id = $3
	`, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

// handleEstudanteAdicionado — BUG #1 FIX: nome do event type era "EstudanteAdicionadoTurma".
// O aggregate emite "EstudanteAdicionadoATurma" — sem o match correto, o array
// de estudantes da projection_turmas nunca era atualizado.
func (p *TurmasProjection) handleEstudanteAdicionado(event db.Event) error {
	var payload struct {
		CodigoEstudante string `json:"CodigoEstudante"`
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
		version = $2, updated_at = CURRENT_TIMESTAMP, last_event_id = $3
		WHERE id = $4
	`, payload.CodigoEstudante, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

// handleEstudanteRemovido — BUG #1 FIX: nome do event type era "EstudanteRemovidoTurma".
// O aggregate emite "EstudanteRemovidoDaTurma" — sem o match correto, estudantes
// removidos continuavam aparecendo no array da projeção.
func (p *TurmasProjection) handleEstudanteRemovido(event db.Event) error {
	var payload struct {
		CodigoEstudante string `json:"CodigoEstudante"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error EstudanteRemovidoDaTurma: %w", err)
	}

	_, err := p.client.DB().Exec(`
		UPDATE projection_turmas
		SET estudantes = (
			SELECT COALESCE(json_agg(val), '[]'::json)
			FROM (
				SELECT jsonb_array_elements_text(estudantes::jsonb) AS val
			) sub
			WHERE val != $1
		),
		version = $2, updated_at = CURRENT_TIMESTAMP, last_event_id = $3
		WHERE id = $4
	`, payload.CodigoEstudante, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

// handleTurmaAtualizada — BUG #1 FIX: nome do event type era "TurmaAtualizada".
// O aggregate emite "TurmaDadosAtualizados" — sem o match correto, atualizações
// de nível, turno e curso_id nunca eram refletidas na projeção.
func (p *TurmasProjection) handleTurmaAtualizada(event db.Event) error {
	var payload struct {
		Nivel   *string    `json:"Nivel"`
		CursoID *uuid.UUID `json:"CursoID"`
		Turno   *string    `json:"Turno"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error TurmaDadosAtualizados: %w", err)
	}

	if payload.Nivel != nil {
		if _, err := p.client.DB().Exec(
			`UPDATE projection_turmas SET nivel = $1 WHERE id = $2`,
			*payload.Nivel, event.AggregateID,
		); err != nil {
			return fmt.Errorf("erro ao atualizar nivel: %w", err)
		}
	}
	if payload.CursoID != nil {
		if _, err := p.client.DB().Exec(
			`UPDATE projection_turmas SET curso_id = $1 WHERE id = $2`,
			payload.CursoID.String(), event.AggregateID,
		); err != nil {
			return fmt.Errorf("erro ao atualizar curso_id: %w", err)
		}
	}
	if payload.Turno != nil {
		if _, err := p.client.DB().Exec(
			`UPDATE projection_turmas SET turno = $1 WHERE id = $2`,
			*payload.Turno, event.AggregateID,
		); err != nil {
			return fmt.Errorf("erro ao atualizar turno: %w", err)
		}
	}

	_, err := p.client.DB().Exec(`
		UPDATE projection_turmas
		SET version = $1, updated_at = CURRENT_TIMESTAMP, last_event_id = $2
		WHERE id = $3
	`, event.EventVersion, event.EventID, event.AggregateID)
	return err
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
			AND deleted_at IS NULL
		LIMIT 1
	`, codigoTurma, codigoAcademia)
	return scanTurmaRow(row)
}

func (p *TurmasProjection) GetByAcademia(codigoAcademia string) ([]TurmaDTO, error) {
	rows, err := p.client.DB().Query(`
		SELECT id, codigo_turma, codigo_academia, nivel, curso_id, turno,
		       estudantes, status, created_at, updated_at, version
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
// Inclui turmas com deleted_at preenchido para permitir verificação de status
// durante operações de cascata (ex: DeletarCurso verifica t.Status antes de deletar).
func (p *TurmasProjection) ListByCurso(cursoID uuid.UUID) ([]TurmaDTO, error) {
	rows, err := p.client.DB().Query(`
		SELECT id, codigo_turma, codigo_academia, nivel, curso_id, turno,
		       estudantes, status, created_at, updated_at, version
		FROM projection_turmas
		WHERE curso_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
	`, cursoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTurmas(rows)
}

func scanTurmaRow(row *sql.Row) (*TurmaDTO, error) {
	var dto TurmaDTO
	var estudantesJSON []byte
	err := row.Scan(
		&dto.ID, &dto.CodigoTurma, &dto.CodigoAcademia, &dto.Nivel, &dto.CursoID, &dto.Turno,
		&estudantesJSON, &dto.Status, &dto.CreatedAt, &dto.UpdatedAt, &dto.Version,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(estudantesJSON, &dto.Estudantes); err != nil {
		dto.Estudantes = []string{}
	}
	if dto.Estudantes == nil {
		dto.Estudantes = []string{}
	}
	return &dto, nil
}

// scanTurmas — BUG #6 FIX: anteriormente usava `continue` em caso de erro de Scan,
// ignorando silenciosamente linhas corrompidas. Agora retorna o erro explicitamente,
// consistente com o padrão do restante do sistema.
func scanTurmas(rows *sql.Rows) ([]TurmaDTO, error) {
	var turmas []TurmaDTO
	for rows.Next() {
		var dto TurmaDTO
		var estudantesJSON []byte
		if err := rows.Scan(
			&dto.ID, &dto.CodigoTurma, &dto.CodigoAcademia, &dto.Nivel, &dto.CursoID, &dto.Turno,
			&estudantesJSON, &dto.Status, &dto.CreatedAt, &dto.UpdatedAt, &dto.Version,
		); err != nil {
			return nil, fmt.Errorf("erro ao escanear turma: %w", err)
		}
		if err := json.Unmarshal(estudantesJSON, &dto.Estudantes); err != nil {
			dto.Estudantes = []string{}
		}
		if dto.Estudantes == nil {
			dto.Estudantes = []string{}
		}
		turmas = append(turmas, dto)
	}
	return turmas, rows.Err()
}