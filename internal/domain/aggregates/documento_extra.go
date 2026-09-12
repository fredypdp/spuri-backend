package aggregates

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"spuri/internal/utils"
)

// TiposDocumentoExtraPermitidos define os únicos valores aceitos para o
// campo Tipo de um DocumentoExtra — mesma ideia de conjunto fechado usada em
// TipoDetalhePersonalizado (ver Módulo de Serviços Extras).
var TiposDocumentoExtraPermitidos = map[string]bool{"pdf": true, "jpg": true}

// DocumentoExtra é a definição, mantida pela academia, de um documento
// adicional (além dos fixos do sistema: BI, cédula, declaração, certificados)
// exigido — ou apenas oferecido — no cadastro direto e na solicitação de
// matrícula do estudante, para um ano_academico específico.
//
// Cada estudante que efetivamente envia o arquivo correspondente tem esse
// envio registrado em projection_estudantes.documentos, na chave
// "documento_extra.<DocumentoExtra.ID>" — este aggregate guarda apenas a
// DEFINIÇÃO (rótulo, tipo exigido, obrigatoriedade, escopo), não os arquivos.
type DocumentoExtra struct {
	BaseAggregate

	CodigoAcademia string
	Rotulo         string
	Tipo           string // "pdf" | "jpg"
	Obrigatorio    bool
	Nivel          string // fundamental | medio | superior — derivado de AnoAcademico, nunca aceito diretamente do payload
	AnoAcademico   string // ex.: "6_ano_fundamental", "2_ano_medio", "1_ano_superior"
	Ativo          bool
	CriadoPor      uuid.UUID
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func NewDocumentoExtra() *DocumentoExtra {
	return &DocumentoExtra{BaseAggregate: BaseAggregate{ID: uuid.New(), Version: 0, UncommittedEvents: []DomainEvent{}}, Ativo: true}
}

func (d *DocumentoExtra) GetType() string { return "DocumentoExtra" }

// ============================================================================
// Eventos
// ============================================================================

type DocumentoExtraCriadoEvent struct {
	BaseEvent
	CodigoAcademia string
	Rotulo         string
	Tipo           string
	Obrigatorio    bool
	Nivel          string
	AnoAcademico   string
	CriadoPor      uuid.UUID
	CreatedAt      time.Time
}

func (e *DocumentoExtraCriadoEvent) GetPayload() interface{} { return e }
func (e *DocumentoExtraCriadoEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type DocumentoExtraAtualizadoEvent struct {
	BaseEvent
	Rotulo        string
	Tipo          string
	Obrigatorio   bool
	Nivel         string
	AnoAcademico  string
	AtualizadoPor uuid.UUID
	UpdatedAt     time.Time
}

func (e *DocumentoExtraAtualizadoEvent) GetPayload() interface{} { return e }
func (e *DocumentoExtraAtualizadoEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type DocumentoExtraDesativadoEvent struct {
	BaseEvent
	DesativadoPor uuid.UUID
	UpdatedAt     time.Time
}

func (e *DocumentoExtraDesativadoEvent) GetPayload() interface{} { return e }
func (e *DocumentoExtraDesativadoEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type DocumentoExtraReativadoEvent struct {
	BaseEvent
	ReativadoPor uuid.UUID
	UpdatedAt    time.Time
}

func (e *DocumentoExtraReativadoEvent) GetPayload() interface{} { return e }
func (e *DocumentoExtraReativadoEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

// ============================================================================
// Apply
// ============================================================================

func (d *DocumentoExtra) Apply(event DomainEvent) error {
	switch event.GetEventType() {
	case "DocumentoExtraCriado":
		return d.applyCriado(event)
	case "DocumentoExtraAtualizado":
		return d.applyAtualizado(event)
	case "DocumentoExtraDesativado":
		d.Ativo = false
		return nil
	case "DocumentoExtraReativado":
		d.Ativo = true
		return nil
	default:
		return fmt.Errorf("tipo de evento desconhecido para DocumentoExtra: %s", event.GetEventType())
	}
}

// ============================================================================
// Validação interna compartilhada
// ============================================================================

// validarCamposDocumentoExtra normaliza e valida rotulo/tipo/ano_academico,
// derivando nivel a partir de ano_academico via NivelDoAnoAcademico — mesma
// função já usada para os documentos fixos de matrícula (solicitacao_matricula.go),
// garantindo que "nivel" nunca seja uma entrada independente e divergente do
// ano informado.
func validarCamposDocumentoExtra(rotulo, tipo, anoAcademico string) (string, string, string, string, error) {
	rotulo = strings.TrimSpace(rotulo)
	if rotulo == "" {
		return "", "", "", "", fmt.Errorf("rotulo é obrigatório")
	}
	if len(rotulo) > 150 {
		return "", "", "", "", fmt.Errorf("rotulo deve ter no máximo 150 caracteres")
	}
	tipo = strings.TrimSpace(strings.ToLower(tipo))
	if !TiposDocumentoExtraPermitidos[tipo] {
		return "", "", "", "", fmt.Errorf("tipo deve ser 'pdf' ou 'jpg'")
	}
	anoAcademico = strings.TrimSpace(anoAcademico)
	if anoAcademico == "" {
		return "", "", "", "", fmt.Errorf("ano_academico é obrigatório")
	}
	var err error
	switch {
	case strings.HasSuffix(anoAcademico, "_ano_fundamental"):
		err = utils.ValidateAnoFundamental(anoAcademico)
	case strings.HasSuffix(anoAcademico, "_ano_medio"):
		err = utils.ValidateAnoMedio(anoAcademico)
	case strings.HasSuffix(anoAcademico, "_ano_superior"):
		err = utils.ValidateAnoSuperior(anoAcademico)
	default:
		err = fmt.Errorf("use o formato N_ano_fundamental, N_ano_medio ou N_ano_superior")
	}
	if err != nil {
		return "", "", "", "", fmt.Errorf("ano_academico inválido: %w", err)
	}
	nivel := NivelDoAnoAcademico(anoAcademico)
	return rotulo, tipo, nivel, anoAcademico, nil
}

// ============================================================================
// Comandos
// ============================================================================

func (d *DocumentoExtra) Criar(codigoAcademia, rotulo, tipo string, obrigatorio bool, anoAcademico string, criadoPor uuid.UUID) error {
	if strings.TrimSpace(codigoAcademia) == "" {
		return fmt.Errorf("codigo_academia é obrigatório")
	}
	rotulo, tipo, nivel, anoAcademico, err := validarCamposDocumentoExtra(rotulo, tipo, anoAcademico)
	if err != nil {
		return err
	}
	e := &DocumentoExtraCriadoEvent{
		BaseEvent:      BaseEvent{EventType: "DocumentoExtraCriado", AggregateID: d.ID},
		CodigoAcademia: codigoAcademia,
		Rotulo:         rotulo,
		Tipo:           tipo,
		Obrigatorio:    obrigatorio,
		Nivel:          nivel,
		AnoAcademico:   anoAcademico,
		CriadoPor:      criadoPor,
		CreatedAt:      time.Now(),
	}
	d.RaiseEvent(e)
	return d.Apply(e)
}

// Atualizar edita rotulo/tipo/obrigatorio/ano_academico de uma definição já
// existente. A mudança vale apenas prospectivamente: documentos já enviados
// por estudantes mantêm o path/tipo com que foram gravados — trocar o tipo
// aqui não invalida nem re-exige arquivos antigos, apenas passa a valer para
// os próximos cadastros/matrículas.
func (d *DocumentoExtra) Atualizar(rotulo, tipo string, obrigatorio bool, anoAcademico string, atualizadoPor uuid.UUID) error {
	rotulo, tipo, nivel, anoAcademico, err := validarCamposDocumentoExtra(rotulo, tipo, anoAcademico)
	if err != nil {
		return err
	}
	e := &DocumentoExtraAtualizadoEvent{
		BaseEvent:     BaseEvent{EventType: "DocumentoExtraAtualizado", AggregateID: d.ID},
		Rotulo:        rotulo,
		Tipo:          tipo,
		Obrigatorio:   obrigatorio,
		Nivel:         nivel,
		AnoAcademico:  anoAcademico,
		AtualizadoPor: atualizadoPor,
		UpdatedAt:     time.Now(),
	}
	d.RaiseEvent(e)
	return d.Apply(e)
}

// Desativar é uma remoção lógica: preserva o histórico (definição continua
// existindo e continua referenciada por documentos já enviados), apenas para
// de ser exigida/oferecida em novos cadastros e matrículas a partir de agora.
func (d *DocumentoExtra) Desativar(p uuid.UUID) error {
	if !d.Ativo {
		return fmt.Errorf("documento extra já está inativo")
	}
	e := &DocumentoExtraDesativadoEvent{BaseEvent: BaseEvent{EventType: "DocumentoExtraDesativado", AggregateID: d.ID}, DesativadoPor: p, UpdatedAt: time.Now()}
	d.RaiseEvent(e)
	return d.Apply(e)
}

func (d *DocumentoExtra) Reativar(p uuid.UUID) error {
	if d.Ativo {
		return fmt.Errorf("documento extra já está ativo")
	}
	e := &DocumentoExtraReativadoEvent{BaseEvent: BaseEvent{EventType: "DocumentoExtraReativado", AggregateID: d.ID}, ReativadoPor: p, UpdatedAt: time.Now()}
	d.RaiseEvent(e)
	return d.Apply(e)
}

// ============================================================================
// Apply handlers
// ============================================================================

func (d *DocumentoExtra) applyCriado(e DomainEvent) error {
	b, err := json.Marshal(e.GetPayload())
	if err != nil {
		return err
	}
	var p DocumentoExtraCriadoEvent
	if err = json.Unmarshal(b, &p); err != nil {
		return err
	}
	d.CodigoAcademia = p.CodigoAcademia
	d.Rotulo = p.Rotulo
	d.Tipo = p.Tipo
	d.Obrigatorio = p.Obrigatorio
	d.Nivel = p.Nivel
	d.AnoAcademico = p.AnoAcademico
	d.Ativo = true
	d.CriadoPor = p.CriadoPor
	d.CreatedAt = p.CreatedAt
	d.UpdatedAt = p.CreatedAt
	return nil
}

func (d *DocumentoExtra) applyAtualizado(e DomainEvent) error {
	b, err := json.Marshal(e.GetPayload())
	if err != nil {
		return err
	}
	var p DocumentoExtraAtualizadoEvent
	if err = json.Unmarshal(b, &p); err != nil {
		return err
	}
	d.Rotulo = p.Rotulo
	d.Tipo = p.Tipo
	d.Obrigatorio = p.Obrigatorio
	d.Nivel = p.Nivel
	d.AnoAcademico = p.AnoAcademico
	d.UpdatedAt = p.UpdatedAt
	return nil
}
