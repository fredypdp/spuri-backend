package aggregates

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Admin struct {
	BaseAggregate
	
	Nome            string
	Email           string
	SenhaHash       string
	Status          string
	Role            string
	EmailVerificado bool
	CreatedAt       time.Time
	CreatedBy       *uuid.UUID
	
	TotalAcoesRealizadas int
}

func NewAdmin() *Admin {
	return &Admin{
		BaseAggregate: BaseAggregate{
			ID:                uuid.New(),
			Version:           0,
			UncommittedEvents: []DomainEvent{},
		},
		EmailVerificado: false,
	}
}

func (a *Admin) GetType() string {
	return "Admin"
}

func (a *Admin) Apply(event DomainEvent) error {
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
	case "EmailVerificado":
		return a.applyEmailVerificado(event)
	default:
		return fmt.Errorf("tipo de evento desconhecido: %s", event.GetEventType())
	}
}

func (a *Admin) Criar(nome string, email string, senhaHash string, role string, createdBy *uuid.UUID) error {
	if nome == "" || email == "" || senhaHash == "" {
		return fmt.Errorf("campos obrigatórios vazios")
	}
	
	validRoles := map[string]bool{"fpp": true, "adm": true, "gerente": true}
	if !validRoles[role] {
		return fmt.Errorf("role deve ser 'fpp', 'adm' ou 'gerente'")
	}

	event := &AdminCriadoEvent{
		BaseEvent:   BaseEvent{EventType: "AdminCriado", AggregateID: a.ID},
		Nome:        nome,
		Email:       email,
		SenhaHash:   senhaHash,
		Role:        role,
		CreatedBy:   createdBy,
		CreatedAt:   time.Now(),
	}

	a.RaiseEvent(event)
	return a.Apply(event)
}

func (a *Admin) VerificarEmail() error {
	if a.EmailVerificado {
		return fmt.Errorf("email já verificado")
	}

	event := &EmailVerificadoEvent{
		BaseEvent:   BaseEvent{EventType: "EmailVerificado", AggregateID: a.ID},
		VerifiedAt:  time.Now(),
	}

	a.RaiseEvent(event)
	return a.Apply(event)
}

func (a *Admin) Ativar(adminID uuid.UUID) error {
	if a.Status == "ativo" {
		return fmt.Errorf("administrador já está ativo")
	}

	event := &AdminAtivadoEvent{
		BaseEvent:   BaseEvent{EventType: "AdminAtivado", AggregateID: a.ID},
		ActivatedBy: adminID,
		ActivatedAt: time.Now(),
	}

	a.RaiseEvent(event)
	return a.Apply(event)
}

func (a *Admin) Desativar(adminID uuid.UUID, motivo string) error {
	if a.Status == "inativo" {
		return fmt.Errorf("administrador já está inativo")
	}

	event := &AdminDesativadoEvent{
		BaseEvent:     BaseEvent{EventType: "AdminDesativado", AggregateID: a.ID},
		DeactivatedBy: adminID,
		Motivo:        motivo,
		DeactivatedAt: time.Now(),
	}

	a.RaiseEvent(event)
	return a.Apply(event)
}

func (a *Admin) RegistrarAcao(acao string, detalhes map[string]interface{}) error {
	if a.Status != "ativo" {
		return fmt.Errorf("administrador está inativo")
	}

	event := &AcaoAdminRegistradaEvent{
		BaseEvent:   BaseEvent{EventType: "AcaoAdminRegistrada", AggregateID: a.ID},
		Acao:        acao,
		Detalhes:    detalhes,
		PerformedAt: time.Now(),
	}

	a.RaiseEvent(event)
	return a.Apply(event)
}

func (a *Admin) AtualizarDados(nome *string, email *string, updatedBy uuid.UUID) error {
	if a.Status != "ativo" {
		return fmt.Errorf("administrador está inativo")
	}

	if nome == nil && email == nil {
		return fmt.Errorf("nenhum campo para atualizar")
	}

	emailAlterado := email != nil && a.Email != *email

	event := &AdminDadosAtualizadosEvent{
		BaseEvent:     BaseEvent{EventType: "AdminDadosAtualizados", AggregateID: a.ID},
		Nome:          nome,
		Email:         email,
		EmailAlterado: emailAlterado,
		UpdatedBy:     updatedBy,
		UpdatedAt:     time.Now(),
	}

	a.RaiseEvent(event)
	return a.Apply(event)
}

