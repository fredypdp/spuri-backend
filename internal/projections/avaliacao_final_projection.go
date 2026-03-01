package projections

import (
	"encoding/json"
	"fmt"
	"log"
	"spuri/internal/db"
	"time"

	"github.com/google/uuid"
)

// AvaliacaoFinalProjection substitui AprovacaoAnoProjection e ReprovacoesProjection.
type AvaliacaoFinalProjection struct {
	client *db.Client
}

func NewAvaliacaoFinalProjection(client *db.Client) *AvaliacaoFinalProjection {
	return &AvaliacaoFinalProjection{client: client}
}

func (p *AvaliacaoFinalProjection) Name() string { return "avaliacao_final" }

// ============================================================================
// Interface Projection
// ============================================================================

func (p *AvaliacaoFinalProjection) GetLastProcessedEventID() (int64, error) {
	var lastID int64
	query := fmt.Sprintf(
		`SELECT last_processed_event_id FROM projection_checkpoints WHERE projection_name = '%s'`,
		db.SafeString(p.Name()),
	)
	err := p.client.DB().QueryRow(query).Scan(&lastID)
	if err != nil {
		return 0, nil
	}
	return lastID, nil
}

func (p *AvaliacaoFinalProjection) UpdateCheckpoint(eventID int64) error {
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

func (p *AvaliacaoFinalProjection) Handle(event db.Event) error {
	if event.AggregateType != "Estudante" {
		return nil
	}
	if event.EventType == "AvaliacaoFinalAnoAcademico" {
		return p.handleAvaliacaoFinal(event)
	}
	return nil
}

func (p *AvaliacaoFinalProjection) Rebuild() error {
	log.Printf("[avaliacao_final] Rebuild iniciado")

	if _, err := p.client.DB().Exec(`DELETE FROM projection_avaliacao_final`); err != nil {
		return fmt.Errorf("falha ao limpar projection_avaliacao_final: %w", err)
	}

	rows, err := p.client.DB().Query(`
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
		       event_version, payload, metadata, occurred_at, recorded_at,
		       ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE aggregate_type = 'Estudante'
		  AND event_type      = 'AvaliacaoFinalAnoAcademico'
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
			log.Printf("[avaliacao_final] rebuild error evento %d: %v", event.ID, err)
		}
		count++
	}

	log.Printf("[avaliacao_final] Rebuild concluído — %d eventos processados", count)
	return rows.Err()
}

// ============================================================================
// Handle interno
// ============================================================================

func (p *AvaliacaoFinalProjection) handleAvaliacaoFinal(event db.Event) error {
	var payload struct {
		ID                  string    `json:"id"`
		CodigoEstudante     string    `json:"codigo_estudante"`
		CodigoAcademia      string    `json:"codigo_academia"`
		AnoLectivo          string    `json:"ano_lectivo"`
		TipoEnsino          string    `json:"tipo_ensino"`
		AnoAcademicoAtual   string    `json:"nivel_ano_academico_atual"`
		ProximoAnoAcademico *string   `json:"proximo_ano_academico"`
		CodigoTurma         *string   `json:"codigo_turma"`
		Aprovado            bool      `json:"aprovado"`
		Observacao          *string   `json:"observacao"`
		RegisteredAt        time.Time `json:"registered_at"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error AvaliacaoFinalAnoAcademico: %w", err)
	}

	rowID := payload.ID
	if rowID == "" {
		rowID = uuid.New().String()
	}

	proximoSQL := "NULL"
	if payload.ProximoAnoAcademico != nil {
		proximoSQL = fmt.Sprintf("'%s'", db.SafeString(*payload.ProximoAnoAcademico))
	}
	observacaoSQL := "NULL"
	if payload.Observacao != nil {
		observacaoSQL = fmt.Sprintf("'%s'", db.SafeString(*payload.Observacao))
	}
	codigoTurmaSQL := "NULL"
	if payload.CodigoTurma != nil {
		codigoTurmaSQL = fmt.Sprintf("'%s'", db.SafeString(*payload.CodigoTurma))
	}

	query := fmt.Sprintf(`
		INSERT INTO projection_avaliacao_final (
			id, event_id, codigo_estudante, codigo_academia,
			ano_lectivo, tipo_ensino, ano_academico_atual, proximo_ano_academico,
			codigo_turma, aprovado, observacao, registered_at, version
		) VALUES (
			'%s', '%s', '%s', '%s',
			'%s', '%s', '%s', %s,
			%s, %t, %s, '%s', %d
		)
		ON CONFLICT (codigo_estudante, codigo_academia, ano_lectivo, tipo_ensino)
		DO UPDATE SET
			id                    = EXCLUDED.id,
			event_id              = EXCLUDED.event_id,
			proximo_ano_academico = EXCLUDED.proximo_ano_academico,
			codigo_turma          = EXCLUDED.codigo_turma,
			aprovado              = EXCLUDED.aprovado,
			observacao            = EXCLUDED.observacao,
			registered_at         = EXCLUDED.registered_at,
			version               = EXCLUDED.version
	`,
		db.SafeString(rowID),
		event.EventID,
		db.SafeString(payload.CodigoEstudante),
		db.SafeString(payload.CodigoAcademia),
		db.SafeString(payload.AnoLectivo),
		db.SafeString(payload.TipoEnsino),
		db.SafeString(payload.AnoAcademicoAtual),
		proximoSQL,
		codigoTurmaSQL,
		payload.Aprovado,
		observacaoSQL,
		payload.RegisteredAt.Format(time.RFC3339),
		event.EventVersion,
	)

	if _, err := p.client.DB().Exec(query); err != nil {
		return fmt.Errorf("erro ao inserir avaliação final: %w", err)
	}

	log.Printf("[avaliacao_final] registrada — estudante=%s tipo=%s aprovado=%v turma=%v",
		payload.CodigoEstudante, payload.TipoEnsino, payload.Aprovado, payload.CodigoTurma)

	return nil
}

// ============================================================================
// DTO
// ============================================================================

type AvaliacaoFinalDTO struct {
	ID                  string    `json:"id"`
	CodigoEstudante     string    `json:"codigo_estudante"`
	CodigoAcademia      string    `json:"codigo_academia"`
	AnoLectivo          string    `json:"ano_lectivo"`
	TipoEnsino          string    `json:"tipo_ensino"`
	AnoAcademicoAtual   string    `json:"ano_academico_atual"`
	ProximoAnoAcademico *string   `json:"proximo_ano_academico,omitempty"`
	CodigoTurma         *string   `json:"codigo_turma,omitempty"`
	Aprovado            bool      `json:"aprovado"`
	Observacao          *string   `json:"observacao,omitempty"`
	RegisteredAt        time.Time `json:"registered_at"`
	Version             int       `json:"version"`
}

// ============================================================================
// Queries de leitura
// ============================================================================

const avaliacaoFinalCols = `
	SELECT id, codigo_estudante, codigo_academia, ano_lectivo, tipo_ensino,
	       ano_academico_atual, proximo_ano_academico, codigo_turma,
	       aprovado, observacao, registered_at, version
	FROM projection_avaliacao_final`

func (p *AvaliacaoFinalProjection) scanAvaliacoes(query string) ([]AvaliacaoFinalDTO, error) {
	rows, err := p.client.DB().Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []AvaliacaoFinalDTO
	for rows.Next() {
		var d AvaliacaoFinalDTO
		if err := rows.Scan(
			&d.ID, &d.CodigoEstudante, &d.CodigoAcademia, &d.AnoLectivo, &d.TipoEnsino,
			&d.AnoAcademicoAtual, &d.ProximoAnoAcademico, &d.CodigoTurma,
			&d.Aprovado, &d.Observacao, &d.RegisteredAt, &d.Version,
		); err != nil {
			return nil, err
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

// GetByEstudante retorna todas as avaliações de um estudante.
func (p *AvaliacaoFinalProjection) GetByEstudante(codigoEstudante string) ([]AvaliacaoFinalDTO, error) {
	query := fmt.Sprintf(`%s WHERE codigo_estudante = '%s' ORDER BY registered_at DESC`,
		avaliacaoFinalCols, db.SafeString(codigoEstudante))
	return p.scanAvaliacoes(query)
}

// GetByAcademia retorna avaliações de uma academia com filtros opcionais.
// tipoEnsino e aprovado são opcionais (nil = sem filtro).
func (p *AvaliacaoFinalProjection) GetByAcademia(codigoAcademia string, tipoEnsino *string, aprovado *bool) ([]AvaliacaoFinalDTO, error) {
	where := fmt.Sprintf("codigo_academia = '%s'", db.SafeString(codigoAcademia))
	if tipoEnsino != nil && *tipoEnsino != "" {
		where += fmt.Sprintf(" AND tipo_ensino = '%s'", db.SafeString(*tipoEnsino))
	}
	if aprovado != nil {
		if *aprovado {
			where += " AND aprovado = TRUE"
		} else {
			where += " AND aprovado = FALSE"
		}
	}
	return p.scanAvaliacoes(fmt.Sprintf("%s WHERE %s ORDER BY registered_at DESC", avaliacaoFinalCols, where))
}

// GetAll retorna todas as avaliações do sistema (uso admin) com filtros opcionais.
func (p *AvaliacaoFinalProjection) GetAll(tipoEnsino *string, aprovado *bool) ([]AvaliacaoFinalDTO, error) {
	where := "1=1"
	if tipoEnsino != nil && *tipoEnsino != "" {
		where += fmt.Sprintf(" AND tipo_ensino = '%s'", db.SafeString(*tipoEnsino))
	}
	if aprovado != nil {
		if *aprovado {
			where += " AND aprovado = TRUE"
		} else {
			where += " AND aprovado = FALSE"
		}
	}
	return p.scanAvaliacoes(fmt.Sprintf("%s WHERE %s ORDER BY registered_at DESC", avaliacaoFinalCols, where))
}

// GetAprovacoes retorna apenas registros aprovado=TRUE.
// codigoAcademia vazio = todas (admin).
func (p *AvaliacaoFinalProjection) GetAprovacoes(codigoAcademia string) ([]AvaliacaoFinalDTO, error) {
	t := true
	if codigoAcademia == "" {
		return p.GetAll(nil, &t)
	}
	return p.GetByAcademia(codigoAcademia, nil, &t)
}

// GetReprovacoes retorna apenas registros aprovado=FALSE.
// codigoAcademia vazio = todas (admin).
func (p *AvaliacaoFinalProjection) GetReprovacoes(codigoAcademia string) ([]AvaliacaoFinalDTO, error) {
	f := false
	if codigoAcademia == "" {
		return p.GetAll(nil, &f)
	}
	return p.GetByAcademia(codigoAcademia, nil, &f)
}