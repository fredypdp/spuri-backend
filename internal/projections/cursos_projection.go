package projections

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"spuri/internal/db"
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

func (p *CursosProjection) Handle(event db.Event) error {
	if event.AggregateType != "Curso" {
		return nil
	}

	switch event.EventType {
	case "CursoCriado":
		return p.handleCursoCriado(event)
	case "CursoAtivado":
		return p.handleCursoAtivado(event)
	case "CursoDesativado":
		return p.handleCursoDesativado(event)
	case "CursoDadosAtualizados":
		return p.handleCursoDadosAtualizados(event)
	}
	return nil
}

func (p *CursosProjection) Rebuild() error {
	if err := p.clear(); err != nil {
		return err
	}

	query := `
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger WHERE aggregate_type = 'Curso' ORDER BY id ASC
	`
	
	rows, err := p.client.DB().Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var event db.Event
		err := rows.Scan(&event.ID, &event.EventID, &event.AggregateID, &event.AggregateType,
			&event.EventType, &event.EventVersion, &event.Payload, &event.Metadata,
			&event.OccurredAt, &event.RecordedAt, &event.LedgerHash, &event.PreviousHash)
		if err != nil {
			return err
		}
		if err := p.Handle(event); err != nil {
			return fmt.Errorf("erro ao processar evento %d: %w", event.ID, err)
		}
	}
	return rows.Err()
}

