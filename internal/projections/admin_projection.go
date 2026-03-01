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

type AdminProjection struct {
	client *db.Client
}

func NewAdminProjection(client *db.Client) *AdminProjection {
	return &AdminProjection{client: client}
}

func (p *AdminProjection) Name() string { return "admins" }

func (p *AdminProjection) Handle(event db.Event) error {
	if event.AggregateType != "Admin" {
		return nil
	}

	handlers := map[string]func(db.Event) error{
		"AdminCriado":           p.handleAdminCriado,
		"AdminAtivado":          p.handleStatusChange("ativo"),
		"AdminDesativado":       p.handleStatusChange("inativo"),
		"AcaoAdminRegistrada":   p.handleAcaoAdminRegistrada,
		"AdminDadosAtualizados": p.handleAdminDadosAtualizados,
		"AdminRoleAtualizado":   p.handleAdminRoleAtualizado,
		"EmailVerificado":       p.handleEmailVerificado,
	}

	if handler, ok := handlers[event.EventType]; ok {
		log.Printf("[DEBUG] Processando %s para admin %s", event.EventType, event.AggregateID)
		return handler(event)
	}
	return nil
}

func (p *AdminProjection) Rebuild() error {
	log.Printf("[DEBUG] Rebuild iniciado")
	
	if err := p.clear(); err != nil {
		return fmt.Errorf("falha ao limpar: %w", err)
	}

	rows, err := p.client.DB().Query(`
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE aggregate_type = 'Admin'
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

func (p *AdminProjection) GetLastProcessedEventID() (int64, error) {
	var lastID int64
	query := fmt.Sprintf(`SELECT last_processed_event_id FROM projection_checkpoints WHERE projection_name = '%s'`,
		db.SafeString(p.Name()))
	
	err := p.client.DB().QueryRow(query).Scan(&lastID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return lastID, err
}

func (p *AdminProjection) UpdateCheckpoint(eventID int64) error {
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

func (p *AdminProjection) clear() error {
	_, err := p.client.DB().Exec(`TRUNCATE TABLE projection_admins CASCADE`)
	return err
}

func (p *AdminProjection) handleAdminCriado(event db.Event) error {
	var payload struct {
		Nome, Email, SenhaHash, Role string
		CreatedBy                    *uuid.UUID
		CreatedAt                    time.Time
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error: %w", err)
	}

	if event.AggregateID == uuid.Nil {
		return fmt.Errorf("UUID inválido")
	}

	query := fmt.Sprintf(`
		INSERT INTO projection_admins (
			id, nome, email, senha_hash, role, status, email_verificado,
			created_by, created_at, updated_at, version, total_acoes_realizadas, last_event_id
		) VALUES ('%s', '%s', '%s', '%s', '%s', 'ativo', FALSE, %s, '%s', CURRENT_TIMESTAMP, %d, 0, '%s')
		ON CONFLICT (id) DO UPDATE SET
			nome = EXCLUDED.nome, email = EXCLUDED.email, senha_hash = EXCLUDED.senha_hash,
			role = EXCLUDED.role, updated_at = EXCLUDED.updated_at, version = EXCLUDED.version,
			last_event_id = EXCLUDED.last_event_id
	`, event.AggregateID, db.SafeString(payload.Nome), db.SafeString(payload.Email),
		db.SafeString(payload.SenhaHash), db.SafeString(payload.Role),
		nullOrUUID(payload.CreatedBy), payload.CreatedAt.Format(time.RFC3339),
		event.EventVersion, event.EventID)

	_, err := p.client.DB().Exec(query)
	return err
}

func (p *AdminProjection) handleStatusChange(status string) func(db.Event) error {
	return func(event db.Event) error {
		if event.AggregateID == uuid.Nil {
			return fmt.Errorf("UUID inválido")
		}

		query := fmt.Sprintf(`
			UPDATE projection_admins
			SET status = '%s', version = %d, updated_at = CURRENT_TIMESTAMP, last_event_id = '%s'
			WHERE id = '%s'
		`, status, event.EventVersion, event.EventID, event.AggregateID)
		
		_, err := p.client.DB().Exec(query)
		return err
	}
}

func (p *AdminProjection) handleAcaoAdminRegistrada(event db.Event) error {
	query := fmt.Sprintf(`
		UPDATE projection_admins
		SET total_acoes_realizadas = total_acoes_realizadas + 1, updated_at = CURRENT_TIMESTAMP
		WHERE id = '%s'
	`, event.AggregateID)
	
	_, err := p.client.DB().Exec(query)
	return err
}

func (p *AdminProjection) handleAdminDadosAtualizados(event db.Event) error {
	var payload struct {
		Nome, Email   *string
		EmailAlterado bool
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}

	if payload.Nome != nil {
		query := fmt.Sprintf(`UPDATE projection_admins SET nome = '%s' WHERE id = '%s'`,
			db.SafeString(*payload.Nome), event.AggregateID)
		p.client.DB().Exec(query)
	}
	
	if payload.Email != nil {
		emailVerif := "email_verificado"
		if payload.EmailAlterado {
			emailVerif = "FALSE"
		}
		query := fmt.Sprintf(`UPDATE projection_admins SET email = '%s', email_verificado = %s WHERE id = '%s'`,
			db.SafeString(*payload.Email), emailVerif, event.AggregateID)
		p.client.DB().Exec(query)
	}

	query := fmt.Sprintf(`
		UPDATE projection_admins
		SET version = %d, updated_at = CURRENT_TIMESTAMP, last_event_id = '%s'
		WHERE id = '%s'
	`, event.EventVersion, event.EventID, event.AggregateID)
	
	_, err := p.client.DB().Exec(query)
	return err
}

func (p *AdminProjection) handleAdminRoleAtualizado(event db.Event) error {
	var payload struct{ NovoRole string }

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}

	query := fmt.Sprintf(`
		UPDATE projection_admins
		SET role = '%s', version = %d, updated_at = CURRENT_TIMESTAMP, last_event_id = '%s'
		WHERE id = '%s'
	`, db.SafeString(payload.NovoRole), event.EventVersion, event.EventID, event.AggregateID)
	
	_, err := p.client.DB().Exec(query)
	return err
}

func (p *AdminProjection) handleEmailVerificado(event db.Event) error {
	query := fmt.Sprintf(`
		UPDATE projection_admins
		SET email_verificado = TRUE, updated_at = CURRENT_TIMESTAMP
		WHERE id = '%s'
	`, event.AggregateID)
	
	_, err := p.client.DB().Exec(query)
	return err
}

func (p *AdminProjection) GetByID(id uuid.UUID) (*AdminDTO, error) {
	if id == uuid.Nil {
		return nil, fmt.Errorf("UUID inválido")
	}
	return p.queryAdmin(fmt.Sprintf("id = '%s'", id))
}

func (p *AdminProjection) GetByEmail(email string) (*AdminDTO, error) {
	return p.queryAdmin(fmt.Sprintf("email = '%s'", db.SafeString(email)))
}

// GetByEmailForLogin usa prepared statement — mais seguro para autenticação
func (p *AdminProjection) GetByEmailForLogin(email string) (*AdminDTO, error) {
	query := `
		SELECT id, nome, email, senha_hash, role, status, email_verificado,
			created_by, created_at, updated_at, total_acoes_realizadas, version
		FROM projection_admins
		WHERE email = $1
		LIMIT 1`

	var dto AdminDTO
	var createdBy sql.NullString

	err := p.client.DB().QueryRow(query, email).Scan(
		&dto.ID, &dto.Nome, &dto.Email, &dto.SenhaHash, &dto.Role, &dto.Status,
		&dto.EmailVerificado, &createdBy, &dto.CreatedAt, &dto.UpdatedAt,
		&dto.TotalAcoesRealizadas, &dto.Version,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar admin por email: %w", err)
	}

	if createdBy.Valid {
		uid, _ := uuid.Parse(createdBy.String)
		dto.CreatedBy = &uid
	}

	return &dto, nil
}

func (p *AdminProjection) queryAdmin(whereClause string) (*AdminDTO, error) {
	query := fmt.Sprintf(`
		SELECT id, nome, email, senha_hash, role, status, email_verificado,
			created_by, created_at, updated_at, total_acoes_realizadas, version
		FROM projection_admins WHERE %s LIMIT 1`, whereClause)
	
	var dto AdminDTO
	var createdBy sql.NullString
	
	err := p.client.DB().QueryRow(query).Scan(
		&dto.ID, &dto.Nome, &dto.Email, &dto.SenhaHash, &dto.Role, &dto.Status,
		&dto.EmailVerificado, &createdBy, &dto.CreatedAt, &dto.UpdatedAt,
		&dto.TotalAcoesRealizadas, &dto.Version,
	)
	
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	
	if createdBy.Valid {
		uid, _ := uuid.Parse(createdBy.String)
		dto.CreatedBy = &uid
	}
	
	return &dto, nil
}

func (p *AdminProjection) GetAll() ([]AdminDTO, error) {
	rows, err := p.client.DB().Query(`
		SELECT id, nome, email, senha_hash, role, status, email_verificado,
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
		var createdBy sql.NullString
		
		if err := rows.Scan(&dto.ID, &dto.Nome, &dto.Email, &dto.SenhaHash, &dto.Role,
			&dto.Status, &dto.EmailVerificado, &createdBy, &dto.CreatedAt,
			&dto.UpdatedAt, &dto.TotalAcoesRealizadas, &dto.Version); err != nil {
			continue
		}
		
		if createdBy.Valid {
			uid, _ := uuid.Parse(createdBy.String)
			dto.CreatedBy = &uid
		}
		dtos = append(dtos, dto)
	}

	return dtos, rows.Err()
}

type AdminDTO struct {
	ID                   uuid.UUID  `json:"id"`
	Nome                 string     `json:"nome"`
	Email                string     `json:"email"`
	SenhaHash            string     `json:"-"`
	Role                 string     `json:"role"`
	Status               string     `json:"status"`
	EmailVerificado      bool       `json:"email_verificado"`
	CreatedBy            *uuid.UUID `json:"created_by,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
	TotalAcoesRealizadas int        `json:"total_acoes_realizadas"`
	Version              int        `json:"version"`
}