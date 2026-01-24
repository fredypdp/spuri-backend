package aggregates

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
)

type Admin struct {
	BaseAggregate
	
	Nome      string
	Email     string
	SenhaHash string
	Status    string
	Role      string
	CreatedAt time.Time
	CreatedBy *uuid.UUID
	
	TotalAcoesRealizadas int
}

func NewAdmin() *Admin {
	log.Printf("[ADMIN_AGGREGATE] Criando nova instância de Admin")
	return &Admin{
		BaseAggregate: BaseAggregate{
			ID:                uuid.New(),
			Version:           0,
			UncommittedEvents: []DomainEvent{},
		},
	}
}

func (a *Admin) GetType() string {
	return "Admin"
}

func (a *Admin) Apply(event DomainEvent) error {
	log.Printf("[ADMIN_AGGREGATE] Aplicando evento: type=%s, aggregate_id=%s", 
		event.GetEventType(), event.GetAggregateID())
	
	switch event.GetEventType() {
	case "AdminCriado":
		return a.applyAdminCriado(event)
	case "AdminAtivado":
		return a.applyAdminAtivado(event)
	case "AdminDesativado":
		return a.applyAdminDesativado(event)
	case "AcaoAdminRegistrada":
		return a.applyAcaoAdminRegistrada(event)
	case "AdminDadosAtualizados":
		return a.applyAdminDadosAtualizados(event)
	case "AdminRoleAtualizado":
		return a.applyAdminRoleAtualizado(event)
	default:
		log.Printf("[ADMIN_AGGREGATE] Tipo de evento desconhecido: %s", event.GetEventType())
		return fmt.Errorf("tipo de evento desconhecido: %s", event.GetEventType())
	}
}

func (a *Admin) Criar(
	nome string,
	email string,
	senhaHash string,
	role string,
	createdBy *uuid.UUID,
) error {
	log.Printf("[ADMIN_AGGREGATE] Executando comando Criar: nome=%s, email=%s, role=%s", 
		nome, email, role)
	
	if nome == "" {
		log.Printf("[ADMIN_AGGREGATE] Nome obrigatório não fornecido")
		return fmt.Errorf("nome é obrigatório")
	}
	if email == "" {
		log.Printf("[ADMIN_AGGREGATE] Email obrigatório não fornecido")
		return fmt.Errorf("email é obrigatório")
	}
	if senhaHash == "" {
		log.Printf("[ADMIN_AGGREGATE] Senha obrigatória não fornecida")
		return fmt.Errorf("senha é obrigatória")
	}
	
	validRoles := map[string]bool{
		"fpp":     true,
		"adm":     true,
		"gerente": true,
	}
	if !validRoles[role] {
		log.Printf("[ADMIN_AGGREGATE] Role inválido: %s", role)
		return fmt.Errorf("role deve ser 'fpp', 'adm' ou 'gerente'")
	}

	event := &AdminCriadoEvent{
		BaseEvent: BaseEvent{
			EventType:   "AdminCriado",
			AggregateID: a.ID,
		},
		Nome:      nome,
		Email:     email,
		SenhaHash: senhaHash,
		Role:      role,
		CreatedBy: createdBy,
		CreatedAt: time.Now(),
	}

	log.Printf("[ADMIN_AGGREGATE] Evento AdminCriado gerado para admin: %s", a.ID)
	a.RaiseEvent(event)
	return a.Apply(event)
}

func (a *Admin) Ativar(adminID uuid.UUID) error {
	log.Printf("[ADMIN_AGGREGATE] Executando comando Ativar: id=%s, status_atual=%s, ativado_por=%s", 
		a.ID, a.Status, adminID)
	
	if a.Status == "ativo" {
		log.Printf("[ADMIN_AGGREGATE] Admin já está ativo")
		return fmt.Errorf("administrador já está ativo")
	}

	event := &AdminAtivadoEvent{
		BaseEvent: BaseEvent{
			EventType:   "AdminAtivado",
			AggregateID: a.ID,
		},
		ActivatedBy: adminID,
		ActivatedAt: time.Now(),
	}

	log.Printf("[ADMIN_AGGREGATE] Evento AdminAtivado gerado")
	a.RaiseEvent(event)
	return a.Apply(event)
}

