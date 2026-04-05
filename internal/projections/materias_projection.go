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

// MateriaDTO — projeção lida do banco.
type MateriaDTO struct {
	ID             uuid.UUID  `json:"id"`
	Nome           string     `json:"nome"`
	Type           string     `json:"type"`
	AnosAcademicos []string   `json:"anos_academicos,omitempty"`
	Periodo        *string    `json:"periodo,omitempty"`
	CodigoAcademia string     `json:"codigo_academia"`
	CursoID        *uuid.UUID `json:"curso_id,omitempty"`
	Status         string     `json:"status"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	Version        int        `json:"version"`
}

type MateriasProjection struct {
	client *db.Client
}

func NewMateriasProjection(client *db.Client) *MateriasProjection {
	return &MateriasProjection{client: client}
}

func (p *MateriasProjection) Name() string { return "materias" }

// ============================================================================
// Interface Projection
// ============================================================================

func (p *MateriasProjection) GetLastProcessedEventID() (int64, error) {
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

func (p *MateriasProjection) UpdateCheckpoint(eventID int64) error {
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

func (p *MateriasProjection) Handle(event db.Event) error {
	if event.AggregateType != "MateriaDisciplinar" {
		return nil
	}
	handlers := map[string]func(db.Event) error{
		"MateriaCriada":           p.handleMateriaCriada,
		"MateriaAtivada":          p.handleStatusChange("ativo"),
		"MateriaDesativada":       p.handleStatusChange("inativo"),
		"MateriaDadosAtualizados": p.handleMateriaDadosAtualizados,
		"MateriaPeriodoDefinido":  p.handleMateriaPeriodoDefinido,
		"MateriaDeletada":         p.handleMateriaDeletada,
	}
	if handler, ok := handlers[event.EventType]; ok {
		log.Printf("[DEBUG] Processando %s para matéria %s", event.EventType, event.AggregateID)
		return handler(event)
	}
	return nil
}

func (p *MateriasProjection) Rebuild() error {
	log.Printf("[DEBUG] [materias] Rebuild iniciado")
	if _, err := p.client.DB().Exec(`TRUNCATE TABLE projection_materias CASCADE`); err != nil {
		return fmt.Errorf("falha ao limpar: %w", err)
	}
	rows, err := p.client.DB().Query(`
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE aggregate_type = 'MateriaDisciplinar'
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
	log.Printf("[DEBUG] [materias] Rebuild concluído: %d eventos", count)
	return rows.Err()
}

// ============================================================================
// Handlers de evento
// ============================================================================

func (p *MateriasProjection) handleMateriaCriada(event db.Event) error {
	var payload struct {
		Nome, Type, CodigoAcademia string
		AnosAcademicos              []string
		CursoID                     *uuid.UUID
		CreatedAt                   time.Time
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}
	if event.AggregateID == uuid.Nil {
		return fmt.Errorf("UUID inválido")
	}

	var anosJSON interface{}
	if len(payload.AnosAcademicos) > 0 {
		b, _ := json.Marshal(payload.AnosAcademicos)
		anosJSON = string(b)
	}

	// Matérias superior nascem inativas; demais nascem ativas
	status := "ativo"
	if payload.Type == "superior" {
		status = "inativo"
	}

	var cursoID interface{}
	if payload.CursoID != nil {
		cursoID = payload.CursoID.String()
	}

	_, err := p.client.DB().Exec(`
		INSERT INTO projection_materias
			(id, nome, type, anos_academicos, codigo_academia, curso_id, periodo, status, created_at, updated_at, version, last_event_id)
		VALUES ($1, $2, $3, $4, $5, $6, NULL, $7, $8, CURRENT_TIMESTAMP, $9, $10)
		ON CONFLICT (id) DO NOTHING
	`,
		event.AggregateID, payload.Nome, payload.Type, anosJSON, payload.CodigoAcademia,
		cursoID, status, payload.CreatedAt, event.EventVersion, event.EventID,
	)
	return err
}

func (p *MateriasProjection) handleStatusChange(status string) func(db.Event) error {
	return func(event db.Event) error {
		_, err := p.client.DB().Exec(`
			UPDATE projection_materias
			SET status = $1, updated_at = CURRENT_TIMESTAMP, version = $2, last_event_id = $3
			WHERE id = $4
		`, status, event.EventVersion, event.EventID, event.AggregateID)
		return err
	}
}

func (p *MateriasProjection) handleMateriaDadosAtualizados(event db.Event) error {
	var payload struct {
		Nome      *string
		UpdatedAt time.Time
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}
	if payload.Nome != nil {
		_, err := p.client.DB().Exec(`
			UPDATE projection_materias
			SET nome = $1, updated_at = CURRENT_TIMESTAMP, version = $2, last_event_id = $3
			WHERE id = $4
		`, *payload.Nome, event.EventVersion, event.EventID, event.AggregateID)
		return err
	}
	return nil
}

