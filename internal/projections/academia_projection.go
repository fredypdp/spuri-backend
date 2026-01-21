package projections

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"spuri/internal/db"
	"time"

	"github.com/google/uuid"
)

type AcademiaProjection struct {
	client *db.Client
	ctx    context.Context
}

func NewAcademiaProjection(client *db.Client) *AcademiaProjection {
	return &AcademiaProjection{
		client: client,
		ctx:    context.Background(),
	}
}

func (p *AcademiaProjection) Name() string {
	return "academias"
}

func (p *AcademiaProjection) Handle(event db.Event) error {
	if event.AggregateType != "Academia" {
		return nil
	}

	switch event.EventType {
	case "AcademiaCriada":
		return p.handleAcademiaCriada(event)
	case "AcademiaAtivada":
		return p.handleAcademiaAtivada(event)
	case "AcademiaDesativada":
		return p.handleAcademiaDesativada(event)
	case "CursosAtualizados":
		return p.handleCursosAtualizados(event)
	case "InscricaoAprovada":
		return p.handleInscricaoAprovada(event)
	case "InscricaoReprovada":
		return p.handleInscricaoReprovada(event)
	case "AcademiaDadosAtualizados":
		return p.handleAcademiaDadosAtualizados(event)
	default:
		return nil
	}
}

