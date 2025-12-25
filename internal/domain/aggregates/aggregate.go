// ============================================================================
// ARQUIVO: internal/domain/aggregates/aggregate.go
// ATUALIZADO: Adicionar suporte para Admin no factory
// ============================================================================

package aggregates

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

// Aggregate interface base para todos os agregados
type Aggregate interface {
	GetID() uuid.UUID
	GetVersion() int
	GetType() string
	Apply(event DomainEvent) error
	GetUncommittedEvents() []DomainEvent
	ClearUncommittedEvents()
}

// DomainEvent interface para eventos de domínio
type DomainEvent interface {
	GetEventType() string
	GetAggregateID() uuid.UUID
	GetPayload() interface{}
	ToJSON() ([]byte, error)
}

// BaseAggregate estrutura base para agregados
type BaseAggregate struct {
	ID                 uuid.UUID
	Version            int
	UncommittedEvents  []DomainEvent
}

func (a *BaseAggregate) GetID() uuid.UUID {
	return a.ID
}

func (a *BaseAggregate) GetVersion() int {
	return a.Version
}

func (a *BaseAggregate) GetUncommittedEvents() []DomainEvent {
	return a.UncommittedEvents
}

func (a *BaseAggregate) ClearUncommittedEvents() {
	a.UncommittedEvents = []DomainEvent{}
}

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

func (e *BaseEvent) GetEventType() string {
	return e.EventType
}

func (e *BaseEvent) GetAggregateID() uuid.UUID {
	return e.AggregateID
}

func (e *BaseEvent) GetPayload() interface{} {
	return e.Payload
}

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
// 🔥 ATUALIZADO: Adicionar suporte para Admin
func (f *DefaultAggregateFactory) Create(aggregateType string) (Aggregate, error) {
	switch aggregateType {
	case "Estudante":
		return NewEstudante(), nil
	case "Academia":
		return NewAcademia(), nil
	case "Admin":
		return NewAdmin(), nil
	default:
		return nil, fmt.Errorf("tipo de agregado desconhecido: %s", aggregateType)
	}
}

// Repository interface para repositório de agregados
type Repository interface {
	Load(id uuid.UUID, aggregateType string) (Aggregate, error)
	Save(aggregate Aggregate) error
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

	aggregate, err := ea.factory.Create(aggregateType)
	if err != nil {
		return nil, err
	}

	for _, event := range events {
		if err := aggregate.Apply(event); err != nil {
			return nil, fmt.Errorf("erro ao aplicar evento %s: %w", 
				event.GetEventType(), err)
		}
	}

	return aggregate, nil
}