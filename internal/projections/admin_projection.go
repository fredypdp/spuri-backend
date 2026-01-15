// ============================================================================
// ARQUIVO 5: internal/projections/admin_projection.go
// 🔥 CORRIGIDO: Todas as queries usando QueryRowContext
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

type AdminProjection struct {
	client *genesisdb.Client
	ctx    context.Context
}

func NewAdminProjection(client *genesisdb.Client) *AdminProjection {
	return &AdminProjection{
		client: client,
		ctx:    context.Background(),
	}
}

func (p *AdminProjection) Name() string {
	return "admins"
}

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

// 🔥 CORRIGIDO
func (p *AdminProjection) GetLastProcessedEventID() (int64, error) {
	query := `
		SELECT last_processed_event_id 
		FROM projection_checkpoints 
		WHERE projection_name = $1
	`

	var lastID int64
	err := p.client.DB().QueryRowContext(p.ctx, query, p.Name()).Scan(&lastID)
	if err != nil {
		return 0, err
	}

	return lastID, nil
}

// 🔥 CORRIGIDO
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

func (p *AdminProjection) clear() error {
	_, err := p.client.DB().ExecContext(p.ctx, `TRUNCATE TABLE projection_admins CASCADE`)
	return err
}

func (p *AdminProjection) handleAdminCriado(event genesisdb.Event) error {
	log.Printf("🔵 [PROJEÇÃO ADMIN] Processando AdminCriado")
	
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
		event.AggregateID, payload.Nome, payload.Email,
		payload.SenhaHash, payload.Role, "ativo",
		payload.CreatedBy, payload.CreatedAt, time.Now(),
		event.EventVersion, 0, event.EventID,
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
	err := p.client.DB().QueryRowContext(p.ctx, query, id).Scan(
		&dto.ID, &dto.Nome, &dto.Email, &dto.SenhaHash,
		&dto.Role, &dto.Status, &dto.CreatedBy,
		&dto.CreatedAt, &dto.UpdatedAt,
		&dto.TotalAcoesRealizadas, &dto.Version,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &dto, nil
}

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
	err := p.client.DB().QueryRowContext(p.ctx, query, email).Scan(
		&dto.ID, &dto.Nome, &dto.Email, &dto.SenhaHash,
		&dto.Role, &dto.Status, &dto.CreatedBy,
		&dto.CreatedAt, &dto.UpdatedAt,
		&dto.TotalAcoesRealizadas, &dto.Version,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &dto, nil
}

func (p *AdminProjection) GetAll() ([]AdminDTO, error) {
	query := `
		SELECT 
			id, nome, email, senha_hash, role, status,
			created_by, created_at, updated_at,
			total_acoes_realizadas, version
		FROM projection_admins
		ORDER BY created_at DESC
	`

	rows, err := p.client.DB().QueryContext(p.ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dtos []AdminDTO
	for rows.Next() {
		var dto AdminDTO
		err := rows.Scan(
			&dto.ID, &dto.Nome, &dto.Email, &dto.SenhaHash,
			&dto.Role, &dto.Status, &dto.CreatedBy,
			&dto.CreatedAt, &dto.UpdatedAt,
			&dto.TotalAcoesRealizadas, &dto.Version,
		)
		if err != nil {
			return nil, err
		}
		dtos = append(dtos, dto)
	}

	return dtos, rows.Err()
}

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