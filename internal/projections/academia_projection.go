// ============================================================================
// ARQUIVO: internal/projections/academia_projection.go
// 🔥 CORRIGIDO: GetLastProcessedEventID com tratamento de erro
// ============================================================================

package projections

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"spuri/internal/genesisdb"
	"time"

	"github.com/google/uuid"
)

type AcademiaProjection struct {
	client *genesisdb.Client
	ctx    context.Context
}

func NewAcademiaProjection(client *genesisdb.Client) *AcademiaProjection {
	return &AcademiaProjection{
		client: client,
		ctx:    context.Background(),
	}
}

func (p *AcademiaProjection) Name() string {
	return "academias"
}

func (p *AcademiaProjection) Handle(event genesisdb.Event) error {
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
	default:
		return nil
	}
}

func (p *AcademiaProjection) Rebuild() error {
	if err := p.clear(); err != nil {
		return err
	}

	query := `
		SELECT 
			id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM genesis_ledger
		WHERE aggregate_type = 'Academia'
		ORDER BY id ASC
	`

	rows, err := p.client.DB().QueryContext(p.ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var event genesisdb.Event
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

// 🔥 CORRIGIDO: Tratamento de sql.ErrNoRows
func (p *AcademiaProjection) GetLastProcessedEventID() (int64, error) {
	query := `
		SELECT last_processed_event_id 
		FROM projection_checkpoints 
		WHERE projection_name = $1
	`

	var lastID int64
	err := p.client.DB().QueryRowContext(p.ctx, query, p.Name()).Scan(&lastID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	return lastID, nil
}

func (p *AcademiaProjection) UpdateCheckpoint(eventID int64) error {
	query := `
		INSERT INTO projection_checkpoints (
			projection_name, 
			last_processed_event_id, 
			last_processed_at,
			events_processed
		) VALUES ($1, $2, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) 
		DO UPDATE SET
			last_processed_event_id = $2,
			last_processed_at = CURRENT_TIMESTAMP,
			events_processed = projection_checkpoints.events_processed + 1
	`

	_, err := p.client.DB().ExecContext(p.ctx, query, p.Name(), eventID)
	return err
}

func (p *AcademiaProjection) clear() error {
	_, err := p.client.DB().ExecContext(p.ctx, `TRUNCATE TABLE projection_academias CASCADE`)
	return err
}

func (p *AcademiaProjection) handleAcademiaCriada(event genesisdb.Event) error {
	log.Printf("🔵 [PROJEÇÃO ACADEMIA] Processando AcademiaCriada")
	
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

	query := `
		INSERT INTO projection_academias (
			id, type, nome, codigo_academia, senha_hash, provincia,
			endereco, numero_telefone, email, website, nivel_escolar,
			status, cursos, version, created_at, updated_at, last_event_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		ON CONFLICT (id) DO UPDATE SET
			type = EXCLUDED.type,
			nome = EXCLUDED.nome,
			codigo_academia = EXCLUDED.codigo_academia,
			senha_hash = EXCLUDED.senha_hash,
			provincia = EXCLUDED.provincia,
			endereco = EXCLUDED.endereco,
			numero_telefone = EXCLUDED.numero_telefone,
			email = EXCLUDED.email,
			website = EXCLUDED.website,
			nivel_escolar = EXCLUDED.nivel_escolar,
			cursos = EXCLUDED.cursos,
			version = EXCLUDED.version,
			updated_at = EXCLUDED.updated_at,
			last_event_id = EXCLUDED.last_event_id
	`

	_, err = p.client.DB().ExecContext(
		p.ctx, query,
		event.AggregateID, payload.Type, payload.Nome, payload.CodigoAcademia,
		payload.SenhaHash, payload.Provincia, payload.Endereco,
		payload.NumeroTelefone, payload.Email, payload.Website,
		payload.NivelEscolar, "ativo", cursosJSON, event.EventVersion,
		payload.CreatedAt, time.Now(), event.EventID,
	)

	return err
}

func (p *AcademiaProjection) handleAcademiaAtivada(event genesisdb.Event) error {
	query := `
		UPDATE projection_academias
		SET 
			status = 'ativo',
			version = $1,
			updated_at = CURRENT_TIMESTAMP,
			last_event_id = $2
		WHERE id = $3
	`

	_, err := p.client.DB().ExecContext(
		p.ctx, query,
		event.EventVersion,
		event.EventID,
		event.AggregateID,
	)

	return err
}

func (p *AcademiaProjection) handleAcademiaDesativada(event genesisdb.Event) error {
	query := `
		UPDATE projection_academias
		SET 
			status = 'inativo',
			version = $1,
			updated_at = CURRENT_TIMESTAMP,
			last_event_id = $2
		WHERE id = $3
	`

	_, err := p.client.DB().ExecContext(
		p.ctx, query,
		event.EventVersion,
		event.EventID,
		event.AggregateID,
	)

	return err
}

func (p *AcademiaProjection) handleCursosAtualizados(event genesisdb.Event) error {
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

	query := `
		UPDATE projection_academias
		SET 
			cursos = $1,
			version = $2,
			updated_at = CURRENT_TIMESTAMP,
			last_event_id = $3
		WHERE id = $4
	`

	_, err = p.client.DB().ExecContext(
		p.ctx, query,
		cursosJSON,
		event.EventVersion,
		event.EventID,
		event.AggregateID,
	)

	return err
}

func (p *AcademiaProjection) handleInscricaoAprovada(event genesisdb.Event) error {
	query := `
		UPDATE projection_academias
		SET 
			total_estudantes = total_estudantes + 1,
			total_inscricoes_pendentes = GREATEST(total_inscricoes_pendentes - 1, 0),
			updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`

	_, err := p.client.DB().ExecContext(p.ctx, query, event.AggregateID)
	return err
}

func (p *AcademiaProjection) handleInscricaoReprovada(event genesisdb.Event) error {
	query := `
		UPDATE projection_academias
		SET 
			total_inscricoes_pendentes = GREATEST(total_inscricoes_pendentes - 1, 0),
			updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`

	_, err := p.client.DB().ExecContext(p.ctx, query, event.AggregateID)
	return err
}

// Query methods

func (p *AcademiaProjection) GetByID(id uuid.UUID) (*AcademiaDTO, error) {
	query := `
		SELECT 
			id, type, nome, codigo_academia, senha_hash, provincia,
			endereco, numero_telefone, email, website, nivel_escolar,
			status, cursos, created_at, updated_at,
			total_estudantes, total_inscricoes_pendentes, version
		FROM projection_academias
		WHERE id = $1
	`

	var dto AcademiaDTO
	var cursosJSON []byte

	err := p.client.DB().QueryRowContext(p.ctx, query, id).Scan(
		&dto.ID, &dto.Type, &dto.Nome, &dto.CodigoAcademia,
		&dto.SenhaHash, &dto.Provincia, &dto.Endereco,
		&dto.NumeroTelefone, &dto.Email, &dto.Website,
		&dto.NivelEscolar, &dto.Status, &cursosJSON,
		&dto.CreatedAt, &dto.UpdatedAt,
		&dto.TotalEstudantes, &dto.TotalInscricoesPendentes,
		&dto.Version,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(cursosJSON, &dto.Cursos); err != nil {
		dto.Cursos = []string{}
	}

	return &dto, nil
}

func (p *AcademiaProjection) GetByCodigoOrEmail(identifier string) (*AcademiaDTO, error) {
	query := `
		SELECT 
			id, type, nome, codigo_academia, senha_hash, provincia,
			endereco, numero_telefone, email, website, nivel_escolar,
			status, cursos, created_at, updated_at,
			total_estudantes, total_inscricoes_pendentes, version
		FROM projection_academias
		WHERE codigo_academia = $1 OR email = $1
		LIMIT 1
	`

	var dto AcademiaDTO
	var cursosJSON []byte

	err := p.client.DB().QueryRowContext(p.ctx, query, identifier).Scan(
		&dto.ID, &dto.Type, &dto.Nome, &dto.CodigoAcademia,
		&dto.SenhaHash, &dto.Provincia, &dto.Endereco,
		&dto.NumeroTelefone, &dto.Email, &dto.Website,
		&dto.NivelEscolar, &dto.Status, &cursosJSON,
		&dto.CreatedAt, &dto.UpdatedAt,
		&dto.TotalEstudantes, &dto.TotalInscricoesPendentes,
		&dto.Version,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(cursosJSON, &dto.Cursos); err != nil {
		dto.Cursos = []string{}
	}

	return &dto, nil
}

func (p *AcademiaProjection) GetByCodigo(codigo string) (*AcademiaDTO, error) {
	query := `
		SELECT 
			id, type, nome, codigo_academia, senha_hash, provincia,
			endereco, numero_telefone, email, website, nivel_escolar,
			status, cursos, created_at, updated_at,
			total_estudantes, total_inscricoes_pendentes, version
		FROM projection_academias
		WHERE codigo_academia = $1
		LIMIT 1
	`

	var dto AcademiaDTO
	var cursosJSON []byte

	err := p.client.DB().QueryRowContext(p.ctx, query, codigo).Scan(
		&dto.ID, &dto.Type, &dto.Nome, &dto.CodigoAcademia,
		&dto.SenhaHash, &dto.Provincia, &dto.Endereco,
		&dto.NumeroTelefone, &dto.Email, &dto.Website,
		&dto.NivelEscolar, &dto.Status, &cursosJSON,
		&dto.CreatedAt, &dto.UpdatedAt,
		&dto.TotalEstudantes, &dto.TotalInscricoesPendentes,
		&dto.Version,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(cursosJSON, &dto.Cursos); err != nil {
		dto.Cursos = []string{}
	}

	return &dto, nil
}

type AcademiaDTO struct {
	ID                       uuid.UUID  `json:"id"`
	Type                     string     `json:"type"`
	Nome                     string     `json:"nome"`
	CodigoAcademia           string     `json:"codigo_academia"`
	SenhaHash                string     `json:"-"`
	Provincia                string     `json:"provincia"`
	Endereco                 string     `json:"endereco"`
	NumeroTelefone           *string    `json:"numero_telefone,omitempty"`
	Email                    *string    `json:"email,omitempty"`
	Website                  *string    `json:"website,omitempty"`
	NivelEscolar             *string    `json:"nivel_escolar,omitempty"`
	Status                   string     `json:"status"`
	Cursos                   []string   `json:"cursos"`
	CreatedAt                time.Time  `json:"created_at"`
	UpdatedAt                time.Time  `json:"updated_at"`
	TotalEstudantes          int        `json:"total_estudantes"`
	TotalInscricoesPendentes int        `json:"total_inscricoes_pendentes"`
	Version                  int        `json:"version"`
}