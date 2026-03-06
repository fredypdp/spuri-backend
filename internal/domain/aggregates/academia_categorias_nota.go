package aggregates

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// Comando
// ============================================================================

// AdicionarCategoriaNotaSuperior adiciona uma categoria de nota personalizada.
//
// categoriasExistentes — lista atual de categorias (nome) da academia,
// carregada pelo handler via projeção. O aggregate também verifica
// a.CategoriasNota em estado para detectar duplicatas durante o ciclo
// de vida em memória (FIX A-02).
func (a *Academia) AdicionarCategoriaNotaSuperior(
	nome string,
	descricao *string,
	adicionadoPor uuid.UUID,
	categoriasExistentes []string,
) error {
	if a.Type != "superior" {
		return fmt.Errorf("categorias de nota são exclusivas de academias do tipo 'superior'")
	}
	if nome == "" {
		return fmt.Errorf("nome da categoria não pode ser vazio")
	}

	// FIX A-02: verificação de unicidade usando estado do aggregate (CategoriasNota)
	// além do parâmetro externo, para cobrir o caso em que o handler não passa
	// categoriasExistentes corretamente.
	for _, c := range a.CategoriasNota {
		if c == nome {
			return fmt.Errorf("categoria '%s' já existe nesta academia (detectado via estado do aggregate)", nome)
		}
	}
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

// applyCategoriaNotaAdicionada — FIX A-02: deserializa o payload para detectar
// corrupção silenciosa e mantém a.CategoriasNota em estado para que comandos
// subsequentes possam detectar duplicatas sem depender de parâmetros externos.
func (a *Academia) applyCategoriaNotaAdicionada(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applyCategoriaNotaAdicionada: marshal error: %w", err)
	}
	var ev CategoriaNotaAdicionadaEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyCategoriaNotaAdicionada: unmarshal error (payload corrompido): %w", err)
	}
	if ev.Nome == "" {
		return fmt.Errorf("applyCategoriaNotaAdicionada: campo Nome vazio no payload")
	}
	// Inicializa slice se necessário (ex: aggregate recém-criado)
	if a.CategoriasNota == nil {
		a.CategoriasNota = []string{}
	}
	a.CategoriasNota = append(a.CategoriasNota, ev.Nome)
	return nil
}

// ============================================================================
// Evento
// ============================================================================

type CategoriaNotaAdicionadaEvent struct {
	BaseEvent
	CodigoAcademia string
	Nome           string
	Descricao      *string
	AdicionadoPor  uuid.UUID
	CreatedAt      time.Time
}

func (e *CategoriaNotaAdicionadaEvent) GetPayload() interface{} { return e }
func (e *CategoriaNotaAdicionadaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }