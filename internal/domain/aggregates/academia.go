package aggregates

import (
	"encoding/json"
	"fmt"
	"spuri/internal/utils"
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// Aggregate
// ============================================================================

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
	// AnosAcademicos define os anos do ensino fundamental que esta academia oferece.
	// Obrigatório para tipo="escola" com nivel_escolar em ["fundamental","misto"].
	// Deve ser um subconjunto de primeiro_fundamental…nono_fundamental.
	// NULL/vazio para academias do tipo "superior" ou escolas apenas de médio.
	AnosAcademicos []string
	Status         string
	Cursos         []string
	CreatedAt      time.Time

	TotalEstudantes int
}

func NewAcademia() *Academia {
	return &Academia{
		BaseAggregate: BaseAggregate{
			ID:                uuid.New(),
			Version:           0,
			UncommittedEvents: []DomainEvent{},
		},
		Status:          "inativo",
		AnosAcademicos:  []string{},
		Cursos:          []string{},
		EmailVerificado: false,
	}
}

func (a *Academia) GetType() string { return "Academia" }

// ============================================================================
// Apply dispatcher
// ============================================================================

func (a *Academia) Apply(event DomainEvent) error {
	switch event.GetEventType() {
	case "AcademiaCriada":
		return a.applyAcademiaCriada(event)
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
	case "CategoriaNotaAdicionada":
		return a.applyCategoriaNotaAdicionada(event)
	default:
		return fmt.Errorf("tipo de evento desconhecido: %s", event.GetEventType())
	}
}

// ============================================================================
// Commands
// ============================================================================

// Criar registra o evento de criação da academia.
//
// anosAcademicos — obrigatório quando tipo="escola" E nivel_escolar IN
// ("fundamental","misto"). Deve ser subconjunto de primeiro…nono_fundamental.
// Para tipo="superior" ou nivel_escolar="medio" deve ser nil/vazio.
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
	anosAcademicos []string,
) error {
	if tipo != "escola" && tipo != "superior" {
		return fmt.Errorf("tipo deve ser 'escola' ou 'superior'")
	}
	if nome == "" || codigoAcademia == "" {
		return fmt.Errorf("campos obrigatórios vazios")
	}
	if tipo == "escola" && nivelEscolar == nil {
		return fmt.Errorf("nivel_escolar é obrigatório para escolas")
	}

	// Validar anosAcademicos conforme tipo/nivel
	anosValidados, err := validarAnosAcademicos(tipo, nivelEscolar, anosAcademicos)
	if err != nil {
		return err
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
		AnosAcademicos: anosValidados,
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

// AtualizarDados atualiza campos da academia.
// anosAcademicos — se não-nil, atualiza a lista. As mesmas regras de validação
// de Criar() se aplicam (obrigatório e não-vazio para fundamental/misto, nil para o resto).
// Passe nil para não alterar o campo.
func (a *Academia) AtualizarDados(
	nome *string,
	provincia *string,
	endereco *string,
	numeroTelefone *string,
	email *string,
	website *string,
	nivelEscolar *string,
	cursos []string,
	anosAcademicos []string,
) error {
	if nome == nil && provincia == nil && endereco == nil && numeroTelefone == nil &&
		email == nil && website == nil && nivelEscolar == nil && cursos == nil && anosAcademicos == nil {
		return fmt.Errorf("nenhum campo para atualizar")
	}

	// Se anosAcademicos foi enviado (não-nil), revalidar.
	// Usa nivelEscolar novo (se enviado) ou o atual do aggregate.
	var anosValidados []string
	if anosAcademicos != nil {
		nivelEfetivo := a.NivelEscolar
		if nivelEscolar != nil {
			nivelEfetivo = nivelEscolar
		}
		var err error
		anosValidados, err = validarAnosAcademicos(a.Type, nivelEfetivo, anosAcademicos)
		if err != nil {
			return err
		}
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
		AnosAcademicos: anosValidados, // nil quando não alterado
		Cursos:         cursos,
		EmailAlterado:  emailAlterado,
		UpdatedAt:      time.Now(),
	}

	a.RaiseEvent(event)
	return a.Apply(event)
}

// ============================================================================
// Apply handlers (event sourcing — reconstroem estado do aggregate)
// ============================================================================

func (a *Academia) applyAcademiaCriada(event DomainEvent) error {
	payload := event.GetPayload()
	data, _ := json.Marshal(payload)

	var ev AcademiaCriadaEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

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
	a.AnosAcademicos = ev.AnosAcademicos
	a.Cursos = ev.Cursos
	// Status e EmailVerificado são sempre fixos na criação — independente do payload
	a.Status = "inativo"
	a.EmailVerificado = false
	a.CreatedAt = ev.CreatedAt
	return nil
}

func (a *Academia) applyAcademiaAtivada(_ DomainEvent) error {
	a.Status = "ativo"
	return nil
}

func (a *Academia) applyAcademiaDesativada(_ DomainEvent) error {
	a.Status = "inativo"
	return nil
}

func (a *Academia) applyCursosAtualizados(event DomainEvent) error {
	data, _ := json.Marshal(event.GetPayload())
	var ev CursosAtualizadosEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}
	a.Cursos = ev.NovoCursos
	return nil
}

func (a *Academia) applyAcademiaDadosAtualizados(event DomainEvent) error {
	data, _ := json.Marshal(event.GetPayload())
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
	if ev.AnosAcademicos != nil {
		a.AnosAcademicos = ev.AnosAcademicos
	}
	if ev.Cursos != nil {
		a.Cursos = ev.Cursos
	}
	return nil
}

func (a *Academia) applyEmailVerificado(_ DomainEvent) error {
	a.EmailVerificado = true
	return nil
}

func (a *Academia) applyCategoriaNotaAdicionada(_ DomainEvent) error {
	// Não altera estado do aggregate Academia — apenas registra o evento.
	return nil
}

// ============================================================================
// Validação interna
// ============================================================================

func validarAnosAcademicos(tipo string, nivelEscolar *string, anos []string) ([]string, error) {
	// Universidades nunca têm anos fundamentais
	if tipo == "superior" {
		return nil, nil
	}

	if nivelEscolar == nil {
		return nil, nil
	}

	switch *nivelEscolar {
	case "fundamental", "misto":
		// Obrigatório e não pode ser vazio
		if len(anos) == 0 {
			return nil, fmt.Errorf(
				"anos_academicos é obrigatório e não pode estar vazio para escolas de nivel_escolar '%s'. "+
					"Informe ao menos um ano (ex: primeiro_fundamental, segundo_fundamental, etc.)",
				*nivelEscolar,
			)
		}
		if err := utils.ValidateAnosFundamental(anos); err != nil {
			return nil, err
		}
		return anos, nil

	case "medio":
		// Escolas apenas de médio não têm anos fundamentais
		if len(anos) > 0 {
			return nil, fmt.Errorf("escolas de nivel_escolar 'medio' não devem definir anos_academicos (use os anos no curso)")
		}
		return nil, nil
	}

	return nil, fmt.Errorf("nivel_escolar desconhecido: '%s'", *nivelEscolar)
}

// ============================================================================
// Eventos
// ============================================================================

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
	AnosAcademicos []string
	Cursos         []string
	CreatedAt      time.Time
}

func (e *AcademiaCriadaEvent) GetPayload() interface{} { return e }

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
	// AnosAcademicos — nil = campo não alterado nesta operação
	AnosAcademicos []string
	Cursos         []string
	EmailAlterado  bool
	UpdatedAt      time.Time
}

func (e *AcademiaDadosAtualizadosEvent) GetPayload() interface{} { return e }