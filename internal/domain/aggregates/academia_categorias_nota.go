package aggregates

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// Comando
// ============================================================================

func (a *Academia) AdicionarCategoriaNota(
	codigo string,
	nome string,
	descricao *string,
	adicionadoPor uuid.UUID,
	codigosExistentes []string,
) error {
	codigo = strings.TrimSpace(codigo)
	nome = strings.TrimSpace(nome)
	if codigo == "" {
		return fmt.Errorf("codigo da categoria não pode ser vazio")
	}
	if nome == "" {
		return fmt.Errorf("nome da categoria não pode ser vazio")
	}
	if strings.Contains(codigo, " ") {
		return fmt.Errorf("codigo da categoria não pode conter espaços")
	}
	if ok, _ := regexp.MatchString(`^[a-z0-9_]+$`, codigo); !ok {
		return fmt.Errorf("codigo da categoria inválido: use apenas letras minúsculas, números e underscore")
	}

	for _, c := range a.CategoriasNota {
		if c == codigo {
			return fmt.Errorf("categoria '%s' já existe nesta academia (detectado via estado do aggregate)", codigo)
		}
	}
	for _, c := range codigosExistentes {
		if c == codigo {
			return fmt.Errorf("categoria '%s' já existe nesta academia", codigo)
		}
	}

	event := &CategoriaNotaAdicionadaEvent{
		BaseEvent:      BaseEvent{EventType: "CategoriaNotaAdicionada", AggregateID: a.ID},
		CodigoAcademia: a.CodigoAcademia,
		Codigo:         codigo,
		Nome:           nome,
		Descricao:      descricao,
		AdicionadoPor:  adicionadoPor,
		CreatedAt:      time.Now(),
	}

	a.RaiseEvent(event)
	return a.Apply(event)
}

func (a *Academia) RemoverCategoriaNota(
	codigo string,
	removidoPor uuid.UUID,
	codigosExistentes []string,
) error {
	codigo = strings.TrimSpace(codigo)
	if codigo == "" {
		return fmt.Errorf("codigo da categoria não pode ser vazio")
	}

	existeNoAggregate := false
	for _, c := range a.CategoriasNota {
		if c == codigo {
			existeNoAggregate = true
			break
		}
	}

	existeNaProjecao := false
	for _, c := range codigosExistentes {
		if c == codigo {
			existeNaProjecao = true
			break
		}
	}

	if !existeNoAggregate && !existeNaProjecao {
		return fmt.Errorf("categoria '%s' não existe nesta academia", codigo)
	}

	event := &CategoriaNotaRemovidaEvent{
		BaseEvent:      BaseEvent{EventType: "CategoriaNotaRemovida", AggregateID: a.ID},
		CodigoAcademia: a.CodigoAcademia,
		Codigo:         codigo,
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
	if ev.Codigo == "" {
		return fmt.Errorf("applyCategoriaNotaAdicionada: Codigo vazio no payload")
	}
	a.CategoriasNota = append(a.CategoriasNota, ev.Codigo)
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
	if ev.Codigo == "" {
		return fmt.Errorf("applyCategoriaNotaRemovida: Codigo vazio no payload")
	}

	if len(a.CategoriasNota) == 0 {
		return nil
	}
	nova := make([]string, 0, len(a.CategoriasNota))
	for _, c := range a.CategoriasNota {
		if c != ev.Codigo {
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
	Codigo         string
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
	Codigo         string
	RemovidoPor    uuid.UUID
	CreatedAt      time.Time
}

func (e *CategoriaNotaRemovidaEvent) GetPayload() interface{} { return e }
func (e *CategoriaNotaRemovidaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }
