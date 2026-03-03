// ============================================================================
// ARQUIVO: internal/projections/admin_projection.go
//
// CORREÇÕES APLICADAS:
//   [A31] — handleAdminDadosAtualizados: verifica unicidade de email na projeção
//            antes de executar UPDATE. Evita que evento com email duplicado
//            quebre o rebuild silenciosamente.
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

type AdminProjection struct {
	client *db.Client
}

func NewAdminProjection(client *db.Client) *AdminProjection {
	return &AdminProjection{client: client}
}

func (p *AdminProjection) Name() string { return "admins" }

// ============================================================================
// Interface Projection
// ============================================================================

func (p *AdminProjection) GetLastProcessedEventID() (int64, error) {
	var lastID int64
	err := p.client.DB().QueryRow(
		`SELECT last_processed_event_id FROM projection_checkpoints WHERE projection_name = $1`,
		p.Name(),
	).Scan(&lastID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return lastID, err
}

func (p *AdminProjection) UpdateCheckpoint(eventID int64) error {
	eventID = int64(db.ValidateOffset(int(eventID)))
	_, err := p.client.DB().Exec(`
		INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at, events_processed)
		VALUES ($1, $2, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) DO UPDATE SET
			last_processed_event_id = $2,
			last_processed_at = CURRENT_TIMESTAMP,
			events_processed = projection_checkpoints.events_processed + 1
	`, p.Name(), eventID)
	return err
}

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
		"AdminSenhaAlterada":    p.handleAdminSenhaAlterada,
	}
	if handler, ok := handlers[event.EventType]; ok {
		return handler(event)
	}
	return nil
}

// Rebuild reconstrói a projeção a partir do ledger.
//
// ⚠️  ATENÇÃO: O TRUNCATE remove todos os admins temporariamente.
// Durante o rebuild, requisições que dependam de projection_admins (login,
// middleware RequireAdminRole) podem falhar com 403/404.
// Execute preferencialmente em janela de manutenção.
func (p *AdminProjection) Rebuild() error {
	log.Printf("[DEBUG] [admins] Rebuild iniciado — atenção: causa indisponibilidade temporária")
	if _, err := p.client.DB().Exec(`TRUNCATE TABLE projection_admins CASCADE`); err != nil {
		return fmt.Errorf("falha ao limpar projeção: %w", err)
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
		var prevHash sql.NullString
		if err := rows.Scan(
			&event.ID, &event.EventID, &event.AggregateID, &event.AggregateType,
			&event.EventType, &event.EventVersion, &event.Payload, &event.Metadata,
			&event.OccurredAt, &event.RecordedAt, &event.LedgerHash, &prevHash,
		); err != nil {
			return err
		}
		if prevHash.Valid {
			event.PreviousHash = &prevHash.String
		}
		if err := p.Handle(event); err != nil {
			return fmt.Errorf("erro no evento %d: %w", event.ID, err)
		}
		count++
	}
	log.Printf("[DEBUG] [admins] Rebuild concluído: %d eventos", count)
	return rows.Err()
}

// ============================================================================
// Handlers de evento
// ============================================================================

func (p *AdminProjection) handleAdminCriado(event db.Event) error {
	var payload struct {
		Nome      string
		Email     string
		SenhaHash string
		Role      string
		CreatedBy *uuid.UUID
		CreatedAt time.Time
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}

	var createdBy interface{}
	if payload.CreatedBy != nil {
		createdBy = payload.CreatedBy.String()
	}

	_, err := p.client.DB().Exec(`
		INSERT INTO projection_admins
			(id, nome, email, senha_hash, role, status, email_verificado,
			 created_by, created_at, updated_at, version, last_event_id)
		VALUES ($1, $2, $3, $4, $5, 'ativo', FALSE, $6, $7, CURRENT_TIMESTAMP, $8, $9)
		ON CONFLICT (id) DO NOTHING
	`,
		event.AggregateID, payload.Nome, payload.Email, payload.SenhaHash, payload.Role,
		createdBy, payload.CreatedAt, event.EventVersion, event.EventID,
	)
	return err
}

func (p *AdminProjection) handleStatusChange(status string) func(db.Event) error {
	return func(event db.Event) error {
		if event.AggregateID == uuid.Nil {
			return fmt.Errorf("UUID inválido no evento de mudança de status")
		}
		_, err := p.client.DB().Exec(`
			UPDATE projection_admins
			SET status = $1, version = $2, updated_at = CURRENT_TIMESTAMP, last_event_id = $3
			WHERE id = $4
		`, status, event.EventVersion, event.EventID, event.AggregateID)
		return err
	}
}

func (p *AdminProjection) handleAcaoAdminRegistrada(event db.Event) error {
	_, err := p.client.DB().Exec(`
		UPDATE projection_admins
		SET total_acoes_realizadas = total_acoes_realizadas + 1,
		    version                = $1,
		    last_event_id          = $2,
		    updated_at             = CURRENT_TIMESTAMP
		WHERE id = $3
	`, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

// handleAdminDadosAtualizados aplica atualização de nome/email na projeção.
//
// [A31] CORRIGIDO: antes do UPDATE de email, verifica se o novo email já
// pertence a outro admin. Se sim, retorna erro para impedir inconsistência
// ledger ↔ projeção. O evento já está gravado no ledger (imutável), mas ao
// menos o rebuild não quebrará silenciosamente: o erro será logado e o evento
// ficará marcado como falha permanente.
func (p *AdminProjection) handleAdminDadosAtualizados(event db.Event) error {
	var payload struct {
		Nome          *string
		Email         *string
		EmailAlterado bool
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}

	tx, err := p.client.DB().Begin()
	if err != nil {
		return fmt.Errorf("erro ao iniciar transação: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if payload.Nome != nil {
		if _, err := tx.Exec(
			`UPDATE projection_admins SET nome = $1 WHERE id = $2`,
			*payload.Nome, event.AggregateID,
		); err != nil {
			return fmt.Errorf("erro ao atualizar nome: %w", err)
		}
	}

	if payload.Email != nil {
		// [A31] Verificar unicidade antes de aplicar o UPDATE
		var conflictID string
		err := tx.QueryRow(
			`SELECT id FROM projection_admins WHERE email = $1 AND id != $2`,
			*payload.Email, event.AggregateID,
		).Scan(&conflictID)
		if err == nil {
			// Outro admin já possui este email
			return fmt.Errorf(
				"handleAdminDadosAtualizados: email '%s' já pertence ao admin %s — evento %d gravado no ledger mas não aplicável na projeção",
				*payload.Email, conflictID, event.ID,
			)
		}
		if err != sql.ErrNoRows {
			return fmt.Errorf("erro ao verificar unicidade de email: %w", err)
		}

		if _, err := tx.Exec(
			`UPDATE projection_admins SET email = $1 WHERE id = $2`,
			*payload.Email, event.AggregateID,
		); err != nil {
			return fmt.Errorf("erro ao atualizar email: %w", err)
		}
		if payload.EmailAlterado {
			if _, err := tx.Exec(
				`UPDATE projection_admins SET email_verificado = FALSE WHERE id = $1`,
				event.AggregateID,
			); err != nil {
				return fmt.Errorf("erro ao resetar email_verificado: %w", err)
			}
		}
	}

	if _, err := tx.Exec(`
		UPDATE projection_admins
		SET version = $1, updated_at = CURRENT_TIMESTAMP, last_event_id = $2
		WHERE id = $3
	`, event.EventVersion, event.EventID, event.AggregateID); err != nil {
		return fmt.Errorf("erro ao atualizar version: %w", err)
	}

	return tx.Commit()
}

func (p *AdminProjection) handleAdminRoleAtualizado(event db.Event) error {
	var payload struct{ NovoRole string }
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}
	_, err := p.client.DB().Exec(`
		UPDATE projection_admins
		SET role = $1, version = $2, updated_at = CURRENT_TIMESTAMP, last_event_id = $3
		WHERE id = $4
	`, payload.NovoRole, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

func (p *AdminProjection) handleEmailVerificado(event db.Event) error {
	_, err := p.client.DB().Exec(`
		UPDATE projection_admins
		SET email_verificado = TRUE, version = $1, updated_at = CURRENT_TIMESTAMP, last_event_id = $2
		WHERE id = $3
	`, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

// handleAdminSenhaAlterada aplica a nova senha_hash na projeção.
// Garante que rebuild restaura a senha correta.
func (p *AdminProjection) handleAdminSenhaAlterada(event db.Event) error {
	var payload struct {
		NovaSenhaHash string
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}

	if payload.NovaSenhaHash == "" {
		return fmt.Errorf("AdminSenhaAlterada: NovaSenhaHash vazio no payload")
	}

	_, err := p.client.DB().Exec(`
		UPDATE projection_admins
		SET senha_hash = $1, version = $2, updated_at = CURRENT_TIMESTAMP, last_event_id = $3
		WHERE id = $4
	`, payload.NovaSenhaHash, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

// ============================================================================
// Queries de leitura
// ============================================================================

// AdminDTO representa os dados de leitura de um administrador.
// SenhaHash tem tag json:"-" — nunca serializado em respostas HTTP.
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
	Version              int        `json:"version"`
	TotalAcoesRealizadas int        `json:"total_acoes_realizadas"`
}

func (p *AdminProjection) GetByID(id uuid.UUID) (*AdminDTO, error) {
	if id == uuid.Nil {
		return nil, fmt.Errorf("UUID inválido")
	}
	row := p.client.DB().QueryRow(`
		SELECT id, nome, email, senha_hash, role, status, email_verificado,
			created_by, created_at, updated_at, version, total_acoes_realizadas
		FROM projection_admins WHERE id = $1
	`, id)
	return scanAdmin(row)
}

func (p *AdminProjection) GetByEmail(email string) (*AdminDTO, error) {
	row := p.client.DB().QueryRow(`
		SELECT id, nome, email, senha_hash, role, status, email_verificado,
			created_by, created_at, updated_at, version, total_acoes_realizadas
		FROM projection_admins WHERE email = $1
	`, email)
	return scanAdmin(row)
}

// GetByEmailForLogin é um alias de GetByEmail — usado no fluxo de autenticação.
func (p *AdminProjection) GetByEmailForLogin(email string) (*AdminDTO, error) {
	return p.GetByEmail(email)
}

func (p *AdminProjection) GetAll() ([]AdminDTO, error) {
	rows, err := p.client.DB().Query(`
		SELECT id, nome, email, senha_hash, role, status, email_verificado,
			created_by, created_at, updated_at, version, total_acoes_realizadas
		FROM projection_admins
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []AdminDTO
	for rows.Next() {
		var dto AdminDTO
		var createdBy sql.NullString
		if err := rows.Scan(
			&dto.ID, &dto.Nome, &dto.Email, &dto.SenhaHash, &dto.Role, &dto.Status,
			&dto.EmailVerificado, &createdBy, &dto.CreatedAt, &dto.UpdatedAt,
			&dto.Version, &dto.TotalAcoesRealizadas,
		); err != nil {
			continue
		}
		if createdBy.Valid {
			cid, _ := uuid.Parse(createdBy.String)
			dto.CreatedBy = &cid
		}
		result = append(result, dto)
	}
	return result, rows.Err()
}

func scanAdmin(row *sql.Row) (*AdminDTO, error) {
	var dto AdminDTO
	var createdBy sql.NullString
	err := row.Scan(
		&dto.ID, &dto.Nome, &dto.Email, &dto.SenhaHash, &dto.Role, &dto.Status,
		&dto.EmailVerificado, &createdBy, &dto.CreatedAt, &dto.UpdatedAt,
		&dto.Version, &dto.TotalAcoesRealizadas,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if createdBy.Valid {
		cid, _ := uuid.Parse(createdBy.String)
		dto.CreatedBy = &cid
	}
	return &dto, nil
}