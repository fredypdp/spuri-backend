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

// CursoDTO — projeção lida do banco.
type CursoDTO struct {
	ID             uuid.UUID `json:"id"`
	Nome           string    `json:"nome"`
	Type           string    `json:"type"`
	AnosAcademicos []string  `json:"anos_academicos"`
	// Periodos: preenchido apenas para type="superior".
	// Para type="medio" retorna array vazio.
	Periodos       []string  `json:"periodos"`
	CodigoAcademia string    `json:"codigo_academia"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	Version        int       `json:"version"`
}

type CursosProjection struct {
	client *db.Client
}

func NewCursosProjection(client *db.Client) *CursosProjection {
	return &CursosProjection{client: client}
}

func (p *CursosProjection) Name() string { return "cursos" }

func (p *CursosProjection) GetLastProcessedEventID() (int64, error) {
	var lastID int64
	err := p.client.DB().QueryRow(`
		SELECT last_processed_event_id
		FROM projection_checkpoints
		WHERE projection_name = 'cursos'
	`).Scan(&lastID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return lastID, err
}

func (p *CursosProjection) UpdateCheckpoint(eventID int64) error {
	_, err := p.client.DB().Exec(`
		UPDATE projection_checkpoints
		SET last_processed_event_id = $1, last_processed_at = CURRENT_TIMESTAMP
		WHERE projection_name = 'cursos'
	`, eventID)
	return err
}

// Handle processa eventos do ledger e atualiza projection_cursos.
func (p *CursosProjection) Handle(event db.Event) error {
	switch event.EventType {
	case "CursoCriado":
		return p.handleCursoCriado(event)
	case "CursoAtivado":
		return p.handleStatusChange("ativo")(event)
	case "CursoDesativado":
		return p.handleStatusChange("inativo")(event)
	case "CursoDadosAtualizados":
		return p.handleCursoDadosAtualizados(event)
	default:
		return nil
	}
}

func (p *CursosProjection) handleCursoCriado(event db.Event) error {
	var payload struct {
		Nome           string    `json:"Nome"`
		Type           string    `json:"Type"`
		AnosAcademicos []string  `json:"AnosAcademicos"`
		Periodos       []string  `json:"Periodos"`
		CodigoAcademia string    `json:"CodigoAcademia"`
		CreatedAt      time.Time `json:"CreatedAt"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleCursoCriado: unmarshal falhou: %w", err)
	}

	anosJSON, err := json.Marshal(payload.AnosAcademicos)
	if err != nil {
		return fmt.Errorf("handleCursoCriado: marshal anos_academicos falhou: %w", err)
	}

	// Periodos: NULL para medio, JSON array para superior
	periodosExpr := "NULL"
	if payload.Type == "superior" && len(payload.Periodos) > 0 {
		periodosJSON, err := json.Marshal(payload.Periodos)
		if err != nil {
			return fmt.Errorf("handleCursoCriado: marshal periodos falhou: %w", err)
		}
		periodosExpr = fmt.Sprintf("'%s'", db.SafeString(string(periodosJSON)))
	}

	query := fmt.Sprintf(`
		INSERT INTO projection_cursos
			(id, nome, type, anos_academicos, periodos, codigo_academia, status, created_at, updated_at, version, last_event_id)
		VALUES
			('%s', '%s', '%s', '%s', %s, '%s', 'ativo', '%s', CURRENT_TIMESTAMP, %d, '%s')
		ON CONFLICT (id) DO NOTHING
	`,
		event.AggregateID,
		db.SafeString(payload.Nome),
		db.SafeString(payload.Type),
		db.SafeString(string(anosJSON)),
		periodosExpr,
		db.SafeString(payload.CodigoAcademia),
		payload.CreatedAt.Format(time.RFC3339),
		event.EventVersion,
		event.EventID,
	)

	_, err = p.client.DB().Exec(query)
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
		Nome           *string   `json:"Nome"`
		Type           *string   `json:"Type"`
		AnosAcademicos []string  `json:"AnosAcademicos"`
		// Periodos: nil = não alterar; ponteiro para slice = atualizar
		Periodos       *[]string `json:"Periodos"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleCursoDadosAtualizados: unmarshal falhou: %w", err)
	}

	if payload.Nome != nil {
		query := fmt.Sprintf(`UPDATE projection_cursos SET nome = '%s' WHERE id = '%s'`,
			db.SafeString(*payload.Nome), event.AggregateID)
		if _, err := p.client.DB().Exec(query); err != nil {
			return err
		}
	}
	if payload.Type != nil {
		query := fmt.Sprintf(`UPDATE projection_cursos SET type = '%s' WHERE id = '%s'`,
			db.SafeString(*payload.Type), event.AggregateID)
		if _, err := p.client.DB().Exec(query); err != nil {
			return err
		}
	}
	if payload.AnosAcademicos != nil {
		anosJSON, _ := json.Marshal(payload.AnosAcademicos)
		query := fmt.Sprintf(`UPDATE projection_cursos SET anos_academicos = '%s' WHERE id = '%s'`,
			db.SafeString(string(anosJSON)), event.AggregateID)
		if _, err := p.client.DB().Exec(query); err != nil {
			return err
		}
	}
	if payload.Periodos != nil {
		var periodosExpr string
		if len(*payload.Periodos) == 0 {
			periodosExpr = "NULL"
		} else {
			periodosJSON, _ := json.Marshal(*payload.Periodos)
			periodosExpr = fmt.Sprintf("'%s'", db.SafeString(string(periodosJSON)))
		}
		query := fmt.Sprintf(`UPDATE projection_cursos SET periodos = %s WHERE id = '%s'`,
			periodosExpr, event.AggregateID)
		if _, err := p.client.DB().Exec(query); err != nil {
			return err
		}
	}

	query := fmt.Sprintf(`
		UPDATE projection_cursos
		SET version = %d, updated_at = CURRENT_TIMESTAMP, last_event_id = '%s'
		WHERE id = '%s'
	`, event.EventVersion, event.EventID, event.AggregateID)

	_, err := p.client.DB().Exec(query)
	return err
}

// ============================================================================
// Queries
// ============================================================================

func (p *CursosProjection) GetByID(id uuid.UUID) (*CursoDTO, error) {
	if id == uuid.Nil {
		return nil, fmt.Errorf("UUID inválido")
	}

	query := fmt.Sprintf(`
		SELECT id, nome, type, anos_academicos, periodos, codigo_academia, status, created_at, updated_at, version
		FROM projection_cursos WHERE id = '%s'
	`, id)

	var dto CursoDTO
	var anosJSON []byte
	var periodosJSON []byte
	err := p.client.DB().QueryRow(query).Scan(
		&dto.ID, &dto.Nome, &dto.Type, &anosJSON, &periodosJSON,
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
	return &dto, nil
}

func (p *CursosProjection) GetByAcademia(codigoAcademia string) ([]CursoDTO, error) {
	query := fmt.Sprintf(`
		SELECT id, nome, type, anos_academicos, periodos, codigo_academia, status, created_at, updated_at, version
		FROM projection_cursos
		WHERE codigo_academia = '%s'
		ORDER BY created_at DESC
	`, db.SafeString(codigoAcademia))

	rows, err := p.client.DB().Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cursos []CursoDTO
	for rows.Next() {
		var dto CursoDTO
		var anosJSON []byte
		var periodosJSON []byte
		if err := rows.Scan(
			&dto.ID, &dto.Nome, &dto.Type, &anosJSON, &periodosJSON,
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
		cursos = append(cursos, dto)
	}

	log.Printf("[DEBUG] %d cursos encontrados para academia %s", len(cursos), codigoAcademia)
	return cursos, rows.Err()
}

// Rebuild reconstrói a projeção a partir do ledger.
func (p *CursosProjection) Rebuild() error {
	log.Printf("[cursos] Rebuild iniciado")

	if _, err := p.client.DB().Exec(`DELETE FROM projection_cursos`); err != nil {
		return fmt.Errorf("falha ao limpar projection_cursos: %w", err)
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
		if err := rows.Scan(
			&event.ID, &event.EventID, &event.AggregateID, &event.AggregateType,
			&event.EventType, &event.EventVersion, &event.Payload, &event.Metadata,
			&event.OccurredAt, &event.RecordedAt, &event.LedgerHash, &event.PreviousHash,
		); err != nil {
			return err
		}

		if err := p.Handle(event); err != nil {
			log.Printf("[WARN] cursos rebuild: evento %d falhou: %v", event.ID, err)
		}
		count++
	}

	log.Printf("[cursos] Rebuild concluído — %d eventos processados", count)
	return rows.Err()
}