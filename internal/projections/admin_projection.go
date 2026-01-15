// ============================================================================
// ARQUIVO: internal/projections/admin_projection.go
// 🔥 CORRIGIDO: TODAS as queries de leitura usando formato direto
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

	rows, err := p.client.DB().Query(query)
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

func (p *AdminProjection) GetLastProcessedEventID() (int64, error) {
	query := fmt.Sprintf(`
		SELECT last_processed_event_id 
		FROM projection_checkpoints 
		WHERE projection_name = '%s'
	`, p.Name())

	var lastID int64
	err := p.client.DB().QueryRow(query).Scan(&lastID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	return lastID, nil
}

func (p *AdminProjection) UpdateCheckpoint(eventID int64) error {
	query := fmt.Sprintf(`
		INSERT INTO projection_checkpoints (
			projection_name, 
			last_processed_event_id, 
			last_processed_at,
			events_processed
		) VALUES ('%s', %d, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) 
		DO UPDATE SET
			last_processed_event_id = %d,
			last_processed_at = CURRENT_TIMESTAMP,
			events_processed = projection_checkpoints.events_processed + 1
	`, p.Name(), eventID, eventID)

	_, err := p.client.DB().Exec(query)
	return err
}

func (p *AdminProjection) clear() error {
	_, err := p.client.DB().Exec(`TRUNCATE TABLE projection_admins CASCADE`)
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

	log.Printf("📋 [ADMIN] Nome: %s, Email: %s, Role: %s", payload.Nome, payload.Email, payload.Role)
	log.Printf("🔐 [ADMIN] SenhaHash (primeiros 30): %s...", payload.SenhaHash[:30])

	// 🔥 CORRIGIDO: Query direta
	createdByStr := "NULL"
	if payload.CreatedBy != nil {
		createdByStr = fmt.Sprintf("'%s'", payload.CreatedBy.String())
	}

	query := fmt.Sprintf(`
		INSERT INTO projection_admins (
			id, nome, email, senha_hash, role, status,
			created_by, created_at, updated_at, version,
			total_acoes_realizadas, last_event_id
		) VALUES (
			'%s', '%s', '%s', '%s', '%s', 'ativo',
			%s, '%s', '%s', %d, 0, '%s'
		)
		ON CONFLICT (id) DO UPDATE SET
			nome = EXCLUDED.nome,
			email = EXCLUDED.email,
			senha_hash = EXCLUDED.senha_hash,
			role = EXCLUDED.role,
			updated_at = EXCLUDED.updated_at,
			version = EXCLUDED.version,
			last_event_id = EXCLUDED.last_event_id
	`,
		event.AggregateID.String(),
		escapeStringAdmin(payload.Nome),
		escapeStringAdmin(payload.Email),
		payload.SenhaHash, // NÃO escapar hash bcrypt
		payload.Role,
		createdByStr,
		payload.CreatedAt.Format(time.RFC3339),
		time.Now().Format(time.RFC3339),
		event.EventVersion,
		event.EventID.String(),
	)

	result, err := p.client.DB().Exec(query)
	if err != nil {
		log.Printf("❌ [ADMIN] Erro ao salvar: %v", err)
		return err
	}

	rows, _ := result.RowsAffected()
	log.Printf("✅ [ADMIN] Salvo com sucesso! (rows: %d)", rows)

	return nil
}

func (p *AdminProjection) handleAdminAtivado(event genesisdb.Event) error {
	query := fmt.Sprintf(`
		UPDATE projection_admins
		SET 
			status = 'ativo',
			version = %d,
			updated_at = CURRENT_TIMESTAMP,
			last_event_id = '%s'
		WHERE id = '%s'
	`, event.EventVersion, event.EventID.String(), event.AggregateID.String())

	_, err := p.client.DB().Exec(query)
	return err
}

func (p *AdminProjection) handleAdminDesativado(event genesisdb.Event) error {
	query := fmt.Sprintf(`
		UPDATE projection_admins
		SET 
			status = 'inativo',
			version = %d,
			updated_at = CURRENT_TIMESTAMP,
			last_event_id = '%s'
		WHERE id = '%s'
	`, event.EventVersion, event.EventID.String(), event.AggregateID.String())

	_, err := p.client.DB().Exec(query)
	return err
}

func (p *AdminProjection) handleAcaoAdminRegistrada(event genesisdb.Event) error {
	query := fmt.Sprintf(`
		UPDATE projection_admins
		SET 
			total_acoes_realizadas = total_acoes_realizadas + 1,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = '%s'
	`, event.AggregateID.String())

	_, err := p.client.DB().Exec(query)
	return err
}

// Query methods - 🔥 TODAS CORRIGIDAS

func (p *AdminProjection) GetByID(id uuid.UUID) (*AdminDTO, error) {
	query := fmt.Sprintf(`
		SELECT 
			id, nome, email, senha_hash, role, status,
			created_by, created_at, updated_at,
			total_acoes_realizadas, version
		FROM projection_admins
		WHERE id = '%s'
	`, id.String())

	var dto AdminDTO
	err := p.client.DB().QueryRow(query).Scan(
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
	
	// 🔥 CORRIGIDO: Query direta
	query := fmt.Sprintf(`
		SELECT 
			id, nome, email, senha_hash, role, status,
			created_by, created_at, updated_at,
			total_acoes_realizadas, version
		FROM projection_admins
		WHERE email = '%s'
	`, email)

	var dto AdminDTO
	err := p.client.DB().QueryRow(query).Scan(
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
	query := `
		SELECT 
			id, nome, email, senha_hash, role, status,
			created_by, created_at, updated_at,
			total_acoes_realizadas, version
		FROM projection_admins
		ORDER BY created_at DESC
	`

	rows, err := p.client.DB().Query(query)
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

// Helper
func escapeStringAdmin(s string) string {
	result := ""
	for _, char := range s {
		if char == '\'' {
			result += "''"
		} else if char == '\\' {
			result += "\\\\"
		} else {
			result += string(char)
		}
	}
	return result
}