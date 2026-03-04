// ============================================================================
// ARQUIVO: internal/domain/aggregates/academia.go
// ============================================================================
// REGRA DE ORGANIZAÇÃO:
//   academia.go                  → struct, Apply, comandos core, eventos base
//   academia_categorias_nota.go  → AdicionarCategoriaNotaSuperior,
//                                  CategoriaNotaAdicionadaEvent,
//                                  applyCategoriaNotaAdicionada
// ============================================================================

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

	// TotalEstudantes é mantido apenas pela projeção — não pelo aggregate.
	// Este campo existe no struct apenas para compatibilidade de leitura.
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
	case "AcademiaSenhaAlterada":
		// FIX C1: evento de senha passa pelo ledger
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
		// FIX C12: UUID do admin que criou a academia — rastreabilidade forense
		CriadoPor: criadoPor,
		CreatedAt: time.Now(),
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

// Desativar registra o evento de desativação.
//
// FIX C9: DesativadoPor adicionado ao payload do evento para auditoria forense.
func (a *Academia) Desativar(motivo string, desativadoPor uuid.UUID) error {
	if a.Status == "inativo" {
		return fmt.Errorf("academia já está inativa")
	}

	event := &AcademiaDesativadaEvent{
		BaseEvent:     BaseEvent{EventType: "AcademiaDesativada", AggregateID: a.ID},
		Motivo:        motivo,
		// FIX C9: quem desativou está no payload — não apenas nos metadados
		DesativadoPor: desativadoPor,
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
//
// FIX C5/C10: verificação de status adicionada ao aggregate como defesa em
// profundidade. O middleware ValidarStatusAcademia protege a rota, mas o aggregate
// agora também rejeita a operação se a academia estiver inativa — princípio de
// defesa em profundidade.
//
// FIX E-11 / E-14: revalida coerência de anos_academicos com nivel_escolar
// atual (ou o novo nivel_escolar, se estiver sendo alterado simultaneamente).
// Impede gravar anos inválidos no ledger.
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
	// FIX C5/C10: defesa em profundidade — aggregate verifica status
	if a.Status != "ativo" {
		return fmt.Errorf("academia está inativa — não é possível atualizar dados")
	}

	if nome == nil && provincia == nil && endereco == nil &&
		numeroTelefone == nil && email == nil && website == nil &&
		nivelEscolar == nil && anosAcademicos == nil && cursos == nil {
		return fmt.Errorf("nenhum campo para atualizar")
	}

	// FIX E-11 / E-14: determinar o nivel_escolar efetivo após a atualização
	// para validar anos_academicos corretamente.
	nivelEfetivo := a.NivelEscolar
	if nivelEscolar != nil {
		nivelEfetivo = nivelEscolar
	}

	// Revalidar anos_academicos se foram fornecidos ou se nivel_escolar mudou.
	if anosAcademicos != nil || nivelEscolar != nil {
		if _, err := validarAnosAcademicos(a.Type, nivelEfetivo, anosAcademicos); err != nil {
			return err
		}
	}

	// Impedir que academia superior receba nivel_escolar.
	if a.Type == "superior" && nivelEscolar != nil {
		return fmt.Errorf("academia do tipo 'superior' não pode ter nivel_escolar")
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
		AnosAcademicos: anosAcademicos,
		Cursos:         cursos,
		EmailAlterado:  emailAlterado,
		UpdatedAt:      time.Now(),
	}

	a.RaiseEvent(event)
	return a.Apply(event)
}

// AlterarSenha registra a troca de senha como evento no ledger.
//
// FIX C1: antes, a senha era alterada via UPDATE direto na projeção,
// bypassando o event sourcing. Agora emite AcademiaSenhaAlteradaEvent,
// garantindo: (1) rastreabilidade completa no ledger, (2) rebuild restaura
// a senha correta, (3) consistência com o padrão do Admin.
func (a *Academia) AlterarSenha(novaSenhaHash string, changedBy uuid.UUID, motivo string) error {
	if novaSenhaHash == "" {
		return fmt.Errorf("hash da nova senha não pode ser vazio")
	}

	event := &AcademiaSenhaAlteradaEvent{
		BaseEvent:     BaseEvent{EventType: "AcademiaSenhaAlterada", AggregateID: a.ID},
		NovaSenhaHash: novaSenhaHash,
		ChangedBy:     changedBy,
		Motivo:        motivo,
		ChangedAt:     time.Now(),
	}

	a.RaiseEvent(event)
	return a.Apply(event)
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
		// Escolas de médio NÃO devem ter anos_academicos
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
	// FIX C12: UUID do admin que criou a academia — para auditoria forense.
	// Nil se criado por processo interno sem usuário identificado.
	CriadoPor *uuid.UUID
	CreatedAt time.Time
}

func (e *AcademiaCriadaEvent) GetPayload() interface{} { return e }

type AcademiaAtivadaEvent struct {
	BaseEvent
	ActivatedAt time.Time
}

func (e *AcademiaAtivadaEvent) GetPayload() interface{} { return e }

// AcademiaDesativadaEvent — FIX C9: DesativadoPor adicionado para auditoria forense.
type AcademiaDesativadaEvent struct {
	BaseEvent
	Motivo string
	// FIX C9: UUID do admin responsável pela desativação — no payload do evento,
	// não apenas nos metadados do ledger. Permite auditoria forense sem metadata.
	DesativadoPor uuid.UUID
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

// AcademiaSenhaAlteradaEvent — FIX C1: evento de senha via event sourcing.
// Antes a senha era alterada via UPDATE direto, bypassando o ledger.
// Agora toda alteração de senha gera este evento, garantindo:
//   - Auditoria completa no ledger
//   - Rebuild restaura a senha correta
//   - Consistência com AdminSenhaAlteradaEvent
type AcademiaSenhaAlteradaEvent struct {
	BaseEvent
	NovaSenhaHash string
	// ChangedBy: UUID do usuário que realizou a troca
	// — pode ser a própria academia (auto-alteração) ou uuid.Nil para reset via sistema
	ChangedBy uuid.UUID
	// Motivo: "alteracao_usuario" | "reset_senha"
	Motivo    string
	ChangedAt time.Time
}

func (e *AcademiaSenhaAlteradaEvent) GetPayload() interface{} { return e }

// ============================================================================
// Apply handlers
// NOTA: applyCategoriaNotaAdicionada está em academia_categorias_nota.go
// ============================================================================

// FIX E-06: todos os apply handlers agora propagam erros de json.Unmarshal
// ao invés de silenciá-los com `_`. Estado parcial/zerado era aplicado
// silenciosamente quando o payload falhava no parse.

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
	// Status e EmailVerificado são sempre fixos na criação — independente do payload
	a.Status = "inativo"
	a.EmailVerificado = false
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

// applyAcademiaSenhaAlterada aplica a nova senha_hash ao estado do aggregate.
//
// FIX C1: implementação do apply handler para o novo evento de senha.
func (a *Academia) applyAcademiaSenhaAlterada(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applyAcademiaSenhaAlterada: marshal error: %w", err)
	}
	var ev AcademiaSenhaAlteradaEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyAcademiaSenhaAlterada: unmarshal error: %w", err)
	}
	a.SenhaHash = ev.NovaSenhaHash
	return nil
}