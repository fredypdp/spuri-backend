package aggregates

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"strings"
	"time"
)

type Sumario struct {
	BaseAggregate
	CodigoAcademia, SumarioTitulo      string
	Descricao                          *string
	Periodo, AnoAcademico, Nivel, Type string
	CursoID                            *uuid.UUID
	MateriaID, CriadoPor               uuid.UUID
	Deletado                           bool
}

func NewSumario() *Sumario {
	return &Sumario{BaseAggregate: BaseAggregate{ID: uuid.New(), UncommittedEvents: []DomainEvent{}}}
}
func (s *Sumario) GetType() string { return "Sumario" }

type SumarioCriadoEvent struct {
	BaseEvent
	CodigoAcademia string     `json:"codigo_academia"`
	SumarioTitulo  string     `json:"sumario_titulo"`
	Descricao      *string    `json:"descricao,omitempty"`
	Periodo        string     `json:"periodo"`
	AnoAcademico   string     `json:"ano_academico"`
	Nivel          string     `json:"nivel"`
	Type           string     `json:"type"`
	CursoID        *uuid.UUID `json:"curso_id,omitempty"`
	MateriaID      uuid.UUID  `json:"materia_id"`
	CriadoPor      uuid.UUID  `json:"criado_por"`
	CriadoEm       time.Time  `json:"criado_em"`
}

func (e *SumarioCriadoEvent) GetPayload() interface{} { return e }
func (e *SumarioCriadoEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type SumarioDadosAtualizadosEvent struct {
	BaseEvent
	SumarioTitulo *string   `json:"sumario_titulo,omitempty"`
	Descricao     *string   `json:"descricao,omitempty"`
	AtualizadoPor uuid.UUID `json:"atualizado_por"`
	AtualizadoEm  time.Time `json:"atualizado_em"`
}

func (e *SumarioDadosAtualizadosEvent) GetPayload() interface{} { return e }
func (e *SumarioDadosAtualizadosEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type SumarioDeletadoEvent struct {
	BaseEvent
	DeletadoPor uuid.UUID `json:"deletado_por"`
	DeletadoEm  time.Time `json:"deletado_em"`
}

func (e *SumarioDeletadoEvent) GetPayload() interface{} { return e }
func (e *SumarioDeletadoEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }
func (s *Sumario) Criar(t string, d *string, ca, tipo, nivel, periodo, ano string, curso *uuid.UUID, materia, por uuid.UUID) error {
	t = strings.TrimSpace(t)
	if len(t) < 3 || len(t) > 200 {
		return fmt.Errorf("sumario_titulo deve ter entre 3 e 200 caracteres")
	}
	if strings.TrimSpace(ca) == "" {
		return fmt.Errorf("codigo_academia é obrigatório")
	}
	if tipo != TipoEscolar && tipo != TipoSuperior {
		return fmt.Errorf("type deve ser 'escolar' ou 'superior'")
	}
	if nivel != "fundamental" && nivel != "medio" && nivel != "superior" {
		return fmt.Errorf("nivel deve ser 'fundamental', 'medio' ou 'superior'")
	}
	esperado := TipoSuperior
	if nivel != "superior" {
		esperado = TipoEscolar
	}
	if tipo != esperado {
		return fmt.Errorf("type (%s) incoerente com nivel (%s)", tipo, nivel)
	}
	if strings.TrimSpace(periodo) == "" || strings.TrimSpace(ano) == "" {
		return fmt.Errorf("periodo e ano_academico são obrigatórios")
	}
	if nivel == "fundamental" && curso != nil {
		return fmt.Errorf("sumário de matéria fundamental não deve ter curso_id")
	}
	if nivel != "fundamental" && curso == nil {
		return fmt.Errorf("sumário de matéria %s exige curso_id", nivel)
	}
	if materia == uuid.Nil || por == uuid.Nil {
		return fmt.Errorf("materia_id e criado_por são obrigatórios")
	}
	if d != nil {
		x := strings.TrimSpace(*d)
		if len(x) > 2000 {
			return fmt.Errorf("descricao deve ter no máximo 2000 caracteres")
		}
		d = &x
	}
	e := &SumarioCriadoEvent{BaseEvent: BaseEvent{EventType: "SumarioCriado", AggregateID: s.ID}, CodigoAcademia: ca, SumarioTitulo: t, Descricao: d, Periodo: periodo, AnoAcademico: ano, Nivel: nivel, Type: tipo, CursoID: curso, MateriaID: materia, CriadoPor: por, CriadoEm: time.Now().UTC()}
	s.RaiseEvent(e)
	return s.Apply(e)
}
func (s *Sumario) AtualizarDados(t, d *string, por uuid.UUID) error {
	if s.Deletado {
		return fmt.Errorf("não é possível atualizar um sumário deletado")
	}
	if t == nil && d == nil {
		return fmt.Errorf("nenhum dado para atualizar")
	}
	if t != nil {
		x := strings.TrimSpace(*t)
		if len(x) < 3 || len(x) > 200 {
			return fmt.Errorf("sumario_titulo deve ter entre 3 e 200 caracteres")
		}
		t = &x
	}
	if d != nil {
		x := strings.TrimSpace(*d)
		if len(x) > 2000 {
			return fmt.Errorf("descricao deve ter no máximo 2000 caracteres")
		}
		d = &x
	}
	if por == uuid.Nil {
		return fmt.Errorf("atualizado_por é obrigatório")
	}
	e := &SumarioDadosAtualizadosEvent{BaseEvent: BaseEvent{EventType: "SumarioDadosAtualizados", AggregateID: s.ID}, SumarioTitulo: t, Descricao: d, AtualizadoPor: por, AtualizadoEm: time.Now().UTC()}
	s.RaiseEvent(e)
	return s.Apply(e)
}
func (s *Sumario) Deletar(por uuid.UUID) error {
	if s.Deletado {
		return fmt.Errorf("sumário já está deletado")
	}
	if por == uuid.Nil {
		return fmt.Errorf("deletado_por é obrigatório")
	}
	e := &SumarioDeletadoEvent{BaseEvent: BaseEvent{EventType: "SumarioDeletado", AggregateID: s.ID}, DeletadoPor: por, DeletadoEm: time.Now().UTC()}
	s.RaiseEvent(e)
	return s.Apply(e)
}
func (s *Sumario) Apply(e DomainEvent) error {
	b, _ := json.Marshal(e.GetPayload())
	switch e.GetEventType() {
	case "SumarioCriado":
		var p SumarioCriadoEvent
		if err := json.Unmarshal(b, &p); err != nil {
			return err
		}
		s.CodigoAcademia, s.SumarioTitulo, s.Descricao, s.Periodo, s.AnoAcademico, s.Nivel, s.Type, s.CursoID, s.MateriaID, s.CriadoPor = p.CodigoAcademia, p.SumarioTitulo, p.Descricao, p.Periodo, p.AnoAcademico, p.Nivel, p.Type, p.CursoID, p.MateriaID, p.CriadoPor
	case "SumarioDadosAtualizados":
		var p SumarioDadosAtualizadosEvent
		if err := json.Unmarshal(b, &p); err != nil {
			return err
		}
		if p.SumarioTitulo != nil {
			s.SumarioTitulo = *p.SumarioTitulo
		}
		if p.Descricao != nil {
			s.Descricao = p.Descricao
		}
	case "SumarioDeletado":
		s.Deletado = true
	default:
		return fmt.Errorf("evento desconhecido para Sumario: %s", e.GetEventType())
	}
	return nil
}
