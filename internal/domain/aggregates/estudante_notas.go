package aggregates

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// Constantes de tipo e categoria
// ============================================================================

const (
	TipoEscolar  = "escolar"
	TipoSuperior = "superior"
)

// PeriodosEscolar são os períodos fixos para notas do tipo escolar.
var PeriodosEscolar = []string{"1_trimestre", "2_trimestre", "3_trimestre"}

var categoriasEscolar = map[string]bool{
	"nota_escola":    true,
	"nota_professor": true,
}

var categoriasSuperiorFixas = map[string]bool{
	"nota_pp1":   true,
	"nota_pp2":   true,
	"nota_exame": true,
}

// ============================================================================
// Eventos
// ============================================================================

// NotasRegistradasEvent — emitido ao registrar uma nota pela primeira vez.
// AnoAcademico é sempre preenchido pelo back end.
type NotasRegistradasEvent struct {
	BaseEvent
	CodigoEstudante      string
	CodigoAcademia       string
	AnoLectivo           string
	AnoAcademico         string // inferido pelo back end
	Periodo              string
	MateriaDisciplinarID uuid.UUID
	Tipo                 string // "escolar" | "superior"
	Categoria            string
	Nota                 float64
	Observacao           *string
	RegisteredAt         time.Time
}

func (e *NotasRegistradasEvent) GetPayload() interface{} { return e }

// NotaAtualizadaEvent — emitido ao corrigir uma nota existente.
// Observacao é OBRIGATÓRIA neste evento (justificativa da correção).
type NotaAtualizadaEvent struct {
	BaseEvent
	CodigoEstudante      string
	CodigoAcademia       string
	AnoLectivo           string
	Periodo              string
	MateriaDisciplinarID uuid.UUID
	Tipo                 string
	Categoria            string
	NotaAnterior         float64
	NotaNova             float64
	Observacao           string // obrigatória
	UpdatedAt            time.Time
}

func (e *NotaAtualizadaEvent) GetPayload() interface{} { return e }

// ============================================================================
// Validações internas
// ============================================================================

// validarPeriodoComLista valida se o período informado pertence à lista de
// períodos válidos para o contexto (escolar=3 trimestres fixos; superior=
// períodos do curso).
// periodosValidos nunca deve ser vazio — o handler é responsável por preenchê-lo.
func validarPeriodoComLista(periodo string, periodosValidos []string) error {
	if len(periodosValidos) == 0 {
		return fmt.Errorf("lista de períodos válidos não foi fornecida")
	}
	for _, p := range periodosValidos {
		if p == periodo {
			return nil
		}
	}
	return fmt.Errorf("período '%s' inválido. Períodos aceitos para este contexto: %s",
		periodo, strings.Join(periodosValidos, ", "))
}

func validarCategoria(tipo, categoria string, categoriasAdicionais []string) error {
	switch tipo {
	case TipoEscolar:
		if !categoriasEscolar[categoria] {
			return fmt.Errorf(
				"categoria inválida para tipo escolar. Use: nota_escola, nota_professor",
			)
		}
	case TipoSuperior:
		if categoriasSuperiorFixas[categoria] {
			return nil
		}
		for _, ca := range categoriasAdicionais {
			if ca == categoria {
				return nil
			}
		}
		if !strings.HasPrefix(categoria, "nota_") {
			return fmt.Errorf(
				"categorias adicionais devem seguir o formato nota_[nome]",
			)
		}
		return fmt.Errorf(
			"categoria '%s' não reconhecida. Registre-a como categoria adicional antes de usá-la",
			categoria,
		)
	default:
		return fmt.Errorf("tipo inválido: use 'escolar' ou 'superior'")
	}
	return nil
}

// ============================================================================
// Comandos
// ============================================================================

// RegistrarNota registra uma nota pela primeira vez.
//
// periodosValidos: lista de períodos aceitos para este tipo de nota.
//   - tipo="escolar"  → sempre PeriodosEscolar (handler preenche)
//   - tipo="superior" → curso.Periodos da matéria (handler preenche)
//
// anoAcademico é inferido pelo handler (não vem do request):
//   - estudante no fundamental → estudante.AnoEscolar
//   - estudante no médio/superior → materia.AnosAcademicos[0]
//
// categoriasAdicionais: lista de categorias extras cadastradas pela academia
// (somente relevante para tipo "superior", pode ser nil caso contrário).
func (e *Estudante) RegistrarNota(
	codigoAcademia string,
	anoLectivo string,
	anoAcademico string,
	periodo string,
	materiaDisciplinarID uuid.UUID,
	tipo string,
	categoria string,
	nota float64,
	observacao *string,
	categoriasAdicionais []string,
	periodosValidos []string,
) error {
	if e.CodigoAcademia == nil || *e.CodigoAcademia != codigoAcademia {
		return fmt.Errorf("estudante não pertence a esta academia")
	}
	if strings.TrimSpace(anoAcademico) == "" {
		return fmt.Errorf("ano_academico não pode ser vazio")
	}
	if err := validarPeriodoComLista(periodo, periodosValidos); err != nil {
		return err
	}
	if err := validarCategoria(tipo, categoria, categoriasAdicionais); err != nil {
		return err
	}
	if nota < 0 || nota > 20 {
		return fmt.Errorf("nota deve estar entre 0 e 20")
	}

	event := &NotasRegistradasEvent{
		BaseEvent:            BaseEvent{EventType: "NotasRegistradas", AggregateID: e.ID},
		CodigoEstudante:      e.CodigoEstudante,
		CodigoAcademia:       codigoAcademia,
		AnoLectivo:           anoLectivo,
		AnoAcademico:         anoAcademico,
		Periodo:              periodo,
		MateriaDisciplinarID: materiaDisciplinarID,
		Tipo:                 tipo,
		Categoria:            categoria,
		Nota:                 nota,
		Observacao:           observacao,
		RegisteredAt:         time.Now(),
	}

	e.RaiseEvent(event)
	return e.Apply(event)
}

// AtualizarNota corrige uma nota já registrada.
// observacao é OBRIGATÓRIA — deve justificar a correção.
// periodosValidos: mesmo critério que RegistrarNota.
func (e *Estudante) AtualizarNota(
	codigoAcademia string,
	anoLectivo string,
	periodo string,
	materiaDisciplinarID uuid.UUID,
	tipo string,
	categoria string,
	notaAnterior float64,
	notaNova float64,
	observacao string,
	categoriasAdicionais []string,
	periodosValidos []string,
) error {
	if e.CodigoAcademia == nil || *e.CodigoAcademia != codigoAcademia {
		return fmt.Errorf("estudante não pertence a esta academia")
	}
	if strings.TrimSpace(observacao) == "" {
		return fmt.Errorf("observacao é obrigatória ao atualizar uma nota")
	}
	if err := validarPeriodoComLista(periodo, periodosValidos); err != nil {
		return err
	}
	if err := validarCategoria(tipo, categoria, categoriasAdicionais); err != nil {
		return err
	}
	if notaNova < 0 || notaNova > 20 {
		return fmt.Errorf("nota deve estar entre 0 e 20")
	}

	event := &NotaAtualizadaEvent{
		BaseEvent:            BaseEvent{EventType: "NotaAtualizada", AggregateID: e.ID},
		CodigoEstudante:      e.CodigoEstudante,
		CodigoAcademia:       codigoAcademia,
		AnoLectivo:           anoLectivo,
		Periodo:              periodo,
		MateriaDisciplinarID: materiaDisciplinarID,
		Tipo:                 tipo,
		Categoria:            categoria,
		NotaAnterior:         notaAnterior,
		NotaNova:             notaNova,
		Observacao:           observacao,
		UpdatedAt:            time.Now(),
	}

	e.RaiseEvent(event)
	return e.Apply(event)
}

// ============================================================================
// Apply handlers
// ============================================================================

func (e *Estudante) applyNotasRegistradas(event DomainEvent) error {
	// O agregado Estudante não mantém estado de notas em memória —
	// elas são gerenciadas pela projeção. Apenas deixa a versão incrementar.
	return nil
}

func (e *Estudante) applyNotaAtualizada(event DomainEvent) error {
	return nil
}