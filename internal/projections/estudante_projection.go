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
		INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at, events_processed)
		VALUES ($1, $2, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) DO UPDATE SET
			last_processed_event_id = $2,
			last_processed_at = CURRENT_TIMESTAMP,
			events_processed = projection_checkpoints.events_processed + 1
	`, p.Name(), eventID)
	return err
}

func (p *EstudanteProjection) Handle(event db.Event) error {
	if event.AggregateType != "Estudante" {
		return nil
	}
	handlers := map[string]func(db.Event) error{
		"EstudanteCriado": p.handleEstudanteCriado,
		// FIX BUG #2: "EstudanteCriadoComVinculo" estava ausente no map.
		// Estudantes criados via academia (POST /academia/estudante/register)
		// emitem este evento — sem este entry, nunca apareciam na projection_estudantes.
		"EstudanteCriadoComVinculo":          p.handleEstudanteCriadoComVinculo,
		"StatusEscolarFundamentalAtualizado": p.handleStatusEscolarFundamental,
		"StatusEscolarMedioAtualizado":       p.handleStatusEscolarMedio,
		"StatusSuperiorAtualizado":           p.handleStatusSuperiorAtualizado,
		"DadosPessoaisAtualizados":           p.handleDadosPessoaisAtualizados,
		"DadosAcademicosAtualizados":         p.handleDadosAcademicosAtualizados,
		"EmailVerificado":                    p.handleEmailVerificado,
		"CursoAlterado":                      p.handleCursoAlterado,
		"AprovacaoAnoRegistrada":             p.handleAprovacaoAnoRegistrada,
		"SenhaAlterada":                      p.handleSenhaAlterada,
		// FIX RISCO #1: "AvaliacaoFinalAnoAcademico" estava ausente no map.
		// O aggregate Estudante possui applyAvaliacaoFinalAnoAcademico() no switch Apply(),
		// portanto a projeção também deve acompanhar as mudanças de estado
		// (avanço de ano acadêmico após aprovação na avaliação final).
		"AvaliacaoFinalAnoAcademico": p.handleAvaliacaoFinalAnoAcademico,
	}
	if handler, ok := handlers[event.EventType]; ok {
		return handler(event)
	}
	return nil
}

func (p *EstudanteProjection) Rebuild() error {
	log.Printf("[DEBUG] [estudantes] Rebuild iniciado")
	if err := p.clear(); err != nil {
		return fmt.Errorf("falha ao limpar: %w", err)
	}
	rows, err := p.client.DB().Query(`
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE aggregate_type = 'Estudante'
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
	log.Printf("[DEBUG] [estudantes] Rebuild concluído: %d eventos processados", count)
	return rows.Err()
}

func (p *EstudanteProjection) clear() error {
	_, err := p.client.DB().Exec(`TRUNCATE TABLE projection_estudantes CASCADE`)
	return err
}

// ============================================================================
// Event handlers
// ============================================================================

