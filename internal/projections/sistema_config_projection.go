package projections

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"spuri/internal/db"
	"time"

	"github.com/google/uuid"
)

type SistemaConfigProjection struct {
	client *db.Client
	ctx    context.Context
}

func NewSistemaConfigProjection(client *db.Client) *SistemaConfigProjection {
	return &SistemaConfigProjection{client: client, ctx: context.Background()}
}

func (p *SistemaConfigProjection) Name() string { return "sistema_config" }

// ============================================================================
// Interface Projection
// ============================================================================

func (p *SistemaConfigProjection) GetLastProcessedEventID() (int64, error) {
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

func (p *SistemaConfigProjection) UpdateCheckpoint(eventID int64) error {
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

func (p *SistemaConfigProjection) Handle(event db.Event) error {
	if event.AggregateType != "SistemaConfig" {
		return nil
	}
	handlers := map[string]func(db.Event) error{
		"AnoLetivoDefinido": p.handleAnoLetivoDefinido,
	}
	if handler, ok := handlers[event.EventType]; ok {
		log.Printf("[DEBUG] [sistema_config] Processando %s", event.EventType)
		return handler(event)
	}
	return nil
}

// ============================================================================
// Rebuild
// ============================================================================

func (p *SistemaConfigProjection) Rebuild() error {
	log.Printf("[DEBUG] [sistema_config] Rebuild iniciado")
	if err := p.clear(); err != nil {
		return fmt.Errorf("falha ao limpar: %w", err)
	}
	rows, err := p.client.DB().Query(`
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE aggregate_type = 'SistemaConfig'
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
	log.Printf("[DEBUG] [sistema_config] Rebuild concluído: %d eventos", count)
	return rows.Err()
}

func (p *SistemaConfigProjection) clear() error {
	_, err := p.client.DB().Exec(`TRUNCATE TABLE projection_sistema_config CASCADE`)
	return err
}

// ============================================================================
// Handler de evento
// ============================================================================

// handleAnoLetivoDefinido — P3-11: DataInicio e DataFim são *time.Time (ponteiros).
// Eventos legados sem esses campos no JSON terão nil → NULL no banco, em vez
// de time.Time{} (0001-01-01 00:00:00) que corromperia os dados de leitura.
// DefinidoPor também é ponteiro para suportar eventos legados sem o campo.
func (p *SistemaConfigProjection) handleAnoLetivoDefinido(event db.Event) error {
	var payload struct {
		// O campo no aggregate é Valor; AnoLetivo é alias para compatibilidade.
		Valor      string     `json:"Valor"`
		AnoLetivo  string     `json:"AnoLetivo"`   // alias legado
		DataInicio *time.Time `json:"DataInicio"`  // P3-11: ponteiro nil-safe
		DataFim    *time.Time `json:"DataFim"`     // P3-11: ponteiro nil-safe
		DefinidoPor *uuid.UUID `json:"DefinidoPor"` // ponteiro nil-safe
		Observacao *string    `json:"Observacao"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleAnoLetivoDefinido: parse error: %w", err)
	}

	// Resolve o valor efetivo do ano letivo (Valor tem precedência sobre AnoLetivo legado).
	anoLetivo := payload.Valor
	if anoLetivo == "" {
		anoLetivo = payload.AnoLetivo
	}
	if anoLetivo == "" {
		return fmt.Errorf("handleAnoLetivoDefinido: campo Valor/AnoLetivo ausente no payload")
	}

	_, err := p.client.DB().Exec(`
		INSERT INTO projection_sistema_config (
			chave, valor, ano_letivo_atual, data_inicio, data_fim,
			definido_por, observacao, updated_at, event_id, version
		) VALUES (
			'ano_letivo_atual', $1, $1, $2, $3,
			$4, $5, CURRENT_TIMESTAMP, $6, $7
		)
		ON CONFLICT (chave) DO UPDATE SET
			valor            = EXCLUDED.valor,
			ano_letivo_atual = EXCLUDED.ano_letivo_atual,
			data_inicio      = EXCLUDED.data_inicio,
			data_fim         = EXCLUDED.data_fim,
			definido_por     = EXCLUDED.definido_por,
			observacao       = EXCLUDED.observacao,
			updated_at       = CURRENT_TIMESTAMP,
			event_id         = EXCLUDED.event_id,
			version          = EXCLUDED.version
	`,
		anoLetivo, payload.DataInicio, payload.DataFim,
		payload.DefinidoPor, payload.Observacao,
		event.EventID, event.EventVersion,
	)
	if err != nil {
		return fmt.Errorf("handleAnoLetivoDefinido: exec error: %w", err)
	}
	return nil
}

// ============================================================================
// Queries de leitura
// ============================================================================

type SistemaConfigDTO struct {
	Chave          string     `json:"chave"`
	Valor          string     `json:"valor"`
	AnoLetivoAtual string     `json:"ano_letivo_atual"`
	DataInicio     *time.Time `json:"data_inicio,omitempty"`
	DataFim        *time.Time `json:"data_fim,omitempty"`
	DefinidoPor    *uuid.UUID `json:"definido_por,omitempty"`
	Observacao     *string    `json:"observacao,omitempty"`
	UpdatedAt      time.Time  `json:"updated_at"`
	Version        int        `json:"version"`
}

func (p *SistemaConfigProjection) GetAnoLetivoAtual() (*SistemaConfigDTO, error) {
	var dto SistemaConfigDTO
	err := p.client.DB().QueryRow(`
		SELECT chave, valor, ano_letivo_atual, data_inicio, data_fim,
			definido_por, observacao, updated_at, version
		FROM projection_sistema_config
		WHERE chave = 'ano_letivo_atual'
		LIMIT 1
	`).Scan(
		&dto.Chave, &dto.Valor, &dto.AnoLetivoAtual,
		&dto.DataInicio, &dto.DataFim,
		&dto.DefinidoPor, &dto.Observacao,
		&dto.UpdatedAt, &dto.Version,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &dto, err
}

func (p *SistemaConfigProjection) GetByChave(chave string) (*SistemaConfigDTO, error) {
	var dto SistemaConfigDTO
	err := p.client.DB().QueryRow(`
		SELECT chave, valor, ano_letivo_atual, data_inicio, data_fim,
			definido_por, observacao, updated_at, version
		FROM projection_sistema_config
		WHERE chave = $1
		LIMIT 1
	`, chave).Scan(
		&dto.Chave, &dto.Valor, &dto.AnoLetivoAtual,
		&dto.DataInicio, &dto.DataFim,
		&dto.DefinidoPor, &dto.Observacao,
		&dto.UpdatedAt, &dto.Version,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &dto, err
}

// GetValor retorna apenas o valor string de uma chave.
func (p *SistemaConfigProjection) GetValor(chave string) (string, error) {
	var valor string
	err := p.client.DB().QueryRow(`
		SELECT valor FROM projection_sistema_config WHERE chave = $1 LIMIT 1
	`, chave).Scan(&valor)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("configuração '%s' não definida", chave)
	}
	return valor, err
}
