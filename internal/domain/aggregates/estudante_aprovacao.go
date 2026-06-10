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
