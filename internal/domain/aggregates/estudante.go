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

// Estudante — genero e data_nascimento são sempre preenchidos (obrigatórios no cadastro).
type Estudante struct {
	BaseAggregate

	Nome                     string
	CodigoEstudante          string
	SenhaHash                string
	Email                    *string
	Telefone                 *string
	BilheteIdentidade        *string
	BilheteIdentidadeResp    *string
	Genero                   string    // obrigatório
	DataNascimento           time.Time // obrigatório; valor zero indica aggregate não carregado
	CodigoAcademia           *string
	Status                   string
	StatusEscolarFundamental string
	StatusEscolarMedio       string
	StatusSuperior           string
	AnoEscolar               *string
	AnoEscolarMedio          *string
	AnoSuperior              *string
	CursoMedioID             *uuid.UUID
	CursoSuperiorID          *uuid.UUID
	CreatedAt                time.Time
	EmailVerificado          bool

	// AvaliacoesPorAno previne double-submit de avaliações finais.
	// Chave: "<tipoEnsino>_<anoLectivo>_<anoAcademicoAtual>"
	AvaliacoesPorAno map[string]bool

	// Mapa de notas registradas por chave composta
	NotasRegistradasPorChave map[string]bool

	// Mapa de faltas registradas por chave composta
	FaltasRegistradasPorChave map[string]bool
}

func NewEstudante() *Estudante {
	return &Estudante{
		BaseAggregate: BaseAggregate{
			ID:                uuid.New(),
			Version:           0,
			UncommittedEvents: []DomainEvent{},
		},
		Status:                    "inativo",
		AvaliacoesPorAno:          make(map[string]bool),
		NotasRegistradasPorChave:  make(map[string]bool),
		FaltasRegistradasPorChave: make(map[string]bool),
	}
}

func (e *Estudante) GetType() string { return "Estudante" }

// ============================================================================
// Apply dispatcher
// ============================================================================

func (e *Estudante) Apply(event DomainEvent) error {
	switch event.GetEventType() {
	case "EstudanteCriadoComVinculo":
		return e.applyEstudanteCriadoComVinculo(event)
	case "FaltasRegistradas":
		return e.applyFaltasRegistradas(event)
	case "FaltaAtualizada":
		return e.applyFaltaAtualizada(event)
	case "FaltaDeletada":
		return e.applyFaltaDeletada(event)
	case "NotasRegistradas":
		return e.applyNotasRegistradas(event)
	case "NotaAtualizada":
		return e.applyNotaAtualizada(event)
	case "NotaDeletada":
		return e.applyNotaDeletada(event)
	case "StatusEscolarFundamentalAtualizado":
		return e.applyStatusEscolarFundamentalAtualizado(event)
	case "StatusEscolarMedioAtualizado":
		return e.applyStatusEscolarMedioAtualizado(event)
	case "StatusSuperiorAtualizado":
		return e.applyStatusSuperiorAtualizado(event)
	case "CursoAlterado":
		return e.applyCursoAlterado(event)
	case "AvaliacaoFinalEscolar":
		return e.applyAvaliacaoFinalEscolar(event)
	case "AvaliacaoFinalSuperior":
		return e.applyAvaliacaoFinalSuperior(event)
	case "DadosPessoaisAtualizados":
		return e.applyDadosPessoaisAtualizados(event)
	case "DadosAcademicosAtualizados":
		return e.applyDadosAcademicosAtualizados(event)
	case "SenhaAlterada":
		return e.applySenhaAlterada(event)
	case "EmailVerificadoEstudante":
		return e.applyEmailVerificado(event)
	default:
		return fmt.Errorf("tipo de evento desconhecido: %s", event.GetEventType())
	}
}

// ============================================================================
// Tipos de evento
// ============================================================================

// EstudanteCriadoComVinculoEvent — genero e data_nascimento são obrigatórios
// e gravados como valores não-nulos no ledger.
type EstudanteCriadoComVinculoEvent struct {
	BaseEvent
	Nome                     string
	CodigoEstudante          string
	SenhaHash                string
	Email                    *string
	Telefone                 *string
	BilheteIdentidade        *string
	BilheteIdentidadeResp    *string
	Genero                   string    // obrigatório
	DataNascimento           time.Time // obrigatório
	AnoEscolar               *string
	AnoEscolarMedio          *string
	AnoSuperior              *string
	CursoMedioID             *uuid.UUID
	CursoSuperiorID          *uuid.UUID
	StatusEscolarFundamental string
	StatusEscolarMedio       string
	StatusSuperior           string
	CodigoAcademia           string
	AcademiaID               *uuid.UUID
	CreatedAt                time.Time
}

