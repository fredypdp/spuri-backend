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

func (p *EstudanteProjection) Handle(event db.Event) error {
	if event.AggregateType != "Estudante" {
		return nil
	}

	handlers := map[string]func(db.Event) error{
		"EstudanteCriado":                    p.handleEstudanteCriado,
		"EstudanteCriadoComVinculo":          p.handleEstudanteCriadoComVinculo,
		"InscricaoAprovada":                  p.handleInscricaoAprovada,
		"EstudanteVinculado":                 p.handleEstudanteVinculado,
		"StatusSuperiorAtualizado":           p.handleStatusSuperiorAtualizado,
		"DadosPessoaisAtualizados":           p.handleDadosPessoaisAtualizados,
		"DadosAcademicosAtualizados":         p.handleDadosAcademicosAtualizados,
		"EmailVerificado":                    p.handleEmailVerificado,
		"CursoAlterado":                      p.handleCursoAlterado,
		"AprovacaoAnoRegistrada":             p.handleAprovacaoAnoRegistrada,
		"StatusEscolarFundamentalAtualizado": p.handleStatusEscolarFundamentalAtualizado,
		"StatusEscolarMedioAtualizado":       p.handleStatusEscolarMedioAtualizado,
		"AvaliacaoFinalAnoAcademico":         p.handleAvaliacaoFinalAnoAcademico, // FIX #1
	}

	if handler, ok := handlers[event.EventType]; ok {
		log.Printf("[DEBUG] Processando %s para estudante %s", event.EventType, event.AggregateID)
		return handler(event)
	}
	return nil
}

func (p *EstudanteProjection) Rebuild() error {
	log.Printf("[DEBUG] Rebuild iniciado")

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

func (p *EstudanteProjection) GetLastProcessedEventID() (int64, error) {
	var lastID int64
	query := fmt.Sprintf(`SELECT last_processed_event_id FROM projection_checkpoints WHERE projection_name = '%s'`,
		db.SafeString(p.Name()))
	err := p.client.DB().QueryRow(query).Scan(&lastID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return lastID, err
}

func (p *EstudanteProjection) UpdateCheckpoint(eventID int64) error {
	eventID = int64(db.ValidateOffset(int(eventID)))
	query := fmt.Sprintf(`
		INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at, events_processed)
		VALUES ('%s', %d, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) DO UPDATE SET
			last_processed_event_id = %d,
			last_processed_at = CURRENT_TIMESTAMP,
			events_processed = projection_checkpoints.events_processed + 1
	`, db.SafeString(p.Name()), eventID, eventID)
	_, err := p.client.DB().Exec(query)
	return err
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
		return fmt.Errorf("parse error: %w", err)
	}

	if event.AggregateID == uuid.Nil || payload.SenhaHash == "" || payload.CodigoEstudante == "" {
		return fmt.Errorf("dados obrigatórios inválidos")
	}

	query := fmt.Sprintf(`
		INSERT INTO projection_estudantes (
			id, nome, genero, codigo_estudante, senha_hash, email, telefone, email_verificado,
			bilhete_identidade, bilhete_identidade_responsavel, codigo_academia,
			status, status_escolar_fundamental, status_escolar_medio, status_superior,
			ano_escolar, ano_escolar_medio, ano_superior, curso_medio_id, curso_superior_id,
			version, created_at, updated_at, last_event_id
		) VALUES (
			'%s', '%s', '%s', '%s', '%s', %s, %s, FALSE, %s, %s, NULL,
			'inativo', '%s', '%s', '%s', %s, %s, %s, %s,
			%d, '%s', CURRENT_TIMESTAMP, '%s'
		)
		ON CONFLICT (id) DO UPDATE SET
			nome = EXCLUDED.nome, genero = EXCLUDED.genero,
			codigo_estudante = EXCLUDED.codigo_estudante,
			senha_hash = EXCLUDED.senha_hash, email = EXCLUDED.email, telefone = EXCLUDED.telefone,
			bilhete_identidade = EXCLUDED.bilhete_identidade,
			bilhete_identidade_responsavel = EXCLUDED.bilhete_identidade_responsavel,
			status = EXCLUDED.status,
			status_escolar_fundamental = EXCLUDED.status_escolar_fundamental,
			status_escolar_medio = EXCLUDED.status_escolar_medio,
			status_superior = EXCLUDED.status_superior,
			ano_escolar = EXCLUDED.ano_escolar, ano_escolar_medio = EXCLUDED.ano_escolar_medio,
			ano_superior = EXCLUDED.ano_superior,
			curso_medio_id = EXCLUDED.curso_medio_id, curso_superior_id = EXCLUDED.curso_superior_id,
			version = EXCLUDED.version, updated_at = EXCLUDED.updated_at,
			last_event_id = EXCLUDED.last_event_id
	`, event.AggregateID,
		db.SafeString(payload.Nome), db.SafeString(payload.Genero),
		db.SafeString(payload.CodigoEstudante), db.SafeString(payload.SenhaHash),
		nullOrString(payload.Email), nullOrString(payload.Telefone),
		nullOrString(payload.BilheteIdentidade), nullOrString(payload.BilheteIdentidadeResp),
		db.SafeString(payload.StatusEscolarFundamental),
		db.SafeString(payload.StatusEscolarMedio),
		db.SafeString(payload.StatusSuperior),
		nullOrString(payload.AnoEscolar), nullOrString(payload.AnoEscolarMedio), nullOrString(payload.AnoSuperior),
		nullOrUUID(payload.CursoMedioID), nullOrUUID(payload.CursoSuperiorID),
		event.EventVersion, payload.CreatedAt.Format(time.RFC3339), event.EventID)

	_, err := p.client.DB().Exec(query)
	return err
}

func (p *EstudanteProjection) handleEstudanteCriadoComVinculo(event db.Event) error {
	var payload struct {
		Nome, CodigoEstudante, SenhaHash string
		StatusEscolarFundamental         string
		StatusEscolarMedio               string
		StatusSuperior                   string
		CodigoAcademia                   string
		Genero                           string
		Email, Telefone, BilheteIdentidade, BilheteIdentidadeResp *string
		AnoEscolar, AnoEscolarMedio, AnoSuperior                  *string
		CursoMedioID, CursoSuperiorID                             *uuid.UUID
		CreatedAt                                                 time.Time
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error: %w", err)
	}

	log.Printf("[DEBUG] EstudanteCriadoComVinculo - AnoEscolar: %v, CursoMedioID: %v", payload.AnoEscolar, payload.CursoMedioID)

	if event.AggregateID == uuid.Nil || payload.SenhaHash == "" || payload.CodigoEstudante == "" {
		return fmt.Errorf("dados obrigatórios inválidos")
	}

	query := fmt.Sprintf(`
		INSERT INTO projection_estudantes (
			id, nome, genero, codigo_estudante, senha_hash, email, telefone, email_verificado,
			bilhete_identidade, bilhete_identidade_responsavel, codigo_academia,
			status, status_escolar_fundamental, status_escolar_medio, status_superior,
			ano_escolar, ano_escolar_medio, ano_superior, curso_medio_id, curso_superior_id,
			version, created_at, updated_at, last_event_id
		) VALUES (
			'%s', '%s', '%s', '%s', '%s', %s, %s, FALSE, %s, %s, '%s',
			'ativo', '%s', '%s', '%s', %s, %s, %s, %s,
			%d, '%s', CURRENT_TIMESTAMP, '%s'
		)
		ON CONFLICT (id) DO UPDATE SET
			nome = EXCLUDED.nome, genero = EXCLUDED.genero,
			codigo_estudante = EXCLUDED.codigo_estudante,
			senha_hash = EXCLUDED.senha_hash, email = EXCLUDED.email, telefone = EXCLUDED.telefone,
			bilhete_identidade = EXCLUDED.bilhete_identidade,
			bilhete_identidade_responsavel = EXCLUDED.bilhete_identidade_responsavel,
			codigo_academia = EXCLUDED.codigo_academia, status = EXCLUDED.status,
			status_escolar_fundamental = EXCLUDED.status_escolar_fundamental,
			status_escolar_medio = EXCLUDED.status_escolar_medio,
			status_superior = EXCLUDED.status_superior,
			ano_escolar = EXCLUDED.ano_escolar, ano_escolar_medio = EXCLUDED.ano_escolar_medio,
			ano_superior = EXCLUDED.ano_superior,
			curso_medio_id = EXCLUDED.curso_medio_id, curso_superior_id = EXCLUDED.curso_superior_id,
			version = EXCLUDED.version, updated_at = EXCLUDED.updated_at,
			last_event_id = EXCLUDED.last_event_id
	`, event.AggregateID,
		db.SafeString(payload.Nome), db.SafeString(payload.Genero),
		db.SafeString(payload.CodigoEstudante), db.SafeString(payload.SenhaHash),
		nullOrString(payload.Email), nullOrString(payload.Telefone),
		nullOrString(payload.BilheteIdentidade), nullOrString(payload.BilheteIdentidadeResp),
		db.SafeString(payload.CodigoAcademia),
		db.SafeString(payload.StatusEscolarFundamental),
		db.SafeString(payload.StatusEscolarMedio),
		db.SafeString(payload.StatusSuperior),
		nullOrString(payload.AnoEscolar), nullOrString(payload.AnoEscolarMedio), nullOrString(payload.AnoSuperior),
		nullOrUUID(payload.CursoMedioID), nullOrUUID(payload.CursoSuperiorID),
		event.EventVersion, payload.CreatedAt.Format(time.RFC3339), event.EventID)

	if _, err := p.client.DB().Exec(query); err != nil {
		return err
	}

	updateAcademiaQuery := fmt.Sprintf(`
		UPDATE projection_academias
		SET total_estudantes = total_estudantes + 1, updated_at = CURRENT_TIMESTAMP
		WHERE codigo_academia = '%s'
	`, db.SafeString(payload.CodigoAcademia))
	p.client.DB().Exec(updateAcademiaQuery)

	return nil
}

func (p *EstudanteProjection) handleInscricaoAprovada(event db.Event) error {
	var payload struct {
		InscricaoID uuid.UUID
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error: %w", err)
	}

	query := fmt.Sprintf(`
		UPDATE projection_estudantes
		SET total_inscricoes = total_inscricoes + 1,
			version = %d, updated_at = CURRENT_TIMESTAMP, last_event_id = '%s'
		WHERE id = '%s'
	`, event.EventVersion, event.EventID, event.AggregateID)
	_, err := p.client.DB().Exec(query)
	return err
}

func (p *EstudanteProjection) handleEstudanteVinculado(event db.Event) error {
	var payload struct {
		CodigoAcademia string
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error: %w", err)
	}

	query := fmt.Sprintf(`
		UPDATE projection_estudantes
		SET codigo_academia = '%s', status = 'ativo',
			version = %d, updated_at = CURRENT_TIMESTAMP, last_event_id = '%s'
		WHERE id = '%s'
	`, db.SafeString(payload.CodigoAcademia), event.EventVersion, event.EventID, event.AggregateID)
	_, err := p.client.DB().Exec(query)
	return err
}

func (p *EstudanteProjection) handleStatusSuperiorAtualizado(event db.Event) error {
	var payload struct{ NovoStatus string }
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error: %w", err)
	}

	query := fmt.Sprintf(`
		UPDATE projection_estudantes
		SET status_superior = '%s', version = %d,
			updated_at = CURRENT_TIMESTAMP, last_event_id = '%s'
		WHERE id = '%s'
	`, db.SafeString(payload.NovoStatus), event.EventVersion, event.EventID, event.AggregateID)
	_, err := p.client.DB().Exec(query)
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

	setClauses := fmt.Sprintf("version = %d, updated_at = CURRENT_TIMESTAMP, last_event_id = '%s'",
		event.EventVersion, event.EventID)

	if payload.Nome != nil {
		setClauses += fmt.Sprintf(", nome = '%s'", db.SafeString(*payload.Nome))
	}
	if payload.Email != nil {
		setClauses += fmt.Sprintf(", email = '%s'", db.SafeString(*payload.Email))
		if payload.EmailAlterado {
			setClauses += ", email_verificado = FALSE"
		}
	}
	if payload.Telefone != nil {
		setClauses += fmt.Sprintf(", telefone = '%s'", db.SafeString(*payload.Telefone))
	}
	if payload.BilheteIdentidade != nil {
		setClauses += fmt.Sprintf(", bilhete_identidade = '%s'", db.SafeString(*payload.BilheteIdentidade))
	}
	if payload.BilheteIdentidadeResp != nil {
		setClauses += fmt.Sprintf(", bilhete_identidade_responsavel = '%s'", db.SafeString(*payload.BilheteIdentidadeResp))
	}

	query := fmt.Sprintf(`UPDATE projection_estudantes SET %s WHERE id = '%s'`, setClauses, event.AggregateID)
	_, err := p.client.DB().Exec(query)
	return err
}

func (p *EstudanteProjection) handleDadosAcademicosAtualizados(event db.Event) error {
	var payload struct {
		AnoEscolar, AnoEscolarMedio, AnoSuperior *string
		CursoMedioID                             *uuid.UUID
		CursoSuperiorID                          *uuid.UUID
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error: %w", err)
	}

	setClauses := fmt.Sprintf("version = %d, updated_at = CURRENT_TIMESTAMP, last_event_id = '%s'",
		event.EventVersion, event.EventID)

	if payload.AnoEscolar != nil {
		setClauses += fmt.Sprintf(", ano_escolar = '%s'", db.SafeString(*payload.AnoEscolar))
	}
	if payload.AnoEscolarMedio != nil {
		setClauses += fmt.Sprintf(", ano_escolar_medio = '%s'", db.SafeString(*payload.AnoEscolarMedio))
	}
	if payload.AnoSuperior != nil {
		setClauses += fmt.Sprintf(", ano_superior = '%s'", db.SafeString(*payload.AnoSuperior))
	}
	if payload.CursoMedioID != nil {
		setClauses += fmt.Sprintf(", curso_medio_id = '%s'", payload.CursoMedioID.String())
	}
	if payload.CursoSuperiorID != nil {
		setClauses += fmt.Sprintf(", curso_superior_id = '%s'", payload.CursoSuperiorID.String())
	}

	query := fmt.Sprintf(`UPDATE projection_estudantes SET %s WHERE id = '%s'`, setClauses, event.AggregateID)
	_, err := p.client.DB().Exec(query)
	return err
}

func (p *EstudanteProjection) handleEmailVerificado(event db.Event) error {
	query := fmt.Sprintf(`
		UPDATE projection_estudantes
		SET email_verificado = TRUE, version = %d,
			updated_at = CURRENT_TIMESTAMP, last_event_id = '%s'
		WHERE id = '%s'
	`, event.EventVersion, event.EventID, event.AggregateID)
	_, err := p.client.DB().Exec(query)
	return err
}

func (p *EstudanteProjection) handleCursoAlterado(event db.Event) error {
	var payload struct {
		CursoID    uuid.UUID
		TipoEnsino string
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error: %w", err)
	}

	col := "curso_medio_id"
	if payload.TipoEnsino == "superior" {
		col = "curso_superior_id"
	}

	query := fmt.Sprintf(`
		UPDATE projection_estudantes
		SET %s = '%s', version = %d,
			updated_at = CURRENT_TIMESTAMP, last_event_id = '%s'
		WHERE id = '%s'
	`, col, payload.CursoID.String(), event.EventVersion, event.EventID, event.AggregateID)
	_, err := p.client.DB().Exec(query)
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

	var setClauses string
	if payload.ProximoNivel != nil {
		switch payload.TipoEnsino {
		case "fundamental":
			setClauses = fmt.Sprintf("ano_escolar = '%s'", db.SafeString(*payload.ProximoNivel))
		case "medio":
			setClauses = fmt.Sprintf("ano_escolar_medio = '%s'", db.SafeString(*payload.ProximoNivel))
		case "superior":
			setClauses = fmt.Sprintf("ano_superior = '%s'", db.SafeString(*payload.ProximoNivel))
		}
	} else {
		switch payload.TipoEnsino {
		case "fundamental":
			setClauses = "status_escolar_fundamental = 'finalizado'"
		case "medio":
			setClauses = "status_escolar_medio = 'finalizado'"
		case "superior":
			setClauses = "status_superior = 'finalizado'"
		}
	}

	if setClauses == "" {
		return nil
	}

	query := fmt.Sprintf(`
		UPDATE projection_estudantes
		SET %s, version = %d, updated_at = CURRENT_TIMESTAMP, last_event_id = '%s'
		WHERE id = '%s'
	`, setClauses, event.EventVersion, event.EventID, event.AggregateID)
	_, err := p.client.DB().Exec(query)
	return err
}

func (p *EstudanteProjection) handleStatusEscolarFundamentalAtualizado(event db.Event) error {
	var payload struct {
		NovoStatus string `json:"NovoStatus"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error: %w", err)
	}
	query := fmt.Sprintf(`
		UPDATE projection_estudantes
		SET status_escolar_fundamental = '%s', version = %d,
			updated_at = CURRENT_TIMESTAMP, last_event_id = '%s'
		WHERE id = '%s'
	`, db.SafeString(payload.NovoStatus), event.EventVersion, event.EventID, event.AggregateID)
	_, err := p.client.DB().Exec(query)
	return err
}

func (p *EstudanteProjection) handleStatusEscolarMedioAtualizado(event db.Event) error {
	var payload struct {
		NovoStatus string `json:"NovoStatus"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error: %w", err)
	}
	query := fmt.Sprintf(`
		UPDATE projection_estudantes
		SET status_escolar_medio = '%s', version = %d,
			updated_at = CURRENT_TIMESTAMP, last_event_id = '%s'
		WHERE id = '%s'
	`, db.SafeString(payload.NovoStatus), event.EventVersion, event.EventID, event.AggregateID)
	_, err := p.client.DB().Exec(query)
	return err
}

// handleAvaliacaoFinalAnoAcademico atualiza projection_estudantes quando o novo
// evento AvaliacaoFinalAnoAcademico é emitido.
// - Aprovado + ProximoAnoAcademico != nil  → avança o ano do ciclo correspondente.
// - Aprovado + ProximoAnoAcademico == nil  → finaliza o ciclo (status = "finalizado").
// - Reprovado                              → nenhuma alteração de estado.
func (p *EstudanteProjection) handleAvaliacaoFinalAnoAcademico(event db.Event) error {
	var payload struct {
		TipoEnsino          string  `json:"tipo_ensino"`
		ProximoAnoAcademico *string `json:"proximo_ano_academico"`
		Aprovado            bool    `json:"aprovado"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error AvaliacaoFinalAnoAcademico: %w", err)
	}

	// Reprovação: não altera estado da projeção do estudante
	if !payload.Aprovado {
		return nil
	}

	var setClauses string
	if payload.ProximoAnoAcademico != nil {
		switch payload.TipoEnsino {
		case "fundamental":
			setClauses = fmt.Sprintf("ano_escolar = '%s'", db.SafeString(*payload.ProximoAnoAcademico))
		case "medio":
			setClauses = fmt.Sprintf("ano_escolar_medio = '%s'", db.SafeString(*payload.ProximoAnoAcademico))
		case "superior":
			setClauses = fmt.Sprintf("ano_superior = '%s'", db.SafeString(*payload.ProximoAnoAcademico))
		}
	} else {
		// Último ano do ciclo — finaliza
		switch payload.TipoEnsino {
		case "fundamental":
			setClauses = "status_escolar_fundamental = 'finalizado'"
		case "medio":
			setClauses = "status_escolar_medio = 'finalizado'"
		case "superior":
			setClauses = "status_superior = 'finalizado'"
		}
	}

	if setClauses == "" {
		return nil
	}

	query := fmt.Sprintf(`
		UPDATE projection_estudantes
		SET %s, version = %d, updated_at = CURRENT_TIMESTAMP, last_event_id = '%s'
		WHERE id = '%s'
	`, setClauses, event.EventVersion, event.EventID, event.AggregateID)

	_, err := p.client.DB().Exec(query)
	return err
}

// ============================================================================
// Query methods
// ============================================================================

func (p *EstudanteProjection) GetByID(id uuid.UUID) (*EstudanteDTO, error) {
	return p.queryEstudante(fmt.Sprintf("id = '%s'", id))
}

func (p *EstudanteProjection) GetByCodigo(codigo string) (*EstudanteDTO, error) {
	return p.queryEstudante(fmt.Sprintf("codigo_estudante = '%s'", db.SafeString(codigo)))
}

func (p *EstudanteProjection) GetByBilhete(bilhete string) (*EstudanteDTO, error) {
	safe := db.SafeString(bilhete)
	return p.queryEstudante(fmt.Sprintf("bilhete_identidade = '%s' OR bilhete_identidade_responsavel = '%s'", safe, safe))
}

func (p *EstudanteProjection) GetByBilheteIdentidadePrincipal(bilhete string) (*EstudanteDTO, error) {
	return p.queryEstudante(fmt.Sprintf("bilhete_identidade = '%s'", db.SafeString(bilhete)))
}

func (p *EstudanteProjection) queryEstudante(whereClause string) (*EstudanteDTO, error) {
	query := fmt.Sprintf(`
		SELECT id, nome, codigo_estudante, senha_hash, email, telefone, email_verificado,
			bilhete_identidade, bilhete_identidade_responsavel, codigo_academia,
			status, status_escolar_fundamental, status_escolar_medio, status_superior,
			ano_escolar, ano_escolar_medio, ano_superior, curso_medio_id, curso_superior_id,
			created_at, updated_at, total_notas, total_faltas, total_inscricoes, version
		FROM projection_estudantes WHERE %s LIMIT 1
	`, whereClause)

	var dto EstudanteDTO
	var cursoMedioID, cursoSuperiorID sql.NullString

	err := p.client.DB().QueryRow(query).Scan(
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