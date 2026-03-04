// ============================================================================
// ARQUIVO: internal/domain/aggregates/estudante.go
//
// CORREÇÕES APLICADAS:
//   FIX-C3  — VerificarEmail() adicionado ao aggregate Estudante (event sourcing)
//   FIX-C4  — Rotas de status escolar REMOVIDAS das rotas de estudante (feito no main.go)
//              O aggregate mantém os comandos, mas os handlers agora exigem academia.
//   FIX-C5  — apply handlers propagam erros de json.Unmarshal
//   FIX-C6  — EmailVerificado adicionado ao switch Apply()
//
// REGRA DE ORGANIZAÇÃO DOS ARQUIVOS:
//   estudante.go           → struct, Apply switch, eventos base, comandos core
//   estudante_falta.go     → RegistrarFalta
//   estudante_notas.go     → eventos de nota, RegistrarNota, AtualizarNota,
//                            applyNotasRegistradas, applyNotaAtualizada
//   estudante_avaliacao.go → AvaliacaoFinalAnoAcademicoEvent, RegistrarAvaliacaoFinal,
//                            applyAvaliacaoFinalAnoAcademico
//   estudante_aprovacao.go → AprovacaoAnoRegistradaEvent,
//                            StatusEscolarFundamentalAtualizadoEvent,
//                            StatusEscolarMedioAtualizadoEvent
// ============================================================================

package aggregates

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// Aggregate
// ============================================================================

type Estudante struct {
	BaseAggregate

	Nome                     string
	CodigoEstudante          string
	SenhaHash                string
	Email                    *string
	Telefone                 *string
	BilheteIdentidade        *string
	BilheteIdentidadeResp    *string
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
	Genero                   string
	EmailVerificado          bool
}

func NewEstudante() *Estudante {
	return &Estudante{
		BaseAggregate: BaseAggregate{
			ID:                uuid.New(),
			Version:           0,
			UncommittedEvents: []DomainEvent{},
		},
		Status: "inativo",
	}
}

func (e *Estudante) GetType() string { return "Estudante" }

// ============================================================================
// Apply dispatcher
// ============================================================================

func (e *Estudante) Apply(event DomainEvent) error {
	switch event.GetEventType() {
	case "EstudanteCriado":
		return e.applyEstudanteCriado(event)
	case "EstudanteCriadoComVinculo":
		return e.applyEstudanteCriadoComVinculo(event)
	case "FaltasRegistradas":
		return e.applyFaltasRegistradas(event)
	case "NotasRegistradas":
		return e.applyNotasRegistradas(event)
	case "NotaAtualizada":
		return e.applyNotaAtualizada(event)
	case "StatusEscolarFundamentalAtualizado":
		return e.applyStatusEscolarFundamentalAtualizado(event)
	case "StatusEscolarMedioAtualizado":
		return e.applyStatusEscolarMedioAtualizado(event)
	case "StatusSuperiorAtualizado":
		return e.applyStatusSuperiorAtualizado(event)
	case "CursoAlterado":
		return e.applyCursoAlterado(event)
	case "AprovacaoAnoRegistrada":
		return e.applyAprovacaoAnoRegistrada(event)
	case "AvaliacaoFinalAnoAcademico":
		return e.applyAvaliacaoFinalAnoAcademico(event)
	case "DadosPessoaisAtualizados":
		return e.applyDadosPessoaisAtualizados(event)
	case "DadosAcademicosAtualizados":
		return e.applyDadosAcademicosAtualizados(event)
	case "SenhaAlterada":
		return e.applySenhaAlterada(event)
	// FIX-C6: EmailVerificado agora tratado no Apply do estudante
	case "EmailVerificadoEstudante":
		return e.applyEmailVerificado(event)
	default:
		return fmt.Errorf("tipo de evento desconhecido: %s", event.GetEventType())
	}
}

