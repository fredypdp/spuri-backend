// ============================================================================
// ARQUIVO: internal/domain/aggregates/curso.go
// Agregado Curso (Event Sourcing) + logs de debug
// ============================================================================

package aggregates

import (
	"encoding/json"
	"fmt"
	"log"
	"spuri/internal/utils"
	"time"

	"github.com/google/uuid"
)

type Curso struct {
	BaseAggregate
	
	Nome           string
	Type           string   // "medio" ou "superior"
	Nivel          []string // ["7ano", "8ano"] ou ["1ano", "2ano", "3ano"]
	CodigoAcademia string
	Status         string // "ativo" ou "inativo"
	CreatedAt      time.Time
}

func NewCurso() *Curso {
	log.Printf("[DEBUG] Criando novo agregado Curso")
	return &Curso{
		BaseAggregate: BaseAggregate{
			ID:                uuid.New(),
			Version:           0,
			UncommittedEvents: []DomainEvent{},
		},
		Status: "ativo",
	}
}

func (c *Curso) GetType() string {
	return "Curso"
}

func (c *Curso) Apply(event DomainEvent) error {
	log.Printf("[DEBUG] Aplicando evento %s ao Curso %s", event.GetEventType(), c.ID)
	
	switch event.GetEventType() {
	case "CursoCriado":
		return c.applyCursoCriado(event)
	case "CursoAtivado":
		return c.applyCursoAtivado(event)
	case "CursoDesativado":
		return c.applyCursoDesativado(event)
	case "CursoDadosAtualizados":
		return c.applyCursoDadosAtualizados(event)
	default:
		log.Printf("[ERROR] Tipo de evento desconhecido: %s", event.GetEventType())
		return fmt.Errorf("tipo de evento desconhecido: %s", event.GetEventType())
	}
}

// Comandos

func (c *Curso) Criar(
	nome string,
	tipo string,
	nivel []string,
	codigoAcademia string,
) error {
	log.Printf("[DEBUG] Criando curso: nome=%s, tipo=%s, nivel=%v, academia=%s", nome, tipo, nivel, codigoAcademia)
	
	// Validações
	if nome == "" {
		log.Printf("[ERROR] Nome é obrigatório")
		return fmt.Errorf("nome é obrigatório")
	}
	if tipo != "medio" && tipo != "superior" {
		log.Printf("[ERROR] Tipo inválido: %s", tipo)
		return fmt.Errorf("tipo deve ser 'medio' ou 'superior'")
	}
	if len(nivel) == 0 {
		log.Printf("[ERROR] Nível é obrigatório")
		return fmt.Errorf("nível é obrigatório")
	}
	if codigoAcademia == "" {
		log.Printf("[ERROR] Código da academia é obrigatório")
		return fmt.Errorf("código da academia é obrigatório")
	}
	
	// Validar anos acadêmicos
	if err := utils.ValidateNivelCurso(tipo, nivel); err != nil {
		log.Printf("[ERROR] Validação de nível falhou: %v", err)
		return err
	}

	event := &CursoCriadoEvent{
		BaseEvent: BaseEvent{
			EventType:   "CursoCriado",
			AggregateID: c.ID,
		},
		Nome:           nome,
		Type:           tipo,
		Nivel:          nivel,
		CodigoAcademia: codigoAcademia,
		CreatedAt:      time.Now(),
	}

	log.Printf("[DEBUG] Evento CursoCriado criado para curso %s", c.ID)
	c.RaiseEvent(event)
	return c.Apply(event)
}

func (c *Curso) Ativar() error {
	log.Printf("[DEBUG] Ativando curso %s (status atual: %s)", c.ID, c.Status)
	
	if c.Status == "ativo" {
		log.Printf("[ERROR] Curso já está ativo")
		return fmt.Errorf("curso já está ativo")
	}

	event := &CursoAtivadoEvent{
		BaseEvent: BaseEvent{
			EventType:   "CursoAtivado",
			AggregateID: c.ID,
		},
		ActivatedAt: time.Now(),
	}

	c.RaiseEvent(event)
	return c.Apply(event)
}

func (c *Curso) Desativar() error {
	log.Printf("[DEBUG] Desativando curso %s (status atual: %s)", c.ID, c.Status)
	
	if c.Status == "inativo" {
		log.Printf("[ERROR] Curso já está inativo")
		return fmt.Errorf("curso já está inativo")
	}

	event := &CursoDesativadoEvent{
		BaseEvent: BaseEvent{
			EventType:   "CursoDesativado",
			AggregateID: c.ID,
		},
		DeactivatedAt: time.Now(),
	}

	c.RaiseEvent(event)
	return c.Apply(event)
}

// Event Handlers

