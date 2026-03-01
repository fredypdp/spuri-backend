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

// ── Handlers de evento ────────────────────────────────────────────────────────

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

	anosJSON, _ := json.Marshal(payload.AnosAcademicos)

	// Matérias superior nascem inativas; demais nascem ativas
	status := "ativo"
	if payload.Type == "superior" {
		status = "inativo"
	}

	query := fmt.Sprintf(`
		INSERT INTO projection_materias
			(id, nome, type, anos_academicos, codigo_academia, curso_id, periodo, status, created_at, updated_at, version, last_event_id)
		VALUES ('%s', '%s', '%s', %s, '%s', %s, NULL, '%s', '%s', CURRENT_TIMESTAMP, %d, '%s')
		ON CONFLICT (id) DO NOTHING
	`,
		event.AggregateID,
		db.SafeString(payload.Nome),
		db.SafeString(payload.Type),
		nullOrString2(anosJSON),
		db.SafeString(payload.CodigoAcademia),
		nullOrUUID(payload.CursoID),
		status,
		payload.CreatedAt.Format("2006-01-02 15:04:05"),
		event.EventVersion,
		event.EventID,
	)

	_, err := p.client.DB().Exec(query)
	return err
}

func (p *MateriasProjection) handleStatusChange(status string) func(db.Event) error {
	return func(event db.Event) error {
		query := fmt.Sprintf(`
			UPDATE projection_materias
			SET status = '%s', updated_at = CURRENT_TIMESTAMP,
			    version = %d, last_event_id = '%s'
			WHERE id = '%s'
		`, status, event.EventVersion, event.EventID, event.AggregateID)
		_, err := p.client.DB().Exec(query)
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
		query := fmt.Sprintf(`
			UPDATE projection_materias
			SET nome = '%s', updated_at = CURRENT_TIMESTAMP,
			    version = %d, last_event_id = '%s'
			WHERE id = '%s'
		`, db.SafeString(*payload.Nome), event.EventVersion, event.EventID, event.AggregateID)
		_, err := p.client.DB().Exec(query)
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

	query := fmt.Sprintf(`
		UPDATE projection_materias
		SET periodo = '%s', updated_at = CURRENT_TIMESTAMP,
		    version = %d, last_event_id = '%s'
		WHERE id = '%s'
	`, db.SafeString(payload.Periodo), event.EventVersion, event.EventID, event.AggregateID)

	_, err := p.client.DB().Exec(query)
	return err
}

func (p *MateriasProjection) handleMateriaDeletada(event db.Event) error {
	var payload struct {
		DeletedAt time.Time `json:"DeletedAt"`
	}
	// Payload pode estar vazio em eventos antigos; usar hora do evento como fallback.
	_ = json.Unmarshal(event.Payload, &payload)
	if payload.DeletedAt.IsZero() {
		payload.DeletedAt = event.OccurredAt
	}

	query := fmt.Sprintf(`
		UPDATE projection_materias
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

// ── Queries ───────────────────────────────────────────────────────────────────

func (p *MateriasProjection) GetByID(id uuid.UUID) (*MateriaDTO, error) {
	if id == uuid.Nil {
		return nil, fmt.Errorf("UUID inválido")
	}

	query := fmt.Sprintf(`
		SELECT id, nome, type, anos_academicos, periodo, codigo_academia, curso_id, status, created_at, updated_at, version
		FROM projection_materias WHERE id = '%s'
	`, id)

	var dto MateriaDTO
	var anosJSON sql.NullString
	var cursoID sql.NullString
	var periodo sql.NullString

	err := p.client.DB().QueryRow(query).Scan(
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

func (p *MateriasProjection) GetByAcademia(codigoAcademia string) ([]MateriaDTO, error) {
	query := fmt.Sprintf(`
		SELECT id, nome, type, anos_academicos, periodo, codigo_academia, curso_id, status, created_at, updated_at, version
		FROM projection_materias
		WHERE codigo_academia = '%s'
		ORDER BY created_at DESC
	`, db.SafeString(codigoAcademia))

	rows, err := p.client.DB().Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var materias []MateriaDTO
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

		materias = append(materias, dto)
	}

	log.Printf("[DEBUG] %d matérias encontradas para academia %s", len(materias), codigoAcademia)
	return materias, rows.Err()
}

func (p *MateriasProjection) GetByCurso(cursoID uuid.UUID) ([]MateriaDTO, error) {
	query := fmt.Sprintf(`
		SELECT id, nome, type, anos_academicos, periodo, codigo_academia, curso_id, status, created_at, updated_at, version
		FROM projection_materias
		WHERE curso_id = '%s'
	`, cursoID)

	rows, err := p.client.DB().Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var materias []MateriaDTO
	for rows.Next() {
		var dto MateriaDTO
		var anosJSON, cursoIDStr, periodo sql.NullString
		err := rows.Scan(
			&dto.ID, &dto.Nome, &dto.Type, &anosJSON, &periodo,
			&dto.CodigoAcademia, &cursoIDStr, &dto.Status,
			&dto.CreatedAt, &dto.UpdatedAt, &dto.Version,
		)
		if err != nil {
			return nil, err
		}
		if anosJSON.Valid {
			json.Unmarshal([]byte(anosJSON.String), &dto.AnosAcademicos)
		}
		if cursoIDStr.Valid {
			cid, _ := uuid.Parse(cursoIDStr.String)
			dto.CursoID = &cid
		}
		if periodo.Valid && periodo.String != "" {
			dto.Periodo = &periodo.String
		}
		materias = append(materias, dto)
	}
	return materias, rows.Err()
}

// ── Checkpoint & Rebuild ──────────────────────────────────────────────────────

func (p *MateriasProjection) GetLastProcessedEventID() (int64, error) {
	var lastID int64
	query := fmt.Sprintf(`SELECT last_processed_event_id FROM projection_checkpoints WHERE projection_name = '%s'`,
		db.SafeString(p.Name()))

	err := p.client.DB().QueryRow(query).Scan(&lastID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return lastID, err
}

func (p *MateriasProjection) UpdateCheckpoint(eventID int64) error {
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

func (p *MateriasProjection) Rebuild() error {
	log.Printf("[DEBUG] Rebuild iniciado")

	if err := p.clear(); err != nil {
		return fmt.Errorf("falha ao limpar: %w", err)
	}

	rows, err := p.client.DB().Query(`
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger WHERE aggregate_type = 'MateriaDisciplinar' ORDER BY id ASC
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

func (p *MateriasProjection) clear() error {
	_, err := p.client.DB().Exec(`TRUNCATE TABLE projection_materias CASCADE`)
	return err
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func nullOrString2(data []byte) string {
	if len(data) == 0 || string(data) == "null" {
		return "NULL"
	}
	return fmt.Sprintf("'%s'", db.SafeString(string(data)))
}