// ============================================================================
// ARQUIVO: internal/domain/aggregates/aggregate.go
//
// CORREÇÕES APLICADAS (Etapa 1):
//   #1/#2 — BaseEvent.ToJSON() agora retorna json.Marshal(e) em vez de
//            json.Marshal(e.Payload).
//            Razão: quando o repository reconstrói um BaseEvent do banco
//            (Payload = json.RawMessage) e ToJSON() é chamado, o resultado
//            anterior omitia EventType e AggregateID, tornando o payload
//            incompleto caso o evento fosse re-gravado (reprocessamento/migração).
//            Com a correção, BaseEvent serializa todos os seus campos, ficando
//            consistente com o padrão de todos os eventos concretos.
// ============================================================================

package aggregates

import (
	"encoding/json"
	"fmt"
	"log"

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
	ID                uuid.UUID
	Version           int
	UncommittedEvents []DomainEvent
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
	log.Printf("[DEBUG] Limpando %d eventos não commitados", len(a.UncommittedEvents))
	a.UncommittedEvents = []DomainEvent{}
}

func (a *BaseAggregate) RaiseEvent(event DomainEvent) {
	log.Printf("[DEBUG] Levantando evento: %s para agregado: %s", event.GetEventType(), a.ID)
	a.UncommittedEvents = append(a.UncommittedEvents, event)
	a.Version++
	log.Printf("[DEBUG] Versão do agregado incrementada para: %d", a.Version)
}

// BaseEvent estrutura base para eventos.
//
// Eventos concretos DEVEM sobrescrever GetPayload() e ToJSON() para
// incluir seus campos específicos na serialização.
//
// GetPayload() base retorna e.Payload (campo interno), usado quando o
// evento foi reconstruído do banco com payload já serializado.
//
// ToJSON() base retorna json.Marshal(e) — inclui EventType, AggregateID e
// Payload. Garante que nenhum path que chame ToJSON() em um BaseEvent
// reconstruído do banco produza um payload incompleto.
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

// GetPayload retorna o campo Payload interno (json.RawMessage quando
// reconstruído do banco, ou nil para eventos concretos que sobrescrevem este método).
func (e *BaseEvent) GetPayload() interface{} {
	return e.Payload
}

// ToJSON — FIX #1/#2: serializa o struct BaseEvent completo (EventType +
// AggregateID + Payload), não apenas e.Payload.
// Eventos concretos sobrescrevem este método com json.Marshal(e) no tipo concreto.
func (e *BaseEvent) ToJSON() ([]byte, error) {
	log.Printf("[DEBUG] Convertendo evento %s para JSON (BaseEvent.ToJSON)", e.EventType)
	return json.Marshal(e)
}

// AggregateFactory cria agregados a partir de eventos
type AggregateFactory interface {
	Create(aggregateType string) (Aggregate, error)
}

// DefaultAggregateFactory fábrica padrão de agregados
type DefaultAggregateFactory struct{}

// Create cria um agregado baseado no tipo
func (f *DefaultAggregateFactory) Create(aggregateType string) (Aggregate, error) {
	log.Printf("[DEBUG] Criando agregado do tipo: %s", aggregateType)

	switch aggregateType {
	case "Estudante":
		return NewEstudante(), nil
	case "Academia":
		return NewAcademia(), nil
	case "Admin":
		return NewAdmin(), nil
	case "Curso":
		return NewCurso(), nil
	case "MateriaDisciplinar":
		return NewMateriaDisciplinar(), nil
	case "SistemaConfig":
		return NewSistemaConfigComID(uuid.Nil), nil
	case "Turma":
		return NewTurma(), nil
	default:
		log.Printf("[ERROR] Tipo de agregado desconhecido: %s", aggregateType)
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
	log.Printf("[DEBUG] Criando novo EventApplier")
	return &EventApplier{
		factory: factory,
	}
}

// BuildFromEvents reconstrói um agregado a partir de eventos
func (ea *EventApplier) BuildFromEvents(
	aggregateType string,
	events []DomainEvent,
) (Aggregate, error) {
	log.Printf("[DEBUG] Reconstruindo agregado %s a partir de %d eventos", aggregateType, len(events))

	if len(events) == 0 {
		log.Printf("[ERROR] Nenhum evento fornecido para reconstruir agregado")
		return nil, fmt.Errorf("nenhum evento fornecido")
	}

	aggregate, err := ea.factory.Create(aggregateType)
	if err != nil {
		log.Printf("[ERROR] Erro ao criar agregado: %v", err)
		return nil, err
	}

	for i, event := range events {
		log.Printf("[DEBUG] Aplicando evento %d/%d: %s", i+1, len(events), event.GetEventType())
		if err := aggregate.Apply(event); err != nil {
			log.Printf("[ERROR] Erro ao aplicar evento %s: %v", event.GetEventType(), err)
			return nil, fmt.Errorf("erro ao aplicar evento %s: %w",
				event.GetEventType(), err)
		}
	}

	log.Printf("[DEBUG] Agregado reconstruído com sucesso. Versão final: %d", aggregate.GetVersion())
	return aggregate, nil
}