func (p *EstudanteProjection) handleEstudanteCriado(event db.Event) error {
	var payload struct {
		Nome, CodigoEstudante, SenhaHash string
		StatusEscolarFundamental         string
		StatusEscolarMedio               string
		StatusSuperior                   string
		Genero                           string
		Email, Telefone, BilheteIdentidade, BilheteIdentidadeResp *string
		AnoEscolar, AnoEscolarMedio, AnoSuperior                  *string
		CursoMedioID, CursoSuperiorID                             *uuid.UUID
		CreatedAt                                                 time.Time
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error EstudanteCriado: %w", err)
	}
	if event.AggregateID == uuid.Nil {
		return fmt.Errorf("UUID inválido no evento EstudanteCriado")
	}

	var cursoMedioID, cursoSuperiorID interface{}
	if payload.CursoMedioID != nil {
		cursoMedioID = payload.CursoMedioID.String()
	}
	if payload.CursoSuperiorID != nil {
		cursoSuperiorID = payload.CursoSuperiorID.String()
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
	return err
}

// handleEstudanteCriadoComVinculo processa o evento emitido quando uma academia
// cria um estudante diretamente (POST /academia/estudante/register).
//
// FIX BUG #2: handler estava ausente — estudantes criados via academia nunca
// apareciam na projection_estudantes. A diferença em relação ao handleEstudanteCriado
// é que o estudante nasce com status 'ativo' e com codigo_academia já preenchido,
// pois já está vinculado à academia no momento da criação.
func (p *EstudanteProjection) handleEstudanteCriadoComVinculo(event db.Event) error {
	var payload struct {
		Nome, CodigoEstudante, SenhaHash string
		StatusEscolarFundamental         string
		StatusEscolarMedio               string
		StatusSuperior                   string
		Genero                           string
		CodigoAcademia                   string
		Email, Telefone, BilheteIdentidade, BilheteIdentidadeResp *string
		AnoEscolar, AnoEscolarMedio, AnoSuperior                  *string
		CursoMedioID, CursoSuperiorID                             *uuid.UUID
		CreatedAt                                                 time.Time
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error EstudanteCriadoComVinculo: %w", err)
	}
	if event.AggregateID == uuid.Nil {
		return fmt.Errorf("UUID inválido no evento EstudanteCriadoComVinculo")
	}

	var cursoMedioID, cursoSuperiorID interface{}
	if payload.CursoMedioID != nil {
		cursoMedioID = payload.CursoMedioID.String()
	}
	if payload.CursoSuperiorID != nil {
		cursoSuperiorID = payload.CursoSuperiorID.String()
	}

	_, err := p.client.DB().Exec(`
		INSERT INTO projection_estudantes (
			id, nome, codigo_estudante, senha_hash, email, telefone, email_verificado,
			bilhete_identidade, bilhete_identidade_responsavel, genero,
			status, status_escolar_fundamental, status_escolar_medio, status_superior,
			ano_escolar, ano_escolar_medio, ano_superior, curso_medio_id, curso_superior_id,
			codigo_academia,
			created_at, updated_at, version, last_event_id
		) VALUES (
			$1, $2, $3, $4, $5, $6, FALSE,
			$7, $8, $9,
			'ativo', $10, $11, $12,
			$13, $14, $15, $16, $17,
			$18,
			$19, CURRENT_TIMESTAMP, $20, $21
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
	return err
}

func (p *EstudanteProjection) handleStatusEscolarFundamental(event db.Event) error {
	var payload struct{ NovoStatus string }
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error: %w", err)
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
		return fmt.Errorf("parse error: %w", err)
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
		return fmt.Errorf("parse error: %w", err)
	}
	_, err := p.client.DB().Exec(`
		UPDATE projection_estudantes
		SET status_superior = $1, version = $2,
			updated_at = CURRENT_TIMESTAMP, last_event_id = $3
		WHERE id = $4
	`, payload.NovoStatus, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

func (p *EstudanteProjection) handleDadosPessoaisAtualizados(event db.Event) error {
	var payload struct {
		Nome, Email, Telefone, BilheteIdentidade, BilheteIdentidadeResp *string
		EmailAlterado                                                    bool
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error: %w", err)
	}

	if payload.Nome != nil {
		p.client.DB().Exec(`UPDATE projection_estudantes SET nome = $1 WHERE id = $2`, *payload.Nome, event.AggregateID)
	}
	if payload.Email != nil {
		p.client.DB().Exec(`UPDATE projection_estudantes SET email = $1 WHERE id = $2`, *payload.Email, event.AggregateID)
		if payload.EmailAlterado {
			p.client.DB().Exec(`UPDATE projection_estudantes SET email_verificado = FALSE WHERE id = $1`, event.AggregateID)
		}
	}
	if payload.Telefone != nil {
		p.client.DB().Exec(`UPDATE projection_estudantes SET telefone = $1 WHERE id = $2`, *payload.Telefone, event.AggregateID)
	}
	if payload.BilheteIdentidade != nil {
		p.client.DB().Exec(`UPDATE projection_estudantes SET bilhete_identidade = $1 WHERE id = $2`, *payload.BilheteIdentidade, event.AggregateID)
	}
	if payload.BilheteIdentidadeResp != nil {
		p.client.DB().Exec(`UPDATE projection_estudantes SET bilhete_identidade_responsavel = $1 WHERE id = $2`, *payload.BilheteIdentidadeResp, event.AggregateID)
	}

	_, err := p.client.DB().Exec(`
		UPDATE projection_estudantes
		SET version = $1, updated_at = CURRENT_TIMESTAMP, last_event_id = $2
		WHERE id = $3
	`, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

func (p *EstudanteProjection) handleDadosAcademicosAtualizados(event db.Event) error {
	var payload struct {
		AnoEscolar      *string
		AnoEscolarMedio *string
		AnoSuperior     *string
		CursoMedioID    *uuid.UUID
		CursoSuperiorID *uuid.UUID
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error: %w", err)
	}

	if payload.AnoEscolar != nil {
		p.client.DB().Exec(`UPDATE projection_estudantes SET ano_escolar = $1 WHERE id = $2`, *payload.AnoEscolar, event.AggregateID)
	}
	if payload.AnoEscolarMedio != nil {
		p.client.DB().Exec(`UPDATE projection_estudantes SET ano_escolar_medio = $1 WHERE id = $2`, *payload.AnoEscolarMedio, event.AggregateID)
	}
	if payload.AnoSuperior != nil {
		p.client.DB().Exec(`UPDATE projection_estudantes SET ano_superior = $1 WHERE id = $2`, *payload.AnoSuperior, event.AggregateID)
	}
	if payload.CursoMedioID != nil {
		p.client.DB().Exec(`UPDATE projection_estudantes SET curso_medio_id = $1 WHERE id = $2`, payload.CursoMedioID.String(), event.AggregateID)
	}
	if payload.CursoSuperiorID != nil {
		p.client.DB().Exec(`UPDATE projection_estudantes SET curso_superior_id = $1 WHERE id = $2`, payload.CursoSuperiorID.String(), event.AggregateID)
	}

	_, err := p.client.DB().Exec(`
		UPDATE projection_estudantes
		SET version = $1, updated_at = CURRENT_TIMESTAMP, last_event_id = $2
		WHERE id = $3
	`, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

func (p *EstudanteProjection) handleEmailVerificado(event db.Event) error {
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
		return fmt.Errorf("parse error: %w", err)
	}

	var col string
	if payload.TipoEnsino == "medio" {
		col = "curso_medio_id"
	} else {
		col = "curso_superior_id"
	}

	// col is an internal constant
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
		return fmt.Errorf("parse error: %w", err)
	}

	// Reprovação: projeção de reprovações cuida disto; nada a fazer aqui
	if !payload.Aprovado {
		return nil
	}

	// Aprovado: avança para o próximo nível
	if payload.ProximoNivel == nil {
		return nil
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
		return nil
	}

	// col is an internal constant
	query := fmt.Sprintf(`
		UPDATE projection_estudantes
		SET %s = $1, version = $2, updated_at = CURRENT_TIMESTAMP, last_event_id = $3
		WHERE id = $4
	`, col)
	_, err := p.client.DB().Exec(query, *payload.ProximoNivel, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

func (p *EstudanteProjection) handleSenhaAlterada(event db.Event) error {
	var payload struct{ NovaSenhaHash string }
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error SenhaAlterada: %w", err)
	}
	_, err := p.client.DB().Exec(`
		UPDATE projection_estudantes
		SET senha_hash = $1, version = $2, updated_at = CURRENT_TIMESTAMP, last_event_id = $3
		WHERE id = $4
	`, payload.NovaSenhaHash, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

// handleAvaliacaoFinalAnoAcademico atualiza o ano acadêmico do estudante na
// projeção após a avaliação final ser registrada com aprovação.
//
// FIX RISCO #1: evento estava ausente no map de Handle(). O aggregate Estudante
// tem case "AvaliacaoFinalAnoAcademico" no switch Apply() que muta o estado
// (avança ProximoAnoAcademico), portanto a projeção deve acompanhar.
func (p *EstudanteProjection) handleAvaliacaoFinalAnoAcademico(event db.Event) error {
	var payload struct {
		TipoEnsino          string  `json:"TipoEnsino"`
		ProximoAnoAcademico *string `json:"ProximoAnoAcademico"`
		Aprovado            bool    `json:"Aprovado"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error AvaliacaoFinalAnoAcademico: %w", err)
	}

	// Reprovado ou sem próximo ano: nada a atualizar na projeção do estudante
	if !payload.Aprovado || payload.ProximoAnoAcademico == nil {
		return nil
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
		return nil
	}

	// col is an internal constant
	query := fmt.Sprintf(`
		UPDATE projection_estudantes
		SET %s = $1, version = $2, updated_at = CURRENT_TIMESTAMP, last_event_id = $3
		WHERE id = $4
	`, col)
	_, err := p.client.DB().Exec(query, *payload.ProximoAnoAcademico, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

// ============================================================================
// Query methods
// ============================================================================

const estudanteSelectCols = `
	id, nome, codigo_estudante, senha_hash, email, telefone, email_verificado,
	bilhete_identidade, bilhete_identidade_responsavel, codigo_academia,
	status, status_escolar_fundamental, status_escolar_medio, status_superior,
	ano_escolar, ano_escolar_medio, ano_superior, curso_medio_id, curso_superior_id,
	created_at, updated_at, total_notas, total_faltas, total_inscricoes, version
`

func (p *EstudanteProjection) GetByID(id uuid.UUID) (*EstudanteDTO, error) {
	row := p.client.DB().QueryRow(
		`SELECT `+estudanteSelectCols+` FROM projection_estudantes WHERE id = $1 LIMIT 1`,
		id,
	)
	return scanEstudante(row)
}

func (p *EstudanteProjection) GetByCodigo(codigo string) (*EstudanteDTO, error) {
	row := p.client.DB().QueryRow(
		`SELECT `+estudanteSelectCols+` FROM projection_estudantes WHERE codigo_estudante = $1 LIMIT 1`,
		codigo,
	)
	return scanEstudante(row)
}

func (p *EstudanteProjection) GetByEmail(email string) (*EstudanteDTO, error) {
	row := p.client.DB().QueryRow(
		`SELECT `+estudanteSelectCols+` FROM projection_estudantes WHERE email = $1 LIMIT 1`,
		email,
	)
	return scanEstudante(row)
}

func (p *EstudanteProjection) GetByBilhete(bilhete string) (*EstudanteDTO, error) {
	row := p.client.DB().QueryRow(
		`SELECT `+estudanteSelectCols+`
		FROM projection_estudantes
		WHERE bilhete_identidade = $1 OR bilhete_identidade_responsavel = $1
		LIMIT 1`,
		bilhete,
	)
	return scanEstudante(row)
}

func (p *EstudanteProjection) GetByBilheteIdentidadePrincipal(bilhete string) (*EstudanteDTO, error) {
	row := p.client.DB().QueryRow(
		`SELECT `+estudanteSelectCols+` FROM projection_estudantes WHERE bilhete_identidade = $1 LIMIT 1`,
		bilhete,
	)
	return scanEstudante(row)
}

func (p *EstudanteProjection) CountByCurso(cursoID uuid.UUID) (int, error) {
	var count int
	err := p.client.DB().QueryRow(`
		SELECT COUNT(*)
		FROM projection_estudantes
		WHERE curso_medio_id = $1 OR curso_superior_id = $1
	`, cursoID).Scan(&count)
	return count, err
}

func scanEstudante(row *sql.Row) (*EstudanteDTO, error) {
	var dto EstudanteDTO
	var cursoMedioID, cursoSuperiorID sql.NullString
	err := row.Scan(
		&dto.ID, &dto.Nome, &dto.CodigoEstudante, &dto.SenhaHash,
		&dto.Email, &dto.Telefone, &dto.EmailVerificado,
		&dto.BilheteIdentidade, &dto.BilheteIdentidadeResp, &dto.CodigoAcademia,
		&dto.Status, &dto.StatusEscolarFundamental, &dto.StatusEscolarMedio, &dto.StatusSuperior,
		&dto.AnoEscolar, &dto.AnoEscolarMedio, &dto.AnoSuperior, &cursoMedioID, &cursoSuperiorID,
		&dto.CreatedAt, &dto.UpdatedAt, &dto.TotalNotas, &dto.TotalFaltas,
		&dto.TotalInscricoes, &dto.Version,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if cursoMedioID.Valid {
		cid, _ := uuid.Parse(cursoMedioID.String)
		dto.CursoMedioID = &cid
	}
	if cursoSuperiorID.Valid {
		cid, _ := uuid.Parse(cursoSuperiorID.String)
		dto.CursoSuperiorID = &cid
	}
	return &dto, nil
}

// ============================================================================
// DTO
// ============================================================================

type EstudanteDTO struct {
	ID                       uuid.UUID  `json:"id"`
	Nome                     string     `json:"nome"`
	CodigoEstudante          string     `json:"codigo_estudante"`
	SenhaHash                string     `json:"-"`
	Email                    *string    `json:"email,omitempty"`
	Telefone                 *string    `json:"telefone,omitempty"`
	EmailVerificado          bool       `json:"email_verificado"`
	BilheteIdentidade        *string    `json:"bilhete_identidade,omitempty"`
	BilheteIdentidadeResp    *string    `json:"bilhete_identidade_responsavel,omitempty"`
	CodigoAcademia           *string    `json:"codigo_academia,omitempty"`
	Status                   string     `json:"status"`
	StatusEscolarFundamental string     `json:"status_escolar_fundamental"`
	StatusEscolarMedio       string     `json:"status_escolar_medio"`
	StatusSuperior           string     `json:"status_superior"`
	AnoEscolar               *string    `json:"ano_escolar,omitempty"`
	AnoEscolarMedio          *string    `json:"ano_escolar_medio,omitempty"`
	AnoSuperior              *string    `json:"ano_superior,omitempty"`
	CursoMedioID             *uuid.UUID `json:"curso_medio_id,omitempty"`
	CursoSuperiorID          *uuid.UUID `json:"curso_superior_id,omitempty"`
	CreatedAt                time.Time  `json:"created_at"`
	UpdatedAt                time.Time  `json:"updated_at"`
	TotalNotas               int        `json:"total_notas"`
	TotalFaltas              int        `json:"total_faltas"`
	TotalInscricoes          int        `json:"total_inscricoes"`
	Version                  int        `json:"version"`
}