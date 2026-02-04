// ============================================================================
// ARQUIVO: internal/projections/estudante_projection.go
// ATUALIZADO: curso_medio_id e curso_superior_id agora são UUID
// ============================================================================

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
		"EstudanteCriado": p.handleEstudanteCriado,
		"InscricaoAprovada": p.handleInscricaoAprovada,
		"EstudanteVinculado": p.handleEstudanteVinculado,
		"StatusEscolarAtualizado": p.handleStatusEscolarAtualizado,
		"StatusSuperiorAtualizado": p.handleStatusSuperiorAtualizado,
		"DadosPessoaisAtualizados": p.handleDadosPessoaisAtualizados,
		"DadosAcademicosAtualizados": p.handleDadosAcademicosAtualizados,
		"EmailVerificado": p.handleEmailVerificado,
		"CursoAlterado": p.handleCursoAlterado,
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
			last_processed_event_id = %d, last_processed_at = CURRENT_TIMESTAMP,
			events_processed = projection_checkpoints.events_processed + 1
	`, db.SafeString(p.Name()), eventID, eventID)
	
	_, err := p.client.DB().Exec(query)
	return err
}

func (p *EstudanteProjection) clear() error {
	_, err := p.client.DB().Exec(`TRUNCATE TABLE projection_estudantes CASCADE`)
	return err
}

// 🔥 ATUALIZADO
func (p *EstudanteProjection) handleEstudanteCriado(event db.Event) error {
	var payload struct {
		Nome, CodigoEstudante, SenhaHash, StatusEscolar, StatusSuperior string
		Email, Telefone, BilheteIdentidade, BilheteIdentidadeResp       *string
		AnoEscolar, AnoSuperior                                         *string
		CursoMedioID, CursoSuperiorID                                   *uuid.UUID // 🔥 MUDOU
		CreatedAt                                                        time.Time
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error: %w", err)
	}

	if event.AggregateID == uuid.Nil || payload.SenhaHash == "" || payload.CodigoEstudante == "" {
		return fmt.Errorf("dados obrigatórios inválidos")
	}

	query := fmt.Sprintf(`
		INSERT INTO projection_estudantes (
			id, nome, codigo_estudante, senha_hash, email, telefone, email_verificado,
			bilhete_identidade, bilhete_identidade_responsavel, codigo_academia,
			status, status_escolar, status_superior, ano_escolar, ano_superior,
			curso_medio_id, curso_superior_id, version, created_at, updated_at, last_event_id
		) VALUES (
			'%s', '%s', '%s', '%s', %s, %s, FALSE, %s, %s, NULL,
			'inativo', '%s', '%s', %s, %s, %s, %s, %d, '%s', CURRENT_TIMESTAMP, '%s'
		)
		ON CONFLICT (id) DO UPDATE SET
			nome = EXCLUDED.nome, codigo_estudante = EXCLUDED.codigo_estudante,
			senha_hash = EXCLUDED.senha_hash, email = EXCLUDED.email, telefone = EXCLUDED.telefone,
			bilhete_identidade = EXCLUDED.bilhete_identidade,
			bilhete_identidade_responsavel = EXCLUDED.bilhete_identidade_responsavel,
			status = EXCLUDED.status, status_escolar = EXCLUDED.status_escolar,
			status_superior = EXCLUDED.status_superior, ano_escolar = EXCLUDED.ano_escolar,
			ano_superior = EXCLUDED.ano_superior, curso_medio_id = EXCLUDED.curso_medio_id,
			curso_superior_id = EXCLUDED.curso_superior_id, version = EXCLUDED.version,
			updated_at = EXCLUDED.updated_at, last_event_id = EXCLUDED.last_event_id
	`, event.AggregateID, db.SafeString(payload.Nome), db.SafeString(payload.CodigoEstudante),
		db.SafeString(payload.SenhaHash), nullOrString(payload.Email), nullOrString(payload.Telefone),
		nullOrString(payload.BilheteIdentidade), nullOrString(payload.BilheteIdentidadeResp),
		db.SafeString(payload.StatusEscolar), db.SafeString(payload.StatusSuperior),
		nullOrString(payload.AnoEscolar), nullOrString(payload.AnoSuperior),
		nullOrUUID(payload.CursoMedioID), nullOrUUID(payload.CursoSuperiorID), // 🔥 MUDOU
		event.EventVersion, payload.CreatedAt.Format(time.RFC3339), event.EventID)

	_, err := p.client.DB().Exec(query)
	return err
}

func (p *EstudanteProjection) handleInscricaoAprovada(event db.Event) error {
	query := fmt.Sprintf(`
		UPDATE projection_estudantes
		SET version = %d, updated_at = CURRENT_TIMESTAMP, last_event_id = '%s',
			total_inscricoes = total_inscricoes + 1
		WHERE id = '%s'
	`, event.EventVersion, event.EventID, event.AggregateID)
	
	_, err := p.client.DB().Exec(query)
	return err
}

func (p *EstudanteProjection) handleEstudanteVinculado(event db.Event) error {
	var payload struct{ CodigoAcademia string }

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}

	query := fmt.Sprintf(`
		UPDATE projection_estudantes
		SET codigo_academia = '%s', status = 'ativo', version = %d,
			updated_at = CURRENT_TIMESTAMP, last_event_id = '%s'
		WHERE id = '%s'
	`, db.SafeString(payload.CodigoAcademia), event.EventVersion, event.EventID, event.AggregateID)

	_, err := p.client.DB().Exec(query)
	return err
}

func (p *EstudanteProjection) handleStatusEscolarAtualizado(event db.Event) error {
	var payload struct{ NovoStatus string }

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}

	statusSup := "status_superior"
	if payload.NovoStatus == "inativo" {
		statusSup = "'inativo'"
	}

	query := fmt.Sprintf(`
		UPDATE projection_estudantes
		SET status_escolar = '%s', status_superior = %s, version = %d,
			updated_at = CURRENT_TIMESTAMP, last_event_id = '%s'
		WHERE id = '%s'
	`, db.SafeString(payload.NovoStatus), statusSup, event.EventVersion, event.EventID, event.AggregateID)
	
	_, err := p.client.DB().Exec(query)
	return err
}

func (p *EstudanteProjection) handleStatusSuperiorAtualizado(event db.Event) error {
	var payload struct{ NovoStatus string }

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}

	query := fmt.Sprintf(`
		UPDATE projection_estudantes
		SET status_superior = '%s', version = %d, updated_at = CURRENT_TIMESTAMP, last_event_id = '%s'
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
		return err
	}

	updates := map[string]*string{
		"nome":                             payload.Nome,
		"telefone":                         payload.Telefone,
		"bilhete_identidade":               payload.BilheteIdentidade,
		"bilhete_identidade_responsavel":   payload.BilheteIdentidadeResp,
	}

	for field, value := range updates {
		if value != nil {
			query := fmt.Sprintf(`UPDATE projection_estudantes SET %s = '%s' WHERE id = '%s'`,
				field, db.SafeString(*value), event.AggregateID)
			p.client.DB().Exec(query)
		}
	}

	if payload.Email != nil {
		emailVerif := "email_verificado"
		if payload.EmailAlterado {
			emailVerif = "FALSE"
		}
		query := fmt.Sprintf(`UPDATE projection_estudantes SET email = '%s', email_verificado = %s WHERE id = '%s'`,
			db.SafeString(*payload.Email), emailVerif, event.AggregateID)
		p.client.DB().Exec(query)
	}

	query := fmt.Sprintf(`
		UPDATE projection_estudantes
		SET version = %d, updated_at = CURRENT_TIMESTAMP, last_event_id = '%s'
		WHERE id = '%s'
	`, event.EventVersion, event.EventID, event.AggregateID)
	
	_, err := p.client.DB().Exec(query)
	return err
}

