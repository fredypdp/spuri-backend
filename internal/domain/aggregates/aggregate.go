package aggregates

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

// Aggregate interface base para todos os agregados
type Aggregate interface {
	// GetID retorna o ID do agregado
	GetID() uuid.UUID
	
	// GetVersion retorna a versão atual do agregado
	GetVersion() int
	
	// GetType retorna o tipo do agregado
	GetType() string
	
	// Apply aplica um evento ao agregado
	Apply(event DomainEvent) error
	
	// GetUncommittedEvents retorna eventos não commitados
	GetUncommittedEvents() []DomainEvent
	
	// ClearUncommittedEvents limpa eventos não commitados
	ClearUncommittedEvents()
}

// DomainEvent interface para eventos de domínio
type DomainEvent interface {
	// GetEventType retorna o tipo do evento
	GetEventType() string
	
	// GetAggregateID retorna o ID do agregado
	GetAggregateID() uuid.UUID
	
	// GetPayload retorna o payload do evento
	GetPayload() interface{}
	
	// ToJSON converte para JSON
	ToJSON() ([]byte, error)
}

// BaseAggregate estrutura base para agregados
type BaseAggregate struct {
	ID                 uuid.UUID
	Version            int
	UncommittedEvents  []DomainEvent
}

// GetID implementa Aggregate
func (a *BaseAggregate) GetID() uuid.UUID {
	return a.ID
}

// GetVersion implementa Aggregate
func (a *BaseAggregate) GetVersion() int {
	return a.Version
}

// GetUncommittedEvents implementa Aggregate
func (a *BaseAggregate) GetUncommittedEvents() []DomainEvent {
	return a.UncommittedEvents
}

// ClearUncommittedEvents implementa Aggregate
func (a *BaseAggregate) ClearUncommittedEvents() {
	a.UncommittedEvents = []DomainEvent{}
}

// RaiseEvent adiciona um evento não commitado
func (a *BaseAggregate) RaiseEvent(event DomainEvent) {
	a.UncommittedEvents = append(a.UncommittedEvents, event)
	a.Version++
}

// BaseEvent estrutura base para eventos
type BaseEvent struct {
	EventType   string
	AggregateID uuid.UUID
	Payload     interface{}
}

// GetEventType implementa DomainEvent
func (e *BaseEvent) GetEventType() string {
	return e.EventType
}

// GetAggregateID implementa DomainEvent
func (e *BaseEvent) GetAggregateID() uuid.UUID {
	return e.AggregateID
}

// GetPayload implementa DomainEvent
func (e *BaseEvent) GetPayload() interface{} {
	return e.Payload
}

// ToJSON implementa DomainEvent
func (e *BaseEvent) ToJSON() ([]byte, error) {
	return json.Marshal(e.Payload)
}

// AggregateFactory cria agregados a partir de eventos
type AggregateFactory interface {
	Create(aggregateType string) (Aggregate, error)
}

// DefaultAggregateFactory fábrica padrão de agregados
type DefaultAggregateFactory struct{}

// Create cria um agregado baseado no tipo
func (f *DefaultAggregateFactory) Create(aggregateType string) (Aggregate, error) {
	switch aggregateType {
	case "Estudante":
		return NewEstudante(), nil
	case "Academia":
		return NewAcademia(), nil
	default:
		return nil, fmt.Errorf("tipo de agregado desconhecido: %s", aggregateType)
	}
}

// Repository interface para repositório de agregados
type Repository interface {
	// Load carrega um agregado pelo ID
	Load(id uuid.UUID, aggregateType string) (Aggregate, error)
	
	// Save salva um agregado
	Save(aggregate Aggregate) error
	
	// Exists verifica se um agregado existe
	Exists(id uuid.UUID) (bool, error)
}

// EventApplier aplica eventos a um agregado
type EventApplier struct {
	factory AggregateFactory
}

// NewEventApplier cria um novo EventApplier
func NewEventApplier(factory AggregateFactory) *EventApplier {
	return &EventApplier{
		factory: factory,
	}
}

// BuildFromEvents reconstrói um agregado a partir de eventos
func (ea *EventApplier) BuildFromEvents(
	aggregateType string,
	events []DomainEvent,
) (Aggregate, error) {
	if len(events) == 0 {
		return nil, fmt.Errorf("nenhum evento fornecido")
	}

	// Criar agregado vazio
	aggregate, err := ea.factory.Create(aggregateType)
	if err != nil {
		return nil, err
	}

	// Aplicar todos os eventos em ordem
	for _, event := range events {
		if err := aggregate.Apply(event); err != nil {
			return nil, fmt.Errorf("erro ao aplicar evento %s: %w", 
				event.GetEventType(), err)
		}
	}

	return aggregate, nil
}