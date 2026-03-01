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

type ReprovacoesProjection struct {
	client *db.Client
}

func NewReprovacoesProjection(client *db.Client) *ReprovacoesProjection {
	return &ReprovacoesProjection{client: client}
}

func (p *ReprovacoesProjection) Name() string { return "reprovacoes" }

// ============================================================================
// Interface Projection
// ============================================================================

func (p *ReprovacoesProjection) GetLastProcessedEventID() (int64, error) {
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

func (p *ReprovacoesProjection) UpdateCheckpoint(eventID int64) error {
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

func (p *ReprovacoesProjection) Handle(event db.Event) error {
	if event.AggregateType != "Estudante" {
		return nil
	}
	if event.EventType == "AprovacaoAnoRegistrada" {
		return p.handleAprovacaoAnoRegistrada(event)
	}
	return nil
}

func (p *ReprovacoesProjection) Rebuild() error {
	log.Printf("[reprovacoes] Rebuild iniciado")
	if _, err := p.client.DB().Exec(`DELETE FROM projection_reprovacoes`); err != nil {
		return fmt.Errorf("falha ao limpar projection_reprovacoes: %w", err)
	}
	rows, err := p.client.DB().Query(`
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
		       event_version, payload, metadata, occurred_at, recorded_at,
		       ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE aggregate_type = 'Estudante'
		  AND event_type      = 'AprovacaoAnoRegistrada'
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
	log.Printf("[reprovacoes] Rebuild concluído: %d eventos processados", count)
	return rows.Err()
}

// ============================================================================
// Handler de evento
// ============================================================================

func (p *ReprovacoesProjection) handleAprovacaoAnoRegistrada(event db.Event) error {
	var payload struct {
		CodigoEstudante string    `json:"CodigoEstudante"`
		CodigoAcademia  string    `json:"CodigoAcademia"`
		AnoLectivo      string    `json:"AnoLectivo"`
		TipoEnsino      string    `json:"TipoEnsino"`
		NivelAtual      string    `json:"NivelAtual"`
		Aprovado        bool      `json:"Aprovado"`
		Observacao      *string   `json:"Observacao"`
		RegisteredAt    time.Time `json:"RegisteredAt"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error AprovacaoAnoRegistrada (reprovacoes): %w", err)
	}

	// Apenas registra reprovações
	if payload.Aprovado {
		return nil
	}

	_, err := p.client.DB().Exec(`
		INSERT INTO projection_reprovacoes (
			id, event_id,
			codigo_estudante, codigo_academia,
			ano_lectivo, tipo_ensino, nivel_reprovado, observacao,
			registered_at, version
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`,
		uuid.New(), event.EventID,
		payload.CodigoEstudante, payload.CodigoAcademia,
		payload.AnoLectivo, payload.TipoEnsino, payload.NivelAtual, payload.Observacao,
		payload.RegisteredAt, event.EventVersion,
	)
	if err != nil {
		return fmt.Errorf("erro ao inserir reprovação: %w", err)
	}

	log.Printf("[reprovacoes] Reprovação registrada — estudante=%s nível=%s tipo=%s",
		payload.CodigoEstudante, payload.NivelAtual, payload.TipoEnsino)
	return nil
}

// ============================================================================
// Queries de leitura
// ============================================================================

type ReprovacaoDTO struct {
	ID              string    `json:"id"`
	EventID         string    `json:"event_id"`
	CodigoEstudante string    `json:"codigo_estudante"`
	CodigoAcademia  string    `json:"codigo_academia"`
	AnoLectivo      string    `json:"ano_lectivo"`
	TipoEnsino      string    `json:"tipo_ensino"`
	NivelReprovado  string    `json:"nivel_reprovado"`
	Observacao      *string   `json:"observacao,omitempty"`
	RegisteredAt    time.Time `json:"registered_at"`
	Version         int       `json:"version"`
}

const reprovacaoSelectCols = `
	id, event_id, codigo_estudante, codigo_academia,
	ano_lectivo, tipo_ensino, nivel_reprovado, observacao,
	registered_at, version
`

func (p *ReprovacoesProjection) GetByEstudante(codigoEstudante string) ([]ReprovacaoDTO, error) {
	rows, err := p.client.DB().Query(
		`SELECT `+reprovacaoSelectCols+` FROM projection_reprovacoes
		WHERE codigo_estudante = $1 ORDER BY registered_at DESC`,
		codigoEstudante,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanReprovacoes(rows)
}

func (p *ReprovacoesProjection) GetByAcademia(codigoAcademia string) ([]ReprovacaoDTO, error) {
	rows, err := p.client.DB().Query(
		`SELECT `+reprovacaoSelectCols+` FROM projection_reprovacoes
		WHERE codigo_academia = $1 ORDER BY registered_at DESC`,
		codigoAcademia,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanReprovacoes(rows)
}

func (p *ReprovacoesProjection) GetAll() ([]ReprovacaoDTO, error) {
	rows, err := p.client.DB().Query(
		`SELECT ` + reprovacaoSelectCols + ` FROM projection_reprovacoes ORDER BY registered_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanReprovacoes(rows)
}

func scanReprovacoes(rows *sql.Rows) ([]ReprovacaoDTO, error) {
	var result []ReprovacaoDTO
	for rows.Next() {
		var dto ReprovacaoDTO
		if err := rows.Scan(
			&dto.ID, &dto.EventID, &dto.CodigoEstudante, &dto.CodigoAcademia,
			&dto.AnoLectivo, &dto.TipoEnsino, &dto.NivelReprovado, &dto.Observacao,
			&dto.RegisteredAt, &dto.Version,
		); err != nil {
			continue
		}
		result = append(result, dto)
	}
	return result, rows.Err()
}