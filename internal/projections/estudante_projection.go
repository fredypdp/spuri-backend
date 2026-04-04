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
	case "EstudanteCriadoComVinculo":
		return p.handleEstudanteCriadoComVinculo(event)
	case "StatusEscolarFundamentalAtualizado":
		return p.handleStatusEscolarFundamental(event)
	case "StatusEscolarMedioAtualizado":
		return p.handleStatusEscolarMedio(event)
	case "StatusEscolarAtualizado":
		return p.handleStatusEscolarAtualizado(event)
	case "StatusSuperiorAtualizado":
		return p.handleStatusSuperiorAtualizado(event)
	case "DadosPessoaisAtualizados":
		return p.handleDadosPessoaisAtualizados(event)
	case "DadosAcademicosAtualizados":
		return p.handleDadosAcademicosAtualizados(event)
	case "EmailVerificadoEstudante":
		return p.handleEmailVerificadoEstudante(event)
	case "CursoAlterado":
		return p.handleCursoAlterado(event)
	case "SenhaAlterada":
		return p.handleSenhaAlterada(event)
	case "AvaliacaoFinalAnoAcademico":
		return p.handleAvaliacaoFinalAnoAcademico(event)
	case "NotasRegistradas", "FaltasRegistradas":
		return p.handleVersionOnly(event)
	}
	return nil
}

// ============================================================================
// Rebuild
// ============================================================================

func (p *EstudanteProjection) Rebuild() error {
	log.Printf("[DEBUG] [estudantes] Rebuild iniciado")
	if err := p.clear(); err != nil {
		return fmt.Errorf("falha ao limpar projection_estudantes: %w", err)
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
		return fmt.Errorf("erro ao buscar eventos para rebuild: %w", err)
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
			return fmt.Errorf("erro ao escanear evento %d: %w", count, err)
		}
		if prevHash.Valid {
			event.PreviousHash = &prevHash.String
		}
		if err := p.Handle(event); err != nil {
			return fmt.Errorf("erro ao processar evento %d (type=%s): %w", event.ID, event.EventType, err)
		}
		count++
	}

	log.Printf("[DEBUG] [estudantes] Rebuild concluído: %d eventos processados", count)
	return rows.Err()
}

func (p *EstudanteProjection) clear() error {
	_, err := p.client.DB().Exec(`DELETE FROM projection_estudantes`)
	return err
}

// ============================================================================
// Handlers de eventos
// ============================================================================

func (p *EstudanteProjection) handleEstudanteCriadoComVinculo(event db.Event) error {
	var payload struct {
		Nome                     string     `json:"Nome"`
		CodigoEstudante          string     `json:"CodigoEstudante"`
		SenhaHash                string     `json:"SenhaHash"`
		Email                    *string    `json:"Email"`
		Telefone                 *string    `json:"Telefone"`
		BilheteIdentidade        *string    `json:"BilheteIdentidade"`
		BilheteIdentidadeResp    *string    `json:"BilheteIdentidadeResp"`
		Genero                   string     `json:"Genero"`
		DataNascimento           time.Time  `json:"DataNascimento"`
		StatusEscolarFundamental string     `json:"StatusEscolarFundamental"`
		StatusEscolarMedio       string     `json:"StatusEscolarMedio"`
		StatusSuperior           string     `json:"StatusSuperior"`
		AnoEscolar               *string    `json:"AnoEscolar"`
		AnoEscolarMedio          *string    `json:"AnoEscolarMedio"`
		AnoSuperior              *string    `json:"AnoSuperior"`
		CursoMedioID             *uuid.UUID `json:"CursoMedioID"`
		CursoSuperiorID          *uuid.UUID `json:"CursoSuperiorID"`
		CodigoAcademia           string     `json:"CodigoAcademia"`
		CreatedAt                time.Time  `json:"CreatedAt"`
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

	// IMPORTANTE: projection_estudantes possui FK para projection_cursos.
	// Como as projeções são processadas de forma assíncrona e independente,
	// estudantes pode avançar mais rápido que cursos e tentar inserir um
	// curso_*_id ainda não materializado em projection_cursos.
	// Nesse caso, não bloqueamos toda a projeção: removemos temporariamente
	// o vínculo de curso para permitir que o estudante seja projetado.
	cursoMedioID, cursoSuperiorID, err := p.resolveExistingCursoIDs(cursoMedioID, cursoSuperiorID, event.EventID)
	if err != nil {
		return fmt.Errorf("handleEstudanteCriadoComVinculo: falha ao validar cursos referenciados: %w", err)
	}

	_, err = p.client.DB().Exec(`
		INSERT INTO projection_estudantes (
			id, nome, codigo_estudante, senha_hash, email, telefone, email_verificado,
			bilhete_identidade, bilhete_identidade_responsavel, genero,
			data_nascimento,
			status, status_escolar_fundamental, status_escolar_medio, status_superior,
			ano_escolar, ano_escolar_medio, ano_superior, curso_medio_id, curso_superior_id,
			codigo_academia, created_at, updated_at, version, last_event_id
		) VALUES (
			$1, $2, $3, $4, $5, $6, FALSE,
			$7, $8, $9,
			$10,
			'ativo', $11, $12, $13,
			$14, $15, $16, $17, $18,
			$19, $20, CURRENT_TIMESTAMP, $21, $22
		)
		ON CONFLICT DO NOTHING
	`,
		event.AggregateID, payload.Nome, payload.CodigoEstudante, payload.SenhaHash,
		payload.Email, payload.Telefone,
		payload.BilheteIdentidade, payload.BilheteIdentidadeResp, payload.Genero,
		payload.DataNascimento,
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

func (p *EstudanteProjection) resolveExistingCursoIDs(
	cursoMedioID *string,
	cursoSuperiorID *string,
	eventID uuid.UUID,
) (*string, *string, error) {
	if cursoMedioID != nil {
		exists, err := p.cursoExists(*cursoMedioID)
		if err != nil {
			return nil, nil, err
		}
		if !exists {
			log.Printf("[WARN] [estudantes] evento=%s referenciou curso_medio_id=%s inexistente na projection_cursos no momento do processamento; vínculo será salvo como NULL", eventID, *cursoMedioID)
			cursoMedioID = nil
		}
	}

	if cursoSuperiorID != nil {
		exists, err := p.cursoExists(*cursoSuperiorID)
		if err != nil {
			return nil, nil, err
		}
		if !exists {
			log.Printf("[WARN] [estudantes] evento=%s referenciou curso_superior_id=%s inexistente na projection_cursos no momento do processamento; vínculo será salvo como NULL", eventID, *cursoSuperiorID)
			cursoSuperiorID = nil
		}
	}

	return cursoMedioID, cursoSuperiorID, nil
}

func (p *EstudanteProjection) cursoExists(cursoID string) (bool, error) {
	var exists bool
	err := p.client.DB().QueryRow(`
		SELECT EXISTS (
			SELECT 1
			FROM projection_cursos
			WHERE id = $1
		)
	`, cursoID).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func (p *EstudanteProjection) handleStatusEscolarFundamental(event db.Event) error {
	var payload struct {
		NovoStatus string `json:"NovoStatus"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleStatusEscolarFundamental: parse error: %w", err)
	}
	_, err := p.client.DB().Exec(`
		UPDATE projection_estudantes
		SET status_escolar_fundamental = $1,
			version = $2, updated_at = CURRENT_TIMESTAMP, last_event_id = $3
		WHERE id = $4
	`, payload.NovoStatus, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

func (p *EstudanteProjection) handleStatusEscolarMedio(event db.Event) error {
	var payload struct {
		NovoStatus string `json:"NovoStatus"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleStatusEscolarMedio: parse error: %w", err)
	}
	_, err := p.client.DB().Exec(`
		UPDATE projection_estudantes
		SET status_escolar_medio = $1,
			version = $2, updated_at = CURRENT_TIMESTAMP, last_event_id = $3
		WHERE id = $4
	`, payload.NovoStatus, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

// handleStatusEscolarAtualizado — handler legado para o evento StatusEscolarAtualizado
// (emitido antes da migration 008 que dividiu o campo). Mantido para rebuild
// de ledgers históricos — não remove; apenas ignora se banco já está em novo formato.
func (p *EstudanteProjection) handleStatusEscolarAtualizado(event db.Event) error {
	var payload struct {
		NovoStatus string `json:"NovoStatus"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleStatusEscolarAtualizado: parse error: %w", err)
	}
	_, err := p.client.DB().Exec(`
		UPDATE projection_estudantes
		SET status_escolar_fundamental = $1, status_escolar_medio = $1,
			version = $2, updated_at = CURRENT_TIMESTAMP, last_event_id = $3
		WHERE id = $4
	`, payload.NovoStatus, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

func (p *EstudanteProjection) handleStatusSuperiorAtualizado(event db.Event) error {
	var payload struct {
		NovoStatus string `json:"NovoStatus"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleStatusSuperiorAtualizado: parse error: %w", err)
	}
	_, err := p.client.DB().Exec(`
		UPDATE projection_estudantes
		SET status_superior = $1,
			version = $2, updated_at = CURRENT_TIMESTAMP, last_event_id = $3
		WHERE id = $4
	`, payload.NovoStatus, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

func (p *EstudanteProjection) handleDadosPessoaisAtualizados(event db.Event) error {
	var payload struct {
		Nome                  *string    `json:"Nome"`
		Email                 *string    `json:"Email"`
		Telefone              *string    `json:"Telefone"`
		BilheteIdentidade     *string    `json:"BilheteIdentidade"`
		BilheteIdentidadeResp *string    `json:"BilheteIdentidadeResp"`
		DataNascimento        *time.Time `json:"DataNascimento"`
		EmailAlterado         bool       `json:"EmailAlterado"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleDadosPessoaisAtualizados: parse error: %w", err)
	}

	setClauses := []string{}
	args := []interface{}{}
	idx := 1

	if payload.Nome != nil {
		setClauses = append(setClauses, fmt.Sprintf("nome = $%d", idx))
		args = append(args, *payload.Nome)
		idx++
	}
	if payload.Email != nil {
		setClauses = append(setClauses, fmt.Sprintf("email = $%d", idx))
		args = append(args, *payload.Email)
		idx++
	}
	if payload.EmailAlterado {
		setClauses = append(setClauses, "email_verificado = FALSE")
	}
	if payload.Telefone != nil {
		setClauses = append(setClauses, fmt.Sprintf("telefone = $%d", idx))
		args = append(args, *payload.Telefone)
		idx++
	}
	if payload.BilheteIdentidade != nil {
		setClauses = append(setClauses, fmt.Sprintf("bilhete_identidade = $%d", idx))
		args = append(args, *payload.BilheteIdentidade)
		idx++
	}
	if payload.BilheteIdentidadeResp != nil {
		setClauses = append(setClauses, fmt.Sprintf("bilhete_identidade_responsavel = $%d", idx))
		args = append(args, *payload.BilheteIdentidadeResp)
		idx++
	}
	if payload.DataNascimento != nil {
		setClauses = append(setClauses, fmt.Sprintf("data_nascimento = $%d", idx))
		args = append(args, *payload.DataNascimento)
		idx++
	}

	if len(setClauses) == 0 {
		_, err := p.client.DB().Exec(`
			UPDATE projection_estudantes
			SET version = $1, updated_at = CURRENT_TIMESTAMP, last_event_id = $2
			WHERE id = $3
		`, event.EventVersion, event.EventID, event.AggregateID)
		return err
	}

	setClauses = append(setClauses,
		fmt.Sprintf("version = $%d", idx),
		fmt.Sprintf("last_event_id = $%d", idx+1),
		"updated_at = CURRENT_TIMESTAMP",
	)
	args = append(args, event.EventVersion, event.EventID, event.AggregateID)
	query := fmt.Sprintf("UPDATE projection_estudantes SET %s WHERE id = $%d",
		strings.Join(setClauses, ", "), idx+2)
	_, err := p.client.DB().Exec(query, args...)
	return err
}

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
	idx := 1

	if payload.AnoEscolar != nil {
		setClauses = append(setClauses, fmt.Sprintf("ano_escolar = $%d", idx))
		args = append(args, *payload.AnoEscolar)
		idx++
	}
	if payload.AnoEscolarMedio != nil {
		setClauses = append(setClauses, fmt.Sprintf("ano_escolar_medio = $%d", idx))
		args = append(args, *payload.AnoEscolarMedio)
		idx++
	}
	if payload.AnoSuperior != nil {
		setClauses = append(setClauses, fmt.Sprintf("ano_superior = $%d", idx))
		args = append(args, *payload.AnoSuperior)
		idx++
	}
	if payload.CursoMedioID != nil {
		setClauses = append(setClauses, fmt.Sprintf("curso_medio_id = $%d", idx))
		args = append(args, payload.CursoMedioID.String())
		idx++
	}
	if payload.CursoSuperiorID != nil {
		setClauses = append(setClauses, fmt.Sprintf("curso_superior_id = $%d", idx))
		args = append(args, payload.CursoSuperiorID.String())
		idx++
	}

	if len(setClauses) == 0 {
		_, err := p.client.DB().Exec(`
			UPDATE projection_estudantes
			SET version = $1, updated_at = CURRENT_TIMESTAMP, last_event_id = $2
			WHERE id = $3
		`, event.EventVersion, event.EventID, event.AggregateID)
		return err
	}

	setClauses = append(setClauses,
		fmt.Sprintf("version = $%d", idx),
		fmt.Sprintf("last_event_id = $%d", idx+1),
		"updated_at = CURRENT_TIMESTAMP",
	)
	args = append(args, event.EventVersion, event.EventID, event.AggregateID)
	query := fmt.Sprintf("UPDATE projection_estudantes SET %s WHERE id = $%d",
		strings.Join(setClauses, ", "), idx+2)
	_, err := p.client.DB().Exec(query, args...)
	return err
}

func (p *EstudanteProjection) handleEmailVerificadoEstudante(event db.Event) error {
	_, err := p.client.DB().Exec(`
		UPDATE projection_estudantes
		SET email_verificado = TRUE,
			version = $1, updated_at = CURRENT_TIMESTAMP, last_event_id = $2
		WHERE id = $3
	`, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

func (p *EstudanteProjection) handleCursoAlterado(event db.Event) error {
	var payload struct {
		TipoEnsino string    `json:"TipoEnsino"`
		CursoID    uuid.UUID `json:"CursoID"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleCursoAlterado: parse error: %w", err)
	}
	var col string
	switch payload.TipoEnsino {
	case "medio":
		col = "curso_medio_id"
	case "superior":
		col = "curso_superior_id"
	default:
		return fmt.Errorf("handleCursoAlterado: TipoEnsino inválido: %q", payload.TipoEnsino)
	}
	query := fmt.Sprintf(`
		UPDATE projection_estudantes
		SET %s = $1, version = $2, updated_at = CURRENT_TIMESTAMP, last_event_id = $3
		WHERE id = $4
	`, col)
	_, err := p.client.DB().Exec(query, payload.CursoID.String(), event.EventVersion, event.EventID, event.AggregateID)
	return err
}

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

// handleAvaliacaoFinalAnoAcademico atualiza o ano escolar do estudante quando
// aprovado ou marca o status como "finalizado" quando é o último ano do ciclo.
func (p *EstudanteProjection) handleAvaliacaoFinalAnoAcademico(event db.Event) error {
	var payload struct {
		TipoEnsino          string  `json:"TipoEnsino"`
		ProximoAnoAcademico *string `json:"ProximoAnoAcademico"`
		Aprovado            bool    `json:"Aprovado"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleAvaliacaoFinalAnoAcademico: parse error: %w", err)
	}

	// Reprovado ou sem próximo nível sem ser o último — apenas avança versão.
	if !payload.Aprovado {
		_, err := p.client.DB().Exec(`
			UPDATE projection_estudantes
			SET version = $1, updated_at = CURRENT_TIMESTAMP, last_event_id = $2
			WHERE id = $3
		`, event.EventVersion, event.EventID, event.AggregateID)
		return err
	}

	// Aprovado e é o último ano do ciclo — marcar status como finalizado.
	if payload.ProximoAnoAcademico == nil {
		var statusCol string
		switch payload.TipoEnsino {
		case "fundamental":
			statusCol = "status_escolar_fundamental"
		case "medio":
			statusCol = "status_escolar_medio"
		case "superior":
			statusCol = "status_superior"
		default:
			return fmt.Errorf("handleAvaliacaoFinalAnoAcademico: TipoEnsino inválido: %q", payload.TipoEnsino)
		}
		query := fmt.Sprintf(`
			UPDATE projection_estudantes
			SET %s = 'finalizado', version = $1, updated_at = CURRENT_TIMESTAMP, last_event_id = $2
			WHERE id = $3
		`, statusCol)
		_, err := p.client.DB().Exec(query, event.EventVersion, event.EventID, event.AggregateID)
		return err
	}

	// Aprovado — avançar para o próximo nível.
	var col string
	switch payload.TipoEnsino {
	case "fundamental":
		col = "ano_escolar"
	case "medio":
		col = "ano_escolar_medio"
	case "superior":
		col = "ano_superior"
	default:
		return fmt.Errorf("handleAvaliacaoFinalAnoAcademico: TipoEnsino inválido: %q", payload.TipoEnsino)
	}
	query := fmt.Sprintf(`
		UPDATE projection_estudantes
		SET %s = $1, version = $2, updated_at = CURRENT_TIMESTAMP, last_event_id = $3
		WHERE id = $4
	`, col)
	_, err := p.client.DB().Exec(query, payload.ProximoAnoAcademico, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

func (p *EstudanteProjection) handleVersionOnly(event db.Event) error {
	_, err := p.client.DB().Exec(`
		UPDATE projection_estudantes
		SET version = $1, updated_at = CURRENT_TIMESTAMP, last_event_id = $2
		WHERE id = $3
	`, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

// ============================================================================
// DTO de leitura
// ============================================================================

// EstudanteDTO — genero e data_nascimento são sempre preenchidos.
type EstudanteDTO struct {
	ID                       uuid.UUID `json:"id"`
	Nome                     string    `json:"nome"`
	CodigoEstudante          string    `json:"codigo_estudante"`
	Email                    *string   `json:"email,omitempty"`
	Telefone                 *string   `json:"telefone,omitempty"`
	EmailVerificado          bool      `json:"email_verificado"`
	BilheteIdentidade        *string   `json:"bilhete_identidade,omitempty"`
	BilheteIdentidadeResp    *string   `json:"bilhete_identidade_responsavel,omitempty"`
	Genero                   string    `json:"genero"`
	DataNascimento           time.Time `json:"data_nascimento"`
	CodigoAcademia           *string   `json:"codigo_academia,omitempty"`
	Status                   string    `json:"status"`
	StatusEscolarFundamental string    `json:"status_escolar_fundamental"`
	StatusEscolarMedio       string    `json:"status_escolar_medio"`
	StatusSuperior           string    `json:"status_superior"`
	AnoEscolar               *string   `json:"ano_escolar,omitempty"`
	AnoEscolarMedio          *string   `json:"ano_escolar_medio,omitempty"`
	AnoSuperior              *string   `json:"ano_superior,omitempty"`
	CursoMedioID             *string   `json:"curso_medio_id,omitempty"`
	CursoSuperiorID          *string   `json:"curso_superior_id,omitempty"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
	Version                  int       `json:"version"`
}

// estudanteCols lista as colunas na mesma ordem dos Scan abaixo.
const estudanteCols = `
	id, nome, codigo_estudante, email, telefone, email_verificado,
	bilhete_identidade, bilhete_identidade_responsavel, genero,
	data_nascimento,
	codigo_academia, status, status_escolar_fundamental, status_escolar_medio, status_superior,
	ano_escolar, ano_escolar_medio, ano_superior, curso_medio_id, curso_superior_id,
	created_at, updated_at, version
`

func scanEstudante(row *sql.Row) (*EstudanteDTO, error) {
	var e EstudanteDTO
	err := row.Scan(
		&e.ID, &e.Nome, &e.CodigoEstudante, &e.Email, &e.Telefone, &e.EmailVerificado,
		&e.BilheteIdentidade, &e.BilheteIdentidadeResp, &e.Genero,
		&e.DataNascimento,
		&e.CodigoAcademia, &e.Status, &e.StatusEscolarFundamental, &e.StatusEscolarMedio, &e.StatusSuperior,
		&e.AnoEscolar, &e.AnoEscolarMedio, &e.AnoSuperior, &e.CursoMedioID, &e.CursoSuperiorID,
		&e.CreatedAt, &e.UpdatedAt, &e.Version,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func scanEstudanteRows(rows *sql.Rows) ([]EstudanteDTO, error) {
	var estudantes []EstudanteDTO
	for rows.Next() {
		var e EstudanteDTO
		if err := rows.Scan(
			&e.ID, &e.Nome, &e.CodigoEstudante, &e.Email, &e.Telefone, &e.EmailVerificado,
			&e.BilheteIdentidade, &e.BilheteIdentidadeResp, &e.Genero,
			&e.DataNascimento,
			&e.CodigoAcademia, &e.Status, &e.StatusEscolarFundamental, &e.StatusEscolarMedio, &e.StatusSuperior,
			&e.AnoEscolar, &e.AnoEscolarMedio, &e.AnoSuperior, &e.CursoMedioID, &e.CursoSuperiorID,
			&e.CreatedAt, &e.UpdatedAt, &e.Version,
		); err != nil {
			return nil, err
		}
		estudantes = append(estudantes, e)
	}
	return estudantes, rows.Err()
}

func (p *EstudanteProjection) GetByID(id uuid.UUID) (*EstudanteDTO, error) {
	return scanEstudante(p.client.DB().QueryRow(
		`SELECT `+estudanteCols+` FROM projection_estudantes WHERE id = $1`, id,
	))
}

func (p *EstudanteProjection) GetByCodigo(codigo string) (*EstudanteDTO, error) {
	return scanEstudante(p.client.DB().QueryRow(
		`SELECT `+estudanteCols+` FROM projection_estudantes WHERE codigo_estudante = $1`, codigo,
	))
}

func (p *EstudanteProjection) GetByEmail(email string) (*EstudanteDTO, error) {
	return scanEstudante(p.client.DB().QueryRow(
		`SELECT `+estudanteCols+` FROM projection_estudantes WHERE email = $1`, email,
	))
}

func (p *EstudanteProjection) GetAll() ([]EstudanteDTO, error) {
	rows, err := p.client.DB().Query(
		`SELECT ` + estudanteCols + ` FROM projection_estudantes ORDER BY nome ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEstudanteRows(rows)
}

func (p *EstudanteProjection) GetByAcademia(codigoAcademia string) ([]EstudanteDTO, error) {
	rows, err := p.client.DB().Query(
		`SELECT `+estudanteCols+` FROM projection_estudantes WHERE codigo_academia = $1 ORDER BY nome ASC`,
		codigoAcademia,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEstudanteRows(rows)
}

// ============================================================================
// Auth DTO — exclusivo para autenticação, nunca exposto em respostas HTTP
// ============================================================================

type EstudanteAuthDTO struct {
	ID              uuid.UUID `json:"-"`
	Nome            string    `json:"-"`
	Codigo          string    `json:"-"`
	Status          string    `json:"-"`
	Hash            string    `json:"-"`
	Email           *string   `json:"-"`
	EmailVerificado bool      `json:"-"`
}

func (p *EstudanteProjection) GetAuthByCodigo(codigo string) (*EstudanteAuthDTO, error) {
	var e EstudanteAuthDTO
	err := p.client.DB().QueryRow(
		`SELECT id, nome, codigo_estudante, status, senha_hash
		 FROM projection_estudantes WHERE codigo_estudante = $1`,
		codigo,
	).Scan(&e.ID, &e.Nome, &e.Codigo, &e.Status, &e.Hash)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (p *EstudanteProjection) GetAuthByID(id uuid.UUID) (*EstudanteAuthDTO, error) {
	var e EstudanteAuthDTO
	err := p.client.DB().QueryRow(
		`SELECT id, nome, codigo_estudante, status, senha_hash
		 FROM projection_estudantes WHERE id = $1`,
		id,
	).Scan(&e.ID, &e.Nome, &e.Codigo, &e.Status, &e.Hash)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (p *EstudanteProjection) GetAuthByIdentificador(identificador string) (*EstudanteAuthDTO, error) {
	var e EstudanteAuthDTO
	var emailNull sql.NullString
	err := p.client.DB().QueryRow(`
		SELECT id, nome, codigo_estudante, status, senha_hash,
		       email, COALESCE(email_verificado, FALSE)
		FROM projection_estudantes
		WHERE codigo_estudante = $1
		   OR email            = $1
		LIMIT 1
	`, identificador).Scan(&e.ID, &e.Nome, &e.Codigo, &e.Status, &e.Hash,
		&emailNull, &e.EmailVerificado)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if emailNull.Valid {
		e.Email = &emailNull.String
	}
	return &e, nil
}

func (p *EstudanteProjection) GetByBilheteIdentidadePrincipal(bilhete string) (*EstudanteDTO, error) {
	return scanEstudante(p.client.DB().QueryRow(
		`SELECT `+estudanteCols+` FROM projection_estudantes
		 WHERE bilhete_identidade = $1 LIMIT 1`,
		bilhete,
	))
}

func (p *EstudanteProjection) CountByCurso(cursoID uuid.UUID) (int, error) {
	var count int
	err := p.client.DB().QueryRow(
		`SELECT COUNT(*) FROM projection_estudantes
		 WHERE (curso_medio_id = $1 OR curso_superior_id = $1) AND status = 'ativo'`,
		cursoID.String(),
	).Scan(&count)
	return count, err
}
