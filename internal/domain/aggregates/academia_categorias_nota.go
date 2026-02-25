package aggregates

import (
	"fmt"
	"regexp"
	"time"
)

// Regex: nome deve ser nota_[letras/números/underscore], mínimo 1 char após "nota_"
var regexCategoria = regexp.MustCompile(`^nota_[a-z0-9_]+$`)

// ============================================================================
// Evento
// ============================================================================

type CategoriaNotaAdicionadaEvent struct {
	BaseEvent
	CodigoAcademia string
	Nome           string // formato: nota_[nome]
	Descricao      *string
	CreatedAt      time.Time
}

func (e *CategoriaNotaAdicionadaEvent) GetPayload() interface{} { return e }

// ============================================================================
// Comando
// ============================================================================

// AdicionarCategoriaNotaSuperior permite que uma academia do tipo "superior"
// cadastre uma categoria adicional de nota no formato nota_[nome].
//
// categoriasExistentes: lista de nomes já cadastrados pela academia
// (carregados da projection_categorias_nota antes de chamar este método)
func (a *Academia) AdicionarCategoriaNotaSuperior(
	nome string,
	descricao *string,
	categoriasExistentes []string,
) error {
	if a.Type != "superior" {
		return fmt.Errorf("somente academias do tipo 'superior' podem criar categorias de nota")
	}
	if a.Status != "ativo" {
		return fmt.Errorf("academia está inativa")
	}

	if !regexCategoria.MatchString(nome) {
		return fmt.Errorf(
			"nome de categoria inválido: use apenas letras minúsculas, números e underscore após 'nota_' (ex: nota_trabalho)",
		)
	}
	
	// Categorias fixas não podem ser sobrescritas
	fixas := map[string]bool{
		"nota_pp1": true, "nota_pp2": true, "nota_exame": true,
	}
	if fixas[nome] {
		return fmt.Errorf("'%s' é uma categoria fixa e não pode ser recriada", nome)
	}

	// Verificar duplicata
	for _, c := range categoriasExistentes {
		if c == nome {
			return fmt.Errorf("categoria '%s' já existe nesta academia", nome)
		}
	}

	event := &CategoriaNotaAdicionadaEvent{
		BaseEvent:      BaseEvent{EventType: "CategoriaNotaAdicionada", AggregateID: a.ID},
		CodigoAcademia: a.CodigoAcademia,
		Nome:           nome,
		Descricao:      descricao,
		CreatedAt:      time.Now(),
	}

	a.RaiseEvent(event)
	return a.Apply(event)
}

// ============================================================================
// Apply handler
// ============================================================================

func (a *Academia) applyCategoriaNotaAdicionada(event DomainEvent) error {
	// Academia não mantém lista de categorias em estado — gerenciado pela projeção
	return nil
}