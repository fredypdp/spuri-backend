package aggregates

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	CampoEdicaoNome           = "nome"
	CampoEdicaoBI             = "bilhete_identidade"
	CampoEdicaoBIEncarregado  = "bilhete_identidade_encarregado"
	CampoEdicaoDataNascimento = "data_nascimento"
)

type SolicitacaoEdicaoDadoEstudante struct {
	*BaseAggregate
	CodigoSolicitacao       string
	CodigoEstudante         string
	CodigoAcademia          string
	Campo                   string
	ValorAtual              string
	ValorSolicitado         string
	DocumentoTemporarioPath string
	DocumentoTemporarioURL  string
	Status                  string
	MotivoReprovacao        string
	SolicitadoPor           string
	DecididoPor             string
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

func NewSolicitacaoEdicaoDadoEstudante() *SolicitacaoEdicaoDadoEstudante {
	return &SolicitacaoEdicaoDadoEstudante{BaseAggregate: &BaseAggregate{ID: uuid.New(), UncommittedEvents: []DomainEvent{}}, Status: StatusSolicitacaoPendente}
}
func (s *SolicitacaoEdicaoDadoEstudante) GetType() string { return "SolicitacaoEdicaoDadoEstudante" }

func CampoEdicaoSensivelValido(campo string) bool {
	switch campo {
	case CampoEdicaoNome, CampoEdicaoBI, CampoEdicaoBIEncarregado, CampoEdicaoDataNascimento:
		return true
	}
	return false
}

func (s *SolicitacaoEdicaoDadoEstudante) Criar(codigo, codigoEstudante, codigoAcademia, campo, atual, solicitado, path, url, solicitadoPor string) error {
	if strings.TrimSpace(codigo) == "" || strings.TrimSpace(codigoEstudante) == "" || strings.TrimSpace(codigoAcademia) == "" || !CampoEdicaoSensivelValido(campo) || strings.TrimSpace(path) == "" || strings.TrimSpace(solicitadoPor) == "" {
		return fmt.Errorf("dados obrigatórios da solicitação inválidos")
	}
	now := time.Now()
	ev := &SolicitacaoEdicaoDadoEstudanteCriadaEvent{BaseEvent: BaseEvent{EventType: "SolicitacaoEdicaoDadoEstudanteCriada", AggregateID: s.ID}, CodigoSolicitacao: codigo, CodigoEstudante: codigoEstudante, CodigoAcademia: codigoAcademia, Campo: campo, ValorAtual: atual, ValorSolicitado: solicitado, DocumentoTemporarioPath: path, DocumentoTemporarioURL: url, Status: StatusSolicitacaoPendente, SolicitadoPor: solicitadoPor, CreatedAt: now, UpdatedAt: now}
	s.RaiseEvent(ev)
	return s.Apply(ev)
}
func (s *SolicitacaoEdicaoDadoEstudante) Aprovar(decididoPor string) error {
	if s.Status != StatusSolicitacaoPendente {
		return fmt.Errorf("solicitação já decidida")
	}
	now := time.Now()
	ev := &SolicitacaoEdicaoDadoEstudanteAprovadaEvent{BaseEvent: BaseEvent{EventType: "SolicitacaoEdicaoDadoEstudanteAprovada", AggregateID: s.ID}, CodigoSolicitacao: s.CodigoSolicitacao, Campo: s.Campo, DecididoPor: decididoPor, DecididoAt: now, UpdatedAt: now}
	s.RaiseEvent(ev)
	return s.Apply(ev)
}
func (s *SolicitacaoEdicaoDadoEstudante) Reprovar(decididoPor, motivo string) error {
	if s.Status != StatusSolicitacaoPendente {
		return fmt.Errorf("solicitação já decidida")
	}
	if strings.TrimSpace(motivo) == "" {
		return fmt.Errorf("motivo_reprovacao é obrigatório")
	}
	now := time.Now()
	ev := &SolicitacaoEdicaoDadoEstudanteReprovadaEvent{BaseEvent: BaseEvent{EventType: "SolicitacaoEdicaoDadoEstudanteReprovada", AggregateID: s.ID}, CodigoSolicitacao: s.CodigoSolicitacao, Campo: s.Campo, MotivoReprovacao: strings.TrimSpace(motivo), DecididoPor: decididoPor, DecididoAt: now, UpdatedAt: now}
	s.RaiseEvent(ev)
	return s.Apply(ev)
}

func (s *SolicitacaoEdicaoDadoEstudante) Apply(event DomainEvent) error {
	data, _ := json.Marshal(event.GetPayload())
	switch event.GetEventType() {
	case "SolicitacaoEdicaoDadoEstudanteCriada":
		var ev SolicitacaoEdicaoDadoEstudanteCriadaEvent
		if err := json.Unmarshal(data, &ev); err != nil {
			return err
		}
		s.CodigoSolicitacao = ev.CodigoSolicitacao
		s.CodigoEstudante = ev.CodigoEstudante
		s.CodigoAcademia = ev.CodigoAcademia
		s.Campo = ev.Campo
		s.ValorAtual = ev.ValorAtual
		s.ValorSolicitado = ev.ValorSolicitado
		s.DocumentoTemporarioPath = ev.DocumentoTemporarioPath
		s.DocumentoTemporarioURL = ev.DocumentoTemporarioURL
		s.Status = ev.Status
		s.SolicitadoPor = ev.SolicitadoPor
		s.CreatedAt = ev.CreatedAt
		s.UpdatedAt = ev.UpdatedAt
		return nil
	case "SolicitacaoEdicaoDadoEstudanteAprovada":
		var ev SolicitacaoEdicaoDadoEstudanteAprovadaEvent
		_ = json.Unmarshal(data, &ev)
		s.Status = StatusSolicitacaoAprovada
		s.DecididoPor = ev.DecididoPor
		s.UpdatedAt = ev.UpdatedAt
		return nil
	case "SolicitacaoEdicaoDadoEstudanteReprovada":
		var ev SolicitacaoEdicaoDadoEstudanteReprovadaEvent
		_ = json.Unmarshal(data, &ev)
		s.Status = StatusSolicitacaoReprovada
		s.MotivoReprovacao = ev.MotivoReprovacao
		s.DecididoPor = ev.DecididoPor
		s.UpdatedAt = ev.UpdatedAt
		return nil
	}
	return fmt.Errorf("tipo de evento desconhecido: %s", event.GetEventType())
}

type SolicitacaoEdicaoDadoEstudanteCriadaEvent struct {
	BaseEvent
	CodigoSolicitacao, CodigoEstudante, CodigoAcademia, Campo, ValorAtual, ValorSolicitado, DocumentoTemporarioPath, DocumentoTemporarioURL, Status, SolicitadoPor string
	CreatedAt, UpdatedAt                                                                                                                                           time.Time
}

func (e *SolicitacaoEdicaoDadoEstudanteCriadaEvent) GetPayload() interface{} { return e }
func (e *SolicitacaoEdicaoDadoEstudanteCriadaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type SolicitacaoEdicaoDadoEstudanteAprovadaEvent struct {
	BaseEvent
	CodigoSolicitacao, Campo, DecididoPor string
	DecididoAt, UpdatedAt                 time.Time
}

func (e *SolicitacaoEdicaoDadoEstudanteAprovadaEvent) GetPayload() interface{} { return e }
func (e *SolicitacaoEdicaoDadoEstudanteAprovadaEvent) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}

type SolicitacaoEdicaoDadoEstudanteReprovadaEvent struct {
	BaseEvent
	CodigoSolicitacao, Campo, MotivoReprovacao, DecididoPor string
	DecididoAt, UpdatedAt                                   time.Time
}

func (e *SolicitacaoEdicaoDadoEstudanteReprovadaEvent) GetPayload() interface{} { return e }
func (e *SolicitacaoEdicaoDadoEstudanteReprovadaEvent) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}
