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
	AnosAcademicos []string
	Status         string
	Cursos         []string
	CreatedAt      time.Time

	// CategoriasNota mantém as categorias de nota cadastradas pela academia.
	// FIX A-02: campo adicionado para que o aggregate possa detectar duplicatas.
	CategoriasNota []string

	// FIX A-01/A-03: campos de auditoria de ativação/desativação.
	AtivadoPor    uuid.UUID
	AtivadoEm     time.Time
	DesativadoPor uuid.UUID
	DesativadoEm  time.Time

	// TotalEstudantes é mantido apenas pela projeção — não pelo aggregate.
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
		CategoriasNota:  []string{},
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
	case "AcademiaSenhaAlterada":
		return a.applyAcademiaSenhaAlterada(event)
	case "CategoriaNotaAdicionada":
		// applyCategoriaNotaAdicionada definido em academia_categorias_nota.go
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
//
// FIX C12: CriadoPor adicionado ao evento para rastreabilidade forense completa.
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
	criadoPor *uuid.UUID,
) error {
	if tipo != "escola" && tipo != "superior" {
		return fmt.Errorf("tipo deve ser 'escola' ou 'superior'")
	}
	if nome == "" || codigoAcademia == "" {
		return fmt.Errorf("campos obrigatórios vazios")
	}
	if senhaHash == "" {
		return fmt.Errorf("senha_hash não pode ser vazio")
	}
	if tipo == "escola" && nivelEscolar == nil {
		return fmt.Errorf("nivel_escolar é obrigatório para escolas")
	}

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
		CriadoPor:      criadoPor,
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

// Ativar registra a ativação sem autor explícito (legado — mantido para
// retrocompatibilidade com handlers existentes).
// Use AtivarComAutor quando o ID do executor estiver disponível.
func (a *Academia) Ativar() error {
	return a.AtivarComAutor(uuid.Nil)
}

// AtivarComAutor — FIX A-03: registra a ativação com o UUID de quem ativou.
// Etapa 4 deve migrar os handlers de academia para chamar este método
// em vez de Ativar().
func (a *Academia) AtivarComAutor(ativadoPor uuid.UUID) error {
	if a.Status == "ativo" {
		return fmt.Errorf("academia já está ativa")
	}

	event := &AcademiaAtivadaEvent{
		BaseEvent:   BaseEvent{EventType: "AcademiaAtivada", AggregateID: a.ID},
		AtivadoPor:  ativadoPor,
		ActivatedAt: time.Now(),
	}

	a.RaiseEvent(event)
	return a.Apply(event)
}

// Desativar registra o evento de desativação.
// FIX C9: DesativadoPor adicionado ao payload do evento para auditoria forense.
func (a *Academia) Desativar(motivo string, desativadoPor uuid.UUID) error {
	if a.Status == "inativo" {
		return fmt.Errorf("academia já está inativa")
	}

	event := &AcademiaDesativadaEvent{
		BaseEvent:     BaseEvent{EventType: "AcademiaDesativada", AggregateID: a.ID},
		Motivo:        motivo,
		DesativadoPor: desativadoPor,
		DeactivatedAt: time.Now(),
	}

	a.RaiseEvent(event)
	return a.Apply(event)
}

func (a *Academia) AtualizarCursos(novosCursos []string) error {
	event := &CursosAtualizadosEvent{
		BaseEvent:  BaseEvent{EventType: "CursosAtualizados", AggregateID: a.ID},
		NovoCursos: novosCursos,
		UpdatedAt:  time.Now(),
	}

	a.RaiseEvent(event)
	return a.Apply(event)
}

func (a *Academia) AtualizarDados(
	nome *string,
	provincia *string,
	endereco *string,
	numeroTelefone *string,
	email *string,
	website *string,
	nivelEscolar *string,
	anosAcademicos []string,
	cursos []string,
) error {
	if nome == nil && provincia == nil && endereco == nil &&
		numeroTelefone == nil && email == nil && website == nil &&
		nivelEscolar == nil && anosAcademicos == nil && cursos == nil {
		return fmt.Errorf("nenhum campo para atualizar")
	}

	emailAlterado := email != nil && a.Email != nil && *a.Email != *email

	event := &AcademiaDadosAtualizadosEvent{
		BaseEvent:      BaseEvent{EventType: "AcademiaDadosAtualizados", AggregateID: a.ID},
		Nome:           nome,
		Provincia:      provincia,
		Endereco:       endereco,
		NumeroTelefone: numeroTelefone,
		Email:          email,
		Website:        website,
		NivelEscolar:   nivelEscolar,
		AnosAcademicos: anosAcademicos,
		Cursos:         cursos,
		EmailAlterado:  emailAlterado,
		UpdatedAt:      time.Now(),
	}

	a.RaiseEvent(event)
	return a.Apply(event)
}

// AlterarSenha emite o evento AcademiaSenhaAlterada via event sourcing.
// FIX C1: senha agora passa pelo ledger.
func (a *Academia) AlterarSenha(novaSenhaHash string, alteradoPor uuid.UUID, motivo string) error {
	if novaSenhaHash == "" {
		return fmt.Errorf("senha_hash não pode ser vazio")
	}

	event := &AcademiaSenhaAlteradaEvent{
		BaseEvent:     BaseEvent{EventType: "AcademiaSenhaAlterada", AggregateID: a.ID},
		NovaSenhaHash: novaSenhaHash,
		AlteradoPor:   alteradoPor,
		Motivo:        motivo,
		ChangedAt:     time.Now(),
	}

	a.RaiseEvent(event)
	return a.Apply(event)
}

// ============================================================================
// Apply handlers
// NOTA: applyCategoriaNotaAdicionada está em academia_categorias_nota.go
// ============================================================================

func (a *Academia) applyAcademiaCriada(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applyAcademiaCriada: marshal error: %w", err)
	}

	var ev AcademiaCriadaEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyAcademiaCriada: unmarshal error: %w", err)
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
	a.CreatedAt = ev.CreatedAt
	a.Status = "inativo"
	a.EmailVerificado = false
	if a.CategoriasNota == nil {
		a.CategoriasNota = []string{}
	}
	return nil
}

// applyAcademiaAtivada — FIX A-01/A-03: deserializa o payload para atualizar
// AtivadoPor e AtivadoEm no estado do aggregate.
func (a *Academia) applyAcademiaAtivada(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applyAcademiaAtivada: marshal error: %w", err)
	}
	var ev AcademiaAtivadaEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyAcademiaAtivada: unmarshal error: %w", err)
	}
	a.Status = "ativo"
	a.AtivadoPor = ev.AtivadoPor
	a.AtivadoEm = ev.ActivatedAt
	return nil
}

// applyAcademiaDesativada — FIX A-01: deserializa o payload para atualizar
// DesativadoPor e DesativadoEm no estado do aggregate.
func (a *Academia) applyAcademiaDesativada(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applyAcademiaDesativada: marshal error: %w", err)
	}
	var ev AcademiaDesativadaEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyAcademiaDesativada: unmarshal error: %w", err)
	}
	a.Status = "inativo"
	a.DesativadoPor = ev.DesativadoPor
	a.DesativadoEm = ev.DeactivatedAt
	return nil
}

func (a *Academia) applyCursosAtualizados(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applyCursosAtualizados: marshal error: %w", err)
	}
	var ev CursosAtualizadosEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyCursosAtualizados: unmarshal error: %w", err)
	}
	a.Cursos = ev.NovoCursos
	return nil
}

func (a *Academia) applyAcademiaDadosAtualizados(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applyAcademiaDadosAtualizados: marshal error: %w", err)
	}
	var ev AcademiaDadosAtualizadosEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyAcademiaDadosAtualizados: unmarshal error: %w", err)
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