func (p *EstudanteProjection) handleDadosAcademicosAtualizados(event db.Event) error {
	var payload struct {
		AnoEscolar, AnoSuperior       *string
		CursoMedioID, CursoSuperiorID *uuid.UUID // 🔥 MUDOU
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}

	if payload.AnoEscolar != nil {
		query := fmt.Sprintf(`UPDATE projection_estudantes SET ano_escolar = '%s' WHERE id = '%s'`,
			db.SafeString(*payload.AnoEscolar), event.AggregateID)
		p.client.DB().Exec(query)
	}
	
	if payload.AnoSuperior != nil {
		query := fmt.Sprintf(`UPDATE projection_estudantes SET ano_superior = '%s' WHERE id = '%s'`,
			db.SafeString(*payload.AnoSuperior), event.AggregateID)
		p.client.DB().Exec(query)
	}
	
	// 🔥 MUDOU
	if payload.CursoMedioID != nil {
		query := fmt.Sprintf(`UPDATE projection_estudantes SET curso_medio_id = '%s' WHERE id = '%s'`,
			*payload.CursoMedioID, event.AggregateID)
		p.client.DB().Exec(query)
	}
	
	if payload.CursoSuperiorID != nil {
		query := fmt.Sprintf(`UPDATE projection_estudantes SET curso_superior_id = '%s' WHERE id = '%s'`,
			*payload.CursoSuperiorID, event.AggregateID)
		p.client.DB().Exec(query)
	}

	query := fmt.Sprintf(`
		UPDATE projection_estudantes
		SET version = %d, updated_at = CURRENT_TIMESTAMP, last_event_id = '%s'
		WHERE id = '%s'
	`, event.EventVersion, event.EventID, event.AggregateID)
	
	_, err := p.client.DB().Exec(query)
	return err
}

func (p *EstudanteProjection) handleCursoAlterado(event db.Event) error {
	var payload struct {
		TipoEnsino string
		CursoID    uuid.UUID
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}

	var query string
	if payload.TipoEnsino == "medio" {
		query = fmt.Sprintf(`
			UPDATE projection_estudantes 
			SET curso_medio_id = '%s', version = %d, updated_at = CURRENT_TIMESTAMP, last_event_id = '%s'
			WHERE id = '%s'
		`, payload.CursoID, event.EventVersion, event.EventID, event.AggregateID)
	} else {
		query = fmt.Sprintf(`
			UPDATE projection_estudantes 
			SET curso_superior_id = '%s', version = %d, updated_at = CURRENT_TIMESTAMP, last_event_id = '%s'
			WHERE id = '%s'
		`, payload.CursoID, event.EventVersion, event.EventID, event.AggregateID)
	}
	
	_, err := p.client.DB().Exec(query)
	return err
}

func (p *EstudanteProjection) handleEmailVerificado(event db.Event) error {
	query := fmt.Sprintf(`
		UPDATE projection_estudantes
		SET email_verificado = TRUE, updated_at = CURRENT_TIMESTAMP
		WHERE id = '%s'
	`, event.AggregateID)
	
	_, err := p.client.DB().Exec(query)
	return err
}

func (p *EstudanteProjection) GetByID(id uuid.UUID) (*EstudanteDTO, error) {
	if id == uuid.Nil {
		return nil, fmt.Errorf("UUID inválido")
	}
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

// 🔥 ATUALIZADO
func (p *EstudanteProjection) queryEstudante(whereClause string) (*EstudanteDTO, error) {
	query := fmt.Sprintf(`
		SELECT id, nome, codigo_estudante, senha_hash, email, telefone, email_verificado,
			bilhete_identidade, bilhete_identidade_responsavel, codigo_academia,
			status, status_escolar, status_superior, ano_escolar, ano_superior,
			curso_medio_id, curso_superior_id, created_at, updated_at, total_notas,
			total_faltas, total_inscricoes, version
		FROM projection_estudantes WHERE %s LIMIT 1
	`, whereClause)

	var dto EstudanteDTO
	var cursoMedioID, cursoSuperiorID sql.NullString
	
	err := p.client.DB().QueryRow(query).Scan(
		&dto.ID, &dto.Nome, &dto.CodigoEstudante, &dto.SenhaHash,
		&dto.Email, &dto.Telefone, &dto.EmailVerificado,
		&dto.BilheteIdentidade, &dto.BilheteIdentidadeResp, &dto.CodigoAcademia,
		&dto.Status, &dto.StatusEscolar, &dto.StatusSuperior,
		&dto.AnoEscolar, &dto.AnoSuperior, &cursoMedioID, &cursoSuperiorID,
		&dto.CreatedAt, &dto.UpdatedAt, &dto.TotalNotas, &dto.TotalFaltas,
		&dto.TotalInscricoes, &dto.Version,
	)
	
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	
	// 🔥 MUDOU
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

// 🔥 ATUALIZADO
type EstudanteDTO struct {
	ID                    uuid.UUID  `json:"id"`
	Nome                  string     `json:"nome"`
	CodigoEstudante       string     `json:"codigo_estudante"`
	SenhaHash             string     `json:"-"`
	Email                 *string    `json:"email,omitempty"`
	Telefone              *string    `json:"telefone,omitempty"`
	EmailVerificado       bool       `json:"email_verificado"`
	BilheteIdentidade     *string    `json:"bilhete_identidade,omitempty"`
	BilheteIdentidadeResp *string    `json:"bilhete_identidade_responsavel,omitempty"`
	CodigoAcademia        *string    `json:"codigo_academia,omitempty"`
	Status                string     `json:"status"`
	StatusEscolar         string     `json:"status_escolar"`
	StatusSuperior        string     `json:"status_superior"`
	AnoEscolar            *string    `json:"ano_escolar,omitempty"`
	AnoSuperior           *string    `json:"ano_superior,omitempty"`
	CursoMedioID          *uuid.UUID `json:"curso_medio_id,omitempty"`    // 🔥 MUDOU
	CursoSuperiorID       *uuid.UUID `json:"curso_superior_id,omitempty"` // 🔥 MUDOU
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
	TotalNotas            int        `json:"total_notas"`
	TotalFaltas           int        `json:"total_faltas"`
	TotalInscricoes       int        `json:"total_inscricoes"`
	Version               int        `json:"version"`
}