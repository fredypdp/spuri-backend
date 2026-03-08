package aggregates

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

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
	if strings.TrimSpace(faltaID) == "" {
		return fmt.Errorf("falta_id inválido")
	}
	if strings.TrimSpace(motivo) == "" {
		return fmt.Errorf("motivo é obrigatório para deletar falta")
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
// Eventos
// ============================================================================

// FaltaAtualizadaEvent — emitido ao corrigir uma falta existente.
// Todos os campos de dados são ponteiros: nil = não alterar.
// AtualizadoPor identifica o executor no payload (self-contained para auditoria).
type FaltaAtualizadaEvent struct {
	BaseEvent
	CodigoEstudante      string
	CodigoAcademia       string
	FaltaID              string     // ID da linha em projection_faltas
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
// EventType: "FaltaDeletada".
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
// Apply handlers
// ============================================================================

func (e *Estudante) applyFaltaDeletada(_ DomainEvent) error {
	// O aggregate Estudante não mantém faltas em estado — gerenciado pela projeção.
	return nil
}