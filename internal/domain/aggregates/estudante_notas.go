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
// EventType: "NotasRegistradas" (este é o nome canônico — a projeção deve
// escutar exatamente este string).
// AnoAcademico é sempre preenchido pelo back end (nunca pelo cliente).
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
// EventType: "NotaAtualizada" (este é o nome canônico — a projeção deve
// escutar exatamente este string).
// Observacao é OBRIGATÓRIA neste evento (justificativa da correção).
// A identificação da nota na projeção é feita pela chave natural composta:
// (CodigoEstudante, AnoLectivo, Periodo, MateriaDisciplinarID, Tipo, Categoria).
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
	Observacao           string // obrigatória — justificativa da correção
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
	return fmt.Errorf(
		"período '%s' inválido. Aceitos: %s",
		periodo, strings.Join(periodosValidos, ", "),
	)
}

// validarCategoria valida se a categoria pertence ao conjunto permitido
// para o tipo de nota.
//   - tipo="escolar"  → categoriasEscolar (fixas)
//   - tipo="superior" → categoriasSuperiorFixas ∪ categoriasAdicionais
func validarCategoria(tipo, categoria string, categoriasAdicionais []string) error {
	switch tipo {
	case TipoEscolar:
		if !categoriasEscolar[categoria] {
			validas := make([]string, 0, len(categoriasEscolar))
			for k := range categoriasEscolar {
				validas = append(validas, k)
			}
			return fmt.Errorf(
				"categoria '%s' inválida para notas escolares. Aceitas: %s",
				categoria, strings.Join(validas, ", "),
			)
		}
	case TipoSuperior:
		if categoriasSuperiorFixas[categoria] {
			return nil
		}
		for _, extra := range categoriasAdicionais {
			if extra == categoria {
				return nil
			}
		}
		return fmt.Errorf(
			"categoria '%s' inválida para notas superiores. "+
				"Categorias fixas: nota_pp1, nota_pp2, nota_exame. "+
				"Categorias adicionais cadastradas: %s",
			categoria, strings.Join(categoriasAdicionais, ", "),
		)
	default:
		return fmt.Errorf("tipo de nota desconhecido: '%s'", tipo)
	}
	return nil
}

// ============================================================================
// Método de comando: RegistrarNota
// ============================================================================

// RegistrarNota registra uma nota do estudante em uma matéria.
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

	// EventType = "NotasRegistradas"
	// A projeção NotasProjection.Handle() escuta este exato string.
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

// ============================================================================
// Método de comando: AtualizarNota
// ============================================================================

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

	// EventType = "NotaAtualizada"
	// A projeção NotasProjection.Handle() escuta este exato string.
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

// applyNotasRegistradas — o aggregate Estudante não mantém estado de notas
// em memória. As notas vivem exclusivamente na NotasProjection.
// Este handler existe apenas para que o Apply() não retorne erro desconhecido.
func (e *Estudante) applyNotasRegistradas(event DomainEvent) error {
	// Intencional: nenhum estado do aggregate é alterado.
	// A NotasProjection é a única responsável por persistir notas.
	return nil
}

// applyNotaAtualizada — idem: nenhum estado do aggregate é alterado.
func (e *Estudante) applyNotaAtualizada(event DomainEvent) error {
	// Intencional: nenhum estado do aggregate é alterado.
	return nil
}