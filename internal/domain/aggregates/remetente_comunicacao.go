package aggregates

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	ProvedorComunicacaoGoSMS = "GOSMS"
	ProvedorComunicacaoZiett = "ZIETT"
)

// RemetenteComunicacaoIDGoSMS e RemetenteComunicacaoIDZiett são IDs
// determinísticos e fixos: existe no máximo UM RemetenteComunicacao por
// provedor. "Cadastrar remetente" para um provedor que já tem um remetente
// configurado SUBSTITUI o remetente existente (novo evento sobre o MESMO
// agregado, nunca um agregado novo) — não existe conceito de múltiplos
// remetentes para o mesmo provedor. Gerados uma única vez com
// uuid.NewSHA1(uuid.NameSpaceOID, []byte("spuri.comunicacao.remetente.GOSMS"))
// (e o equivalente para ZIETT) e fixados aqui como constantes — nunca
// recalcular em runtime nem gerar novos valores.
var (
	RemetenteComunicacaoIDGoSMS = uuid.MustParse("04b3ec64-e246-53cf-8600-dc247c1f12e2")
	RemetenteComunicacaoIDZiett = uuid.MustParse("ba19eb30-516c-5d07-a5a8-b47ada777e0c")
)

// RemetenteAggregateID retorna o ID determinístico do agregado
// RemetenteComunicacao para o provedor informado (já normalizado para
// maiúsculas), ou uuid.Nil e false se o provedor não for reconhecido.
func RemetenteAggregateID(provedor string) (uuid.UUID, bool) {
	switch provedor {
	case ProvedorComunicacaoGoSMS:
		return RemetenteComunicacaoIDGoSMS, true
	case ProvedorComunicacaoZiett:
		return RemetenteComunicacaoIDZiett, true
	default:
		return uuid.Nil, false
	}
}

type RemetenteComunicacao struct {
	BaseAggregate
	Provedor           string
	Identificador      string
	TokenAPICifrado    string
	ConfiguradoPor     uuid.UUID
	ConfiguradoPorTipo string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func NewRemetenteComunicacao() *RemetenteComunicacao {
	return &RemetenteComunicacao{BaseAggregate: BaseAggregate{ID: uuid.New(), Version: 0, UncommittedEvents: []DomainEvent{}}}
}
func (r *RemetenteComunicacao) GetType() string { return "RemetenteComunicacao" }

type RemetenteComunicacaoConfiguradoEvent struct {
	BaseEvent
	Provedor           string
	Identificador      string
	TokenAPICifrado    string
	ConfiguradoPor     uuid.UUID
	ConfiguradoPorTipo string
	ConfiguradoEm      time.Time
}

func (e *RemetenteComunicacaoConfiguradoEvent) GetPayload() interface{} { return e }
func (e *RemetenteComunicacaoConfiguradoEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

func (r *RemetenteComunicacao) Apply(event DomainEvent) error {
	switch event.GetEventType() {
	case "RemetenteComunicacaoConfigurado":
		return r.applyConfigurado(event)
	default:
		return fmt.Errorf("tipo de evento desconhecido para RemetenteComunicacao: %s", event.GetEventType())
	}
}

var identificadorGoSMSRegex = regexp.MustCompile(`^[A-Z0-9]{1,11}$`)

// Configurar cria (primeira chamada) ou substitui (chamadas seguintes) as
// credenciais de envio para um provedor. Quem chama este método é
// responsável por já ter deixado r.ID igual ao ID determinístico de
// RemetenteAggregateID(provedor) antes de chamar Configurar (via SetID
// quando o agregado é novo, ou via repository.Load quando já existe — ver
// ResolverRemetenteAggregate em internal/handlers/comunicacao_handlers.go).
// tokenAPICifrado já deve vir cifrado pelo chamador
// (services.EncryptComunicacaoSegredo) — este método nunca lida com o
// token em texto plano.
func (r *RemetenteComunicacao) Configurar(provedor, identificador, tokenAPICifrado string, configuradoPor uuid.UUID) error {
	provedor = strings.ToUpper(strings.TrimSpace(provedor))
	if provedor != ProvedorComunicacaoGoSMS && provedor != ProvedorComunicacaoZiett {
		return fmt.Errorf("provedor inválido: use GOSMS ou ZIETT")
	}
	identificador = strings.TrimSpace(identificador)
	if identificador == "" {
		return fmt.Errorf("identificador é obrigatório")
	}
	switch provedor {
	case ProvedorComunicacaoGoSMS:
		identificador = strings.ToUpper(identificador)
		if !identificadorGoSMSRegex.MatchString(identificador) {
			return fmt.Errorf("identificador do GoSMS deve ter de 1 a 11 caracteres alfanuméricos maiúsculos (padrão de Sender ID alfanumérico GSM), ex.: SPURI")
		}
	case ProvedorComunicacaoZiett:
		if _, err := uuid.Parse(identificador); err != nil {
			return fmt.Errorf("identificador do Ziett deve ser o UUID do remitter_id configurado no painel da Ziett")
		}
	}
	if strings.TrimSpace(tokenAPICifrado) == "" {
		return fmt.Errorf("token de API é obrigatório")
	}
	e := &RemetenteComunicacaoConfiguradoEvent{
		BaseEvent:          BaseEvent{EventType: "RemetenteComunicacaoConfigurado", AggregateID: r.ID},
		Provedor:           provedor,
		Identificador:      identificador,
		TokenAPICifrado:    tokenAPICifrado,
		ConfiguradoPor:     configuradoPor,
		ConfiguradoPorTipo: "admin",
		ConfiguradoEm:      time.Now(),
	}
	r.RaiseEvent(e)
	return r.Apply(e)
}

func (r *RemetenteComunicacao) applyConfigurado(e DomainEvent) error {
	b, err := json.Marshal(e.GetPayload())
	if err != nil {
		return err
	}
	var p RemetenteComunicacaoConfiguradoEvent
	if err = json.Unmarshal(b, &p); err != nil {
		return err
	}
	r.Provedor = p.Provedor
	r.Identificador = p.Identificador
	r.TokenAPICifrado = p.TokenAPICifrado
	r.ConfiguradoPor = p.ConfiguradoPor
	r.ConfiguradoPorTipo = p.ConfiguradoPorTipo
	if r.CreatedAt.IsZero() {
		r.CreatedAt = p.ConfiguradoEm
	}
	r.UpdatedAt = p.ConfiguradoEm
	return nil
}
