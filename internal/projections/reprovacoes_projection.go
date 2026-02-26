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

func (p *ReprovacoesProjection) Handle(event db.Event) error {
	if event.AggregateType != "Estudante" || event.EventType != "AprovacaoAnoRegistrada" {
		return nil
	}
	return p.handleReprovacao(event)
}

func (p *ReprovacoesProjection) handleReprovacao(event db.Event) error {
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
		return fmt.Errorf("falha ao deserializar AprovacaoAnoRegistrada: %w", err)
	}

	// Só persiste reprovações
	if payload.Aprovado {
		return nil
	}

	observacaoSQL := "NULL"
	if payload.Observacao != nil {
		observacaoSQL = fmt.Sprintf("'%s'", db.SafeString(*payload.Observacao))
	}

	query := fmt.Sprintf(`
		INSERT INTO projection_reprovacoes
			(id, event_id, codigo_estudante, codigo_academia,
			 ano_lectivo, tipo_ensino, nivel_reprovado, observacao,
			 registered_at, version)
		VALUES
			('%s', '%s', '%s', '%s',
			 '%s', '%s', '%s', %s,
			 '%s', %d)
	`,
		uuid.New(),
		event.EventID,
		db.SafeString(payload.CodigoEstudante),
		db.SafeString(payload.CodigoAcademia),
		db.SafeString(payload.AnoLectivo),
		db.SafeString(payload.TipoEnsino),
		db.SafeString(payload.NivelAtual),
		observacaoSQL,
		payload.RegisteredAt.Format(time.RFC3339),
		event.EventVersion,
	)

	if _, err := p.client.DB().Exec(query); err != nil {
		return fmt.Errorf("erro ao inserir reprovação: %w", err)
	}

	log.Printf("[reprovacoes] Reprovação registrada — estudante=%s nível=%s tipo=%s",
		payload.CodigoEstudante, payload.NivelAtual, payload.TipoEnsino)

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
			log.Printf("[reprovacoes] erro no evento %d: %v", event.ID, err)
		}
		count++
	}

	log.Printf("[reprovacoes] Rebuild concluído: %d eventos processados", count)
	return rows.Err()
}

func (p *ReprovacoesProjection) GetLastProcessedEventID() (int64, error) {
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

func (p *ReprovacoesProjection) UpdateCheckpoint(eventID int64) error {
	eventID = int64(db.ValidateOffset(int(eventID)))
	query := fmt.Sprintf(`
		INSERT INTO projection_checkpoints
			(projection_name, last_processed_event_id, last_processed_at, events_processed)
		VALUES ('%s', %d, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) DO UPDATE SET
			last_processed_event_id = %d,
			last_processed_at       = CURRENT_TIMESTAMP,
			events_processed        = projection_checkpoints.events_processed + 1
	`, db.SafeString(p.Name()), eventID, eventID)
	_, err := p.client.DB().Exec(query)
	return err
}

// ============================================================================
// Queries de leitura
// ============================================================================

type ReprovacaoDTO struct {
	ID              uuid.UUID `json:"id"`
	CodigoEstudante string    `json:"codigo_estudante"`
	CodigoAcademia  string    `json:"codigo_academia"`
	AnoLectivo      string    `json:"ano_lectivo"`
	TipoEnsino      string    `json:"tipo_ensino"`
	NivelReprovado  string    `json:"nivel_reprovado"`
	Observacao      *string   `json:"observacao,omitempty"`
	RegisteredAt    time.Time `json:"registered_at"`
}

func (p *ReprovacoesProjection) GetByEstudante(codigoEstudante string) ([]ReprovacaoDTO, error) {
	query := fmt.Sprintf(`
		SELECT id, codigo_estudante, codigo_academia, ano_lectivo,
		       tipo_ensino, nivel_reprovado, observacao, registered_at
		FROM projection_reprovacoes
		WHERE codigo_estudante = '%s'
		ORDER BY registered_at DESC
	`, db.SafeString(codigoEstudante))

	return p.scanReprovacoes(query)
}

func (p *ReprovacoesProjection) GetByAcademia(codigoAcademia string, tipoEnsino *string) ([]ReprovacaoDTO, error) {
	filter := ""
	if tipoEnsino != nil && *tipoEnsino != "" {
		filter = fmt.Sprintf(` AND tipo_ensino = '%s'`, db.SafeString(*tipoEnsino))
	}

	query := fmt.Sprintf(`
		SELECT id, codigo_estudante, codigo_academia, ano_lectivo,
		       tipo_ensino, nivel_reprovado, observacao, registered_at
		FROM projection_reprovacoes
		WHERE codigo_academia = '%s'%s
		ORDER BY registered_at DESC
	`, db.SafeString(codigoAcademia), filter)

	return p.scanReprovacoes(query)
}

func (p *ReprovacoesProjection) scanReprovacoes(query string) ([]ReprovacaoDTO, error) {
	rows, err := p.client.DB().Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ReprovacaoDTO
	for rows.Next() {
		var dto ReprovacaoDTO
		if err := rows.Scan(
			&dto.ID, &dto.CodigoEstudante, &dto.CodigoAcademia, &dto.AnoLectivo,
			&dto.TipoEnsino, &dto.NivelReprovado, &dto.Observacao, &dto.RegisteredAt,
		); err != nil {
			return nil, err
		}
		result = append(result, dto)
	}
	return result, rows.Err()
}