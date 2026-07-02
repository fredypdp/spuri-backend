package projections

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"spuri/internal/db"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
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
	switch event.EventType {
	case "CategoriaNotaAdicionada":
		log.Printf("[categorias_nota] Processando CategoriaNotaAdicionada: %s", event.EventID)
		return p.handleCategoriaAdicionada(event)
	case "CategoriaNotaRemovida":
		log.Printf("[categorias_nota] Processando CategoriaNotaRemovida: %s", event.EventID)
		return p.handleCategoriaRemovida(event)
	default:
		return nil
	}
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
		WHERE event_type IN ('CategoriaNotaAdicionada', 'CategoriaNotaRemovida')
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

// clear limpa a tabela de categorias de nota.
//
// FIX PROJ-06: substituído TRUNCATE TABLE ... CASCADE por DELETE FROM.
// TRUNCATE CASCADE propaga a deleção para tabelas dependentes via FK, podendo
// silenciosamente destruir dados de outras projeções sem disparar seus rebuilds.
// DELETE FROM remove apenas as linhas desta tabela, sem efeitos colaterais em
// tabelas relacionadas. Como projection_categorias_nota raramente tem dependentes
// críticos em runtime, DELETE é seguro e correto aqui.
func (p *CategoriasNotaProjection) clear() error {
	_, err := p.client.DB().Exec(`DELETE FROM projection_categorias_nota`)
	return err
}

// ============================================================================
// Queries de leitura
// ============================================================================

// CategoriaNotaDTO representa uma categoria de nota cadastrada por uma academia.
type CategoriaNotaDTO struct {
	ID             uuid.UUID  `json:"id"`
	CodigoAcademia string     `json:"codigo_academia"`
	Codigo         string     `json:"codigo"`
	Nome           string     `json:"nome"`
	Descricao      *string    `json:"descricao,omitempty"`
	AnosAcademicos []string   `json:"anos_academicos"`
	AdicionadoPor  *uuid.UUID `json:"adicionado_por,omitempty"`
	Status         string     `json:"-"`
	CreatedAt      time.Time  `json:"created_at"`
	Version        int        `json:"version"`
}

// ListarPorAcademia retorna todas as categorias ativas de uma academia.
// Usado por ListarCategoriasNota e carregarCategoriasAdicionais.
func (p *CategoriasNotaProjection) ListarPorAcademia(codigoAcademia string) ([]CategoriaNotaDTO, error) {
	rows, err := p.client.DB().Query(`
		SELECT id, codigo_academia, codigo, nome, descricao, COALESCE(anos_academicos, '[]'::jsonb), adicionado_por,
			status, created_at, version
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
		var adicionadoPor sql.NullString
		var anosJSON []byte
		if err := rows.Scan(
			&c.ID, &c.CodigoAcademia, &c.Codigo, &c.Nome, &c.Descricao, &anosJSON, &adicionadoPor,
			&c.Status, &c.CreatedAt, &c.Version,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(anosJSON, &c.AnosAcademicos)
		if adicionadoPor.Valid {
			uid, _ := uuid.Parse(adicionadoPor.String)
			c.AdicionadoPor = &uid
		}
		cats = append(cats, c)
	}
	return cats, rows.Err()
}

// GetCodigosByAcademia retorna apenas os códigos das categorias ativas de uma academia.
// Usado por CriarCategoriaNotaSuperior para verificar duplicatas antes de
// emitir o evento — sem expor o DTO completo ao aggregate.
func (p *CategoriasNotaProjection) GetCodigosByAcademia(codigoAcademia string) ([]string, error) {
	rows, err := p.client.DB().Query(`
		SELECT codigo FROM projection_categorias_nota
		WHERE codigo_academia = $1 AND status = 'ativo'
		ORDER BY codigo ASC
	`, codigoAcademia)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var codigos []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		codigos = append(codigos, n)
	}
	return codigos, rows.Err()
}

// ============================================================================
// Handler de evento
// ============================================================================

// handleCategoriaAdicionada persiste o evento CategoriaNotaAdicionada na projeção.
//
// P3-09: lê e persiste AdicionadoPor do payload para auditoria direta na projeção.
//
// FIX: ON CONFLICT DO UPDATE garante que um rebuild sempre sobrescreve dados
// corrompidos (ex: created_at zerado por bug anterior). O campo status não é
// atualizado intencionalmente — é gerenciado por eventos futuros de ativação/
// inativação e não deve ser sobrescrito pelo evento de criação em um rebuild.
func (p *CategoriasNotaProjection) handleCategoriaAdicionada(event db.Event) error {
	var payload struct {
		CodigoAcademia string     `json:"CodigoAcademia"`
		Codigo         string     `json:"Codigo"`
		Nome           string     `json:"Nome"`
		Descricao      *string    `json:"Descricao"`
		AnosAcademicos []string   `json:"AnosAcademicos"`
		AdicionadoPor  *uuid.UUID `json:"AdicionadoPor"`
		CreatedAt      time.Time  `json:"CreatedAt"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleCategoriaAdicionada: parse error: %w", err)
	}

	var adicionadoPor interface{}
	if payload.AdicionadoPor != nil {
		adicionadoPor = payload.AdicionadoPor.String()
	}

	projectionID := categoriaNotaProjectionID(event)

	_, err := p.client.DB().Exec(`
		INSERT INTO projection_categorias_nota (
			id, codigo_academia, codigo, nome, descricao, anos_academicos, adicionado_por,
			status, created_at, event_id, version
		) VALUES ($1, $2, $3, $4, $5, to_jsonb($6::text[]), $7, 'ativo', $8, $9, $10)
		ON CONFLICT (codigo_academia, codigo) DO UPDATE SET
			nome           = EXCLUDED.nome,
			descricao      = EXCLUDED.descricao,
			anos_academicos = EXCLUDED.anos_academicos,
			adicionado_por = EXCLUDED.adicionado_por,
			created_at     = EXCLUDED.created_at,
			event_id       = EXCLUDED.event_id,
			version        = EXCLUDED.version
	`,
		projectionID,
		payload.CodigoAcademia,
		payload.Codigo,
		payload.Nome,
		payload.Descricao,
		pq.Array(payload.AnosAcademicos),
		adicionadoPor,
		payload.CreatedAt,
		event.EventID,
		event.EventVersion,
	)
	if err != nil {
		return fmt.Errorf("handleCategoriaAdicionada: exec error: %w", err)
	}
	return nil
}

func categoriaNotaProjectionID(event db.Event) uuid.UUID {
	return event.EventID
}

func (p *CategoriasNotaProjection) handleCategoriaRemovida(event db.Event) error {
	var payload struct {
		CodigoAcademia string    `json:"CodigoAcademia"`
		Codigo         string    `json:"Codigo"`
		CreatedAt      time.Time `json:"CreatedAt"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleCategoriaRemovida: parse error: %w", err)
	}
	if payload.CodigoAcademia == "" || payload.Codigo == "" {
		return fmt.Errorf("handleCategoriaRemovida: payload inválido")
	}

	_, err := p.client.DB().Exec(`
		UPDATE projection_categorias_nota
		SET status = 'inativo', event_id = $1, version = $2
		WHERE codigo_academia = $3 AND codigo = $4
	`, event.EventID, event.EventVersion, payload.CodigoAcademia, payload.Codigo)
	if err != nil {
		return fmt.Errorf("handleCategoriaRemovida: exec error: %w", err)
	}
	return nil
}