func (p *AcademiaProjection) Rebuild() error {
	if err := p.clear(); err != nil {
		return err
	}

	ctx := context.Background()
	rows, err := p.client.DB().QueryContext(ctx, `
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE aggregate_type = 'Academia'
		ORDER BY id ASC
	`)
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

func (p *AcademiaProjection) GetLastProcessedEventID() (int64, error) {
	ctx := context.Background()
	var lastID int64
	err := p.client.DB().QueryRowContext(ctx, `
		SELECT last_processed_event_id 
		FROM projection_checkpoints 
		WHERE projection_name = $1
	`, p.Name()).Scan(&lastID)
	
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return lastID, err
}

func (p *AcademiaProjection) UpdateCheckpoint(eventID int64) error {
	ctx := context.Background()
	_, err := p.client.DB().ExecContext(ctx, `
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

func (p *AcademiaProjection) clear() error {
	ctx := context.Background()
	_, err := p.client.DB().ExecContext(ctx, `TRUNCATE TABLE projection_academias CASCADE`)
	return err
}

func (p *AcademiaProjection) handleAcademiaCriada(event db.Event) error {
	var payload struct {
		Type           string    `json:"Type"`
		Nome           string    `json:"Nome"`
		CodigoAcademia string    `json:"CodigoAcademia"`
		SenhaHash      string    `json:"SenhaHash"`
		Provincia      string    `json:"Provincia"`
		Endereco       string    `json:"Endereco"`
		NumeroTelefone *string   `json:"NumeroTelefone"`
		Email          *string   `json:"Email"`
		Website        *string   `json:"Website"`
		NivelEscolar   *string   `json:"NivelEscolar"`
		Cursos         []string  `json:"Cursos"`
		CreatedAt      time.Time `json:"CreatedAt"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	if payload.SenhaHash == "" {
		return fmt.Errorf("SenhaHash vazio no evento")
	}

	cursosJSON, err := json.Marshal(payload.Cursos)
	if err != nil {
		return err
	}

	ctx := context.Background()
	_, err = p.client.DB().ExecContext(ctx, `
		INSERT INTO projection_academias (
			id, type, nome, codigo_academia, senha_hash, provincia,
			endereco, numero_telefone, email, website, nivel_escolar,
			status, cursos, version, created_at, updated_at, last_event_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, 'inativo', $12, $13, $14, $15, $16)
		ON CONFLICT (id) DO UPDATE SET
			type = EXCLUDED.type, nome = EXCLUDED.nome,
			codigo_academia = EXCLUDED.codigo_academia, senha_hash = EXCLUDED.senha_hash,
			provincia = EXCLUDED.provincia, endereco = EXCLUDED.endereco,
			numero_telefone = EXCLUDED.numero_telefone, email = EXCLUDED.email,
			website = EXCLUDED.website, nivel_escolar = EXCLUDED.nivel_escolar,
			cursos = EXCLUDED.cursos, version = EXCLUDED.version,
			updated_at = EXCLUDED.updated_at, last_event_id = EXCLUDED.last_event_id
	`, event.AggregateID, payload.Type, payload.Nome, payload.CodigoAcademia,
		payload.SenhaHash, payload.Provincia, payload.Endereco, payload.NumeroTelefone,
		payload.Email, payload.Website, payload.NivelEscolar, cursosJSON,
		event.EventVersion, payload.CreatedAt, time.Now(), event.EventID)

	return err
}

func (p *AcademiaProjection) handleAcademiaAtivada(event db.Event) error {
	ctx := context.Background()
	_, err := p.client.DB().ExecContext(ctx, `
		UPDATE projection_academias
		SET status = 'ativo', version = $1, updated_at = CURRENT_TIMESTAMP, last_event_id = $2
		WHERE id = $3
	`, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

func (p *AcademiaProjection) handleAcademiaDesativada(event db.Event) error {
	ctx := context.Background()
	_, err := p.client.DB().ExecContext(ctx, `
		UPDATE projection_academias
		SET status = 'inativo', version = $1, updated_at = CURRENT_TIMESTAMP, last_event_id = $2
		WHERE id = $3
	`, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

func (p *AcademiaProjection) handleCursosAtualizados(event db.Event) error {
	var payload struct {
		NovoCursos []string `json:"NovoCursos"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	cursosJSON, err := json.Marshal(payload.NovoCursos)
	if err != nil {
		return err
	}

	ctx := context.Background()
	_, err = p.client.DB().ExecContext(ctx, `
		UPDATE projection_academias
		SET cursos = $1, version = $2, updated_at = CURRENT_TIMESTAMP, last_event_id = $3
		WHERE id = $4
	`, cursosJSON, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

func (p *AcademiaProjection) handleInscricaoAprovada(event db.Event) error {
	ctx := context.Background()
	_, err := p.client.DB().ExecContext(ctx, `
		UPDATE projection_academias
		SET total_estudantes = total_estudantes + 1,
			total_inscricoes_pendentes = GREATEST(total_inscricoes_pendentes - 1, 0),
			updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`, event.AggregateID)
	return err
}

func (p *AcademiaProjection) handleInscricaoReprovada(event db.Event) error {
	ctx := context.Background()
	_, err := p.client.DB().ExecContext(ctx, `
		UPDATE projection_academias
		SET total_inscricoes_pendentes = GREATEST(total_inscricoes_pendentes - 1, 0),
			updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`, event.AggregateID)
	return err
}

func (p *AcademiaProjection) handleAcademiaDadosAtualizados(event db.Event) error {
	var payload struct {
		Nome           *string  `json:"Nome"`
		Provincia      *string  `json:"Provincia"`
		Endereco       *string  `json:"Endereco"`
		NumeroTelefone *string  `json:"NumeroTelefone"`
		Email          *string  `json:"Email"`
		Website        *string  `json:"Website"`
		NivelEscolar   *string  `json:"NivelEscolar"`
		Cursos         []string `json:"Cursos"`
		EmailAlterado  bool     `json:"EmailAlterado"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	ctx := context.Background()
	
	if payload.Nome != nil {
		p.client.DB().ExecContext(ctx, `UPDATE projection_academias SET nome = $1 WHERE id = $2`, *payload.Nome, event.AggregateID)
	}
	if payload.Provincia != nil {
		p.client.DB().ExecContext(ctx, `UPDATE projection_academias SET provincia = $1 WHERE id = $2`, *payload.Provincia, event.AggregateID)
	}
	if payload.Endereco != nil {
		p.client.DB().ExecContext(ctx, `UPDATE projection_academias SET endereco = $1 WHERE id = $2`, *payload.Endereco, event.AggregateID)
	}
	if payload.NumeroTelefone != nil {
		p.client.DB().ExecContext(ctx, `UPDATE projection_academias SET numero_telefone = $1 WHERE id = $2`, *payload.NumeroTelefone, event.AggregateID)
	}
	if payload.Email != nil {
		if payload.EmailAlterado {
			p.client.DB().ExecContext(ctx, `UPDATE projection_academias SET email = $1, email_verificado = FALSE WHERE id = $2`, *payload.Email, event.AggregateID)
		} else {
			p.client.DB().ExecContext(ctx, `UPDATE projection_academias SET email = $1 WHERE id = $2`, *payload.Email, event.AggregateID)
		}
	}
	if payload.Website != nil {
		p.client.DB().ExecContext(ctx, `UPDATE projection_academias SET website = $1 WHERE id = $2`, *payload.Website, event.AggregateID)
	}
	if payload.NivelEscolar != nil {
		p.client.DB().ExecContext(ctx, `UPDATE projection_academias SET nivel_escolar = $1 WHERE id = $2`, *payload.NivelEscolar, event.AggregateID)
	}
	if payload.Cursos != nil {
		cursosJSON, _ := json.Marshal(payload.Cursos)
		p.client.DB().ExecContext(ctx, `UPDATE projection_academias SET cursos = $1 WHERE id = $2`, cursosJSON, event.AggregateID)
	}

	_, err := p.client.DB().ExecContext(ctx, `
		UPDATE projection_academias SET version = $1, updated_at = CURRENT_TIMESTAMP, last_event_id = $2 WHERE id = $3
	`, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

func (p *AcademiaProjection) GetByID(id uuid.UUID) (*AcademiaDTO, error) {
	ctx := context.Background()
	var dto AcademiaDTO
	var cursosJSON []byte

	err := p.client.DB().QueryRowContext(ctx, `
		SELECT id, type, nome, codigo_academia, senha_hash, provincia,
			endereco, numero_telefone, email, website, nivel_escolar,
			status, cursos, created_at, updated_at,
			total_estudantes, total_inscricoes_pendentes, version
		FROM projection_academias WHERE id = $1
	`, id).Scan(
		&dto.ID, &dto.Type, &dto.Nome, &dto.CodigoAcademia,
		&dto.SenhaHash, &dto.Provincia, &dto.Endereco,
		&dto.NumeroTelefone, &dto.Email, &dto.Website,
		&dto.NivelEscolar, &dto.Status, &cursosJSON,
		&dto.CreatedAt, &dto.UpdatedAt,
		&dto.TotalEstudantes, &dto.TotalInscricoesPendentes, &dto.Version,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	json.Unmarshal(cursosJSON, &dto.Cursos)
	return &dto, nil
}

func (p *AcademiaProjection) GetByCodigoOrEmail(identifier string) (*AcademiaDTO, error) {
	ctx := context.Background()
	var dto AcademiaDTO
	var cursosJSON []byte

	err := p.client.DB().QueryRowContext(ctx, `
		SELECT id, type, nome, codigo_academia, senha_hash, provincia,
			endereco, numero_telefone, email, website, nivel_escolar,
			status, cursos, created_at, updated_at,
			total_estudantes, total_inscricoes_pendentes, version
		FROM projection_academias
		WHERE codigo_academia = $1 OR email = $1
		LIMIT 1
	`, identifier).Scan(
		&dto.ID, &dto.Type, &dto.Nome, &dto.CodigoAcademia,
		&dto.SenhaHash, &dto.Provincia, &dto.Endereco,
		&dto.NumeroTelefone, &dto.Email, &dto.Website,
		&dto.NivelEscolar, &dto.Status, &cursosJSON,
		&dto.CreatedAt, &dto.UpdatedAt,
		&dto.TotalEstudantes, &dto.TotalInscricoesPendentes, &dto.Version,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	json.Unmarshal(cursosJSON, &dto.Cursos)
	return &dto, nil
}

func (p *AcademiaProjection) GetByCodigo(codigo string) (*AcademiaDTO, error) {
	ctx := context.Background()
	var dto AcademiaDTO
	var cursosJSON []byte

	err := p.client.DB().QueryRowContext(ctx, `
		SELECT id, type, nome, codigo_academia, senha_hash, provincia,
			endereco, numero_telefone, email, website, nivel_escolar,
			status, cursos, created_at, updated_at,
			total_estudantes, total_inscricoes_pendentes, version
		FROM projection_academias WHERE codigo_academia = $1
	`, codigo).Scan(
		&dto.ID, &dto.Type, &dto.Nome, &dto.CodigoAcademia,
		&dto.SenhaHash, &dto.Provincia, &dto.Endereco,
		&dto.NumeroTelefone, &dto.Email, &dto.Website,
		&dto.NivelEscolar, &dto.Status, &cursosJSON,
		&dto.CreatedAt, &dto.UpdatedAt,
		&dto.TotalEstudantes, &dto.TotalInscricoesPendentes, &dto.Version,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	json.Unmarshal(cursosJSON, &dto.Cursos)
	return &dto, nil
}

type AcademiaDTO struct {
	ID                       uuid.UUID `json:"id"`
	Type                     string    `json:"type"`
	Nome                     string    `json:"nome"`
	CodigoAcademia           string    `json:"codigo_academia"`
	SenhaHash                string    `json:"-"`
	Provincia                string    `json:"provincia"`
	Endereco                 string    `json:"endereco"`
	NumeroTelefone           *string   `json:"numero_telefone,omitempty"`
	Email                    *string   `json:"email,omitempty"`
	Website                  *string   `json:"website,omitempty"`
	NivelEscolar             *string   `json:"nivel_escolar,omitempty"`
	Status                   string    `json:"status"`
	Cursos                   []string  `json:"cursos"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
	TotalEstudantes          int       `json:"total_estudantes"`
	TotalInscricoesPendentes int       `json:"total_inscricoes_pendentes"`
	Version                  int       `json:"version"`
}