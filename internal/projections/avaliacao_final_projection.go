package projections

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"spuri/internal/db"
	"strings"
	"time"

	"github.com/google/uuid"
)

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
	err := p.client.DB().QueryRow(
		`SELECT last_processed_event_id FROM projection_checkpoints WHERE projection_name = $1`,
		p.Name(),
	).Scan(&lastID)
	if err != nil {
		return 0, nil
	}
	return lastID, nil
}

func (p *AvaliacaoFinalProjection) UpdateCheckpoint(eventID int64) error {
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

func (p *AvaliacaoFinalProjection) Handle(event db.Event) error {
	if event.AggregateType != "Estudante" {
		return nil
	}
	if event.EventType == "AvaliacaoFinalEscolar" || event.EventType == "AvaliacaoFinalSuperior" {
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
		WHERE aggregate_type = 'Estudante' AND event_type IN ('AvaliacaoFinalEscolar','AvaliacaoFinalSuperior')
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
	log.Printf("[avaliacao_final] Rebuild concluído: %d eventos", count)
	return rows.Err()
}

// ============================================================================
// Handler de evento
// ============================================================================

func (p *AvaliacaoFinalProjection) handleAvaliacaoFinal(event db.Event) error {
	payload, err := parseAvaliacaoFinalPayload(event.Payload)
	if err != nil {
		return fmt.Errorf("parse error AvaliacaoFinalAnoAcademico: %w", err)
	}

	_, err = p.client.DB().Exec(`
		INSERT INTO projection_avaliacao_final (
			id, event_id,
			codigo_estudante, codigo_academia,
			ano_lectivo, tipo_ensino,
			ano_academico_atual, proximo_ano_academico,
			aprovado, observacao, type, nota_final, nota_minima_aprovacao,
			regra_avaliacao_final_id, formula_snapshot, aplica_se_reprovado_em_type,
			registered_at, version
		) VALUES (
			uuid_generate_v4(), $1,
			$2, $3,
			$4, $5,
			$6, $7,
			$8, $9, $10, $11, $12,
			$13, $14, $15,
			CURRENT_TIMESTAMP, $16
		)
		ON CONFLICT (codigo_estudante, codigo_academia, ano_lectivo, tipo_ensino, ano_academico_atual, type)
		DO NOTHING
	`,
		event.EventID,
		payload.CodigoEstudante, payload.CodigoAcademia,
		payload.AnoLectivo, payload.TipoEnsino,
		payload.AnoAcademicoAtual, payload.ProximoAnoAcademico,
		payload.Aprovado, payload.Observacao, payload.Type, payload.NotaFinal, payload.NotaMinimaAprovacao,
		payload.RegraAvaliacaoFinalID, payload.FormulaSnapshot, payload.AplicaSeReprovadoEmType,
		event.EventVersion,
	)
	if err != nil {
		return fmt.Errorf("upsert avaliacao_final: %w", err)
	}
	log.Printf("[avaliacao_final] Avaliação registrada — estudante=%s tipo=%s aprovado=%v",
		payload.CodigoEstudante, payload.TipoEnsino, payload.Aprovado)
	return nil
}

type avaliacaoFinalPayload struct {
	CodigoEstudante         string
	CodigoAcademia          string
	AnoLectivo              string
	TipoEnsino              string
	AnoAcademicoAtual       string
	ProximoAnoAcademico     *string
	Aprovado                bool
	Observacao              *string
	Type                    string
	NotaFinal               float64
	NotaMinimaAprovacao     float64
	RegraAvaliacaoFinalID   *uuid.UUID
	FormulaSnapshot         json.RawMessage
	AplicaSeReprovadoEmType *string
}

func parseAvaliacaoFinalPayload(raw json.RawMessage) (avaliacaoFinalPayload, error) {
	var snake struct {
		CodigoEstudante         string          `json:"codigo_estudante"`
		CodigoAcademia          string          `json:"codigo_academia"`
		AnoLectivo              string          `json:"ano_lectivo"`
		TipoEnsino              string          `json:"tipo_ensino"`
		AnoAcademicoAtual       string          `json:"nivel_ano_academico_atual"`
		ProximoAnoAcademico     *string         `json:"proximo_ano_academico"`
		Aprovado                bool            `json:"aprovado"`
		Observacao              *string         `json:"observacao"`
		Type                    string          `json:"type"`
		NotaFinal               float64         `json:"nota_final"`
		NotaMinimaAprovacao     float64         `json:"nota_minima_aprovacao"`
		RegraAvaliacaoFinalID   *uuid.UUID      `json:"regra_avaliacao_final_id"`
		FormulaSnapshot         json.RawMessage `json:"formula_snapshot"`
		AplicaSeReprovadoEmType *string         `json:"aplica_se_reprovado_em_type"`
	}
	if err := json.Unmarshal(raw, &snake); err != nil {
		return avaliacaoFinalPayload{}, err
	}

	// Compatibilidade com payloads legados em PascalCase.
	var legacy struct {
		CodigoEstudante         string          `json:"CodigoEstudante"`
		CodigoAcademia          string          `json:"CodigoAcademia"`
		AnoLectivo              string          `json:"AnoLectivo"`
		TipoEnsino              string          `json:"TipoEnsino"`
		AnoAcademicoAtual       string          `json:"AnoAcademicoAtual"`
		ProximoAnoAcademico     *string         `json:"ProximoAnoAcademico"`
		Aprovado                bool            `json:"Aprovado"`
		Observacao              *string         `json:"Observacao"`
		Type                    string          `json:"Type"`
		NotaFinal               float64         `json:"NotaFinal"`
		NotaMinimaAprovacao     float64         `json:"NotaMinimaAprovacao"`
		RegraAvaliacaoFinalID   *uuid.UUID      `json:"RegraAvaliacaoFinalID"`
		FormulaSnapshot         json.RawMessage `json:"FormulaSnapshot"`
		AplicaSeReprovadoEmType *string         `json:"AplicaSeReprovadoEmType"`
	}
	if err := json.Unmarshal(raw, &legacy); err != nil {
		return avaliacaoFinalPayload{}, err
	}

	payload := avaliacaoFinalPayload{
		CodigoEstudante:         firstNonEmpty(snake.CodigoEstudante, legacy.CodigoEstudante),
		CodigoAcademia:          firstNonEmpty(snake.CodigoAcademia, legacy.CodigoAcademia),
		AnoLectivo:              firstNonEmpty(snake.AnoLectivo, legacy.AnoLectivo),
		TipoEnsino:              firstNonEmpty(snake.TipoEnsino, legacy.TipoEnsino),
		AnoAcademicoAtual:       firstNonEmpty(snake.AnoAcademicoAtual, legacy.AnoAcademicoAtual),
		ProximoAnoAcademico:     snake.ProximoAnoAcademico,
		Aprovado:                snake.Aprovado || legacy.Aprovado,
		Observacao:              snake.Observacao,
		Type:                    firstNonEmpty(snake.Type, legacy.Type, "normal"),
		NotaFinal:               firstNonZero(snake.NotaFinal, legacy.NotaFinal),
		NotaMinimaAprovacao:     firstNonZero(snake.NotaMinimaAprovacao, legacy.NotaMinimaAprovacao),
		RegraAvaliacaoFinalID:   snake.RegraAvaliacaoFinalID,
		FormulaSnapshot:         snake.FormulaSnapshot,
		AplicaSeReprovadoEmType: snake.AplicaSeReprovadoEmType,
	}
	if payload.ProximoAnoAcademico == nil {
		payload.ProximoAnoAcademico = legacy.ProximoAnoAcademico
	}
	if payload.Observacao == nil {
		payload.Observacao = legacy.Observacao
	}
	if payload.RegraAvaliacaoFinalID == nil {
		payload.RegraAvaliacaoFinalID = legacy.RegraAvaliacaoFinalID
	}
	if len(payload.FormulaSnapshot) == 0 {
		payload.FormulaSnapshot = legacy.FormulaSnapshot
	}
	if payload.AplicaSeReprovadoEmType == nil {
		payload.AplicaSeReprovadoEmType = legacy.AplicaSeReprovadoEmType
	}
	return payload, nil
}

func firstNonZero(values ...float64) float64 {
	for _, v := range values {
		if v != 0 {
			return v
		}
	}
	return 0
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// ============================================================================
// Queries de leitura
// ============================================================================

type AvaliacaoFinalDTO struct {
	ID                      uuid.UUID       `json:"id"`
	EventID                 uuid.UUID       `json:"event_id"`
	CodigoEstudante         string          `json:"codigo_estudante"`
	CodigoAcademia          string          `json:"codigo_academia"`
	AnoLectivo              string          `json:"ano_lectivo"`
	TipoEnsino              string          `json:"tipo_ensino"`
	AnoAcademicoAtual       string          `json:"ano_academico_atual"`
	ProximoAnoAcademico     *string         `json:"proximo_ano_academico,omitempty"`
	Aprovado                bool            `json:"aprovado"`
	Observacao              *string         `json:"observacao,omitempty"`
	Type                    string          `json:"type"`
	NotaFinal               *float64        `json:"nota_final,omitempty"`
	NotaMinimaAprovacao     *float64        `json:"nota_minima_aprovacao,omitempty"`
	RegraAvaliacaoFinalID   *uuid.UUID      `json:"regra_avaliacao_final_id,omitempty"`
	FormulaSnapshot         json.RawMessage `json:"formula_snapshot,omitempty"`
	AplicaSeReprovadoEmType *string         `json:"aplica_se_reprovado_em_type,omitempty"`
	RegisteredAt            time.Time       `json:"registered_at"`
	Version                 int             `json:"version"`
}

type AvaliacaoFinalFilters struct {
	TipoEnsino        *string
	AnoLectivo        *string
	AnoAcademicoAtual *string
	CodigoTurma       *string
	Type              *string
}

const avaliacaoFinalCols = `
	id, event_id, codigo_estudante, codigo_academia,
	ano_lectivo, tipo_ensino, ano_academico_atual, proximo_ano_academico,
	aprovado, observacao, type, nota_final, nota_minima_aprovacao, regra_avaliacao_final_id, formula_snapshot, aplica_se_reprovado_em_type, registered_at, version
`

func (p *AvaliacaoFinalProjection) ExistsByEstudanteAnoLetivoNivel(codigoEstudante, codigoAcademia, anoLectivo, tipoEnsino, anoAcademicoAtual string) (bool, error) {
	var exists bool
	err := p.client.DB().QueryRow(`
		SELECT EXISTS (
			SELECT 1
			FROM projection_avaliacao_final
			WHERE codigo_estudante = $1
			  AND codigo_academia = $2
			  AND ano_lectivo = $3
			  AND (
				($4 = 'superior' AND tipo_ensino = 'superior')
				OR ($4 <> 'superior' AND tipo_ensino <> 'superior')
			  )
			  AND ano_academico_atual = $5
		)`,
		codigoEstudante, codigoAcademia, anoLectivo, tipoEnsino, anoAcademicoAtual,
	).Scan(&exists)
	return exists, err
}

func (p *AvaliacaoFinalProjection) ExistsByEstudanteAnoLetivo(codigoEstudante, codigoAcademia, anoLectivo string) (bool, error) {
	var exists bool
	err := p.client.DB().QueryRow(`
		SELECT EXISTS (
			SELECT 1
			FROM projection_avaliacao_final
			WHERE codigo_estudante = $1
			  AND codigo_academia = $2
			  AND ano_lectivo = $3
		)`,
		codigoEstudante, codigoAcademia, anoLectivo,
	).Scan(&exists)
	return exists, err
}

func (p *AvaliacaoFinalProjection) GetByEstudante(codigoEstudante string) ([]AvaliacaoFinalDTO, error) {
	rows, err := p.client.DB().Query(
		`SELECT `+avaliacaoFinalCols+` FROM projection_avaliacao_final
		WHERE codigo_estudante = $1 ORDER BY registered_at DESC`,
		codigoEstudante,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAvaliacoes(rows)
}

// GetByAcademia retorna avaliações de uma academia.
// tipoEnsino e aprovado são opcionais (nil = sem filtro).
func (p *AvaliacaoFinalProjection) GetByAcademia(codigoAcademia string, tipoEnsino *string, aprovado *bool) ([]AvaliacaoFinalDTO, error) {
	return p.ListByFilters(&codigoAcademia, aprovado, AvaliacaoFinalFilters{
		TipoEnsino: tipoEnsino,
	})
}

// GetAll retorna todas as avaliações do sistema (uso admin) com filtros opcionais.
func (p *AvaliacaoFinalProjection) GetAll(tipoEnsino *string, aprovado *bool) ([]AvaliacaoFinalDTO, error) {
	return p.ListByFilters(nil, aprovado, AvaliacaoFinalFilters{
		TipoEnsino: tipoEnsino,
	})
}

func (p *AvaliacaoFinalProjection) ListByFilters(codigoAcademiaEscopo *string, aprovado *bool, filtros AvaliacaoFinalFilters) ([]AvaliacaoFinalDTO, error) {
	query := `SELECT ` + avaliacaoFinalCols + ` FROM projection_avaliacao_final avf`
	conditions := make([]string, 0, 8)
	args := make([]interface{}, 0, 8)
	add := func(sql string, value interface{}) {
		args = append(args, value)
		conditions = append(conditions, fmt.Sprintf(sql, len(args)))
	}

	if codigoAcademiaEscopo != nil && *codigoAcademiaEscopo != "" {
		add("avf.codigo_academia = $%d", *codigoAcademiaEscopo)
	}
	if filtros.TipoEnsino != nil && *filtros.TipoEnsino != "" {
		add("avf.tipo_ensino = $%d", *filtros.TipoEnsino)
	}
	if filtros.AnoLectivo != nil && *filtros.AnoLectivo != "" {
		add("avf.ano_lectivo = $%d", *filtros.AnoLectivo)
	}
	if filtros.AnoAcademicoAtual != nil && *filtros.AnoAcademicoAtual != "" {
		add("avf.ano_academico_atual = $%d", *filtros.AnoAcademicoAtual)
	}
	if filtros.Type != nil && *filtros.Type != "" {
		add("avf.type = $%d", *filtros.Type)
	}
	if aprovado != nil {
		if *aprovado {
			conditions = append(conditions, "avf.aprovado = TRUE")
		} else {
			conditions = append(conditions, "avf.aprovado = FALSE")
			conditions = append(conditions, condicaoReprovacaoDefinitiva())
		}
	}
	if filtros.CodigoTurma != nil && *filtros.CodigoTurma != "" {
		args = append(args, *filtros.CodigoTurma)
		conditions = append(conditions, fmt.Sprintf(`
EXISTS (
	SELECT 1
	FROM projection_turmas t
	WHERE t.deleted_at IS NULL
	  AND t.codigo_academia = avf.codigo_academia
	  AND t.codigo_turma = $%d
	  AND EXISTS (
		  SELECT 1
		  FROM jsonb_array_elements_text(
			  COALESCE(t.historico_estudantes_ano_letivo -> avf.ano_lectivo, '[]'::jsonb)
		  ) AS est(codigo)
		  WHERE est.codigo = avf.codigo_estudante
	  )
)`, len(args)))
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY avf.registered_at DESC"

	rows, err := p.client.DB().Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAvaliacoes(rows)
}

// GetAprovacoes retorna apenas aprovado=TRUE. codigoAcademia vazio = todas (admin).
func (p *AvaliacaoFinalProjection) GetAprovacoes(codigoAcademia string) ([]AvaliacaoFinalDTO, error) {
	t := true
	if codigoAcademia == "" {
		return p.GetAll(nil, &t)
	}
	return p.GetByAcademia(codigoAcademia, nil, &t)
}

// GetReprovacoes retorna apenas aprovado=FALSE. codigoAcademia vazio = todas (admin).
func (p *AvaliacaoFinalProjection) GetReprovacoes(codigoAcademia string) ([]AvaliacaoFinalDTO, error) {
	f := false
	if codigoAcademia == "" {
		return p.GetAll(nil, &f)
	}
	return p.GetByAcademia(codigoAcademia, nil, &f)
}

// GetAprovacoesByEstudante retorna aprovações (aprovado=TRUE) de um estudante.
func (p *AvaliacaoFinalProjection) GetAprovacoesByEstudante(codigoEstudante string) ([]AvaliacaoFinalDTO, error) {
	rows, err := p.client.DB().Query(
		`SELECT `+avaliacaoFinalCols+` FROM projection_avaliacao_final
		WHERE codigo_estudante = $1 AND aprovado = TRUE ORDER BY registered_at DESC`,
		codigoEstudante,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAvaliacoes(rows)
}

// GetReprovacoesByEstudante retorna reprovações (aprovado=FALSE) de um estudante.
func (p *AvaliacaoFinalProjection) GetReprovacoesByEstudante(codigoEstudante string) ([]AvaliacaoFinalDTO, error) {
	rows, err := p.client.DB().Query(
		`SELECT `+avaliacaoFinalCols+` FROM projection_avaliacao_final avf
		WHERE avf.codigo_estudante = $1
		  AND avf.aprovado = FALSE
		  AND `+condicaoReprovacaoDefinitiva()+`
		ORDER BY avf.registered_at DESC`,
		codigoEstudante,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAvaliacoes(rows)
}

func (p *AvaliacaoFinalProjection) GetByEstudanteETipo(codigoEstudante, tipoEnsino string) ([]AvaliacaoFinalDTO, error) {
	rows, err := p.client.DB().Query(
		`SELECT `+avaliacaoFinalCols+` FROM projection_avaliacao_final
		WHERE codigo_estudante = $1 AND tipo_ensino = $2
		ORDER BY registered_at DESC`,
		codigoEstudante, tipoEnsino,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAvaliacoes(rows)
}

// condicaoReprovacaoDefinitiva exclui reprovações intermediárias quando a
// academia tem uma regra ativa posterior que se aplica aos reprovados naquele
// type. Nesses casos, a reprovação só é definitiva após a última avaliação
// dependente também reprovar.
func condicaoReprovacaoDefinitiva() string {
	return `NOT EXISTS (
		SELECT 1
		FROM projection_regras_avaliacao_final raf
		WHERE raf.codigo_academia = avf.codigo_academia
		  AND raf.tipo_ensino = avf.tipo_ensino
		  AND raf.status = 'ativo'
		  AND raf.aplica_se_reprovado_em_type = avf.type
		  AND raf.anos_academicos ? avf.ano_academico_atual
	)`
}

func scanAvaliacoes(rows *sql.Rows) ([]AvaliacaoFinalDTO, error) {
	var result []AvaliacaoFinalDTO
	for rows.Next() {
		var dto AvaliacaoFinalDTO
		if err := rows.Scan(
			&dto.ID, &dto.EventID, &dto.CodigoEstudante, &dto.CodigoAcademia,
			&dto.AnoLectivo, &dto.TipoEnsino, &dto.AnoAcademicoAtual, &dto.ProximoAnoAcademico,
			&dto.Aprovado, &dto.Observacao, &dto.Type, &dto.NotaFinal, &dto.NotaMinimaAprovacao, &dto.RegraAvaliacaoFinalID, &dto.FormulaSnapshot, &dto.AplicaSeReprovadoEmType, &dto.RegisteredAt, &dto.Version,
		); err != nil {
			continue
		}
		result = append(result, dto)
	}
	return result, rows.Err()
}

func (p *AvaliacaoFinalProjection) ExistsByEstudanteAnoLetivoNivelType(codigoEstudante, codigoAcademia, anoLectivo, tipoEnsino, anoAcademicoAtual, avaliacaoType string) (bool, error) {
	var exists bool
	err := p.client.DB().QueryRow(`SELECT EXISTS (SELECT 1 FROM projection_avaliacao_final WHERE codigo_estudante=$1 AND codigo_academia=$2 AND ano_lectivo=$3 AND tipo_ensino=$4 AND ano_academico_atual=$5 AND type=$6)`, codigoEstudante, codigoAcademia, anoLectivo, tipoEnsino, anoAcademicoAtual, avaliacaoType).Scan(&exists)
	return exists, err
}

func (p *AvaliacaoFinalProjection) GetResultadoByType(codigoEstudante, codigoAcademia, anoLectivo, tipoEnsino, anoAcademicoAtual, avaliacaoType string) (*AvaliacaoFinalDTO, error) {
	row := p.client.DB().QueryRow(`SELECT `+avaliacaoFinalCols+` FROM projection_avaliacao_final WHERE codigo_estudante=$1 AND codigo_academia=$2 AND ano_lectivo=$3 AND tipo_ensino=$4 AND ano_academico_atual=$5 AND type=$6 ORDER BY registered_at DESC LIMIT 1`, codigoEstudante, codigoAcademia, anoLectivo, tipoEnsino, anoAcademicoAtual, avaliacaoType)
	var dto AvaliacaoFinalDTO
	if err := row.Scan(&dto.ID, &dto.EventID, &dto.CodigoEstudante, &dto.CodigoAcademia, &dto.AnoLectivo, &dto.TipoEnsino, &dto.AnoAcademicoAtual, &dto.ProximoAnoAcademico, &dto.Aprovado, &dto.Observacao, &dto.Type, &dto.NotaFinal, &dto.NotaMinimaAprovacao, &dto.RegraAvaliacaoFinalID, &dto.FormulaSnapshot, &dto.AplicaSeReprovadoEmType, &dto.RegisteredAt, &dto.Version); err != nil {
		return nil, err
	}
	return &dto, nil
}
