// ============================================================================
// ARQUIVO: internal/projections/sistema_config_projection.go
// Projeção de leitura para configurações do sistema
// ============================================================================

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

func (p *SistemaConfigProjection) GetLastProcessedEventID() (int64, error) {
	var lastID int64
	query := fmt.Sprintf(
		`SELECT last_processed_event_id FROM projection_checkpoints WHERE projection_name = '%s'`,
		db.SafeString(p.Name()),
	)
	err := p.client.DB().QueryRow(query).Scan(&lastID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return lastID, err
}

func (p *SistemaConfigProjection) UpdateCheckpoint(eventID int64) error {
	eventID = int64(db.ValidateOffset(int(eventID)))
	query := fmt.Sprintf(`
		INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at, events_processed)
		VALUES ('%s', %d, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) DO UPDATE SET
			last_processed_event_id = %d,
			last_processed_at = CURRENT_TIMESTAMP,
			events_processed = projection_checkpoints.events_processed + 1
	`, db.SafeString(p.Name()), eventID, eventID)
	_, err := p.client.DB().Exec(query)
	return err
}

func (p *SistemaConfigProjection) clear() error {
	_, err := p.client.DB().Exec(`TRUNCATE TABLE projection_sistema_config`)
	return err
}

// --- Handlers de eventos ---

func (p *SistemaConfigProjection) handleAnoLetivoDefinido(event db.Event) error {
	var payload struct {
		Chave       string    `json:"Chave"`
		Valor       string    `json:"Valor"`
		DefinidoPor uuid.UUID `json:"DefinidoPor"`
		DefinidoEm  time.Time `json:"DefinidoEm"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error: %w", err)
	}

	query := fmt.Sprintf(`
		INSERT INTO projection_sistema_config (chave, valor, updated_by, updated_at, version, last_event_id)
		VALUES ('%s', '%s', '%s', '%s', %d, '%s')
		ON CONFLICT (chave) DO UPDATE SET
			valor         = EXCLUDED.valor,
			updated_by    = EXCLUDED.updated_by,
			updated_at    = EXCLUDED.updated_at,
			version       = projection_sistema_config.version + 1,
			last_event_id = EXCLUDED.last_event_id
	`,
		db.SafeString(payload.Chave),
		db.SafeString(payload.Valor),
		payload.DefinidoPor,
		payload.DefinidoEm.Format("2006-01-02 15:04:05"),
		event.EventVersion,
		event.EventID,
	)

	_, err := p.client.DB().Exec(query)
	if err != nil {
		return fmt.Errorf("erro ao upsert config: %w", err)
	}

	log.Printf("✅ [sistema_config] %s = %s (por %s)", payload.Chave, payload.Valor, payload.DefinidoPor)
	return nil
}

// GetValor retorna o valor de uma chave da projeção.
func (p *SistemaConfigProjection) GetValor(chave string) (string, error) {
	var valor string
	err := p.client.DB().QueryRow(
		`SELECT valor FROM projection_sistema_config WHERE chave = $1`,
		chave,
	).Scan(&valor)

	if err == sql.ErrNoRows {
		return "", fmt.Errorf("configuração '%s' não definida", chave)
	}
	return valor, err
}