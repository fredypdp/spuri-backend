// ============================================================================
// ARQUIVO: internal/domain/aggregates/estudante_avaliacao.go
//
// CORREÇÕES APLICADAS (Etapa 1):
//   Etapa1-ToJSON — ToJSON() adicionado a AvaliacaoFinalAnoAcademicoEvent.
//         Antes herdava BaseEvent.ToJSON() que serializava e.Payload=nil
//         = "null" gravado no ledger. Rebuild impossível.
// ============================================================================

package aggregates

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type AvaliacaoFinalAnoAcademicoEvent struct {
	BaseEvent
	ID                  uuid.UUID `json:"id"`
	CodigoEstudante     string    `json:"codigo_estudante"`
	CodigoAcademia      string    `json:"codigo_academia"`
	AnoLectivo          string    `json:"ano_lectivo"`
	TipoEnsino          string    `json:"tipo_ensino"`
	AnoAcademicoAtual   string    `json:"nivel_ano_academico_atual"`
	ProximoAnoAcademico *string   `json:"proximo_ano_academico,omitempty"`
	CodigoTurma         *string   `json:"codigo_turma,omitempty"`
	Aprovado            bool      `json:"aprovado"`
	Observacao          *string   `json:"observacao,omitempty"`
	Tipo                string    `json:"tipo"`
	RegisteredAt        time.Time `json:"registered_at"`
}

func (e *AvaliacaoFinalAnoAcademicoEvent) GetPayload() interface{} { return e }
func (e *AvaliacaoFinalAnoAcademicoEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

func (e *Estudante) RegistrarAvaliacaoFinal(
	codigoAcademia string,
	anoLectivo string,
	tipoEnsino string,
	anoAcademicoAtual string,
	proximoAnoAcademico *string,
	codigoTurma *string,
	aprovado bool,
	observacao *string,
) error {
	if e.CodigoAcademia == nil || *e.CodigoAcademia != codigoAcademia {
		return fmt.Errorf("estudante não pertence a esta academia")
	}

	event := &AvaliacaoFinalAnoAcademicoEvent{
		BaseEvent:           BaseEvent{EventType: "AvaliacaoFinalAnoAcademico", AggregateID: e.ID},
		ID:                  uuid.New(),
		CodigoEstudante:     e.CodigoEstudante,
		CodigoAcademia:      codigoAcademia,
		AnoLectivo:          anoLectivo,
		TipoEnsino:          tipoEnsino,
		AnoAcademicoAtual:   anoAcademicoAtual,
		ProximoAnoAcademico: proximoAnoAcademico,
		CodigoTurma:         codigoTurma,
		Aprovado:            aprovado,
		Observacao:          observacao,
		Tipo:                "avaliacao_final",
		RegisteredAt:        time.Now(),
	}

	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) applyAvaliacaoFinalAnoAcademico(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applyAvaliacaoFinalAnoAcademico: marshal error: %w", err)
	}
	var ev AvaliacaoFinalAnoAcademicoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyAvaliacaoFinalAnoAcademico: unmarshal error: %w", err)
	}

	if !ev.Aprovado {
		return nil
	}

	if ev.ProximoAnoAcademico != nil {
		switch ev.TipoEnsino {
		case "fundamental":
			e.AnoEscolar = ev.ProximoAnoAcademico
		case "medio":
			e.AnoEscolarMedio = ev.ProximoAnoAcademico
		case "superior":
			e.AnoSuperior = ev.ProximoAnoAcademico
		}
	} else {
		switch ev.TipoEnsino {
		case "fundamental":
			e.StatusEscolarFundamental = "finalizado"
		case "medio":
			e.StatusEscolarMedio = "finalizado"
		case "superior":
			e.StatusSuperior = "finalizado"
		}
	}

	return nil
}