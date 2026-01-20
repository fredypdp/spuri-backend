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

type AdminProjection struct {
	client *db.Client
	ctx    context.Context
}

func NewAdminProjection(client *db.Client) *AdminProjection {
	return &AdminProjection{
		client: client,
		ctx:    context.Background(),
	}
}

func (p *AdminProjection) Name() string {
	return "admins"
}

func (p *AdminProjection) Handle(event db.Event) error {
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
	case "AdminDadosAtualizados":
		return p.handleAdminDadosAtualizados(event)
	case "AdminRoleAtualizado":
		return p.handleAdminRoleAtualizado(event)
	default:
		return nil
	}
}

func (p *AdminProjection) Rebuild() error {
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
	`, "Admin")
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

func (p *AdminProjection) GetLastProcessedEventID() (int64, error) {
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

func (p *AdminProjection) UpdateCheckpoint(eventID int64) error {
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

func (p *AdminProjection) clear() error {
	_, err := p.client.DB().Exec(`TRUNCATE TABLE projection_admins CASCADE`)
	return err
}

func (p *AdminProjection) handleAdminCriado(event db.Event) error {
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

	log.Printf("📋 [ADMIN] Nome: %s, Email: %s, Role: %s", payload.Nome, payload.Email, payload.Role)
	log.Printf("🔒 [ADMIN] SenhaHash (primeiros 30): %s...", payload.SenhaHash[:30])

	_, err := p.client.DB().Exec(`
		INSERT INTO projection_admins (
			id, nome, email, senha_hash, role, status,
			created_by, created_at, updated_at, version,
			total_acoes_realizadas, last_event_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (id) DO UPDATE SET
			nome = EXCLUDED.nome, email = EXCLUDED.email,
			senha_hash = EXCLUDED.senha_hash, role = EXCLUDED.role,
			updated_at = EXCLUDED.updated_at, version = EXCLUDED.version,
			last_event_id = EXCLUDED.last_event_id
	`, event.AggregateID, payload.Nome, payload.Email, payload.SenhaHash,
		payload.Role, "ativo", payload.CreatedBy, payload.CreatedAt, time.Now(),
		event.EventVersion, 0, event.EventID)

	if err != nil {
		log.Printf("❌ [ADMIN] Erro ao salvar: %v", err)
		return err
	}

	log.Printf("✅ [ADMIN] Salvo com sucesso!")
	return nil
}

func (p *AdminProjection) handleAdminAtivado(event db.Event) error {
	_, err := p.client.DB().Exec(`
		UPDATE projection_admins
		SET status = $1, version = $2, updated_at = CURRENT_TIMESTAMP, last_event_id = $3
		WHERE id = $4
	`, "ativo", event.EventVersion, event.EventID, event.AggregateID)
	return err
}

func (p *AdminProjection) handleAdminDesativado(event db.Event) error {
	_, err := p.client.DB().Exec(`
		UPDATE projection_admins
		SET status = $1, version = $2, updated_at = CURRENT_TIMESTAMP, last_event_id = $3
		WHERE id = $4
	`, "inativo", event.EventVersion, event.EventID, event.AggregateID)
	return err
}

func (p *AdminProjection) handleAcaoAdminRegistrada(event db.Event) error {
	_, err := p.client.DB().Exec(`
		UPDATE projection_admins
		SET total_acoes_realizadas = total_acoes_realizadas + 1, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`, event.AggregateID)
	return err
}

func (p *AdminProjection) handleAdminDadosAtualizados(event db.Event) error {
	var payload struct {
		Nome  *string `json:"Nome"`
		Email *string `json:"Email"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	if payload.Nome != nil {
		_, err := p.client.DB().Exec(`UPDATE projection_admins SET nome = $1 WHERE id = $2`, *payload.Nome, event.AggregateID)
		if err != nil {
			return err
		}
	}
	if payload.Email != nil {
		_, err := p.client.DB().Exec(`UPDATE projection_admins SET email = $1 WHERE id = $2`, *payload.Email, event.AggregateID)
		if err != nil {
			return err
		}
	}

	_, err := p.client.DB().Exec(`
		UPDATE projection_admins SET version = $1, updated_at = CURRENT_TIMESTAMP, last_event_id = $2 WHERE id = $3
	`, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

func (p *AdminProjection) handleAdminRoleAtualizado(event db.Event) error {
	var payload struct {
		NovoRole string `json:"NovoRole"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	_, err := p.client.DB().Exec(`
		UPDATE projection_admins
		SET role = $1, version = $2, updated_at = CURRENT_TIMESTAMP, last_event_id = $3
		WHERE id = $4
	`, payload.NovoRole, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

func (p *AdminProjection) GetByID(id uuid.UUID) (*AdminDTO, error) {
	var dto AdminDTO
	err := p.client.DB().QueryRow(`
		SELECT id, nome, email, senha_hash, role, status,
			created_by, created_at, updated_at, total_acoes_realizadas, version
		FROM projection_admins WHERE id = $1
	`, id).Scan(
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
	log.Printf("🔍 [ADMIN PROJECTION] GetByEmail: %s", email)

	var dto AdminDTO
	err := p.client.DB().QueryRow(`
		SELECT id, nome, email, senha_hash, role, status,
			created_by, created_at, updated_at, total_acoes_realizadas, version
		FROM projection_admins WHERE email = $1
	`, email).Scan(
		&dto.ID, &dto.Nome, &dto.Email, &dto.SenhaHash,
		&dto.Role, &dto.Status, &dto.CreatedBy,
		&dto.CreatedAt, &dto.UpdatedAt,
		&dto.TotalAcoesRealizadas, &dto.Version,
	)

	if err == sql.ErrNoRows {
		log.Printf("❌ [ADMIN PROJECTION] Não encontrado: %s", email)
		return nil, nil
	}
	if err != nil {
		log.Printf("❌ [ADMIN PROJECTION] Erro: %v", err)
		return nil, err
	}

	log.Printf("✅ [ADMIN PROJECTION] Encontrado: %s (Hash: %s...)", dto.Nome, dto.SenhaHash[:30])
	return &dto, nil
}

func (p *AdminProjection) GetAll() ([]AdminDTO, error) {
	rows, err := p.client.DB().Query(`
		SELECT id, nome, email, senha_hash, role, status,
			created_by, created_at, updated_at, total_acoes_realizadas, version
		FROM projection_admins ORDER BY created_at DESC
	`)
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