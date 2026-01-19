// ============================================================================
// ARQUIVO: internal/domain/aggregates/curso.go
// Agregado Curso (Event Sourcing)
// ============================================================================

package aggregates

import (
	"encoding/json"
	"fmt"
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
	switch event.GetEventType() {
	case "CursoCriado":
		return c.applyCursoCriado(event)
	case "CursoAtivado":
		return c.applyCursoAtivado(event)
	case "CursoDesativado":
		return c.applyCursoDesativado(event)
	default:
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
	// Validações
	if nome == "" {
		return fmt.Errorf("nome é obrigatório")
	}
	if tipo != "medio" && tipo != "superior" {
		return fmt.Errorf("tipo deve ser 'medio' ou 'superior'")
	}
	if len(nivel) == 0 {
		return fmt.Errorf("nível é obrigatório")
	}
	if codigoAcademia == "" {
		return fmt.Errorf("código da academia é obrigatório")
	}
	
	// 🔥 Validar anos acadêmicos
	if err := utils.ValidateNivelCurso(tipo, nivel); err != nil {
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

	c.RaiseEvent(event)
	return c.Apply(event)
}

func (c *Curso) Ativar() error {
	if c.Status == "ativo" {
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
	if c.Status == "inativo" {
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
	payload := event.GetPayload()
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	var ev CursoCriadoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	c.ID = event.GetAggregateID()
	c.Nome = ev.Nome
	c.Type = ev.Type
	c.Nivel = ev.Nivel
	c.CodigoAcademia = ev.CodigoAcademia
	c.Status = "ativo"
	c.CreatedAt = ev.CreatedAt

	return nil
}

func (c *Curso) applyCursoAtivado(event DomainEvent) error {
	c.Status = "ativo"
	return nil
}

func (c *Curso) applyCursoDesativado(event DomainEvent) error {
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