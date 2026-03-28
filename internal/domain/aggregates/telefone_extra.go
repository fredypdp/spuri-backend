package aggregates

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// telefoneRegex valida formatos internacionais e locais.
// Aceita: +244923000000, 923000000, +1-800-555-0000, etc.
var telefoneRegex = regexp.MustCompile(`^\+?[0-9]{7,15}$`)

// ============================================================================
// Aggregate
// ============================================================================

// TelefoneExtra representa um número de telefone adicional de qualquer usuário.
// É um aggregate dedicado para que cada adição/verificação fique registrada
// no ledger com rastreabilidade completa.
type TelefoneExtra struct {
	BaseAggregate

	IDUser         uuid.UUID
	TipoUser       string // "estudante" | "academia" | "admin"
	NumeroTelefone string
	Verificado     bool
	RegisteredAt   time.Time
}

func NewTelefoneExtra() *TelefoneExtra {
	return &TelefoneExtra{
		BaseAggregate: BaseAggregate{
			ID:                uuid.New(),
			Version:           0,
			UncommittedEvents: []DomainEvent{},
		},
		Verificado: false,
	}
}

func (t *TelefoneExtra) GetType() string { return "TelefoneExtra" }

// ============================================================================
// Apply dispatcher
// ============================================================================

func (t *TelefoneExtra) Apply(event DomainEvent) error {
	switch event.GetEventType() {
	case "TelefoneExtraAdicionado":
		return t.applyTelefoneExtraAdicionado(event)
	case "TelefoneExtraVerificado":
		return t.applyTelefoneExtraVerificado(event)
	default:
		return fmt.Errorf("tipo de evento desconhecido: %s", event.GetEventType())
	}
}

// ============================================================================
// Eventos
// ============================================================================

type TelefoneExtraAdicionadoEvent struct {
	BaseEvent
	IDUser         uuid.UUID
	TipoUser       string
	NumeroTelefone string
	RegisteredAt   time.Time
}

func (e *TelefoneExtraAdicionadoEvent) GetPayload() interface{} { return e }
func (e *TelefoneExtraAdicionadoEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type TelefoneExtraVerificadoEvent struct {
	BaseEvent
	IDUser         uuid.UUID
	TipoUser       string
	NumeroTelefone string
	VerificadoEm   time.Time
}

func (e *TelefoneExtraVerificadoEvent) GetPayload() interface{} { return e }
func (e *TelefoneExtraVerificadoEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

// ============================================================================
// Comandos
// ============================================================================

// Adicionar registra um novo telefone extra para um usuário.
//
// Pré-condições verificadas no aggregate:
//   - número tem formato válido
//   - tipo_user é um dos valores permitidos
//
// Pré-condições verificadas pelo HANDLER (requerem acesso à projeção):
//   - número não está verificado por outro usuário
//   - usuário ainda não cadastrou este número
func (t *TelefoneExtra) Adicionar(
	idUser uuid.UUID,
	tipoUser string,
	numeroTelefone string,
) error {
	if idUser == uuid.Nil {
		return fmt.Errorf("id_user inválido")
	}

	tiposValidos := map[string]bool{"estudante": true, "academia": true, "admin": true}
	if !tiposValidos[tipoUser] {
		return fmt.Errorf("tipo_user deve ser 'estudante', 'academia' ou 'admin'")
	}

	normalized := normalizarTelefone(numeroTelefone)
	if err := validarTelefoneExtra(normalized); err != nil {
		return err
	}

	event := &TelefoneExtraAdicionadoEvent{
		BaseEvent:      BaseEvent{EventType: "TelefoneExtraAdicionado", AggregateID: t.ID},
		IDUser:         idUser,
		TipoUser:       tipoUser,
		NumeroTelefone: normalized,
		RegisteredAt:   time.Now(),
	}
	t.RaiseEvent(event)
	return t.Apply(event)
}

// Verificar marca o telefone como verificado pelo usuário.
//
// Pré-condições verificadas no aggregate:
//   - telefone ainda não foi verificado
//   - a verificação pertence ao dono do registro
//
// Pré-condições verificadas pelo HANDLER (requerem acesso à projeção):
//   - número não está verificado por outro usuário
func (t *TelefoneExtra) Verificar(idUser uuid.UUID, tipoUser string) error {
	if t.Verificado {
		return fmt.Errorf("telefone já está verificado")
	}
	if t.IDUser != idUser {
		return fmt.Errorf("apenas o dono pode verificar este telefone")
	}
	if t.TipoUser != tipoUser {
		return fmt.Errorf("tipo de usuário não corresponde ao dono do telefone")
	}

	event := &TelefoneExtraVerificadoEvent{
		BaseEvent:      BaseEvent{EventType: "TelefoneExtraVerificado", AggregateID: t.ID},
		IDUser:         idUser,
		TipoUser:       tipoUser,
		NumeroTelefone: t.NumeroTelefone,
		VerificadoEm:   time.Now(),
	}
	t.RaiseEvent(event)
	return t.Apply(event)
}

// ============================================================================
// Apply handlers
// ============================================================================

func (t *TelefoneExtra) applyTelefoneExtraAdicionado(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applyTelefoneExtraAdicionado: marshal error: %w", err)
	}
	var ev TelefoneExtraAdicionadoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyTelefoneExtraAdicionado: unmarshal error: %w", err)
	}
	t.IDUser = ev.IDUser
	t.TipoUser = ev.TipoUser
	t.NumeroTelefone = ev.NumeroTelefone
	t.Verificado = false
	t.RegisteredAt = ev.RegisteredAt
	return nil
}

func (t *TelefoneExtra) applyTelefoneExtraVerificado(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applyTelefoneExtraVerificado: marshal error: %w", err)
	}
	var ev TelefoneExtraVerificadoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyTelefoneExtraVerificado: unmarshal error: %w", err)
	}
	t.Verificado = true
	return nil
}

// ============================================================================
// Helpers internos
// ============================================================================

// normalizarTelefone remove espaços, hífens e parênteses.
func normalizarTelefone(numero string) string {
	s := strings.TrimSpace(numero)
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, "(", "")
	s = strings.ReplaceAll(s, ")", "")
	return s
}

// validarTelefoneExtra verifica formato após normalização.
func validarTelefoneExtra(numero string) error {
	if numero == "" {
		return fmt.Errorf("numero_telefone não pode ser vazio")
	}
	if !telefoneRegex.MatchString(numero) {
		return fmt.Errorf(
			"numero_telefone '%s' inválido: use apenas dígitos (7–15), opcionalmente precedidos de '+'",
			numero,
		)
	}
	return nil
}
