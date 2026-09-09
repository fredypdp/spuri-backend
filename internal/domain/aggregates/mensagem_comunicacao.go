package aggregates

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	StatusMensagemComunicacaoEnviada = "enviada"
	StatusMensagemComunicacaoFalhou  = "falhou"
)

// TentativaEnvioComunicacao registra o resultado de UMA tentativa de envio
// por UM provedor (parte do array detalhes_tentativas gravado em
// MensagemComunicacaoRegistradaEvent). No máximo duas tentativas por
// mensagem: a do provedor padrão e, se ela falhar ou o provedor não tiver
// remetente configurado, a do outro provedor.
type TentativaEnvioComunicacao struct {
	Provedor          string `json:"provedor"`
	Sucesso           bool   `json:"sucesso"`
	MensagemExternaID string `json:"mensagem_externa_id,omitempty"`
	ErroCodigo        string `json:"erro_codigo,omitempty"`
	ErroMensagem      string `json:"erro_mensagem,omitempty"`
}

type MensagemComunicacao struct {
	BaseAggregate
	Destinatario       string
	Conteudo           string
	ProvedorTentado1   string
	ProvedorTentado2   string
	ProvedorUtilizado  string
	Status             string
	MensagemExternaID  string
	DetalhesTentativas []TentativaEnvioComunicacao
	EnviadoPor         uuid.UUID
	EnviadoPorTipo     string
	CodigoAcademia     string
	CreatedAt          time.Time
}

func NewMensagemComunicacao() *MensagemComunicacao {
	return &MensagemComunicacao{BaseAggregate: BaseAggregate{ID: uuid.New(), Version: 0, UncommittedEvents: []DomainEvent{}}}
}
func (m *MensagemComunicacao) GetType() string { return "MensagemComunicacao" }

type MensagemComunicacaoRegistradaEvent struct {
	BaseEvent
	Destinatario       string
	Conteudo           string
	ProvedorTentado1   string
	ProvedorTentado2   string
	ProvedorUtilizado  string
	Status             string
	MensagemExternaID  string
	DetalhesTentativas []TentativaEnvioComunicacao
	EnviadoPor         uuid.UUID
	EnviadoPorTipo     string
	CodigoAcademia     string
	CreatedAt          time.Time
}

func (e *MensagemComunicacaoRegistradaEvent) GetPayload() interface{} { return e }
func (e *MensagemComunicacaoRegistradaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

func (m *MensagemComunicacao) Apply(event DomainEvent) error {
	switch event.GetEventType() {
	case "MensagemComunicacaoRegistrada":
		return m.applyRegistrada(event)
	default:
		return fmt.Errorf("tipo de evento desconhecido para MensagemComunicacao: %s", event.GetEventType())
	}
}

// Registrar grava o resultado final (sucesso ou falha, já com todas as
// tentativas de provedor esgotadas) de UM envio de mensagem. Este é o
// único evento deste agregado — MensagemComunicacao é criada uma única vez
// e nunca é alterada depois (registro de auditoria imutável). Toda a
// validação de destinatario/conteudo já deve ter acontecido ANTES de
// chamar Registrar (no handler, antes de gastar qualquer tentativa de
// envio real) — este método não valida, apenas registra o resultado.
func (m *MensagemComunicacao) Registrar(
	destinatario, conteudo string,
	provedorTentado1, provedorTentado2, provedorUtilizado string,
	status, mensagemExternaID string,
	detalhes []TentativaEnvioComunicacao,
	enviadoPor uuid.UUID,
	enviadoPorTipo, codigoAcademia string,
) error {
	e := &MensagemComunicacaoRegistradaEvent{
		BaseEvent:          BaseEvent{EventType: "MensagemComunicacaoRegistrada", AggregateID: m.ID},
		Destinatario:       destinatario,
		Conteudo:           conteudo,
		ProvedorTentado1:   provedorTentado1,
		ProvedorTentado2:   provedorTentado2,
		ProvedorUtilizado:  provedorUtilizado,
		Status:             status,
		MensagemExternaID:  mensagemExternaID,
		DetalhesTentativas: detalhes,
		EnviadoPor:         enviadoPor,
		EnviadoPorTipo:     enviadoPorTipo,
		CodigoAcademia:     codigoAcademia,
		CreatedAt:          time.Now(),
	}
	m.RaiseEvent(e)
	return m.Apply(e)
}

func (m *MensagemComunicacao) applyRegistrada(e DomainEvent) error {
	b, err := json.Marshal(e.GetPayload())
	if err != nil {
		return err
	}
	var p MensagemComunicacaoRegistradaEvent
	if err = json.Unmarshal(b, &p); err != nil {
		return err
	}
	m.Destinatario = p.Destinatario
	m.Conteudo = p.Conteudo
	m.ProvedorTentado1 = p.ProvedorTentado1
	m.ProvedorTentado2 = p.ProvedorTentado2
	m.ProvedorUtilizado = p.ProvedorUtilizado
	m.Status = p.Status
	m.MensagemExternaID = p.MensagemExternaID
	m.DetalhesTentativas = p.DetalhesTentativas
	m.EnviadoPor = p.EnviadoPor
	m.EnviadoPorTipo = p.EnviadoPorTipo
	m.CodigoAcademia = p.CodigoAcademia
	m.CreatedAt = p.CreatedAt
	return nil
}
