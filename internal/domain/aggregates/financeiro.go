package aggregates

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

// Financeiro é o aggregate raiz do módulo financeiro. O estado consultável fica
// nas projeções financeiro_*; o aggregate valida e versiona eventos canônicos no ledger.
type Financeiro struct {
	*BaseAggregate
}

type EventoFinanceiroLedger struct {
	BaseEvent
}

func NewFinanceiro() *Financeiro {
	return &Financeiro{BaseAggregate: &BaseAggregate{ID: uuid.New(), UncommittedEvents: []DomainEvent{}}}
}

func (f *Financeiro) GetType() string { return "Financeiro" }

func (f *Financeiro) Apply(event DomainEvent) error {
	if event.GetAggregateID() == uuid.Nil {
		return fmt.Errorf("aggregate_id financeiro inválido")
	}
	if f.ID == uuid.Nil {
		f.ID = event.GetAggregateID()
	}
	f.Version++
	return nil
}

func (e EventoFinanceiroLedger) ToJSON() ([]byte, error) { return json.Marshal(e) }
