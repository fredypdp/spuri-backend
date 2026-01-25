package aggregates

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Academia struct {
	BaseAggregate
	
	Type            string
	Nome            string
	CodigoAcademia  string
	SenhaHash       string
	Provincia       string
	Endereco        string
	NumeroTelefone  *string
	Email           *string
	EmailVerificado bool
	Website         *string
	NivelEscolar    *string
	Status          string
	Cursos          []string
	CreatedAt       time.Time
	
	TotalEstudantes          int
	TotalInscricoesPendentes int
}

func NewAcademia() *Academia {
	return &Academia{
		BaseAggregate: BaseAggregate{
			ID:                uuid.New(),
			Version:           0,
			UncommittedEvents: []DomainEvent{},
		},
		Status:          "inativo",
		Cursos:          []string{},
		EmailVerificado: false,
	}
}

func (a *Academia) GetType() string {
	return "Academia"
}

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
	case "AcademiaDadosAtualizados":
		return a.applyAcademiaDadosAtualizados(event)
	case "EmailVerificado":
		return a.applyEmailVerificado(event)
	default:
		return fmt.Errorf("tipo de evento desconhecido: %s", event.GetEventType())
	}
}

func (a *Academia) Criar(tipo string, nome string, codigoAcademia string, senhaHash string, provincia string, endereco string, numeroTelefone *string, email *string, website *string, nivelEscolar *string, cursos []string) error {
	if tipo != "escola" && tipo != "superior" {
		return fmt.Errorf("tipo deve ser 'escola' ou 'superior'")
	}
	if nome == "" || codigoAcademia == "" {
		return fmt.Errorf("campos obrigatórios vazios")
	}
	
	if tipo == "escola" && nivelEscolar == nil {
		return fmt.Errorf("nivel_escolar é obrigatório para escolas")
	}

	event := &AcademiaCriadaEvent{
		BaseEvent:      BaseEvent{EventType: "AcademiaCriada", AggregateID: a.ID},
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

func (a *Academia) VerificarEmail() error {
	if a.EmailVerificado {
		return fmt.Errorf("email já verificado")
	}

	event := &EmailVerificadoEvent{
		BaseEvent:  BaseEvent{EventType: "EmailVerificado", AggregateID: a.ID},
		VerifiedAt: time.Now(),
	}

	a.RaiseEvent(event)
	return a.Apply(event)
}

func (a *Academia) AprovarInscricao(estudanteID uuid.UUID, inscricaoID uuid.UUID, tipo string, anoInscricao string, curso *string) error {
	if a.Status != "ativo" {
		return fmt.Errorf("academia está inativa")
	}

	event := &InscricaoAprovadaPorAcademiaEvent{
		BaseEvent:    BaseEvent{EventType: "InscricaoAprovada", AggregateID: a.ID},
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

func (a *Academia) ReprovarInscricao(estudanteID uuid.UUID, inscricaoID uuid.UUID, motivo string) error {
	if a.Status != "ativo" {
		return fmt.Errorf("academia está inativa")
	}

	event := &InscricaoReprovadaPorAcademiaEvent{
		BaseEvent:   BaseEvent{EventType: "InscricaoReprovada", AggregateID: a.ID},
		EstudanteID: estudanteID,
		InscricaoID: inscricaoID,
		AcademiaID:  a.ID,
		Motivo:      motivo,
		RejectedAt:  time.Now(),
	}

	a.RaiseEvent(event)
	return a.Apply(event)
}

func (a *Academia) Ativar() error {
	if a.Status == "ativo" {
		return fmt.Errorf("academia já está ativa")
	}

	event := &AcademiaAtivadaEvent{
		BaseEvent:   BaseEvent{EventType: "AcademiaAtivada", AggregateID: a.ID},
		ActivatedAt: time.Now(),
	}

	a.RaiseEvent(event)
	return a.Apply(event)
}

func (a *Academia) Desativar(motivo string) error {
	if a.Status == "inativo" {
		return fmt.Errorf("academia já está inativa")
	}

	event := &AcademiaDesativadaEvent{
		BaseEvent:     BaseEvent{EventType: "AcademiaDesativada", AggregateID: a.ID},
		Motivo:        motivo,
		DeactivatedAt: time.Now(),
	}

	a.RaiseEvent(event)
	return a.Apply(event)
}

func (a *Academia) AtualizarCursos(cursos []string) error {
	if a.Status != "ativo" {
		return fmt.Errorf("academia está inativa")
	}

	event := &CursosAtualizadosEvent{
		BaseEvent:  BaseEvent{EventType: "CursosAtualizados", AggregateID: a.ID},
		NovoCursos: cursos,
		UpdatedAt:  time.Now(),
	}

	a.RaiseEvent(event)
	return a.Apply(event)
}

func (a *Academia) AtualizarDados(nome *string, provincia *string, endereco *string, numeroTelefone *string, email *string, website *string, nivelEscolar *string, cursos []string) error {
	if nome == nil && provincia == nil && endereco == nil && numeroTelefone == nil && email == nil && website == nil && nivelEscolar == nil && cursos == nil {
		return fmt.Errorf("nenhum campo para atualizar")
	}

	emailAlterado := email != nil && (a.Email == nil || *a.Email != *email)

	event := &AcademiaDadosAtualizadosEvent{
		BaseEvent:      BaseEvent{EventType: "AcademiaDadosAtualizados", AggregateID: a.ID},
		Nome:           nome,
		Provincia:      provincia,
		Endereco:       endereco,
		NumeroTelefone: numeroTelefone,
		Email:          email,
		Website:        website,
		NivelEscolar:   nivelEscolar,
		Cursos:         cursos,
		EmailAlterado:  emailAlterado,
		UpdatedAt:      time.Now(),
	}

	a.RaiseEvent(event)
	return a.Apply(event)
}

// Event Handlers
func (a *Academia) applyAcademiaCriada(event DomainEvent) error {
	payload := event.GetPayload()
	data, _ := json.Marshal(payload)
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
	a.EmailVerificado = false
	a.Website = ev.Website
	a.NivelEscolar = ev.NivelEscolar
	a.Cursos = ev.Cursos
	a.Status = "inativo"
	a.CreatedAt = ev.CreatedAt
	return nil
}

func (a *Academia) applyEmailVerificado(event DomainEvent) error {
	a.EmailVerificado = true
	return nil
}

func (a *Academia) applyInscricaoAprovada(event DomainEvent) error {
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
	data, _ := json.Marshal(payload)
	var ev CursosAtualizadosEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	a.Cursos = ev.NovoCursos
	return nil
}

func (a *Academia) applyAcademiaDadosAtualizados(event DomainEvent) error {
	payload := event.GetPayload()
	data, _ := json.Marshal(payload)
	var ev AcademiaDadosAtualizadosEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	if ev.Nome != nil {
		a.Nome = *ev.Nome
	}
	if ev.Provincia != nil {
		a.Provincia = *ev.Provincia
	}
	if ev.Endereco != nil {
		a.Endereco = *ev.Endereco
	}
	if ev.NumeroTelefone != nil {
		a.NumeroTelefone = ev.NumeroTelefone
	}
	if ev.Email != nil {
		a.Email = ev.Email
		if ev.EmailAlterado {
			a.EmailVerificado = false
		}
	}
	if ev.Website != nil {
		a.Website = ev.Website
	}
	if ev.NivelEscolar != nil {
		a.NivelEscolar = ev.NivelEscolar
	}
	if ev.Cursos != nil {
		a.Cursos = ev.Cursos
	}

	return nil
}

// Eventos
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

func (e *AcademiaCriadaEvent) GetPayload() interface{} { return e }

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

func (e *InscricaoAprovadaPorAcademiaEvent) GetPayload() interface{} { return e }

type InscricaoReprovadaPorAcademiaEvent struct {
	BaseEvent
	EstudanteID uuid.UUID
	InscricaoID uuid.UUID
	AcademiaID  uuid.UUID
	Motivo      string
	RejectedAt  time.Time
}

func (e *InscricaoReprovadaPorAcademiaEvent) GetPayload() interface{} { return e }

type AcademiaAtivadaEvent struct {
	BaseEvent
	ActivatedAt time.Time
}

func (e *AcademiaAtivadaEvent) GetPayload() interface{} { return e }

type AcademiaDesativadaEvent struct {
	BaseEvent
	Motivo        string
	DeactivatedAt time.Time
}

func (e *AcademiaDesativadaEvent) GetPayload() interface{} { return e }

type CursosAtualizadosEvent struct {
	BaseEvent
	NovoCursos []string
	UpdatedAt  time.Time
}

func (e *CursosAtualizadosEvent) GetPayload() interface{} { return e }

type AcademiaDadosAtualizadosEvent struct {
	BaseEvent
	Nome           *string
	Provincia      *string
	Endereco       *string
	NumeroTelefone *string
	Email          *string
	Website        *string
	NivelEscolar   *string
	Cursos         []string
	EmailAlterado  bool
	UpdatedAt      time.Time
}

func (e *AcademiaDadosAtualizadosEvent) GetPayload() interface{} { return e }