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
	AnoEscolar            *string // ano atual no ciclo fundamental
	AnoEscolarMedio       *string // ano atual no ciclo médio
	AnoSuperior           *string // ano atual no ciclo superior
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
	anoEscolarMedio *string,
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
		AnoEscolarMedio:          anoEscolarMedio,
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
	anoEscolarMedio *string,
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

	log.Printf("[DEBUG] CriarComVinculo - AnoEscolar=%v, AnoEscolarMedio=%v, CursoMedioID=%v", anoEscolar, anoEscolarMedio, cursoMedioID)

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
		AnoEscolarMedio:          anoEscolarMedio,
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

func (e *Estudante) AtualizarDadosAcademicos(anoEscolar *string, anoEscolarMedio *string, anoSuperior *string, cursoMedioID *uuid.UUID, cursoSuperiorID *uuid.UUID) error {
	if anoEscolar == nil && anoEscolarMedio == nil && anoSuperior == nil && cursoMedioID == nil && cursoSuperiorID == nil {
		return fmt.Errorf("nenhum campo para atualizar")
	}

	event := &DadosAcademicosAtualizadosEvent{
		BaseEvent:       BaseEvent{EventType: "DadosAcademicosAtualizados", AggregateID: e.ID},
		AnoEscolar:      anoEscolar,
		AnoEscolarMedio: anoEscolarMedio,
		AnoSuperior:     anoSuperior,
		CursoMedioID:    cursoMedioID,
		CursoSuperiorID: cursoSuperiorID,
		UpdatedAt:       time.Now(),
	}

	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) RegistrarFalta(
    codigoAcademia string,
    anoLectivo string,
    anoAcademico string,
    data time.Time,
    materiaDisciplinarID uuid.UUID,
    quantidade int,
    observacao *string,
) error {
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
		AnoAcademico:         anoAcademico,
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
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return err
	}
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
	e.AnoEscolarMedio = ev.AnoEscolarMedio
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
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return err
	}
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
	e.AnoEscolarMedio = ev.AnoEscolarMedio
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
	return nil
}

func (e *Estudante) applyInscricaoAprovada(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return err
	}
	var ev InscricaoAprovadaPorAcademiaEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	for i, inscricao := range e.Inscricoes {
		if inscricao.ID == ev.InscricaoID {
			e.Inscricoes[i].Status = "aprovado"
			e.Inscricoes[i].StatusUsado = true
			break
		}
	}

	e.CodigoAcademia = &ev.CodigoAcademia
	e.Status = "ativo"
	return nil
}

func (e *Estudante) applyInscricaoReprovada(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return err
	}
	var ev InscricaoReprovadaPorAcademiaEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	for i, inscricao := range e.Inscricoes {
		if inscricao.ID == ev.InscricaoID {
			e.Inscricoes[i].Status = "reprovado"
			e.Inscricoes[i].StatusUsado = true
			break
		}
	}
	return nil
}

func (e *Estudante) applyEstudanteVinculado(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return err
	}
	var ev EstudanteVinculadoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	for i := range e.Inscricoes {
		if e.Inscricoes[i].ID == ev.InscricaoID {
			e.Inscricoes[i].StatusUsado = true
			break
		}
	}
	e.CodigoAcademia = &ev.CodigoAcademia
	e.Status = "ativo"
	return nil
}

func (e *Estudante) applyStatusSuperiorAtualizado(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return err
	}
	var ev StatusSuperiorAtualizadoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	e.StatusSuperior = ev.NovoStatus
	return nil
}

func (e *Estudante) applyDadosPessoaisAtualizados(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return err
	}
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
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return err
	}
	var ev DadosAcademicosAtualizadosEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	if ev.AnoEscolar != nil {
		e.AnoEscolar = ev.AnoEscolar
	}
	if ev.AnoEscolarMedio != nil {
		e.AnoEscolarMedio = ev.AnoEscolarMedio
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

func (e *Estudante) applyEstudanteInscrito(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return err
	}
	var ev EstudanteInscritoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	e.Inscricoes = append(e.Inscricoes, Inscricao{
		ID:             ev.InscricaoID,
		CodigoAcademia: ev.CodigoAcademia,
		Tipo:           ev.Tipo,
		AnoInscricao:   ev.AnoInscricao,
		CursoID:        ev.CursoID,
		Status:         "espera",
		StatusUsado:    false,
		CreatedAt:      ev.CreatedAt,
	})
	return nil
}


// VincularAcademia — estudante usa inscrição aprovada para se vincular.
func (e *Estudante) VincularAcademia(inscricaoID uuid.UUID) error {
	var alvo *Inscricao
	for i := range e.Inscricoes {
		if e.Inscricoes[i].ID == inscricaoID {
			alvo = &e.Inscricoes[i]
			break
		}
	}
	if alvo == nil {
		return fmt.Errorf("inscrição não encontrada")
	}
	if alvo.Status != "aprovado" {
		return fmt.Errorf("inscrição não está aprovada")
	}
	if alvo.StatusUsado {
		return fmt.Errorf("inscrição já foi utilizada")
	}

	event := &EstudanteVinculadoEvent{
		BaseEvent:      BaseEvent{EventType: "EstudanteVinculado", AggregateID: e.ID},
		InscricaoID:    inscricaoID,
		CodigoAcademia: alvo.CodigoAcademia,
		VinculadoAt:    time.Now(),
	}
	e.RaiseEvent(event)
	return e.Apply(event)
}

// AtualizarStatusEscolarFundamental — atualiza status_escolar_fundamental.
func (e *Estudante) AtualizarStatusEscolarFundamental(novoStatus string) error {
	validStatus := map[string]bool{"inativo": true, "em_andamento": true, "finalizado": true}
	if !validStatus[novoStatus] {
		return fmt.Errorf("status inválido: %s", novoStatus)
	}
	event := &StatusEscolarFundamentalAtualizadoEvent{
		BaseEvent:  BaseEvent{EventType: "StatusEscolarFundamentalAtualizado", AggregateID: e.ID},
		NovoStatus: novoStatus,
		UpdatedAt:  time.Now(),
	}
	e.RaiseEvent(event)
	return e.Apply(event)
}

// AtualizarStatusEscolarMedio — atualiza status_escolar_medio.
func (e *Estudante) AtualizarStatusEscolarMedio(novoStatus string) error {
	validStatus := map[string]bool{"inativo": true, "em_andamento": true, "finalizado": true}
	if !validStatus[novoStatus] {
		return fmt.Errorf("status inválido: %s", novoStatus)
	}
	event := &StatusEscolarMedioAtualizadoEvent{
		BaseEvent:  BaseEvent{EventType: "StatusEscolarMedioAtualizado", AggregateID: e.ID},
		NovoStatus: novoStatus,
		UpdatedAt:  time.Now(),
	}
	e.RaiseEvent(event)
	return e.Apply(event)
}

// RegistrarAprovacaoAno — academia registra aprovação ou reprovação de ano escolar.
// Aprovado=true: avança para ProximoNivel (ou finaliza se último ano).
// Aprovado=false: apenas registra reprovação, nenhum estado é alterado.
func (e *Estudante) RegistrarAprovacaoAno(
	codigoAcademia string,
	anoLectivo string,
	tipoEnsino string,
	nivelAtual string,
	proximoNivel *string,
	aprovado bool,
	observacao *string,
) error {
	if tipoEnsino != "fundamental" && tipoEnsino != "medio" && tipoEnsino != "superior" {
		return fmt.Errorf("tipo_ensino deve ser 'fundamental', 'medio' ou 'superior'")
	}
	if e.CodigoAcademia == nil || *e.CodigoAcademia != codigoAcademia {
		return fmt.Errorf("estudante não pertence a esta academia")
	}

	event := &AprovacaoAnoRegistradaEvent{
		BaseEvent:    BaseEvent{EventType: "AprovacaoAnoRegistrada", AggregateID: e.ID},
		CodigoEstudante: e.CodigoEstudante,
		CodigoAcademia:  codigoAcademia,
		AnoLectivo:      anoLectivo,
		TipoEnsino:      tipoEnsino,
		NivelAtual:      nivelAtual,
		ProximoNivel:    proximoNivel,
		Aprovado:        aprovado,
		Observacao:      observacao,
		RegisteredAt:    time.Now(),
	}
	e.RaiseEvent(event)
	return e.Apply(event)
}

// ─── apply handlers ausentes ──────────────────────────────────────────────────

// applyFaltasRegistradas — faltas não alteram estado do aggregate.
func (e *Estudante) applyFaltasRegistradas(event DomainEvent) error {
	return nil
}

func (e *Estudante) applyAprovacaoAnoRegistrada(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return err
	}
	var ev AprovacaoAnoRegistradaEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	// Reprovação: não altera estado
	if !ev.Aprovado {
		return nil
	}

	// Aprovação: avança para próximo nível ou finaliza
	if ev.ProximoNivel != nil {
		switch ev.TipoEnsino {
		case "fundamental":
			e.AnoEscolar = ev.ProximoNivel
		case "medio":
			e.AnoEscolarMedio = ev.ProximoNivel
		case "superior":
			e.AnoSuperior = ev.ProximoNivel
		}
	} else {
		// Último ano — finaliza o status do ciclo
		switch ev.TipoEnsino {
		case "fundamental":
			e.StatusEscolarFundamental = "finalizado"
		case "medio":
			e.StatusEscolarMedio = "finalizado"
		case "superior":
			e.StatusSuperior = "finalizado"
		}
	}
	return nil
}

func (e *Estudante) applyStatusEscolarFundamentalAtualizado(event DomainEvent) error {
	data, _ := json.Marshal(event.GetPayload())
	var ev StatusEscolarFundamentalAtualizadoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}
	e.StatusEscolarFundamental = ev.NovoStatus
	return nil
}

func (e *Estudante) applyStatusEscolarMedioAtualizado(event DomainEvent) error {
	data, _ := json.Marshal(event.GetPayload())
	var ev StatusEscolarMedioAtualizadoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}
	e.StatusEscolarMedio = ev.NovoStatus
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
	AnoEscolarMedio          *string // novo
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
	AnoEscolarMedio          *string // novo
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
	AnoAcademico         string
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
	AnoEscolarMedio *string // novo
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