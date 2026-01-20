package projections

import (
	"context"
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
	ctx    context.Context
}

func NewEstudanteProjection(client *db.Client) *EstudanteProjection {
	return &EstudanteProjection{
		client: client,
		ctx:    context.Background(),
	}
}

func (p *EstudanteProjection) Name() string {
	return "estudantes"
}

func (p *EstudanteProjection) Handle(event db.Event) error {
	if event.AggregateType != "Estudante" {
		return nil
	}

	switch event.EventType {
	case "EstudanteCriado":
		return p.handleEstudanteCriado(event)
	case "InscricaoAprovada":
		return p.handleInscricaoAprovada(event)
	case "EstudanteVinculado":
		return p.handleEstudanteVinculado(event)
	case "StatusEscolarAtualizado":
		return p.handleStatusEscolarAtualizado(event)
	case "StatusSuperiorAtualizado":
		return p.handleStatusSuperiorAtualizado(event)
	case "DadosPessoaisAtualizados":
		return p.handleDadosPessoaisAtualizados(event)
	case "DadosAcademicosAtualizados":
		return p.handleDadosAcademicosAtualizados(event)
	default:
		return nil
	}
}

func (p *EstudanteProjection) Rebuild() error {
	if err := p.clear(); err != nil {
		return err
	}

	rows, err := p.client.DB().Query(`
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE aggregate_type = $1
		ORDER BY id ASC
	`, "Estudante")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var event db.Event
		err := rows.Scan(
			&event.ID, &event.EventID, &event.AggregateID, &event.AggregateType,
			&event.EventType, &event.EventVersion, &event.Payload, &event.Metadata,
			&event.OccurredAt, &event.RecordedAt, &event.LedgerHash, &event.PreviousHash,
		)
		if err != nil {
			return err
		}

		if err := p.Handle(event); err != nil {
			return fmt.Errorf("erro ao processar evento %d: %w", event.ID, err)
		}
	}

	return rows.Err()
}

func (p *EstudanteProjection) GetLastProcessedEventID() (int64, error) {
	var lastID int64
	err := p.client.DB().QueryRow(`
		SELECT last_processed_event_id 
		FROM projection_checkpoints 
		WHERE projection_name = $1
	`, p.Name()).Scan(&lastID)
	
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return lastID, err
}

func (p *EstudanteProjection) UpdateCheckpoint(eventID int64) error {
	_, err := p.client.DB().Exec(`
		INSERT INTO projection_checkpoints (
			projection_name, last_processed_event_id, last_processed_at, events_processed
		) VALUES ($1, $2, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) 
		DO UPDATE SET
			last_processed_event_id = $2,
			last_processed_at = CURRENT_TIMESTAMP,
			events_processed = projection_checkpoints.events_processed + 1
	`, p.Name(), eventID)
	return err
}

func (p *EstudanteProjection) clear() error {
	_, err := p.client.DB().Exec(`TRUNCATE TABLE projection_estudantes CASCADE`)
	return err
}

func (p *EstudanteProjection) handleEstudanteCriado(event db.Event) error {
	var payload struct {
		Nome                  string    `json:"Nome"`
		CodigoEstudante       string    `json:"CodigoEstudante"`
		SenhaHash             string    `json:"SenhaHash"`
		Email                 *string   `json:"Email"`
		Telefone              *string   `json:"Telefone"`
		BilheteIdentidade     *string   `json:"BilheteIdentidade"`
		BilheteIdentidadeResp *string   `json:"BilheteIdentidadeResp"`
		AnoEscolar            *string   `json:"AnoEscolar"`
		AnoSuperior           *string   `json:"AnoSuperior"`
		CursoMedio            *string   `json:"CursoMedio"`
		CursoSuperior         *string   `json:"CursoSuperior"`
		StatusEscolar         string    `json:"StatusEscolar"`
		StatusSuperior        string    `json:"StatusSuperior"`
		CreatedAt             time.Time `json:"CreatedAt"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	if payload.SenhaHash == "" {
		return fmt.Errorf("SenhaHash vazio no evento")
	}
	if payload.CodigoEstudante == "" {
		return fmt.Errorf("CodigoEstudante vazio no evento")
	}

	_, err := p.client.DB().Exec(`
		INSERT INTO projection_estudantes (
			id, nome, codigo_estudante, senha_hash, 
			email, telefone, email_verificado,
			bilhete_identidade, bilhete_identidade_responsavel, codigo_academia,
			status, status_escolar, status_superior,
			ano_escolar, ano_superior, curso_medio, curso_superior,
			version, created_at, updated_at, last_event_id
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
			$11, $12, $13, $14, $15, $16, $17,
			$18, $19, $20, $21
		)
		ON CONFLICT (id) DO UPDATE SET
			nome = EXCLUDED.nome, codigo_estudante = EXCLUDED.codigo_estudante,
			senha_hash = EXCLUDED.senha_hash, email = EXCLUDED.email,
			telefone = EXCLUDED.telefone, bilhete_identidade = EXCLUDED.bilhete_identidade,
			bilhete_identidade_responsavel = EXCLUDED.bilhete_identidade_responsavel,
			status = EXCLUDED.status, status_escolar = EXCLUDED.status_escolar,
			status_superior = EXCLUDED.status_superior, ano_escolar = EXCLUDED.ano_escolar,
			ano_superior = EXCLUDED.ano_superior, curso_medio = EXCLUDED.curso_medio,
			curso_superior = EXCLUDED.curso_superior, version = EXCLUDED.version,
			updated_at = EXCLUDED.updated_at, last_event_id = EXCLUDED.last_event_id
	`, event.AggregateID, payload.Nome, payload.CodigoEstudante, payload.SenhaHash,
		payload.Email, payload.Telefone, false, payload.BilheteIdentidade, payload.BilheteIdentidadeResp, nil,
		"inativo", payload.StatusEscolar, payload.StatusSuperior, payload.AnoEscolar, payload.AnoSuperior,
		payload.CursoMedio, payload.CursoSuperior, event.EventVersion, payload.CreatedAt,
		time.Now(), event.EventID)

	if err != nil {
		// ✅ CORRIGIDO: Log genérico sem dados sensíveis
		log.Printf("[ESTUDANTE_PROJECTION] Erro ao processar EstudanteCriado (event_id: %s)", event.EventID)
		return err
	}

	return nil
}

func (p *EstudanteProjection) handleInscricaoAprovada(event db.Event) error {
	_, err := p.client.DB().Exec(`
		UPDATE projection_estudantes
		SET version = $1, updated_at = CURRENT_TIMESTAMP, last_event_id = $2,
			total_inscricoes = total_inscricoes + 1
		WHERE id = $3
	`, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

func (p *EstudanteProjection) handleEstudanteVinculado(event db.Event) error {
	var payload struct {
		CodigoAcademia string `json:"CodigoAcademia"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	_, err := p.client.DB().Exec(`
		UPDATE projection_estudantes
		SET codigo_academia = $1, status = $2, version = $3,
			updated_at = CURRENT_TIMESTAMP, last_event_id = $4
		WHERE id = $5
	`, payload.CodigoAcademia, "ativo", event.EventVersion, event.EventID, event.AggregateID)

	return err
}

func (p *EstudanteProjection) handleStatusEscolarAtualizado(event db.Event) error {
	var payload struct {
		NovoStatus string `json:"NovoStatus"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	if payload.NovoStatus == "inativo" {
		_, err := p.client.DB().Exec(`
			UPDATE projection_estudantes
			SET status_escolar = $1, status_superior = $2, version = $3,
				updated_at = CURRENT_TIMESTAMP, last_event_id = $4
			WHERE id = $5
		`, payload.NovoStatus, "inativo", event.EventVersion, event.EventID, event.AggregateID)
		return err
	}

	_, err := p.client.DB().Exec(`
		UPDATE projection_estudantes
		SET status_escolar = $1, version = $2, updated_at = CURRENT_TIMESTAMP, last_event_id = $3
		WHERE id = $4
	`, payload.NovoStatus, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

func (p *EstudanteProjection) handleStatusSuperiorAtualizado(event db.Event) error {
	var payload struct {
		NovoStatus string `json:"NovoStatus"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	_, err := p.client.DB().Exec(`
		UPDATE projection_estudantes
		SET status_superior = $1, version = $2, updated_at = CURRENT_TIMESTAMP, last_event_id = $3
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
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	if payload.Nome != nil {
		_, err := p.client.DB().Exec(`UPDATE projection_estudantes SET nome = $1 WHERE id = $2`, *payload.Nome, event.AggregateID)
		if err != nil {
			return err
		}
	}
	if payload.Email != nil {
		if payload.EmailAlterado {
			_, err := p.client.DB().Exec(`UPDATE projection_estudantes SET email = $1, email_verificado = $2 WHERE id = $3`, *payload.Email, false, event.AggregateID)
			if err != nil {
				return err
			}
		} else {
			_, err := p.client.DB().Exec(`UPDATE projection_estudantes SET email = $1 WHERE id = $2`, *payload.Email, event.AggregateID)
			if err != nil {
				return err
			}
		}
	}
	if payload.Telefone != nil {
		_, err := p.client.DB().Exec(`UPDATE projection_estudantes SET telefone = $1 WHERE id = $2`, *payload.Telefone, event.AggregateID)
		if err != nil {
			return err
		}
	}
	if payload.BilheteIdentidade != nil {
		_, err := p.client.DB().Exec(`UPDATE projection_estudantes SET bilhete_identidade = $1 WHERE id = $2`, *payload.BilheteIdentidade, event.AggregateID)
		if err != nil {
			return err
		}
	}
	if payload.BilheteIdentidadeResp != nil {
		_, err := p.client.DB().Exec(`UPDATE projection_estudantes SET bilhete_identidade_responsavel = $1 WHERE id = $2`, *payload.BilheteIdentidadeResp, event.AggregateID)
		if err != nil {
			return err
		}
	}

	_, err := p.client.DB().Exec(`
		UPDATE projection_estudantes SET version = $1, updated_at = CURRENT_TIMESTAMP, last_event_id = $2 WHERE id = $3
	`, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

func (p *EstudanteProjection) handleDadosAcademicosAtualizados(event db.Event) error {
	var payload struct {
		AnoEscolar    *string `json:"AnoEscolar"`
		AnoSuperior   *string `json:"AnoSuperior"`
		CursoMedio    *string `json:"CursoMedio"`
		CursoSuperior *string `json:"CursoSuperior"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	if payload.AnoEscolar != nil {
		_, err := p.client.DB().Exec(`UPDATE projection_estudantes SET ano_escolar = $1 WHERE id = $2`, *payload.AnoEscolar, event.AggregateID)
		if err != nil {
			return err
		}
	}
	if payload.AnoSuperior != nil {
		_, err := p.client.DB().Exec(`UPDATE projection_estudantes SET ano_superior = $1 WHERE id = $2`, *payload.AnoSuperior, event.AggregateID)
		if err != nil {
			return err
		}
	}
	if payload.CursoMedio != nil {
		_, err := p.client.DB().Exec(`UPDATE projection_estudantes SET curso_medio = $1 WHERE id = $2`, *payload.CursoMedio, event.AggregateID)
		if err != nil {
			return err
		}
	}
	if payload.CursoSuperior != nil {
		_, err := p.client.DB().Exec(`UPDATE projection_estudantes SET curso_superior = $1 WHERE id = $2`, *payload.CursoSuperior, event.AggregateID)
		if err != nil {
			return err
		}
	}

	_, err := p.client.DB().Exec(`
		UPDATE projection_estudantes SET version = $1, updated_at = CURRENT_TIMESTAMP, last_event_id = $2 WHERE id = $3
	`, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

func (p *EstudanteProjection) GetByID(id uuid.UUID) (*EstudanteDTO, error) {
	var dto EstudanteDTO
	err := p.client.DB().QueryRow(`
		SELECT id, nome, codigo_estudante, senha_hash, 
			email, telefone, email_verificado,
			bilhete_identidade, bilhete_identidade_responsavel, codigo_academia,
			status, status_escolar, status_superior,
			ano_escolar, ano_superior, curso_medio, curso_superior,
			created_at, updated_at, total_notas, total_faltas, total_inscricoes, version
		FROM projection_estudantes WHERE id = $1
	`, id).Scan(
		&dto.ID, &dto.Nome, &dto.CodigoEstudante, &dto.SenhaHash,
		&dto.Email, &dto.Telefone, &dto.EmailVerificado,
		&dto.BilheteIdentidade, &dto.BilheteIdentidadeResp, &dto.CodigoAcademia,
		&dto.Status, &dto.StatusEscolar, &dto.StatusSuperior,
		&dto.AnoEscolar, &dto.AnoSuperior, &dto.CursoMedio, &dto.CursoSuperior,
		&dto.CreatedAt, &dto.UpdatedAt,
		&dto.TotalNotas, &dto.TotalFaltas, &dto.TotalInscricoes, &dto.Version,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &dto, err
}

func (p *EstudanteProjection) GetByCodigo(codigo string) (*EstudanteDTO, error) {
	var dto EstudanteDTO
	err := p.client.DB().QueryRow(`
		SELECT id, nome, codigo_estudante, senha_hash, 
			email, telefone, email_verificado,
			bilhete_identidade, bilhete_identidade_responsavel, codigo_academia,
			status, status_escolar, status_superior,
			ano_escolar, ano_superior, curso_medio, curso_superior,
			created_at, updated_at, total_notas, total_faltas, total_inscricoes, version
		FROM projection_estudantes WHERE codigo_estudante = $1
	`, codigo).Scan(
		&dto.ID, &dto.Nome, &dto.CodigoEstudante, &dto.SenhaHash,
		&dto.Email, &dto.Telefone, &dto.EmailVerificado,
		&dto.BilheteIdentidade, &dto.BilheteIdentidadeResp, &dto.CodigoAcademia,
		&dto.Status, &dto.StatusEscolar, &dto.StatusSuperior,
		&dto.AnoEscolar, &dto.AnoSuperior, &dto.CursoMedio, &dto.CursoSuperior,
		&dto.CreatedAt, &dto.UpdatedAt,
		&dto.TotalNotas, &dto.TotalFaltas, &dto.TotalInscricoes, &dto.Version,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &dto, nil
}

func (p *EstudanteProjection) GetByBilhete(bilhete string) (*EstudanteDTO, error) {
	var dto EstudanteDTO
	err := p.client.DB().QueryRow(`
		SELECT id, nome, codigo_estudante, senha_hash, 
			email, telefone, email_verificado,
			bilhete_identidade, bilhete_identidade_responsavel, codigo_academia,
			status, status_escolar, status_superior,
			ano_escolar, ano_superior, curso_medio, curso_superior,
			created_at, updated_at, total_notas, total_faltas, total_inscricoes, version
		FROM projection_estudantes
		WHERE bilhete_identidade = $1 OR bilhete_identidade_responsavel = $1
		LIMIT 1
	`, bilhete).Scan(
		&dto.ID, &dto.Nome, &dto.CodigoEstudante, &dto.SenhaHash,
		&dto.Email, &dto.Telefone, &dto.EmailVerificado,
		&dto.BilheteIdentidade, &dto.BilheteIdentidadeResp, &dto.CodigoAcademia,
		&dto.Status, &dto.StatusEscolar, &dto.StatusSuperior,
		&dto.AnoEscolar, &dto.AnoSuperior, &dto.CursoMedio, &dto.CursoSuperior,
		&dto.CreatedAt, &dto.UpdatedAt,
		&dto.TotalNotas, &dto.TotalFaltas, &dto.TotalInscricoes, &dto.Version,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &dto, err
}

type EstudanteDTO struct {
	ID                    uuid.UUID `db:"id" json:"id"`
	Nome                  string    `db:"nome" json:"nome"`
	CodigoEstudante       string    `db:"codigo_estudante" json:"codigo_estudante"`
	SenhaHash             string    `db:"senha_hash" json:"-"`
	Email                 *string   `db:"email" json:"email,omitempty"`
	Telefone              *string   `db:"telefone" json:"telefone,omitempty"`
	EmailVerificado       bool      `db:"email_verificado" json:"email_verificado"`
	BilheteIdentidade     *string   `db:"bilhete_identidade" json:"bilhete_identidade,omitempty"`
	BilheteIdentidadeResp *string   `db:"bilhete_identidade_responsavel" json:"bilhete_identidade_responsavel,omitempty"`
	CodigoAcademia        *string   `db:"codigo_academia" json:"codigo_academia,omitempty"`
	Status                string    `db:"status" json:"status"`
	StatusEscolar         string    `db:"status_escolar" json:"status_escolar"`
	StatusSuperior        string    `db:"status_superior" json:"status_superior"`
	AnoEscolar            *string   `db:"ano_escolar" json:"ano_escolar,omitempty"`
	AnoSuperior           *string   `db:"ano_superior" json:"ano_superior,omitempty"`
	CursoMedio            *string   `db:"curso_medio" json:"curso_medio,omitempty"`
	CursoSuperior         *string   `db:"curso_superior" json:"curso_superior,omitempty"`
	CreatedAt             time.Time `db:"created_at" json:"created_at"`
	UpdatedAt             time.Time `db:"updated_at" json:"updated_at"`
	TotalNotas            int       `db:"total_notas" json:"total_notas"`
	TotalFaltas           int       `db:"total_faltas" json:"total_faltas"`
	TotalInscricoes       int       `db:"total_inscricoes" json:"total_inscricoes"`
	Version               int       `db:"version" json:"version"`
}