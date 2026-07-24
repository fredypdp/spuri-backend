package aggregates

import (
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"spuri/internal/utils"

	"github.com/google/uuid"
)

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

var adminHierarchy = map[string]int{
	"fpp":     3,
	"adm":     2,
	"gerente": 1,
}

// ============================================================================
// Aggregate
// ============================================================================

type Admin struct {
	BaseAggregate

	Nome               string
	Email              string
	Telefone           *string
	TelefoneVerificado bool
	SenhaHash          string
	Role               string
	Status             string
	EmailVerificado    bool
	CreatedBy          *uuid.UUID
	CreatedAt          time.Time

	// FIX AD-02: campos de auditoria de ativação/desativação adicionados ao
	// estado do aggregate para rastreabilidade no ciclo de vida em memória.
	ActivatedAt   time.Time
	ActivatedBy   uuid.UUID
	DeactivatedAt time.Time
	DeactivatedBy uuid.UUID

	TotalAcoesRealizadas int
}

func NewAdmin() *Admin {
	return &Admin{
		BaseAggregate: BaseAggregate{
			ID:                uuid.New(),
			Version:           0,
			UncommittedEvents: []DomainEvent{},
		},
	}
}

func (a *Admin) GetType() string { return "Admin" }

// ============================================================================
// Apply dispatcher
// ============================================================================

func (a *Admin) Apply(event DomainEvent) error {
	switch event.GetEventType() {
	case "AdminCriado":
		return a.applyAdminCriado(event)
	case "EmailVerificado":
		return a.applyEmailVerificado(event)
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
	case "AdminSenhaAlterada":
		return a.applyAdminSenhaAlterada(event)
	default:
		return fmt.Errorf("tipo de evento desconhecido: %s", event.GetEventType())
	}
}

// ============================================================================
// Comandos
// ============================================================================

// Criar registra o evento de criação do admin.
// senhaHash deve ter pelo menos 60 caracteres (comprimento mínimo de bcrypt).
func (a *Admin) Criar(nome, email string, telefone *string, senhaHash, role string, createdBy *uuid.UUID) error {
	if nome == "" || email == "" || telefone == nil || *telefone == "" || senhaHash == "" {
		return fmt.Errorf("campos obrigatórios vazios")
	}

	if len(senhaHash) < 60 {
		return fmt.Errorf("senhaHash inválido: esperado hash bcrypt (mínimo 60 caracteres)")
	}

	if !emailRegex.MatchString(email) {
		return fmt.Errorf("formato de email inválido")
	}

	if err := utils.ValidatePhone(*telefone); err != nil {
		return err
	}

	validRoles := map[string]bool{"fpp": true, "adm": true, "gerente": true}
	if !validRoles[role] {
		return fmt.Errorf("role deve ser 'fpp', 'adm' ou 'gerente'")
	}

	event := &AdminCriadoEvent{
		BaseEvent: BaseEvent{EventType: "AdminCriado", AggregateID: a.ID},
		Nome:      nome,
		Email:     email,
		Telefone:  telefone,
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

func (a *Admin) AtualizarDados(nome *string, email *string, telefone *string, updatedBy uuid.UUID) error {
	if a.Status != "ativo" {
		return fmt.Errorf("administrador está inativo")
	}

	if nome == nil && email == nil && telefone == nil {
		return fmt.Errorf("nenhum campo para atualizar")
	}

	if email != nil && !emailRegex.MatchString(*email) {
		return fmt.Errorf("formato de email inválido")
	}
	if telefone != nil {
		if err := utils.ValidatePhone(*telefone); err != nil {
			return err
		}
	}

	emailAlterado := email != nil && a.Email != *email
	telefoneAlterado := telefone != nil && (a.Telefone == nil || *a.Telefone != *telefone)

	event := &AdminDadosAtualizadosEvent{
		BaseEvent:        BaseEvent{EventType: "AdminDadosAtualizados", AggregateID: a.ID},
		Nome:             nome,
		Email:            email,
		Telefone:         telefone,
		EmailAlterado:    emailAlterado,
		TelefoneAlterado: telefoneAlterado,
		UpdatedBy:        updatedBy,
		UpdatedAt:        time.Now(),
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
func (a *Admin) AlterarSenha(novaSenhaHash string, alteradoPor uuid.UUID, motivo string) error {
	if a.Status != "ativo" {
		return fmt.Errorf("administrador está inativo")
	}

	if novaSenhaHash == "" {
		return fmt.Errorf("hash da nova senha não pode ser vazio")
	}

	event := &AdminSenhaAlteradaEvent{
		BaseEvent:     BaseEvent{EventType: "AdminSenhaAlterada", AggregateID: a.ID},
		NovaSenhaHash: novaSenhaHash,
		AlteradoPor:   alteradoPor,
		Motivo:        motivo,
		ChangedAt:     time.Now(),
	}

	a.RaiseEvent(event)
	return a.Apply(event)
}

// ValidatePermission verifica que este admin tem role ESTRITAMENTE superior ao targetRole.
func (a *Admin) ValidatePermission(targetRole string) error {
	if a.Status != "ativo" {
		return fmt.Errorf("administrador está inativo")
	}

	myLevel, myOk := adminHierarchy[a.Role]
	targetLevel, targetOk := adminHierarchy[targetRole]

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
// Apply handlers
// ============================================================================

func (a *Admin) applyAdminCriado(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
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
	a.Telefone = ev.Telefone
	a.SenhaHash = ev.SenhaHash
	a.Role = ev.Role
	a.Status = "ativo"
	a.EmailVerificado = false
	a.CreatedBy = ev.CreatedBy
	a.CreatedAt = ev.CreatedAt
	return nil
}

func (a *Admin) applyEmailVerificado(_ DomainEvent) error {
	a.EmailVerificado = true
	return nil
}

// applyAdminAtivado — FIX AD-02: deserializa o payload para atualizar os
// campos de auditoria (ActivatedBy, ActivatedAt) no estado do aggregate.
func (a *Admin) applyAdminAtivado(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applyAdminAtivado: marshal error: %w", err)
	}
	var ev AdminAtivadoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyAdminAtivado: unmarshal error: %w", err)
	}
	a.Status = "ativo"
	a.ActivatedBy = ev.ActivatedBy
	a.ActivatedAt = ev.ActivatedAt
	return nil
}

// applyAdminDesativado — FIX AD-02: deserializa o payload para atualizar os
// campos de auditoria (DeactivatedBy, DeactivatedAt) no estado do aggregate.
func (a *Admin) applyAdminDesativado(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applyAdminDesativado: marshal error: %w", err)
	}
	var ev AdminDesativadoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyAdminDesativado: unmarshal error: %w", err)
	}
	a.Status = "inativo"
	a.DeactivatedBy = ev.DeactivatedBy
	a.DeactivatedAt = ev.DeactivatedAt
	return nil
}

// applyAcaoAdminRegistrada — FIX AD-01: deserializa o payload para detectar
// corrupção silenciosa de Detalhes (map[string]interface{}).
// O aggregate apenas incrementa o contador; os detalhes são usados só pela projeção.
func (a *Admin) applyAcaoAdminRegistrada(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applyAcaoAdminRegistrada: marshal error: %w", err)
	}
	var ev AcaoAdminRegistradaEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyAcaoAdminRegistrada: unmarshal error (payload corrompido): %w", err)
	}
	a.TotalAcoesRealizadas++
	return nil
}

