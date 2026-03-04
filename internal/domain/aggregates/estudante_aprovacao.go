// ============================================================================
// ARQUIVO: internal/domain/aggregates/estudante_aprovacao.go
//
// CORREÇÕES APLICADAS (Etapa 1):
//   Etapa1-ToJSON — ToJSON() adicionado a todos os eventos concretos.
//         Antes herdavam BaseEvent.ToJSON() que serializava e.Payload=nil
//         = "null" gravado no ledger. Rebuild seria impossível pois o
//         unmarshal de "null" retorna struct zerado.
// ============================================================================

package aggregates

import (
	"encoding/json"
	"time"
)

// AprovacaoAnoRegistradaEvent — academia registra decisão sobre o ano letivo.
type AprovacaoAnoRegistradaEvent struct {
	BaseEvent
	CodigoEstudante string
	CodigoAcademia  string
	AnoLectivo      string
	TipoEnsino      string  // "fundamental" | "medio" | "superior"
	NivelAtual      string  // nível no momento da decisão
	ProximoNivel    *string // nil = reprovado OU último ano do ciclo
	Aprovado        bool
	Observacao      *string
	RegisteredAt    time.Time
}

func (e *AprovacaoAnoRegistradaEvent) GetPayload() interface{} { return e }
func (e *AprovacaoAnoRegistradaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

// StatusEscolarFundamentalAtualizadoEvent — atualização manual do ciclo fundamental.
type StatusEscolarFundamentalAtualizadoEvent struct {
	BaseEvent
	NovoStatus string
	UpdatedAt  time.Time
}

func (e *StatusEscolarFundamentalAtualizadoEvent) GetPayload() interface{} { return e }
func (e *StatusEscolarFundamentalAtualizadoEvent) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}

// StatusEscolarMedioAtualizadoEvent — atualização manual do ciclo médio.
type StatusEscolarMedioAtualizadoEvent struct {
	BaseEvent
	NovoStatus string
	UpdatedAt  time.Time
}

func (e *StatusEscolarMedioAtualizadoEvent) GetPayload() interface{} { return e }
func (e *StatusEscolarMedioAtualizadoEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }