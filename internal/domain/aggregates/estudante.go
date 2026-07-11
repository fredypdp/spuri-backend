package aggregates

import (
	"encoding/json"
	"fmt"
	"spuri/internal/utils"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// Aggregate
// ============================================================================

// Estudante — genero e data_nascimento são sempre preenchidos (obrigatórios no cadastro).
type Estudante struct {
	BaseAggregate

	Nome                          string
	CodigoEstudante               string
	SenhaHash                     string
	Email                         *string
	Telefone                      *string
	TelefoneVerificado            bool
	TelefoneResponsavel           *string
	TelefoneResponsavelVerificado bool
	BilheteIdentidade             *string
	BilheteIdentidadeResp         *string
	Genero                        string    // obrigatório
	DataNascimento                time.Time // obrigatório; valor zero indica aggregate não carregado
	CodigoAcademia                *string
	Status                        string
	StatusEscolarFundamental      string
	StatusEscolarMedio            string
	StatusSuperior                string
	SemestreAtual                 *int
	AnoEscolar                    *string
	AnoEscolarMedio               *string
	AnoSuperior                   *string
	CursoMedioID                  *uuid.UUID
	CursoSuperiorID               *uuid.UUID
	CreatedAt                     time.Time
	EmailVerificado               bool
	Documentos                    map[string]DocumentoMatricula

	// AvaliacoesPorAno previne double-submit de avaliações finais.
	// Chaves: "ano_letivo:<anoLectivo>" e "nivel:<escolar|superior>:<anoLectivo>:<anoAcademicoAtual>"
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
	case "NotasRegistradas":
		return e.applyNotasRegistradas(event)
	case "MatriculaFundamentalEfetivada", "FundamentalRetomado":
		return e.applyFundamentalEmAndamento(event)
	case "FundamentalInterrompido":
		return e.applyFundamentalInativo(event)
	case "EquivalenciaFundamentalReconhecida":
		return e.applyFundamentalFinalizado(event)
	case "MatriculaMedioEfetivada", "MedioRetomado":
		return e.applyMedioEmAndamento(event)
	case "MedioInterrompido":
		return e.applyMedioInativo(event)
	case "EquivalenciaMedioReconhecida":
		return e.applyMedioFinalizado(event)
	case "MatriculaSuperiorEfetivada", "MatriculaSuperiorReativada", "IngressoSuperiorPorEquivalenciaRegistrado":
		return e.applySuperiorEmAndamento(event)
	case "SuperiorTrancado", "SuperiorAbandonado":
		return e.applySuperiorInativo(event)
	case "EstudanteDesvinculadoDaAcademia":
		return e.applyEstudanteDesvinculadoDaAcademia(event)
	case "EstudanteReintegrado":
		return e.applyEstudanteReintegrado(event)
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
	Nome                          string
	CodigoEstudante               string
	SenhaHash                     string
	Email                         *string
	Telefone                      *string
	TelefoneVerificado            bool
	TelefoneResponsavel           *string
	TelefoneResponsavelVerificado bool
	BilheteIdentidade             *string
	BilheteIdentidadeResp         *string
	Genero                        string    // obrigatório
	DataNascimento                time.Time // obrigatório
	AnoEscolar                    *string
	AnoEscolarMedio               *string
	AnoSuperior                   *string
	SemestreAtual                 *int
	CursoMedioID                  *uuid.UUID
	CursoSuperiorID               *uuid.UUID
	StatusEscolarFundamental      string
	StatusEscolarMedio            string
	StatusSuperior                string
	CodigoAcademia                string
	AcademiaID                    *uuid.UUID
	CreatedAt                     time.Time
	Documentos                    map[string]DocumentoMatricula
}

func (e *EstudanteCriadoComVinculoEvent) GetPayload() interface{} { return e }
func (e *EstudanteCriadoComVinculoEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type MatriculaFundamentalEfetivadaEvent struct {
	BaseEvent
	AnoEscolar   string
	EfetivadaPor uuid.UUID
	EfetivadaAt  time.Time
}

func (e *MatriculaFundamentalEfetivadaEvent) GetPayload() interface{} { return e }
func (e *MatriculaFundamentalEfetivadaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type FundamentalInterrompidoEvent struct {
	BaseEvent
	Motivo          string
	InterrompidoPor uuid.UUID
	InterrompidoAt  time.Time
}

func (e *FundamentalInterrompidoEvent) GetPayload() interface{} { return e }
func (e *FundamentalInterrompidoEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type MatriculaMedioEfetivadaEvent struct {
	BaseEvent
	AnoEscolar   string
	CursoID      uuid.UUID
	EfetivadaPor uuid.UUID
	EfetivadaAt  time.Time
}

func (e *MatriculaMedioEfetivadaEvent) GetPayload() interface{} { return e }
func (e *MatriculaMedioEfetivadaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type MedioInterrompidoEvent struct {
	BaseEvent
	Motivo          string
	InterrompidoPor uuid.UUID
	InterrompidoAt  time.Time
}

func (e *MedioInterrompidoEvent) GetPayload() interface{} { return e }
func (e *MedioInterrompidoEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type MatriculaSuperiorEfetivadaEvent struct {
	BaseEvent
	CursoID       uuid.UUID
	AnoSuperior   string
	SemestreAtual int
	EfetivadaPor  uuid.UUID
	EfetivadaAt   time.Time
}

func (e *MatriculaSuperiorEfetivadaEvent) GetPayload() interface{} { return e }
func (e *MatriculaSuperiorEfetivadaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type SuperiorTrancadoEvent struct {
	BaseEvent
	Motivo      string
	TrancadoPor uuid.UUID
	TrancadoAt  time.Time
}

func (e *SuperiorTrancadoEvent) GetPayload() interface{} { return e }
func (e *SuperiorTrancadoEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type EstudanteDesvinculadoDaAcademiaEvent struct {
	BaseEvent
	CodigoAcademia  string
	CodigoEstudante string
	Motivo          string
	Nivel           string
	DesvinculadoPor uuid.UUID
	DesvinculadoAt  time.Time
}

func (e *EstudanteDesvinculadoDaAcademiaEvent) GetPayload() interface{} { return e }
func (e *EstudanteDesvinculadoDaAcademiaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type EstudanteReintegradoEvent struct {
	BaseEvent
	CodigoAcademia  string
	CodigoEstudante string
	TipoEnsino      string
	AnoEscolar      *string
	AnoEscolarMedio *string
	AnoSuperior     *string
	SemestreAtual   *int
	CursoMedioID    *uuid.UUID
	CursoSuperiorID *uuid.UUID
	ReintegradoPor  uuid.UUID
	ReintegradoAt   time.Time
}

func (e *EstudanteReintegradoEvent) GetPayload() interface{} { return e }
func (e *EstudanteReintegradoEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

// DadosPessoaisAtualizadosEvent — DataNascimento é ponteiro porque nil = "não alterar".
// Genero não pode ser atualizado após o cadastro.
type DadosPessoaisAtualizadosEvent struct {
	BaseEvent
	Nome                  *string
	Email                 *string
	Telefone              *string
	TelefoneResponsavel   *string
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

func ValidarBilhetesMatricula(bilhete, bilheteResp *string) error {
	if bilhete == nil || bilheteResp == nil {
		return nil
	}
	bi := strings.TrimSpace(*bilhete)
	biResp := strings.TrimSpace(*bilheteResp)
	if bi == "" || biResp == "" {
		return nil
	}
	if strings.EqualFold(bi, biResp) {
		return fmt.Errorf("bilhete_identidade e bilhete_identidade_responsavel não podem ser iguais")
	}
	return nil
}

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
	telefoneResponsavel *string,
	bilhete *string,
	bilheteResp *string,
	genero string,
	dataNascimento time.Time,
	anoEscolar *string,
	anoEscolarMedio *string,
	anoSuperior *string,
	cursoMedioID *uuid.UUID,
	cursoSuperiorID *uuid.UUID,
	academiaID *uuid.UUID,
	codigoAcademia string,
	documentosOpt ...map[string]DocumentoMatricula,
) error {
	return e.criarComVinculo(nome, codigoEstudante, senhaHash, email, telefone, telefoneResponsavel, bilhete, bilheteResp, genero, dataNascimento, anoEscolar, anoEscolarMedio, anoSuperior, cursoMedioID, cursoSuperiorID, academiaID, codigoAcademia, true, documentosOpt...)
}

func (e *Estudante) CriarComVinculoComDocumentosOpcionais(
	nome string,
	codigoEstudante string,
	senhaHash string,
	email *string,
	telefone *string,
	telefoneResponsavel *string,
	bilhete *string,
	bilheteResp *string,
	genero string,
	dataNascimento time.Time,
	anoEscolar *string,
	anoEscolarMedio *string,
	anoSuperior *string,
	cursoMedioID *uuid.UUID,
	cursoSuperiorID *uuid.UUID,
	academiaID *uuid.UUID,
	codigoAcademia string,
	documentosOpt ...map[string]DocumentoMatricula,
) error {
	return e.criarComVinculo(nome, codigoEstudante, senhaHash, email, telefone, telefoneResponsavel, bilhete, bilheteResp, genero, dataNascimento, anoEscolar, anoEscolarMedio, anoSuperior, cursoMedioID, cursoSuperiorID, academiaID, codigoAcademia, false, documentosOpt...)
}

func (e *Estudante) criarComVinculo(
	nome string,
	codigoEstudante string,
	senhaHash string,
	email *string,
	telefone *string,
	telefoneResponsavel *string,
	bilhete *string,
	bilheteResp *string,
	genero string,
	dataNascimento time.Time,
	anoEscolar *string,
	anoEscolarMedio *string,
	anoSuperior *string,
	cursoMedioID *uuid.UUID,
	cursoSuperiorID *uuid.UUID,
	academiaID *uuid.UUID,
	codigoAcademia string,
	exigirDocumentosEscolares bool,
	documentosOpt ...map[string]DocumentoMatricula,
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
	if err := ValidarBilhetesMatricula(bilhete, bilheteResp); err != nil {
		return err
	}
	if err := ValidarTelefonesMatricula(telefone, telefoneResponsavel, anoEscolar, anoEscolarMedio, anoSuperior); err != nil {
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

	documentos := map[string]DocumentoMatricula{}
	if len(documentosOpt) > 0 && documentosOpt[0] != nil {
		documentos = documentosOpt[0]
	}
	if exigirDocumentosEscolares {
		if err := ValidarDocumentosMatricula(bilhete, bilheteResp, anoEscolar, anoEscolarMedio, anoSuperior, documentos); err != nil {
			return err
		}
	}

	statusFund := "em_andamento"
	statusMed := "inativo"
	statusSup := "inativo"

	event := &EstudanteCriadoComVinculoEvent{
		BaseEvent:                BaseEvent{EventType: "EstudanteCriadoComVinculo", AggregateID: e.ID},
		Nome:                     nome,
		CodigoEstudante:          codigoEstudante,
		SenhaHash:                senhaHash,
		Email:                    email,
		Telefone:                 telefone,
		TelefoneResponsavel:      telefoneResponsavel,
		BilheteIdentidade:        bilhete,
		BilheteIdentidadeResp:    bilheteResp,
		Genero:                   genero,
		DataNascimento:           dataNascimento,
		AnoEscolar:               anoEscolar,
		AnoEscolarMedio:          anoEscolarMedio,
		AnoSuperior:              anoSuperior,
		SemestreAtual:            nil,
		CursoMedioID:             cursoMedioID,
		CursoSuperiorID:          cursoSuperiorID,
		StatusEscolarFundamental: statusFund,
		StatusEscolarMedio:       statusMed,
		StatusSuperior:           statusSup,
		CodigoAcademia:           codigoAcademia,
		AcademiaID:               academiaID,
		CreatedAt:                time.Now(),
		Documentos:               documentos,
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
	telefoneResponsavel *string,
	bilheteIdentidade *string,
	bilheteIdentidadeResp *string,
	dataNascimento *time.Time,
) error {
	if nome == nil && email == nil && telefone == nil && telefoneResponsavel == nil &&
		bilheteIdentidade == nil && bilheteIdentidadeResp == nil && dataNascimento == nil {
		return fmt.Errorf("nenhum campo para atualizar")
	}
	if dataNascimento != nil {
		if err := validarDataNascimento(*dataNascimento); err != nil {
			return err
		}
	}
	effectiveBilhete := e.BilheteIdentidade
	if bilheteIdentidade != nil {
		effectiveBilhete = bilheteIdentidade
	}
	effectiveBilheteResp := e.BilheteIdentidadeResp
	if bilheteIdentidadeResp != nil {
		effectiveBilheteResp = bilheteIdentidadeResp
	}
	if err := ValidarBilhetesMatricula(effectiveBilhete, effectiveBilheteResp); err != nil {
		return err
	}
	if !isNilOrBlank(e.AnoEscolar) || !isNilOrBlank(e.AnoEscolarMedio) {
		if isNilOrBlank(effectiveBilheteResp) {
			return fmt.Errorf("bilhete_identidade_responsavel é obrigatório para estudante escolar")
		}
	}
	effectiveTelefone := e.Telefone
	if telefone != nil {
		effectiveTelefone = telefone
	}
	effectiveTelefoneResp := e.TelefoneResponsavel
	if telefoneResponsavel != nil {
		effectiveTelefoneResp = telefoneResponsavel
	}
	if effectiveTelefone == nil && effectiveTelefoneResp == nil {
		return fmt.Errorf("telefone ou telefone_responsavel deve ser informado")
	}
	if effectiveTelefone != nil {
		if err := utils.ValidatePhone(*effectiveTelefone); err != nil {
			return err
		}
	}
	if effectiveTelefoneResp != nil {
		if err := utils.ValidatePhone(*effectiveTelefoneResp); err != nil {
			return err
		}
	}
	if effectiveTelefone != nil && effectiveTelefoneResp != nil && *effectiveTelefone == *effectiveTelefoneResp {
		return fmt.Errorf("telefone e telefone_responsavel não podem ser iguais")
	}

	emailAlterado := email != nil && (e.Email == nil || *e.Email != *email)
	event := &DadosPessoaisAtualizadosEvent{
		BaseEvent:             BaseEvent{EventType: "DadosPessoaisAtualizados", AggregateID: e.ID},
		Nome:                  nome,
		Email:                 email,
		Telefone:              telefone,
		TelefoneResponsavel:   telefoneResponsavel,
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

func (e *Estudante) MatricularFundamental(anoEscolar string, efetuadoPor uuid.UUID) error {
	if err := utils.ValidateAnoFundamental(anoEscolar); err != nil {
		return fmt.Errorf("ano_escolar_fundamental inválido: %w", err)
	}
	if e.Status == "arquivado" {
		return fmt.Errorf("estudante arquivado deve ser reintegrado antes de receber matrícula")
	}
	event := &MatriculaFundamentalEfetivadaEvent{
		BaseEvent:    BaseEvent{EventType: "MatriculaFundamentalEfetivada", AggregateID: e.ID},
		AnoEscolar:   anoEscolar,
		EfetivadaPor: efetuadoPor,
		EfetivadaAt:  time.Now(),
	}
	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) InterromperFundamental(motivo string, interrompidoPor uuid.UUID) error {
	if strings.TrimSpace(motivo) == "" {
		return fmt.Errorf("motivo é obrigatório")
	}
	if e.StatusEscolarFundamental != "em_andamento" {
		return fmt.Errorf("só pode interromper fundamental em andamento")
	}
	event := &FundamentalInterrompidoEvent{BaseEvent: BaseEvent{EventType: "FundamentalInterrompido", AggregateID: e.ID}, Motivo: motivo, InterrompidoPor: interrompidoPor, InterrompidoAt: time.Now()}
	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) MatricularMedio(anoEscolar string, cursoID uuid.UUID, efetuadoPor uuid.UUID) error {
	if err := utils.ValidateAnoMedio(anoEscolar); err != nil {
		return fmt.Errorf("ano_escolar_medio inválido: %w", err)
	}
	if cursoID == uuid.Nil {
		return fmt.Errorf("curso_id é obrigatório")
	}
	if e.StatusEscolarFundamental != "finalizado" {
		return fmt.Errorf("matrícula no médio exige status_escolar_fundamental finalizado")
	}
	event := &MatriculaMedioEfetivadaEvent{BaseEvent: BaseEvent{EventType: "MatriculaMedioEfetivada", AggregateID: e.ID}, AnoEscolar: anoEscolar, CursoID: cursoID, EfetivadaPor: efetuadoPor, EfetivadaAt: time.Now()}
	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) InterromperMedio(motivo string, interrompidoPor uuid.UUID) error {
	if strings.TrimSpace(motivo) == "" {
		return fmt.Errorf("motivo é obrigatório")
	}
	if e.StatusEscolarMedio != "em_andamento" {
		return fmt.Errorf("só pode interromper médio em andamento")
	}
	event := &MedioInterrompidoEvent{BaseEvent: BaseEvent{EventType: "MedioInterrompido", AggregateID: e.ID}, Motivo: motivo, InterrompidoPor: interrompidoPor, InterrompidoAt: time.Now()}
	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) MatricularSuperior(cursoID uuid.UUID, efetuadoPor uuid.UUID) error {
	if cursoID == uuid.Nil {
		return fmt.Errorf("curso_id é obrigatório")
	}
	if e.StatusEscolarFundamental != "finalizado" && e.StatusEscolarFundamental != "inativo" {
		return fmt.Errorf("matrícula superior exige fundamental finalizado ou inativo")
	}
	if e.StatusEscolarMedio != "finalizado" && e.StatusEscolarMedio != "inativo" {
		return fmt.Errorf("matrícula superior exige médio finalizado ou inativo")
	}
	ano := "1_ano_superior"
	semestre := 1
	if e.CursoSuperiorID != nil && *e.CursoSuperiorID == cursoID {
		if e.AnoSuperior != nil {
			ano = *e.AnoSuperior
		}
		if e.SemestreAtual != nil && *e.SemestreAtual > 0 {
			semestre = *e.SemestreAtual
		}
	}
	event := &MatriculaSuperiorEfetivadaEvent{BaseEvent: BaseEvent{EventType: "MatriculaSuperiorEfetivada", AggregateID: e.ID}, CursoID: cursoID, AnoSuperior: ano, SemestreAtual: semestre, EfetivadaPor: efetuadoPor, EfetivadaAt: time.Now()}
	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) TrancarSuperior(motivo string, trancadoPor uuid.UUID) error {
	if strings.TrimSpace(motivo) == "" {
		return fmt.Errorf("motivo é obrigatório")
	}
	if e.StatusSuperior != "em_andamento" {
		return fmt.Errorf("só pode trancar superior em andamento")
	}
	event := &SuperiorTrancadoEvent{BaseEvent: BaseEvent{EventType: "SuperiorTrancado", AggregateID: e.ID}, Motivo: motivo, TrancadoPor: trancadoPor, TrancadoAt: time.Now()}
	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) DesvincularDaAcademia(codigoAcademia, motivo string, desvinculadoPor uuid.UUID) error {
	if e.CodigoAcademia == nil || *e.CodigoAcademia != codigoAcademia {
		return fmt.Errorf("estudante não pertence a esta academia")
	}
	if e.Status != "ativo" {
		return fmt.Errorf("apenas estudante ativo pode ser desvinculado")
	}
	if strings.TrimSpace(motivo) == "" {
		return fmt.Errorf("motivo é obrigatório")
	}
	nivel := e.nivelAcademicoAtual()
	event := &EstudanteDesvinculadoDaAcademiaEvent{BaseEvent: BaseEvent{EventType: "EstudanteDesvinculadoDaAcademia", AggregateID: e.ID}, CodigoAcademia: codigoAcademia, CodigoEstudante: e.CodigoEstudante, Motivo: motivo, Nivel: nivel, DesvinculadoPor: desvinculadoPor, DesvinculadoAt: time.Now()}
	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) Reintegrar(codigoAcademia, tipoEnsino string, anoEscolar, anoEscolarMedio *string, cursoMedioID, cursoSuperiorID *uuid.UUID, reintegradoPor uuid.UUID) error {
	if e.Status != "arquivado" {
		return fmt.Errorf("apenas estudante arquivado pode ser reintegrado")
	}
	event := &EstudanteReintegradoEvent{BaseEvent: BaseEvent{EventType: "EstudanteReintegrado", AggregateID: e.ID}, CodigoAcademia: codigoAcademia, CodigoEstudante: e.CodigoEstudante, TipoEnsino: tipoEnsino, ReintegradoPor: reintegradoPor, ReintegradoAt: time.Now()}
	switch tipoEnsino {
	case "fundamental":
		anoEscolar = e.AnoEscolar
		if anoEscolar == nil {
			return fmt.Errorf("não foi possível determinar o ano_escolar_fundamental anterior do estudante para reingresso")
		}
		if err := utils.ValidateAnoFundamental(*anoEscolar); err != nil {
			return fmt.Errorf("ano_escolar_fundamental inválido: %w", err)
		}
		event.AnoEscolar = anoEscolar
	case "medio":
		if cursoMedioID == nil {
			cursoMedioID = e.CursoMedioID
		}
		if cursoMedioID == nil {
			return fmt.Errorf("não foi possível determinar o curso_medio_id anterior do estudante para reingresso")
		}
		if e.CursoMedioID != nil && *e.CursoMedioID == *cursoMedioID && e.AnoEscolarMedio != nil {
			anoEscolarMedio = e.AnoEscolarMedio
		} else {
			inicio := "1_ano_medio"
			anoEscolarMedio = &inicio
		}
		if anoEscolarMedio == nil {
			return fmt.Errorf("ano_escolar_medio é obrigatório")
		}
		if err := utils.ValidateAnoMedio(*anoEscolarMedio); err != nil {
			return fmt.Errorf("ano_escolar_medio inválido: %w", err)
		}
		event.AnoEscolarMedio = anoEscolarMedio
		event.CursoMedioID = cursoMedioID
	case "superior":
		if cursoSuperiorID == nil {
			cursoSuperiorID = e.CursoSuperiorID
		}
		if cursoSuperiorID == nil {
			return fmt.Errorf("não foi possível determinar o curso_superior_id anterior do estudante para reingresso")
		}
		ano := "1_ano_superior"
		semestre := 1
		if e.CursoSuperiorID != nil && *e.CursoSuperiorID == *cursoSuperiorID {
			if e.AnoSuperior != nil {
				ano = *e.AnoSuperior
			}
			if e.SemestreAtual != nil && *e.SemestreAtual > 0 {
				semestre = *e.SemestreAtual
			}
		}
		event.AnoSuperior = &ano
		event.SemestreAtual = &semestre
		event.CursoSuperiorID = cursoSuperiorID
	default:
		return fmt.Errorf("tipo_ensino deve ser fundamental, medio ou superior")
	}
	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) nivelAcademicoAtual() string {
	if e.StatusSuperior == "em_andamento" {
		if e.AnoSuperior != nil && e.SemestreAtual != nil {
			return fmt.Sprintf("superior:%s:semestre_%d", *e.AnoSuperior, *e.SemestreAtual)
		}
		if e.AnoSuperior != nil {
			return "superior:" + *e.AnoSuperior
		}
		return "superior"
	}
	if e.StatusEscolarMedio == "em_andamento" {
		if e.AnoEscolarMedio != nil {
			return "medio:" + *e.AnoEscolarMedio
		}
		return "medio"
	}
	if e.StatusEscolarFundamental == "em_andamento" {
		if e.AnoEscolar != nil {
			return "fundamental:" + *e.AnoEscolar
		}
		return "fundamental"
	}
	return "sem_etapa_em_andamento"
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
		if e.StatusEscolarFundamental != "finalizado" {
			return fmt.Errorf("só pode alterar curso médio se status_escolar_fundamental for 'finalizado'")
		}
		if e.CursoMedioID != nil && *e.CursoMedioID == cursoID {
			return nil
		}
	} else {
		if e.StatusSuperior != "em_andamento" {
			return fmt.Errorf("só pode alterar curso superior se status_superior for 'em_andamento'")
		}
		if e.CursoSuperiorID != nil && *e.CursoSuperiorID == cursoID {
			return nil
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
	e.TelefoneResponsavel = ev.TelefoneResponsavel
	e.BilheteIdentidade = ev.BilheteIdentidade
	e.BilheteIdentidadeResp = ev.BilheteIdentidadeResp
	e.Genero = ev.Genero
	e.DataNascimento = ev.DataNascimento
	e.AnoEscolar = ev.AnoEscolar
	e.AnoEscolarMedio = ev.AnoEscolarMedio
	e.AnoSuperior = ev.AnoSuperior
	e.SemestreAtual = ev.SemestreAtual
	e.CursoMedioID = ev.CursoMedioID
	e.CursoSuperiorID = ev.CursoSuperiorID
	e.StatusEscolarFundamental = ev.StatusEscolarFundamental
	e.StatusEscolarMedio = ev.StatusEscolarMedio
	e.StatusSuperior = ev.StatusSuperior
	e.CodigoAcademia = &ev.CodigoAcademia
	e.Status = "ativo"
	e.CreatedAt = ev.CreatedAt
	e.Documentos = ev.Documentos
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

func (e *Estudante) applyFundamentalEmAndamento(event DomainEvent) error {
	data, _ := json.Marshal(event.GetPayload())
	var ev struct{ AnoEscolar string }
	_ = json.Unmarshal(data, &ev)
	e.StatusEscolarFundamental = "em_andamento"
	if event.GetEventType() == "FundamentalRetomado" && e.AnoEscolar != nil {
		return nil
	}
	if ev.AnoEscolar != "" {
		e.AnoEscolar = &ev.AnoEscolar
	}
	return nil
}

func (e *Estudante) applyFundamentalInativo(DomainEvent) error {
	e.StatusEscolarFundamental = "inativo"
	return nil
}
func (e *Estudante) applyFundamentalFinalizado(DomainEvent) error {
	e.StatusEscolarFundamental = "finalizado"
	return nil
}

func (e *Estudante) applyMedioEmAndamento(event DomainEvent) error {
	data, _ := json.Marshal(event.GetPayload())
	var ev struct {
		AnoEscolar string
		CursoID    uuid.UUID
	}
	_ = json.Unmarshal(data, &ev)
	e.StatusEscolarMedio = "em_andamento"
	if event.GetEventType() == "MedioRetomado" {
		if ev.CursoID != uuid.Nil && e.CursoMedioID != nil && *e.CursoMedioID == ev.CursoID && e.AnoEscolarMedio != nil {
			return nil
		}
		if ev.CursoID != uuid.Nil && (e.CursoMedioID == nil || *e.CursoMedioID != ev.CursoID) {
			inicio := "1_ano_medio"
			e.AnoEscolarMedio = &inicio
			e.CursoMedioID = &ev.CursoID
			return nil
		}
	}
	if ev.AnoEscolar != "" {
		e.AnoEscolarMedio = &ev.AnoEscolar
	}
	if ev.CursoID != uuid.Nil {
		e.CursoMedioID = &ev.CursoID
	}
	return nil
}

func (e *Estudante) applyMedioInativo(DomainEvent) error {
	e.StatusEscolarMedio = "inativo"
	return nil
}
func (e *Estudante) applyMedioFinalizado(DomainEvent) error {
	e.StatusEscolarMedio = "finalizado"
	return nil
}

func (e *Estudante) applySuperiorEmAndamento(event DomainEvent) error {
	data, _ := json.Marshal(event.GetPayload())
	var ev struct {
		CursoID       uuid.UUID
		AnoSuperior   string
		SemestreAtual int
	}
	_ = json.Unmarshal(data, &ev)
	e.StatusSuperior = "em_andamento"
	if event.GetEventType() == "MatriculaSuperiorReativada" {
		if ev.CursoID != uuid.Nil && e.CursoSuperiorID != nil && *e.CursoSuperiorID == ev.CursoID {
			return nil
		}
		if ev.CursoID != uuid.Nil {
			ano := "1_ano_superior"
			semestre := 1
			e.CursoSuperiorID = &ev.CursoID
			e.AnoSuperior = &ano
			e.SemestreAtual = &semestre
			return nil
		}
	}
	if ev.CursoID != uuid.Nil {
		e.CursoSuperiorID = &ev.CursoID
	}
	if ev.AnoSuperior != "" {
		e.AnoSuperior = &ev.AnoSuperior
	}
	if ev.SemestreAtual > 0 {
		e.SemestreAtual = &ev.SemestreAtual
	}
	return nil
}

func (e *Estudante) applySuperiorInativo(DomainEvent) error { e.StatusSuperior = "inativo"; return nil }

func (e *Estudante) applyEstudanteDesvinculadoDaAcademia(DomainEvent) error {
	e.Status = "arquivado"
	return nil
}

func (e *Estudante) applyEstudanteReintegrado(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applyEstudanteReintegrado: marshal error: %w", err)
	}
	var ev EstudanteReintegradoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyEstudanteReintegrado: unmarshal error: %w", err)
	}
	e.Status = "ativo"
	e.CodigoAcademia = &ev.CodigoAcademia
	switch ev.TipoEnsino {
	case "fundamental":
		e.StatusEscolarFundamental = "em_andamento"
		e.AnoEscolar = ev.AnoEscolar
	case "medio":
		e.StatusEscolarMedio = "em_andamento"
		e.AnoEscolarMedio = ev.AnoEscolarMedio
		e.CursoMedioID = ev.CursoMedioID
	case "superior":
		e.StatusSuperior = "em_andamento"
		e.AnoSuperior = ev.AnoSuperior
		e.SemestreAtual = ev.SemestreAtual
		e.CursoSuperiorID = ev.CursoSuperiorID
	}
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
		inicio := "1_ano_medio"
		e.AnoEscolarMedio = &inicio
	} else {
		e.CursoSuperiorID = &ev.CursoID
		ano := "1_ano_superior"
		semestre := 1
		e.AnoSuperior = &ano
		e.SemestreAtual = &semestre
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
	if ev.TelefoneResponsavel != nil {
		e.TelefoneResponsavel = ev.TelefoneResponsavel
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
