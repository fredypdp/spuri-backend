// ============================================================================
// ARQUIVO: internal/domain/aggregates/academia_categorias_nota.go
//
// CORREÇÕES APLICADAS (Etapa 1):
//   #19 — CategoriaNotaAdicionadaEvent agora inclui AdicionadoPor uuid.UUID
//         para rastreabilidade forense completa (quem adicionou a categoria).
//         Sem este campo, era impossível determinar o responsável sem depender
//         dos metadados do ledger.
//   Etapa1-ToJSON — ToJSON() implementado no evento (antes herdava
//         BaseEvent.ToJSON() que serializava e.Payload=nil = "null" no ledger).
//
// NOTA PARA ETAPA 4:
//   O handler que chama AdicionarCategoriaNotaSuperior deve passar o UUID do
//   admin autenticado como parâmetro adicionadoPor.
// ============================================================================

package aggregates

import (
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/google/uuid"
)

// Regex: nome deve ser nota_[letras/números/underscore], mínimo 1 char após "nota_"
var regexCategoria = regexp.MustCompile(`^nota_[a-z0-9_]+$`)

// ============================================================================
// Evento
// ============================================================================

// CategoriaNotaAdicionadaEvent — FIX #19: AdicionadoPor adicionado para
// rastreabilidade forense. Sem este campo, era impossível determinar qual
// admin adicionou a categoria sem acesso à tabela de metadados do ledger.
type CategoriaNotaAdicionadaEvent struct {
	BaseEvent
	CodigoAcademia string
	Nome           string     // formato: nota_[nome]
	Descricao      *string
	AdicionadoPor  uuid.UUID  // FIX #19: UUID do admin responsável
	CreatedAt      time.Time
}

func (e *CategoriaNotaAdicionadaEvent) GetPayload() interface{} { return e }
func (e *CategoriaNotaAdicionadaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

// ============================================================================
// Comando
// ============================================================================

// AdicionarCategoriaNotaSuperior permite que uma academia do tipo "superior"
// cadastre uma categoria adicional de nota no formato nota_[nome].
//
// adicionadoPor: UUID do admin que está realizando a operação (FIX #19).
// categoriasExistentes: lista de nomes já cadastrados pela academia
// (carregados da projection_categorias_nota antes de chamar este método).
func (a *Academia) AdicionarCategoriaNotaSuperior(
	nome string,
	descricao *string,
	adicionadoPor uuid.UUID,
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
		AdicionadoPor:  adicionadoPor,
		CreatedAt:      time.Now(),
	}

	a.RaiseEvent(event)
	return a.Apply(event)
}

// ============================================================================
// Apply handler
// ============================================================================

func (a *Academia) applyCategoriaNotaAdicionada(_ DomainEvent) error {
	// Academia não mantém lista de categorias em estado — gerenciado pela projeção
	return nil
}