func (a *Admin) Desativar(adminID uuid.UUID, motivo string) error {
	log.Printf("[ADMIN_AGGREGATE] Executando comando Desativar: id=%s, motivo=%s, desativado_por=%s", 
		a.ID, motivo, adminID)
	
	if a.Status == "inativo" {
		log.Printf("[ADMIN_AGGREGATE] Admin já está inativo")
		return fmt.Errorf("administrador já está inativo")
	}

	event := &AdminDesativadoEvent{
		BaseEvent: BaseEvent{
			EventType:   "AdminDesativado",
			AggregateID: a.ID,
		},
		DeactivatedBy: adminID,
		Motivo:        motivo,
		DeactivatedAt: time.Now(),
	}

	log.Printf("[ADMIN_AGGREGATE] Evento AdminDesativado gerado")
	a.RaiseEvent(event)
	return a.Apply(event)
}

func (a *Admin) RegistrarAcao(acao string, detalhes map[string]interface{}) error {
	log.Printf("[ADMIN_AGGREGATE] Executando comando RegistrarAcao: id=%s, acao=%s", a.ID, acao)
	
	if a.Status != "ativo" {
		log.Printf("[ADMIN_AGGREGATE] Admin inativo, não pode registrar ações: status=%s", a.Status)
		return fmt.Errorf("administrador está inativo")
	}

	event := &AcaoAdminRegistradaEvent{
		BaseEvent: BaseEvent{
			EventType:   "AcaoAdminRegistrada",
			AggregateID: a.ID,
		},
		Acao:        acao,
		Detalhes:    detalhes,
		PerformedAt: time.Now(),
	}

	log.Printf("[ADMIN_AGGREGATE] Evento AcaoAdminRegistrada gerado")
	a.RaiseEvent(event)
	return a.Apply(event)
}

func (a *Admin) AtualizarDados(
	nome *string,
	email *string,
	updatedBy uuid.UUID,
) error {
	log.Printf("[ADMIN_AGGREGATE] Executando comando AtualizarDados: id=%s, updated_by=%s", 
		a.ID, updatedBy)
	
	if a.Status != "ativo" {
		log.Printf("[ADMIN_AGGREGATE] Admin inativo: status=%s", a.Status)
		return fmt.Errorf("administrador está inativo")
	}

	if nome == nil && email == nil {
		log.Printf("[ADMIN_AGGREGATE] Nenhum campo fornecido para atualização")
		return fmt.Errorf("nenhum campo para atualizar")
	}

	if nome != nil && *nome == "" {
		log.Printf("[ADMIN_AGGREGATE] Nome vazio fornecido")
		return fmt.Errorf("nome não pode ser vazio")
	}

	if email != nil && *email == "" {
		log.Printf("[ADMIN_AGGREGATE] Email vazio fornecido")
		return fmt.Errorf("email não pode ser vazio")
	}

	event := &AdminDadosAtualizadosEvent{
		BaseEvent: BaseEvent{
			EventType:   "AdminDadosAtualizados",
			AggregateID: a.ID,
		},
		Nome:      nome,
		Email:     email,
		UpdatedBy: updatedBy,
		UpdatedAt: time.Now(),
	}

	log.Printf("[ADMIN_AGGREGATE] Evento AdminDadosAtualizados gerado")
	a.RaiseEvent(event)
	return a.Apply(event)
}

