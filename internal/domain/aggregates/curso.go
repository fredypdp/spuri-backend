package aggregates

import (
	"encoding/json"
	"fmt"
	"log"
	"spuri/internal/utils"
	"time"

	"github.com/google/uuid"
)

type Curso struct {
	BaseAggregate

	Nome           string
	Type           string   // "medio" ou "superior"
	AnosAcademicos []string // Anos do curso definidos pela academia: ex. ["1ano","2ano","3ano"]
	CodigoAcademia string
	Status         string // "ativo" ou "inativo"
	CreatedAt      time.Time
}

func NewCurso() *Curso {
	log.Printf("[DEBUG] Criando novo agregado Curso")
	return &Curso{
		BaseAggregate: BaseAggregate{
			ID:                uuid.New(),
			Version:           0,
			UncommittedEvents: []DomainEvent{},
		},
		Status:         "ativo",
		AnosAcademicos: []string{},
	}
}

func (c *Curso) GetType() string {
	return "Curso"
}

func (c *Curso) Apply(event DomainEvent) error {
	log.Printf("[DEBUG] Aplicando evento %s ao Curso %s", event.GetEventType(), c.ID)

	switch event.GetEventType() {
	case "CursoCriado":
		return c.applyCursoCriado(event)
	case "CursoAtivado":
		return c.applyCursoAtivado(event)
	case "CursoDesativado":
		return c.applyCursoDesativado(event)
	case "CursoDadosAtualizados":
		return c.applyCursoDadosAtualizados(event)
	default:
		log.Printf("[ERROR] Tipo de evento desconhecido: %s", event.GetEventType())
		return fmt.Errorf("tipo de evento desconhecido: %s", event.GetEventType())
	}
}

// ============================================================================
// Commands
// ============================================================================

func (c *Curso) Criar(
	nome string,
	tipo string,
	anosAcademicos []string,
	codigoAcademia string,
) error {
	log.Printf("[DEBUG] Criando curso: nome=%s, tipo=%s, anosAcademicos=%v, academia=%s",
		nome, tipo, anosAcademicos, codigoAcademia)

	if nome == "" {
		return fmt.Errorf("nome é obrigatório")
	}
	if tipo != "medio" && tipo != "superior" {
		return fmt.Errorf("tipo deve ser 'medio' ou 'superior'")
	}
	if len(anosAcademicos) == 0 {
		return fmt.Errorf("anos_academicos é obrigatório")
	}
	if codigoAcademia == "" {
		return fmt.Errorf("código da academia é obrigatório")
	}

	if err := utils.ValidateAnosCurso(tipo, anosAcademicos); err != nil {
		return err
	}

	event := &CursoCriadoEvent{
		BaseEvent: BaseEvent{
			EventType:   "CursoCriado",
			AggregateID: c.ID,
		},
		Nome:           nome,
		Type:           tipo,
		AnosAcademicos: anosAcademicos,
		CodigoAcademia: codigoAcademia,
		CreatedAt:      time.Now(),
	}

	log.Printf("[DEBUG] Evento CursoCriado criado para curso %s", c.ID)
	c.RaiseEvent(event)
	return c.Apply(event)
}

func (c *Curso) Ativar() error {
	log.Printf("[DEBUG] Ativando curso %s (status atual: %s)", c.ID, c.Status)

	if c.Status == "ativo" {
		return fmt.Errorf("curso já está ativo")
	}

	event := &CursoAtivadoEvent{
		BaseEvent: BaseEvent{
			EventType:   "CursoAtivado",
			AggregateID: c.ID,
		},
		ActivatedAt: time.Now(),
	}

	c.RaiseEvent(event)
	return c.Apply(event)
}

func (c *Curso) Desativar() error {
	log.Printf("[DEBUG] Desativando curso %s (status atual: %s)", c.ID, c.Status)

	if c.Status == "inativo" {
		return fmt.Errorf("curso já está inativo")
	}

	event := &CursoDesativadoEvent{
		BaseEvent: BaseEvent{
			EventType:   "CursoDesativado",
			AggregateID: c.ID,
		},
		DeactivatedAt: time.Now(),
	}

	c.RaiseEvent(event)
	return c.Apply(event)
}

// AtualizarDados atualiza nome, tipo e/ou anos_academicos do curso.
// Passe nil/nil/nil para não alterar os respectivos campos.
func (c *Curso) AtualizarDados(nome *string, tipo *string, anosAcademicos []string) error {
	if nome == nil && tipo == nil && anosAcademicos == nil {
		return fmt.Errorf("nenhum campo para atualizar")
	}

	// Validar anosAcademicos se enviados
	if anosAcademicos != nil {
		tipoEfetivo := c.Type
		if tipo != nil {
			tipoEfetivo = *tipo
		}
		if err := utils.ValidateAnosCurso(tipoEfetivo, anosAcademicos); err != nil {
			return err
		}
	}

	event := &CursoDadosAtualizadosEvent{
		BaseEvent: BaseEvent{
			EventType:   "CursoDadosAtualizados",
			AggregateID: c.ID,
		},
		Nome:           nome,
		Type:           tipo,
		AnosAcademicos: anosAcademicos,
		UpdatedAt:      time.Now(),
	}

	c.RaiseEvent(event)
	return c.Apply(event)
}

// ============================================================================
// Apply handlers
// ============================================================================

func (c *Curso) applyCursoCriado(event DomainEvent) error {
	payload := event.GetPayload()
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	var ev CursoCriadoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	c.ID = ev.AggregateID
	c.Nome = ev.Nome
	c.Type = ev.Type
	c.AnosAcademicos = ev.AnosAcademicos
	c.CodigoAcademia = ev.CodigoAcademia
	c.Status = "ativo"
	c.CreatedAt = ev.CreatedAt

	log.Printf("[DEBUG] CursoCriado aplicado: %s (%s) — %d anos", c.Nome, c.Type, len(c.AnosAcademicos))
	return nil
}

func (c *Curso) applyCursoAtivado(event DomainEvent) error {
	c.Status = "ativo"
	return nil
}

func (c *Curso) applyCursoDesativado(event DomainEvent) error {
	c.Status = "inativo"
	return nil
}

func (c *Curso) applyCursoDadosAtualizados(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return err
	}

	var ev CursoDadosAtualizadosEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	if ev.Nome != nil {
		c.Nome = *ev.Nome
	}
	if ev.Type != nil {
		c.Type = *ev.Type
	}
	if ev.AnosAcademicos != nil {
		c.AnosAcademicos = ev.AnosAcademicos
	}
	return nil
}

// ============================================================================
// Eventos
// ============================================================================

type CursoCriadoEvent struct {
	BaseEvent
	Nome           string
	Type           string
	AnosAcademicos []string
	CodigoAcademia string
	CreatedAt      time.Time
}

func (e *CursoCriadoEvent) GetPayload() interface{} { return e }

type CursoAtivadoEvent struct {
	BaseEvent
	ActivatedAt time.Time
}

func (e *CursoAtivadoEvent) GetPayload() interface{} { return e }

type CursoDesativadoEvent struct {
	BaseEvent
	DeactivatedAt time.Time
}

func (e *CursoDesativadoEvent) GetPayload() interface{} { return e }

type CursoDadosAtualizadosEvent struct {
	BaseEvent
	Nome           *string
	Type           *string
	AnosAcademicos []string
	UpdatedAt      time.Time
}

func (e *CursoDadosAtualizadosEvent) GetPayload() interface{} { return e }