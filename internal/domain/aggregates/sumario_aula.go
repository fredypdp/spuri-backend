package aggregates

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type SumarioAula struct{ BaseAggregate }

func NewSumarioAula() *SumarioAula {
	return &SumarioAula{BaseAggregate: BaseAggregate{ID: uuid.New(), UncommittedEvents: []DomainEvent{}}}
}
func (s *SumarioAula) GetType() string               { return "SumarioAula" }
func (s *SumarioAula) Apply(event DomainEvent) error { return nil }

type SumarioAulaCriadoEvent struct {
	BaseEvent
	AcademiaID     uuid.UUID
	CodigoAcademia string
	SumarioTitulo  string
	Descricao      *string
	Periodo        string
	AnoAcademico   int
	Nivel          string
	Type           string
	CursoID        *uuid.UUID
	MateriaID      uuid.UUID
	CriadoPor      uuid.UUID
	CriadoEm       time.Time
	AtualizadoEm   time.Time
}

func (e *SumarioAulaCriadoEvent) GetPayload() interface{} { return e }
func (e *SumarioAulaCriadoEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type SumarioAulaAtualizadoEvent struct {
	BaseEvent
	SumarioTitulo *string
	Descricao     *string
	Periodo       *string
	AnoAcademico  *int
	CursoID       *uuid.UUID
	MateriaID     *uuid.UUID
	AtualizadoPor uuid.UUID
	AtualizadoEm  time.Time
}

func (e *SumarioAulaAtualizadoEvent) GetPayload() interface{} { return e }
func (e *SumarioAulaAtualizadoEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type SumarioAulaDesativadoEvent struct {
	BaseEvent
	DesativadoPor uuid.UUID
	DesativadoEm  time.Time
}

func (e *SumarioAulaDesativadoEvent) GetPayload() interface{} { return e }
func (e *SumarioAulaDesativadoEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

func (s *SumarioAula) Criar(academiaID uuid.UUID, codigoAcademia, titulo string, descricao *string, periodo string, anoAcademico int, nivel, typ string, cursoID *uuid.UUID, materiaID, criadoPor uuid.UUID) error {
	if strings.TrimSpace(titulo) == "" || len(strings.TrimSpace(titulo)) < 3 || len(titulo) > 200 {
		return fmt.Errorf("sumario_titulo deve ter entre 3 e 200 caracteres")
	}
	if materiaID == uuid.Nil {
		return fmt.Errorf("materia_id é obrigatório")
	}
	now := time.Now()
	ev := &SumarioAulaCriadoEvent{BaseEvent: BaseEvent{EventType: "SumarioAulaCriado", AggregateID: s.ID}, AcademiaID: academiaID, CodigoAcademia: codigoAcademia, SumarioTitulo: strings.TrimSpace(titulo), Descricao: descricao, Periodo: periodo, AnoAcademico: anoAcademico, Nivel: nivel, Type: typ, CursoID: cursoID, MateriaID: materiaID, CriadoPor: criadoPor, CriadoEm: now, AtualizadoEm: now}
	s.RaiseEvent(ev)
	return s.Apply(ev)
}
func (s *SumarioAula) Atualizar(titulo, descricao, periodo *string, anoAcademico *int, cursoID *uuid.UUID, materiaID *uuid.UUID, user uuid.UUID) error {
	if titulo == nil && descricao == nil && periodo == nil && anoAcademico == nil && cursoID == nil && materiaID == nil {
		return fmt.Errorf("ao menos um campo deve ser fornecido para atualização")
	}
	if titulo != nil && (strings.TrimSpace(*titulo) == "" || len(strings.TrimSpace(*titulo)) < 3 || len(*titulo) > 200) {
		return fmt.Errorf("sumario_titulo deve ter entre 3 e 200 caracteres")
	}
	ev := &SumarioAulaAtualizadoEvent{BaseEvent: BaseEvent{EventType: "SumarioAulaAtualizado", AggregateID: s.ID}, SumarioTitulo: titulo, Descricao: descricao, Periodo: periodo, AnoAcademico: anoAcademico, CursoID: cursoID, MateriaID: materiaID, AtualizadoPor: user, AtualizadoEm: time.Now()}
	s.RaiseEvent(ev)
	return s.Apply(ev)
}
func (s *SumarioAula) Desativar(user uuid.UUID) error {
	ev := &SumarioAulaDesativadoEvent{BaseEvent: BaseEvent{EventType: "SumarioAulaDesativado", AggregateID: s.ID}, DesativadoPor: user, DesativadoEm: time.Now()}
	s.RaiseEvent(ev)
	return s.Apply(ev)
}
