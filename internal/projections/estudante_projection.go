// ============================================================================
// ARQUIVO: internal/projections/estudante_projection.go
//
// CORREÇÕES APLICADAS (Etapa 3):
//   P3-01/02 — Handle() consolidado em switch canônico cobrindo TODOS os eventos
//              emitidos pelo aggregate Estudante:
//              EstudanteCriado, EstudanteCriadoComVinculo, EstudanteInscrito,
//              InscricaoAprovada, InscricaoReprovada, EstudanteVinculado,
//              StatusEscolarFundamentalAtualizado, StatusEscolarMedioAtualizado,
//              StatusEscolarAtualizado, StatusSuperiorAtualizado,
//              DadosPessoaisAtualizados, DadosAcademicosAtualizados,
//              EmailVerificadoEstudante, CursoAlterado, AprovacaoAnoRegistrada,
//              SenhaAlterada, AvaliacaoFinalAnoAcademico,
//              NotasRegistradas, FaltasRegistradas.
//   P3-03 — Rebuild() reprocessa eventos do ledger (não era só DELETE + reset).
//   P3-04 — Rebuild() usa sql.NullString para previous_hash.
//   P3-05 — handleCursoAlterado: adicionado default com erro explícito.
//   P3-06 — handleAprovacaoAnoRegistrada: padrão já tem default; mantido.
//   P3-21 — Nota sobre atomicidade: UpdateCheckpoint é responsabilidade do Manager.
//   P3-23 — EstudanteInscrito e InscricaoReprovada agora têm handlers.
//
// Versão anterior tinha dois arquivos conflitantes; este arquivo é a versão
// canônica e única para estudante_projection.go.
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

// ============================================================================
// Handle — roteador canônico de eventos
//
// Cobre todos os eventos que o aggregate Estudante pode emitir.
// Eventos não reconhecidos são ignorados silenciosamente (comportamento seguro).
// ============================================================================

func (p *EstudanteProjection) Handle(event db.Event) error {
	if event.AggregateType != "Estudante" {
		return nil
	}

	switch event.EventType {
	case "EstudanteCriado":
		return p.handleEstudanteCriado(event)
	case "EstudanteCriadoComVinculo":
		return p.handleEstudanteCriadoComVinculo(event)
	case "EstudanteInscrito":
		// P3-23: handler adicionado — antes inexistente.
		return p.handleEstudanteInscrito(event)
	case "InscricaoAprovada":
		return p.handleInscricaoAprovada(event)
	case "InscricaoReprovada":
		// P3-23: handler adicionado — antes inexistente.
		return p.handleInscricaoReprovada(event)
	case "EstudanteVinculado":
		return p.handleEstudanteVinculado(event)
	// StatusEscolarFundamentalAtualizado e StatusEscolarMedioAtualizado:
	// emitidos pelo aggregate em estudante_aprovacao.go.
	case "StatusEscolarFundamentalAtualizado":
		return p.handleStatusEscolarFundamental(event)
	case "StatusEscolarMedioAtualizado":
		return p.handleStatusEscolarMedio(event)
	// StatusEscolarAtualizado e StatusSuperiorAtualizado: legado/compatibilidade.
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
	case "AprovacaoAnoRegistrada":
		return p.handleAprovacaoAnoRegistrada(event)
	case "SenhaAlterada":
		return p.handleSenhaAlterada(event)
	case "AvaliacaoFinalAnoAcademico":
		return p.handleAvaliacaoFinalAnoAcademico(event)
	// NotasRegistradas e FaltasRegistradas: o aggregate Estudante os emite;
	// as projeções dedicadas (notas_projection, faltas_projection) os processam
	// com os dados completos. Aqui apenas atualizamos a versão na projeção
	// de estudantes para manter consistência de version/last_event_id.
	case "NotasRegistradas", "FaltasRegistradas":
		return p.handleVersionOnly(event)
	}

	return nil
}

// ============================================================================
// Rebuild — reprocessa todos os eventos do ledger do aggregate Estudante.
//
// P3-03: substituiu a versão anterior que apenas fazia DELETE + reset do
//        checkpoint, sem reler o ledger. Agora o rebuild é completo e fiel.
// P3-04: usa sql.NullString para previous_hash (pode ser NULL no banco).
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
		// P3-04: sql.NullString para previous_hash — o primeiro evento de cada
		// aggregate tem previous_hash = NULL no banco.
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

