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
	UpdatedAt time.Time
}

func (e *MateriaDadosAtualizadosEvent) GetPayload() interface{} { return e }

// MateriaPeriodoDefinidoEvent — define o período de uma matéria superior.
type MateriaPeriodoDefinidoEvent struct {
	BaseEvent
	Periodo   string
	UpdatedAt time.Time
}

func (e *MateriaPeriodoDefinidoEvent) GetPayload() interface{} { return e }

// MateriaDeletadaEvent — marca a matéria como deletada (soft-delete via event sourcing).
type MateriaDeletadaEvent struct {
	BaseEvent
	DeletedAt time.Time
}

func (e *MateriaDeletadaEvent) GetPayload() interface{} { return e }

// ── Apply handlers ────────────────────────────────────────────────────────────
//
// REGRA: apply handlers NÃO devem chamar m.Version++.
// O incremento é feito exclusivamente por BaseAggregate.RaiseEvent().
// Isso é consistente com todos os outros aggregates do sistema (Turma, Academia, Curso...).

func (m *MateriaDisciplinar) applyMateriaCriada(event DomainEvent) error {
	log.Printf("[DEBUG] Aplicando MateriaCriada ao agregado %s", event.GetAggregateID())

	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return err
	}
	var ev MateriaCriadaEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	m.ID             = event.GetAggregateID()
	m.Nome           = ev.Nome
	m.Type           = ev.Type
	m.AnosAcademicos = ev.AnosAcademicos
	m.CodigoAcademia = ev.CodigoAcademia
	m.CursoID        = ev.CursoID
	m.CreatedAt      = ev.CreatedAt
	m.Periodo        = "" // sempre vazio na criação

	// Superior nasce inativo; demais nascem ativos
	if ev.Type == "superior" {
		m.Status = "inativo"
	} else {
		m.Status = "ativo"
	}

	// BUG #4 FIX: m.Version++ REMOVIDO — RaiseEvent já incrementa.
	log.Printf("[DEBUG] Matéria criada: %s (%s) status=%s", m.Nome, m.ID, m.Status)
	return nil
}

func (m *MateriaDisciplinar) applyMateriaAtivada(event DomainEvent) error {
	log.Printf("[DEBUG] Aplicando MateriaAtivada ao agregado %s", event.GetAggregateID())
	m.Status = "ativo"
	// BUG #4 FIX: m.Version++ REMOVIDO — RaiseEvent já incrementa.
	return nil
}

func (m *MateriaDisciplinar) applyMateriaDesativada(event DomainEvent) error {
	log.Printf("[DEBUG] Aplicando MateriaDesativada ao agregado %s", event.GetAggregateID())
	m.Status = "inativo"
	// BUG #4 FIX: m.Version++ REMOVIDO — RaiseEvent já incrementa.
	return nil
}

func (m *MateriaDisciplinar) applyMateriaDadosAtualizados(event DomainEvent) error {
	log.Printf("[DEBUG] Aplicando MateriaDadosAtualizados ao agregado %s", event.GetAggregateID())

	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return err
	}
	var ev MateriaDadosAtualizadosEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}
	if ev.Nome != nil {
		m.Nome = *ev.Nome
	}
	// BUG #4 FIX: m.Version++ REMOVIDO — RaiseEvent já incrementa.
	return nil
}

func (m *MateriaDisciplinar) applyMateriaPeriodoDefinido(event DomainEvent) error {
	log.Printf("[DEBUG] Aplicando MateriaPeriodoDefinido ao agregado %s", event.GetAggregateID())

	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return err
	}
	var ev MateriaPeriodoDefinidoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}
	m.Periodo = ev.Periodo
	// BUG #4 FIX: m.Version++ REMOVIDO — RaiseEvent já incrementa.
	return nil
}

