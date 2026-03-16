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

// chaveFalta retorna a chave composta usada para detectar duplicatas de falta no aggregate.
// Formato: "<anoLectivo>_<data AAAA-MM-DD>_<materiaID>"
// Deve coincidir com a constraint UNIQUE(codigo_estudante, codigo_academia, data, materia_disciplinar_id)
// do banco — academia e estudante já são implícitos pelo aggregate carregado.
func chaveFalta(anoLectivo string, data time.Time, materiaID uuid.UUID) string {
	return anoLectivo + "_" + data.UTC().Format("2006-01-02") + "_" + materiaID.String()
}

// ============================================================================
// Método de comando: RegistrarFalta
// ============================================================================

// RegistrarFalta registra uma falta do estudante em uma matéria.
//
// anoAcademico é inferido pelo handler:
//   - Estudante no fundamental (AnoEscolar != nil) → AnoEscolar do estudante
//   - Estudante no médio/superior                  → Nivel[0] da matéria
//
// FIX FALTA-AGG-01: guard de duplicata via FaltasRegistradasPorChave antes de
// emitir o evento. Evita double-submit e retorna erro de negócio claro em vez
// de deixar a violação de unique constraint 23505 explodir na projeção.
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

	// FIX FALTA-AGG-01: detectar duplicata via estado do aggregate.
	// Evita double-submit e a violação de unique constraint 23505 na projeção,
	// retornando um erro de negócio claro ao invés de um 500.
	chave := chaveFalta(anoLectivo, data, materiaDisciplinarID)
	if e.FaltasRegistradasPorChave != nil && e.FaltasRegistradasPorChave[chave] {
		return fmt.Errorf(
			"falta já registrada para data '%s' e materia '%s' no ano letivo '%s'",
			data.UTC().Format("2006-01-02"), materiaDisciplinarID, anoLectivo,
		)
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

// applyFaltasRegistradas — FIX FALTA-AGG-01: mantém FaltasRegistradasPorChave
// em estado para que RegistrarFalta possa detectar duplicatas sem depender da projeção.
// A chave usada aqui corresponde à constraint UNIQUE do banco.
func (e *Estudante) applyFaltasRegistradas(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applyFaltasRegistradas: marshal error: %w", err)
	}
	var ev FaltasRegistradasEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyFaltasRegistradas: unmarshal error (payload corrompido): %w", err)
	}

	if e.FaltasRegistradasPorChave == nil {
		e.FaltasRegistradasPorChave = make(map[string]bool)
	}
	chave := chaveFalta(ev.AnoLectivo, ev.Data, ev.MateriaDisciplinarID)
	e.FaltasRegistradasPorChave[chave] = true
	return nil
}

// applyFaltaAtualizada — aggregate não mantém os valores da falta em estado;
// estado de faltas é gerenciado exclusivamente pela projeção.
func (e *Estudante) applyFaltaAtualizada(_ DomainEvent) error {
	return nil
}

// applyFaltaDeletada — não remove a chave do mapa intencionalmente.
// Uma falta deletada não deve ser re-registrada com a mesma combinação
// de chave (mesma data + matéria + ano letivo). A projeção controla o soft delete.
func (e *Estudante) applyFaltaDeletada(_ DomainEvent) error {
	return nil
}