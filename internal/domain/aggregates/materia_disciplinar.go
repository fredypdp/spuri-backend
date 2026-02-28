// ============================================================================
// ARQUIVO: internal/domain/aggregates/materia_disciplinar.go
// Agregado MatériaDisciplinar (Event Sourcing) + logs de debug
// ============================================================================

package aggregates

import (
	"encoding/json"
	"fmt"
	"log"
	"spuri/internal/utils"
	"time"

	"github.com/google/uuid"
)

type MateriaDisciplinar struct {
	BaseAggregate

	Nome            string
	Type            string     // "fundamental", "medio", "superior"
	AnosAcademicos  []string   // Apenas para fundamental: ["1ano", "2ano"]
	CodigoAcademia  string
	CursoID         *uuid.UUID // NULL para fundamental
	Status          string     // "ativo" ou "inativo"
	CreatedAt       time.Time
}

func NewMateriaDisciplinar() *MateriaDisciplinar {
	log.Printf("[DEBUG] Criando novo agregado MateriaDisciplinar")
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
	default:
		log.Printf("[ERROR] Tipo de evento desconhecido: %s", event.GetEventType())
		return fmt.Errorf("tipo de evento desconhecido: %s", event.GetEventType())
	}
}

// ── Comandos ──────────────────────────────────────────────────────────────────

func (m *MateriaDisciplinar) Criar(
	nome string,
	tipo string,
	anosAcademicos []string,
	codigoAcademia string,
	cursoID *uuid.UUID,
) error {
	log.Printf("[DEBUG] Criando matéria: nome=%s, tipo=%s, anosAcademicos=%v, academia=%s, cursoID=%v",
		nome, tipo, anosAcademicos, codigoAcademia, cursoID)

	// Validações
	if nome == "" {
		log.Printf("[ERROR] Nome é obrigatório")
		return fmt.Errorf("nome é obrigatório")
	}

	if tipo != "fundamental" && tipo != "medio" && tipo != "superior" {
		log.Printf("[ERROR] Tipo inválido: %s", tipo)
		return fmt.Errorf("tipo deve ser 'fundamental', 'medio' ou 'superior'")
	}

	if codigoAcademia == "" {
		log.Printf("[ERROR] Código da academia é obrigatório")
		return fmt.Errorf("código da academia é obrigatório")
	}

	// Fundamental não pode ter curso_id
	if tipo == "fundamental" && cursoID != nil {
		log.Printf("[ERROR] Matéria fundamental não pode ter curso associado")
		return fmt.Errorf("matérias fundamentais não podem ter curso associado")
	}

	// Medio/Superior deve ter curso_id
	if (tipo == "medio" || tipo == "superior") && cursoID == nil {
		log.Printf("[ERROR] Matéria medio/superior sem curso associado")
		return fmt.Errorf("matérias de médio/superior devem ter curso associado")
	}

	// Fundamental deve ter anos_academicos
	if tipo == "fundamental" && len(anosAcademicos) == 0 {
		log.Printf("[ERROR] Matéria fundamental sem anos_academicos definidos")
		return fmt.Errorf("matérias fundamentais devem ter anos_academicos definidos")
	}

	// Validar anos fundamentais
	if tipo == "fundamental" {
		if err := utils.ValidateAnosFundamental(anosAcademicos); err != nil {
			log.Printf("[ERROR] Validação de anos_academicos fundamental falhou: %v", err)
			return err
		}
	}

	event := &MateriaCriadaEvent{
		BaseEvent: BaseEvent{
			EventType:   "MateriaCriada",
			AggregateID: m.ID,
		},
		Nome:            nome,
		Type:            tipo,
		AnosAcademicos:  anosAcademicos,
		CodigoAcademia:  codigoAcademia,
		CursoID:         cursoID,
		CreatedAt:       time.Now(),
	}

	log.Printf("[DEBUG] Evento MateriaCriada criado para matéria %s", m.ID)
	m.RaiseEvent(event)
	return m.Apply(event)
}

func (m *MateriaDisciplinar) Ativar() error {
	log.Printf("[DEBUG] Ativando matéria %s (status atual: %s)", m.ID, m.Status)

	if m.Status == "ativo" {
		log.Printf("[ERROR] Matéria já está ativa")
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
	log.Printf("[DEBUG] Desativando matéria %s (status atual: %s)", m.ID, m.Status)

	if m.Status == "inativo" {
		log.Printf("[ERROR] Matéria já está inativa")
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

// AtualizarDados atualiza dados da matéria
func (m *MateriaDisciplinar) AtualizarDados(
	nome *string,
	tipo *string,
) error {
	log.Printf("[DEBUG] Atualizando dados da matéria %s", m.ID)

	if m.Status != "ativo" {
		log.Printf("[ERROR] Matéria inativa não pode ser atualizada")
		return fmt.Errorf("matéria inativa não pode ser atualizada")
	}

	if nome == nil && tipo == nil {
		log.Printf("[ERROR] Nenhum campo para atualizar")
		return fmt.Errorf("nenhum campo para atualizar")
	}

	if nome != nil && *nome == "" {
		log.Printf("[ERROR] Nome não pode ser vazio")
		return fmt.Errorf("nome não pode ser vazio")
	}

	if tipo != nil {
		if *tipo != "fundamental" && *tipo != "medio" && *tipo != "superior" {
			log.Printf("[ERROR] Tipo inválido: %s", *tipo)
			return fmt.Errorf("tipo deve ser 'fundamental', 'medio' ou 'superior'")
		}

		// Se mudando de fundamental para medio/superior, precisa ter curso_id
		if m.Type == "fundamental" && (*tipo == "medio" || *tipo == "superior") && m.CursoID == nil {
			log.Printf("[ERROR] Não é possível mudar para medio/superior sem curso associado")
			return fmt.Errorf("não é possível mudar para medio/superior sem curso associado")
		}

		// Se mudando para fundamental, não pode ter curso_id
		if *tipo == "fundamental" && m.CursoID != nil {
			log.Printf("[ERROR] Matéria fundamental não pode ter curso associado")
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

	log.Printf("[DEBUG] Evento MateriaDadosAtualizados criado")
	m.RaiseEvent(event)
	return m.Apply(event)
}

// ── Event Handlers ────────────────────────────────────────────────────────────

func (m *MateriaDisciplinar) applyMateriaCriada(event DomainEvent) error {
	log.Printf("[DEBUG] Aplicando MateriaCriada ao agregado %s", event.GetAggregateID())

	payload := event.GetPayload()
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[ERROR] Erro ao serializar payload: %v", err)
		return err
	}

	var ev MateriaCriadaEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		log.Printf("[ERROR] Erro ao deserializar evento: %v", err)
		return err
	}

	m.ID              = event.GetAggregateID()
	m.Nome            = ev.Nome
	m.Type            = ev.Type
	m.AnosAcademicos  = ev.AnosAcademicos
	m.CodigoAcademia  = ev.CodigoAcademia
	m.CursoID         = ev.CursoID
	m.Status          = "ativo"
	m.CreatedAt       = ev.CreatedAt

	log.Printf("[DEBUG] Matéria criada: %s (%s)", m.Nome, m.ID)
	return nil
}

func (m *MateriaDisciplinar) applyMateriaAtivada(event DomainEvent) error {
	log.Printf("[DEBUG] Aplicando MateriaAtivada ao agregado %s", event.GetAggregateID())
	m.Status = "ativo"
	return nil
}

func (m *MateriaDisciplinar) applyMateriaDesativada(event DomainEvent) error {
	log.Printf("[DEBUG] Aplicando MateriaDesativada ao agregado %s", event.GetAggregateID())
	m.Status = "inativo"
	return nil
}

func (m *MateriaDisciplinar) applyMateriaDadosAtualizados(event DomainEvent) error {
	log.Printf("[DEBUG] Aplicando MateriaDadosAtualizados ao agregado %s", event.GetAggregateID())

	payload := event.GetPayload()
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[ERROR] Erro ao serializar payload: %v", err)
		return err
	}

	var ev MateriaDadosAtualizadosEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		log.Printf("[ERROR] Erro ao deserializar evento: %v", err)
		return err
	}

	if ev.Nome != nil {
		log.Printf("[DEBUG] Atualizando nome: %s -> %s", m.Nome, *ev.Nome)
		m.Nome = *ev.Nome
	}
	if ev.Type != nil {
		log.Printf("[DEBUG] Atualizando tipo: %s -> %s", m.Type, *ev.Type)
		m.Type = *ev.Type
	}

	log.Printf("[DEBUG] Dados da matéria atualizados com sucesso")
	return nil
}

// ── Eventos ───────────────────────────────────────────────────────────────────

type MateriaCriadaEvent struct {
	BaseEvent
	Nome            string
	Type            string
	AnosAcademicos  []string
	CodigoAcademia  string
	CursoID         *uuid.UUID
	CreatedAt       time.Time
}

func (e *MateriaCriadaEvent) GetPayload() interface{} { return e }

type MateriaAtivadaEvent struct {
	BaseEvent
	ActivatedAt time.Time
}

func (e *MateriaAtivadaEvent) GetPayload() interface{} { return e }

type MateriaDesativadaEvent struct {
	BaseEvent
	DeactivatedAt time.Time
}

func (e *MateriaDesativadaEvent) GetPayload() interface{} { return e }

type MateriaDadosAtualizadosEvent struct {
	BaseEvent
	Nome      *string
	Type      *string
	UpdatedAt time.Time
}

func (e *MateriaDadosAtualizadosEvent) GetPayload() interface{} { return e }