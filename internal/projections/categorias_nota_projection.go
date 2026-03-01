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

// ============================================================================
// Interface Projection
// ============================================================================

func (p *CategoriasNotaProjection) GetLastProcessedEventID() (int64, error) {
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

func (p *CategoriasNotaProjection) UpdateCheckpoint(eventID int64) error {
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

	log.Printf("[categorias_nota] Rebuild concluído: %d eventos", count)
	return rows.Err()
}

func (p *CategoriasNotaProjection) clear() error {
	_, err := p.client.DB().Exec(`TRUNCATE TABLE projection_categorias_nota CASCADE`)
	return err
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

	_, err := p.client.DB().Exec(`
		INSERT INTO projection_categorias_nota
			(codigo_academia, nome, descricao, status, created_at, event_id, version)
		VALUES ($1, $2, $3, 'ativo', $4, $5, $6)
		ON CONFLICT (codigo_academia, nome) DO NOTHING
	`,
		payload.CodigoAcademia, payload.Nome, payload.Descricao,
		payload.CreatedAt, event.EventID, event.EventVersion,
	)
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

func (p *CategoriasNotaProjection) ListarPorAcademia(codigoAcademia string) ([]CategoriaNotaDTO, error) {
	rows, err := p.client.DB().Query(`
		SELECT id, codigo_academia, nome, descricao, status, created_at
		FROM projection_categorias_nota
		WHERE codigo_academia = $1 AND status = 'ativo'
		ORDER BY created_at ASC
	`, codigoAcademia)
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

func (p *CategoriasNotaProjection) ExisteCategoria(codigoAcademia, nome string) (bool, error) {
	var count int
	err := p.client.DB().QueryRow(`
		SELECT COUNT(*) FROM projection_categorias_nota
		WHERE codigo_academia = $1 AND nome = $2 AND status = 'ativo'
	`, codigoAcademia, nome).Scan(&count)
	return count > 0, err
}