package aggregates

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"spuri/internal/utils"

	"github.com/google/uuid"
)

// SolicitacaoAlteracaoNIFAcademia é o aggregate de event sourcing que
// representa o fluxo de aprovação para alteração de NIF de uma academia.
//
// Espelha deliberadamente o padrão já usado por SolicitacaoEdicaoDadoEstudante
// (internal/domain/aggregates/solicitacao_edicao_dado_estudante.go): Criar
// grava o pedido no ledger sem tocar no dado real da Academia; só Aprovar
// dispara (no handler) a alteração efetiva via Academia.AlterarNIFPorSolicitacao.
// Reprovar apenas encerra a solicitação — nenhum dado da Academia muda.
//
// Diferença deliberada em relação a SolicitacaoEdicaoDadoEstudante: esta
// solicitação não exige documento comprobatório (fora de escopo da tarefa) e
// é decidida por um Admin (role "adm" ou "fpp"), não pela academia.
type SolicitacaoAlteracaoNIFAcademia struct {
	*BaseAggregate

	CodigoSolicitacao string
	CodigoAcademia    string
	NIFAtual          string
	NIFSolicitado     string
	Status            string
	MotivoReprovacao  string
	SolicitadoPor     string
	DecididoPor       string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func NewSolicitacaoAlteracaoNIFAcademia() *SolicitacaoAlteracaoNIFAcademia {
	return &SolicitacaoAlteracaoNIFAcademia{
		BaseAggregate: &BaseAggregate{ID: uuid.New(), UncommittedEvents: []DomainEvent{}},
		Status:        StatusSolicitacaoPendente,
	}
}

func (s *SolicitacaoAlteracaoNIFAcademia) GetType() string { return "SolicitacaoAlteracaoNIFAcademia" }

// Criar registra o pedido de alteração de NIF. nifAtual é o NIF vigente da
// academia no momento do pedido (capturado pelo handler a partir da
// projeção); nifSolicitado é o novo valor pretendido. Nenhum dado da
// Academia é alterado aqui — apenas o pedido é gravado no ledger.
func (s *SolicitacaoAlteracaoNIFAcademia) Criar(codigo, codigoAcademia, nifAtual, nifSolicitado, solicitadoPor string) error {
	codigo = strings.TrimSpace(codigo)
	codigoAcademia = strings.TrimSpace(codigoAcademia)
	solicitadoPor = strings.TrimSpace(solicitadoPor)
	nifAtual = strings.TrimSpace(nifAtual)
	nifSolicitado = strings.TrimSpace(nifSolicitado)

	if codigo == "" || codigoAcademia == "" || solicitadoPor == "" {
		return fmt.Errorf("dados obrigatórios da solicitação inválidos")
	}
	if err := utils.ValidateNIF(nifSolicitado); err != nil {
		return err
	}
	if strings.EqualFold(nifAtual, nifSolicitado) {
		return fmt.Errorf("nif_solicitado deve ser diferente do nif atual")
	}

	now := time.Now()
	ev := &SolicitacaoAlteracaoNIFAcademiaCriadaEvent{
		BaseEvent:         BaseEvent{EventType: "SolicitacaoAlteracaoNIFAcademiaCriada", AggregateID: s.ID},
		CodigoSolicitacao: codigo,
		CodigoAcademia:    codigoAcademia,
		NIFAtual:          nifAtual,
		NIFSolicitado:     nifSolicitado,
		Status:            StatusSolicitacaoPendente,
		SolicitadoPor:     solicitadoPor,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	s.RaiseEvent(ev)
	return s.Apply(ev)
}

// Aprovar apenas marca a solicitação como aprovada. A alteração real do NIF
// na Academia é responsabilidade do handler, que deve chamar
// Academia.AlterarNIFPorSolicitacao ANTES ou DEPOIS de persistir este
// evento, dentro da mesma requisição — ver
// DecidirSolicitacaoAlteracaoNIFAcademiaHandler.
func (s *SolicitacaoAlteracaoNIFAcademia) Aprovar(decididoPor string) error {
	if s.Status != StatusSolicitacaoPendente {
		return fmt.Errorf("solicitação já decidida")
	}
	decididoPor = strings.TrimSpace(decididoPor)
	if decididoPor == "" {
		return fmt.Errorf("decidido_por é obrigatório")
	}
	now := time.Now()
	ev := &SolicitacaoAlteracaoNIFAcademiaAprovadaEvent{
		BaseEvent:         BaseEvent{EventType: "SolicitacaoAlteracaoNIFAcademiaAprovada", AggregateID: s.ID},
		CodigoSolicitacao: s.CodigoSolicitacao,
		NIFSolicitado:     s.NIFSolicitado,
		DecididoPor:       decididoPor,
		DecididoAt:        now,
		UpdatedAt:         now,
	}
	s.RaiseEvent(ev)
	return s.Apply(ev)
}

func (s *SolicitacaoAlteracaoNIFAcademia) Reprovar(decididoPor, motivo string) error {
	if s.Status != StatusSolicitacaoPendente {
		return fmt.Errorf("solicitação já decidida")
	}
	decididoPor = strings.TrimSpace(decididoPor)
	if decididoPor == "" {
		return fmt.Errorf("decidido_por é obrigatório")
	}
	motivo = strings.TrimSpace(motivo)
	if motivo == "" {
		return fmt.Errorf("motivo_reprovacao é obrigatório")
	}
	now := time.Now()
	ev := &SolicitacaoAlteracaoNIFAcademiaReprovadaEvent{
		BaseEvent:         BaseEvent{EventType: "SolicitacaoAlteracaoNIFAcademiaReprovada", AggregateID: s.ID},
		CodigoSolicitacao: s.CodigoSolicitacao,
		MotivoReprovacao:  motivo,
		DecididoPor:       decididoPor,
		DecididoAt:        now,
		UpdatedAt:         now,
	}
	s.RaiseEvent(ev)
	return s.Apply(ev)
}

func (s *SolicitacaoAlteracaoNIFAcademia) Apply(event DomainEvent) error {
	data, _ := json.Marshal(event.GetPayload())
	switch event.GetEventType() {
	case "SolicitacaoAlteracaoNIFAcademiaCriada":
		var ev SolicitacaoAlteracaoNIFAcademiaCriadaEvent
		if err := json.Unmarshal(data, &ev); err != nil {
			return err
		}
		s.CodigoSolicitacao = ev.CodigoSolicitacao
		s.CodigoAcademia = ev.CodigoAcademia
		s.NIFAtual = ev.NIFAtual
		s.NIFSolicitado = ev.NIFSolicitado
		s.Status = ev.Status
		s.SolicitadoPor = ev.SolicitadoPor
		s.CreatedAt = ev.CreatedAt
		s.UpdatedAt = ev.UpdatedAt
		return nil
	case "SolicitacaoAlteracaoNIFAcademiaAprovada":
		var ev SolicitacaoAlteracaoNIFAcademiaAprovadaEvent
		if err := json.Unmarshal(data, &ev); err != nil {
			return err
		}
		s.Status = StatusSolicitacaoAprovada
		s.DecididoPor = ev.DecididoPor
		s.UpdatedAt = ev.UpdatedAt
		return nil
	case "SolicitacaoAlteracaoNIFAcademiaReprovada":
		var ev SolicitacaoAlteracaoNIFAcademiaReprovadaEvent
		if err := json.Unmarshal(data, &ev); err != nil {
			return err
		}
		s.Status = StatusSolicitacaoReprovada
		s.MotivoReprovacao = ev.MotivoReprovacao
		s.DecididoPor = ev.DecididoPor
		s.UpdatedAt = ev.UpdatedAt
		return nil
	}
	return fmt.Errorf("tipo de evento desconhecido: %s", event.GetEventType())
}

// ============================================================================
// Eventos
// ============================================================================

type SolicitacaoAlteracaoNIFAcademiaCriadaEvent struct {
	BaseEvent
	CodigoSolicitacao, CodigoAcademia, NIFAtual, NIFSolicitado, Status, SolicitadoPor string
	CreatedAt, UpdatedAt                                                              time.Time
}

func (e *SolicitacaoAlteracaoNIFAcademiaCriadaEvent) GetPayload() interface{} { return e }
func (e *SolicitacaoAlteracaoNIFAcademiaCriadaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type SolicitacaoAlteracaoNIFAcademiaAprovadaEvent struct {
	BaseEvent
	CodigoSolicitacao, NIFSolicitado, DecididoPor string
	DecididoAt, UpdatedAt                         time.Time
}

func (e *SolicitacaoAlteracaoNIFAcademiaAprovadaEvent) GetPayload() interface{} { return e }
func (e *SolicitacaoAlteracaoNIFAcademiaAprovadaEvent) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}

type SolicitacaoAlteracaoNIFAcademiaReprovadaEvent struct {
	BaseEvent
	CodigoSolicitacao, MotivoReprovacao, DecididoPor string
	DecididoAt, UpdatedAt                            time.Time
}

func (e *SolicitacaoAlteracaoNIFAcademiaReprovadaEvent) GetPayload() interface{} { return e }
func (e *SolicitacaoAlteracaoNIFAcademiaReprovadaEvent) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}