func (a *Admin) AtualizarRole(novoRole string, updatedBy uuid.UUID, updatedByRole string) error {
	log.Printf("[ADMIN_AGGREGATE] Executando comando AtualizarRole: id=%s, role_atual=%s, novo_role=%s, updated_by_role=%s", 
		a.ID, a.Role, novoRole, updatedByRole)
	
	if a.Status != "ativo" {
		log.Printf("[ADMIN_AGGREGATE] Admin inativo: status=%s", a.Status)
		return fmt.Errorf("administrador está inativo")
	}

	if updatedByRole != "fpp" {
		log.Printf("[ADMIN_AGGREGATE] Apenas FPP pode alterar roles. Role atual: %s", updatedByRole)
		return fmt.Errorf("apenas FPP pode alterar roles")
	}

	validRoles := map[string]bool{
		"fpp":     true,
		"adm":     true,
		"gerente": true,
	}
	if !validRoles[novoRole] {
		log.Printf("[ADMIN_AGGREGATE] Role inválido: %s", novoRole)
		return fmt.Errorf("role deve ser 'fpp', 'adm' ou 'gerente'")
	}

	if a.Role == novoRole {
		log.Printf("[ADMIN_AGGREGATE] Admin já possui este role: %s", novoRole)
		return fmt.Errorf("admin já possui este role")
	}

	event := &AdminRoleAtualizadoEvent{
		BaseEvent: BaseEvent{
			EventType:   "AdminRoleAtualizado",
			AggregateID: a.ID,
		},
		RoleAnterior: a.Role,
		NovoRole:     novoRole,
		UpdatedBy:    updatedBy,
		UpdatedAt:    time.Now(),
	}

	log.Printf("[ADMIN_AGGREGATE] Evento AdminRoleAtualizado gerado: %s -> %s", a.Role, novoRole)
	a.RaiseEvent(event)
	return a.Apply(event)
}

func (a *Admin) ValidatePermission(targetRole string) error {
	log.Printf("[ADMIN_AGGREGATE] Validando permissão: admin_role=%s, target_role=%s", a.Role, targetRole)
	
	if a.Status != "ativo" {
		log.Printf("[ADMIN_AGGREGATE] Admin inativo")
		return fmt.Errorf("administrador está inativo")
	}

	hierarchy := map[string]int{
		"fpp":     3,
		"adm":     2,
		"gerente": 1,
	}

	currentLevel := hierarchy[a.Role]
	targetLevel := hierarchy[targetRole]

	if currentLevel <= targetLevel {
		log.Printf("[ADMIN_AGGREGATE] Permissão negada: level_atual=%d, level_target=%d", 
			currentLevel, targetLevel)
		return fmt.Errorf("permissão negada: role '%s' não pode gerenciar '%s'", a.Role, targetRole)
	}

	log.Printf("[ADMIN_AGGREGATE] Permissão concedida")
	return nil
}

// Event Handlers

func (a *Admin) applyAdminCriado(event DomainEvent) error {
	log.Printf("[ADMIN_AGGREGATE] Aplicando AdminCriado")
	
	payload := event.GetPayload()
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[ADMIN_AGGREGATE] Erro ao serializar payload: %v", err)
		return err
	}

	var ev AdminCriadoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		log.Printf("[ADMIN_AGGREGATE] Erro ao deserializar evento: %v", err)
		return err
	}

	a.ID = event.GetAggregateID()
	a.Nome = ev.Nome
	a.Email = ev.Email
	a.SenhaHash = ev.SenhaHash
	a.Role = ev.Role
	a.Status = "ativo"
	a.CreatedBy = ev.CreatedBy
	a.CreatedAt = ev.CreatedAt

	log.Printf("[ADMIN_AGGREGATE] Estado atualizado: nome=%s, email=%s, role=%s, status=%s", 
		a.Nome, a.Email, a.Role, a.Status)
	return nil
}

func (a *Admin) applyAdminAtivado(event DomainEvent) error {
	log.Printf("[ADMIN_AGGREGATE] Aplicando AdminAtivado")
	a.Status = "ativo"
	log.Printf("[ADMIN_AGGREGATE] Status atualizado: %s", a.Status)
	return nil
}

