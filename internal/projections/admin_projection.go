// ============================================================================
// ARQUIVO: internal/projections/admin_projection.go
// ProjeÃ§Ã£o de leitura para administradores
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

// AdminProjection projeÃ§Ã£o de administradores
type AdminProjection struct {
	client *genesisdb.Client
	ctx    context.Context
}

// NewAdminProjection cria nova projeÃ§Ã£o de admin
func NewAdminProjection(client *genesisdb.Client) *AdminProjection {
	return &AdminProjection{
		client: client,
		ctx:    context.Background(),
	}
}

// Name implementa Projection
func (p *AdminProjection) Name() string {
	return "admins"
}

// Handle processa um evento
func (p *AdminProjection) Handle(event genesisdb.Event) error {
	if event.AggregateType != "Admin" {
		return nil
	}

	switch event.EventType {
	case "AdminCriado":
		return p.handleAdminCriado(event)
	case "AdminAtivado":
		return p.handleAdminAtivado(event)
	case "AdminDesativado":
		return p.handleAdminDesativado(event)
	case "AcaoAdminRegistrada":
		return p.handleAcaoAdminRegistrada(event)
	default:
		return nil
	}
}

// Rebuild reconstrÃ³i a projeÃ§Ã£o do zero
func (p *AdminProjection) Rebuild() error {
	if err := p.clear(); err != nil {
		return err
	}

	query := `
		SELECT 
			id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM genesis_ledger
		WHERE aggregate_type = 'Admin'
		ORDER BY id ASC
	`

	var events []genesisdb.Event
	if err := p.client.DB().Select(&events, query); err != nil {
		return err
	}

	for _, event := range events {
		if err := p.Handle(event); err != nil {
			return fmt.Errorf("erro ao processar evento %d: %w", event.ID, err)
		}
	}

	return nil
}

// GetLastProcessedEventID implementa Projection
func (p *AdminProjection) GetLastProcessedEventID() (int64, error) {
	query := `
		SELECT last_processed_event_id 
		FROM projection_checkpoints 
		WHERE projection_name = $1
	`

	var lastID int64
	err := p.client.DB().GetContext(p.ctx, &lastID, query, p.Name())
	if err != nil {
		return 0, err
	}

	return lastID, nil
}

// UpdateCheckpoint implementa Projection
func (p *AdminProjection) UpdateCheckpoint(eventID int64) error {
	query := `
		UPDATE projection_checkpoints
		SET 
			last_processed_event_id = $1,
			last_processed_at = CURRENT_TIMESTAMP,
			events_processed = events_processed + 1
		WHERE projection_name = $2
	`

	_, err := p.client.DB().ExecContext(p.ctx, query, eventID, p.Name())
	return err
}

// clear limpa a projeÃ§Ã£o
func (p *AdminProjection) clear() error {
	query := `TRUNCATE TABLE projection_admins CASCADE`
	_, err := p.client.DB().ExecContext(p.ctx, query)
	return err
}

// Event Handlers

func (p *AdminProjection) handleAdminCriado(event genesisdb.Event) error {
	log.Printf("ðŸ”µ [PROJEÃ‡ÃƒO ADMIN] Processando AdminCriado")
	
	var payload struct {
		Nome      string     `json:"Nome"`
		Email     string     `json:"Email"`
		SenhaHash string     `json:"SenhaHash"`
		Role      string     `json:"Role"`
		CreatedBy *uuid.UUID `json:"CreatedBy"`
		CreatedAt time.Time  `json:"CreatedAt"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	query := `
		INSERT INTO projection_admins (
			id, nome, email, senha_hash, role, status,
			created_by, created_at, updated_at, version,
			total_acoes_realizadas, last_event_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (id) DO UPDATE SET
			nome = EXCLUDED.nome,
			email = EXCLUDED.email,
			senha_hash = EXCLUDED.senha_hash,
			role = EXCLUDED.role,
			updated_at = EXCLUDED.updated_at,
			version = EXCLUDED.version,
			last_event_id = EXCLUDED.last_event_id
	`

	_, err := p.client.DB().ExecContext(
		p.ctx, query,
		event.AggregateID,
		payload.Nome,
		payload.Email,
		payload.SenhaHash,
		payload.Role,
		"ativo",
		payload.CreatedBy,
		payload.CreatedAt,
		time.Now(),
		event.EventVersion,
		0,
		event.EventID,
	)

	return err
}

func (p *AdminProjection) handleAdminAtivado(event genesisdb.Event) error {
	query := `
		UPDATE projection_admins
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

func (p *AdminProjection) handleAdminDesativado(event genesisdb.Event) error {
	query := `
		UPDATE projection_admins
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

func (p *AdminProjection) handleAcaoAdminRegistrada(event genesisdb.Event) error {
	query := `
		UPDATE projection_admins
		SET 
			total_acoes_realizadas = total_acoes_realizadas + 1,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`

	_, err := p.client.DB().ExecContext(p.ctx, query, event.AggregateID)
	return err
}

// Query methods

// GetByID busca admin por ID
func (p *AdminProjection) GetByID(id uuid.UUID) (*AdminDTO, error) {
	query := `
		SELECT 
			id, nome, email, senha_hash, role, status,
			created_by, created_at, updated_at,
			total_acoes_realizadas, version
		FROM projection_admins
		WHERE id = $1
	`

	var dto AdminDTO
	err := p.client.DB().GetContext(p.ctx, &dto, query, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &dto, nil
}

// GetByEmail busca admin por email
func (p *AdminProjection) GetByEmail(email string) (*AdminDTO, error) {
	query := `
		SELECT 
			id, nome, email, senha_hash, role, status,
			created_by, created_at, updated_at,
			total_acoes_realizadas, version
		FROM projection_admins
		WHERE email = $1
	`

	var dto AdminDTO
	err := p.client.DB().GetContext(p.ctx, &dto, query, email)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &dto, nil
}

// GetAll lista todos os administradores
func (p *AdminProjection) GetAll() ([]AdminDTO, error) {
	query := `
		SELECT 
			id, nome, email, senha_hash, role, status,
			created_by, created_at, updated_at,
			total_acoes_realizadas, version
		FROM projection_admins
		ORDER BY created_at DESC
	`

	var dtos []AdminDTO
	err := p.client.DB().SelectContext(p.ctx, &dtos, query)
	return dtos, err
}

// AdminDTO DTO da projeÃ§Ã£o
type AdminDTO struct {
	ID                   uuid.UUID  `db:"id" json:"id"`
	Nome                 string     `db:"nome" json:"nome"`
	Email                string     `db:"email" json:"email"`
	SenhaHash            string     `db:"senha_hash" json:"-"`
	Role                 string     `db:"role" json:"role"`
	Status               string     `db:"status" json:"status"`
	CreatedBy            *uuid.UUID `db:"created_by" json:"created_by,omitempty"`
	CreatedAt            time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt            time.Time  `db:"updated_at" json:"updated_at"`
	TotalAcoesRealizadas int        `db:"total_acoes_realizadas" json:"total_acoes_realizadas"`
	Version              int        `db:"version" json:"version"`
}