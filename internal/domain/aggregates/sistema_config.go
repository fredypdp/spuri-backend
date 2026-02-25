// ============================================================================
// ARQUIVO: internal/domain/aggregates/sistema_config.go
// Agregado de configuração global do sistema (singleton por chave)
// ============================================================================

package aggregates

import (
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/google/uuid"
)

var reAnoLetivo = regexp.MustCompile(`^\d{4}_\d{4}$`)

// SistemaConfig representa uma configuração global do sistema.
// Usa um UUID determinístico derivado da chave para ser um singleton por chave.
type SistemaConfig struct {
	BaseAggregate

	Chave     string
	Valor     string
	UpdatedBy uuid.UUID
	UpdatedAt time.Time
}

func NewSistemaConfig() *SistemaConfig {
	return &SistemaConfig{
		BaseAggregate: BaseAggregate{
			ID:                uuid.New(),
			Version:           0,
			UncommittedEvents: []DomainEvent{},
		},
	}
}

// NewSistemaConfigComID cria o agregado com ID fixo (usado pelo handler para carregar/criar).
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

// DefinirAnoLetivo valida e gera o evento AnoLetivoDefinido.
func (s *SistemaConfig) DefinirAnoLetivo(valor string, adminID uuid.UUID) error {
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
		DefinidoPor: adminID,
		DefinidoEm:  time.Now(),
	}

	s.RaiseEvent(event)
	return s.Apply(event)
}

// --- Event Handlers ---

func (s *SistemaConfig) applyAnoLetivoDefinido(event DomainEvent) error {
	payload := event.GetPayload()
	data, _ := json.Marshal(payload)

	var ev AnoLetivoDefinidoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	s.ID = event.GetAggregateID()
	s.Chave = ev.Chave
	s.Valor = ev.Valor
	s.UpdatedBy = ev.DefinidoPor
	s.UpdatedAt = ev.DefinidoEm
	return nil
}

// --- Eventos ---

type AnoLetivoDefinidoEvent struct {
	BaseEvent
	Chave       string
	Valor       string
	DefinidoPor uuid.UUID
	DefinidoEm  time.Time
}

func (e *AnoLetivoDefinidoEvent) GetPayload() interface{} { return e }