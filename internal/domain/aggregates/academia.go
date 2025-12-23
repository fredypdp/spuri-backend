package aggregates

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Academia agregado raiz
type Academia struct {
	BaseAggregate
	
	// Estado
	Type           string
	Nome           string
	CodigoAcademia string
	SenhaHash      string
	Provincia      string
	Endereco       string
	NumeroTelefone *string
	Email          *string
	Website        *string
	NivelEscolar   *string
	Status         string
	Cursos         []string
	CreatedAt      time.Time
	
	// Estatísticas (reconstruídas de eventos)
	TotalEstudantes          int
	TotalInscricoesPendentes int
}

// NewAcademia cria um novo agregado Academia
func NewAcademia() *Academia {
	return &Academia{
		BaseAggregate: BaseAggregate{
			ID:                uuid.New(),
			Version:           0,
			UncommittedEvents: []DomainEvent{},
		},
		Cursos: []string{},
	}
}

// GetType implementa Aggregate
func (a *Academia) GetType() string {
	return "Academia"
}

// Apply aplica eventos ao agregado
func (a *Academia) Apply(event DomainEvent) error {
	switch event.GetEventType() {
	case "AcademiaCriada":
		return a.applyAcademiaCriada(event)
	case "InscricaoAprovada":
		return a.applyInscricaoAprovada(event)
	case "InscricaoReprovada":
		return a.applyInscricaoReprovada(event)
	case "AcademiaAtivada":
		return a.applyAcademiaAtivada(event)
	case "AcademiaDesativada":
		return a.applyAcademiaDesativada(event)
	case "CursosAtualizados":
		return a.applyCursosAtualizados(event)
	default:
		return fmt.Errorf("tipo de evento desconhecido: %s", event.GetEventType())
	}
}

// Comandos - geram eventos

// Criar cria uma nova academia
func (a *Academia) Criar(
	tipo string,
	nome string,
	codigoAcademia string,
	senhaHash string,
	provincia string,
	endereco string,
	numeroTelefone *string,
	email *string,
	website *string,
	nivelEscolar *string,
	cursos []string,
) error {
	// Validações
	if tipo != "escola" && tipo != "superior" {
		return fmt.Errorf("tipo deve ser 'escola' ou 'superior'")
	}
	if nome == "" {
		return fmt.Errorf("nome é obrigatório")
	}
	if codigoAcademia == "" {
		return fmt.Errorf("código é obrigatório")
	}
	if tipo == "escola" && nivelEscolar == nil {
		return fmt.Errorf("nivel_escolar é obrigatório para escolas")
	}

	// Criar evento
	event := &AcademiaCriadaEvent{
		BaseEvent: BaseEvent{
			EventType:   "AcademiaCriada",
			AggregateID: a.ID,
		},
		Type:           tipo,
		Nome:           nome,
		CodigoAcademia: codigoAcademia,
		SenhaHash:      senhaHash,
		Provincia:      provincia,
		Endereco:       endereco,
		NumeroTelefone: numeroTelefone,
		Email:          email,
		Website:        website,
		NivelEscolar:   nivelEscolar,
		Cursos:         cursos,
		CreatedAt:      time.Now(),
	}

	a.RaiseEvent(event)
	return a.Apply(event)
}

// AprovarInscricao aprova uma inscrição de estudante
func (a *Academia) AprovarInscricao(
	estudanteID uuid.UUID,
	inscricaoID uuid.UUID,
	tipo string,
	anoInscricao string,
	curso *string,
) error {
	// Validações
	if a.Status != "ativo" {
		return fmt.Errorf("academia está inativa")
	}

	// Criar evento
	event := &InscricaoAprovadaPorAcademiaEvent{
		BaseEvent: BaseEvent{
			EventType:   "InscricaoAprovada",
			AggregateID: a.ID,
		},
		EstudanteID:  estudanteID,
		InscricaoID:  inscricaoID,
		AcademiaID:   a.ID,
		Tipo:         tipo,
		AnoInscricao: anoInscricao,
		Curso:        curso,
		ApprovedAt:   time.Now(),
	}

	a.RaiseEvent(event)
	return a.Apply(event)
}

// ReprovarInscricao reprova uma inscrição
func (a *Academia) ReprovarInscricao(
	estudanteID uuid.UUID,
	inscricaoID uuid.UUID,
	motivo string,
) error {
	// Validações
	if a.Status != "ativo" {
		return fmt.Errorf("academia está inativa")
	}

	// Criar evento
	event := &InscricaoReprovadaPorAcademiaEvent{
		BaseEvent: BaseEvent{
			EventType:   "InscricaoReprovada",
			AggregateID: a.ID,
		},
		EstudanteID: estudanteID,
		InscricaoID: inscricaoID,
		AcademiaID:  a.ID,
		Motivo:      motivo,
		RejectedAt:  time.Now(),
	}

	a.RaiseEvent(event)
	return a.Apply(event)
}