func (a *Academia) applyAcademiaSenhaAlterada(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applyAcademiaSenhaAlterada: marshal error: %w", err)
	}
	var ev AcademiaSenhaAlteradaEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyAcademiaSenhaAlterada: unmarshal error: %w", err)
	}
	if ev.NovaSenhaHash == "" {
		return fmt.Errorf("applyAcademiaSenhaAlterada: NovaSenhaHash vazio no payload")
	}
	a.SenhaHash = ev.NovaSenhaHash
	return nil
}

// ============================================================================
// Validações internas
// ============================================================================

func validarAnosAcademicos(tipo string, nivelEscolar *string, anos []string) ([]string, error) {
	if tipo == "superior" {
		return nil, nil
	}

	if nivelEscolar == nil {
		return nil, nil
	}

	switch *nivelEscolar {
	case "fundamental", "misto":
		if len(anos) == 0 {
			return nil, fmt.Errorf(
				"escolas de nivel_escolar '%s' devem definir anos_academicos. "+
					"Informe ao menos um ano (ex: primeiro_fundamental, segundo_fundamental, etc.)",
				*nivelEscolar,
			)
		}
		if err := utils.ValidateAnosFundamental(anos); err != nil {
			return nil, err
		}
		return anos, nil

	case "medio":
		if len(anos) > 0 {
			return nil, fmt.Errorf(
				"escolas de nivel_escolar 'medio' não devem definir anos_academicos",
			)
		}
		return nil, nil
	}

	return nil, nil
}

// ============================================================================
// Eventos
// ============================================================================

// AcademiaCriadaEvent — FIX C12: CriadoPor adicionado para rastreabilidade.
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
	CriadoPor      *uuid.UUID
	CreatedAt      time.Time
}

func (e *AcademiaCriadaEvent) GetPayload() interface{} { return e }
func (e *AcademiaCriadaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

// AcademiaAtivadaEvent — FIX A-03: AtivadoPor adicionado para paridade com
// AcademiaDesativadaEvent (que já tinha DesativadoPor via FIX C9).
type AcademiaAtivadaEvent struct {
	BaseEvent
	AtivadoPor  uuid.UUID
	ActivatedAt time.Time
}

func (e *AcademiaAtivadaEvent) GetPayload() interface{} { return e }
func (e *AcademiaAtivadaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

// AcademiaDesativadaEvent — FIX C9: DesativadoPor adicionado para auditoria forense.
type AcademiaDesativadaEvent struct {
	BaseEvent
	Motivo        string
	DesativadoPor uuid.UUID
	DeactivatedAt time.Time
}

func (e *AcademiaDesativadaEvent) GetPayload() interface{} { return e }
func (e *AcademiaDesativadaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type CursosAtualizadosEvent struct {
	BaseEvent
	NovoCursos []string
	UpdatedAt  time.Time
}

func (e *CursosAtualizadosEvent) GetPayload() interface{} { return e }
func (e *CursosAtualizadosEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type AcademiaDadosAtualizadosEvent struct {
	BaseEvent
	Nome           *string
	Provincia      *string
	Endereco       *string
	NumeroTelefone *string
	Email          *string
	Website        *string
	NivelEscolar   *string
	AnosAcademicos []string
	Cursos         []string
	EmailAlterado  bool
	UpdatedAt      time.Time
}

func (e *AcademiaDadosAtualizadosEvent) GetPayload() interface{} { return e }
func (e *AcademiaDadosAtualizadosEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

// AcademiaSenhaAlteradaEvent — FIX C1: evento de senha via event sourcing.
type AcademiaSenhaAlteradaEvent struct {
	BaseEvent
	NovaSenhaHash string
	AlteradoPor   uuid.UUID
	Motivo        string
	ChangedAt     time.Time
}

func (e *AcademiaSenhaAlteradaEvent) GetPayload() interface{} { return e }
func (e *AcademiaSenhaAlteradaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }
