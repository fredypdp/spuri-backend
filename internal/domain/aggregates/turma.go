package aggregates

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
)

type Turma struct {
	BaseAggregate

	CodigoTurma    string
	CodigoAcademia string
	Nivel          string
	CursoID        *uuid.UUID
	Turno          string   // "manha", "tarde", "noite"
	Estudantes     []string // lista de codigo_estudante
	Status         string   // "ativo", "inativo", "deletado"
	CreatedAt      time.Time
	DeletedAt      *time.Time

	// FIX T-01: campos de auditoria de ativação/desativação.
	StatusAlteradoPor uuid.UUID
	StatusAlteradoEm  time.Time
}

func NewTurma() *Turma {
	log.Printf("[DEBUG] Criando novo agregado Turma")
	return &Turma{
		BaseAggregate: BaseAggregate{
			ID:                uuid.New(),
			Version:           0,
			UncommittedEvents: []DomainEvent{},
		},
		Estudantes: []string{},
		Status:     "ativo",
	}
}

func (t *Turma) GetType() string { return "Turma" }

func (t *Turma) Apply(event DomainEvent) error {
	log.Printf("[DEBUG] Aplicando evento %s à Turma %s", event.GetEventType(), t.ID)
	switch event.GetEventType() {
	case "TurmaCriada":
		return t.applyTurmaCriada(event)
	case "TurmaAtivada":
		return t.applyTurmaStatusChange("ativo", event)
	case "TurmaDesativada":
		return t.applyTurmaStatusChange("inativo", event)
	case "EstudanteAdicionadoATurma":
		return t.applyEstudanteAdicionado(event)
	case "EstudanteRemovidoDaTurma":
		return t.applyEstudanteRemovido(event)
	case "TurmaDadosAtualizados":
		return t.applyTurmaDadosAtualizados(event)
	case "TurmaDeletada":
		return t.applyTurmaDeletada(event)
	default:
		return fmt.Errorf("tipo de evento desconhecido: %s", event.GetEventType())
	}
}

// ── Comandos ──────────────────────────────────────────────────────────────────

func (t *Turma) Criar(
	codigoTurma string,
	codigoAcademia string,
	nivel string,
	cursoID *uuid.UUID,
	turno string,
	criadoPor uuid.UUID,
) error {
	if codigoTurma == "" || codigoAcademia == "" || nivel == "" {
		return fmt.Errorf("codigo_turma, codigo_academia e nivel são obrigatórios")
	}
	if turno != "manha" && turno != "tarde" && turno != "noite" {
		return fmt.Errorf("turno deve ser 'manha', 'tarde' ou 'noite'")
	}

	event := &TurmaCriadaEvent{
		BaseEvent:      BaseEvent{EventType: "TurmaCriada", AggregateID: t.ID},
		CodigoTurma:    codigoTurma,
		CodigoAcademia: codigoAcademia,
		Nivel:          nivel,
		CursoID:        cursoID,
		Turno:          turno,
		CriadoPor:      criadoPor,
		CreatedAt:      time.Now(),
	}
	t.RaiseEvent(event)
	return t.Apply(event)
}

func (t *Turma) Ativar(ativadoPor uuid.UUID) error {
	if t.Status == "ativo" {
		return fmt.Errorf("turma já está ativa")
	}
	event := &TurmaStatusEvent{
		BaseEvent:   BaseEvent{EventType: "TurmaAtivada", AggregateID: t.ID},
		AlteradoPor: ativadoPor,
	}
	t.RaiseEvent(event)
	return t.Apply(event)
}

func (t *Turma) Desativar(desativadoPor uuid.UUID) error {
	if t.Status == "inativo" {
		return fmt.Errorf("turma já está inativa")
	}
	event := &TurmaStatusEvent{
		BaseEvent:   BaseEvent{EventType: "TurmaDesativada", AggregateID: t.ID},
		AlteradoPor: desativadoPor,
	}
	t.RaiseEvent(event)
	return t.Apply(event)
}

func (t *Turma) AdicionarEstudante(codigoEstudante string, adicionadoPor uuid.UUID) error {
	return t.AdicionarEstudanteNoAnoLectivo(codigoEstudante, "", adicionadoPor)
}

func (t *Turma) AdicionarEstudanteNoAnoLectivo(codigoEstudante, anoLectivo string, adicionadoPor uuid.UUID) error {
	if codigoEstudante == "" {
		return fmt.Errorf("codigo_estudante é obrigatório")
	}
	for _, e := range t.Estudantes {
		if e == codigoEstudante {
			return fmt.Errorf("estudante já pertence a esta turma")
		}
	}
	event := &EstudanteTurmaEvent{
		BaseEvent:       BaseEvent{EventType: "EstudanteAdicionadoATurma", AggregateID: t.ID},
		CodigoEstudante: codigoEstudante,
		AnoLectivo:      anoLectivo,
		AlteradoPor:     adicionadoPor,
	}
	t.RaiseEvent(event)
	return t.Apply(event)
}

func (t *Turma) RemoverEstudante(codigoEstudante string, removidoPor uuid.UUID) error {
	return t.RemoverEstudanteNoAnoLectivo(codigoEstudante, "", removidoPor)
}

func (t *Turma) RemoverEstudanteNoAnoLectivo(codigoEstudante, anoLectivo string, removidoPor uuid.UUID) error {
	found := false
	for _, e := range t.Estudantes {
		if e == codigoEstudante {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("estudante não pertence a esta turma")
	}
	event := &EstudanteTurmaEvent{
		BaseEvent:       BaseEvent{EventType: "EstudanteRemovidoDaTurma", AggregateID: t.ID},
		CodigoEstudante: codigoEstudante,
		AnoLectivo:      anoLectivo,
		AlteradoPor:     removidoPor,
	}
	t.RaiseEvent(event)
	return t.Apply(event)
}

func (t *Turma) AtualizarDados(nivel *string, cursoID *uuid.UUID, turno *string, atualizadoPor uuid.UUID) error {
	if turno != nil && *turno != "manha" && *turno != "tarde" && *turno != "noite" {
		return fmt.Errorf("turno deve ser 'manha', 'tarde' ou 'noite'")
	}
	event := &TurmaDadosAtualizadosEvent{
		BaseEvent:     BaseEvent{EventType: "TurmaDadosAtualizados", AggregateID: t.ID},
		Nivel:         nivel,
		CursoID:       cursoID,
		Turno:         turno,
		AtualizadoPor: atualizadoPor,
	}
	t.RaiseEvent(event)
	return t.Apply(event)
}

// Deletar emite TurmaDeletada.
// Pré-condições (verificadas pelo aggregate):
//   - turma deve estar inativa
//   - turma não pode ter estudantes ativos
//
// NOTA T-02: a validação de processos ativos (avaliações em andamento, etc.)
// é responsabilidade do handler (requer acesso à projeção).
func (t *Turma) Deletar(deletadoPor uuid.UUID, motivo string) error {
	if t.Status == "deletado" {
		return fmt.Errorf("turma já está deletada")
	}
	if t.Status == "ativo" {
		return fmt.Errorf("desative a turma antes de deletá-la")
	}
	if len(t.Estudantes) > 0 {
		return fmt.Errorf("remova todos os estudantes antes de deletar a turma (%d restantes)", len(t.Estudantes))
	}

	event := &TurmaDeletadaEvent{
		BaseEvent:   BaseEvent{EventType: "TurmaDeletada", AggregateID: t.ID},
		DeletadoPor: deletadoPor,
		Motivo:      motivo,
		DeletedAt:   time.Now(),
	}
	t.RaiseEvent(event)
	return t.Apply(event)
}

// ── Aplicadores ───────────────────────────────────────────────────────────────

func (t *Turma) applyTurmaCriada(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applyTurmaCriada: marshal error: %w", err)
	}
	var ev TurmaCriadaEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyTurmaCriada: unmarshal error: %w", err)
	}
	t.CodigoTurma = ev.CodigoTurma
	t.CodigoAcademia = ev.CodigoAcademia
	t.Nivel = ev.Nivel
	t.CursoID = ev.CursoID
	t.Turno = ev.Turno
	t.Status = "ativo"
	t.CreatedAt = ev.CreatedAt
	return nil
}

// applyTurmaStatusChange — FIX T-01: deserializa o payload TurmaStatusEvent
// para salvar AlteradoPor no estado do aggregate.
// A versão anterior (applyStatusChange) ignorava o payload completamente,
// descartando a informação de quem realizou a ativação/desativação.
func (t *Turma) applyTurmaStatusChange(novoStatus string, event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applyTurmaStatusChange: marshal error: %w", err)
	}
	var ev TurmaStatusEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyTurmaStatusChange: unmarshal error: %w", err)
	}
	t.Status = novoStatus
	t.StatusAlteradoPor = ev.AlteradoPor
	t.StatusAlteradoEm = time.Now()
	return nil
}

func (t *Turma) applyEstudanteAdicionado(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applyEstudanteAdicionado: marshal error: %w", err)
	}
	var ev EstudanteTurmaEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyEstudanteAdicionado: unmarshal error: %w", err)
	}
	t.Estudantes = append(t.Estudantes, ev.CodigoEstudante)
	return nil
}

func (t *Turma) applyEstudanteRemovido(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applyEstudanteRemovido: marshal error: %w", err)
	}
	var ev EstudanteTurmaEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyEstudanteRemovido: unmarshal error: %w", err)
	}
	updated := []string{}
	for _, e := range t.Estudantes {
		if e != ev.CodigoEstudante {
			updated = append(updated, e)
		}
	}
	t.Estudantes = updated
	return nil
}

func (t *Turma) applyTurmaDadosAtualizados(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applyTurmaDadosAtualizados: marshal error: %w", err)
	}
	var ev TurmaDadosAtualizadosEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyTurmaDadosAtualizados: unmarshal error: %w", err)
	}
	if ev.Nivel != nil {
		t.Nivel = *ev.Nivel
	}
	if ev.CursoID != nil {
		t.CursoID = ev.CursoID
	}
	if ev.Turno != nil {
		t.Turno = *ev.Turno
	}
	return nil
}

func (t *Turma) applyTurmaDeletada(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applyTurmaDeletada: marshal error: %w", err)
	}
	var ev TurmaDeletadaEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyTurmaDeletada: unmarshal error: %w", err)
	}
	t.Status = "deletado"
	t.DeletedAt = &ev.DeletedAt
	return nil
}

// ── Eventos ───────────────────────────────────────────────────────────────────

type TurmaCriadaEvent struct {
	BaseEvent
	CodigoTurma    string
	CodigoAcademia string
	Nivel          string
	CursoID        *uuid.UUID
	Turno          string
	CriadoPor      uuid.UUID
	CreatedAt      time.Time
}

func (e *TurmaCriadaEvent) GetPayload() interface{} { return e }
func (e *TurmaCriadaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type TurmaStatusEvent struct {
	BaseEvent
	AlteradoPor uuid.UUID
}

func (e *TurmaStatusEvent) GetPayload() interface{} { return e }
func (e *TurmaStatusEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type EstudanteTurmaEvent struct {
	BaseEvent
	CodigoEstudante string
	AnoLectivo      string
	AlteradoPor     uuid.UUID
}

func (e *EstudanteTurmaEvent) GetPayload() interface{} { return e }
func (e *EstudanteTurmaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type TurmaDadosAtualizadosEvent struct {
	BaseEvent
	Nivel         *string
	CursoID       *uuid.UUID
	Turno         *string
	AtualizadoPor uuid.UUID
}

func (e *TurmaDadosAtualizadosEvent) GetPayload() interface{} { return e }
func (e *TurmaDadosAtualizadosEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type TurmaDeletadaEvent struct {
	BaseEvent
	DeletadoPor uuid.UUID
	Motivo      string
	DeletedAt   time.Time
}

func (e *TurmaDeletadaEvent) GetPayload() interface{} { return e }
func (e *TurmaDeletadaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }
