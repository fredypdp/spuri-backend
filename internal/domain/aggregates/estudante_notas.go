package aggregates

import (
	"encoding/json"
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

var categoriasEscolarFixas = map[string]bool{
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
// EventType: "NotasRegistradas" (canônico — a projeção escuta exatamente este string).
// AnoAcademico é sempre preenchido pelo back end (nunca pelo cliente).
//
// FIX E-06: campo RegistradoPor adicionado para auditoria self-contained.
type NotasRegistradasEvent struct {
	BaseEvent
	CodigoEstudante      string
	CodigoAcademia       string
	AnoLectivo           string
	AnoAcademico         string    // inferido pelo back end
	Periodo              string
	MateriaDisciplinarID uuid.UUID
	Tipo                 string    // "escolar" | "superior"
	Categoria            string
	Nota                 float64
	Observacao           *string
	RegisteredAt         time.Time
	// FIX E-06: UUID do usuário que registrou a nota. uuid.Nil = legado/não preenchido.
	RegistradoPor uuid.UUID
}

func (e *NotasRegistradasEvent) GetPayload() interface{} { return e }
func (e *NotasRegistradasEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

// NotaAtualizadaEvent — emitido ao corrigir uma nota existente.
// EventType: "NotaAtualizada" (canônico).
// Observacao é OBRIGATÓRIA neste evento (justificativa da correção).
//
// FIX E-07: campo AtualizadoPor adicionado para auditoria self-contained.
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
	Observacao           string    // obrigatória — justificativa da correção
	UpdatedAt            time.Time
	// FIX E-07: UUID do usuário que corrigiu a nota. uuid.Nil = legado/não preenchido.
	AtualizadoPor uuid.UUID
}

func (e *NotaAtualizadaEvent) GetPayload() interface{} { return e }
func (e *NotaAtualizadaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

// NotaDeletadaEvent — emitido ao fazer soft delete de uma nota.
// EventType: "NotaDeletada".
// Motivo é OBRIGATÓRIO para auditoria self-contained no ledger.
type NotaDeletadaEvent struct {
	BaseEvent
	NotaID          uuid.UUID
	CodigoEstudante string
	CodigoAcademia  string
	Motivo          string
	DeletadoPor     uuid.UUID
	DeletedAt       time.Time
}

func (e *NotaDeletadaEvent) GetPayload() interface{} { return e }
func (e *NotaDeletadaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

// ============================================================================
// Validações internas
// ============================================================================

func validarPeriodoComLista(periodo string, periodosValidos []string) error {
	if periodo == "" {
		return fmt.Errorf("periodo não pode ser vazio")
	}
	for _, p := range periodosValidos {
		if p == periodo {
			return nil
		}
	}
	return fmt.Errorf("periodo '%s' inválido para este contexto. Aceitos: %v", periodo, periodosValidos)
}

// validarCategoria verifica se a categoria é aceita para o tipo de nota.
//
// Regras:
//   - TipoEscolar:  aceita categorias fixas (nota_escola, nota_professor) OU
//                  categorias adicionais cadastradas pela academia.
//   - TipoSuperior: aceita categorias fixas (nota_pp1, nota_pp2, nota_exame) OU
//                  categorias adicionais cadastradas pela academia.
//
// categoriasAdicionais é a lista de categorias extras cadastradas pela academia
// e deve ser fornecida pelo handler para ambos os tipos.
func validarCategoria(tipo string, categoria string, categoriasAdicionais []string) error {
	if categoria == "" {
		return fmt.Errorf("categoria não pode ser vazia")
	}
	switch tipo {
	case TipoEscolar:
		if categoriasEscolarFixas[categoria] {
			return nil
		}
		for _, c := range categoriasAdicionais {
			if c == categoria {
				return nil
			}
		}
		return fmt.Errorf(
			"categoria '%s' inválida para tipo 'escolar'. "+
				"Aceitas: nota_escola, nota_professor, ou categorias adicionais da academia",
			categoria,
		)
	case TipoSuperior:
		if categoriasSuperiorFixas[categoria] {
			return nil
		}
		for _, c := range categoriasAdicionais {
			if c == categoria {
				return nil
			}
		}
		return fmt.Errorf(
			"categoria '%s' não reconhecida para tipo 'superior'. "+
				"Aceitas: nota_pp1, nota_pp2, nota_exame, ou categorias adicionais da academia",
			categoria,
		)
	default:
		return fmt.Errorf("tipo '%s' inválido. Use 'escolar' ou 'superior'", tipo)
	}
}

// ============================================================================
// Método de comando: RegistrarNota
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
// categoriasAdicionais: lista de categorias extras cadastradas pela academia.
// Deve ser fornecida para qualquer tipo — escolar ou superior.
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
	registradoPor uuid.UUID,
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
		RegistradoPor:        registradoPor,
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
// A categoria não pode ser alterada numa atualização — é repassada da nota original.
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
	periodosValidos []string,
	atualizadoPor uuid.UUID,
) error {
	if e.CodigoAcademia == nil || *e.CodigoAcademia != codigoAcademia {
		return fmt.Errorf("estudante não pertence a esta academia")
	}
	if strings.TrimSpace(observacao) == "" {
		return fmt.Errorf("observacao é obrigatória para atualizar uma nota")
	}
	if err := validarPeriodoComLista(periodo, periodosValidos); err != nil {
		return err
	}
	if notaNova < 0 || notaNova > 20 {
		return fmt.Errorf("nota_nova deve estar entre 0 e 20")
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
		AtualizadoPor:        atualizadoPor,
	}

	e.RaiseEvent(event)
	return e.Apply(event)
}

// ============================================================================
// Método de comando: DeletarNota
// ============================================================================

// DeletarNota faz soft delete de uma nota existente.
// motivo é OBRIGATÓRIO para auditoria.
func (e *Estudante) DeletarNota(
	codigoAcademia string,
	notaID uuid.UUID,
	motivo string,
	deletadoPor uuid.UUID,
) error {
	if e.CodigoAcademia == nil || *e.CodigoAcademia != codigoAcademia {
		return fmt.Errorf("estudante não pertence a esta academia")
	}
	if strings.TrimSpace(motivo) == "" {
		return fmt.Errorf("motivo é obrigatório para deletar uma nota")
	}

	event := &NotaDeletadaEvent{
		BaseEvent:       BaseEvent{EventType: "NotaDeletada", AggregateID: e.ID},
		NotaID:          notaID,
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

// applyNotasRegistradas — o aggregate Estudante não mantém notas em estado;
// o estado de notas é gerenciado exclusivamente pela projeção.
func (e *Estudante) applyNotasRegistradas(_ DomainEvent) error {
	return nil
}

// applyNotaAtualizada — idem: estado gerenciado pela projeção.
func (e *Estudante) applyNotaAtualizada(_ DomainEvent) error {
	return nil
}

// applyNotaDeletada — idem: estado gerenciado pela projeção.
func (e *Estudante) applyNotaDeletada(_ DomainEvent) error {
	return nil
}