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

	handlers := map[string]func(db.Event) error{
		"CursoCriado":           p.handleCursoCriado,
		"CursoAtivado":          p.handleStatusChange("ativo"),
		"CursoDesativado":       p.handleStatusChange("inativo"),
		"CursoDadosAtualizados": p.handleCursoDadosAtualizados,
	}

	if handler, ok := handlers[event.EventType]; ok {
		log.Printf("[DEBUG] Processando %s para curso %s", event.EventType, event.AggregateID)
		return handler(event)
	}
	return nil
}

func (p *CursosProjection) Rebuild() error {
	log.Printf("[DEBUG] Rebuild iniciado")
	
	if err := p.clear(); err != nil {
		return fmt.Errorf("falha ao limpar: %w", err)
	}

	rows, err := p.client.DB().Query(`
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger WHERE aggregate_type = 'Curso' ORDER BY id ASC
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

func (p *CursosProjection) GetLastProcessedEventID() (int64, error) {
	var lastID int64
	query := fmt.Sprintf(`SELECT last_processed_event_id FROM projection_checkpoints WHERE projection_name = '%s'`,
		db.SafeString(p.Name()))
	
	err := p.client.DB().QueryRow(query).Scan(&lastID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return lastID, err
}

func (p *CursosProjection) UpdateCheckpoint(eventID int64) error {
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

func (p *CursosProjection) clear() error {
	_, err := p.client.DB().Exec(`TRUNCATE TABLE projection_cursos CASCADE`)
	return err
}

func (p *CursosProjection) handleCursoCriado(event db.Event) error {
	var payload struct {
		Nome, Type, CodigoAcademia string
		Nivel                       []string
		CreatedAt                   time.Time
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}

	if event.AggregateID == uuid.Nil {
		return fmt.Errorf("UUID inválido")
	}

	nivelJSON, _ := json.Marshal(payload.Nivel)

	query := fmt.Sprintf(`
		INSERT INTO projection_cursos (id, nome, type, nivel, codigo_academia, status, created_at, updated_at, version, last_event_id)
		VALUES ('%s', '%s', '%s', '%s', '%s', 'ativo', '%s', CURRENT_TIMESTAMP, %d, '%s')
	`, event.AggregateID, db.SafeString(payload.Nome), db.SafeString(payload.Type),
		db.SafeString(string(nivelJSON)), db.SafeString(payload.CodigoAcademia),
		payload.CreatedAt.Format(time.RFC3339), event.EventVersion, event.EventID)

	_, err := p.client.DB().Exec(query)
	return err
}

func (p *CursosProjection) handleStatusChange(status string) func(db.Event) error {
	return func(event db.Event) error {
		if event.AggregateID == uuid.Nil {
			return fmt.Errorf("UUID inválido")
		}

		query := fmt.Sprintf(`
			UPDATE projection_cursos
			SET status = '%s', version = %d, updated_at = CURRENT_TIMESTAMP
			WHERE id = '%s'
		`, status, event.EventVersion, event.AggregateID)
		
		_, err := p.client.DB().Exec(query)
		return err
	}
}

func (p *CursosProjection) handleCursoDadosAtualizados(event db.Event) error {
	var payload struct {
		Nome, Type *string
		Nivel      []string
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}

	if payload.Nome != nil {
		query := fmt.Sprintf(`UPDATE projection_cursos SET nome = '%s' WHERE id = '%s'`,
			db.SafeString(*payload.Nome), event.AggregateID)
		p.client.DB().Exec(query)
	}
	if payload.Type != nil {
		query := fmt.Sprintf(`UPDATE projection_cursos SET type = '%s' WHERE id = '%s'`,
			db.SafeString(*payload.Type), event.AggregateID)
		p.client.DB().Exec(query)
	}
	if payload.Nivel != nil {
		nivelJSON, _ := json.Marshal(payload.Nivel)
		query := fmt.Sprintf(`UPDATE projection_cursos SET nivel = '%s' WHERE id = '%s'`,
			db.SafeString(string(nivelJSON)), event.AggregateID)
		p.client.DB().Exec(query)
	}

	query := fmt.Sprintf(`
		UPDATE projection_cursos
		SET version = %d, updated_at = CURRENT_TIMESTAMP, last_event_id = '%s'
		WHERE id = '%s'
	`, event.EventVersion, event.EventID, event.AggregateID)
	
	_, err := p.client.DB().Exec(query)
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
	err := p.client.DB().QueryRow(query).Scan(&dto.ID, &dto.Nome, &dto.Type, &nivelJSON,
		&dto.CodigoAcademia, &dto.Status, &dto.CreatedAt, &dto.UpdatedAt, &dto.Version)
	
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
	query := fmt.Sprintf(`
		SELECT id, nome, type, nivel, codigo_academia, status, created_at, updated_at, version
		FROM projection_cursos WHERE codigo_academia = '%s' ORDER BY created_at DESC
	`, db.SafeString(codigoAcademia))
	
	rows, err := p.client.DB().Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cursos []CursoDTO
	for rows.Next() {
		var dto CursoDTO
		var nivelJSON []byte
		if err := rows.Scan(&dto.ID, &dto.Nome, &dto.Type, &nivelJSON, &dto.CodigoAcademia,
			&dto.Status, &dto.CreatedAt, &dto.UpdatedAt, &dto.Version); err != nil {
			continue
		}
		json.Unmarshal(nivelJSON, &dto.Nivel)
		cursos = append(cursos, dto)
	}

	log.Printf("[DEBUG] %d cursos encontrados para academia %s", len(cursos), codigoAcademia)
	return cursos, rows.Err()
}

type CursoDTO struct {
	ID             uuid.UUID `json:"id"`
	Nome           string    `json:"nome"`
	Type           string    `json:"type"`
	Nivel          []string  `json:"nivel"`
	CodigoAcademia string    `json:"codigo_academia"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	Version        int       `json:"version"`
}