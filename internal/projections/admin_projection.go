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

func (p *AdminProjection) Name() string {
	return "admins"
}

func (p *AdminProjection) Handle(event db.Event) error {
	log.Printf("[ADMIN_PROJECTION] Recebendo evento: type=%s, aggregate_id=%s, event_id=%s", 
		event.EventType, event.AggregateID, event.EventID)
	
	if event.AggregateType != "Admin" {
		log.Printf("[ADMIN_PROJECTION] Ignorando evento de tipo %s", event.AggregateType)
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
		log.Printf("[ADMIN_PROJECTION] Tipo de evento desconhecido: %s", event.EventType)
		return nil
	}
}

func (p *AdminProjection) Rebuild() error {
	log.Printf("[ADMIN_PROJECTION] Iniciando rebuild da projeção")
	
	if err := p.clear(); err != nil {
		log.Printf("[ADMIN_PROJECTION] Erro ao limpar projeção: %v", err)
		return err
	}

	query := `
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE aggregate_type = 'Admin'
		ORDER BY id ASC
	`
	
	log.Printf("[ADMIN_PROJECTION] Executando query de rebuild")
	rows, err := p.client.DB().Query(query)
	if err != nil {
		log.Printf("[ADMIN_PROJECTION] Erro ao executar query de rebuild: %v", err)
		return err
	}
	defer rows.Close()

	eventCount := 0
	for rows.Next() {
		var event db.Event
		err := rows.Scan(
			&event.ID, &event.EventID, &event.AggregateID, &event.AggregateType,
			&event.EventType, &event.EventVersion, &event.Payload, &event.Metadata,
			&event.OccurredAt, &event.RecordedAt, &event.LedgerHash, &event.PreviousHash,
		)
		if err != nil {
			log.Printf("[ADMIN_PROJECTION] Erro ao fazer scan do evento: %v", err)
			return err
		}

		if err := p.Handle(event); err != nil {
			log.Printf("[ADMIN_PROJECTION] Erro ao processar evento %d: %v", event.ID, err)
			return fmt.Errorf("erro ao processar evento %d: %w", event.ID, err)
		}
		eventCount++
	}

	log.Printf("[ADMIN_PROJECTION] Rebuild concluído. %d eventos processados", eventCount)
	return rows.Err()
}

func (p *AdminProjection) GetLastProcessedEventID() (int64, error) {
	safeName := db.SafeString(p.Name())
	
	query := fmt.Sprintf(`
		SELECT last_processed_event_id 
		FROM projection_checkpoints 
		WHERE projection_name = '%s'
	`, safeName)
	
	log.Printf("[ADMIN_PROJECTION] Buscando último evento processado: %s", query)
	
	var lastID int64
	err := p.client.DB().QueryRow(query).Scan(&lastID)
	
	if err == sql.ErrNoRows {
		log.Printf("[ADMIN_PROJECTION] Nenhum checkpoint encontrado, retornando 0")
		return 0, nil
	}
	
	if err != nil {
		log.Printf("[ADMIN_PROJECTION] Erro ao buscar checkpoint: %v", err)
	} else {
		log.Printf("[ADMIN_PROJECTION] Último evento processado: %d", lastID)
	}
	
	return lastID, err
}

func (p *AdminProjection) UpdateCheckpoint(eventID int64) error {
	safeName := db.SafeString(p.Name())
	eventID = int64(db.ValidateOffset(int(eventID)))
	
	query := fmt.Sprintf(`
		INSERT INTO projection_checkpoints (
			projection_name, last_processed_event_id, last_processed_at, events_processed
		) VALUES ('%s', %d, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) 
		DO UPDATE SET
			last_processed_event_id = %d,
			last_processed_at = CURRENT_TIMESTAMP,
			events_processed = projection_checkpoints.events_processed + 1
	`, safeName, eventID, eventID)
	
	log.Printf("[ADMIN_PROJECTION] Atualizando checkpoint para event_id=%d", eventID)
	
	_, err := p.client.DB().Exec(query)
	if err != nil {
		log.Printf("[ADMIN_PROJECTION] Erro ao atualizar checkpoint: %v", err)
	}
	return err
}

func (p *AdminProjection) clear() error {
	log.Printf("[ADMIN_PROJECTION] Limpando tabela projection_admins")
	_, err := p.client.DB().Exec(`TRUNCATE TABLE projection_admins CASCADE`)
	if err != nil {
		log.Printf("[ADMIN_PROJECTION] Erro ao limpar tabela: %v", err)
	}
	return err
}

func (p *AdminProjection) handleAdminCriado(event db.Event) error {
	log.Printf("[ADMIN_PROJECTION] Processando AdminCriado: event_id=%s", event.EventID)
	
	var payload struct {
		Nome      string     `json:"Nome"`
		Email     string     `json:"Email"`
		SenhaHash string     `json:"SenhaHash"`
		Role      string     `json:"Role"`
		CreatedBy *uuid.UUID `json:"CreatedBy"`
		CreatedAt time.Time  `json:"CreatedAt"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		log.Printf("[ADMIN_PROJECTION] Erro ao parsear payload: %v", err)
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	log.Printf("[ADMIN_PROJECTION] Dados do admin: nome=%s, email=%s, role=%s", 
		payload.Nome, payload.Email, payload.Role)

	aggID := event.AggregateID
	if aggID == uuid.Nil {
		log.Printf("[ADMIN_PROJECTION] UUID inválido")
		return fmt.Errorf("UUID inválido")
	}

	safeNome := db.SafeString(payload.Nome)
	safeEmail := db.SafeString(payload.Email)
	safeHash := db.SafeString(payload.SenhaHash)
	safeRole := db.SafeString(payload.Role)

	var createdByStr string
	if payload.CreatedBy != nil {
		createdByStr = fmt.Sprintf("'%s'", *payload.CreatedBy)
		log.Printf("[ADMIN_PROJECTION] Admin criado por: %s", *payload.CreatedBy)
	} else {
		createdByStr = "NULL"
		log.Printf("[ADMIN_PROJECTION] Admin criado sem referência de criador")
	}

	query := fmt.Sprintf(`
		INSERT INTO projection_admins (
			id, nome, email, senha_hash, role, status,
			created_by, created_at, updated_at, version,
			total_acoes_realizadas, last_event_id
		) VALUES ('%s', '%s', '%s', '%s', '%s', 'ativo', %s, '%s', CURRENT_TIMESTAMP, %d, 0, '%s')
		ON CONFLICT (id) DO UPDATE SET
			nome = EXCLUDED.nome, email = EXCLUDED.email,
			senha_hash = EXCLUDED.senha_hash, role = EXCLUDED.role,
			updated_at = EXCLUDED.updated_at, version = EXCLUDED.version,
			last_event_id = EXCLUDED.last_event_id
	`, aggID, safeNome, safeEmail, safeHash, safeRole, createdByStr,
		payload.CreatedAt.Format(time.RFC3339), event.EventVersion, event.EventID)

	log.Printf("[ADMIN_PROJECTION] Executando insert/update: %s", query)

	_, err := p.client.DB().Exec(query)
	if err != nil {
		log.Printf("[ADMIN_PROJECTION] Erro ao processar AdminCriado (event_id: %s): %v", event.EventID, err)
		return err
	}

	log.Printf("[ADMIN_PROJECTION] AdminCriado processado com sucesso: id=%s", aggID)
	return nil
}

func (p *AdminProjection) handleAdminAtivado(event db.Event) error {
	log.Printf("[ADMIN_PROJECTION] Processando AdminAtivado: event_id=%s", event.EventID)
	
	aggID := event.AggregateID
	if aggID == uuid.Nil {
		log.Printf("[ADMIN_PROJECTION] UUID inválido")
		return fmt.Errorf("UUID inválido")
	}

	query := fmt.Sprintf(`
		UPDATE projection_admins
		SET status = 'ativo', version = %d, updated_at = CURRENT_TIMESTAMP, last_event_id = '%s'
		WHERE id = '%s'
	`, event.EventVersion, event.EventID, aggID)
	
	log.Printf("[ADMIN_PROJECTION] Executando update: %s", query)
	
	_, err := p.client.DB().Exec(query)
	if err != nil {
		log.Printf("[ADMIN_PROJECTION] Erro ao ativar admin %s: %v", aggID, err)
	} else {
		log.Printf("[ADMIN_PROJECTION] Admin %s ativado com sucesso", aggID)
	}
	return err
}

func (p *AdminProjection) handleAdminDesativado(event db.Event) error {
	log.Printf("[ADMIN_PROJECTION] Processando AdminDesativado: event_id=%s", event.EventID)
	
	aggID := event.AggregateID
	if aggID == uuid.Nil {
		log.Printf("[ADMIN_PROJECTION] UUID inválido")
		return fmt.Errorf("UUID inválido")
	}

	query := fmt.Sprintf(`
		UPDATE projection_admins
		SET status = 'inativo', version = %d, updated_at = CURRENT_TIMESTAMP, last_event_id = '%s'
		WHERE id = '%s'
	`, event.EventVersion, event.EventID, aggID)
	
	log.Printf("[ADMIN_PROJECTION] Executando update: %s", query)
	
	_, err := p.client.DB().Exec(query)
	if err != nil {
		log.Printf("[ADMIN_PROJECTION] Erro ao desativar admin %s: %v", aggID, err)
	} else {
		log.Printf("[ADMIN_PROJECTION] Admin %s desativado com sucesso", aggID)
	}
	return err
}

func (p *AdminProjection) handleAcaoAdminRegistrada(event db.Event) error {
	log.Printf("[ADMIN_PROJECTION] Processando AcaoAdminRegistrada: event_id=%s", event.EventID)
	
	aggID := event.AggregateID
	if aggID == uuid.Nil {
		log.Printf("[ADMIN_PROJECTION] UUID inválido")
		return fmt.Errorf("UUID inválido")
	}

	query := fmt.Sprintf(`
		UPDATE projection_admins
		SET total_acoes_realizadas = total_acoes_realizadas + 1, updated_at = CURRENT_TIMESTAMP
		WHERE id = '%s'
	`, aggID)
	
	log.Printf("[ADMIN_PROJECTION] Incrementando contador de ações para admin %s", aggID)
	
	_, err := p.client.DB().Exec(query)
	if err != nil {
		log.Printf("[ADMIN_PROJECTION] Erro ao registrar ação: %v", err)
	}
	return err
}

func (p *AdminProjection) handleAdminDadosAtualizados(event db.Event) error {
	log.Printf("[ADMIN_PROJECTION] Processando AdminDadosAtualizados: event_id=%s", event.EventID)
	
	var payload struct {
		Nome  *string `json:"Nome"`
		Email *string `json:"Email"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		log.Printf("[ADMIN_PROJECTION] Erro ao parsear payload: %v", err)
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	aggID := event.AggregateID
	if aggID == uuid.Nil {
		log.Printf("[ADMIN_PROJECTION] UUID inválido")
		return fmt.Errorf("UUID inválido")
	}

	if payload.Nome != nil {
		safe := db.SafeString(*payload.Nome)
		query := fmt.Sprintf(`UPDATE projection_admins SET nome = '%s' WHERE id = '%s'`, safe, aggID)
		log.Printf("[ADMIN_PROJECTION] Atualizando nome: %s", query)
		p.client.DB().Exec(query)
	}
	
	if payload.Email != nil {
		safe := db.SafeString(*payload.Email)
		query := fmt.Sprintf(`UPDATE projection_admins SET email = '%s' WHERE id = '%s'`, safe, aggID)
		log.Printf("[ADMIN_PROJECTION] Atualizando email: %s", query)
		p.client.DB().Exec(query)
	}

	query := fmt.Sprintf(`
		UPDATE projection_admins SET version = %d, updated_at = CURRENT_TIMESTAMP, last_event_id = '%s' WHERE id = '%s'
	`, event.EventVersion, event.EventID, aggID)
	
	log.Printf("[ADMIN_PROJECTION] Atualizando version e timestamp: %s", query)
	
	_, err := p.client.DB().Exec(query)
	if err != nil {
		log.Printf("[ADMIN_PROJECTION] Erro ao atualizar dados: %v", err)
	} else {
		log.Printf("[ADMIN_PROJECTION] Dados do admin %s atualizados com sucesso", aggID)
	}
	return err
}

func (p *AdminProjection) handleAdminRoleAtualizado(event db.Event) error {
	log.Printf("[ADMIN_PROJECTION] Processando AdminRoleAtualizado: event_id=%s", event.EventID)
	
	var payload struct {
		NovoRole string `json:"NovoRole"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		log.Printf("[ADMIN_PROJECTION] Erro ao parsear payload: %v", err)
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	log.Printf("[ADMIN_PROJECTION] Novo role: %s", payload.NovoRole)

	aggID := event.AggregateID
	if aggID == uuid.Nil {
		log.Printf("[ADMIN_PROJECTION] UUID inválido")
		return fmt.Errorf("UUID inválido")
	}

	safeRole := db.SafeString(payload.NovoRole)

	query := fmt.Sprintf(`
		UPDATE projection_admins
		SET role = '%s', version = %d, updated_at = CURRENT_TIMESTAMP, last_event_id = '%s'
		WHERE id = '%s'
	`, safeRole, event.EventVersion, event.EventID, aggID)
	
	log.Printf("[ADMIN_PROJECTION] Executando update: %s", query)
	
	_, err := p.client.DB().Exec(query)
	if err != nil {
		log.Printf("[ADMIN_PROJECTION] Erro ao atualizar role: %v", err)
	} else {
		log.Printf("[ADMIN_PROJECTION] Role atualizado com sucesso para admin %s", aggID)
	}
	return err
}

func (p *AdminProjection) GetByID(id uuid.UUID) (*AdminDTO, error) {
	log.Printf("[ADMIN_PROJECTION] Buscando admin por ID: %s", id)
	
	if id == uuid.Nil {
		log.Printf("[ADMIN_PROJECTION] UUID inválido fornecido")
		return nil, fmt.Errorf("UUID inválido")
	}

	query := fmt.Sprintf(`
		SELECT id, nome, email, senha_hash, role, status,
			created_by, created_at, updated_at, total_acoes_realizadas, version
		FROM projection_admins WHERE id = '%s'`, id)
	
	log.Printf("[ADMIN_PROJECTION] Executando query: %s", query)
	
	var dto AdminDTO
	var createdBy sql.NullString
	
	err := p.client.DB().QueryRow(query).Scan(
		&dto.ID, &dto.Nome, &dto.Email, &dto.SenhaHash, &dto.Role, &dto.Status,
		&createdBy, &dto.CreatedAt, &dto.UpdatedAt, &dto.TotalAcoesRealizadas, &dto.Version,
	)
	
	if err == sql.ErrNoRows {
		log.Printf("[ADMIN_PROJECTION] Admin não encontrado: %s", id)
		return nil, nil
	}
	if err != nil {
		log.Printf("[ADMIN_PROJECTION] Erro ao buscar admin: %v", err)
		return nil, err
	}
	
	if createdBy.Valid {
		uid, _ := uuid.Parse(createdBy.String)
		dto.CreatedBy = &uid
	}
	
	log.Printf("[ADMIN_PROJECTION] Admin encontrado: %s (%s)", dto.Nome, dto.Email)
	return &dto, nil
}

func (p *AdminProjection) GetByEmail(email string) (*AdminDTO, error) {
	log.Printf("[ADMIN_PROJECTION] Buscando admin por email: %s", email)
	
	safeEmail := db.SafeString(email)

	query := fmt.Sprintf(`
		SELECT id, nome, email, senha_hash, role, status,
			created_by, created_at, updated_at, total_acoes_realizadas, version
		FROM projection_admins WHERE email = '%s'`, safeEmail)
	
	log.Printf("[ADMIN_PROJECTION] Executando query: %s", query)
	
	var dto AdminDTO
	var createdBy sql.NullString
	
	err := p.client.DB().QueryRow(query).Scan(
		&dto.ID, &dto.Nome, &dto.Email, &dto.SenhaHash, &dto.Role, &dto.Status,
		&createdBy, &dto.CreatedAt, &dto.UpdatedAt, &dto.TotalAcoesRealizadas, &dto.Version,
	)

	if err == sql.ErrNoRows {
		log.Printf("[ADMIN_PROJECTION] Admin não encontrado com email: %s", email)
		return nil, nil
	}
	if err != nil {
		log.Printf("[ADMIN_PROJECTION] Erro ao buscar admin por email: %v", err)
		return nil, err
	}
	
	if createdBy.Valid {
		uid, _ := uuid.Parse(createdBy.String)
		dto.CreatedBy = &uid
	}

	log.Printf("[ADMIN_PROJECTION] Admin encontrado: %s (%s)", dto.Nome, dto.ID)
	return &dto, nil
}

func (p *AdminProjection) GetAll() ([]AdminDTO, error) {
	log.Printf("[ADMIN_PROJECTION] Buscando todos os admins")
	
	query := `
		SELECT id, nome, email, senha_hash, role, status,
			created_by, created_at, updated_at, total_acoes_realizadas, version
		FROM projection_admins ORDER BY created_at DESC
	`
	
	rows, err := p.client.DB().Query(query)
	if err != nil {
		log.Printf("[ADMIN_PROJECTION] Erro ao buscar admins: %v", err)
		return nil, err
	}
	defer rows.Close()

	var dtos []AdminDTO
	count := 0
	for rows.Next() {
		var dto AdminDTO
		var createdBy sql.NullString
		err := rows.Scan(
			&dto.ID, &dto.Nome, &dto.Email, &dto.SenhaHash, &dto.Role, &dto.Status,
			&createdBy, &dto.CreatedAt, &dto.UpdatedAt, &dto.TotalAcoesRealizadas, &dto.Version,
		)
		if err != nil {
			log.Printf("[ADMIN_PROJECTION] Erro ao fazer scan do admin: %v", err)
			continue
		}
		if createdBy.Valid {
			uid, _ := uuid.Parse(createdBy.String)
			dto.CreatedBy = &uid
		}
		dtos = append(dtos, dto)
		count++
	}

	log.Printf("[ADMIN_PROJECTION] %d admins encontrados", count)
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