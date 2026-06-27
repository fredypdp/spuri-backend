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

// SetID permite setar o ID do agregado após a criação pela factory.
// Usado em BuildFromEvents para corrigir o ID quando a factory cria o
// agregado com uuid.Nil (ex: SistemaConfig) e o apply handler do primeiro
// evento ainda não foi executado.
//
// FIX B-02: resolve o caso em que DefaultAggregateFactory.Create retorna
// SistemaConfig com ID = uuid.Nil antes de qualquer evento ser aplicado.
func (a *BaseAggregate) SetID(id uuid.UUID) {
	a.ID = id
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

// ToJSON — FIX B-01: serializa o struct BaseEvent completo (EventType +
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

// Create cria um agregado baseado no tipo.
// NOTA B-02: SistemaConfig é criado com uuid.Nil intencionalmente — o ID real
// é injetado via BuildFromEvents.SetID() antes da aplicação dos eventos.
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
	case "Turma":
		return NewTurma(), nil
	case "SolicitacaoMatricula":
		return NewSolicitacaoMatricula(), nil
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

// idSetter é uma interface local para injetar o ID no agregado antes
// de aplicar os eventos. Satisfeita por todos os tipos que embarcam *BaseAggregate.
type idSetter interface {
	SetID(uuid.UUID)
}

// BuildFromEvents reconstrói um agregado a partir de eventos.
//
// FIX B-02: antes de aplicar o primeiro evento, injeta o AggregateID do
// primeiro evento no agregado via SetID — corrige o caso em que a factory
// cria SistemaConfig com ID = uuid.Nil.
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

	// FIX B-02: injeta o ID real antes de aplicar qualquer evento.
	// Isso evita que comandos emitidos sobre o agregado reconstruído
	// gravem AggregateID = uuid.Nil no ledger.
	if aggregate.GetID() == uuid.Nil {
		if setter, ok := aggregate.(idSetter); ok {
			setter.SetID(events[0].GetAggregateID())
			log.Printf("[DEBUG] ID do agregado %s injetado via SetID: %s",
				aggregateType, events[0].GetAggregateID())
		}
	}

	for i, event := range events {
		log.Printf("[DEBUG] Aplicando evento %d/%d: %s", i+1, len(events), event.GetEventType())
		if err := aggregate.Apply(event); err != nil {
			log.Printf("[ERROR] Erro ao aplicar evento %s: %v", event.GetEventType(), err)
			return nil, fmt.Errorf("erro ao aplicar evento %s: %w",
				event.GetEventType(), err)
		}
	}

	log.Printf("[DEBUG] Agregado %s reconstruído com sucesso. Versão: %d",
		aggregateType, aggregate.GetVersion())
	return aggregate, nil
}