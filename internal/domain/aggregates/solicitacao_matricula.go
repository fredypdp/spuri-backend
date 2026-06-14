package aggregates

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	StatusSolicitacaoPendente  = "pendente"
	StatusSolicitacaoAprovada  = "aprovada"
	StatusSolicitacaoReprovada = "reprovada"
)

type SolicitacaoMatricula struct {
	BaseAggregate
	CodigoSolicitacao            string
	CodigoAcademia               string
	Nome                         string
	Genero                       string
	DataNascimento               time.Time
	Email                        *string
	Telefone                     *string
	BilheteIdentidade            *string
	BilheteIdentidadeResponsavel *string
	AnoEscolarFundamental        *string
	AnoEscolarMedio              *string
	CursoMedioID                 *uuid.UUID
	AnoSuperior                  *string
	CursoSuperiorID              *uuid.UUID
	Status                       string
	MotivoReprovacao             *string
	Documentos                   map[string]DocumentoMatricula
	CodigoEstudanteGerado        *string
	AprovadaPor                  *uuid.UUID
	ReprovadaPor                 *uuid.UUID
	CreatedAt                    time.Time
	UpdatedAt                    time.Time
}

func NewSolicitacaoMatricula() *SolicitacaoMatricula {
	return &SolicitacaoMatricula{
		BaseAggregate: BaseAggregate{ID: uuid.New(), Version: 0, UncommittedEvents: []DomainEvent{}},
		Status:        StatusSolicitacaoPendente,
		Documentos:    map[string]DocumentoMatricula{},
	}
}

func (s *SolicitacaoMatricula) GetType() string { return "SolicitacaoMatricula" }

func (s *SolicitacaoMatricula) Apply(event DomainEvent) error {
	switch event.GetEventType() {
	case "SolicitacaoMatriculaCriada":
		return s.applyCriada(event)
	case "SolicitacaoMatriculaAprovada":
		return s.applyAprovada(event)
	case "SolicitacaoMatriculaReprovada":
		return s.applyReprovada(event)
	default:
		return fmt.Errorf("tipo de evento desconhecido: %s", event.GetEventType())
	}
}

func (s *SolicitacaoMatricula) Criar(codigoSolicitacao, codigoAcademia, nome, genero string, dataNascimento time.Time, email, telefone, bi, biResp, anoFund, anoMedio *string, cursoMedioID *uuid.UUID, anoSuperior *string, cursoSuperiorID *uuid.UUID, documentos map[string]DocumentoMatricula) error {
	if strings.TrimSpace(codigoSolicitacao) == "" || strings.TrimSpace(codigoAcademia) == "" || strings.TrimSpace(nome) == "" {
		return fmt.Errorf("codigo_solicitacao, codigo_academia e nome são obrigatórios")
	}
	if genero != "masculino" && genero != "feminino" {
		return fmt.Errorf("genero deve ser 'masculino' ou 'feminino'")
	}
	if !dataNascimento.Before(time.Now().UTC().Truncate(24 * time.Hour)) {
		return fmt.Errorf("data_nascimento deve ser anterior à data atual")
	}
	if isNilOrBlank(bi) && isNilOrBlank(biResp) {
		return fmt.Errorf("bilhete_identidade ou bilhete_identidade_responsavel é obrigatório")
	}
	if bilhetesSolicitacaoIguais(bi, biResp) {
		return fmt.Errorf("bilhete_identidade e bilhete_identidade_responsavel não podem ser iguais")
	}
	if documentos == nil || len(documentos) == 0 {
		return fmt.Errorf("documentos são obrigatórios")
	}
	if _, ok := documentos["bi_estudante"]; !ok {
		if _, okResp := documentos["bi_responsavel"]; !okResp {
			return fmt.Errorf("documento bi_estudante ou bi_responsavel é obrigatório")
		}
	}
	now := time.Now().UTC()
	ev := &SolicitacaoMatriculaCriadaEvent{BaseEvent: BaseEvent{EventType: "SolicitacaoMatriculaCriada", AggregateID: s.ID}, CodigoSolicitacao: codigoSolicitacao, CodigoAcademia: codigoAcademia, Nome: nome, Genero: genero, DataNascimento: dataNascimento, Email: email, Telefone: telefone, BilheteIdentidade: bi, BilheteIdentidadeResponsavel: biResp, AnoEscolarFundamental: anoFund, AnoEscolarMedio: anoMedio, CursoMedioID: cursoMedioID, AnoSuperior: anoSuperior, CursoSuperiorID: cursoSuperiorID, Status: StatusSolicitacaoPendente, Documentos: documentos, CreatedAt: now, UpdatedAt: now}
	s.RaiseEvent(ev)
	return nil
}

func (s *SolicitacaoMatricula) Aprovar(aprovadaPor uuid.UUID, codigoEstudanteGerado string) error {
	if s.Status != StatusSolicitacaoPendente {
		return fmt.Errorf("solicitação já foi aprovada ou reprovada")
	}
	if aprovadaPor == uuid.Nil || strings.TrimSpace(codigoEstudanteGerado) == "" {
		return fmt.Errorf("aprovada_por e codigo_estudante_gerado são obrigatórios")
	}
	ev := &SolicitacaoMatriculaAprovadaEvent{BaseEvent: BaseEvent{EventType: "SolicitacaoMatriculaAprovada", AggregateID: s.ID}, CodigoSolicitacao: s.CodigoSolicitacao, CodigoAcademia: s.CodigoAcademia, AprovadaPor: aprovadaPor, CodigoEstudanteGerado: codigoEstudanteGerado, ApprovedAt: time.Now().UTC()}
	s.RaiseEvent(ev)
	return nil
}

func (s *SolicitacaoMatricula) Reprovar(reprovadaPor uuid.UUID, motivo string) error {
	if s.Status != StatusSolicitacaoPendente {
		return fmt.Errorf("solicitação já foi aprovada ou reprovada")
	}
	motivo = strings.TrimSpace(motivo)
	if reprovadaPor == uuid.Nil || motivo == "" {
		return fmt.Errorf("motivo_reprovacao é obrigatório")
	}
	ev := &SolicitacaoMatriculaReprovadaEvent{BaseEvent: BaseEvent{EventType: "SolicitacaoMatriculaReprovada", AggregateID: s.ID}, CodigoSolicitacao: s.CodigoSolicitacao, CodigoAcademia: s.CodigoAcademia, ReprovadaPor: reprovadaPor, MotivoReprovacao: motivo, RejectedAt: time.Now().UTC()}
	s.RaiseEvent(ev)
	return nil
}

func isNilOrBlank(v *string) bool { return v == nil || strings.TrimSpace(*v) == "" }

func bilhetesSolicitacaoIguais(bi, biResp *string) bool {
	if isNilOrBlank(bi) || isNilOrBlank(biResp) {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(*bi), strings.TrimSpace(*biResp))
}

type DocumentoMatricula struct {
	Path        string `json:"path"`
	FileURL     string `json:"file_url"`
	DownloadURL string `json:"download_url"`
}

func (d *DocumentoMatricula) UnmarshalJSON(data []byte) error {
	var legacyPath string
	if err := json.Unmarshal(data, &legacyPath); err == nil {
		d.Path = legacyPath
		return nil
	}
	type alias DocumentoMatricula
	var v alias
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*d = DocumentoMatricula(v)
	return nil
}

type SolicitacaoMatriculaCriadaEvent struct {
	BaseEvent
	CodigoSolicitacao            string
	CodigoAcademia               string
	Nome                         string
	Genero                       string
	DataNascimento               time.Time
	Email                        *string
	Telefone                     *string
	BilheteIdentidade            *string
	BilheteIdentidadeResponsavel *string
	AnoEscolarFundamental        *string
	AnoEscolarMedio              *string
	CursoMedioID                 *uuid.UUID
	AnoSuperior                  *string
	CursoSuperiorID              *uuid.UUID
	Status                       string
	Documentos                   map[string]DocumentoMatricula
	CreatedAt                    time.Time
	UpdatedAt                    time.Time
}

func (e *SolicitacaoMatriculaCriadaEvent) GetPayload() interface{} { return e }
func (e *SolicitacaoMatriculaCriadaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type SolicitacaoMatriculaAprovadaEvent struct {
	BaseEvent
	CodigoSolicitacao     string
	CodigoAcademia        string
	AprovadaPor           uuid.UUID
	CodigoEstudanteGerado string
	ApprovedAt            time.Time
}

func (e *SolicitacaoMatriculaAprovadaEvent) GetPayload() interface{} { return e }
func (e *SolicitacaoMatriculaAprovadaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type SolicitacaoMatriculaReprovadaEvent struct {
	BaseEvent
	CodigoSolicitacao string
	CodigoAcademia    string
	ReprovadaPor      uuid.UUID
	MotivoReprovacao  string
	RejectedAt        time.Time
}

func (e *SolicitacaoMatriculaReprovadaEvent) GetPayload() interface{} { return e }
func (e *SolicitacaoMatriculaReprovadaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

func (s *SolicitacaoMatricula) applyCriada(event DomainEvent) error {
	data, _ := json.Marshal(event.GetPayload())
	var ev SolicitacaoMatriculaCriadaEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}
	s.CodigoSolicitacao = ev.CodigoSolicitacao
	s.CodigoAcademia = ev.CodigoAcademia
	s.Nome = ev.Nome
	s.Genero = ev.Genero
	s.DataNascimento = ev.DataNascimento
	s.Email = ev.Email
	s.Telefone = ev.Telefone
	s.BilheteIdentidade = ev.BilheteIdentidade
	s.BilheteIdentidadeResponsavel = ev.BilheteIdentidadeResponsavel
	s.AnoEscolarFundamental = ev.AnoEscolarFundamental
	s.AnoEscolarMedio = ev.AnoEscolarMedio
	s.CursoMedioID = ev.CursoMedioID
	s.AnoSuperior = ev.AnoSuperior
	s.CursoSuperiorID = ev.CursoSuperiorID
	s.Status = ev.Status
	s.Documentos = ev.Documentos
	s.CreatedAt = ev.CreatedAt
	s.UpdatedAt = ev.UpdatedAt
	return nil
}
func (s *SolicitacaoMatricula) applyAprovada(event DomainEvent) error {
	data, _ := json.Marshal(event.GetPayload())
	var ev SolicitacaoMatriculaAprovadaEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}
	s.Status = StatusSolicitacaoAprovada
	s.AprovadaPor = &ev.AprovadaPor
	s.CodigoEstudanteGerado = &ev.CodigoEstudanteGerado
	s.UpdatedAt = ev.ApprovedAt
	return nil
}
func (s *SolicitacaoMatricula) applyReprovada(event DomainEvent) error {
	data, _ := json.Marshal(event.GetPayload())
	var ev SolicitacaoMatriculaReprovadaEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}
	s.Status = StatusSolicitacaoReprovada
	s.ReprovadaPor = &ev.ReprovadaPor
	s.MotivoReprovacao = &ev.MotivoReprovacao
	s.UpdatedAt = ev.RejectedAt
	return nil
}
