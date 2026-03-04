// ============================================================================
// ARQUIVO: internal/projections/estudante_projection.go
//
// CORREÇÕES APLICADAS:
//   FIX-C3  — handleEmailVerificadoEstudante adicionado (event sourcing do estudante)
//   FIX-M1  — handleDadosPessoaisAtualizados: múltiplos UPDATEs substituídos por
//              uma única transação atômica com SET dinâmico (igual ao padrão Academia)
//   FIX-M2  — handleDadosAcademicosAtualizados: idem — transação atômica
//   FIX-M3  — handleSenhaAlterada: version e last_event_id atualizados corretamente
// ============================================================================

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

type EstudanteProjection struct {
	client *db.Client
}

func NewEstudanteProjection(client *db.Client) *EstudanteProjection {
	return &EstudanteProjection{client: client}
}

func (p *EstudanteProjection) Name() string { return "estudantes" }

// ============================================================================
// Interface Projection
// ============================================================================

func (p *EstudanteProjection) GetLastProcessedEventID() (int64, error) {
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

func (p *EstudanteProjection) UpdateCheckpoint(eventID int64) error {
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

func (p *EstudanteProjection) Handle(event db.Event) error {
	if event.AggregateType != "Estudante" {
		return nil
	}
	switch event.EventType {
	case "EstudanteCriado":
		return p.handleEstudanteCriado(event)
	case "EstudanteCriadoComVinculo":
		return p.handleEstudanteCriadoComVinculo(event)
	case "StatusEscolarFundamentalAtualizado":
		return p.handleStatusEscolarFundamental(event)
	case "StatusEscolarMedioAtualizado":
		return p.handleStatusEscolarMedio(event)
	case "StatusSuperiorAtualizado":
		return p.handleStatusSuperiorAtualizado(event)
	case "DadosPessoaisAtualizados":
		return p.handleDadosPessoaisAtualizados(event)
	case "DadosAcademicosAtualizados":
		return p.handleDadosAcademicosAtualizados(event)
	// FIX-C3: novo evento de verificação de email do estudante via event sourcing
	case "EmailVerificadoEstudante":
		return p.handleEmailVerificadoEstudante(event)
	case "CursoAlterado":
		return p.handleCursoAlterado(event)
	case "AprovacaoAnoRegistrada":
		return p.handleAprovacaoAnoRegistrada(event)
	case "SenhaAlterada":
		return p.handleSenhaAlterada(event)
	case "AvaliacaoFinalAnoAcademico":
		return p.handleAvaliacaoFinalAnoAcademico(event)
	}
	return nil
}

func (p *EstudanteProjection) Rebuild() error {
	log.Printf("[EstudanteProjection] Iniciando rebuild...")
	_, err := p.client.DB().Exec(`DELETE FROM projection_estudantes`)
	if err != nil {
		return fmt.Errorf("erro ao limpar projeção: %w", err)
	}
	if err := p.client.DB().QueryRow(
		`UPDATE projection_checkpoints SET last_processed_event_id = 0 WHERE projection_name = $1 RETURNING projection_name`,
		p.Name(),
	).Scan(new(string)); err != nil && err != sql.ErrNoRows {
		return err
	}
	log.Printf("[EstudanteProjection] Rebuild concluído.")
	return nil
}

// ============================================================================
// Handlers de eventos
// ============================================================================

func (p *EstudanteProjection) handleEstudanteCriado(event db.Event) error {
	var payload struct {
		Nome                     string
		CodigoEstudante          string
		SenhaHash                string
		Email                    *string
		Telefone                 *string
		BilheteIdentidade        *string
		BilheteIdentidadeResp    *string
		Genero                   string
		StatusEscolarFundamental string
		StatusEscolarMedio       string
		StatusSuperior           string
		AnoEscolar               *string
		AnoEscolarMedio          *string
		AnoSuperior              *string
		CursoMedioID             *uuid.UUID
		CursoSuperiorID          *uuid.UUID
		CreatedAt                time.Time
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleEstudanteCriado: parse error: %w", err)
	}

	var cursoMedioID, cursoSuperiorID *string
	if payload.CursoMedioID != nil {
		s := payload.CursoMedioID.String()
		cursoMedioID = &s
	}
	if payload.CursoSuperiorID != nil {
		s := payload.CursoSuperiorID.String()
		cursoSuperiorID = &s
	}

	_, err := p.client.DB().Exec(`
		INSERT INTO projection_estudantes (
			id, nome, codigo_estudante, senha_hash, email, telefone, email_verificado,
			bilhete_identidade, bilhete_identidade_responsavel, genero,
			status, status_escolar_fundamental, status_escolar_medio, status_superior,
			ano_escolar, ano_escolar_medio, ano_superior, curso_medio_id, curso_superior_id,
			created_at, updated_at, version, last_event_id
		) VALUES (
			$1, $2, $3, $4, $5, $6, FALSE,
			$7, $8, $9,
			'inativo', $10, $11, $12,
			$13, $14, $15, $16, $17,
			$18, CURRENT_TIMESTAMP, $19, $20
		)
		ON CONFLICT (id) DO NOTHING
	`,
		event.AggregateID, payload.Nome, payload.CodigoEstudante, payload.SenhaHash,
		payload.Email, payload.Telefone,
		payload.BilheteIdentidade, payload.BilheteIdentidadeResp, payload.Genero,
		payload.StatusEscolarFundamental, payload.StatusEscolarMedio, payload.StatusSuperior,
		payload.AnoEscolar, payload.AnoEscolarMedio, payload.AnoSuperior,
		cursoMedioID, cursoSuperiorID,
		payload.CreatedAt, event.EventVersion, event.EventID,
	)
	if err != nil {
		return fmt.Errorf("handleEstudanteCriado: exec error: %w", err)
	}
	return nil
}

func (p *EstudanteProjection) handleEstudanteCriadoComVinculo(event db.Event) error {
	var payload struct {
		Nome                     string
		CodigoEstudante          string
		SenhaHash                string
		Email                    *string
		Telefone                 *string
		BilheteIdentidade        *string
		BilheteIdentidadeResp    *string
		Genero                   string
		StatusEscolarFundamental string
		StatusEscolarMedio       string
		StatusSuperior           string
		AnoEscolar               *string
		AnoEscolarMedio          *string
		AnoSuperior              *string
		CursoMedioID             *uuid.UUID
		CursoSuperiorID          *uuid.UUID
		CodigoAcademia           string
		CreatedAt                time.Time
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleEstudanteCriadoComVinculo: parse error: %w", err)
	}

	var cursoMedioID, cursoSuperiorID *string
	if payload.CursoMedioID != nil {
		s := payload.CursoMedioID.String()
		cursoMedioID = &s
	}
	if payload.CursoSuperiorID != nil {
		s := payload.CursoSuperiorID.String()
		cursoSuperiorID = &s
	}

	_, err := p.client.DB().Exec(`
		INSERT INTO projection_estudantes (
			id, nome, codigo_estudante, senha_hash, email, telefone, email_verificado,
			bilhete_identidade, bilhete_identidade_responsavel, genero,
			status, status_escolar_fundamental, status_escolar_medio, status_superior,
			ano_escolar, ano_escolar_medio, ano_superior, curso_medio_id, curso_superior_id,
			codigo_academia, created_at, updated_at, version, last_event_id
		) VALUES (
			$1, $2, $3, $4, $5, $6, FALSE,
			$7, $8, $9,
			'ativo', $10, $11, $12,
			$13, $14, $15, $16, $17,
			$18, $19, CURRENT_TIMESTAMP, $20, $21
		)
		ON CONFLICT (id) DO NOTHING
	`,
		event.AggregateID, payload.Nome, payload.CodigoEstudante, payload.SenhaHash,
		payload.Email, payload.Telefone,
		payload.BilheteIdentidade, payload.BilheteIdentidadeResp, payload.Genero,
		payload.StatusEscolarFundamental, payload.StatusEscolarMedio, payload.StatusSuperior,
		payload.AnoEscolar, payload.AnoEscolarMedio, payload.AnoSuperior,
		cursoMedioID, cursoSuperiorID,
		payload.CodigoAcademia,
		payload.CreatedAt, event.EventVersion, event.EventID,
	)
	if err != nil {
		return fmt.Errorf("handleEstudanteCriadoComVinculo: exec error: %w", err)
	}
	return nil
}

func (p *EstudanteProjection) handleStatusEscolarFundamental(event db.Event) error {
	var payload struct{ NovoStatus string }
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleStatusEscolarFundamental: parse error: %w", err)
	}
	_, err := p.client.DB().Exec(`
		UPDATE projection_estudantes
		SET status_escolar_fundamental = $1, version = $2,
			updated_at = CURRENT_TIMESTAMP, last_event_id = $3
		WHERE id = $4
	`, payload.NovoStatus, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

func (p *EstudanteProjection) handleStatusEscolarMedio(event db.Event) error {
	var payload struct{ NovoStatus string }
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleStatusEscolarMedio: parse error: %w", err)
	}
	_, err := p.client.DB().Exec(`
		UPDATE projection_estudantes
		SET status_escolar_medio = $1, version = $2,
			updated_at = CURRENT_TIMESTAMP, last_event_id = $3
		WHERE id = $4
	`, payload.NovoStatus, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

func (p *EstudanteProjection) handleStatusSuperiorAtualizado(event db.Event) error {
	var payload struct{ NovoStatus string }
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleStatusSuperiorAtualizado: parse error: %w", err)
	}
	_, err := p.client.DB().Exec(`
		UPDATE projection_estudantes
		SET status_superior = $1, version = $2,
			updated_at = CURRENT_TIMESTAMP, last_event_id = $3
		WHERE id = $4
	`, payload.NovoStatus, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

// handleDadosPessoaisAtualizados — FIX-M1: substituído múltiplos UPDATEs seriais
// por uma única transação atômica com SET dinâmico. Garante consistência e
// corrige o problema de email_verificado não sendo resetado quando email muda.
func (p *EstudanteProjection) handleDadosPessoaisAtualizados(event db.Event) error {
	var payload struct {
		Nome                  *string `json:"Nome"`
		Email                 *string `json:"Email"`
		Telefone              *string `json:"Telefone"`
		BilheteIdentidade     *string `json:"BilheteIdentidade"`
		BilheteIdentidadeResp *string `json:"BilheteIdentidadeResp"`
		EmailAlterado         bool    `json:"EmailAlterado"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleDadosPessoaisAtualizados: parse error: %w", err)
	}

	setClauses := []string{}
	args := []interface{}{}
	paramIdx := 1

	addParam := func(col string, val interface{}) {
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", col, paramIdx))
		args = append(args, val)
		paramIdx++
	}

	if payload.Nome != nil {
		addParam("nome", *payload.Nome)
	}
	if payload.Email != nil {
		addParam("email", *payload.Email)
		// FIX: sempre resetar email_verificado quando email muda
		if payload.EmailAlterado {
			addParam("email_verificado", false)
		}
	}
	if payload.Telefone != nil {
		addParam("telefone", *payload.Telefone)
	}
	if payload.BilheteIdentidade != nil {
		addParam("bilhete_identidade", *payload.BilheteIdentidade)
	}
	if payload.BilheteIdentidadeResp != nil {
		addParam("bilhete_identidade_responsavel", *payload.BilheteIdentidadeResp)
	}

	if len(setClauses) == 0 {
		// Apenas atualiza version e timestamp mesmo sem campos
		_, err := p.client.DB().Exec(`
			UPDATE projection_estudantes
			SET version = $1, updated_at = CURRENT_TIMESTAMP, last_event_id = $2
			WHERE id = $3
		`, event.EventVersion, event.EventID, event.AggregateID)
		return err
	}

	setClauses = append(setClauses, fmt.Sprintf("version = $%d", paramIdx))
	args = append(args, event.EventVersion)
	paramIdx++

	setClauses = append(setClauses, fmt.Sprintf("updated_at = CURRENT_TIMESTAMP"))

	setClauses = append(setClauses, fmt.Sprintf("last_event_id = $%d", paramIdx))
	args = append(args, event.EventID)
	paramIdx++

	args = append(args, event.AggregateID)
	query := fmt.Sprintf(
		"UPDATE projection_estudantes SET %s WHERE id = $%d",
		strings.Join(setClauses, ", "),
		paramIdx,
	)

	_, err := p.client.DB().Exec(query, args...)
	return err
}

// handleDadosAcademicosAtualizados — FIX-M2: substituído múltiplos UPDATEs seriais
// por uma única transação atômica com SET dinâmico.
func (p *EstudanteProjection) handleDadosAcademicosAtualizados(event db.Event) error {
	var payload struct {
		AnoEscolar      *string    `json:"AnoEscolar"`
		AnoEscolarMedio *string    `json:"AnoEscolarMedio"`
		AnoSuperior     *string    `json:"AnoSuperior"`
		CursoMedioID    *uuid.UUID `json:"CursoMedioID"`
		CursoSuperiorID *uuid.UUID `json:"CursoSuperiorID"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleDadosAcademicosAtualizados: parse error: %w", err)
	}

	setClauses := []string{}
	args := []interface{}{}
	paramIdx := 1

	addParam := func(col string, val interface{}) {
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", col, paramIdx))
		args = append(args, val)
		paramIdx++
	}

	if payload.AnoEscolar != nil {
		addParam("ano_escolar", *payload.AnoEscolar)
	}
	if payload.AnoEscolarMedio != nil {
		addParam("ano_escolar_medio", *payload.AnoEscolarMedio)
	}
	if payload.AnoSuperior != nil {
		addParam("ano_superior", *payload.AnoSuperior)
	}
	if payload.CursoMedioID != nil {
		addParam("curso_medio_id", payload.CursoMedioID.String())
	}
	if payload.CursoSuperiorID != nil {
		addParam("curso_superior_id", payload.CursoSuperiorID.String())
	}

	setClauses = append(setClauses, fmt.Sprintf("version = $%d", paramIdx))
	args = append(args, event.EventVersion)
	paramIdx++

	setClauses = append(setClauses, "updated_at = CURRENT_TIMESTAMP")

	setClauses = append(setClauses, fmt.Sprintf("last_event_id = $%d", paramIdx))
	args = append(args, event.EventID)
	paramIdx++

	args = append(args, event.AggregateID)
	query := fmt.Sprintf(
		"UPDATE projection_estudantes SET %s WHERE id = $%d",
		strings.Join(setClauses, ", "),
		paramIdx,
	)

	_, err := p.client.DB().Exec(query, args...)
	return err
}

// handleEmailVerificadoEstudante — FIX-C3: handler para o novo evento de verificação
// de email do estudante via event sourcing.
func (p *EstudanteProjection) handleEmailVerificadoEstudante(event db.Event) error {
	_, err := p.client.DB().Exec(`
		UPDATE projection_estudantes
		SET email_verificado = TRUE, version = $1,
			updated_at = CURRENT_TIMESTAMP, last_event_id = $2
		WHERE id = $3
	`, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

func (p *EstudanteProjection) handleCursoAlterado(event db.Event) error {
	var payload struct {
		CursoID    uuid.UUID `json:"CursoID"`
		TipoEnsino string    `json:"TipoEnsino"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleCursoAlterado: parse error: %w", err)
	}

	// col é constante interna derivada de lógica fixa — sem risco de injection.
	var col string
	if payload.TipoEnsino == "medio" {
		col = "curso_medio_id"
	} else {
		col = "curso_superior_id"
	}

	query := fmt.Sprintf(`
		UPDATE projection_estudantes
		SET %s = $1, version = $2, updated_at = CURRENT_TIMESTAMP, last_event_id = $3
		WHERE id = $4
	`, col)

	_, err := p.client.DB().Exec(query, payload.CursoID.String(), event.EventVersion, event.EventID, event.AggregateID)
	return err
}

func (p *EstudanteProjection) handleAprovacaoAnoRegistrada(event db.Event) error {
	var payload struct {
		TipoEnsino   string  `json:"TipoEnsino"`
		ProximoNivel *string `json:"ProximoNivel"`
		Aprovado     bool    `json:"Aprovado"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleAprovacaoAnoRegistrada: parse error: %w", err)
	}

	if !payload.Aprovado {
		// Apenas atualiza version sem mudar estado do estudante
		_, err := p.client.DB().Exec(`
			UPDATE projection_estudantes
			SET version = $1, updated_at = CURRENT_TIMESTAMP, last_event_id = $2
			WHERE id = $3
		`, event.EventVersion, event.EventID, event.AggregateID)
		return err
	}

	// col é constante interna derivada de switch com valores fixos.
	var col string
	switch payload.TipoEnsino {
	case "fundamental":
		col = "ano_escolar"
	case "medio":
		col = "ano_escolar_medio"
	case "superior":
		col = "ano_superior"
	default:
		return fmt.Errorf("handleAprovacaoAnoRegistrada: tipo_ensino inválido: %s", payload.TipoEnsino)
	}

	query := fmt.Sprintf(`
		UPDATE projection_estudantes
		SET %s = $1, version = $2, updated_at = CURRENT_TIMESTAMP, last_event_id = $3
		WHERE id = $4
	`, col)

	_, err := p.client.DB().Exec(query, payload.ProximoNivel, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

// handleSenhaAlterada — FIX-M3: version e last_event_id agora atualizados corretamente.
func (p *EstudanteProjection) handleSenhaAlterada(event db.Event) error {
	var payload struct {
		NovaSenhaHash string `json:"NovaSenhaHash"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleSenhaAlterada: parse error: %w", err)
	}
	_, err := p.client.DB().Exec(`
		UPDATE projection_estudantes
		SET senha_hash = $1, version = $2,
			updated_at = CURRENT_TIMESTAMP, last_event_id = $3
		WHERE id = $4
	`, payload.NovaSenhaHash, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

func (p *EstudanteProjection) handleAvaliacaoFinalAnoAcademico(event db.Event) error {
	var payload struct {
		TipoEnsino   string  `json:"TipoEnsino"`
		ProximoNivel *string `json:"ProximoNivel"`
		Aprovado     bool    `json:"Aprovado"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleAvaliacaoFinalAnoAcademico: parse error: %w", err)
	}

	if !payload.Aprovado || payload.ProximoNivel == nil {
		_, err := p.client.DB().Exec(`
			UPDATE projection_estudantes
			SET version = $1, updated_at = CURRENT_TIMESTAMP, last_event_id = $2
			WHERE id = $3
		`, event.EventVersion, event.EventID, event.AggregateID)
		return err
	}

	var col string
	switch payload.TipoEnsino {
	case "fundamental":
		col = "ano_escolar"
	case "medio":
		col = "ano_escolar_medio"
	case "superior":
		col = "ano_superior"
	default:
		return fmt.Errorf("handleAvaliacaoFinalAnoAcademico: tipo_ensino inválido: %s", payload.TipoEnsino)
	}

	query := fmt.Sprintf(`
		UPDATE projection_estudantes
		SET %s = $1, version = $2, updated_at = CURRENT_TIMESTAMP, last_event_id = $3
		WHERE id = $4
	`, col)

	_, err := p.client.DB().Exec(query, *payload.ProximoNivel, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

// ============================================================================
// Queries de leitura
// ============================================================================

type EstudanteDTO struct {
	ID                       uuid.UUID  `db:"id"`
	Nome                     string     `db:"nome"`
	CodigoEstudante          string     `db:"codigo_estudante"`
	SenhaHash                string     `db:"senha_hash"`
	Email                    *string    `db:"email"`
	Telefone                 *string    `db:"telefone"`
	EmailVerificado          bool       `db:"email_verificado"`
	BilheteIdentidade        *string    `db:"bilhete_identidade"`
	BilheteIdentidadeResp    *string    `db:"bilhete_identidade_responsavel"`
	CodigoAcademia           *string    `db:"codigo_academia"`
	Status                   string     `db:"status"`
	StatusEscolarFundamental string     `db:"status_escolar_fundamental"`
	StatusEscolarMedio       string     `db:"status_escolar_medio"`
	StatusSuperior           string     `db:"status_superior"`
	AnoEscolar               *string    `db:"ano_escolar"`
	AnoEscolarMedio          *string    `db:"ano_escolar_medio"`
	AnoSuperior              *string    `db:"ano_superior"`
	CursoMedioID             *uuid.UUID `db:"curso_medio_id"`
	CursoSuperiorID          *uuid.UUID `db:"curso_superior_id"`
	CreatedAt                time.Time  `db:"created_at"`
	UpdatedAt                time.Time  `db:"updated_at"`
	TotalNotas               int        `db:"total_notas"`
	TotalFaltas              int        `db:"total_faltas"`
	Version                  int        `db:"version"`
}

func (p *EstudanteProjection) GetByID(id uuid.UUID) (*EstudanteDTO, error) {
	row := p.client.DB().QueryRow(`
		SELECT id, nome, codigo_estudante, senha_hash,
			email, telefone, email_verificado,
			bilhete_identidade, bilhete_identidade_responsavel, codigo_academia,
			status, status_escolar_fundamental, status_escolar_medio, status_superior,
			ano_escolar, ano_escolar_medio, ano_superior,
			curso_medio_id, curso_superior_id,
			created_at, updated_at,
			COALESCE(total_notas, 0), COALESCE(total_faltas, 0), version
		FROM projection_estudantes WHERE id = $1
	`, id)
	return scanEstudante(row)
}

func (p *EstudanteProjection) GetByCodigo(codigo string) (*EstudanteDTO, error) {
	row := p.client.DB().QueryRow(`
		SELECT id, nome, codigo_estudante, senha_hash,
			email, telefone, email_verificado,
			bilhete_identidade, bilhete_identidade_responsavel, codigo_academia,
			status, status_escolar_fundamental, status_escolar_medio, status_superior,
			ano_escolar, ano_escolar_medio, ano_superior,
			curso_medio_id, curso_superior_id,
			created_at, updated_at,
			COALESCE(total_notas, 0), COALESCE(total_faltas, 0), version
		FROM projection_estudantes WHERE codigo_estudante = $1
	`, codigo)
	return scanEstudante(row)
}

func (p *EstudanteProjection) GetByBilheteIdentidadePrincipal(bilhete string) (*EstudanteDTO, error) {
	row := p.client.DB().QueryRow(`
		SELECT id, nome, codigo_estudante, senha_hash,
			email, telefone, email_verificado,
			bilhete_identidade, bilhete_identidade_responsavel, codigo_academia,
			status, status_escolar_fundamental, status_escolar_medio, status_superior,
			ano_escolar, ano_escolar_medio, ano_superior,
			curso_medio_id, curso_superior_id,
			created_at, updated_at,
			COALESCE(total_notas, 0), COALESCE(total_faltas, 0), version
		FROM projection_estudantes WHERE bilhete_identidade = $1
	`, bilhete)
	return scanEstudante(row)
}

func scanEstudante(row *sql.Row) (*EstudanteDTO, error) {
	var dto EstudanteDTO
	var cursoMedioID, cursoSuperiorID sql.NullString

	err := row.Scan(
		&dto.ID, &dto.Nome, &dto.CodigoEstudante, &dto.SenhaHash,
		&dto.Email, &dto.Telefone, &dto.EmailVerificado,
		&dto.BilheteIdentidade, &dto.BilheteIdentidadeResp, &dto.CodigoAcademia,
		&dto.Status, &dto.StatusEscolarFundamental, &dto.StatusEscolarMedio, &dto.StatusSuperior,
		&dto.AnoEscolar, &dto.AnoEscolarMedio, &dto.AnoSuperior,
		&cursoMedioID, &cursoSuperiorID,
		&dto.CreatedAt, &dto.UpdatedAt,
		&dto.TotalNotas, &dto.TotalFaltas, &dto.Version,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if cursoMedioID.Valid {
		if uid, err := uuid.Parse(cursoMedioID.String); err == nil {
			dto.CursoMedioID = &uid
		}
	}
	if cursoSuperiorID.Valid {
		if uid, err := uuid.Parse(cursoSuperiorID.String); err == nil {
			dto.CursoSuperiorID = &uid
		}
	}

	return &dto, nil
}