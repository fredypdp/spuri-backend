// ============================================================================
// ARQUIVO: internal/domain/aggregates/admin.go
//
// CORREÇÕES APLICADAS:
//   #1  — Criar() valida hash bcrypt mínimo (60 chars)
//   #7  — Apply handlers retornam erro em vez de panic
//   [A05] — ValidatePermission: protegido contra role desconhecido (zero-value
//            no map retornava 0, permitindo operação com role inválido)
// ============================================================================

package aggregates

import (
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/google/uuid"
)

// emailRegex valida formato básico de email.
var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

// adminHierarchy é a hierarquia canônica de roles.
// Centralizada aqui para evitar divergência com middleware e handlers.
var adminHierarchy = map[string]int{"fpp": 3, "adm": 2, "gerente": 1}

// ============================================================================
// Struct
// ============================================================================

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

func (a *Admin) GetType() string { return "Admin" }

// ============================================================================
// Apply — roteador de eventos
// ============================================================================

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
	case "AdminSenhaAlterada":
		return a.applyAdminSenhaAlterada(event)
	default:
		return fmt.Errorf("tipo de evento desconhecido: %s", event.GetEventType())
	}
}

// ============================================================================
// Comandos
// ============================================================================

// Criar cria um novo administrador.
// senhaHash deve ter pelo menos 60 caracteres (comprimento mínimo de bcrypt).
func (a *Admin) Criar(nome, email, senhaHash, role string, createdBy *uuid.UUID) error {
	if nome == "" || email == "" || senhaHash == "" {
		return fmt.Errorf("campos obrigatórios vazios")
	}

	if len(senhaHash) < 60 {
		return fmt.Errorf("senhaHash inválido: esperado hash bcrypt (mínimo 60 caracteres)")
	}

	if !emailRegex.MatchString(email) {
		return fmt.Errorf("formato de email inválido")
	}

	validRoles := map[string]bool{"fpp": true, "adm": true, "gerente": true}
	if !validRoles[role] {
		return fmt.Errorf("role deve ser 'fpp', 'adm' ou 'gerente'")
	}

	event := &AdminCriadoEvent{
		BaseEvent: BaseEvent{EventType: "AdminCriado", AggregateID: a.ID},
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

func (a *Admin) VerificarEmail() error {
	if a.EmailVerificado {
		return fmt.Errorf("email já verificado")
	}

	event := &EmailVerificadoEvent{
		BaseEvent:  BaseEvent{EventType: "EmailVerificado", AggregateID: a.ID},
		VerifiedAt: time.Now(),
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

	if email != nil && !emailRegex.MatchString(*email) {
		return fmt.Errorf("formato de email inválido")
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

// AtualizarRole altera o role do admin.
// REGRA DE NEGÓCIO DELIBERADA: apenas FPP pode alterar roles.
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

// AlterarSenha registra a troca de senha como evento no ledger.
func (a *Admin) AlterarSenha(novaSenhaHash string, changedBy uuid.UUID, motivo string) error {
	if a.Status != "ativo" {
		return fmt.Errorf("administrador está inativo")
	}

	if novaSenhaHash == "" {
		return fmt.Errorf("hash da nova senha não pode ser vazio")
	}

	event := &AdminSenhaAlteradaEvent{
		BaseEvent:     BaseEvent{EventType: "AdminSenhaAlterada", AggregateID: a.ID},
		NovaSenhaHash: novaSenhaHash,
		ChangedBy:     changedBy,
		Motivo:        motivo,
		ChangedAt:     time.Now(),
	}

	a.RaiseEvent(event)
	return a.Apply(event)
}

// ValidatePermission verifica que este admin tem role ESTRITAMENTE superior ao targetRole.
//
// [A05] CORRIGIDO: antes, um targetRole desconhecido retornava 0 do map,
// e a condição `hierarchy[a.Role] <= 0` era false para qualquer role válido,
// permitindo a operação. Agora valida explicitamente ambos os roles.
func (a *Admin) ValidatePermission(targetRole string) error {
	if a.Status != "ativo" {
		return fmt.Errorf("administrador está inativo")
	}

	myLevel, myOk := adminHierarchy[a.Role]
	targetLevel, targetOk := adminHierarchy[targetRole]

	// Rejeita roles desconhecidos em qualquer dos lados
	if !myOk {
		return fmt.Errorf("role do executor '%s' é inválido", a.Role)
	}
	if !targetOk {
		return fmt.Errorf("role alvo '%s' é inválido", targetRole)
	}

	if myLevel <= targetLevel {
		return fmt.Errorf("permissão negada: role '%s' não pode gerenciar '%s'", a.Role, targetRole)
	}

	return nil
}

// ============================================================================
// Apply handlers — todos com verificação de erro em json.Marshal/Unmarshal
// ============================================================================

func (a *Admin) applyAdminCriado(event DomainEvent) error {
	payload := event.GetPayload()
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("applyAdminCriado: erro ao serializar payload: %w", err)
	}
	var ev AdminCriadoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyAdminCriado: erro ao deserializar evento: %w", err)
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
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("applyAdminDadosAtualizados: erro ao serializar payload: %w", err)
	}
	var ev AdminDadosAtualizadosEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyAdminDadosAtualizados: erro ao deserializar evento: %w", err)
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
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("applyAdminRoleAtualizado: erro ao serializar payload: %w", err)
	}
	var ev AdminRoleAtualizadoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyAdminRoleAtualizado: erro ao deserializar evento: %w", err)
	}
	a.Role = ev.NovoRole
	return nil
}

func (a *Admin) applyAdminSenhaAlterada(event DomainEvent) error {
	payload := event.GetPayload()
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("applyAdminSenhaAlterada: erro ao serializar payload: %w", err)
	}
	var ev AdminSenhaAlteradaEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyAdminSenhaAlterada: erro ao deserializar evento: %w", err)
	}
	if ev.NovaSenhaHash == "" {
		return fmt.Errorf("applyAdminSenhaAlterada: NovaSenhaHash vazio no payload")
	}
	a.SenhaHash = ev.NovaSenhaHash
	return nil
}

// ============================================================================
// Eventos
// ============================================================================

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

type AdminSenhaAlteradaEvent struct {
	BaseEvent
	NovaSenhaHash string
	ChangedBy     uuid.UUID
	Motivo        string
	ChangedAt     time.Time
}

func (e *AdminSenhaAlteradaEvent) GetPayload() interface{} { return e }