func (p *MateriasProjection) handleMateriaPeriodoDefinido(event db.Event) error {
	var payload struct {
		Periodo   string
		UpdatedAt time.Time
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}
	_, err := p.client.DB().Exec(`
		UPDATE projection_materias
		SET periodo = $1, updated_at = CURRENT_TIMESTAMP, version = $2, last_event_id = $3
		WHERE id = $4
	`, payload.Periodo, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

func (p *MateriasProjection) handleMateriaDeletada(event db.Event) error {
	var payload struct {
		DeletedAt time.Time `json:"DeletedAt"`
	}
	_ = json.Unmarshal(event.Payload, &payload)
	if payload.DeletedAt.IsZero() {
		payload.DeletedAt = event.OccurredAt
	}
	_, err := p.client.DB().Exec(`
		UPDATE projection_materias
		SET status = 'deletado', deleted_at = $1,
			updated_at = CURRENT_TIMESTAMP, version = $2, last_event_id = $3
		WHERE id = $4
	`, payload.DeletedAt.UTC(), event.EventVersion, event.EventID, event.AggregateID)
	return err
}

// ============================================================================
// Queries de leitura
// ============================================================================

func (p *MateriasProjection) GetByID(id uuid.UUID) (*MateriaDTO, error) {
	if id == uuid.Nil {
		return nil, fmt.Errorf("UUID inválido")
	}
	row := p.client.DB().QueryRow(`
		SELECT id, nome, type, anos_academicos, periodo, codigo_academia, curso_id, status, created_at, updated_at, version
		FROM projection_materias WHERE id = $1
	`, id)
	return scanMateria(row)
}

func (p *MateriasProjection) GetByAcademia(codigoAcademia string) ([]MateriaDTO, error) {
	rows, err := p.client.DB().Query(`
		SELECT id, nome, type, anos_academicos, periodo, codigo_academia, curso_id, status, created_at, updated_at, version
		FROM projection_materias
		WHERE codigo_academia = $1
			AND deleted_at IS NULL
		ORDER BY created_at DESC
	`, codigoAcademia)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result, err := scanMaterias(rows)
	log.Printf("[DEBUG] %d matérias encontradas para academia %s", len(result), codigoAcademia)
	return result, err
}

func (p *MateriasProjection) GetByCurso(cursoID uuid.UUID) ([]MateriaDTO, error) {
	rows, err := p.client.DB().Query(`
		SELECT id, nome, type, anos_academicos, periodo, codigo_academia, curso_id, status, created_at, updated_at, version
		FROM projection_materias
		WHERE curso_id = $1
			AND deleted_at IS NULL
		ORDER BY created_at DESC
	`, cursoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMaterias(rows)
}

func (p *MateriasProjection) GetActiveByAcademia(codigoAcademia string) ([]MateriaDTO, error) {
	rows, err := p.client.DB().Query(`
		SELECT id, nome, type, anos_academicos, periodo, codigo_academia, curso_id, status, created_at, updated_at, version
		FROM projection_materias
		WHERE codigo_academia = $1 AND status = 'ativo' AND deleted_at IS NULL
		ORDER BY created_at DESC
	`, codigoAcademia)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMaterias(rows)
}

func scanMateria(row *sql.Row) (*MateriaDTO, error) {
	var dto MateriaDTO
	var anosJSON sql.NullString
	var cursoID sql.NullString
	var periodo sql.NullString
	err := row.Scan(
		&dto.ID, &dto.Nome, &dto.Type, &anosJSON, &periodo,
		&dto.CodigoAcademia, &cursoID, &dto.Status,
		&dto.CreatedAt, &dto.UpdatedAt, &dto.Version,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if anosJSON.Valid && anosJSON.String != "" {
		json.Unmarshal([]byte(anosJSON.String), &dto.AnosAcademicos)
	}
	if cursoID.Valid {
		cid, _ := uuid.Parse(cursoID.String)
		dto.CursoID = &cid
	}
	if periodo.Valid {
		dto.Periodo = &periodo.String
	}
	return &dto, nil
}

func scanMaterias(rows *sql.Rows) ([]MateriaDTO, error) {
	var result []MateriaDTO
	for rows.Next() {
		var dto MateriaDTO
		var anosJSON sql.NullString
		var cursoID sql.NullString
		var periodo sql.NullString
		if err := rows.Scan(
			&dto.ID, &dto.Nome, &dto.Type, &anosJSON, &periodo,
			&dto.CodigoAcademia, &cursoID, &dto.Status,
			&dto.CreatedAt, &dto.UpdatedAt, &dto.Version,
		); err != nil {
			continue
		}
		if anosJSON.Valid && anosJSON.String != "" {
			json.Unmarshal([]byte(anosJSON.String), &dto.AnosAcademicos)
		}
		if cursoID.Valid {
			cid, _ := uuid.Parse(cursoID.String)
			dto.CursoID = &cid
		}
		if periodo.Valid {
			dto.Periodo = &periodo.String
		}
		result = append(result, dto)
	}
	return result, rows.Err()
}

// nullOrString2 — helper legado para compatibilidade (não mais usado nos handlers)
func nullOrString2(b []byte) string {
	if len(b) == 0 || string(b) == "null" {
		return "NULL"
	}
	value := string(b)
	if !db.SafeString(value) {
		return "NULL"
	}
	return fmt.Sprintf("'%s'", value)
}
