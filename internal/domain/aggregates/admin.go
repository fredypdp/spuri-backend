// ============================================================================
// ARQUIVO: internal/domain/aggregates/admin.go
// Agregado Admin com hierarquia de permissões (fpp > adm > gerente)
// ============================================================================

package aggregates

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Admin agregado raiz para administradores
type Admin struct {
	BaseAggregate
	
	// Estado
	Nome      string
	Email     string
	SenhaHash string
	Status    string // "ativo" ou "inativo"
	Role      string // "fpp", "adm", "gerente"
	CreatedAt time.Time
	CreatedBy *uuid.UUID // ID do admin que criou (se aplicável)
	
	// Estatísticas
	TotalAcoesRealizadas int
}

// NewAdmin cria um novo agregado Admin
func NewAdmin() *Admin {
	return &Admin{
		BaseAggregate: BaseAggregate{
			ID:                uuid.New(),
			Version:           0,
			UncommittedEvents: []DomainEvent{},
		},
	}
}

// GetType implementa Aggregate
func (a *Admin) GetType() string {
	return "Admin"
}

// Apply aplica eventos ao agregado
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
	default:
		return fmt.Errorf("tipo de evento desconhecido: %s", event.GetEventType())
	}
}

// Comandos - geram eventos

// Criar cria um novo administrador
func (a *Admin) Criar(
	nome string,
	email string,
	senhaHash string,
	role string,
	createdBy *uuid.UUID,
) error {
	// Validações
	if nome == "" {
		return fmt.Errorf("nome é obrigatório")
	}
	if email == "" {
		return fmt.Errorf("email é obrigatório")
	}
	if senhaHash == "" {
		return fmt.Errorf("senha é obrigatória")
	}
	
	// Validar role
	validRoles := map[string]bool{
		"fpp":     true,
		"adm":     true,
		"gerente": true,
	}
	if !validRoles[role] {
		return fmt.Errorf("role deve ser 'fpp', 'adm' ou 'gerente'")
	}

	// Criar evento
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

	a.RaiseEvent(event)
	return a.Apply(event)
}

// Ativar ativa o administrador
func (a *Admin) Ativar(adminID uuid.UUID) error {
	if a.Status == "ativo" {
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

	a.RaiseEvent(event)
	return a.Apply(event)
}

// Desativar desativa o administrador
func (a *Admin) Desativar(adminID uuid.UUID, motivo string) error {
	if a.Status == "inativo" {
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

	a.RaiseEvent(event)
	return a.Apply(event)
}

// RegistrarAcao registra uma ação administrativa
func (a *Admin) RegistrarAcao(acao string, detalhes map[string]interface{}) error {
	if a.Status != "ativo" {
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

	a.RaiseEvent(event)
	return a.Apply(event)
}

// Event Handlers - aplicam eventos ao estado

func (a *Admin) applyAdminCriado(event DomainEvent) error {
	payload := event.GetPayload()
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

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
	a.CreatedBy = ev.CreatedBy
	a.CreatedAt = ev.CreatedAt

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

// Eventos do Admin

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

// ValidatePermission verifica se o admin tem permissão para uma ação
func (a *Admin) ValidatePermission(targetRole string) error {
	if a.Status != "ativo" {
		return fmt.Errorf("administrador está inativo")
	}

	// Hierarquia: fpp > adm > gerente
	hierarchy := map[string]int{
		"fpp":     3,
		"adm":     2,
		"gerente": 1,
	}

	currentLevel := hierarchy[a.Role]
	targetLevel := hierarchy[targetRole]

	if currentLevel <= targetLevel {
		return fmt.Errorf("permissão negada: role '%s' não pode gerenciar '%s'", a.Role, targetRole)
	}

	return nil
}