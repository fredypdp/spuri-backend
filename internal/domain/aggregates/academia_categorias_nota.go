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

func (a *Academia) AdicionarCategoriaNota(
	nome string,
	descricao *string,
	adicionadoPor uuid.UUID,
	categoriasExistentes []string,
) error {
	if nome == "" {
		return fmt.Errorf("nome da categoria não pode ser vazio")
	}

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

func (a *Academia) RemoverCategoriaNota(
	nome string,
	removidoPor uuid.UUID,
	categoriasExistentes []string,
) error {
	if nome == "" {
		return fmt.Errorf("nome da categoria não pode ser vazio")
	}

	existeNoAggregate := false
	for _, c := range a.CategoriasNota {
		if c == nome {
			existeNoAggregate = true
			break
		}
	}

	existeNaProjecao := false
	for _, c := range categoriasExistentes {
		if c == nome {
			existeNaProjecao = true
			break
		}
	}

	if !existeNoAggregate && !existeNaProjecao {
		return fmt.Errorf("categoria '%s' não existe nesta academia", nome)
	}

	event := &CategoriaNotaRemovidaEvent{
		BaseEvent:      BaseEvent{EventType: "CategoriaNotaRemovida", AggregateID: a.ID},
		CodigoAcademia: a.CodigoAcademia,
		Nome:           nome,
		RemovidoPor:    removidoPor,
		CreatedAt:      time.Now(),
	}

	a.RaiseEvent(event)
	return a.Apply(event)
}

// ============================================================================
// Apply handler
// ============================================================================

func (a *Academia) applyCategoriaNotaAdicionada(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applyCategoriaNotaAdicionada: marshal error: %w", err)
	}
	var ev CategoriaNotaAdicionadaEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyCategoriaNotaAdicionada: unmarshal error: %w", err)
	}
	if ev.Nome == "" {
		return fmt.Errorf("applyCategoriaNotaAdicionada: Nome vazio no payload")
	}
	a.CategoriasNota = append(a.CategoriasNota, ev.Nome)
	return nil
}

func (a *Academia) applyCategoriaNotaRemovida(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applyCategoriaNotaRemovida: marshal error: %w", err)
	}
	var ev CategoriaNotaRemovidaEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyCategoriaNotaRemovida: unmarshal error: %w", err)
	}
	if ev.Nome == "" {
		return fmt.Errorf("applyCategoriaNotaRemovida: Nome vazio no payload")
	}

	if len(a.CategoriasNota) == 0 {
		return nil
	}
	nova := make([]string, 0, len(a.CategoriasNota))
	for _, c := range a.CategoriasNota {
		if c != ev.Nome {
			nova = append(nova, c)
		}
	}
	a.CategoriasNota = nova
	return nil
}

// ============================================================================
// Evento
// ============================================================================

// CategoriaNotaAdicionadaEvent — emitido ao adicionar uma categoria de nota.
// EventType: "CategoriaNotaAdicionada" (canônico — não alterar).
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

type CategoriaNotaRemovidaEvent struct {
	BaseEvent
	CodigoAcademia string
	Nome           string
	RemovidoPor    uuid.UUID
	CreatedAt      time.Time
}

func (e *CategoriaNotaRemovidaEvent) GetPayload() interface{} { return e }
func (e *CategoriaNotaRemovidaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }
