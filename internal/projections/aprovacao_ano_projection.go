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

type AprovacaoAnoProjection struct {
	client *db.Client
}

func NewAprovacaoAnoProjection(client *db.Client) *AprovacaoAnoProjection {
	return &AprovacaoAnoProjection{client: client}
}

func (p *AprovacaoAnoProjection) Name() string { return "aprovacao_ano" }

// ============================================================================
// Interface Projection
// ============================================================================

// GetLastProcessedEventID — P3-07: substituiu versão com fmt.Sprintf+SafeString(bool).
func (p *AprovacaoAnoProjection) GetLastProcessedEventID() (int64, error) {
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

// UpdateCheckpoint — P3-07: substituiu versão com fmt.Sprintf+SafeString(bool).
func (p *AprovacaoAnoProjection) UpdateCheckpoint(eventID int64) error {
	eventID = int64(db.ValidateOffset(int(eventID)))
	_, err := p.client.DB().Exec(`
		INSERT INTO projection_checkpoints
			(projection_name, last_processed_event_id, last_processed_at, events_processed)
		VALUES ($1, $2, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) DO UPDATE SET
			last_processed_event_id = $2,
			last_processed_at       = CURRENT_TIMESTAMP,
			events_processed        = projection_checkpoints.events_processed + 1
	`, p.Name(), eventID)
	return err
}

// ============================================================================
// Handle
// ============================================================================

func (p *AprovacaoAnoProjection) Handle(event db.Event) error {
	if event.AggregateType != "Estudante" {
		return nil
	}
	if event.EventType == "AprovacaoAnoRegistrada" {
		log.Printf("[DEBUG] [aprovacao_ano] Processando AprovacaoAnoRegistrada: %s", event.EventID)
		return p.handleAprovacaoAnoRegistrada(event)
	}
	return nil
}

// ============================================================================
// Rebuild — P3-08: usa sql.NullString para previous_hash.
// ============================================================================

func (p *AprovacaoAnoProjection) Rebuild() error {
	log.Printf("[DEBUG] [aprovacao_ano] Rebuild iniciado")
	if err := p.clear(); err != nil {
		return fmt.Errorf("falha ao limpar: %w", err)
	}

	rows, err := p.client.DB().Query(`
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE aggregate_type = 'Estudante' AND event_type = 'AprovacaoAnoRegistrada'
		ORDER BY id ASC
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var event db.Event
		// P3-08: sql.NullString para suportar previous_hash = NULL.
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

	log.Printf("[DEBUG] [aprovacao_ano] Rebuild concluído: %d eventos processados", count)
	return rows.Err()
}

func (p *AprovacaoAnoProjection) clear() error {
	_, err := p.client.DB().Exec(`TRUNCATE TABLE projection_aprovacao_ano CASCADE`)
	return err
}

// ============================================================================
// Handler de evento
// ============================================================================

func (p *AprovacaoAnoProjection) handleAprovacaoAnoRegistrada(event db.Event) error {
	var payload struct {
		CodigoEstudante string    `json:"CodigoEstudante"`
		CodigoAcademia  string    `json:"CodigoAcademia"`
		AnoLectivo      string    `json:"AnoLectivo"`
		NivelAtual      string    `json:"NivelAtual"`
		ProximoNivel    *string   `json:"ProximoNivel"`   // FIX: era "NivelSeguinte"
		Aprovado        bool      `json:"Aprovado"`        // FIX: era "AvancarAno"
		Observacao      *string   `json:"Observacao"`
		RegisteredAt    time.Time `json:"RegisteredAt"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleAprovacaoAnoRegistrada: parse error: %w", err)
	}

	_, err := p.client.DB().Exec(`
		INSERT INTO projection_aprovacao_ano (
			id, event_id, codigo_estudante, codigo_academia,
			ano_lectivo, nivel_atual, nivel_seguinte,
			aprovado, observacao, registered_at, version
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7,
			$8, $9, $10, $11
		)
		ON CONFLICT DO NOTHING
	`,
		uuid.New(), event.EventID,
		payload.CodigoEstudante, payload.CodigoAcademia,
		payload.AnoLectivo, payload.NivelAtual, payload.ProximoNivel,
		payload.Aprovado, payload.Observacao, payload.RegisteredAt, event.EventVersion,
	)
	if err != nil {
		return fmt.Errorf("handleAprovacaoAnoRegistrada: exec error: %w", err)
	}
	return nil
}

// ============================================================================
// Queries de leitura
// ============================================================================

type AprovacaoAnoDTO struct {
	ID              uuid.UUID `json:"id"`
	EventID         uuid.UUID `json:"event_id"`
	CodigoEstudante string    `json:"codigo_estudante"`
	CodigoAcademia  string    `json:"codigo_academia"`
	AnoLectivo      string    `json:"ano_lectivo"`
	NivelAtual      string    `json:"nivel_atual"`
	NivelSeguinte   *string   `json:"nivel_seguinte,omitempty"`
	Aprovado        bool      `json:"aprovado"`
	Observacao      *string   `json:"observacao,omitempty"`
	RegisteredAt    time.Time `json:"registered_at"`
	Version         int       `json:"version"`
}

const aprovacaoCols = `
	id, event_id, codigo_estudante, codigo_academia,
	ano_lectivo, nivel_atual, nivel_seguinte,
	aprovado, observacao, registered_at, version
`

func scanAprovacao(rows *sql.Rows) ([]AprovacaoAnoDTO, error) {
	var result []AprovacaoAnoDTO
	for rows.Next() {
		var a AprovacaoAnoDTO
		if err := rows.Scan(
			&a.ID, &a.EventID, &a.CodigoEstudante, &a.CodigoAcademia,
			&a.AnoLectivo, &a.NivelAtual, &a.NivelSeguinte,
			&a.Aprovado, &a.Observacao, &a.RegisteredAt, &a.Version,
		); err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

func (p *AprovacaoAnoProjection) GetByEstudante(codigoEstudante string) ([]AprovacaoAnoDTO, error) {
	rows, err := p.client.DB().Query(
		`SELECT `+aprovacaoCols+`
		FROM projection_aprovacao_ano
		WHERE codigo_estudante = $1
		ORDER BY registered_at DESC`,
		codigoEstudante,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAprovacao(rows)
}

func (p *AprovacaoAnoProjection) GetByAcademia(codigoAcademia string) ([]AprovacaoAnoDTO, error) {
	rows, err := p.client.DB().Query(
		`SELECT `+aprovacaoCols+`
		FROM projection_aprovacao_ano
		WHERE codigo_academia = $1
		ORDER BY registered_at DESC`,
		codigoAcademia,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAprovacao(rows)
}