func (a *Admin) applyAdminDadosAtualizados(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applyAdminDadosAtualizados: marshal error: %w", err)
	}
	var ev AdminDadosAtualizadosEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyAdminDadosAtualizados: unmarshal error: %w", err)
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
	if ev.Telefone != nil {
		a.Telefone = ev.Telefone
		if ev.TelefoneAlterado {
			a.TelefoneVerificado = false
		}
	}
	return nil
}

func (a *Admin) applyAdminRoleAtualizado(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applyAdminRoleAtualizado: marshal error: %w", err)
	}
	var ev AdminRoleAtualizadoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyAdminRoleAtualizado: unmarshal error: %w", err)
	}
	a.Role = ev.NovoRole
	return nil
}

func (a *Admin) applyAdminSenhaAlterada(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applyAdminSenhaAlterada: marshal error: %w", err)
	}
	var ev AdminSenhaAlteradaEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyAdminSenhaAlterada: unmarshal error: %w", err)
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
	Telefone  *string
	SenhaHash string
	Role      string
	CreatedBy *uuid.UUID
	CreatedAt time.Time
}

func (e *AdminCriadoEvent) GetPayload() interface{} { return e }
func (e *AdminCriadoEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type AdminAtivadoEvent struct {
	BaseEvent
	ActivatedBy uuid.UUID
	ActivatedAt time.Time
}

func (e *AdminAtivadoEvent) GetPayload() interface{} { return e }
func (e *AdminAtivadoEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type AdminDesativadoEvent struct {
	BaseEvent
	DeactivatedBy uuid.UUID
	Motivo        string
	DeactivatedAt time.Time
}

func (e *AdminDesativadoEvent) GetPayload() interface{} { return e }
func (e *AdminDesativadoEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type AcaoAdminRegistradaEvent struct {
	BaseEvent
	Acao        string
	Detalhes    map[string]interface{}
	PerformedAt time.Time
}

func (e *AcaoAdminRegistradaEvent) GetPayload() interface{} { return e }
func (e *AcaoAdminRegistradaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type AdminDadosAtualizadosEvent struct {
	BaseEvent
	Nome             *string
	Email            *string
	EmailAlterado    bool
	Telefone         *string
	TelefoneAlterado bool
	UpdatedBy        uuid.UUID
	UpdatedAt        time.Time
}

func (e *AdminDadosAtualizadosEvent) GetPayload() interface{} { return e }
func (e *AdminDadosAtualizadosEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type AdminRoleAtualizadoEvent struct {
	BaseEvent
	RoleAnterior string
	NovoRole     string
	UpdatedBy    uuid.UUID
	UpdatedAt    time.Time
}

func (e *AdminRoleAtualizadoEvent) GetPayload() interface{} { return e }
func (e *AdminRoleAtualizadoEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type AdminSenhaAlteradaEvent struct {
	BaseEvent
	NovaSenhaHash string
	AlteradoPor   uuid.UUID
	Motivo        string
	ChangedAt     time.Time
}

func (e *AdminSenhaAlteradaEvent) GetPayload() interface{} { return e }
func (e *AdminSenhaAlteradaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }
