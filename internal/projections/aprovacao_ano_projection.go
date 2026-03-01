package projections

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"spuri/internal/db"
	"time"
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

func (p *AprovacaoAnoProjection) UpdateCheckpoint(eventID int64) error {
	eventID = int64(db.ValidateOffset(int(eventID)))
	_, err := p.client.DB().Exec(`
		INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at, events_processed)
		VALUES ($1, $2, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) DO UPDATE SET
			last_processed_event_id = $2,
			last_processed_at       = CURRENT_TIMESTAMP,
			events_processed        = projection_checkpoints.events_processed + 1
	`, p.Name(), eventID)
	return err
}

func (p *AprovacaoAnoProjection) Handle(event db.Event) error {
	if event.AggregateType != "Estudante" {
		return nil
	}
	if event.EventType == "AprovacaoAnoRegistrada" {
		return p.handleAprovacaoAnoRegistrada(event)
	}
	return nil
}

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
	log.Printf("[DEBUG] [aprovacao_ano] Rebuild concluído: %d eventos", count)
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
		TipoEnsino      string    `json:"TipoEnsino"`
		NivelAtual      string    `json:"NivelAtual"`
		ProximoNivel    *string   `json:"ProximoNivel"`
		Aprovado        bool      `json:"Aprovado"`
		Observacao      *string   `json:"Observacao"`
		RegisteredAt    time.Time `json:"RegisteredAt"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error AprovacaoAnoRegistrada: %w", err)
	}

	_, err := p.client.DB().Exec(`
		INSERT INTO projection_aprovacao_ano (
			id, codigo_estudante, codigo_academia, ano_lectivo,
			tipo_ensino, nivel_atual, proximo_nivel,
			aprovado, observacao,
			registered_at, event_id, version
		) VALUES (
			uuid_generate_v4(), $1, $2, $3,
			$4, $5, $6,
			$7, $8,
			$9, $10, $11
		)
	`,
		payload.CodigoEstudante, payload.CodigoAcademia, payload.AnoLectivo,
		payload.TipoEnsino, payload.NivelAtual, payload.ProximoNivel,
		payload.Aprovado, payload.Observacao,
		payload.RegisteredAt, event.EventID, event.EventVersion,
	)
	if err != nil {
		return fmt.Errorf("insert aprovacao_ano: %w", err)
	}
	return nil
}

// ============================================================================
// Queries de leitura
// ============================================================================

type AprovacaoDTO struct {
	ID              string    `json:"id"`
	CodigoEstudante string    `json:"codigo_estudante"`
	CodigoAcademia  string    `json:"codigo_academia"`
	AnoLectivo      string    `json:"ano_lectivo"`
	TipoEnsino      string    `json:"tipo_ensino"`
	NivelAtual      string    `json:"nivel_atual"`
	ProximoNivel    *string   `json:"proximo_nivel,omitempty"`
	Aprovado        bool      `json:"aprovado"`
	Observacao      *string   `json:"observacao,omitempty"`
	RegisteredAt    time.Time `json:"registered_at"`
	EventID         string    `json:"event_id"`
	Version         int       `json:"version"`
}

const aprovacaoSelectCols = `
	id, codigo_estudante, codigo_academia, ano_lectivo,
	tipo_ensino, nivel_atual, proximo_nivel,
	aprovado, observacao, registered_at, event_id, version
`

func (p *AprovacaoAnoProjection) GetByEstudante(codigoEstudante string) ([]AprovacaoDTO, error) {
	rows, err := p.client.DB().Query(
		`SELECT `+aprovacaoSelectCols+` FROM projection_aprovacao_ano
		WHERE codigo_estudante = $1 ORDER BY registered_at DESC`,
		codigoEstudante,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAprovacoes(rows)
}

func (p *AprovacaoAnoProjection) GetByAcademia(codigoAcademia string) ([]AprovacaoDTO, error) {
	rows, err := p.client.DB().Query(
		`SELECT `+aprovacaoSelectCols+` FROM projection_aprovacao_ano
		WHERE codigo_academia = $1 ORDER BY registered_at DESC`,
		codigoAcademia,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAprovacoes(rows)
}

func (p *AprovacaoAnoProjection) GetByEstudanteEAno(codigoEstudante, codigoAcademia, anoLectivo string) ([]AprovacaoDTO, error) {
	rows, err := p.client.DB().Query(
		`SELECT `+aprovacaoSelectCols+` FROM projection_aprovacao_ano
		WHERE codigo_estudante = $1 AND codigo_academia = $2 AND ano_lectivo = $3
		ORDER BY registered_at DESC`,
		codigoEstudante, codigoAcademia, anoLectivo,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAprovacoes(rows)
}

func (p *AprovacaoAnoProjection) GetAll() ([]AprovacaoDTO, error) {
	rows, err := p.client.DB().Query(
		`SELECT ` + aprovacaoSelectCols + ` FROM projection_aprovacao_ano ORDER BY registered_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAprovacoes(rows)
}

func scanAprovacoes(rows *sql.Rows) ([]AprovacaoDTO, error) {
	var result []AprovacaoDTO
	for rows.Next() {
		var dto AprovacaoDTO
		if err := rows.Scan(
			&dto.ID, &dto.CodigoEstudante, &dto.CodigoAcademia, &dto.AnoLectivo,
			&dto.TipoEnsino, &dto.NivelAtual, &dto.ProximoNivel,
			&dto.Aprovado, &dto.Observacao, &dto.RegisteredAt, &dto.EventID, &dto.Version,
		); err != nil {
			continue
		}
		result = append(result, dto)
	}
	return result, rows.Err()
}