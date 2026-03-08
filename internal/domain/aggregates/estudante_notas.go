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
// EventType: "NotasRegistradas" (canônico — a projeção escuta exatamente este string).
// AnoAcademico é sempre preenchido pelo back end (nunca pelo cliente).
//
// FIX E-06: campo RegistradoPor adicionado para auditoria self-contained.
// Etapa 4 deve preencher este campo no handler de registro de notas.
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
	// Etapa 4 deve passar este campo via RegistrarNota.
	RegistradoPor uuid.UUID
}

func (e *NotasRegistradasEvent) GetPayload() interface{} { return e }
func (e *NotasRegistradasEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

// NotaAtualizadaEvent — emitido ao corrigir uma nota existente.
// EventType: "NotaAtualizada" (canônico).
// Observacao é OBRIGATÓRIA neste evento (justificativa da correção).
//
// FIX E-07: campo AtualizadoPor adicionado para auditoria self-contained.
// Etapa 4 deve preencher este campo no handler de correção de notas.
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
	// Etapa 4 deve passar este campo via AtualizarNota.
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

func validarCategoria(tipo string, categoria string, categoriasAdicionais []string) error {
	if categoria == "" {
		return fmt.Errorf("categoria não pode ser vazia")
	}
	switch tipo {
	case TipoEscolar:
		if !categoriasEscolar[categoria] {
			return fmt.Errorf("categoria '%s' inválida para tipo 'escolar'. Aceitas: nota_escola, nota_professor", categoria)
		}
	case TipoSuperior:
		if categoriasSuperiorFixas[categoria] {
			return nil
		}
		for _, c := range categoriasAdicionais {
			if c == categoria {
				return nil
			}
		}
		return fmt.Errorf("categoria '%s' não reconhecida para tipo 'superior'", categoria)
	default:
		return fmt.Errorf("tipo '%s' inválido. Use 'escolar' ou 'superior'", tipo)
	}
	return nil
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
func (e *Estudante) AtualizarNota(
	codigoAcademia string,
	anoLectivo string,
	periodo string,
	materiaDisciplinarID uuid.UUID,
	tipo string,
	categoria string,
	notaAnterior float64,
	novaNota float64,
	observacao string,
	periodosValidos []string,
	atualizadoPor uuid.UUID,
) error {
	if e.CodigoAcademia == nil || *e.CodigoAcademia != codigoAcademia {
		return fmt.Errorf("estudante não pertence a esta academia")
	}
	if strings.TrimSpace(observacao) == "" {
		return fmt.Errorf("observacao é obrigatória para correção de nota")
	}
	if err := validarPeriodoComLista(periodo, periodosValidos); err != nil {
		return err
	}
	if novaNota < 0 || novaNota > 20 {
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
		NotaNova:             novaNota,
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

// DeletarNota faz soft delete de uma nota via event sourcing.
// motivo é OBRIGATÓRIO para auditoria self-contained no ledger.
func (e *Estudante) DeletarNota(
	codigoAcademia string,
	notaID uuid.UUID,
	motivo string,
	deletadoPor uuid.UUID,
) error {
	if e.CodigoAcademia == nil || *e.CodigoAcademia != codigoAcademia {
		return fmt.Errorf("estudante não pertence a esta academia")
	}
	if notaID == uuid.Nil {
		return fmt.Errorf("nota_id inválido")
	}
	if strings.TrimSpace(motivo) == "" {
		return fmt.Errorf("motivo é obrigatório para deletar nota")
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

func (e *Estudante) applyNotasRegistradas(_ DomainEvent) error {
	// O aggregate Estudante não mantém notas em estado — gerenciado pela projeção.
	return nil
}

func (e *Estudante) applyNotaAtualizada(_ DomainEvent) error {
	// O aggregate Estudante não mantém notas em estado — gerenciado pela projeção.
	return nil
}

func (e *Estudante) applyNotaDeletada(_ DomainEvent) error {
	// O aggregate Estudante não mantém notas em estado — gerenciado pela projeção.
	return nil
}