package aggregates

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
)

// ── Agregado ──────────────────────────────────────────────────────────────────

type MateriaDisciplinar struct {
	BaseAggregate

	Nome           string
	Type           string
	AnosAcademicos []string
	Periodo        string // Preenchido apenas para type="superior"
	CodigoAcademia string
	CursoID        *uuid.UUID
	Status         string
	CreatedAt      time.Time
}

func NewMateriaDisciplinar() *MateriaDisciplinar {
	log.Printf("[DEBUG] Criando novo agregado MateriaDisciplinar")
	return &MateriaDisciplinar{
		BaseAggregate: BaseAggregate{
			ID:                uuid.New(),
			Version:           0,
			UncommittedEvents: []DomainEvent{},
		},
		AnosAcademicos: []string{},
	}
}

func (m *MateriaDisciplinar) GetType() string { return "MateriaDisciplinar" }

func (m *MateriaDisciplinar) Apply(event DomainEvent) error {
	log.Printf("[DEBUG] Aplicando evento %s à MateriaDisciplinar %s", event.GetEventType(), m.ID)

	switch event.GetEventType() {
	case "MateriaCriada":
		return m.applyMateriaCriada(event)
	case "MateriaAtivada":
		return m.applyMateriaAtivada(event)
	case "MateriaDesativada":
		return m.applyMateriaDesativada(event)
	case "MateriaDadosAtualizados":
		return m.applyMateriaDadosAtualizados(event)
	case "MateriaPeriodoDefinido":
		return m.applyMateriaPeriodoDefinido(event)
	case "MateriaDeletada":
		return m.applyMateriaDeletada(event)
	default:
		return fmt.Errorf("tipo de evento desconhecido: %s", event.GetEventType())
	}
}

// ── Eventos ───────────────────────────────────────────────────────────────────
// FIX M-01: ToJSON() adicionado a todos os eventos concretos do MateriaDisciplinar.
// Sem essa sobrescrita, BaseEvent.ToJSON() serializa apenas os campos do BaseEvent,
// gravando payload nulo no ledger e impossibilitando o rebuild.

type MateriaCriadaEvent struct {
	BaseEvent
	Nome           string
	Type           string
	AnosAcademicos []string
	CodigoAcademia string
	CursoID        *uuid.UUID
	CreatedAt      time.Time
}

func (e *MateriaCriadaEvent) GetPayload() interface{} { return e }
func (e *MateriaCriadaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) } // FIX M-01

type MateriaAtivadaEvent struct {
	BaseEvent
	ActivatedAt time.Time
}

func (e *MateriaAtivadaEvent) GetPayload() interface{} { return e }
func (e *MateriaAtivadaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) } // FIX M-01

type MateriaDesativadaEvent struct {
	BaseEvent
	DeactivatedAt time.Time
}

func (e *MateriaDesativadaEvent) GetPayload() interface{} { return e }
func (e *MateriaDesativadaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) } // FIX M-01

// MateriaDadosAtualizadosEvent — FIX M-02: campos AnosAcademicos, CursoID e
// AtualizadoPor adicionados para rebuild fiel e auditoria self-contained.
// Campos são ponteiros/nil-safe para eventos legados sem esses campos.
// Etapa 4 deve preencher AtualizadoPor no handler de atualização.
type MateriaDadosAtualizadosEvent struct {
	BaseEvent
	Nome           *string
	// FIX M-02: campos adicionais para rebuild completo.
	AnosAcademicos []string   // nil = não alterar
	CursoID        *uuid.UUID // nil = não alterar
	UpdatedAt      time.Time
	// FIX M-02: UUID do usuário que atualizou. uuid.Nil = legado/não preenchido.
	AtualizadoPor uuid.UUID
}

func (e *MateriaDadosAtualizadosEvent) GetPayload() interface{} { return e }
func (e *MateriaDadosAtualizadosEvent) ToJSON() ([]byte, error) { return json.Marshal(e) } // FIX M-01

// MateriaPeriodoDefinidoEvent — define o período de uma matéria superior.
type MateriaPeriodoDefinidoEvent struct {
	BaseEvent
	Periodo   string
	UpdatedAt time.Time
}

func (e *MateriaPeriodoDefinidoEvent) GetPayload() interface{} { return e }
func (e *MateriaPeriodoDefinidoEvent) ToJSON() ([]byte, error) { return json.Marshal(e) } // FIX M-01

// MateriaDeletadaEvent — marca a matéria como deletada (soft-delete via event sourcing).
type MateriaDeletadaEvent struct {
	BaseEvent
	DeletadoPor uuid.UUID
	Motivo      string
	DeletedAt   time.Time
}

func (e *MateriaDeletadaEvent) GetPayload() interface{} { return e }
func (e *MateriaDeletadaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) } // FIX M-01

// ── Comandos ──────────────────────────────────────────────────────────────────

func (m *MateriaDisciplinar) Criar(
	nome string,
	tipo string,
	anosAcademicos []string,
	codigoAcademia string,
	cursoID *uuid.UUID,
) error {
	log.Printf("[DEBUG] Criando matéria: nome=%s, tipo=%s, academia=%s", nome, tipo, codigoAcademia)

	if nome == "" {
		return fmt.Errorf("nome é obrigatório")
	}
	if tipo != "escolar" && tipo != "superior" {
		return fmt.Errorf("tipo deve ser 'escolar' ou 'superior'")
	}
	if codigoAcademia == "" {
		return fmt.Errorf("codigo_academia é obrigatório")
	}
	if tipo == "superior" && cursoID == nil {
		return fmt.Errorf("curso_id é obrigatório para matérias do tipo 'superior'")
	}

	event := &MateriaCriadaEvent{
		BaseEvent:      BaseEvent{EventType: "MateriaCriada", AggregateID: m.ID},
		Nome:           nome,
		Type:           tipo,
		AnosAcademicos: anosAcademicos,
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
		BaseEvent:   BaseEvent{EventType: "MateriaAtivada", AggregateID: m.ID},
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
		BaseEvent:     BaseEvent{EventType: "MateriaDesativada", AggregateID: m.ID},
		DeactivatedAt: time.Now(),
	}
	m.RaiseEvent(event)
	return m.Apply(event)
}

// AtualizarDados atualiza nome e/ou anos_academicos da matéria.
// FIX M-02: aceita também CursoID e AtualizadoPor.
// Etapa 4 deve preencher atualizadoPor no handler.
func (m *MateriaDisciplinar) AtualizarDados(nome *string, anosAcademicos []string, cursoID *uuid.UUID) error {
	if nome == nil && anosAcademicos == nil && cursoID == nil {
		return fmt.Errorf("nenhum campo para atualizar")
	}

	event := &MateriaDadosAtualizadosEvent{
		BaseEvent:      BaseEvent{EventType: "MateriaDadosAtualizados", AggregateID: m.ID},
		Nome:           nome,
		AnosAcademicos: anosAcademicos,
		CursoID:        cursoID,
		UpdatedAt:      time.Now(),
		AtualizadoPor:  uuid.Nil, // Etapa 4 deve preencher
	}
	m.RaiseEvent(event)
	return m.Apply(event)
}

func (m *MateriaDisciplinar) DefinirPeriodo(periodo string) error {
	if m.Type != "superior" {
		return fmt.Errorf("período só pode ser definido para matérias do tipo 'superior'")
	}
	if periodo == "" {
		return fmt.Errorf("periodo não pode ser vazio")
	}

	event := &MateriaPeriodoDefinidoEvent{
		BaseEvent: BaseEvent{EventType: "MateriaPeriodoDefinido", AggregateID: m.ID},
		Periodo:   periodo,
		UpdatedAt: time.Now(),
	}
	m.RaiseEvent(event)
	return m.Apply(event)
}

func (m *MateriaDisciplinar) Deletar(deletadoPor uuid.UUID, motivo string) error {
	if m.Status == "deletado" {
		return fmt.Errorf("matéria já está deletada")
	}
	if m.Status == "ativo" {
		return fmt.Errorf("desative a matéria antes de deletá-la")
	}

	event := &MateriaDeletadaEvent{
		BaseEvent:   BaseEvent{EventType: "MateriaDeletada", AggregateID: m.ID},
		DeletadoPor: deletadoPor,
		Motivo:      motivo,
		DeletedAt:   time.Now(),
	}
	m.RaiseEvent(event)
	return m.Apply(event)
}

// ── Aplicadores ───────────────────────────────────────────────────────────────

func (m *MateriaDisciplinar) applyMateriaCriada(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applyMateriaCriada: marshal error: %w", err)
	}
	var ev MateriaCriadaEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyMateriaCriada: unmarshal error: %w", err)
	}
	m.Nome = ev.Nome
	m.Type = ev.Type
	m.AnosAcademicos = ev.AnosAcademicos
	m.CodigoAcademia = ev.CodigoAcademia
	m.CursoID = ev.CursoID
	m.Status = "ativo"
	m.CreatedAt = ev.CreatedAt
	return nil
}

func (m *MateriaDisciplinar) applyMateriaAtivada(_ DomainEvent) error {
	m.Status = "ativo"
	return nil
}

func (m *MateriaDisciplinar) applyMateriaDesativada(_ DomainEvent) error {
	m.Status = "inativo"
	return nil
}

func (m *MateriaDisciplinar) applyMateriaDadosAtualizados(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applyMateriaDadosAtualizados: marshal error: %w", err)
	}
	var ev MateriaDadosAtualizadosEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyMateriaDadosAtualizados: unmarshal error: %w", err)
	}
	if ev.Nome != nil {
		m.Nome = *ev.Nome
	}
	// FIX M-02: aplicar campos adicionais quando presentes
	if ev.AnosAcademicos != nil {
		m.AnosAcademicos = ev.AnosAcademicos
	}
	if ev.CursoID != nil {
		m.CursoID = ev.CursoID
	}
	return nil
}

func (m *MateriaDisciplinar) applyMateriaPeriodoDefinido(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applyMateriaPeriodoDefinido: marshal error: %w", err)
	}
	var ev MateriaPeriodoDefinidoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyMateriaPeriodoDefinido: unmarshal error: %w", err)
	}
	m.Periodo = ev.Periodo
	return nil
}

func (m *MateriaDisciplinar) applyMateriaDeletada(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applyMateriaDeletada: marshal error: %w", err)
	}
	var ev MateriaDeletadaEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyMateriaDeletada: unmarshal error: %w", err)
	}
	_ = ev // motivo e deletadoPor usados apenas na projeção
	m.Status = "deletado"
	return nil
}