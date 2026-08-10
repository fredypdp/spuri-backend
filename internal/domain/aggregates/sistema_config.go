package aggregates

import (
	"encoding/json"
	"time"
)

// EmailVerificadoEvent é compartilhado entre Admin e Academia.
// Definido aqui por ser o primeiro aggregate que o utiliza na hierarquia.
type EmailVerificadoEvent struct {
	BaseEvent
	VerifiedAt time.Time
}

func (e *EmailVerificadoEvent) GetPayload() interface{} { return e }
func (e *EmailVerificadoEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }
