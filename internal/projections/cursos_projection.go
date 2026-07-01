package projections

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"time"

	"github.com/google/uuid"
)

type CursosProjection struct {
	client *db.Client
}

func NewCursosProjection(client *db.Client) *CursosProjection {
	return &CursosProjection{client: client}
}

func (p *CursosProjection) Name() string { return "cursos" }

// ============================================================================
// Interface Projection
// ============================================================================

func (p *CursosProjection) GetLastProcessedEventID() (int64, error) {
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

func (p *CursosProjection) UpdateCheckpoint(eventID int64) error {
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

func (p *CursosProjection) Handle(event db.Event) error {
	if event.AggregateType != "Curso" {
		return nil
	}
	switch event.EventType {
	case "CursoCriado":
		return p.handleCursoCriado(event)
	// BUG #B FIX — era "CursoAtualizado" (nome errado):
	case "CursoDadosAtualizados":
		return p.handleCursoDadosAtualizados(event)
	// BUG #B FIX — estavam completamente ausentes:
	case "CursoAtivado":
		return p.handleCursoAtivado(event)
	case "CursoDesativado":
		return p.handleCursoDesativado(event)
	case "CursoDeletado":
		return p.handleCursoDeletado(event)
	}
	return nil
}

func (p *CursosProjection) Rebuild() error {
	log.Printf("[DEBUG] [cursos] Rebuild iniciado")
	if _, err := p.client.DB().Exec(`TRUNCATE TABLE projection_cursos CASCADE`); err != nil {
		return fmt.Errorf("falha ao limpar: %w", err)
	}
	rows, err := p.client.DB().Query(`
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE aggregate_type = 'Curso'
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
	log.Printf("[DEBUG] [cursos] Rebuild concluído: %d eventos", count)
	return rows.Err()
}

// ============================================================================
// Handlers de evento
// ============================================================================

func (p *CursosProjection) handleCursoCriado(event db.Event) error {
	var payload struct {
		Nome           string                             `json:"Nome"`
		Type           string                             `json:"Type"`
		AnosAcademicos []string                           `json:"AnosAcademicos"`
		Periodos       []string                           `json:"Periodos"`
		MateriasChave  []aggregates.MateriasChaveCursoAno `json:"MateriasChave"`
		CodigoAcademia string                             `json:"CodigoAcademia"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error CursoCriado: %w", err)
	}

	anosJSON, _ := json.Marshal(payload.AnosAcademicos)
	materiasChaveJSON, _ := json.Marshal(payload.MateriasChave)
	var periodosJSON interface{}
	if len(payload.Periodos) > 0 {
		b, _ := json.Marshal(payload.Periodos)
		periodosJSON = string(b)
	}

	_, err := p.client.DB().Exec(`
		INSERT INTO projection_cursos (id, nome, type, anos_academicos, periodos, materias_chave, codigo_academia, status, created_at, updated_at, version, last_event_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'ativo', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, $8, $9)
		ON CONFLICT (id) DO NOTHING
	`,
		event.AggregateID, payload.Nome, payload.Type, string(anosJSON), periodosJSON, string(materiasChaveJSON),
		payload.CodigoAcademia, event.EventVersion, event.EventID,
	)
	return err
}

func (p *CursosProjection) handleCursoDadosAtualizados(event db.Event) error {
	var payload struct {
		Nome           *string                             `json:"Nome"`
		Type           *string                             `json:"Type"`
		AnosAcademicos []string                            `json:"AnosAcademicos"`
		Periodos       *[]string                           `json:"Periodos"`
		MateriasChave  *[]aggregates.MateriasChaveCursoAno `json:"MateriasChave"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error CursoDadosAtualizados: %w", err)
	}

	if payload.Nome != nil {
		if _, err := p.client.DB().Exec(
			`UPDATE projection_cursos SET nome = $1 WHERE id = $2`,
			*payload.Nome, event.AggregateID,
		); err != nil {
			return err
		}
	}
	if payload.Type != nil {
		if _, err := p.client.DB().Exec(
			`UPDATE projection_cursos SET type = $1 WHERE id = $2`,
			*payload.Type, event.AggregateID,
		); err != nil {
			return err
		}
	}
	if payload.AnosAcademicos != nil {
		anosJSON, _ := json.Marshal(payload.AnosAcademicos)
		if _, err := p.client.DB().Exec(
			`UPDATE projection_cursos SET anos_academicos = $1 WHERE id = $2`,
			string(anosJSON), event.AggregateID,
		); err != nil {
			return err
		}
	}
	if payload.MateriasChave != nil {
		materiasChaveJSON, _ := json.Marshal(*payload.MateriasChave)
		if _, err := p.client.DB().Exec(`UPDATE projection_cursos SET materias_chave = $1 WHERE id = $2`, string(materiasChaveJSON), event.AggregateID); err != nil {
			return err
		}
	}
	if payload.Periodos != nil {
		if len(*payload.Periodos) == 0 {
			if _, err := p.client.DB().Exec(
				`UPDATE projection_cursos SET periodos = NULL WHERE id = $1`,
				event.AggregateID,
			); err != nil {
				return err
			}
		} else {
			periodosJSON, _ := json.Marshal(*payload.Periodos)
			if _, err := p.client.DB().Exec(
				`UPDATE projection_cursos SET periodos = $1 WHERE id = $2`,
				string(periodosJSON), event.AggregateID,
			); err != nil {
				return err
			}
		}
	}

	_, err := p.client.DB().Exec(`
		UPDATE projection_cursos
		SET version = $1, updated_at = CURRENT_TIMESTAMP, last_event_id = $2
		WHERE id = $3
	`, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

func (p *CursosProjection) handleCursoDeletado(event db.Event) error {
	var payload struct {
		DeletedAt time.Time `json:"DeletedAt"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error CursoDeletado: %w", err)
	}

	_, err := p.client.DB().Exec(`
		UPDATE projection_cursos
		SET status = 'deletado', deleted_at = $1,
			updated_at = CURRENT_TIMESTAMP, version = $2, last_event_id = $3
		WHERE id = $4
	`, payload.DeletedAt.UTC(), event.EventVersion, event.EventID, event.AggregateID)
	return err
}

// Atualiza status para 'ativo' quando Curso.Ativar() é executado.
func (p *CursosProjection) handleCursoAtivado(event db.Event) error {
	_, err := p.client.DB().Exec(`
		UPDATE projection_cursos
		SET status = 'ativo',
			version = $1, updated_at = CURRENT_TIMESTAMP, last_event_id = $2
		WHERE id = $3
	`, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

// Atualiza status para 'inativo' quando Curso.Desativar() é executado.
func (p *CursosProjection) handleCursoDesativado(event db.Event) error {
	_, err := p.client.DB().Exec(`
		UPDATE projection_cursos
		SET status = 'inativo',
			version = $1, updated_at = CURRENT_TIMESTAMP, last_event_id = $2
		WHERE id = $3
	`, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

// ============================================================================
// Queries de leitura
// ============================================================================

type CursoDTO struct {
	ID             uuid.UUID                          `json:"id"`
	Nome           string                             `json:"nome"`
	Type           string                             `json:"type"`
	AnosAcademicos []string                           `json:"anos_academicos"`
	Periodos       []string                           `json:"periodos"`
	MateriasChave  []aggregates.MateriasChaveCursoAno `json:"materias_chave,omitempty"`
	CodigoAcademia string                             `json:"codigo_academia"`
	Status         string                             `json:"status"`
	CreatedAt      time.Time                          `json:"created_at"`
	UpdatedAt      time.Time                          `json:"updated_at"`
	Version        int                                `json:"version"`
}

func (p *CursosProjection) GetByID(id uuid.UUID) (*CursoDTO, error) {
	if id == uuid.Nil {
		return nil, fmt.Errorf("UUID inválido")
	}
	var dto CursoDTO
	var anosJSON []byte
	var periodosJSON []byte
	var materiasChaveJSON []byte
	err := p.client.DB().QueryRow(`
		SELECT id, nome, type, anos_academicos, periodos, materias_chave, codigo_academia, status, created_at, updated_at, version
		FROM projection_cursos WHERE id = $1
	`, id).Scan(
		&dto.ID, &dto.Nome, &dto.Type, &anosJSON, &periodosJSON, &materiasChaveJSON,
		&dto.CodigoAcademia, &dto.Status, &dto.CreatedAt, &dto.UpdatedAt, &dto.Version,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	json.Unmarshal(anosJSON, &dto.AnosAcademicos)
	if periodosJSON != nil {
		json.Unmarshal(periodosJSON, &dto.Periodos)
	}
	if dto.Periodos == nil {
		dto.Periodos = []string{}
	}
	json.Unmarshal(materiasChaveJSON, &dto.MateriasChave)
	if dto.MateriasChave == nil {
		dto.MateriasChave = []aggregates.MateriasChaveCursoAno{}
	}
	return &dto, nil
}

func (p *CursosProjection) GetByAcademia(codigoAcademia string) ([]CursoDTO, error) {
	rows, err := p.client.DB().Query(`
		SELECT id, nome, type, anos_academicos, periodos, materias_chave, codigo_academia, status, created_at, updated_at, version
		FROM projection_cursos
		WHERE codigo_academia = $1
			AND deleted_at IS NULL
		ORDER BY created_at DESC
	`, codigoAcademia)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cursos []CursoDTO
	for rows.Next() {
		var dto CursoDTO
		var anosJSON []byte
		var periodosJSON []byte
		var materiasChaveJSON []byte
		if err := rows.Scan(
			&dto.ID, &dto.Nome, &dto.Type, &anosJSON, &periodosJSON, &materiasChaveJSON,
			&dto.CodigoAcademia, &dto.Status, &dto.CreatedAt, &dto.UpdatedAt, &dto.Version,
		); err != nil {
			continue
		}
		json.Unmarshal(anosJSON, &dto.AnosAcademicos)
		if periodosJSON != nil {
			json.Unmarshal(periodosJSON, &dto.Periodos)
		}
		if dto.Periodos == nil {
			dto.Periodos = []string{}
		}
		json.Unmarshal(materiasChaveJSON, &dto.MateriasChave)
		if dto.MateriasChave == nil {
			dto.MateriasChave = []aggregates.MateriasChaveCursoAno{}
		}
		cursos = append(cursos, dto)
	}
	log.Printf("[DEBUG] %d cursos encontrados para academia %s", len(cursos), codigoAcademia)
	return cursos, rows.Err()
}

func (p *CursosProjection) ListByCurso(cursoID uuid.UUID) ([]CursoDTO, error) {
	rows, err := p.client.DB().Query(`
		SELECT id, nome, type, anos_academicos, periodos, materias_chave, codigo_academia, status, created_at, updated_at, version
		FROM projection_cursos
		WHERE id = $1
			AND (deleted_at IS NULL OR status != 'deletado')
		ORDER BY created_at DESC
	`, cursoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cursos []CursoDTO
	for rows.Next() {
		var dto CursoDTO
		var anosJSON, periodosJSON, materiasChaveJSON []byte
		if err := rows.Scan(
			&dto.ID, &dto.Nome, &dto.Type, &anosJSON, &periodosJSON, &materiasChaveJSON,
			&dto.CodigoAcademia, &dto.Status, &dto.CreatedAt, &dto.UpdatedAt, &dto.Version,
		); err != nil {
			continue
		}
		json.Unmarshal(anosJSON, &dto.AnosAcademicos)
		if periodosJSON != nil {
			json.Unmarshal(periodosJSON, &dto.Periodos)
		}
		if dto.Periodos == nil {
			dto.Periodos = []string{}
		}
		json.Unmarshal(materiasChaveJSON, &dto.MateriasChave)
		if dto.MateriasChave == nil {
			dto.MateriasChave = []aggregates.MateriasChaveCursoAno{}
		}
		cursos = append(cursos, dto)
	}
	return cursos, rows.Err()
}
