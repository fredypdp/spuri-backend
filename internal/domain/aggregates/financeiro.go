package aggregates

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

// Financeiro keeps no mutable business state; its read state is rebuilt solely
// from immutable events, using the same repository guarantees as all aggregates.
type Financeiro struct{ *BaseAggregate }
type EventoFinanceiro struct{ BaseEvent }

// Event names owned by the Financeiro ledger aggregate. CobrancaAppyPayCancelada
// is a local Spuri cancellation; CobrancaAppyPayConflitoPosCancelamento records
// a provider Success observed after that definitive local cancellation.
const (
	CobrancaAppyPayCancelada               = "CobrancaAppyPayCancelada"
	CobrancaAppyPayConflitoPosCancelamento = "CobrancaAppyPayConflitoPosCancelamento"
)

func NewFinanceiro() *Financeiro { return NewFinanceiroWithID(uuid.New()) }
func NewFinanceiroWithID(id uuid.UUID) *Financeiro {
	return &Financeiro{BaseAggregate: &BaseAggregate{ID: id, UncommittedEvents: []DomainEvent{}}}
}
func (f *Financeiro) GetType() string { return "Financeiro" }
func (f *Financeiro) Registrar(eventType string, payload map[string]any) {
	f.RaiseEvent(&EventoFinanceiro{BaseEvent: BaseEvent{EventType: eventType, AggregateID: f.ID, Payload: payload}})
}
func (f *Financeiro) Apply(event DomainEvent) error {
	if event.GetAggregateID() == uuid.Nil {
		return fmt.Errorf("aggregate financeiro inválido")
	}
	if f.ID == uuid.Nil {
		f.ID = event.GetAggregateID()
	}
	f.Version++
	return nil
}
func (e EventoFinanceiro) ToJSON() ([]byte, error) { return json.Marshal(e) }
