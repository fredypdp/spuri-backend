// ============================================================================
// ARQUIVO: internal/projections/categorias_nota_projection.go
//
// CORREÇÕES APLICADAS (Etapa 3):
//   P3-09 — handleCategoriaAdicionada: payload agora lê AdicionadoPor (uuid.UUID)
//            e o persiste em projection_categorias_nota.adicionado_por.
//            A coluna adicionado_por é adicionada pela migration 031.
//   P3-10 — Rebuild() não resetava checkpoint explicitamente; o manager cuida
//            disso via markRebuildStart(). Rebuild() direto é seguro porque
//            o TRUNCATE + reprocessamento são suficientes; documentado.
// ============================================================================

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

// ============================================================================
// Handle
// ============================================================================

func (p *CategoriasNotaProjection) Handle(event db.Event) error {
	if event.EventType != "CategoriaNotaAdicionada" {
		return nil
	}
	log.Printf("[categorias_nota] Processando CategoriaNotaAdicionada: %s", event.EventID)
	return p.handleCategoriaAdicionada(event)
}

// ============================================================================
// Rebuild
// ============================================================================

// Rebuild reconstrói a projeção a partir do ledger.
//
// P3-10: o reset do checkpoint é responsabilidade do Manager via markRebuildStart().
// Quando chamado via Manager (caminho normal), o checkpoint é zerado antes do
// Rebuild() e atualizado para MAX(id) depois. Chamadas diretas (ex: testes)
// devem resetar o checkpoint manualmente se necessário.
func (p *CategoriasNotaProjection) Rebuild() error {
	log.Printf("[categorias_nota] Rebuild iniciado")
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
			return fmt.Errorf("erro ao escanear evento %d: %w", count, err)
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

// handleCategoriaAdicionada — P3-09: lê AdicionadoPor do payload e persiste.
func (p *CategoriasNotaProjection) handleCategoriaAdicionada(event db.Event) error {
	var payload struct {
		CodigoAcademia string     `json:"CodigoAcademia"`
		Nome           string     `json:"Nome"`
		Descricao      *string    `json:"Descricao"`
		// P3-09: campo estava ausente — adicionado para rastreabilidade de quem criou.
		AdicionadoPor  uuid.UUID  `json:"AdicionadoPor"`
		CreatedAt      time.Time  `json:"CreatedAt"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleCategoriaAdicionada: parse error: %w", err)
	}

	_, err := p.client.DB().Exec(`
		INSERT INTO projection_categorias_nota (
			id, codigo_academia, nome, descricao, adicionado_por,
			status, created_at, event_id, version
		) VALUES (
			$1, $2, $3, $4, $5,
			'ativo', $6, $7, $8
		)
		ON CONFLICT (codigo_academia, nome) DO NOTHING
	`,
		event.AggregateID,
		payload.CodigoAcademia, payload.Nome, payload.Descricao, payload.AdicionadoPor,
		payload.CreatedAt, event.EventID, event.EventVersion,
	)
	if err != nil {
		return fmt.Errorf("handleCategoriaAdicionada: exec error: %w", err)
	}
	return nil
}

// ============================================================================
// Queries de leitura
// ============================================================================

type CategoriaNotaDTO struct {
	ID             uuid.UUID  `json:"id"`
	CodigoAcademia string     `json:"codigo_academia"`
	Nome           string     `json:"nome"`
	Descricao      *string    `json:"descricao,omitempty"`
	AdicionadoPor  uuid.UUID  `json:"adicionado_por"`
	Status         string     `json:"status"`
	CreatedAt      time.Time  `json:"created_at"`
	EventID        uuid.UUID  `json:"event_id"`
	Version        int        `json:"version"`
}

func (p *CategoriasNotaProjection) GetByAcademia(codigoAcademia string) ([]CategoriaNotaDTO, error) {
	rows, err := p.client.DB().Query(`
		SELECT id, codigo_academia, nome, descricao, adicionado_por,
			status, created_at, event_id, version
		FROM projection_categorias_nota
		WHERE codigo_academia = $1 AND status = 'ativo'
		ORDER BY created_at ASC
	`, codigoAcademia)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cats []CategoriaNotaDTO
	for rows.Next() {
		var c CategoriaNotaDTO
		if err := rows.Scan(
			&c.ID, &c.CodigoAcademia, &c.Nome, &c.Descricao, &c.AdicionadoPor,
			&c.Status, &c.CreatedAt, &c.EventID, &c.Version,
		); err != nil {
			return nil, err
		}
		cats = append(cats, c)
	}
	return cats, rows.Err()
}

func (p *CategoriasNotaProjection) GetNomesByAcademia(codigoAcademia string) ([]string, error) {
	rows, err := p.client.DB().Query(`
		SELECT nome FROM projection_categorias_nota
		WHERE codigo_academia = $1 AND status = 'ativo'
		ORDER BY nome ASC
	`, codigoAcademia)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nomes []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		nomes = append(nomes, n)
	}
	return nomes, rows.Err()
}
