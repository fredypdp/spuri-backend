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
	RegistradoPor        uuid.UUID
}

func (e *FaltasRegistradasEvent) GetPayload() interface{} { return e }
func (e *FaltasRegistradasEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type FaltaCorrigidaEvent struct {
	BaseEvent
	FaltaAnteriorID      uuid.UUID
	CodigoAcademia       string
	AnoLectivo           string
	Data                 time.Time
	MateriaDisciplinarID uuid.UUID
	NovaQuantidade       int
	NovaObservacao       *string
	Motivo               string
	CorrigidoPor         uuid.UUID
	CorrigidoEm          time.Time
}

func (e *FaltaCorrigidaEvent) GetPayload() interface{} { return e }
func (e *FaltaCorrigidaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

// ============================================================================
// Helpers internos
// ============================================================================

func chaveFalta(codigoEstudante, codigoAcademia, anoLectivo string, data time.Time, materiaID uuid.UUID) string {
	return codigoEstudante + "_" + codigoAcademia + "_" + anoLectivo + "_" + data.Format("2006-01-02") + "_" + materiaID.String()
}

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
	registradoPor uuid.UUID,
) error {
	if e.CodigoAcademia == nil || *e.CodigoAcademia != codigoAcademia {
		return fmt.Errorf("estudante não pertence a esta academia")
	}
	if quantidade <= 0 {
		return fmt.Errorf("quantidade deve ser maior que zero")
	}
	chave := chaveFalta(e.CodigoEstudante, codigoAcademia, anoLectivo, data, materiaDisciplinarID)
	if e.FaltasRegistradasPorChave != nil && e.FaltasRegistradasPorChave[chave] {
		return fmt.Errorf(
			"falta já registrada para data '%s', materia '%s' no ano letivo '%s' para academia '%s'",
			data.Format("2006-01-02"), materiaDisciplinarID, anoLectivo, codigoAcademia,
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
		RegistradoPor:        registradoPor,
	}

	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) CorrigirFalta(faltaAnteriorID uuid.UUID, codigoAcademia, anoLectivo string, data time.Time, materiaID uuid.UUID, novaQuantidade int, novaObservacao *string, motivo string, corrigidoPor uuid.UUID) error {
	if e.CodigoAcademia == nil || *e.CodigoAcademia != codigoAcademia {
		return fmt.Errorf("estudante não pertence a esta academia")
	}
	if faltaAnteriorID == uuid.Nil {
		return fmt.Errorf("id da falta original inválido")
	}
	if strings.TrimSpace(motivo) == "" {
		return fmt.Errorf("motivo da correção é obrigatório")
	}
	if novaQuantidade <= 0 {
		return fmt.Errorf("quantidade deve ser maior que zero")
	}
	chave := chaveFalta(e.CodigoEstudante, codigoAcademia, anoLectivo, data, materiaID)
	if e.FaltasRegistradasPorChave == nil || !e.FaltasRegistradasPorChave[chave] {
		return fmt.Errorf("falta original não encontrada para correção")
	}
	event := &FaltaCorrigidaEvent{BaseEvent: BaseEvent{EventType: "FaltaCorrigida", AggregateID: e.ID}, FaltaAnteriorID: faltaAnteriorID, CodigoAcademia: codigoAcademia, AnoLectivo: anoLectivo, Data: data, MateriaDisciplinarID: materiaID, NovaQuantidade: novaQuantidade, NovaObservacao: novaObservacao, Motivo: strings.TrimSpace(motivo), CorrigidoPor: corrigidoPor, CorrigidoEm: time.Now()}
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
	if e.FaltasRegistradasPorChave == nil {
		e.FaltasRegistradasPorChave = make(map[string]bool)
	}
	chave := chaveFalta(ev.CodigoEstudante, ev.CodigoAcademia, ev.AnoLectivo, ev.Data, ev.MateriaDisciplinarID)
	e.FaltasRegistradasPorChave[chave] = true
	return nil
}

func (e *Estudante) applyFaltaCorrigida(event DomainEvent) error { return nil }
