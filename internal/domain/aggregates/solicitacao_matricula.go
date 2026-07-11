package aggregates

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"spuri/internal/utils"

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
	TelefoneResponsavel          *string
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

func (s *SolicitacaoMatricula) Criar(codigoSolicitacao, codigoAcademia, nome, genero string, dataNascimento time.Time, email, telefone, telefoneResponsavel, bi, biResp, anoFund, anoMedio *string, cursoMedioID *uuid.UUID, anoSuperior *string, cursoSuperiorID *uuid.UUID, documentos map[string]DocumentoMatricula) error {
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
	if err := validarTelefonesMatricula(telefone, telefoneResponsavel, anoFund, anoMedio, anoSuperior); err != nil {
		return err
	}
	if documentos == nil || len(documentos) == 0 {
		return fmt.Errorf("documentos são obrigatórios")
	}
	if err := ValidarDocumentosMatricula(bi, biResp, anoFund, anoMedio, anoSuperior, documentos); err != nil {
		return err
	}
	now := time.Now().UTC()
	ev := &SolicitacaoMatriculaCriadaEvent{BaseEvent: BaseEvent{EventType: "SolicitacaoMatriculaCriada", AggregateID: s.ID}, CodigoSolicitacao: codigoSolicitacao, CodigoAcademia: codigoAcademia, Nome: nome, Genero: genero, DataNascimento: dataNascimento, Email: email, Telefone: telefone, TelefoneResponsavel: telefoneResponsavel, BilheteIdentidade: bi, BilheteIdentidadeResponsavel: biResp, AnoEscolarFundamental: anoFund, AnoEscolarMedio: anoMedio, CursoMedioID: cursoMedioID, AnoSuperior: anoSuperior, CursoSuperiorID: cursoSuperiorID, Status: StatusSolicitacaoPendente, Documentos: documentos, CreatedAt: now, UpdatedAt: now}
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

func validarTelefonesMatricula(telefone, telefoneResponsavel, anoEscolar, anoEscolarMedio, anoSuperior *string) error {
	escolar := !isNilOrBlank(anoEscolar) || !isNilOrBlank(anoEscolarMedio)
	superior := !isNilOrBlank(anoSuperior)
	if escolar && isNilOrBlank(telefoneResponsavel) {
		return fmt.Errorf("telefone_responsavel é obrigatório para estudante escolar")
	}
	if superior && isNilOrBlank(telefone) {
		return fmt.Errorf("telefone é obrigatório para estudante do ensino superior")
	}
	if !escolar && !superior && isNilOrBlank(telefone) && isNilOrBlank(telefoneResponsavel) {
		return fmt.Errorf("telefone ou telefone_responsavel deve ser informado")
	}
	if !isNilOrBlank(telefone) {
		if err := utils.ValidatePhone(*telefone); err != nil {
			return err
		}
	}
	if !isNilOrBlank(telefoneResponsavel) {
		if err := utils.ValidatePhone(*telefoneResponsavel); err != nil {
			return err
		}
	}
	if !isNilOrBlank(telefone) && !isNilOrBlank(telefoneResponsavel) && *telefone == *telefoneResponsavel {
		return fmt.Errorf("telefone e telefone_responsavel não podem ser iguais")
	}
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
	Path         string `json:"path"`
	FileURL      string `json:"file_url"`
	DownloadURL  string `json:"download_url"`
	AnoAcademico string `json:"ano_academico,omitempty"`
}

func (d DocumentoMatricula) TemReferenciaArquivo() bool {
	return strings.TrimSpace(d.Path) != "" || strings.TrimSpace(d.FileURL) != "" || strings.TrimSpace(d.DownloadURL) != ""
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

func ValidarDocumentosMatricula(bi, biResp *string, anoFund, anoMedio, anoSuperior *string, documentos map[string]DocumentoMatricula) error {
	if documentos == nil {
		documentos = map[string]DocumentoMatricula{}
	}
	if !isNilOrBlank(anoSuperior) {
		if isNilOrBlank(bi) {
			return fmt.Errorf("bilhete_identidade é obrigatório para estudante do ensino superior")
		}
		if doc, ok := documentos["bi_estudante"]; !ok || !doc.TemReferenciaArquivo() {
			return fmt.Errorf("bi_estudante é obrigatório para estudante do ensino superior")
		}
		return validarComprovativoAcademico(*anoSuperior, documentos)
	}
	if isNilOrBlank(anoFund) && isNilOrBlank(anoMedio) {
		return nil
	}
	if isNilOrBlank(biResp) {
		return fmt.Errorf("bilhete_identidade_responsavel é obrigatório para estudante escolar")
	}
	if doc, ok := documentos["bi_responsavel"]; !ok || !doc.TemReferenciaArquivo() {
		return fmt.Errorf("bi_responsavel é obrigatório para estudante escolar")
	}
	if !isNilOrBlank(bi) {
		if doc, ok := documentos["bi_estudante"]; !ok || !doc.TemReferenciaArquivo() {
			return fmt.Errorf("bi_estudante é obrigatório quando bilhete_identidade do estudante é informado")
		}
	} else if doc, ok := documentos["cedula_estudante"]; !ok || !doc.TemReferenciaArquivo() {
		return fmt.Errorf("cedula_estudante é obrigatória quando bilhete_identidade do estudante não é informado")
	}
	ano := ""
	if !isNilOrBlank(anoFund) {
		ano = *anoFund
	} else if !isNilOrBlank(anoMedio) {
		ano = *anoMedio
	}
	return validarComprovativoAcademico(ano, documentos)
}

func validarComprovativoAcademico(ano string, documentos map[string]DocumentoMatricula) error {
	ano = strings.TrimSpace(ano)
	if ano == "1_ano_fundamental" {
		return nil
	}
	esperado, ok := anoAcademicoAnterior(ano)
	if !ok {
		return nil
	}
	if requiredCert, requiredLabel := certificadoObrigatorioParaAno(ano); requiredCert != "" {
		if doc, ok := documentos[requiredCert]; ok && doc.TemReferenciaArquivo() {
			return nil
		}
		if err := validarDeclaracaoAnoAnterior(documentos, esperado); err == nil {
			return nil
		} else if hasDocumentoComArquivo(documentos, "declaracao") {
			return err
		}
		return fmt.Errorf("%s ou declaracao do ano académico anterior (%s) é obrigatório para %s", requiredLabel, esperado, ano)
	}
	return validarDeclaracaoAnoAnterior(documentos, esperado)
}

func hasDocumentoComArquivo(documentos map[string]DocumentoMatricula, campo string) bool {
	doc, ok := documentos[campo]
	return ok && doc.TemReferenciaArquivo()
}

func validarDeclaracaoAnoAnterior(documentos map[string]DocumentoMatricula, esperado string) error {
	doc, ok := documentos["declaracao"]
	if !ok || !doc.TemReferenciaArquivo() {
		return fmt.Errorf("declaracao do ano académico anterior (%s) é obrigatória", esperado)
	}
	anoDeclaracao := strings.TrimSpace(doc.AnoAcademico)
	if anoDeclaracao == "" {
		return fmt.Errorf("declaracao deve informar ano_academico esperado %s", esperado)
	}
	if anoDeclaracao != esperado {
		return fmt.Errorf("declaracao deve ser do ano académico anterior %s; recebido %s", esperado, anoDeclaracao)
	}
	return nil
}

func certificadoObrigatorioParaAno(ano string) (string, string) {
	switch ano {
	case "7_ano_fundamental":
		return "certificado_6_ano_fundamental", "certificado do 6.º ano fundamental"
	case "1_ano_medio":
		return "certificado_9_ano_fundamental", "certificado do 9.º ano fundamental"
	case "1_ano_superior":
		return "certificado_ensino_medio", "certificado do ensino médio"
	default:
		return "", ""
	}
}

func anoAcademicoAnterior(ano string) (string, bool) {
	ordem := []string{
		"1_ano_fundamental", "2_ano_fundamental", "3_ano_fundamental", "4_ano_fundamental", "5_ano_fundamental", "6_ano_fundamental", "7_ano_fundamental", "8_ano_fundamental", "9_ano_fundamental",
		"1_ano_medio", "2_ano_medio", "3_ano_medio",
		"1_ano_superior",
	}
	for i, atual := range ordem {
		if atual == ano {
			if i == 0 {
				return "", false
			}
			return ordem[i-1], true
		}
	}
	return "", false
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
	TelefoneResponsavel          *string
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
	s.TelefoneResponsavel = ev.TelefoneResponsavel
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
