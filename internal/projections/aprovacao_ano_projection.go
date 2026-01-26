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
	if event.EventType != "AprovacaoAnoRegistrada" {
		return nil
	}
	log.Printf("[DEBUG] Processando AprovacaoAnoRegistrada: %s", event.EventID)
	return p.handleAprovacaoAnoRegistrada(event)
}

func (p *AprovacaoAnoProjection) Rebuild() error {
	log.Printf("[DEBUG] Rebuild iniciado")
	
	if err := p.clear(); err != nil {
		return fmt.Errorf("falha ao limpar: %w", err)
	}

	rows, err := p.client.DB().Query(`
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger WHERE event_type = 'AprovacaoAnoRegistrada' ORDER BY id ASC
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var event db.Event
		if err := rows.Scan(&event.ID, &event.EventID, &event.AggregateID, &event.AggregateType,
			&event.EventType, &event.EventVersion, &event.Payload, &event.Metadata,
			&event.OccurredAt, &event.RecordedAt, &event.LedgerHash, &event.PreviousHash); err != nil {
			return err
		}

		if err := p.Handle(event); err != nil {
			return fmt.Errorf("erro no evento %d: %w", event.ID, err)
		}
		count++
	}

	log.Printf("[DEBUG] Rebuild concluído: %d eventos processados", count)
	return rows.Err()
}

func (p *AprovacaoAnoProjection) GetLastProcessedEventID() (int64, error) {
	var lastID int64
	query := fmt.Sprintf(`SELECT last_processed_event_id FROM projection_checkpoints WHERE projection_name = '%s'`,
		db.SafeString(p.Name()))
	
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
			last_processed_event_id = %d, last_processed_at = CURRENT_TIMESTAMP,
			events_processed = projection_checkpoints.events_processed + 1
	`, db.SafeString(p.Name()), eventID, eventID)
	
	_, err := p.client.DB().Exec(query)
	return err
}

func (p *AprovacaoAnoProjection) clear() error {
	_, err := p.client.DB().Exec(`TRUNCATE TABLE projection_aprovacao_ano CASCADE`)
	return err
}

func (p *AprovacaoAnoProjection) handleAprovacaoAnoRegistrada(event db.Event) error {
	var payload struct {
		CodigoEstudante, CodigoAcademia, AnoLectivo, NivelAtual string
		NivelSeguinte                                          *string
		AvancarAno                                             bool
		Observacao                                             *string
		RegisteredAt                                           time.Time
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error: %w", err)
	}

	log.Printf("[DEBUG] Registrando aprovação: %s - Avançar: %v", 
		payload.CodigoEstudante, payload.AvancarAno)

	query := fmt.Sprintf(`
		INSERT INTO projection_aprovacao_ano (
			codigo_estudante, codigo_academia, ano_lectivo, nivel_atual,
			nivel_seguinte, avancar_ano, observacao, registered_at, event_id, version
		) VALUES ('%s', '%s', '%s', '%s', %s, %t, %s, '%s', '%s', %d)
		ON CONFLICT (codigo_estudante, codigo_academia, ano_lectivo)
		DO UPDATE SET nivel_atual = EXCLUDED.nivel_atual, nivel_seguinte = EXCLUDED.nivel_seguinte,
			avancar_ano = EXCLUDED.avancar_ano, observacao = EXCLUDED.observacao,
			registered_at = EXCLUDED.registered_at, event_id = EXCLUDED.event_id, version = EXCLUDED.version
	`, db.SafeString(payload.CodigoEstudante), db.SafeString(payload.CodigoAcademia),
		db.SafeString(payload.AnoLectivo), db.SafeString(payload.NivelAtual),
		nullOrString(payload.NivelSeguinte), payload.AvancarAno,
		nullOrString(payload.Observacao), payload.RegisteredAt.Format(time.RFC3339),
		event.EventID, event.EventVersion)

	_, err := p.client.DB().Exec(query)
	return err
}

func (p *AprovacaoAnoProjection) GetByEstudante(codigoEstudante string) ([]AprovacaoDTO, error) {
	return p.queryAprovacoes(fmt.Sprintf(
		`a.codigo_estudante = '%s' ORDER BY a.registered_at DESC`,
		db.SafeString(codigoEstudante)))
}

func (p *AprovacaoAnoProjection) GetByAcademia(codigoAcademia, anoLectivo string) ([]AprovacaoDTO, error) {
	return p.queryAprovacoes(fmt.Sprintf(
		`a.codigo_academia = '%s' AND a.ano_lectivo = '%s' ORDER BY a.registered_at DESC`,
		db.SafeString(codigoAcademia), db.SafeString(anoLectivo)))
}

func (p *AprovacaoAnoProjection) queryAprovacoes(whereClause string) ([]AprovacaoDTO, error) {
	query := fmt.Sprintf(`
		SELECT a.id, a.codigo_estudante, a.codigo_academia, a.ano_lectivo,
			a.nivel_atual, a.nivel_seguinte, a.avancar_ano, a.observacao,
			a.registered_at, a.event_id, a.version
		FROM projection_aprovacao_ano a
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
		if err := rows.Scan(&dto.ID, &dto.CodigoEstudante, &dto.CodigoAcademia, &dto.AnoLectivo,
			&dto.NivelAtual, &dto.NivelSeguinte, &dto.AvancarAno, &dto.Observacao,
			&dto.RegisteredAt, &dto.EventID, &dto.Version); err != nil {
			continue
		}
		result = append(result, dto)
	}

	log.Printf("[DEBUG] %d aprovações encontradas", len(result))
	return result, rows.Err()
}

type AprovacaoDTO struct {
	ID              uuid.UUID `json:"id"`
	CodigoEstudante string    `json:"codigo_estudante"`
	CodigoAcademia  string    `json:"codigo_academia"`
	AnoLectivo      string    `json:"ano_lectivo"`
	NivelAtual      string    `json:"nivel_atual"`
	NivelSeguinte   *string   `json:"nivel_seguinte,omitempty"`
	AvancarAno      bool      `json:"avancar_ano"`
	Observacao      *string   `json:"observacao,omitempty"`
	RegisteredAt    time.Time `json:"registered_at"`
	EventID         uuid.UUID `json:"event_id"`
	Version         int       `json:"version"`
}