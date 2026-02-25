package projections

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"spuri/internal/db"
	"time"
)

type CategoriasNotaProjection struct {
	client *db.Client
}

func NewCategoriasNotaProjection(client *db.Client) *CategoriasNotaProjection {
	return &CategoriasNotaProjection{client: client}
}

func (p *CategoriasNotaProjection) Name() string { return "categorias_nota" }

func (p *CategoriasNotaProjection) Handle(event db.Event) error {
	if event.EventType != "CategoriaNotaAdicionada" {
		return nil
	}
	return p.handleCategoriaAdicionada(event)
}

// ============================================================================
// Rebuild
// ============================================================================

func (p *CategoriasNotaProjection) Rebuild() error {
	if err := p.clear(); err != nil {
		return fmt.Errorf("falha ao limpar: %w", err)
	}

	rows, err := p.client.DB().Query(`
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE event_type = 'CategoriaNotaAdicionada'
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
			return fmt.Errorf("erro no evento %d: %w", event.ID, err)
		}
		count++
	}

	log.Printf("[categorias_nota] Rebuild concluído: %d eventos", count)
	return rows.Err()
}

// ============================================================================
// Handler de evento
// ============================================================================

func (p *CategoriasNotaProjection) handleCategoriaAdicionada(event db.Event) error {
	var payload struct {
		CodigoAcademia string    `json:"CodigoAcademia"`
		Nome           string    `json:"Nome"`
		Descricao      *string   `json:"Descricao"`
		CreatedAt      time.Time `json:"CreatedAt"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error CategoriaNotaAdicionada: %w", err)
	}

	descricaoSQL := "NULL"
	if payload.Descricao != nil {
		descricaoSQL = fmt.Sprintf("'%s'", db.SafeString(*payload.Descricao))
	}

	query := fmt.Sprintf(`
		INSERT INTO projection_categorias_nota
			(codigo_academia, nome, descricao, status, created_at, event_id, version)
		VALUES
			('%s', '%s', %s, 'ativo', '%s', '%s', %d)
		ON CONFLICT (codigo_academia, nome) DO NOTHING
	`,
		db.SafeString(payload.CodigoAcademia),
		db.SafeString(payload.Nome),
		descricaoSQL,
		payload.CreatedAt.Format("2006-01-02 15:04:05"),
		event.EventID,
		event.EventVersion,
	)

	_, err := p.client.DB().Exec(query)
	return err
}

// ============================================================================
// Queries de leitura
// ============================================================================

type CategoriaNotaDTO struct {
	ID             string  `json:"id"`
	CodigoAcademia string  `json:"codigo_academia"`
	Nome           string  `json:"nome"`
	Descricao      *string `json:"descricao,omitempty"`
	Status         string  `json:"status"`
	CreatedAt      string  `json:"created_at"`
}

// ListarPorAcademia retorna todas as categorias adicionais de uma academia
func (p *CategoriasNotaProjection) ListarPorAcademia(codigoAcademia string) ([]CategoriaNotaDTO, error) {
	query := fmt.Sprintf(`
		SELECT id, codigo_academia, nome, descricao, status, created_at
		FROM projection_categorias_nota
		WHERE codigo_academia = '%s' AND status = 'ativo'
		ORDER BY created_at ASC
	`, db.SafeString(codigoAcademia))

	rows, err := p.client.DB().Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var categorias []CategoriaNotaDTO
	for rows.Next() {
		var c CategoriaNotaDTO
		if err := rows.Scan(&c.ID, &c.CodigoAcademia, &c.Nome, &c.Descricao, &c.Status, &c.CreatedAt); err != nil {
			return nil, err
		}
		categorias = append(categorias, c)
	}
	return categorias, rows.Err()
}

// ExisteCategoria verifica se uma categoria já existe para a academia
func (p *CategoriasNotaProjection) ExisteCategoria(codigoAcademia, nome string) (bool, error) {
	var count int
	query := fmt.Sprintf(`
		SELECT COUNT(*) FROM projection_categorias_nota
		WHERE codigo_academia = '%s' AND nome = '%s'
	`, db.SafeString(codigoAcademia), db.SafeString(nome))

	err := p.client.DB().QueryRow(query).Scan(&count)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return count > 0, err
}

// ============================================================================
// Checkpoint
// ============================================================================

func (p *CategoriasNotaProjection) GetLastProcessedEventID() (int64, error) {
	var lastID int64
	query := fmt.Sprintf(`SELECT last_processed_event_id FROM projection_checkpoints WHERE projection_name = '%s'`,
		db.SafeString(p.Name()))
	err := p.client.DB().QueryRow(query).Scan(&lastID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return lastID, err
}

func (p *CategoriasNotaProjection) UpdateCheckpoint(eventID int64) error {
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

func (p *CategoriasNotaProjection) clear() error {
	_, err := p.client.DB().Exec(`TRUNCATE TABLE projection_categorias_nota CASCADE`)
	return err
}