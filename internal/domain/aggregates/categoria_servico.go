package aggregates

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type CategoriaServico struct {
	BaseAggregate
	CodigoAcademia string
	Nome           string
	Ativo          bool
	CriadoPor      uuid.UUID
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func NewCategoriaServico() *CategoriaServico {
	return &CategoriaServico{BaseAggregate: BaseAggregate{ID: uuid.New(), Version: 0, UncommittedEvents: []DomainEvent{}}, Ativo: true}
}
func (c *CategoriaServico) GetType() string { return "CategoriaServico" }

type CategoriaServicoCriadaEvent struct {
	BaseEvent
	CodigoAcademia, Nome string
	CriadoPor            uuid.UUID
	CreatedAt            time.Time
}

func (e *CategoriaServicoCriadaEvent) GetPayload() interface{} { return e }
func (e *CategoriaServicoCriadaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type CategoriaServicoRenomeadaEvent struct {
	BaseEvent
	Nome          string
	AtualizadoPor uuid.UUID
	UpdatedAt     time.Time
}

func (e *CategoriaServicoRenomeadaEvent) GetPayload() interface{} { return e }
func (e *CategoriaServicoRenomeadaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type CategoriaServicoDesativadaEvent struct {
	BaseEvent
	DesativadoPor uuid.UUID
	UpdatedAt     time.Time
}

func (e *CategoriaServicoDesativadaEvent) GetPayload() interface{} { return e }
func (e *CategoriaServicoDesativadaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type CategoriaServicoReativadaEvent struct {
	BaseEvent
	ReativadoPor uuid.UUID
	UpdatedAt    time.Time
}

func (e *CategoriaServicoReativadaEvent) GetPayload() interface{} { return e }
func (e *CategoriaServicoReativadaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

func (c *CategoriaServico) Apply(event DomainEvent) error {
	switch event.GetEventType() {
	case "CategoriaServicoCriada":
		return c.applyCriada(event)
	case "CategoriaServicoRenomeada":
		return c.applyRenomeada(event)
	case "CategoriaServicoDesativada":
		c.Ativo = false
		return nil
	case "CategoriaServicoReativada":
		c.Ativo = true
		return nil
	default:
		return fmt.Errorf("tipo de evento desconhecido para CategoriaServico: %s", event.GetEventType())
	}
}
func (c *CategoriaServico) Criar(codigoAcademia, nome string, criadoPor uuid.UUID) error {
	if strings.TrimSpace(codigoAcademia) == "" {
		return fmt.Errorf("codigo_academia é obrigatório")
	}
	nome = strings.TrimSpace(nome)
	if nome == "" {
		return fmt.Errorf("nome é obrigatório")
	}
	if len(nome) > 100 {
		return fmt.Errorf("nome deve ter no máximo 100 caracteres")
	}
	e := &CategoriaServicoCriadaEvent{BaseEvent: BaseEvent{EventType: "CategoriaServicoCriada", AggregateID: c.ID}, CodigoAcademia: codigoAcademia, Nome: nome, CriadoPor: criadoPor, CreatedAt: time.Now()}
	c.RaiseEvent(e)
	return c.Apply(e)
}
func (c *CategoriaServico) Renomear(nome string, atualizadoPor uuid.UUID) error {
	nome = strings.TrimSpace(nome)
	if nome == "" {
		return fmt.Errorf("nome é obrigatório")
	}
	if len(nome) > 100 {
		return fmt.Errorf("nome deve ter no máximo 100 caracteres")
	}
	e := &CategoriaServicoRenomeadaEvent{BaseEvent: BaseEvent{EventType: "CategoriaServicoRenomeada", AggregateID: c.ID}, Nome: nome, AtualizadoPor: atualizadoPor, UpdatedAt: time.Now()}
	c.RaiseEvent(e)
	return c.Apply(e)
}
func (c *CategoriaServico) Desativar(p uuid.UUID) error {
	if !c.Ativo {
		return fmt.Errorf("categoria já está inativa")
	}
	e := &CategoriaServicoDesativadaEvent{BaseEvent: BaseEvent{EventType: "CategoriaServicoDesativada", AggregateID: c.ID}, DesativadoPor: p, UpdatedAt: time.Now()}
	c.RaiseEvent(e)
	return c.Apply(e)
}
func (c *CategoriaServico) Reativar(p uuid.UUID) error {
	if c.Ativo {
		return fmt.Errorf("categoria já está ativa")
	}
	e := &CategoriaServicoReativadaEvent{BaseEvent: BaseEvent{EventType: "CategoriaServicoReativada", AggregateID: c.ID}, ReativadoPor: p, UpdatedAt: time.Now()}
	c.RaiseEvent(e)
	return c.Apply(e)
}
func (c *CategoriaServico) applyCriada(e DomainEvent) error {
	b, err := json.Marshal(e.GetPayload())
	if err != nil {
		return err
	}
	var p CategoriaServicoCriadaEvent
	if err = json.Unmarshal(b, &p); err != nil {
		return err
	}
	c.CodigoAcademia = p.CodigoAcademia
	c.Nome = p.Nome
	c.Ativo = true
	c.CriadoPor = p.CriadoPor
	c.CreatedAt = p.CreatedAt
	c.UpdatedAt = p.CreatedAt
	return nil
}
func (c *CategoriaServico) applyRenomeada(e DomainEvent) error {
	b, err := json.Marshal(e.GetPayload())
	if err != nil {
		return err
	}
	var p CategoriaServicoRenomeadaEvent
	if err = json.Unmarshal(b, &p); err != nil {
		return err
	}
	c.Nome = p.Nome
	c.UpdatedAt = p.UpdatedAt
	return nil
}
