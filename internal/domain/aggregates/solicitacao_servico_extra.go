package aggregates

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	StatusInscricaoPendente                      = "pendente"
	StatusInscricaoAprovadaPendentePagamentoTaxa = "aprovada_pendente_pagamento_taxa_inscricao"
	StatusInscricaoVinculada                     = "vinculada"
	StatusInscricaoReprovada                     = "reprovada"
	StatusInscricaoCanceladaAntesDaVinculacao    = "cancelada_antes_da_vinculacao"
	StatusInscricaoCancelada                     = "cancelada"
)

// SolicitacaoServicoExtra representa, ao mesmo tempo, o pedido de inscrição
// de um estudante num ServicoExtra e — uma vez aprovado e (se aplicável)
// pago — o próprio vínculo ativo. Não existe uma entidade "Inscrição"
// separada: o mesmo registro muda de status. Ver decisão de design 1 no
// documento da Tarefa 10 para a justificativa (espelha SolicitacaoMatricula,
// que resolve o mesmo problema para matrícula).
type SolicitacaoServicoExtra struct {
	BaseAggregate

	ServicoExtraID  uuid.UUID
	CodigoAcademia  string
	CodigoEstudante string
	Status          string

	MotivoReprovacao   string
	MotivoCancelamento string
	CanceladaPor       string // "academia" | "estudante"

	DocumentoPath string
	DocumentoURL  string

	ValorTaxaInscricao            float64
	MetodosPagamentoTaxaInscricao []string

	AprovadaPor  uuid.UUID
	ReprovadaPor uuid.UUID

	CreatedAt time.Time
	UpdatedAt time.Time
}

func NewSolicitacaoServicoExtra() *SolicitacaoServicoExtra {
	return &SolicitacaoServicoExtra{
		BaseAggregate: BaseAggregate{ID: uuid.New(), Version: 0, UncommittedEvents: []DomainEvent{}},
	}
}

func (s *SolicitacaoServicoExtra) GetType() string { return "SolicitacaoServicoExtra" }

// ---------------------------------------------------------------------------
// Eventos
// ---------------------------------------------------------------------------

type SolicitacaoServicoExtraCriadaEvent struct {
	BaseEvent
	ServicoExtraID  uuid.UUID
	CodigoAcademia  string
	CodigoEstudante string
	DocumentoPath   string
	DocumentoURL    string
	CreatedAt       time.Time
}

func (e *SolicitacaoServicoExtraCriadaEvent) GetPayload() interface{} { return e }
func (e *SolicitacaoServicoExtraCriadaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type SolicitacaoServicoExtraAprovadaPendentePagamentoEvent struct {
	BaseEvent
	ValorTaxaInscricao            float64
	MetodosPagamentoTaxaInscricao []string
	AprovadaPor                   uuid.UUID
	UpdatedAt                     time.Time
}

func (e *SolicitacaoServicoExtraAprovadaPendentePagamentoEvent) GetPayload() interface{} { return e }
func (e *SolicitacaoServicoExtraAprovadaPendentePagamentoEvent) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}

// SolicitacaoServicoExtraVinculadaEvent é o evento terminal de vínculo,
// alcançado tanto pelo caminho sem taxa (a partir de "pendente", com
// AprovadaPor preenchido) quanto pelo caminho com taxa paga (a partir de
// "aprovada_pendente_pagamento_taxa_inscricao", com AprovadaPor
// uuid.Nil — quem vincula é a confirmação de pagamento, não uma pessoa).
type SolicitacaoServicoExtraVinculadaEvent struct {
	BaseEvent
	AprovadaPor uuid.UUID
	UpdatedAt   time.Time
}

func (e *SolicitacaoServicoExtraVinculadaEvent) GetPayload() interface{} { return e }
func (e *SolicitacaoServicoExtraVinculadaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type SolicitacaoServicoExtraReprovadaEvent struct {
	BaseEvent
	MotivoReprovacao string
	ReprovadaPor     uuid.UUID
	UpdatedAt        time.Time
}

func (e *SolicitacaoServicoExtraReprovadaEvent) GetPayload() interface{} { return e }
func (e *SolicitacaoServicoExtraReprovadaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type SolicitacaoServicoExtraCanceladaAntesDaVinculacaoEvent struct {
	BaseEvent
	MotivoCancelamento string
	CanceladaPor       string
	UpdatedAt          time.Time
}

func (e *SolicitacaoServicoExtraCanceladaAntesDaVinculacaoEvent) GetPayload() interface{} {
	return e
}
func (e *SolicitacaoServicoExtraCanceladaAntesDaVinculacaoEvent) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}

type SolicitacaoServicoExtraCanceladaEvent struct {
	BaseEvent
	MotivoCancelamento string
	CanceladaPor       string
	UpdatedAt          time.Time
}

func (e *SolicitacaoServicoExtraCanceladaEvent) GetPayload() interface{} { return e }
func (e *SolicitacaoServicoExtraCanceladaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

// ---------------------------------------------------------------------------
// Apply
// ---------------------------------------------------------------------------

func (s *SolicitacaoServicoExtra) Apply(event DomainEvent) error {
	switch event.GetEventType() {
	case "SolicitacaoServicoExtraCriada":
		return s.applyCriada(event)
	case "SolicitacaoServicoExtraAprovadaPendentePagamento":
		return s.applyAprovadaPendentePagamento(event)
	case "SolicitacaoServicoExtraVinculada":
		return s.applyVinculada(event)
	case "SolicitacaoServicoExtraReprovada":
		return s.applyReprovada(event)
	case "SolicitacaoServicoExtraCanceladaAntesDaVinculacao":
		return s.applyCanceladaAntesDaVinculacao(event)
	case "SolicitacaoServicoExtraCancelada":
		return s.applyCancelada(event)
	default:
		return fmt.Errorf("tipo de evento desconhecido para SolicitacaoServicoExtra: %s", event.GetEventType())
	}
}

// ---------------------------------------------------------------------------
// Commands
// ---------------------------------------------------------------------------

func (s *SolicitacaoServicoExtra) Criar(servicoExtraID uuid.UUID, codigoAcademia, codigoEstudante, documentoPath, documentoURL string) error {
	if servicoExtraID == uuid.Nil {
		return fmt.Errorf("servico_extra_id é obrigatório")
	}
	if strings.TrimSpace(codigoAcademia) == "" || strings.TrimSpace(codigoEstudante) == "" {
		return fmt.Errorf("codigo_academia e codigo_estudante são obrigatórios")
	}
	event := &SolicitacaoServicoExtraCriadaEvent{
		BaseEvent:       BaseEvent{EventType: "SolicitacaoServicoExtraCriada", AggregateID: s.ID},
		ServicoExtraID:  servicoExtraID,
		CodigoAcademia:  codigoAcademia,
		CodigoEstudante: codigoEstudante,
		DocumentoPath:   documentoPath,
		DocumentoURL:    documentoURL,
		CreatedAt:       time.Now(),
	}
	s.RaiseEvent(event)
	return s.Apply(event)
}

// Aprovar aprova uma solicitação pendente. Quando o serviço não tem taxa de
// inscrição, vincula imediatamente. Quando tem, congela valor/métodos da
// taxa NO MOMENTO DA APROVAÇÃO (mudanças posteriores no ServicoExtra não
// alteram o que este estudante especificamente tem a pagar — mesmo
// princípio já usado em SolicitacaoMatricula.MarcarPendentePagamentoMatricula)
// e aguarda pagamento.
func (s *SolicitacaoServicoExtra) Aprovar(temTaxaInscricao bool, valorTaxa float64, metodosTaxa []string, aprovadaPor uuid.UUID) error {
	if s.Status != StatusInscricaoPendente {
		return fmt.Errorf("apenas solicitações pendentes podem ser aprovadas (status atual: %s)", s.Status)
	}
	if !temTaxaInscricao {
		event := &SolicitacaoServicoExtraVinculadaEvent{
			BaseEvent:   BaseEvent{EventType: "SolicitacaoServicoExtraVinculada", AggregateID: s.ID},
			AprovadaPor: aprovadaPor,
			UpdatedAt:   time.Now(),
		}
		s.RaiseEvent(event)
		return s.Apply(event)
	}
	if valorTaxa <= 0 || len(metodosTaxa) == 0 {
		return fmt.Errorf("valor e métodos de pagamento da taxa de inscrição são obrigatórios quando o serviço exige taxa")
	}
	event := &SolicitacaoServicoExtraAprovadaPendentePagamentoEvent{
		BaseEvent:                     BaseEvent{EventType: "SolicitacaoServicoExtraAprovadaPendentePagamento", AggregateID: s.ID},
		ValorTaxaInscricao:            valorTaxa,
		MetodosPagamentoTaxaInscricao: metodosTaxa,
		AprovadaPor:                   aprovadaPor,
		UpdatedAt:                     time.Now(),
	}
	s.RaiseEvent(event)
	return s.Apply(event)
}

func (s *SolicitacaoServicoExtra) Reprovar(motivo string, reprovadaPor uuid.UUID) error {
	if s.Status != StatusInscricaoPendente {
		return fmt.Errorf("apenas solicitações pendentes podem ser reprovadas (status atual: %s)", s.Status)
	}
	if strings.TrimSpace(motivo) == "" {
		return fmt.Errorf("motivo_reprovacao é obrigatório")
	}
	event := &SolicitacaoServicoExtraReprovadaEvent{
		BaseEvent:        BaseEvent{EventType: "SolicitacaoServicoExtraReprovada", AggregateID: s.ID},
		MotivoReprovacao: motivo,
		ReprovadaPor:     reprovadaPor,
		UpdatedAt:        time.Now(),
	}
	s.RaiseEvent(event)
	return s.Apply(event)
}

// VincularAposPagamento efetiva o vínculo depois de o pagamento da taxa de
// inscrição ser confirmado pelo módulo financeiro (chamado a partir dos três
// pontos de confirmação descritos na seção 6.5: resposta síncrona, consulta
// e webhook — exatamente como já acontece para matrícula).
func (s *SolicitacaoServicoExtra) VincularAposPagamento() error {
	if s.Status != StatusInscricaoAprovadaPendentePagamentoTaxa {
		return fmt.Errorf("solicitação não está aguardando pagamento de taxa de inscrição (status atual: %s)", s.Status)
	}
	event := &SolicitacaoServicoExtraVinculadaEvent{
		BaseEvent: BaseEvent{EventType: "SolicitacaoServicoExtraVinculada", AggregateID: s.ID},
		UpdatedAt: time.Now(),
	}
	s.RaiseEvent(event)
	return s.Apply(event)
}

// CancelarAntesDaVinculacao desiste de uma solicitação já aprovada mas ainda
// aguardando pagamento da taxa — nunca chegou a vincular.
func (s *SolicitacaoServicoExtra) CancelarAntesDaVinculacao(motivo, canceladaPor string) error {
	if s.Status != StatusInscricaoAprovadaPendentePagamentoTaxa {
		return fmt.Errorf("apenas solicitações aguardando pagamento de taxa podem ser canceladas neste estágio (status atual: %s)", s.Status)
	}
	if canceladaPor != "academia" && canceladaPor != "estudante" {
		return fmt.Errorf("cancelada_por deve ser 'academia' ou 'estudante'")
	}
	event := &SolicitacaoServicoExtraCanceladaAntesDaVinculacaoEvent{
		BaseEvent:          BaseEvent{EventType: "SolicitacaoServicoExtraCanceladaAntesDaVinculacao", AggregateID: s.ID},
		MotivoCancelamento: motivo,
		CanceladaPor:       canceladaPor,
		UpdatedAt:          time.Now(),
	}
	s.RaiseEvent(event)
	return s.Apply(event)
}

// Cancelar cancela uma inscrição já vinculada/ativa — "cancelar inscrição de
// um estudante" do pedido original, mais a extensão de autocancelamento
// (decisão de design 4).
func (s *SolicitacaoServicoExtra) Cancelar(motivo, canceladaPor string) error {
	if s.Status != StatusInscricaoVinculada {
		return fmt.Errorf("apenas inscrições vinculadas podem ser canceladas (status atual: %s)", s.Status)
	}
	if canceladaPor != "academia" && canceladaPor != "estudante" {
		return fmt.Errorf("cancelada_por deve ser 'academia' ou 'estudante'")
	}
	event := &SolicitacaoServicoExtraCanceladaEvent{
		BaseEvent:          BaseEvent{EventType: "SolicitacaoServicoExtraCancelada", AggregateID: s.ID},
		MotivoCancelamento: motivo,
		CanceladaPor:       canceladaPor,
		UpdatedAt:          time.Now(),
	}
	s.RaiseEvent(event)
	return s.Apply(event)
}

// ---------------------------------------------------------------------------
// Apply handlers
// ---------------------------------------------------------------------------

func (s *SolicitacaoServicoExtra) applyCriada(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return err
	}
	var p SolicitacaoServicoExtraCriadaEvent
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	s.ServicoExtraID = p.ServicoExtraID
	s.CodigoAcademia = p.CodigoAcademia
	s.CodigoEstudante = p.CodigoEstudante
	s.DocumentoPath = p.DocumentoPath
	s.DocumentoURL = p.DocumentoURL
	s.Status = StatusInscricaoPendente
	s.CreatedAt = p.CreatedAt
	s.UpdatedAt = p.CreatedAt
	return nil
}

func (s *SolicitacaoServicoExtra) applyAprovadaPendentePagamento(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return err
	}
	var p SolicitacaoServicoExtraAprovadaPendentePagamentoEvent
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	s.Status = StatusInscricaoAprovadaPendentePagamentoTaxa
	s.ValorTaxaInscricao = p.ValorTaxaInscricao
	s.MetodosPagamentoTaxaInscricao = p.MetodosPagamentoTaxaInscricao
	s.AprovadaPor = p.AprovadaPor
	s.UpdatedAt = p.UpdatedAt
	return nil
}

func (s *SolicitacaoServicoExtra) applyVinculada(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return err
	}
	var p SolicitacaoServicoExtraVinculadaEvent
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	s.Status = StatusInscricaoVinculada
	if p.AprovadaPor != uuid.Nil {
		s.AprovadaPor = p.AprovadaPor
	}
	s.UpdatedAt = p.UpdatedAt
	return nil
}

func (s *SolicitacaoServicoExtra) applyReprovada(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return err
	}
	var p SolicitacaoServicoExtraReprovadaEvent
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	s.Status = StatusInscricaoReprovada
	s.MotivoReprovacao = p.MotivoReprovacao
	s.ReprovadaPor = p.ReprovadaPor
	s.UpdatedAt = p.UpdatedAt
	return nil
}

func (s *SolicitacaoServicoExtra) applyCanceladaAntesDaVinculacao(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return err
	}
	var p SolicitacaoServicoExtraCanceladaAntesDaVinculacaoEvent
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	s.Status = StatusInscricaoCanceladaAntesDaVinculacao
	s.MotivoCancelamento = p.MotivoCancelamento
	s.CanceladaPor = p.CanceladaPor
	s.UpdatedAt = p.UpdatedAt
	return nil
}

func (s *SolicitacaoServicoExtra) applyCancelada(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return err
	}
	var p SolicitacaoServicoExtraCanceladaEvent
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	s.Status = StatusInscricaoCancelada
	s.MotivoCancelamento = p.MotivoCancelamento
	s.CanceladaPor = p.CanceladaPor
	s.UpdatedAt = p.UpdatedAt
	return nil
}