func (a *Admin) AtualizarRole(novoRole string, updatedBy uuid.UUID, updatedByRole string) error {
	if a.Status != "ativo" {
		return fmt.Errorf("administrador está inativo")
	}

	if updatedByRole != "fpp" {
		return fmt.Errorf("apenas FPP pode alterar roles")
	}

	validRoles := map[string]bool{"fpp": true, "adm": true, "gerente": true}
	if !validRoles[novoRole] {
		return fmt.Errorf("role deve ser 'fpp', 'adm' ou 'gerente'")
	}

	if a.Role == novoRole {
		return fmt.Errorf("admin já possui este role")
	}

	event := &AdminRoleAtualizadoEvent{
		BaseEvent:    BaseEvent{EventType: "AdminRoleAtualizado", AggregateID: a.ID},
		RoleAnterior: a.Role,
		NovoRole:     novoRole,
		UpdatedBy:    updatedBy,
		UpdatedAt:    time.Now(),
	}

	a.RaiseEvent(event)
	return a.Apply(event)
}

func (a *Admin) ValidatePermission(targetRole string) error {
	if a.Status != "ativo" {
		return fmt.Errorf("administrador está inativo")
	}

	hierarchy := map[string]int{"fpp": 3, "adm": 2, "gerente": 1}
	if hierarchy[a.Role] <= hierarchy[targetRole] {
		return fmt.Errorf("permissão negada: role '%s' não pode gerenciar '%s'", a.Role, targetRole)
	}

	return nil
}

// Event Handlers
func (a *Admin) applyAdminCriado(event DomainEvent) error {
	payload := event.GetPayload()
	data, _ := json.Marshal(payload)
	var ev AdminCriadoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	a.ID = event.GetAggregateID()
	a.Nome = ev.Nome
	a.Email = ev.Email
	a.SenhaHash = ev.SenhaHash
	a.Role = ev.Role
	a.Status = "ativo"
	a.EmailVerificado = false
	a.CreatedBy = ev.CreatedBy
	a.CreatedAt = ev.CreatedAt
	return nil
}

func (a *Admin) applyEmailVerificado(event DomainEvent) error {
	a.EmailVerificado = true
	return nil
}

func (a *Admin) applyAdminAtivado(event DomainEvent) error {
	a.Status = "ativo"
	return nil
}

func (a *Admin) applyAdminDesativado(event DomainEvent) error {
	a.Status = "inativo"
	return nil
}

func (a *Admin) applyAcaoAdminRegistrada(event DomainEvent) error {
	a.TotalAcoesRealizadas++
	return nil
}

func (a *Admin) applyAdminDadosAtualizados(event DomainEvent) error {
	payload := event.GetPayload()
	data, _ := json.Marshal(payload)
	var ev AdminDadosAtualizadosEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	if ev.Nome != nil {
		a.Nome = *ev.Nome
	}
	if ev.Email != nil {
		a.Email = *ev.Email
		if ev.EmailAlterado {
			a.EmailVerificado = false
		}
	}
	return nil
}

func (a *Admin) applyAdminRoleAtualizado(event DomainEvent) error {
	payload := event.GetPayload()
	data, _ := json.Marshal(payload)
	var ev AdminRoleAtualizadoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

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

func (e *AdminCriadoEvent) GetPayload() interface{} { return e }

type EmailVerificadoEvent struct {
	BaseEvent
	VerifiedAt time.Time
}

func (e *EmailVerificadoEvent) GetPayload() interface{} { return e }

type AdminAtivadoEvent struct {
	BaseEvent
	ActivatedBy uuid.UUID
	ActivatedAt time.Time
}

func (e *AdminAtivadoEvent) GetPayload() interface{} { return e }

type AdminDesativadoEvent struct {
	BaseEvent
	DeactivatedBy uuid.UUID
	Motivo        string
	DeactivatedAt time.Time
}

func (e *AdminDesativadoEvent) GetPayload() interface{} { return e }

type AcaoAdminRegistradaEvent struct {
	BaseEvent
	Acao        string
	Detalhes    map[string]interface{}
	PerformedAt time.Time
}

func (e *AcaoAdminRegistradaEvent) GetPayload() interface{} { return e }

type AdminDadosAtualizadosEvent struct {
	BaseEvent
	Nome          *string
	Email         *string
	EmailAlterado bool
	UpdatedBy     uuid.UUID
	UpdatedAt     time.Time
}

func (e *AdminDadosAtualizadosEvent) GetPayload() interface{} { return e }

type AdminRoleAtualizadoEvent struct {
	BaseEvent
	RoleAnterior string
	NovoRole     string
	UpdatedBy    uuid.UUID
	UpdatedAt    time.Time
}

func (e *AdminRoleAtualizadoEvent) GetPayload() interface{} { return e }