// ============================================================================
// Eventos base (definidos neste arquivo)
// NOTA: AvaliacaoFinalAnoAcademicoEvent está em estudante_avaliacao.go
//       NotasRegistradasEvent / NotaAtualizadaEvent estão em estudante_notas.go
//       AprovacaoAnoRegistradaEvent / StatusEscolar*Event estão em estudante_aprovacao.go
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
	AnoEscolarMedio          *string
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
func (e *EstudanteCriadoEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

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
	AnoEscolarMedio          *string
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
func (e *EstudanteCriadoComVinculoEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

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
func (e *FaltasRegistradasEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type StatusSuperiorAtualizadoEvent struct {
	BaseEvent
	NovoStatus string
	UpdatedAt  time.Time
}

func (e *StatusSuperiorAtualizadoEvent) GetPayload() interface{} { return e }
func (e *StatusSuperiorAtualizadoEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

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

// FIX-C3: EmailVerificadoEstudanteEvent — evento exclusivo do aggregate Estudante.
// Usa nome distinto de "EmailVerificado" (que é do Admin/Academia) para evitar
// ambiguidade na whitelist ValidateEventType e no Apply dispatcher.
type EmailVerificadoEstudanteEvent struct {
	BaseEvent
	VerifiedAt time.Time
}

func (e *EmailVerificadoEstudanteEvent) GetPayload() interface{} { return e }
func (e *EmailVerificadoEstudanteEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

// ============================================================================
// Comandos
// ============================================================================

// Criar cria um estudante SEM vínculo com academia (cadastro direto pelo estudante).
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
		return fmt.Errorf("pelo menos um bilhete de identidade é obrigatório")
	}

	if bilhete != nil && bilheteResp != nil && *bilhete == *bilheteResp {
		return fmt.Errorf("bilhete de identidade do estudante e do responsável não podem ser iguais")
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

// CriarComVinculo cria um estudante JÁ VINCULADO a uma academia.
// Usado exclusivamente pelo endpoint POST /academia/estudante/register.
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
	if nome == "" || codigoEstudante == "" || senhaHash == "" || codigoAcademia == "" {
		return fmt.Errorf("campos obrigatórios vazios")
	}

	if genero != "masculino" && genero != "feminino" {
		return fmt.Errorf("genero deve ser 'masculino' ou 'feminino'")
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

// VerificarEmail emite evento de verificação de email para o estudante.
// FIX-C3: event sourcing completo — antes era UPDATE direto na projeção.
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
	if novaSenhaHash == "" {
		return fmt.Errorf("senha não pode ser vazia")
	}

	event := &SenhaAlteradaEvent{
		BaseEvent:     BaseEvent{EventType: "SenhaAlterada", AggregateID: e.ID},
		NovaSenhaHash: novaSenhaHash,
		AlteradaAt:    time.Now(),
	}

	e.RaiseEvent(event)
	return e.Apply(event)
}

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

func (e *Estudante) AtualizarStatusSuperior(novoStatus string) error {
	validStatus := map[string]bool{"inativo": true, "em_andamento": true, "finalizado": true}
	if !validStatus[novoStatus] {
		return fmt.Errorf("status inválido: %s", novoStatus)
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

func (e *Estudante) AtualizarDadosPessoais(
	nome *string,
	email *string,
	telefone *string,
	bilheteIdentidade *string,
	bilheteIdentidadeResp *string,
) error {
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

func (e *Estudante) AtualizarDadosAcademicos(
	anoEscolar *string,
	anoEscolarMedio *string,
	anoSuperior *string,
	cursoMedioID *uuid.UUID,
	cursoSuperiorID *uuid.UUID,
) error {
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
		BaseEvent:       BaseEvent{EventType: "AprovacaoAnoRegistrada", AggregateID: e.ID},
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

// ============================================================================
// Apply handlers
// NOTA: applyNotasRegistradas e applyNotaAtualizada estão em estudante_notas.go
//       applyAvaliacaoFinalAnoAcademico está em estudante_avaliacao.go
// ============================================================================

func (e *Estudante) applyFaltasRegistradas(_ DomainEvent) error { return nil }

func (e *Estudante) applyEstudanteCriado(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applyEstudanteCriado: marshal error: %w", err)
	}
	var ev EstudanteCriadoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyEstudanteCriado: unmarshal error: %w", err)
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

func (e *Estudante) applyAprovacaoAnoRegistrada(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applyAprovacaoAnoRegistrada: marshal error: %w", err)
	}
	var ev AprovacaoAnoRegistradaEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyAprovacaoAnoRegistrada: unmarshal error: %w", err)
	}

	if !ev.Aprovado {
		return nil
	}

	switch ev.TipoEnsino {
	case "fundamental":
		e.AnoEscolar = ev.ProximoNivel
	case "medio":
		e.AnoEscolarMedio = ev.ProximoNivel
	case "superior":
		e.AnoSuperior = ev.ProximoNivel
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
	e.SenhaHash = ev.NovaSenhaHash
	return nil
}

// applyEmailVerificado — FIX-C3: event sourcing para verificação de email do estudante.
func (e *Estudante) applyEmailVerificado(_ DomainEvent) error {
	e.EmailVerificado = true
	return nil
}

// ============================================================================
// Suppress unused import warning for log (used by BaseAggregate via RaiseEvent)
// ============================================================================
var _ = log.Printf