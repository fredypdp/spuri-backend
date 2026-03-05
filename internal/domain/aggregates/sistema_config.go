package aggregates

import (
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/google/uuid"
)

// reAnoLetivo valida o formato YYYY_YYYY (ex: 2025_2026)
var reAnoLetivo = regexp.MustCompile(`^\d{4}_\d{4}$`)

// ============================================================================
// Aggregate
// ============================================================================

type SistemaConfig struct {
	BaseAggregate

	Chave     string
	Valor     string
	UpdatedBy uuid.UUID
	UpdatedAt time.Time
}

func NewSistemaConfigComID(id uuid.UUID) *SistemaConfig {
	return &SistemaConfig{
		BaseAggregate: BaseAggregate{
			ID:                id,
			Version:           0,
			UncommittedEvents: []DomainEvent{},
		},
	}
}

func (s *SistemaConfig) GetType() string { return "SistemaConfig" }

func (s *SistemaConfig) Apply(event DomainEvent) error {
	switch event.GetEventType() {
	case "AnoLetivoDefinido":
		return s.applyAnoLetivoDefinido(event)
	default:
		return fmt.Errorf("tipo de evento desconhecido: %s", event.GetEventType())
	}
}

// ============================================================================
// Comandos
// ============================================================================

// DefinirAnoLetivo é retrocompatível: cria o evento sem datas e observação.
// Para incluir datas, use DefinirAnoLetivoCompleto.
func (s *SistemaConfig) DefinirAnoLetivo(valor string, adminID uuid.UUID) error {
	return s.definirAnoLetivoInterno(valor, adminID, nil, nil, nil)
}

// DefinirAnoLetivoCompleto cria um evento AnoLetivoDefinido com datas e observação.
// Etapa 4 deve migrar os handlers para usar este método.
func (s *SistemaConfig) DefinirAnoLetivoCompleto(
	valor string,
	adminID uuid.UUID,
	dataInicio *time.Time,
	dataFim *time.Time,
	observacao *string,
) error {
	return s.definirAnoLetivoInterno(valor, adminID, dataInicio, dataFim, observacao)
}

func (s *SistemaConfig) definirAnoLetivoInterno(
	valor string,
	adminID uuid.UUID,
	dataInicio *time.Time,
	dataFim *time.Time,
	observacao *string,
) error {
	if !reAnoLetivo.MatchString(valor) {
		return fmt.Errorf("formato inválido: use YYYY_YYYY (ex: 2025_2026)")
	}

	var anoInicio, anoFim int
	fmt.Sscanf(valor[:4], "%d", &anoInicio)
	fmt.Sscanf(valor[5:], "%d", &anoFim)
	if anoFim != anoInicio+1 {
		return fmt.Errorf("ano letivo deve ser de um ano para o seguinte (ex: 2025_2026)")
	}

	event := &AnoLetivoDefinidoEvent{
		BaseEvent:   BaseEvent{EventType: "AnoLetivoDefinido", AggregateID: s.ID},
		Chave:       "ano_letivo_atual",
		Valor:       valor,
		DataInicio:  dataInicio,
		DataFim:     dataFim,
		Observacao:  observacao,
		DefinidoPor: adminID,
		DefinidoEm:  time.Now(),
	}

	s.RaiseEvent(event)
	return s.Apply(event)
}

// ============================================================================
// Apply handlers
// ============================================================================

// applyAnoLetivoDefinido — FIX #8: propaga erro de json.Marshal.
func (s *SistemaConfig) applyAnoLetivoDefinido(event DomainEvent) error {
	payload := event.GetPayload()
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("applyAnoLetivoDefinido: marshal error: %w", err)
	}

	var ev AnoLetivoDefinidoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyAnoLetivoDefinido: unmarshal error: %w", err)
	}

	s.ID = event.GetAggregateID()
	s.Chave = ev.Chave
	s.Valor = ev.Valor
	s.UpdatedBy = ev.DefinidoPor
	s.UpdatedAt = ev.DefinidoEm
	return nil
}

// ============================================================================
// Eventos
// ============================================================================

// AnoLetivoDefinidoEvent — FIX #9: campos DataInicio, DataFim e Observacao
// adicionados como ponteiros (nil-safe para eventos já gravados sem estes campos).
// O campo Valor contém a string do ano letivo (ex: "2025_2026").
// DataInicio e DataFim contêm as datas de início e fim do ano letivo quando fornecidas.
type AnoLetivoDefinidoEvent struct {
	BaseEvent
	Chave       string
	Valor       string     // ex: "2025_2026"
	DataInicio  *time.Time // nil quando não informado (eventos legacy)
	DataFim     *time.Time // nil quando não informado (eventos legacy)
	Observacao  *string    // nil quando não informado
	DefinidoPor uuid.UUID
	DefinidoEm  time.Time
}

func (e *AnoLetivoDefinidoEvent) GetPayload() interface{} { return e }
func (e *AnoLetivoDefinidoEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

// EmailVerificadoEvent é compartilhado entre Admin e Academia.
// Definido aqui por ser o primeiro aggregate que o utiliza na hierarquia.
type EmailVerificadoEvent struct {
	BaseEvent
	VerifiedAt time.Time
}

func (e *EmailVerificadoEvent) GetPayload() interface{} { return e }
func (e *EmailVerificadoEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }