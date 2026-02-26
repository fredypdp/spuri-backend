package aggregates

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
)

type Estudante struct {
	BaseAggregate

	Nome                  string
	CodigoEstudante       string
	SenhaHash             string
	Email                 *string
	Telefone              *string
	EmailVerificado       bool
	BilheteIdentidade     *string
	BilheteIdentidadeResp *string
	CodigoAcademia        *string
	Status                string
	AnoEscolar            *string
	AnoSuperior           *string
	CursoMedioID          *uuid.UUID
	CursoSuperiorID       *uuid.UUID
	StatusEscolarFundamental string
	StatusEscolarMedio       string
	StatusSuperior           string
	CreatedAt             time.Time
	Genero                string

	Inscricoes []Inscricao
}

type Inscricao struct {
	ID             uuid.UUID
	CodigoAcademia string
	Tipo           string
	AnoInscricao   string
	CursoID        *uuid.UUID
	Status         string
	StatusUsado    bool
	CreatedAt      time.Time
}

func NewEstudante() *Estudante {
	return &Estudante{
		BaseAggregate: BaseAggregate{
			ID:                uuid.New(),
			Version:           0,
			UncommittedEvents: []DomainEvent{},
		},
		Status:                   "inativo",
		StatusEscolarFundamental: "inativo",
		StatusEscolarMedio:       "inativo",
		StatusSuperior:           "inativo",
		EmailVerificado:          false,
		Inscricoes:               []Inscricao{},
	}
}

func (e *Estudante) GetType() string {
	return "Estudante"
}

func (e *Estudante) Apply(event DomainEvent) error {
	switch event.GetEventType() {
	case "EstudanteCriado":
		return e.applyEstudanteCriado(event)
	case "EstudanteCriadoComVinculo":
		return e.applyEstudanteCriadoComVinculo(event)
	case "NotasRegistradas":
		return e.applyNotasRegistradas(event)
	case "NotaAtualizada":
		return e.applyNotaAtualizada(event)
	case "FaltasRegistradas":
		return e.applyFaltasRegistradas(event)
	case "EstudanteInscrito":
		return e.applyEstudanteInscrito(event)
	case "InscricaoAprovada":
		return e.applyInscricaoAprovada(event)
	case "InscricaoReprovada":
		return e.applyInscricaoReprovada(event)
	case "EstudanteVinculado":
		return e.applyEstudanteVinculado(event)
	case "StatusSuperiorAtualizado":
		return e.applyStatusSuperiorAtualizado(event)
	case "DadosPessoaisAtualizados":
		return e.applyDadosPessoaisAtualizados(event)
	case "DadosAcademicosAtualizados":
		return e.applyDadosAcademicosAtualizados(event)
	case "EmailVerificado":
		return e.applyEmailVerificado(event)
	case "CursoAlterado":
		return e.applyCursoAlterado(event)
	case "AprovacaoAnoRegistrada":
		return e.applyAprovacaoAnoRegistrada(event)
	case "StatusEscolarFundamentalAtualizado":
		return e.applyStatusEscolarFundamentalAtualizado(event)
	case "StatusEscolarMedioAtualizado":
		return e.applyStatusEscolarMedioAtualizado(event)
	default:
		return fmt.Errorf("tipo de evento desconhecido: %s", event.GetEventType())
	}
}

func (e *Estudante) Criar(
	nome string,
	codigoEstudante string,
	senhaHash string,
	email *string,
	telefone *string,
	bilhete *string,
	bilheteResp *string,
	anoEscolar *string,
	anoSuperior *string,
	cursoMedioID *uuid.UUID,
	cursoSuperiorID *uuid.UUID,
	statusEscolarFundamental *string,
	statusEscolarMedio *string,
	statusSuperior *string,
	genero string,
) error {
	if nome == "" || codigoEstudante == "" || senhaHash == "" {
		return fmt.Errorf("campos obrigatórios vazios")
	}

	if genero != "masculino" && genero != "feminino" {
		return fmt.Errorf("genero deve ser 'masculino' ou 'feminino'")
	}

	if bilhete == nil && bilheteResp == nil {
		return fmt.Errorf("pelo menos um bilhete de identidade do estudante é obrigatório")
	}

	if bilhete != nil && bilheteResp != nil && *bilhete == *bilheteResp {
		return fmt.Errorf("bilhete de identidade do estudante e bilhete do responsável não podem ser iguais")
	}

	statusFund := "inativo"
	statusMed := "inativo"
	statusSup := "inativo"

	validStatus := map[string]bool{"inativo": true, "em_andamento": true, "finalizado": true}

	if statusEscolarFundamental != nil {
		if !validStatus[*statusEscolarFundamental] {
			return fmt.Errorf("status_escolar_fundamental inválido")
		}
		statusFund = *statusEscolarFundamental
	}
	if statusEscolarMedio != nil {
		if !validStatus[*statusEscolarMedio] {
			return fmt.Errorf("status_escolar_medio inválido")
		}
		statusMed = *statusEscolarMedio
	}
	if statusSuperior != nil {
		if !validStatus[*statusSuperior] {
			return fmt.Errorf("status_superior inválido")
		}
		statusSup = *statusSuperior
	}

	event := &EstudanteCriadoEvent{
		BaseEvent:                BaseEvent{EventType: "EstudanteCriado", AggregateID: e.ID},
		Nome:                     nome,
		CodigoEstudante:          codigoEstudante,
		SenhaHash:                senhaHash,
		Email:                    email,
		Telefone:                 telefone,
		BilheteIdentidade:        bilhete,
		BilheteIdentidadeResp:    bilheteResp,
		AnoEscolar:               anoEscolar,
		AnoSuperior:              anoSuperior,
		CursoMedioID:             cursoMedioID,
		CursoSuperiorID:          cursoSuperiorID,
		StatusEscolarFundamental: statusFund,
		StatusEscolarMedio:       statusMed,
		StatusSuperior:           statusSup,
		CreatedAt:                time.Now(),
		Genero:                   genero,
	}

	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) CriarComVinculo(
	nome string,
	codigoEstudante string,
	senhaHash string,
	email *string,
	telefone *string,
	bilhete *string,
	bilheteResp *string,
	anoEscolar *string,
	anoSuperior *string,
	cursoMedioID *uuid.UUID,
	cursoSuperiorID *uuid.UUID,
	statusEscolarFundamental *string,
	statusEscolarMedio *string,
	statusSuperior *string,
	codigoAcademia string,
	genero string,
) error {
	if nome == "" || codigoEstudante == "" || senhaHash == "" {
		return fmt.Errorf("campos obrigatórios vazios")
	}

	if genero != "masculino" && genero != "feminino" {
		return fmt.Errorf("genero deve ser 'masculino' ou 'feminino'")
	}

	if bilhete == nil && bilheteResp == nil {
		return fmt.Errorf("pelo menos um bilhete de identidade do estudante é obrigatório")
	}

	if bilhete != nil && bilheteResp != nil && *bilhete == *bilheteResp {
		return fmt.Errorf("bilhete de identidade do estudante e bilhete do responsável não podem ser iguais")
	}

	if codigoAcademia == "" {
		return fmt.Errorf("código da academia é obrigatório")
	}

	statusFund := "inativo"
	statusMed := "inativo"
	statusSup := "inativo"

	validStatus := map[string]bool{"inativo": true, "em_andamento": true, "finalizado": true}

	if statusEscolarFundamental != nil {
		if !validStatus[*statusEscolarFundamental] {
			return fmt.Errorf("status_escolar_fundamental inválido")
		}
		statusFund = *statusEscolarFundamental
	}
	if statusEscolarMedio != nil {
		if !validStatus[*statusEscolarMedio] {
			return fmt.Errorf("status_escolar_medio inválido")
		}
		statusMed = *statusEscolarMedio
	}
	if statusSuperior != nil {
		if !validStatus[*statusSuperior] {
			return fmt.Errorf("status_superior inválido")
		}
		statusSup = *statusSuperior
	}

	log.Printf("[DEBUG] CriarComVinculo - AnoEscolar=%v, CursoMedioID=%v", anoEscolar, cursoMedioID)

	event := &EstudanteCriadoComVinculoEvent{
		BaseEvent:                BaseEvent{EventType: "EstudanteCriadoComVinculo", AggregateID: e.ID},
		Nome:                     nome,
		CodigoEstudante:          codigoEstudante,
		SenhaHash:                senhaHash,
		Email:                    email,
		Telefone:                 telefone,
		BilheteIdentidade:        bilhete,
		BilheteIdentidadeResp:    bilheteResp,
		AnoEscolar:               anoEscolar,
		AnoSuperior:              anoSuperior,
		CursoMedioID:             cursoMedioID,
		CursoSuperiorID:          cursoSuperiorID,
		StatusEscolarFundamental: statusFund,
		StatusEscolarMedio:       statusMed,
		StatusSuperior:           statusSup,
		CodigoAcademia:           codigoAcademia,
		CreatedAt:                time.Now(),
		Genero:                   genero,
	}

	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) AtualizarStatusSuperior(novoStatus string) error {
	validStatus := map[string]bool{"inativo": true, "em_andamento": true, "finalizado": true}
	if !validStatus[novoStatus] {
		return fmt.Errorf("status inválido")
	}

	if (novoStatus == "em_andamento" || novoStatus == "finalizado") &&
		e.StatusEscolarFundamental != "finalizado" && e.StatusEscolarMedio != "finalizado" {
		return fmt.Errorf("status_superior só pode ser atualizado se status_escolar_fundamental e status_escolar_medio estiverem como 'finalizado'")
	}

	event := &StatusSuperiorAtualizadoEvent{
		BaseEvent:  BaseEvent{EventType: "StatusSuperiorAtualizado", AggregateID: e.ID},
		NovoStatus: novoStatus,
		UpdatedAt:  time.Now(),
	}

	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) AtualizarDadosPessoais(nome *string, email *string, telefone *string, bilheteIdentidade *string, bilheteIdentidadeResp *string) error {
	if nome == nil && email == nil && telefone == nil && bilheteIdentidade == nil && bilheteIdentidadeResp == nil {
		return fmt.Errorf("nenhum campo para atualizar")
	}

	emailAlterado := email != nil && (e.Email == nil || *e.Email != *email)

	event := &DadosPessoaisAtualizadosEvent{
		BaseEvent:             BaseEvent{EventType: "DadosPessoaisAtualizados", AggregateID: e.ID},
		Nome:                  nome,
		Email:                 email,
		Telefone:              telefone,
		BilheteIdentidade:     bilheteIdentidade,
		BilheteIdentidadeResp: bilheteIdentidadeResp,
		EmailAlterado:         emailAlterado,
		UpdatedAt:             time.Now(),
	}

	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) AtualizarDadosAcademicos(anoEscolar *string, anoSuperior *string, cursoMedioID *uuid.UUID, cursoSuperiorID *uuid.UUID) error {
	if anoEscolar == nil && anoSuperior == nil && cursoMedioID == nil && cursoSuperiorID == nil {
		return fmt.Errorf("nenhum campo para atualizar")
	}

	event := &DadosAcademicosAtualizadosEvent{
		BaseEvent:       BaseEvent{EventType: "DadosAcademicosAtualizados", AggregateID: e.ID},
		AnoEscolar:      anoEscolar,
		AnoSuperior:     anoSuperior,
		CursoMedioID:    cursoMedioID,
		CursoSuperiorID: cursoSuperiorID,
		UpdatedAt:       time.Now(),
	}

	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) RegistrarFalta(codigoAcademia string, anoLectivo string, data time.Time, materiaDisciplinarID uuid.UUID, quantidade int, observacao *string) error {
	if e.CodigoAcademia == nil || *e.CodigoAcademia != codigoAcademia {
		return fmt.Errorf("estudante não pertence a esta academia")
	}

	if quantidade <= 0 {
		return fmt.Errorf("quantidade deve ser maior que zero")
	}

	event := &FaltasRegistradasEvent{
		BaseEvent:            BaseEvent{EventType: "FaltasRegistradas", AggregateID: e.ID},
		CodigoEstudante:      e.CodigoEstudante,
		CodigoAcademia:       codigoAcademia,
		AnoLectivo:           anoLectivo,
		Data:                 data,
		MateriaDisciplinarID: materiaDisciplinarID,
		Quantidade:           quantidade,
		Observacao:           observacao,
		RegisteredAt:         time.Now(),
	}

	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) SolicitarInscricao(codigoAcademia string, tipo string, anoInscricao string, cursoID *uuid.UUID) error {
	if tipo != "escola" && tipo != "universidade" {
		return fmt.Errorf("tipo deve ser 'escola' ou 'universidade'")
	}

	if e.CodigoAcademia != nil && *e.CodigoAcademia == codigoAcademia {
		return fmt.Errorf("você já está matriculado nesta academia")
	}

	for _, inscricao := range e.Inscricoes {
		if inscricao.CodigoAcademia == codigoAcademia && inscricao.Status == "espera" {
			return fmt.Errorf("você já possui uma inscrição pendente nesta academia")
		}
	}

	inscricaoID := uuid.New()
	event := &EstudanteInscritoEvent{
		BaseEvent:      BaseEvent{EventType: "EstudanteInscrito", AggregateID: e.ID},
		InscricaoID:    inscricaoID,
		CodigoAcademia: codigoAcademia,
		Tipo:           tipo,
		AnoInscricao:   anoInscricao,
		CursoID:        cursoID,
		CreatedAt:      time.Now(),
	}

	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) AlterarCurso(cursoID uuid.UUID, tipoEnsino string) error {
	if tipoEnsino != "medio" && tipoEnsino != "superior" {
		return fmt.Errorf("tipo_ensino deve ser 'medio' ou 'superior'")
	}

	event := &CursoAlteradoEvent{
		BaseEvent:  BaseEvent{EventType: "CursoAlterado", AggregateID: e.ID},
		CursoID:    cursoID,
		TipoEnsino: tipoEnsino,
		UpdatedAt:  time.Now(),
	}

	e.RaiseEvent(event)
	return e.Apply(event)
}

// ============================================================================
// Apply handlers
// ============================================================================

func (e *Estudante) applyEstudanteCriado(event DomainEvent) error {
	data, _ := json.Marshal(event.GetPayload())
	var ev EstudanteCriadoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	e.Nome = ev.Nome
	e.CodigoEstudante = ev.CodigoEstudante
	e.SenhaHash = ev.SenhaHash
	e.Email = ev.Email
	e.Telefone = ev.Telefone
	e.BilheteIdentidade = ev.BilheteIdentidade
	e.BilheteIdentidadeResp = ev.BilheteIdentidadeResp
	e.AnoEscolar = ev.AnoEscolar
	e.AnoSuperior = ev.AnoSuperior
	e.CursoMedioID = ev.CursoMedioID
	e.CursoSuperiorID = ev.CursoSuperiorID
	e.StatusEscolarFundamental = ev.StatusEscolarFundamental
	e.StatusEscolarMedio = ev.StatusEscolarMedio
	e.StatusSuperior = ev.StatusSuperior
	e.Status = "inativo"
	e.CreatedAt = ev.CreatedAt
	e.Genero = ev.Genero
	return nil
}

func (e *Estudante) applyEstudanteCriadoComVinculo(event DomainEvent) error {
	data, _ := json.Marshal(event.GetPayload())
	var ev EstudanteCriadoComVinculoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	e.Nome = ev.Nome
	e.CodigoEstudante = ev.CodigoEstudante
	e.SenhaHash = ev.SenhaHash
	e.Email = ev.Email
	e.Telefone = ev.Telefone
	e.BilheteIdentidade = ev.BilheteIdentidade
	e.BilheteIdentidadeResp = ev.BilheteIdentidadeResp
	e.AnoEscolar = ev.AnoEscolar
	e.AnoSuperior = ev.AnoSuperior
	e.CursoMedioID = ev.CursoMedioID
	e.CursoSuperiorID = ev.CursoSuperiorID
	e.StatusEscolarFundamental = ev.StatusEscolarFundamental
	e.StatusEscolarMedio = ev.StatusEscolarMedio
	e.StatusSuperior = ev.StatusSuperior
	e.CodigoAcademia = &ev.CodigoAcademia
	e.Status = "ativo"
	e.CreatedAt = ev.CreatedAt
	e.Genero = ev.Genero

	log.Printf("[DEBUG] applyEstudanteCriadoComVinculo - AnoEscolar=%v, CursoMedioID=%v", e.AnoEscolar, e.CursoMedioID)
	return nil
}

func (e *Estudante) applyFaltasRegistradas(event DomainEvent) error { return nil }

func (e *Estudante) applyEstudanteInscrito(event DomainEvent) error {
	data, _ := json.Marshal(event.GetPayload())
	var ev EstudanteInscritoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	inscricao := Inscricao{
		ID:             ev.InscricaoID,
		CodigoAcademia: ev.CodigoAcademia,
		Tipo:           ev.Tipo,
		AnoInscricao:   ev.AnoInscricao,
		CursoID:        ev.CursoID,
		Status:         "pendente",
		StatusUsado:    false,
		CreatedAt:      ev.CreatedAt,
	}
	e.Inscricoes = append(e.Inscricoes, inscricao)
	return nil
}

func (e *Estudante) applyInscricaoAprovada(event DomainEvent) error {
	data, _ := json.Marshal(event.GetPayload())
	var ev InscricaoAprovadaEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	for i, insc := range e.Inscricoes {
		if insc.ID == ev.InscricaoID {
			e.Inscricoes[i].Status = "aprovado"
			break
		}
	}
	return nil
}

func (e *Estudante) applyInscricaoReprovada(event DomainEvent) error {
	data, _ := json.Marshal(event.GetPayload())
	var ev InscricaoReprovadaEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	for i, insc := range e.Inscricoes {
		if insc.ID == ev.InscricaoID {
			e.Inscricoes[i].Status = "reprovado"
			break
		}
	}
	return nil
}

func (e *Estudante) applyEstudanteVinculado(event DomainEvent) error {
	data, _ := json.Marshal(event.GetPayload())
	var ev EstudanteVinculadoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	e.CodigoAcademia = &ev.CodigoAcademia
	e.Status = "ativo"

	for i, insc := range e.Inscricoes {
		if insc.ID == ev.InscricaoID {
			e.Inscricoes[i].StatusUsado = true
			break
		}
	}
	return nil
}

func (e *Estudante) applyStatusSuperiorAtualizado(event DomainEvent) error {
	data, _ := json.Marshal(event.GetPayload())
	var ev StatusSuperiorAtualizadoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}
	e.StatusSuperior = ev.NovoStatus
	return nil
}

func (e *Estudante) applyDadosPessoaisAtualizados(event DomainEvent) error {
	data, _ := json.Marshal(event.GetPayload())
	var ev DadosPessoaisAtualizadosEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	if ev.Nome != nil {
		e.Nome = *ev.Nome
	}
	if ev.Email != nil {
		e.Email = ev.Email
		if ev.EmailAlterado {
			e.EmailVerificado = false
		}
	}
	if ev.Telefone != nil {
		e.Telefone = ev.Telefone
	}
	if ev.BilheteIdentidade != nil {
		e.BilheteIdentidade = ev.BilheteIdentidade
	}
	if ev.BilheteIdentidadeResp != nil {
		e.BilheteIdentidadeResp = ev.BilheteIdentidadeResp
	}
	return nil
}

func (e *Estudante) applyDadosAcademicosAtualizados(event DomainEvent) error {
	data, _ := json.Marshal(event.GetPayload())
	var ev DadosAcademicosAtualizadosEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	if ev.AnoEscolar != nil {
		e.AnoEscolar = ev.AnoEscolar
	}
	if ev.AnoSuperior != nil {
		e.AnoSuperior = ev.AnoSuperior
	}
	if ev.CursoMedioID != nil {
		e.CursoMedioID = ev.CursoMedioID
	}
	if ev.CursoSuperiorID != nil {
		e.CursoSuperiorID = ev.CursoSuperiorID
	}
	return nil
}

func (e *Estudante) applyEmailVerificado(event DomainEvent) error {
	e.EmailVerificado = true
	return nil
}

func (e *Estudante) applyCursoAlterado(event DomainEvent) error {
	data, _ := json.Marshal(event.GetPayload())
	var ev CursoAlteradoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	if ev.TipoEnsino == "medio" {
		e.CursoMedioID = &ev.CursoID
	} else {
		e.CursoSuperiorID = &ev.CursoID
	}
	return nil
}

// ============================================================================
// Eventos
// ============================================================================

type EstudanteCriadoEvent struct {
	BaseEvent
	Nome                     string
	CodigoEstudante          string
	SenhaHash                string
	Email                    *string
	Telefone                 *string
	BilheteIdentidade        *string
	BilheteIdentidadeResp    *string
	AnoEscolar               *string
	AnoSuperior              *string
	CursoMedioID             *uuid.UUID
	CursoSuperiorID          *uuid.UUID
	StatusEscolarFundamental string
	StatusEscolarMedio       string
	StatusSuperior           string
	CreatedAt                time.Time
	Genero                   string
}

func (e *EstudanteCriadoEvent) GetPayload() interface{} { return e }

type EstudanteCriadoComVinculoEvent struct {
	BaseEvent
	Nome                     string
	CodigoEstudante          string
	SenhaHash                string
	Email                    *string
	Telefone                 *string
	BilheteIdentidade        *string
	BilheteIdentidadeResp    *string
	AnoEscolar               *string
	AnoSuperior              *string
	CursoMedioID             *uuid.UUID
	CursoSuperiorID          *uuid.UUID
	StatusEscolarFundamental string
	StatusEscolarMedio       string
	StatusSuperior           string
	CodigoAcademia           string
	CreatedAt                time.Time
	Genero                   string
}

func (e *EstudanteCriadoComVinculoEvent) GetPayload() interface{} { return e }

type FaltasRegistradasEvent struct {
	BaseEvent
	CodigoEstudante      string
	CodigoAcademia       string
	AnoLectivo           string
	Data                 time.Time
	MateriaDisciplinarID uuid.UUID
	Quantidade           int
	Observacao           *string
	RegisteredAt         time.Time
}

func (e *FaltasRegistradasEvent) GetPayload() interface{} { return e }

type EstudanteInscritoEvent struct {
	BaseEvent
	InscricaoID    uuid.UUID
	CodigoAcademia string
	Tipo           string
	AnoInscricao   string
	CursoID        *uuid.UUID
	CreatedAt      time.Time
}

func (e *EstudanteInscritoEvent) GetPayload() interface{} { return e }

type InscricaoAprovadaEvent struct {
	BaseEvent
	InscricaoID uuid.UUID
	AprovadoAt  time.Time
}

func (e *InscricaoAprovadaEvent) GetPayload() interface{} { return e }

type InscricaoReprovadaEvent struct {
	BaseEvent
	InscricaoID  uuid.UUID
	Observacao   *string
	ReprovadoAt  time.Time
}

func (e *InscricaoReprovadaEvent) GetPayload() interface{} { return e }

type EstudanteVinculadoEvent struct {
	BaseEvent
	InscricaoID    uuid.UUID
	CodigoAcademia string
	VinculadoAt    time.Time
}

func (e *EstudanteVinculadoEvent) GetPayload() interface{} { return e }

type StatusSuperiorAtualizadoEvent struct {
	BaseEvent
	NovoStatus string
	UpdatedAt  time.Time
}

func (e *StatusSuperiorAtualizadoEvent) GetPayload() interface{} { return e }

type DadosPessoaisAtualizadosEvent struct {
	BaseEvent
	Nome                  *string
	Email                 *string
	Telefone              *string
	BilheteIdentidade     *string
	BilheteIdentidadeResp *string
	EmailAlterado         bool
	UpdatedAt             time.Time
}

func (e *DadosPessoaisAtualizadosEvent) GetPayload() interface{} { return e }

type DadosAcademicosAtualizadosEvent struct {
	BaseEvent
	AnoEscolar      *string
	AnoSuperior     *string
	CursoMedioID    *uuid.UUID
	CursoSuperiorID *uuid.UUID
	UpdatedAt       time.Time
}

func (e *DadosAcademicosAtualizadosEvent) GetPayload() interface{} { return e }

type CursoAlteradoEvent struct {
	BaseEvent
	CursoID    uuid.UUID
	TipoEnsino string
	UpdatedAt  time.Time
}

func (e *CursoAlteradoEvent) GetPayload() interface{} { return e }

// VincularAcademia vincula o estudante a uma academia via inscrição aprovada
func (e *Estudante) VincularAcademia(inscricaoID uuid.UUID) error {
	var inscricao *Inscricao
	for i := range e.Inscricoes {
		if e.Inscricoes[i].ID == inscricaoID {
			inscricao = &e.Inscricoes[i]
			break
		}
	}

	if inscricao == nil {
		return fmt.Errorf("inscrição não encontrada")
	}

	if inscricao.Status != "aprovado" {
		return fmt.Errorf("inscrição não foi aprovada")
	}

	if inscricao.StatusUsado {
		return fmt.Errorf("esta inscrição já foi utilizada")
	}

	event := &EstudanteVinculadoEvent{
		BaseEvent:      BaseEvent{EventType: "EstudanteVinculado", AggregateID: e.ID},
		InscricaoID:    inscricaoID,
		CodigoAcademia: inscricao.CodigoAcademia,
		VinculadoAt:    time.Now(),
	}

	e.RaiseEvent(event)
	return e.Apply(event)
}