func (m *MateriaDisciplinar) applyMateriaDeletada(event DomainEvent) error {
	log.Printf("[DEBUG] Aplicando MateriaDeletada ao agregado %s", event.GetAggregateID())
	m.Status = "deletado"
	// BUG #4 FIX: m.Version++ REMOVIDO — RaiseEvent já incrementa.
	return nil
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

	if nome == "" {
		return fmt.Errorf("nome é obrigatório")
	}
	if tipo != "fundamental" && tipo != "medio" && tipo != "superior" {
		return fmt.Errorf("tipo deve ser 'fundamental', 'medio' ou 'superior'")
	}
	if codigoAcademia == "" {
		return fmt.Errorf("código da academia é obrigatório")
	}
	if tipo == "fundamental" && cursoID != nil {
		return fmt.Errorf("matérias fundamentais não podem ter curso associado")
	}
	if (tipo == "medio" || tipo == "superior") && cursoID == nil {
		return fmt.Errorf("matérias de médio/superior devem ter curso associado")
	}
	if tipo == "fundamental" && len(anosAcademicos) == 0 {
		return fmt.Errorf("matérias fundamentais devem ter anos_academicos definidos")
	}

	event := &MateriaCriadaEvent{
		BaseEvent: BaseEvent{
			EventType:   "MateriaCriada",
			AggregateID: m.ID,
		},
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
	log.Printf("[DEBUG] Ativando matéria %s (status atual: %s)", m.ID, m.Status)

	if m.Status == "ativo" {
		return fmt.Errorf("matéria já está ativa")
	}
	if m.Status == "deletado" {
		return fmt.Errorf("matéria deletada não pode ser ativada")
	}
	// Superior exige período preenchido
	if m.Type == "superior" && m.Periodo == "" {
		return fmt.Errorf("matéria superior deve ter o período definido antes de ser ativada")
	}

	event := &MateriaAtivadaEvent{
		BaseEvent:   BaseEvent{EventType: "MateriaAtivada", AggregateID: m.ID},
		ActivatedAt: time.Now(),
	}

	m.RaiseEvent(event)
	return m.Apply(event)
}

func (m *MateriaDisciplinar) Desativar() error {
	log.Printf("[DEBUG] Desativando matéria %s (status atual: %s)", m.ID, m.Status)

	if m.Status == "inativo" {
		return fmt.Errorf("matéria já está inativa")
	}
	if m.Status == "deletado" {
		return fmt.Errorf("matéria deletada não pode ser desativada")
	}

	event := &MateriaDesativadaEvent{
		BaseEvent:     BaseEvent{EventType: "MateriaDesativada", AggregateID: m.ID},
		DeactivatedAt: time.Now(),
	}

	m.RaiseEvent(event)
	return m.Apply(event)
}

// AtualizarDados atualiza o nome da matéria (único campo mutável).
// anos_academicos são imutáveis — desative e recrie para alterar.
func (m *MateriaDisciplinar) AtualizarDados(nome *string) error {
	log.Printf("[DEBUG] Atualizando dados da matéria %s", m.ID)

	if m.Status != "ativo" {
		return fmt.Errorf("matéria inativa não pode ser atualizada")
	}
	if nome == nil || *nome == "" {
		return fmt.Errorf("nome é obrigatório")
	}

	event := &MateriaDadosAtualizadosEvent{
		BaseEvent: BaseEvent{EventType: "MateriaDadosAtualizados", AggregateID: m.ID},
		Nome:      nome,
		UpdatedAt: time.Now(),
	}

	m.RaiseEvent(event)
	return m.Apply(event)
}

// DefinirPeriodo define o período de uma matéria superior.
// Pode ser chamado tanto com a matéria ativa quanto inativa.
// O período deve pertencer ao conjunto de períodos válidos do sistema.
func (m *MateriaDisciplinar) DefinirPeriodo(periodo string) error {
	log.Printf("[DEBUG] Definindo período da matéria %s: %s", m.ID, periodo)

	if m.Type != "superior" {
		return fmt.Errorf("período só pode ser definido em matérias do tipo 'superior'")
	}
	if m.Status == "deletado" {
		return fmt.Errorf("matéria deletada não pode ser modificada")
	}

	periodosValidos := map[string]bool{
		"1_trimestre": true,
		"2_trimestre": true,
		"3_trimestre": true,
		"1_semestre":  true,
		"2_semestre":  true,
	}
	if !periodosValidos[periodo] {
		return fmt.Errorf(
			"período inválido: '%s'. Valores aceitos: 1_trimestre, 2_trimestre, 3_trimestre, 1_semestre, 2_semestre",
			periodo,
		)
	}

	event := &MateriaPeriodoDefinidoEvent{
		BaseEvent: BaseEvent{EventType: "MateriaPeriodoDefinido", AggregateID: m.ID},
		Periodo:   periodo,
		UpdatedAt: time.Now(),
	}

	m.RaiseEvent(event)
	return m.Apply(event)
}

// Deletar emite MateriaDeletada. A matéria deve estar inativa.
// Não remove o histórico do ledger — apenas marca como deletada na projeção.
func (m *MateriaDisciplinar) Deletar() error {
	log.Printf("[DEBUG] Deletando matéria %s (status atual: %s)", m.ID, m.Status)

	if m.Status == "deletado" {
		return fmt.Errorf("matéria já está deletada")
	}
	if m.Status == "ativo" {
		return fmt.Errorf("desative a matéria antes de deletá-la")
	}

	event := &MateriaDeletadaEvent{
		BaseEvent: BaseEvent{EventType: "MateriaDeletada", AggregateID: m.ID},
		DeletedAt: time.Now(),
	}

	m.RaiseEvent(event)
	return m.Apply(event)
}