func (p *EstudanteProjection) handleEstudanteCriado(event db.Event) error {
	var payload struct {
		Nome                     string     `json:"Nome"`
		CodigoEstudante          string     `json:"CodigoEstudante"`
		SenhaHash                string     `json:"SenhaHash"`
		Email                    *string    `json:"Email"`
		Telefone                 *string    `json:"Telefone"`
		BilheteIdentidade        *string    `json:"BilheteIdentidade"`
		BilheteIdentidadeResp    *string    `json:"BilheteIdentidadeResp"`
		Genero                   string     `json:"Genero"`
		StatusEscolarFundamental string     `json:"StatusEscolarFundamental"`
		StatusEscolarMedio       string     `json:"StatusEscolarMedio"`
		StatusSuperior           string     `json:"StatusSuperior"`
		AnoEscolar               *string    `json:"AnoEscolar"`
		AnoEscolarMedio          *string    `json:"AnoEscolarMedio"`
		AnoSuperior              *string    `json:"AnoSuperior"`
		CursoMedioID             *uuid.UUID `json:"CursoMedioID"`
		CursoSuperiorID          *uuid.UUID `json:"CursoSuperiorID"`
		CodigoAcademia           *string    `json:"CodigoAcademia"`
		CreatedAt                time.Time  `json:"CreatedAt"`
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
			codigo_academia, created_at, updated_at, version, last_event_id
		) VALUES (
			$1, $2, $3, $4, $5, $6, FALSE,
			$7, $8, $9,
			'inativo', $10, $11, $12,
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
		return fmt.Errorf("handleEstudanteCriado: exec error: %w", err)
	}
	return nil
}

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

// handleEstudanteInscrito — P3-23: handler adicionado.
// Insere a inscrição em projection_inscricoes quando o estudante solicita inscrição.
func (p *EstudanteProjection) handleEstudanteInscrito(event db.Event) error {
	var payload struct {
		InscricaoID    uuid.UUID  `json:"InscricaoID"`
		CodigoAcademia string     `json:"CodigoAcademia"`
		Tipo           string     `json:"Tipo"`
		AnoInscricao   string     `json:"AnoInscricao"`
		CursoID        *uuid.UUID `json:"CursoID"`
		CreatedAt      time.Time  `json:"CreatedAt"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleEstudanteInscrito: parse error: %w", err)
	}

	// Busca academia_id a partir do codigo_academia
	var academiaID uuid.UUID
	if err := p.client.DB().QueryRow(
		`SELECT id FROM projection_academias WHERE codigo_academia = $1`,
		payload.CodigoAcademia,
	).Scan(&academiaID); err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("handleEstudanteInscrito: academia não encontrada (%s): %w", payload.CodigoAcademia, err)
	}

	var cursoIDStr *string
	if payload.CursoID != nil {
		s := payload.CursoID.String()
		cursoIDStr = &s
	}

	_, err := p.client.DB().Exec(`
		INSERT INTO projection_inscricoes (
			id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso_id, status, status_usado,
			created_at, updated_at, event_id, version
		) VALUES (
			$1, $2,
			(SELECT codigo_estudante FROM projection_estudantes WHERE id = $2),
			$3, $4,
			$5, $6, $7, 'espera', FALSE,
			$8, CURRENT_TIMESTAMP, $9, $10
		)
		ON CONFLICT (id) DO NOTHING
	`,
		payload.InscricaoID, event.AggregateID,
		academiaID, payload.CodigoAcademia,
		payload.Tipo, payload.AnoInscricao, cursoIDStr,
		payload.CreatedAt, event.EventID, event.EventVersion,
	)
	if err != nil {
		return fmt.Errorf("handleEstudanteInscrito: exec error: %w", err)
	}
	return nil
}

// handleInscricaoAprovada atualiza o status da inscrição para 'aprovado'.
func (p *EstudanteProjection) handleInscricaoAprovada(event db.Event) error {
	var payload struct {
		InscricaoID    uuid.UUID  `json:"InscricaoID"`
		CodigoAcademia string     `json:"CodigoAcademia"`
		Tipo           string     `json:"Tipo"`
		AnoInscricao   string     `json:"AnoInscricao"`
		CursoID        *uuid.UUID `json:"CursoID"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleInscricaoAprovada: parse error: %w", err)
	}

	_, err := p.client.DB().Exec(`
		UPDATE projection_inscricoes
		SET status = 'aprovado', updated_at = CURRENT_TIMESTAMP,
			event_id = $1, version = $2
		WHERE estudante_id = $3 AND codigo_academia = $4 AND status = 'espera'
	`, event.EventID, event.EventVersion, event.AggregateID, payload.CodigoAcademia)
	if err != nil {
		return fmt.Errorf("handleInscricaoAprovada: exec error: %w", err)
	}

	// Incrementa total_inscricoes no estudante
	_, err = p.client.DB().Exec(`
		UPDATE projection_estudantes
		SET total_inscricoes = total_inscricoes + 1,
			version = $1, updated_at = CURRENT_TIMESTAMP, last_event_id = $2
		WHERE id = $3
	`, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

// handleInscricaoReprovada — P3-23: handler adicionado.
// Atualiza o status da inscrição para 'reprovado'.
func (p *EstudanteProjection) handleInscricaoReprovada(event db.Event) error {
	var payload struct {
		InscricaoID    uuid.UUID `json:"InscricaoID"`
		CodigoAcademia string    `json:"CodigoAcademia"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleInscricaoReprovada: parse error: %w", err)
	}

	_, err := p.client.DB().Exec(`
		UPDATE projection_inscricoes
		SET status = 'reprovado', updated_at = CURRENT_TIMESTAMP,
			event_id = $1, version = $2
		WHERE estudante_id = $3 AND codigo_academia = $4 AND status = 'espera'
	`, event.EventID, event.EventVersion, event.AggregateID, payload.CodigoAcademia)
	if err != nil {
		return fmt.Errorf("handleInscricaoReprovada: exec error: %w", err)
	}

	// Atualiza version na projeção de estudantes
	_, err = p.client.DB().Exec(`
		UPDATE projection_estudantes
		SET version = $1, updated_at = CURRENT_TIMESTAMP, last_event_id = $2
		WHERE id = $3
	`, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

func (p *EstudanteProjection) handleEstudanteVinculado(event db.Event) error {
	var payload struct {
		CodigoAcademia string    `json:"CodigoAcademia"`
		VinculadoAt    time.Time `json:"VinculadoAt"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleEstudanteVinculado: parse error: %w", err)
	}

	_, err := p.client.DB().Exec(`
		UPDATE projection_estudantes
		SET codigo_academia = $1, status = 'ativo',
			version = $2, updated_at = CURRENT_TIMESTAMP, last_event_id = $3
		WHERE id = $4
	`, payload.CodigoAcademia, event.EventVersion, event.EventID, event.AggregateID)
	return err
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

	if len(setClauses) == 0 {
		// Nenhum campo a atualizar — apenas versão
		_, err := p.client.DB().Exec(`
			UPDATE projection_estudantes
			SET version = $1, updated_at = CURRENT_TIMESTAMP, last_event_id = $2
			WHERE id = $3
		`, event.EventVersion, event.EventID, event.AggregateID)
		return err
	}

	setClauses = append(setClauses,
		fmt.Sprintf("version = $%d", idx), fmt.Sprintf("last_event_id = $%d", idx+1), "updated_at = CURRENT_TIMESTAMP")
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
		fmt.Sprintf("version = $%d", idx), fmt.Sprintf("last_event_id = $%d", idx+1), "updated_at = CURRENT_TIMESTAMP")
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

// handleCursoAlterado — P3-05: default com erro explícito adicionado.
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
		// P3-05: antes era if/else sem default — agora retorna erro explícito.
		return fmt.Errorf("handleCursoAlterado: TipoEnsino inválido: %q", payload.TipoEnsino)
	}

	// col é constante derivada do switch fechado acima — não é input externo.
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
		TipoEnsino    string  `json:"TipoEnsino"`
		NivelSeguinte *string `json:"NivelSeguinte"`
		AvancarAno    bool    `json:"AvancarAno"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleAprovacaoAnoRegistrada: parse error: %w", err)
	}

	if !payload.AvancarAno || payload.NivelSeguinte == nil {
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
		return fmt.Errorf("handleAprovacaoAnoRegistrada: TipoEnsino inválido: %q", payload.TipoEnsino)
	}

	// col é derivada do switch fechado — não é input externo.
	query := fmt.Sprintf(`
		UPDATE projection_estudantes
		SET %s = $1, version = $2, updated_at = CURRENT_TIMESTAMP, last_event_id = $3
		WHERE id = $4
	`, col)
	_, err := p.client.DB().Exec(query, payload.NivelSeguinte, event.EventVersion, event.EventID, event.AggregateID)
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

func (p *EstudanteProjection) handleAvaliacaoFinalAnoAcademico(event db.Event) error {
	var payload struct {
		TipoEnsino    string  `json:"TipoEnsino"`
		ProximoNivel  *string `json:"ProximoNivel"`
		Aprovado      bool    `json:"Aprovado"`
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
		return fmt.Errorf("handleAvaliacaoFinalAnoAcademico: TipoEnsino inválido: %q", payload.TipoEnsino)
	}

	query := fmt.Sprintf(`
		UPDATE projection_estudantes
		SET %s = $1, version = $2, updated_at = CURRENT_TIMESTAMP, last_event_id = $3
		WHERE id = $4
	`, col)
	_, err := p.client.DB().Exec(query, payload.ProximoNivel, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

// handleVersionOnly apenas avança version/last_event_id para eventos que
// têm projeções dedicadas (NotasProjection, FaltasProjection) mas cujo
// aggregate é Estudante — manter consistência de versão na projeção principal.
func (p *EstudanteProjection) handleVersionOnly(event db.Event) error {
	_, err := p.client.DB().Exec(`
		UPDATE projection_estudantes
		SET version = $1, updated_at = CURRENT_TIMESTAMP, last_event_id = $2
		WHERE id = $3
	`, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

// ============================================================================
// Queries de leitura
// ============================================================================

type EstudanteDTO struct {
	ID                       uuid.UUID  `json:"id"`
	Nome                     string     `json:"nome"`
	CodigoEstudante          string     `json:"codigo_estudante"`
	Email                    *string    `json:"email,omitempty"`
	Telefone                 *string    `json:"telefone,omitempty"`
	EmailVerificado          bool       `json:"email_verificado"`
	BilheteIdentidade        *string    `json:"bilhete_identidade,omitempty"`
	BilheteIdentidadeResp    *string    `json:"bilhete_identidade_responsavel,omitempty"`
	Genero                   string     `json:"genero"`
	CodigoAcademia           *string    `json:"codigo_academia,omitempty"`
	Status                   string     `json:"status"`
	StatusEscolarFundamental string     `json:"status_escolar_fundamental"`
	StatusEscolarMedio       string     `json:"status_escolar_medio"`
	StatusSuperior           string     `json:"status_superior"`
	AnoEscolar               *string    `json:"ano_escolar,omitempty"`
	AnoEscolarMedio          *string    `json:"ano_escolar_medio,omitempty"`
	AnoSuperior              *string    `json:"ano_superior,omitempty"`
	CursoMedioID             *string    `json:"curso_medio_id,omitempty"`
	CursoSuperiorID          *string    `json:"curso_superior_id,omitempty"`
	TotalInscricoes          int        `json:"total_inscricoes"`
	CreatedAt                time.Time  `json:"created_at"`
	UpdatedAt                time.Time  `json:"updated_at"`
	Version                  int        `json:"version"`
}

const estudanteCols = `
	id, nome, codigo_estudante, email, telefone, email_verificado,
	bilhete_identidade, bilhete_identidade_responsavel, genero,
	codigo_academia, status, status_escolar_fundamental, status_escolar_medio, status_superior,
	ano_escolar, ano_escolar_medio, ano_superior, curso_medio_id, curso_superior_id,
	total_inscricoes, created_at, updated_at, version
`

func scanEstudante(row *sql.Row) (*EstudanteDTO, error) {
	var e EstudanteDTO
	err := row.Scan(
		&e.ID, &e.Nome, &e.CodigoEstudante, &e.Email, &e.Telefone, &e.EmailVerificado,
		&e.BilheteIdentidade, &e.BilheteIdentidadeResp, &e.Genero,
		&e.CodigoAcademia, &e.Status, &e.StatusEscolarFundamental, &e.StatusEscolarMedio, &e.StatusSuperior,
		&e.AnoEscolar, &e.AnoEscolarMedio, &e.AnoSuperior, &e.CursoMedioID, &e.CursoSuperiorID,
		&e.TotalInscricoes, &e.CreatedAt, &e.UpdatedAt, &e.Version,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
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

	var estudantes []EstudanteDTO
	for rows.Next() {
		var e EstudanteDTO
		if err := rows.Scan(
			&e.ID, &e.Nome, &e.CodigoEstudante, &e.Email, &e.Telefone, &e.EmailVerificado,
			&e.BilheteIdentidade, &e.BilheteIdentidadeResp, &e.Genero,
			&e.CodigoAcademia, &e.Status, &e.StatusEscolarFundamental, &e.StatusEscolarMedio, &e.StatusSuperior,
			&e.AnoEscolar, &e.AnoEscolarMedio, &e.AnoSuperior, &e.CursoMedioID, &e.CursoSuperiorID,
			&e.TotalInscricoes, &e.CreatedAt, &e.UpdatedAt, &e.Version,
		); err != nil {
			return nil, err
		}
		estudantes = append(estudantes, e)
	}
	return estudantes, rows.Err()
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

	var estudantes []EstudanteDTO
	for rows.Next() {
		var e EstudanteDTO
		if err := rows.Scan(
			&e.ID, &e.Nome, &e.CodigoEstudante, &e.Email, &e.Telefone, &e.EmailVerificado,
			&e.BilheteIdentidade, &e.BilheteIdentidadeResp, &e.Genero,
			&e.CodigoAcademia, &e.Status, &e.StatusEscolarFundamental, &e.StatusEscolarMedio, &e.StatusSuperior,
			&e.AnoEscolar, &e.AnoEscolarMedio, &e.AnoSuperior, &e.CursoMedioID, &e.CursoSuperiorID,
			&e.TotalInscricoes, &e.CreatedAt, &e.UpdatedAt, &e.Version,
		); err != nil {
			return nil, err
		}
		estudantes = append(estudantes, e)
	}
	return estudantes, rows.Err()
}
