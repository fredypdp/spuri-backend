// ============================================================================
// ARQUIVO: internal/domain/aggregates/materia_disciplinar.go
// Agregado MatériaDisciplinar (Event Sourcing)
// ============================================================================

package aggregates

import (
	"encoding/json"
	"fmt"
	"spuri/internal/utils"
	"time"

	"github.com/google/uuid"
)

type MateriaDisciplinar struct {
	BaseAggregate
	
	Nome           string
	Type           string     // "fundamental", "medio", "superior"
	Nivel          []string   // Apenas para fundamental: ["1ano", "2ano"]
	CodigoAcademia string
	CursoID        *uuid.UUID // NULL para fundamental
	Status         string     // "ativo" ou "inativo"
	CreatedAt      time.Time
}

func NewMateriaDisciplinar() *MateriaDisciplinar {
	return &MateriaDisciplinar{
		BaseAggregate: BaseAggregate{
			ID:                uuid.New(),
			Version:           0,
			UncommittedEvents: []DomainEvent{},
		},
		Status: "ativo",
	}
}

func (m *MateriaDisciplinar) GetType() string {
	return "MateriaDisciplinar"
}

func (m *MateriaDisciplinar) Apply(event DomainEvent) error {
	switch event.GetEventType() {
	case "MateriaCriada":
		return m.applyMateriaCriada(event)
	case "MateriaAtivada":
		return m.applyMateriaAtivada(event)
	case "MateriaDesativada":
		return m.applyMateriaDesativada(event)
	case "MateriaDadosAtualizados":
		return m.applyMateriaDadosAtualizados(event)
	default:
		return fmt.Errorf("tipo de evento desconhecido: %s", event.GetEventType())
	}
}

// Comandos

func (m *MateriaDisciplinar) Criar(
	nome string,
	tipo string,
	nivel []string,
	codigoAcademia string,
	cursoID *uuid.UUID,
) error {
	// Validações
	if nome == "" {
		return fmt.Errorf("nome é obrigatório")
	}
	
	if tipo != "fundamental" && tipo != "medio" && tipo != "superior" {
		return fmt.Errorf("tipo deve ser 'fundamental', 'medio' ou 'superior'")
	}
	
	if codigoAcademia == "" {
		return fmt.Errorf("código da academia é obrigatório")
	}

	// Fundamental não pode ter curso_id
	if tipo == "fundamental" && cursoID != nil {
		return fmt.Errorf("matérias fundamentais não podem ter curso associado")
	}

	// Medio/Superior deve ter curso_id
	if (tipo == "medio" || tipo == "superior") && cursoID == nil {
		return fmt.Errorf("matérias de médio/superior devem ter curso associado")
	}

	// Fundamental deve ter nível
	if tipo == "fundamental" && len(nivel) == 0 {
		return fmt.Errorf("matérias fundamentais devem ter nível definido")
	}
	
	// 🔥 Validar anos fundamentais
	if tipo == "fundamental" {
		if err := utils.ValidateAnosFundamental(nivel); err != nil {
			return err
		}
	}

	event := &MateriaCriadaEvent{
		BaseEvent: BaseEvent{
			EventType:   "MateriaCriada",
			AggregateID: m.ID,
		},
		Nome:           nome,
		Type:           tipo,
		Nivel:          nivel,
		CodigoAcademia: codigoAcademia,
		CursoID:        cursoID,
		CreatedAt:      time.Now(),
	}

	m.RaiseEvent(event)
	return m.Apply(event)
}

func (m *MateriaDisciplinar) Ativar() error {
	if m.Status == "ativo" {
		return fmt.Errorf("matéria já está ativa")
	}

	event := &MateriaAtivadaEvent{
		BaseEvent: BaseEvent{
			EventType:   "MateriaAtivada",
			AggregateID: m.ID,
		},
		ActivatedAt: time.Now(),
	}

	m.RaiseEvent(event)
	return m.Apply(event)
}

func (m *MateriaDisciplinar) Desativar() error {
	if m.Status == "inativo" {
		return fmt.Errorf("matéria já está inativa")
	}

	event := &MateriaDesativadaEvent{
		BaseEvent: BaseEvent{
			EventType:   "MateriaDesativada",
			AggregateID: m.ID,
		},
		DeactivatedAt: time.Now(),
	}

	m.RaiseEvent(event)
	return m.Apply(event)
}

// Event Handlers

func (m *MateriaDisciplinar) applyMateriaCriada(event DomainEvent) error {
	payload := event.GetPayload()
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	var ev MateriaCriadaEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	m.ID = event.GetAggregateID()
	m.Nome = ev.Nome
	m.Type = ev.Type
	m.Nivel = ev.Nivel
	m.CodigoAcademia = ev.CodigoAcademia
	m.CursoID = ev.CursoID
	m.Status = "ativo"
	m.CreatedAt = ev.CreatedAt

	return nil
}

func (m *MateriaDisciplinar) applyMateriaAtivada(event DomainEvent) error {
	m.Status = "ativo"
	return nil
}

func (m *MateriaDisciplinar) applyMateriaDesativada(event DomainEvent) error {
	m.Status = "inativo"
	return nil
}

// Eventos

type MateriaCriadaEvent struct {
	BaseEvent
	Nome           string
	Type           string
	Nivel          []string
	CodigoAcademia string
	CursoID        *uuid.UUID
	CreatedAt      time.Time
}

func (e *MateriaCriadaEvent) GetPayload() interface{} {
	return e
}

type MateriaAtivadaEvent struct {
	BaseEvent
	ActivatedAt time.Time
}

func (e *MateriaAtivadaEvent) GetPayload() interface{} {
	return e
}

type MateriaDesativadaEvent struct {
	BaseEvent
	DeactivatedAt time.Time
}

func (e *MateriaDesativadaEvent) GetPayload() interface{} {
	return e
}

// AtualizarDados atualiza dados da matéria
func (m *MateriaDisciplinar) AtualizarDados(
	nome *string,
	tipo *string,
) error {
	if m.Status != "ativo" {
		return fmt.Errorf("matéria inativa não pode ser atualizada")
	}

	// Validação: pelo menos um campo deve ser fornecido
	if nome == nil && tipo == nil {
		return fmt.Errorf("nenhum campo para atualizar")
	}

	// Validações específicas
	if nome != nil && *nome == "" {
		return fmt.Errorf("nome não pode ser vazio")
	}

	if tipo != nil {
		if *tipo != "fundamental" && *tipo != "medio" && *tipo != "superior" {
			return fmt.Errorf("tipo deve ser 'fundamental', 'medio' ou 'superior'")
		}

		// Se mudando de fundamental para medio/superior, precisa ter curso_id
		if m.Type == "fundamental" && (*tipo == "medio" || *tipo == "superior") && m.CursoID == nil {
			return fmt.Errorf("não é possível mudar para medio/superior sem curso associado")
		}

		// Se mudando para fundamental, não pode ter curso_id
		if *tipo == "fundamental" && m.CursoID != nil {
			return fmt.Errorf("matérias fundamentais não podem ter curso associado")
		}
	}

	event := &MateriaDadosAtualizadosEvent{
		BaseEvent: BaseEvent{
			EventType:   "MateriaDadosAtualizados",
			AggregateID: m.ID,
		},
		Nome:      nome,
		Type:      tipo,
		UpdatedAt: time.Now(),
	}

	m.RaiseEvent(event)
	return m.Apply(event)
}

func (m *MateriaDisciplinar) applyMateriaDadosAtualizados(event DomainEvent) error {
	payload := event.GetPayload()
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	var ev MateriaDadosAtualizadosEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	// Atualizar apenas os campos fornecidos
	if ev.Nome != nil {
		m.Nome = *ev.Nome
	}
	if ev.Type != nil {
		m.Type = *ev.Type
	}

	return nil
}

type MateriaDadosAtualizadosEvent struct {
	BaseEvent
	Nome      *string
	Type      *string
	UpdatedAt time.Time
}

func (e *MateriaDadosAtualizadosEvent) GetPayload() interface{} {
	return e
}