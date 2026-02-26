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

func (p *AprovacaoAnoProjection) Handle(event db.Event) error {
	if event.EventType == "AprovacaoAnoRegistrada" {
		return p.handleAprovacaoAnoRegistrada(event)
	}
	return nil
}

// ============================================================================
// Rebuild
// ============================================================================

func (p *AprovacaoAnoProjection) Rebuild() error {
	if err := p.clear(); err != nil {
		return fmt.Errorf("falha ao limpar: %w", err)
	}

	rows, err := p.client.DB().Query(`
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE event_type = 'AprovacaoAnoRegistrada'
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

	log.Printf("[aprovacao_ano] Rebuild concluído: %d eventos", count)
	return rows.Err()
}

func (p *AprovacaoAnoProjection) GetLastProcessedEventID() (int64, error) {
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

func (p *AprovacaoAnoProjection) UpdateCheckpoint(eventID int64) error {
	eventID = int64(db.ValidateOffset(int(eventID)))
	query := fmt.Sprintf(`
		INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at, events_processed)
		VALUES ('%s', %d, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) DO UPDATE SET
			last_processed_event_id = %d,
			last_processed_at       = CURRENT_TIMESTAMP,
			events_processed        = projection_checkpoints.events_processed + 1
	`, db.SafeString(p.Name()), eventID, eventID)
	_, err := p.client.DB().Exec(query)
	return err
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
		CodigoEstudante string
		CodigoAcademia  string
		AnoLectivo      string
		TipoEnsino      string
		NivelAtual      string
		ProximoNivel    *string
		Aprovado        bool
		Observacao      *string
		RegisteredAt    time.Time
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error AprovacaoAnoRegistrada: %w", err)
	}

	proximoNivelSQL := "NULL"
	if payload.ProximoNivel != nil {
		proximoNivelSQL = fmt.Sprintf("'%s'", db.SafeString(*payload.ProximoNivel))
	}

	observacaoSQL := "NULL"
	if payload.Observacao != nil {
		observacaoSQL = fmt.Sprintf("'%s'", db.SafeString(*payload.Observacao))
	}

	aprovadoSQL := "FALSE"
	if payload.Aprovado {
		aprovadoSQL = "TRUE"
	}

	query := fmt.Sprintf(`
		INSERT INTO projection_aprovacao_ano (
			id, codigo_estudante, codigo_academia, ano_lectivo,
			tipo_ensino, nivel_atual, proximo_nivel,
			aprovado, observacao,
			registered_at, event_id, version
		) VALUES (
			uuid_generate_v4(), '%s', '%s', '%s',
			'%s', '%s', %s,
			%s, %s,
			'%s', '%s', %d
		)
	`,
		db.SafeString(payload.CodigoEstudante),
		db.SafeString(payload.CodigoAcademia),
		db.SafeString(payload.AnoLectivo),
		db.SafeString(payload.TipoEnsino),
		db.SafeString(payload.NivelAtual),
		proximoNivelSQL,
		aprovadoSQL,
		observacaoSQL,
		payload.RegisteredAt.Format(time.RFC3339),
		event.EventID,
		event.EventVersion,
	)

	if _, err := p.client.DB().Exec(query); err != nil {
		return fmt.Errorf("insert aprovacao_ano: %w", err)
	}

	return nil
}

// ============================================================================
// Queries de leitura
// ============================================================================

func (p *AprovacaoAnoProjection) GetByEstudante(codigoEstudante string) ([]AprovacaoDTO, error) {
	return p.queryAprovacoes(fmt.Sprintf(
		"codigo_estudante = '%s' ORDER BY registered_at DESC",
		db.SafeString(codigoEstudante),
	))
}

func (p *AprovacaoAnoProjection) GetByAcademia(codigoAcademia string) ([]AprovacaoDTO, error) {
	return p.queryAprovacoes(fmt.Sprintf(
		"codigo_academia = '%s' ORDER BY registered_at DESC",
		db.SafeString(codigoAcademia),
	))
}

func (p *AprovacaoAnoProjection) GetByEstudanteEAno(codigoEstudante, codigoAcademia, anoLectivo string) ([]AprovacaoDTO, error) {
	return p.queryAprovacoes(fmt.Sprintf(
		"codigo_estudante = '%s' AND codigo_academia = '%s' AND ano_lectivo = '%s' ORDER BY registered_at DESC",
		db.SafeString(codigoEstudante),
		db.SafeString(codigoAcademia),
		db.SafeString(anoLectivo),
	))
}

func (p *AprovacaoAnoProjection) queryAprovacoes(whereClause string) ([]AprovacaoDTO, error) {
	query := fmt.Sprintf(`
		SELECT id, codigo_estudante, codigo_academia, ano_lectivo,
			tipo_ensino, nivel_atual, proximo_nivel,
			aprovado, observacao,
			registered_at, event_id, version
		FROM projection_aprovacao_ano
		WHERE %s
	`, whereClause)

	rows, err := p.client.DB().Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []AprovacaoDTO
	for rows.Next() {
		var dto AprovacaoDTO
		if err := rows.Scan(
			&dto.ID, &dto.CodigoEstudante, &dto.CodigoAcademia, &dto.AnoLectivo,
			&dto.TipoEnsino, &dto.NivelAtual, &dto.ProximoNivel,
			&dto.Aprovado, &dto.Observacao,
			&dto.RegisteredAt, &dto.EventID, &dto.Version,
		); err != nil {
			continue
		}
		result = append(result, dto)
	}

	log.Printf("[aprovacao_ano] %d registros encontrados", len(result))
	return result, rows.Err()
}

// ============================================================================
// DTO
// ============================================================================

type AprovacaoDTO struct {
	ID              uuid.UUID `json:"id"`
	CodigoEstudante string    `json:"codigo_estudante"`
	CodigoAcademia  string    `json:"codigo_academia"`
	AnoLectivo      string    `json:"ano_lectivo"`
	TipoEnsino      string    `json:"tipo_ensino"`
	NivelAtual      string    `json:"nivel_atual"`
	ProximoNivel    *string   `json:"proximo_nivel,omitempty"`
	Aprovado        bool      `json:"aprovado"`
	Observacao      *string   `json:"observacao,omitempty"`
	RegisteredAt    time.Time `json:"registered_at"`
	EventID         uuid.UUID `json:"event_id"`
	Version         int       `json:"version"`

	// Campo calculado para leitura (não armazenado)
	Resultado string `json:"resultado"`
}