func (c *Curso) applyCursoCriado(event DomainEvent) error {
	log.Printf("[DEBUG] Aplicando CursoCriado ao agregado %s", event.GetAggregateID())
	
	payload := event.GetPayload()
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[ERROR] Erro ao serializar payload: %v", err)
		return err
	}

	var ev CursoCriadoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		log.Printf("[ERROR] Erro ao deserializar evento: %v", err)
		return err
	}

	c.ID = event.GetAggregateID()
	c.Nome = ev.Nome
	c.Type = ev.Type
	c.Nivel = ev.Nivel
	c.CodigoAcademia = ev.CodigoAcademia
	c.Status = "ativo"
	c.CreatedAt = ev.CreatedAt

	log.Printf("[DEBUG] Curso criado: %s (%s)", c.Nome, c.ID)
	return nil
}

func (c *Curso) applyCursoAtivado(event DomainEvent) error {
	log.Printf("[DEBUG] Aplicando CursoAtivado ao agregado %s", event.GetAggregateID())
	c.Status = "ativo"
	return nil
}

func (c *Curso) applyCursoDesativado(event DomainEvent) error {
	log.Printf("[DEBUG] Aplicando CursoDesativado ao agregado %s", event.GetAggregateID())
	c.Status = "inativo"
	return nil
}

// Eventos

type CursoCriadoEvent struct {
	BaseEvent
	Nome           string
	Type           string
	Nivel          []string
	CodigoAcademia string
	CreatedAt      time.Time
}

func (e *CursoCriadoEvent) GetPayload() interface{} {
	return e
}

type CursoAtivadoEvent struct {
	BaseEvent
	ActivatedAt time.Time
}

func (e *CursoAtivadoEvent) GetPayload() interface{} {
	return e
}

type CursoDesativadoEvent struct {
	BaseEvent
	DeactivatedAt time.Time
}

func (e *CursoDesativadoEvent) GetPayload() interface{} {
	return e
}

// AtualizarDados atualiza dados do curso
func (c *Curso) AtualizarDados(
	nome *string,
	tipo *string,
	nivel []string,
) error {
	log.Printf("[DEBUG] Atualizando dados do curso %s", c.ID)
	
	if c.Status != "ativo" {
		log.Printf("[ERROR] Curso inativo não pode ser atualizado")
		return fmt.Errorf("curso inativo não pode ser atualizado")
	}

	// Validação: pelo menos um campo deve ser fornecido
	if nome == nil && tipo == nil && nivel == nil {
		log.Printf("[ERROR] Nenhum campo para atualizar")
		return fmt.Errorf("nenhum campo para atualizar")
	}

	// Validações específicas
	if nome != nil && *nome == "" {
		log.Printf("[ERROR] Nome não pode ser vazio")
		return fmt.Errorf("nome não pode ser vazio")
	}

	if tipo != nil && *tipo != "medio" && *tipo != "superior" {
		log.Printf("[ERROR] Tipo inválido: %s", *tipo)
		return fmt.Errorf("tipo deve ser 'medio' ou 'superior'")
	}

	// Se está atualizando tipo ou nivel, validar
	tipoFinal := c.Type
	if tipo != nil {
		tipoFinal = *tipo
	}

	nivelFinal := c.Nivel
	if nivel != nil {
		nivelFinal = nivel
	}

	// Validar anos acadêmicos
	if err := utils.ValidateNivelCurso(tipoFinal, nivelFinal); err != nil {
		log.Printf("[ERROR] Validação de nível falhou: %v", err)
		return err
	}

	event := &CursoDadosAtualizadosEvent{
		BaseEvent: BaseEvent{
			EventType:   "CursoDadosAtualizados",
			AggregateID: c.ID,
		},
		Nome:      nome,
		Type:      tipo,
		Nivel:     nivel,
		UpdatedAt: time.Now(),
	}

	log.Printf("[DEBUG] Evento CursoDadosAtualizados criado")
	c.RaiseEvent(event)
	return c.Apply(event)
}

func (c *Curso) applyCursoDadosAtualizados(event DomainEvent) error {
	log.Printf("[DEBUG] Aplicando CursoDadosAtualizados ao agregado %s", event.GetAggregateID())
	
	payload := event.GetPayload()
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[ERROR] Erro ao serializar payload: %v", err)
		return err
	}

	var ev CursoDadosAtualizadosEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		log.Printf("[ERROR] Erro ao deserializar evento: %v", err)
		return err
	}

	// Atualizar apenas os campos fornecidos
	if ev.Nome != nil {
		log.Printf("[DEBUG] Atualizando nome: %s -> %s", c.Nome, *ev.Nome)
		c.Nome = *ev.Nome
	}
	if ev.Type != nil {
		log.Printf("[DEBUG] Atualizando tipo: %s -> %s", c.Type, *ev.Type)
		c.Type = *ev.Type
	}
	if ev.Nivel != nil {
		log.Printf("[DEBUG] Atualizando nível: %v -> %v", c.Nivel, ev.Nivel)
		c.Nivel = ev.Nivel
	}

	log.Printf("[DEBUG] Dados do curso atualizados com sucesso")
	return nil
}

type CursoDadosAtualizadosEvent struct {
	BaseEvent
	Nome      *string
	Type      *string
	Nivel     []string
	UpdatedAt time.Time
}

func (e *CursoDadosAtualizadosEvent) GetPayload() interface{} {
	return e
}