func (a *Admin) applyAdminDesativado(event DomainEvent) error {
	log.Printf("[ADMIN_AGGREGATE] Aplicando AdminDesativado")
	a.Status = "inativo"
	log.Printf("[ADMIN_AGGREGATE] Status atualizado: %s", a.Status)
	return nil
}

func (a *Admin) applyAcaoAdminRegistrada(event DomainEvent) error {
	log.Printf("[ADMIN_AGGREGATE] Aplicando AcaoAdminRegistrada")
	a.TotalAcoesRealizadas++
	log.Printf("[ADMIN_AGGREGATE] Total ações realizadas: %d", a.TotalAcoesRealizadas)
	return nil
}

func (a *Admin) applyAdminDadosAtualizados(event DomainEvent) error {
	log.Printf("[ADMIN_AGGREGATE] Aplicando AdminDadosAtualizados")
	
	payload := event.GetPayload()
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[ADMIN_AGGREGATE] Erro ao serializar payload: %v", err)
		return err
	}

	var ev AdminDadosAtualizadosEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		log.Printf("[ADMIN_AGGREGATE] Erro ao deserializar evento: %v", err)
		return err
	}

	if ev.Nome != nil {
		a.Nome = *ev.Nome
		log.Printf("[ADMIN_AGGREGATE] Nome atualizado: %s", a.Nome)
	}
	if ev.Email != nil {
		a.Email = *ev.Email
		log.Printf("[ADMIN_AGGREGATE] Email atualizado: %s", a.Email)
	}

	return nil
}

func (a *Admin) applyAdminRoleAtualizado(event DomainEvent) error {
	log.Printf("[ADMIN_AGGREGATE] Aplicando AdminRoleAtualizado")
	
	payload := event.GetPayload()
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[ADMIN_AGGREGATE] Erro ao serializar payload: %v", err)
		return err
	}

	var ev AdminRoleAtualizadoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		log.Printf("[ADMIN_AGGREGATE] Erro ao deserializar evento: %v", err)
		return err
	}

	log.Printf("[ADMIN_AGGREGATE] Role atualizado: %s -> %s", ev.RoleAnterior, ev.NovoRole)
	a.Role = ev.NovoRole

	return nil
}

// Eventos

type AdminCriadoEvent struct {
	BaseEvent
	Nome      string
	Email     string
	SenhaHash string
	Role      string
	CreatedBy *uuid.UUID
	CreatedAt time.Time
}

func (e *AdminCriadoEvent) GetPayload() interface{} {
	return e
}

type AdminAtivadoEvent struct {
	BaseEvent
	ActivatedBy uuid.UUID
	ActivatedAt time.Time
}

func (e *AdminAtivadoEvent) GetPayload() interface{} {
	return e
}

type AdminDesativadoEvent struct {
	BaseEvent
	DeactivatedBy uuid.UUID
	Motivo        string
	DeactivatedAt time.Time
}

func (e *AdminDesativadoEvent) GetPayload() interface{} {
	return e
}

type AcaoAdminRegistradaEvent struct {
	BaseEvent
	Acao        string
	Detalhes    map[string]interface{}
	PerformedAt time.Time
}

func (e *AcaoAdminRegistradaEvent) GetPayload() interface{} {
	return e
}

type AdminDadosAtualizadosEvent struct {
	BaseEvent
	Nome      *string
	Email     *string
	UpdatedBy uuid.UUID
	UpdatedAt time.Time
}

func (e *AdminDadosAtualizadosEvent) GetPayload() interface{} {
	return e
}

type AdminRoleAtualizadoEvent struct {
	BaseEvent
	RoleAnterior string
	NovoRole     string
	UpdatedBy    uuid.UUID
	UpdatedAt    time.Time
}

func (e *AdminRoleAtualizadoEvent) GetPayload() interface{} {
	return e
}