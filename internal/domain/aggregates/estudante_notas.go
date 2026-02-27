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

// Categorias fixas por tipo
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

// NotasRegistradasEvent — emitido ao registrar uma nota pela primeira vez
type NotasRegistradasEvent struct {
	BaseEvent
	CodigoEstudante      string
	CodigoAcademia       string
	AnoLectivo           string
	AnoAcademico         string
	Periodo              string
	MateriaDisciplinarID uuid.UUID
	Tipo                 string  // "escolar" | "superior"
	Categoria            string  // ver constantes acima
	Nota                 float64
	Observacao           *string // opcional no registro
	RegisteredAt         time.Time
}

func (e *NotasRegistradasEvent) GetPayload() interface{} { return e }

// NotaAtualizadaEvent — emitido ao corrigir uma nota existente
// Observacao é OBRIGATÓRIA neste evento (justificativa da correção)
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
// Validações internas (reutilizadas nos dois comandos)
// ============================================================================

func validarPeriodo(periodo string) error {
	validos := map[string]bool{
		"1_trimestre": true, "2_trimestre": true, "3_trimestre": true,
		"1_semestre": true, "2_semestre": true,
	}
	if !validos[periodo] {
		return fmt.Errorf("período inválido: %s", periodo)
	}
	return nil
}

// validarCategoria verifica se a categoria é válida para o tipo dado.
// categoriasAdicionais são as criadas pela academia (apenas para superior).
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
		// Verificar se é uma categoria adicional cadastrada
		for _, ca := range categoriasAdicionais {
			if ca == categoria {
				return nil
			}
		}
		// Verificar formato mínimo
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
// categoriasAdicionais: lista de categorias extras cadastradas pela academia (pode ser nil).
func (e *Estudante) RegistrarNota(
	codigoAcademia string,
	anoLectivo string,
	periodo string,
	materiaDisciplinarID uuid.UUID,
	tipo string,
	categoria string,
	nota float64,
	observacao *string,
	categoriasAdicionais []string,
) error {
	if e.CodigoAcademia == nil || *e.CodigoAcademia != codigoAcademia {
		return fmt.Errorf("estudante não pertence a esta academia")
	}
	if err := validarPeriodo(periodo); err != nil {
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
) error {
	if e.CodigoAcademia == nil || *e.CodigoAcademia != codigoAcademia {
		return fmt.Errorf("estudante não pertence a esta academia")
	}
	if strings.TrimSpace(observacao) == "" {
		return fmt.Errorf("observacao é obrigatória ao atualizar uma nota")
	}
	if err := validarPeriodo(periodo); err != nil {
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
// Apply handlers (adicionar no switch do Apply() do Estudante)
// ============================================================================

func (e *Estudante) applyNotasRegistradas(event DomainEvent) error {
	// O aggregate Estudante não mantém estado de notas em memória —
	// elas são gerenciadas pela projeção. Apenas incrementa versão.
	return nil
}

func (e *Estudante) applyNotaAtualizada(event DomainEvent) error {
	return nil
}