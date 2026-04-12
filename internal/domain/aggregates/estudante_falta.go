package aggregates

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// Eventos
// ============================================================================

// FaltasRegistradasEvent — emitido ao registrar uma falta.
// EventType: "FaltasRegistradas" (canônico).
type FaltasRegistradasEvent struct {
	BaseEvent
	CodigoEstudante      string
	CodigoAcademia       string
	AnoLectivo           string
	AnoAcademico         string
	Data                 time.Time
	MateriaDisciplinarID uuid.UUID
	Quantidade           int
	Observacao           *string
	RegisteredAt         time.Time
}

func (e *FaltasRegistradasEvent) GetPayload() interface{} { return e }
func (e *FaltasRegistradasEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

// FaltaAtualizadaEvent — emitido ao corrigir uma falta existente.
// EventType: "FaltaAtualizada" (canônico).
// Todos os campos alteráveis são ponteiros — nil = não alterar.
type FaltaAtualizadaEvent struct {
	BaseEvent
	CodigoEstudante      string
	CodigoAcademia       string
	FaltaID              string
	Data                 *time.Time
	MateriaDisciplinarID *uuid.UUID
	Quantidade           *int
	Observacao           *string
	AtualizadoPor        uuid.UUID
	UpdatedAt            time.Time
}

func (e *FaltaAtualizadaEvent) GetPayload() interface{} { return e }
func (e *FaltaAtualizadaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

// FaltaDeletadaEvent — emitido ao fazer soft delete de uma falta.
// EventType: "FaltaDeletada" (canônico).
// Motivo é OBRIGATÓRIO para auditoria self-contained no ledger.
type FaltaDeletadaEvent struct {
	BaseEvent
	FaltaID         string
	CodigoEstudante string
	CodigoAcademia  string
	Motivo          string
	DeletadoPor     uuid.UUID
	DeletedAt       time.Time
}

func (e *FaltaDeletadaEvent) GetPayload() interface{} { return e }
func (e *FaltaDeletadaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

// ============================================================================
// Helpers internos
// ============================================================================

// ============================================================================
// Método de comando: RegistrarFalta
// ============================================================================

// RegistrarFalta registra uma falta do estudante em uma matéria.
//
// anoAcademico é inferido pelo handler:
//   - Estudante no fundamental (AnoEscolar != nil) → AnoEscolar do estudante
//   - Estudante no médio/superior                  → Nivel[0] da matéria
func (e *Estudante) RegistrarFalta(
	codigoAcademia string,
	anoLectivo string,
	anoAcademico string,
	data time.Time,
	materiaDisciplinarID uuid.UUID,
	quantidade int,
	observacao *string,
) error {
	if e.CodigoAcademia == nil || *e.CodigoAcademia != codigoAcademia {
		return fmt.Errorf("estudante não pertence a esta academia")
	}
	if quantidade <= 0 {
		return fmt.Errorf("quantidade deve ser maior que zero")
	}

	event := &FaltasRegistradasEvent{
		BaseEvent:            BaseEvent{EventType: "FaltasRegistradas", AggregateID: e.ID},
		CodigoEstudante:      e.CodigoEstudante,
		CodigoAcademia:       codigoAcademia,
		AnoLectivo:           anoLectivo,
		AnoAcademico:         anoAcademico,
		Data:                 data,
		MateriaDisciplinarID: materiaDisciplinarID,
		Quantidade:           quantidade,
		Observacao:           observacao,
		RegisteredAt:         time.Now(),
	}

	e.RaiseEvent(event)
	return e.Apply(event)
}

// ============================================================================
// Método de comando: AtualizarFalta
// ============================================================================

// AtualizarFalta corrige uma falta já registrada.
// Permite alterar: Data, MateriaDisciplinarID, Quantidade e Observacao.
// Todos os campos são ponteiros — nil = não alterar.
// AtualizadoPor identifica o executor no payload do evento (self-contained).
func (e *Estudante) AtualizarFalta(
	codigoAcademia string,
	faltaID string,
	data *time.Time,
	materiaDisciplinarID *uuid.UUID,
	quantidade *int,
	observacao *string,
	atualizadoPor uuid.UUID,
) error {
	if e.CodigoAcademia == nil || *e.CodigoAcademia != codigoAcademia {
		return fmt.Errorf("estudante não pertence a esta academia")
	}
	if data == nil && materiaDisciplinarID == nil && quantidade == nil && observacao == nil {
		return fmt.Errorf("ao menos um campo deve ser fornecido para atualização")
	}
	if quantidade != nil && *quantidade <= 0 {
		return fmt.Errorf("quantidade deve ser maior que zero")
	}

	event := &FaltaAtualizadaEvent{
		BaseEvent:            BaseEvent{EventType: "FaltaAtualizada", AggregateID: e.ID},
		CodigoEstudante:      e.CodigoEstudante,
		CodigoAcademia:       codigoAcademia,
		FaltaID:              faltaID,
		Data:                 data,
		MateriaDisciplinarID: materiaDisciplinarID,
		Quantidade:           quantidade,
		Observacao:           observacao,
		AtualizadoPor:        atualizadoPor,
		UpdatedAt:            time.Now(),
	}

	e.RaiseEvent(event)
	return e.Apply(event)
}

// ============================================================================
// Método de comando: DeletarFalta
// ============================================================================

// DeletarFalta faz soft delete de uma falta via event sourcing.
// motivo é OBRIGATÓRIO para auditoria self-contained no ledger.
func (e *Estudante) DeletarFalta(
	codigoAcademia string,
	faltaID string,
	motivo string,
	deletadoPor uuid.UUID,
) error {
	if e.CodigoAcademia == nil || *e.CodigoAcademia != codigoAcademia {
		return fmt.Errorf("estudante não pertence a esta academia")
	}
	if strings.TrimSpace(motivo) == "" {
		return fmt.Errorf("motivo é obrigatório para deletar uma falta")
	}

	event := &FaltaDeletadaEvent{
		BaseEvent:       BaseEvent{EventType: "FaltaDeletada", AggregateID: e.ID},
		FaltaID:         faltaID,
		CodigoEstudante: e.CodigoEstudante,
		CodigoAcademia:  codigoAcademia,
		Motivo:          motivo,
		DeletadoPor:     deletadoPor,
		DeletedAt:       time.Now(),
	}

	e.RaiseEvent(event)
	return e.Apply(event)
}

// ============================================================================
// Apply handlers
// ============================================================================

// applyFaltasRegistradas — aggregate não mantém estado derivado para faltas.
// A projeção persiste cada registro sem restrição de unicidade por data/matéria.
func (e *Estudante) applyFaltasRegistradas(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applyFaltasRegistradas: marshal error: %w", err)
	}
	var ev FaltasRegistradasEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyFaltasRegistradas: unmarshal error (payload corrompido): %w", err)
	}

	_ = ev
	return nil
}

// applyFaltaAtualizada — aggregate não mantém os valores da falta em estado;
// estado de faltas é gerenciado exclusivamente pela projeção.
func (e *Estudante) applyFaltaAtualizada(_ DomainEvent) error {
	return nil
}

// applyFaltaDeletada — sem estado derivado adicional no aggregate.
func (e *Estudante) applyFaltaDeletada(_ DomainEvent) error {
	return nil
}