func (p *CursosProjection) GetLastProcessedEventID() (int64, error) {
	safeName := db.SafeString(p.Name())
	
	query := fmt.Sprintf(`
		SELECT last_processed_event_id 
		FROM projection_checkpoints 
		WHERE projection_name = '%s'
	`, safeName)
	
	var lastID int64
	err := p.client.DB().QueryRow(query).Scan(&lastID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return lastID, err
}

func (p *CursosProjection) UpdateCheckpoint(eventID int64) error {
	safeName := db.SafeString(p.Name())
	eventID = int64(db.ValidateOffset(int(eventID)))
	
	query := fmt.Sprintf(`
		INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at, events_processed)
		VALUES ('%s', %d, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) DO UPDATE SET
			last_processed_event_id = %d, last_processed_at = CURRENT_TIMESTAMP,
			events_processed = projection_checkpoints.events_processed + 1
	`, safeName, eventID, eventID)
	
	_, err := p.client.DB().Exec(query)
	return err
}

func (p *CursosProjection) clear() error {
	_, err := p.client.DB().Exec(`TRUNCATE TABLE projection_cursos CASCADE`)
	return err
}

func (p *CursosProjection) handleCursoCriado(event db.Event) error {
	var payload struct {
		Nome           string    `json:"Nome"`
		Type           string    `json:"Type"`
		Nivel          []string  `json:"Nivel"`
		CodigoAcademia string    `json:"CodigoAcademia"`
		CreatedAt      time.Time `json:"CreatedAt"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}

	nivelJSON, _ := json.Marshal(payload.Nivel)
	aggID := event.AggregateID
	if aggID == uuid.Nil {
		return fmt.Errorf("UUID inválido")
	}

	safeNome := db.SafeString(payload.Nome)
	safeType := db.SafeString(payload.Type)
	safeNivel := db.SafeString(string(nivelJSON))
	safeCodigo := db.SafeString(payload.CodigoAcademia)

	query := fmt.Sprintf(`
		INSERT INTO projection_cursos (id, nome, type, nivel, codigo_academia, status, created_at, updated_at, version, last_event_id)
		VALUES ('%s', '%s', '%s', '%s', '%s', 'ativo', '%s', CURRENT_TIMESTAMP, %d, '%s')
	`, aggID, safeNome, safeType, safeNivel, safeCodigo,
		payload.CreatedAt.Format(time.RFC3339), event.EventVersion, event.EventID)

	_, err := p.client.DB().Exec(query)
	return err
}

func (p *CursosProjection) handleCursoAtivado(event db.Event) error {
	aggID := event.AggregateID
	if aggID == uuid.Nil {
		return fmt.Errorf("UUID inválido")
	}

	query := fmt.Sprintf(`
		UPDATE projection_cursos SET status = 'ativo', version = %d, updated_at = CURRENT_TIMESTAMP WHERE id = '%s'
	`, event.EventVersion, aggID)
	
	_, err := p.client.DB().Exec(query)
	return err
}

func (p *CursosProjection) handleCursoDesativado(event db.Event) error {
	aggID := event.AggregateID
	if aggID == uuid.Nil {
		return fmt.Errorf("UUID inválido")
	}

	query := fmt.Sprintf(`
		UPDATE projection_cursos SET status = 'inativo', version = %d, updated_at = CURRENT_TIMESTAMP WHERE id = '%s'
	`, event.EventVersion, aggID)
	
	_, err := p.client.DB().Exec(query)
	return err
}

func (p *CursosProjection) handleCursoDadosAtualizados(event db.Event) error {
	var payload struct {
		Nome  *string  `json:"Nome"`
		Type  *string  `json:"Type"`
		Nivel []string `json:"Nivel"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}

	aggID := event.AggregateID
	if aggID == uuid.Nil {
		return fmt.Errorf("UUID inválido")
	}

	if payload.Nome != nil {
		safe := db.SafeString(*payload.Nome)
		p.client.DB().Exec(fmt.Sprintf(`UPDATE projection_cursos SET nome = '%s' WHERE id = '%s'`, safe, aggID))
	}
	if payload.Type != nil {
		safe := db.SafeString(*payload.Type)
		p.client.DB().Exec(fmt.Sprintf(`UPDATE projection_cursos SET type = '%s' WHERE id = '%s'`, safe, aggID))
	}
	if payload.Nivel != nil {
		nivelJSON, _ := json.Marshal(payload.Nivel)
		safe := db.SafeString(string(nivelJSON))
		p.client.DB().Exec(fmt.Sprintf(`UPDATE projection_cursos SET nivel = '%s' WHERE id = '%s'`, safe, aggID))
	}

	updateQuery := fmt.Sprintf(`
		UPDATE projection_cursos SET version = %d, updated_at = CURRENT_TIMESTAMP, last_event_id = '%s' WHERE id = '%s'
	`, event.EventVersion, event.EventID, aggID)
	
	_, err := p.client.DB().Exec(updateQuery)
	return err
}

func (p *CursosProjection) GetByID(id uuid.UUID) (*CursoDTO, error) {
	if id == uuid.Nil {
		return nil, fmt.Errorf("UUID inválido")
	}

	query := fmt.Sprintf(`
		SELECT id, nome, type, nivel, codigo_academia, status, created_at, updated_at, version
		FROM projection_cursos WHERE id = '%s'
	`, id)
	
	var dto CursoDTO
	var nivelJSON []byte
	err := p.client.DB().QueryRow(query).Scan(
		&dto.ID, &dto.Nome, &dto.Type, &nivelJSON, &dto.CodigoAcademia,
		&dto.Status, &dto.CreatedAt, &dto.UpdatedAt, &dto.Version)
	
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	json.Unmarshal(nivelJSON, &dto.Nivel)
	return &dto, nil
}

func (p *CursosProjection) GetByAcademia(codigoAcademia string) ([]CursoDTO, error) {
	safeCodigo := db.SafeString(codigoAcademia)

	query := fmt.Sprintf(`
		SELECT id, nome, type, nivel, codigo_academia, status, created_at, updated_at, version
		FROM projection_cursos WHERE codigo_academia = '%s' ORDER BY created_at DESC
	`, safeCodigo)
	
	rows, err := p.client.DB().Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cursos []CursoDTO
	for rows.Next() {
		var dto CursoDTO
		var nivelJSON []byte
		err := rows.Scan(&dto.ID, &dto.Nome, &dto.Type, &nivelJSON, &dto.CodigoAcademia,
			&dto.Status, &dto.CreatedAt, &dto.UpdatedAt, &dto.Version)
		if err != nil {
			continue
		}
		json.Unmarshal(nivelJSON, &dto.Nivel)
		cursos = append(cursos, dto)
	}

	return cursos, rows.Err()
}

type CursoDTO struct {
	ID             uuid.UUID `json:"id" db:"id"`
	Nome           string    `json:"nome" db:"nome"`
	Type           string    `json:"type" db:"type"`
	Nivel          []string  `json:"nivel" db:"nivel"`
	CodigoAcademia string    `json:"codigo_academia" db:"codigo_academia"`
	Status         string    `json:"status" db:"status"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
	Version        int       `json:"version" db:"version"`
}