func (e *EstudanteCriadoComVinculoEvent) GetPayload() interface{} { return e }
func (e *EstudanteCriadoComVinculoEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type StatusSuperiorAtualizadoEvent struct {
	BaseEvent
	NovoStatus    string
	AtualizadoPor uuid.UUID
	UpdatedAt     time.Time
}

func (e *StatusSuperiorAtualizadoEvent) GetPayload() interface{} { return e }
func (e *StatusSuperiorAtualizadoEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

// DadosPessoaisAtualizadosEvent — DataNascimento é ponteiro porque nil = "não alterar".
// Genero não pode ser atualizado após o cadastro.
type DadosPessoaisAtualizadosEvent struct {
	BaseEvent
	Nome                  *string
	Email                 *string
	Telefone              *string
	BilheteIdentidade     *string
	BilheteIdentidadeResp *string
	DataNascimento        *time.Time // ponteiro: nil = não alterar
	EmailAlterado         bool
	UpdatedAt             time.Time
}

func (e *DadosPessoaisAtualizadosEvent) GetPayload() interface{} { return e }
func (e *DadosPessoaisAtualizadosEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type DadosAcademicosAtualizadosEvent struct {
	BaseEvent
	AnoEscolar      *string
	AnoEscolarMedio *string
	AnoSuperior     *string
	CursoMedioID    *uuid.UUID
	CursoSuperiorID *uuid.UUID
	UpdatedAt       time.Time
}

func (e *DadosAcademicosAtualizadosEvent) GetPayload() interface{} { return e }
func (e *DadosAcademicosAtualizadosEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type CursoAlteradoEvent struct {
	BaseEvent
	CursoID    uuid.UUID
	TipoEnsino string
	UpdatedAt  time.Time
}

func (e *CursoAlteradoEvent) GetPayload() interface{} { return e }
func (e *CursoAlteradoEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type SenhaAlteradaEvent struct {
	BaseEvent
	NovaSenhaHash string
	AlteradaAt    time.Time
}

func (e *SenhaAlteradaEvent) GetPayload() interface{} { return e }
func (e *SenhaAlteradaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type EmailVerificadoEstudanteEvent struct {
	BaseEvent
	VerifiedAt time.Time
}

func (e *EmailVerificadoEstudanteEvent) GetPayload() interface{} { return e }
func (e *EmailVerificadoEstudanteEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

// ============================================================================
// validarDataNascimento — regra central, compartilhada por CriarComVinculo
// e AtualizarDadosPessoais. A data deve ser estritamente no passado (antes
// de hoje); comparação feita apenas por data, sem componente de hora.
// ============================================================================

func validarDataNascimento(data time.Time) error {
	hoje := time.Now().UTC().Truncate(24 * time.Hour)
	dataNasc := data.UTC().Truncate(24 * time.Hour)
	if !dataNasc.Before(hoje) {
		return fmt.Errorf("data_nascimento deve ser anterior à data atual")
	}
	return nil
}

// ============================================================================
// Comandos
// ============================================================================

// CriarComVinculo cria o estudante com vínculo direto a uma academia.
// genero e dataNascimento são obrigatórios e validados no aggregate.
func (e *Estudante) CriarComVinculo(
	nome string,
	codigoEstudante string,
	senhaHash string,
	email *string,
	telefone *string,
	bilhete *string,
	bilheteResp *string,
	genero string,
	dataNascimento time.Time,
	anoEscolar *string,
	anoEscolarMedio *string,
	anoSuperior *string,
	cursoMedioID *uuid.UUID,
	cursoSuperiorID *uuid.UUID,
	statusEscolarFundamental *string,
	statusEscolarMedio *string,
	statusSuperior *string,
	academiaID *uuid.UUID,
	codigoAcademia string,
) error {
	if nome == "" || codigoEstudante == "" || senhaHash == "" || codigoAcademia == "" {
		return fmt.Errorf("campos obrigatórios vazios")
	}
	if genero != "masculino" && genero != "feminino" {
		return fmt.Errorf("genero deve ser 'masculino' ou 'feminino'")
	}
	if err := validarDataNascimento(dataNascimento); err != nil {
		return err
	}

	if anoEscolar != nil && *anoEscolar != "" {
		if err := utils.ValidateAnoFundamental(*anoEscolar); err != nil {
			return fmt.Errorf("ano_escolar_fundamental inválido: %w", err)
		}
	}
	if anoEscolarMedio != nil && *anoEscolarMedio != "" {
		if err := utils.ValidateAnoMedio(*anoEscolarMedio); err != nil {
			return fmt.Errorf("ano_escolar_medio inválido: %w", err)
		}
	}
	if anoSuperior != nil && *anoSuperior != "" {
		if err := utils.ValidateAnoSuperior(*anoSuperior); err != nil {
			return fmt.Errorf("ano_superior inválido: %w", err)
		}
	}

	statusFund := "em_andamento"
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

	event := &EstudanteCriadoComVinculoEvent{
		BaseEvent:                BaseEvent{EventType: "EstudanteCriadoComVinculo", AggregateID: e.ID},
		Nome:                     nome,
		CodigoEstudante:          codigoEstudante,
		SenhaHash:                senhaHash,
		Email:                    email,
		Telefone:                 telefone,
		BilheteIdentidade:        bilhete,
		BilheteIdentidadeResp:    bilheteResp,
		Genero:                   genero,
		DataNascimento:           dataNascimento,
		AnoEscolar:               anoEscolar,
		AnoEscolarMedio:          anoEscolarMedio,
		AnoSuperior:              anoSuperior,
		CursoMedioID:             cursoMedioID,
		CursoSuperiorID:          cursoSuperiorID,
		StatusEscolarFundamental: statusFund,
		StatusEscolarMedio:       statusMed,
		StatusSuperior:           statusSup,
		CodigoAcademia:           codigoAcademia,
		AcademiaID:               academiaID,
		CreatedAt:                time.Now(),
	}
	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) VerificarEmail() error {
	if e.EmailVerificado {
		return fmt.Errorf("email já verificado")
	}
	event := &EmailVerificadoEstudanteEvent{
		BaseEvent:  BaseEvent{EventType: "EmailVerificadoEstudante", AggregateID: e.ID},
		VerifiedAt: time.Now(),
	}
	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) AlterarSenha(novaSenhaHash string) error {
	if len(novaSenhaHash) < 60 {
		return fmt.Errorf("senhaHash inválido: esperado hash bcrypt (mínimo 60 caracteres)")
	}
	event := &SenhaAlteradaEvent{
		BaseEvent:     BaseEvent{EventType: "SenhaAlterada", AggregateID: e.ID},
		NovaSenhaHash: novaSenhaHash,
		AlteradaAt:    time.Now(),
	}
	e.RaiseEvent(event)
	return e.Apply(event)
}

// AtualizarDadosPessoais — dataNascimento é ponteiro (nil = não alterar).
// Genero não pode ser alterado após o cadastro.
func (e *Estudante) AtualizarDadosPessoais(
	nome *string,
	email *string,
	telefone *string,
	bilheteIdentidade *string,
	bilheteIdentidadeResp *string,
	dataNascimento *time.Time,
) error {
	if nome == nil && email == nil && telefone == nil &&
		bilheteIdentidade == nil && bilheteIdentidadeResp == nil && dataNascimento == nil {
		return fmt.Errorf("nenhum campo para atualizar")
	}
	if dataNascimento != nil {
		if err := validarDataNascimento(*dataNascimento); err != nil {
			return err
		}
	}

	emailAlterado := email != nil && (e.Email == nil || *e.Email != *email)
	event := &DadosPessoaisAtualizadosEvent{
		BaseEvent:             BaseEvent{EventType: "DadosPessoaisAtualizados", AggregateID: e.ID},
		Nome:                  nome,
		Email:                 email,
		Telefone:              telefone,
		BilheteIdentidade:     bilheteIdentidade,
		BilheteIdentidadeResp: bilheteIdentidadeResp,
		DataNascimento:        dataNascimento,
		EmailAlterado:         emailAlterado,
		UpdatedAt:             time.Now(),
	}
	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) AtualizarDadosAcademicos(
	anoEscolar *string,
	anoEscolarMedio *string,
	anoSuperior *string,
	cursoMedioID *uuid.UUID,
	cursoSuperiorID *uuid.UUID,
) error {
	if anoEscolar == nil && anoEscolarMedio == nil && anoSuperior == nil &&
		cursoMedioID == nil && cursoSuperiorID == nil {
		return fmt.Errorf("nenhum campo para atualizar")
	}

	if anoEscolar != nil && *anoEscolar != "" {
		if err := utils.ValidateAnoFundamental(*anoEscolar); err != nil {
			return fmt.Errorf("ano_escolar_fundamental inválido: %w", err)
		}
	}
	if anoEscolarMedio != nil && *anoEscolarMedio != "" {
		if err := utils.ValidateAnoMedio(*anoEscolarMedio); err != nil {
			return fmt.Errorf("ano_escolar_medio inválido: %w", err)
		}
	}
	if anoSuperior != nil && *anoSuperior != "" {
		if err := utils.ValidateAnoSuperior(*anoSuperior); err != nil {
			return fmt.Errorf("ano_superior inválido: %w", err)
		}
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

func (e *Estudante) AtualizarStatusEscolarFundamental(novoStatus string, atualizadoPor uuid.UUID) error {
	validStatus := map[string]bool{"inativo": true, "em_andamento": true, "finalizado": true}
	if !validStatus[novoStatus] {
		return fmt.Errorf("status inválido: '%s'. Use: inativo, em_andamento, finalizado", novoStatus)
	}
	event := &StatusEscolarFundamentalAtualizadoEvent{
		BaseEvent:     BaseEvent{EventType: "StatusEscolarFundamentalAtualizado", AggregateID: e.ID},
		NovoStatus:    novoStatus,
		AtualizadoPor: atualizadoPor,
		UpdatedAt:     time.Now(),
	}
	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) AtualizarStatusEscolarMedio(novoStatus string, atualizadoPor uuid.UUID) error {
	validStatus := map[string]bool{"inativo": true, "em_andamento": true, "finalizado": true}
	if !validStatus[novoStatus] {
		return fmt.Errorf("status inválido: '%s'. Use: inativo, em_andamento, finalizado", novoStatus)
	}
	event := &StatusEscolarMedioAtualizadoEvent{
		BaseEvent:     BaseEvent{EventType: "StatusEscolarMedioAtualizado", AggregateID: e.ID},
		NovoStatus:    novoStatus,
		AtualizadoPor: atualizadoPor,
		UpdatedAt:     time.Now(),
	}
	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) AtualizarStatusSuperior(novoStatus string, atualizadoPor uuid.UUID) error {
	validStatus := map[string]bool{"inativo": true, "em_andamento": true, "finalizado": true}
	if !validStatus[novoStatus] {
		return fmt.Errorf("status inválido: '%s'. Use: inativo, em_andamento, finalizado", novoStatus)
	}
	if novoStatus == "em_andamento" || novoStatus == "finalizado" {
		if e.StatusEscolarFundamental != "inativo" && e.StatusEscolarFundamental != "finalizado" {
			return fmt.Errorf(
				"status_superior só pode avançar se status_escolar_fundamental estiver 'finalizado' ou 'inativo' (atual: '%s')",
				e.StatusEscolarFundamental,
			)
		}
		if e.StatusEscolarMedio != "inativo" && e.StatusEscolarMedio != "finalizado" {
			return fmt.Errorf(
				"status_superior só pode avançar se status_escolar_medio estiver 'finalizado' ou 'inativo' (atual: '%s')",
				e.StatusEscolarMedio,
			)
		}
	}
	event := &StatusSuperiorAtualizadoEvent{
		BaseEvent:     BaseEvent{EventType: "StatusSuperiorAtualizado", AggregateID: e.ID},
		NovoStatus:    novoStatus,
		AtualizadoPor: atualizadoPor,
		UpdatedAt:     time.Now(),
	}
	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) AlterarCurso(cursoID uuid.UUID, tipoEnsino string) error {
	if tipoEnsino != "medio" && tipoEnsino != "superior" {
		return fmt.Errorf("tipo_ensino deve ser 'medio' ou 'superior'")
	}
	if cursoID == uuid.Nil {
		return fmt.Errorf("curso_id inválido")
	}
	if e.CodigoAcademia == nil {
		return fmt.Errorf("estudante não está vinculado a nenhuma academia")
	}
	if tipoEnsino == "medio" {
		if e.StatusEscolarMedio != "em_andamento" {
			return fmt.Errorf("só pode alterar curso médio se status_escolar_medio for 'em_andamento'")
		}
	} else {
		if e.StatusSuperior != "em_andamento" {
			return fmt.Errorf("só pode alterar curso superior se status_superior for 'em_andamento'")
		}
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

func (e *Estudante) applyEstudanteCriadoComVinculo(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applyEstudanteCriadoComVinculo: marshal error: %w", err)
	}
	var ev EstudanteCriadoComVinculoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyEstudanteCriadoComVinculo: unmarshal error: %w", err)
	}
	e.Nome = ev.Nome
	e.CodigoEstudante = ev.CodigoEstudante
	e.SenhaHash = ev.SenhaHash
	e.Email = ev.Email
	e.Telefone = ev.Telefone
	e.BilheteIdentidade = ev.BilheteIdentidade
	e.BilheteIdentidadeResp = ev.BilheteIdentidadeResp
	e.Genero = ev.Genero
	e.DataNascimento = ev.DataNascimento
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
	if e.AvaliacoesPorAno == nil {
		e.AvaliacoesPorAno = make(map[string]bool)
	}
	if e.NotasRegistradasPorChave == nil {
		e.NotasRegistradasPorChave = make(map[string]bool)
	}
	if e.FaltasRegistradasPorChave == nil {
		e.FaltasRegistradasPorChave = make(map[string]bool)
	}
	return nil
}

func (e *Estudante) applyStatusSuperiorAtualizado(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applyStatusSuperiorAtualizado: marshal error: %w", err)
	}
	var ev StatusSuperiorAtualizadoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyStatusSuperiorAtualizado: unmarshal error: %w", err)
	}
	e.StatusSuperior = ev.NovoStatus
	return nil
}

func (e *Estudante) applyCursoAlterado(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applyCursoAlterado: marshal error: %w", err)
	}
	var ev CursoAlteradoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyCursoAlterado: unmarshal error: %w", err)
	}
	if ev.TipoEnsino == "medio" {
		e.CursoMedioID = &ev.CursoID
	} else {
		e.CursoSuperiorID = &ev.CursoID
	}
	return nil
}

func (e *Estudante) applyDadosPessoaisAtualizados(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applyDadosPessoaisAtualizados: marshal error: %w", err)
	}
	var ev DadosPessoaisAtualizadosEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyDadosPessoaisAtualizados: unmarshal error: %w", err)
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
	if ev.DataNascimento != nil {
		e.DataNascimento = *ev.DataNascimento
	}
	return nil
}

func (e *Estudante) applyDadosAcademicosAtualizados(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applyDadosAcademicosAtualizados: marshal error: %w", err)
	}
	var ev DadosAcademicosAtualizadosEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyDadosAcademicosAtualizados: unmarshal error: %w", err)
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

func (e *Estudante) applySenhaAlterada(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applySenhaAlterada: marshal error: %w", err)
	}
	var ev SenhaAlteradaEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applySenhaAlterada: unmarshal error: %w", err)
	}
	if ev.NovaSenhaHash == "" {
		return fmt.Errorf("applySenhaAlterada: NovaSenhaHash vazio no payload")
	}
	e.SenhaHash = ev.NovaSenhaHash
	return nil
}

func (e *Estudante) applyEmailVerificado(_ DomainEvent) error {
	e.EmailVerificado = true
	return nil
}

func (e *Estudante) applyStatusEscolarFundamentalAtualizado(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applyStatusEscolarFundamentalAtualizado: marshal error: %w", err)
	}
	var ev StatusEscolarFundamentalAtualizadoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyStatusEscolarFundamentalAtualizado: unmarshal error: %w", err)
	}
	e.StatusEscolarFundamental = ev.NovoStatus
	return nil
}

func (e *Estudante) applyStatusEscolarMedioAtualizado(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applyStatusEscolarMedioAtualizado: marshal error: %w", err)
	}
	var ev StatusEscolarMedioAtualizadoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyStatusEscolarMedioAtualizado: unmarshal error: %w", err)
	}
	e.StatusEscolarMedio = ev.NovoStatus
	return nil
}
