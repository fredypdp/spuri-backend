package aggregates

import (
	"encoding/json"
	"fmt"
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

	// FIX E-04: mapa de aprovações por tipo+ano para detectar duplicatas
	// em comandos subsequentes sem depender da projeção.
	// Chave: "<tipoEnsino>_<anoLectivo>_<nivelAtual>"
	AprovacoesPorAno map[string]bool
}

func NewEstudante() *Estudante {
	return &Estudante{
		BaseAggregate: BaseAggregate{
			ID:                uuid.New(),
			Version:           0,
			UncommittedEvents: []DomainEvent{},
		},
		Status:           "inativo",
		AprovacoesPorAno: make(map[string]bool),
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
//       AprovacaoAnoRegistradaEvent / StatusEscolar*AtualizadoEvent estão em estudante_aprovacao.go
// ============================================================================

// EstudanteCriadoEvent — FIX E-01: CriadoPor adicionado para rastreabilidade
// de quem iniciou o cadastro self-service (nil = o próprio estudante).
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
	// FIX E-01: nil = auto-cadastro; preenchido quando criado por admin/academia.
	// Etapa 4 deve passar este campo no handler de auto-cadastro (ou mantê-lo nil).
	CriadoPor *uuid.UUID
}

func (e *EstudanteCriadoEvent) GetPayload() interface{} { return e }
func (e *EstudanteCriadoEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

// EstudanteCriadoComVinculoEvent — FIX E-01: AcademiaID adicionado ao payload
// para rastreabilidade forense sem depender dos metadados do ledger.
// Etapa 4 deve preencher AcademiaID no handler de criação via academia.
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
	// FIX E-01: UUID da academia criadora. Etapa 4 preenche este campo.
	AcademiaID *uuid.UUID
	CreatedAt  time.Time
	Genero     string
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

// SenhaAlteradaEvent — evento de troca de senha do estudante.
type SenhaAlteradaEvent struct {
	BaseEvent
	NovaSenhaHash string
	AlteradaAt    time.Time
}

func (e *SenhaAlteradaEvent) GetPayload() interface{} { return e }
func (e *SenhaAlteradaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

// EmailVerificadoEstudanteEvent — evento exclusivo do aggregate Estudante.
// Nome distinto de "EmailVerificado" (Admin/Academia) para evitar ambiguidade.
type EmailVerificadoEstudanteEvent struct {
	BaseEvent
	VerifiedAt time.Time
}

func (e *EmailVerificadoEstudanteEvent) GetPayload() interface{} { return e }
func (e *EmailVerificadoEstudanteEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

// ============================================================================
// Comandos
// ============================================================================

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
		CriadoPor:                nil, // auto-cadastro: sem autor explícito
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
		AcademiaID:               nil, // Etapa 4 preenche com academia.ID
		CreatedAt:                time.Now(),
		Genero:                   genero,
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

// AlterarSenha emite SenhaAlterada.
// FIX E-05: valida comprimento mínimo do hash (bcrypt = mínimo 60 chars).
// Hash inválido gravado no ledger é irrecuperável — falha aqui é obrigatória.
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

// AtualizarStatusSuperior — FIX E-02: a pré-condição foi corrigida para
// permitir estudantes de percurso exclusivamente superior (fundamental e
// médio com status "inativo"). A restrição só bloqueia se algum dos ciclos
// inferiores estiver "em_andamento" (existente mas incompleto).
func (e *Estudante) AtualizarStatusSuperior(novoStatus string) error {
	validStatus := map[string]bool{"inativo": true, "em_andamento": true, "finalizado": true}
	if !validStatus[novoStatus] {
		return fmt.Errorf("status inválido: %s", novoStatus)
	}

	if novoStatus == "em_andamento" || novoStatus == "finalizado" {
		// Só bloqueia se o ciclo existe (não-inativo) mas ainda não foi concluído.
		// Estudantes com fundamental/médio = "inativo" são de percurso superior
		// puro e não são bloqueados.
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
		BaseEvent:  BaseEvent{EventType: "StatusSuperiorAtualizado", AggregateID: e.ID},
		NovoStatus: novoStatus,
		UpdatedAt:  time.Now(),
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

// RegistrarAprovacaoAno — FIX E-04: verifica duplicata usando AprovacoesPorAno
// antes de emitir o evento, para evitar emissão dupla para o mesmo período.
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

	// FIX E-04: detectar duplicata via estado do aggregate
	chave := tipoEnsino + "_" + anoLectivo + "_" + nivelAtual
	if e.AprovacoesPorAno != nil && e.AprovacoesPorAno[chave] {
		return fmt.Errorf(
			"aprovação do tipo '%s' para o ano '%s' / nível '%s' já foi registrada",
			tipoEnsino, anoLectivo, nivelAtual,
		)
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
//       applyStatusEscolar*Atualizado estão em estudante_aprovacao.go (via métodos abaixo)
// ============================================================================

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
	if e.AprovacoesPorAno == nil {
		e.AprovacoesPorAno = make(map[string]bool)
	}
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
	if e.AprovacoesPorAno == nil {
		e.AprovacoesPorAno = make(map[string]bool)
	}
	return nil
}

// applyFaltasRegistradas — FIX E-03: deserializa o payload para detectar
// corrupção silenciosa. O aggregate não mantém faltas em estado (gerenciado
// pela projeção), mas valida a estrutura do evento durante o rebuild.
func (e *Estudante) applyFaltasRegistradas(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applyFaltasRegistradas: marshal error: %w", err)
	}
	var ev FaltasRegistradasEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyFaltasRegistradas: unmarshal error (payload corrompido): %w", err)
	}
	// Aggregate não mantém faltas em estado — apenas valida o payload.
	return nil
}

// applyAprovacaoAnoRegistrada — FIX E-04: registra a aprovação em
// AprovacoesPorAno para permitir detecção de duplicatas em comandos
// subsequentes. Também atualiza o ano escolar quando aprovado.
func (e *Estudante) applyAprovacaoAnoRegistrada(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applyAprovacaoAnoRegistrada: marshal error: %w", err)
	}
	var ev AprovacaoAnoRegistradaEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyAprovacaoAnoRegistrada: unmarshal error: %w", err)
	}

	// FIX E-04: registrar no mapa de aprovações para dedup
	if e.AprovacoesPorAno == nil {
		e.AprovacoesPorAno = make(map[string]bool)
	}
	chave := ev.TipoEnsino + "_" + ev.AnoLectivo + "_" + ev.NivelAtual
	e.AprovacoesPorAno[chave] = true

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

// applySenhaAlterada — FIX E-05: valida que o hash não é vazio no payload
// (defesa em profundidade além da validação no comando AlterarSenha).
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

// applyStatusEscolarFundamentalAtualizado e applyStatusEscolarMedioAtualizado
// são definidos aqui porque o dispatcher Apply() está neste arquivo.
// Os eventos (StatusEscolarFundamentalAtualizadoEvent / StatusEscolarMedioAtualizadoEvent)
// estão definidos em estudante_aprovacao.go.

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