// Ativar ativa a academia
func (a *Academia) Ativar() error {
	if a.Status == "ativo" {
		return fmt.Errorf("academia já está ativa")
	}

	event := &AcademiaAtivadaEvent{
		BaseEvent: BaseEvent{
			EventType:   "AcademiaAtivada",
			AggregateID: a.ID,
		},
		ActivatedAt: time.Now(),
	}

	a.RaiseEvent(event)
	return a.Apply(event)
}

// Desativar desativa a academia
func (a *Academia) Desativar(motivo string) error {
	if a.Status == "inativo" {
		return fmt.Errorf("academia já está inativa")
	}

	event := &AcademiaDesativadaEvent{
		BaseEvent: BaseEvent{
			EventType:   "AcademiaDesativada",
			AggregateID: a.ID,
		},
		Motivo:        motivo,
		DeactivatedAt: time.Now(),
	}

	a.RaiseEvent(event)
	return a.Apply(event)
}

// AtualizarCursos atualiza lista de cursos
func (a *Academia) AtualizarCursos(cursos []string) error {
	if a.Status != "ativo" {
		return fmt.Errorf("academia está inativa")
	}

	event := &CursosAtualizadosEvent{
		BaseEvent: BaseEvent{
			EventType:   "CursosAtualizados",
			AggregateID: a.ID,
		},
		NovoCursos: cursos,
		UpdatedAt:  time.Now(),
	}

	a.RaiseEvent(event)
	return a.Apply(event)
}

// Event Handlers - aplicam eventos ao estado

func (a *Academia) applyAcademiaCriada(event DomainEvent) error {
	payload := event.GetPayload()
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	var ev AcademiaCriadaEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	a.ID = event.GetAggregateID()
	a.Type = ev.Type
	a.Nome = ev.Nome
	a.CodigoAcademia = ev.CodigoAcademia
	a.SenhaHash = ev.SenhaHash
	a.Provincia = ev.Provincia
	a.Endereco = ev.Endereco
	a.NumeroTelefone = ev.NumeroTelefone
	a.Email = ev.Email
	a.Website = ev.Website
	a.NivelEscolar = ev.NivelEscolar
	a.Cursos = ev.Cursos
	a.Status = "ativo"
	a.CreatedAt = ev.CreatedAt

	return nil
}

func (a *Academia) applyInscricaoAprovada(event DomainEvent) error {
	a.TotalEstudantes++
	a.TotalInscricoesPendentes--
	if a.TotalInscricoesPendentes < 0 {
		a.TotalInscricoesPendentes = 0
	}
	return nil
}

func (a *Academia) applyInscricaoReprovada(event DomainEvent) error {
	a.TotalInscricoesPendentes--
	if a.TotalInscricoesPendentes < 0 {
		a.TotalInscricoesPendentes = 0
	}
	return nil
}

func (a *Academia) applyAcademiaAtivada(event DomainEvent) error {
	a.Status = "ativo"
	return nil
}

func (a *Academia) applyAcademiaDesativada(event DomainEvent) error {
	a.Status = "inativo"
	return nil
}

func (a *Academia) applyCursosAtualizados(event DomainEvent) error {
	payload := event.GetPayload()
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	var ev CursosAtualizadosEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	a.Cursos = ev.NovoCursos
	return nil
}

// Eventos da Academia

type AcademiaCriadaEvent struct {
	BaseEvent
	Type           string
	Nome           string
	CodigoAcademia string
	SenhaHash      string
	Provincia      string
	Endereco       string
	NumeroTelefone *string
	Email          *string
	Website        *string
	NivelEscolar   *string
	Cursos         []string
	CreatedAt      time.Time
}

func (e *AcademiaCriadaEvent) GetPayload() interface{} {
	return e
}

type InscricaoAprovadaPorAcademiaEvent struct {
	BaseEvent
	EstudanteID  uuid.UUID
	InscricaoID  uuid.UUID
	AcademiaID   uuid.UUID
	Tipo         string
	AnoInscricao string
	Curso        *string
	ApprovedAt   time.Time
}

func (e *InscricaoAprovadaPorAcademiaEvent) GetPayload() interface{} {
	return e
}

type InscricaoReprovadaPorAcademiaEvent struct {
	BaseEvent
	EstudanteID uuid.UUID
	InscricaoID uuid.UUID
	AcademiaID  uuid.UUID
	Motivo      string
	RejectedAt  time.Time
}

func (e *InscricaoReprovadaPorAcademiaEvent) GetPayload() interface{} {
	return e
}

type AcademiaAtivadaEvent struct {
	BaseEvent
	ActivatedAt time.Time
}

func (e *AcademiaAtivadaEvent) GetPayload() interface{} {
	return e
}

type AcademiaDesativadaEvent struct {
	BaseEvent
	Motivo        string
	DeactivatedAt time.Time
}

func (e *AcademiaDesativadaEvent) GetPayload() interface{} {
	return e
}

type CursosAtualizadosEvent struct {
	BaseEvent
	NovoCursos []string
	UpdatedAt  time.Time
}

func (e *CursosAtualizadosEvent) GetPayload() interface{} {
	return e
}