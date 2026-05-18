package aggregates

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type AvaliacaoFinalBasePayload struct {
	BaseEvent
	ID                     uuid.UUID `json:"id"`
	CodigoEstudante        string    `json:"codigo_estudante"`
	CodigoAcademia         string    `json:"codigo_academia"`
	AnoLectivo             string    `json:"ano_lectivo"`
	TipoEnsino             string    `json:"tipo_ensino"`
	AnoAcademicoAtual      string    `json:"nivel_ano_academico_atual"`
	ProximoAnoAcademico    *string   `json:"proximo_ano_academico,omitempty"`
	CodigoTurma            *string   `json:"codigo_turma,omitempty"`
	CodigosTurmasRemovidas []string  `json:"codigos_turmas_removidas,omitempty"`
	Aprovado               bool      `json:"aprovado"`
	Observacao             *string   `json:"observacao,omitempty"`
	Tipo                   string    `json:"tipo"`
	RegisteredAt           time.Time `json:"registered_at"`
}

type AvaliacaoFinalEscolarEvent struct{ AvaliacaoFinalBasePayload }
type AvaliacaoFinalSuperiorEvent struct{ AvaliacaoFinalBasePayload }

func (e *AvaliacaoFinalEscolarEvent) GetPayload() interface{}  { return e }
func (e *AvaliacaoFinalEscolarEvent) ToJSON() ([]byte, error)  { return json.Marshal(e) }
func (e *AvaliacaoFinalSuperiorEvent) GetPayload() interface{} { return e }
func (e *AvaliacaoFinalSuperiorEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

// chaveAvaliacao retorna a chave de idempotência para avaliações finais.
// Formato: "<anoLectivo>".
//
// A avaliação final representa a decisão única do ano letivo para o estudante,
// independentemente do tipo de ensino ou nível/ano acadêmico informado.
func chaveAvaliacao(anoLectivo string) string {
	return anoLectivo
}

func (e *Estudante) RegistrarAvaliacaoFinal(
	codigoAcademia string,
	anoLectivo string,
	tipoEnsino string,
	anoAcademicoAtual string,
	proximoAnoAcademico *string,
	codigoTurma *string,
	codigosTurmasRemovidas []string,
	aprovado bool,
	observacao *string,
) error {
	if e.CodigoAcademia == nil || *e.CodigoAcademia != codigoAcademia {
		return fmt.Errorf("estudante não pertence a esta academia")
	}

	// Guard de duplicata: impede double-submit e garante idempotência no aggregate.
	// A chave cobre o ano letivo para impedir uma segunda aprovação ou
	// reprovação do mesmo estudante no mesmo ano letivo.
	chave := chaveAvaliacao(anoLectivo)
	if e.AvaliacoesPorAno != nil && e.AvaliacoesPorAno[chave] {
		return fmt.Errorf(
			"avaliação final já registrada no ano letivo '%s'",
			anoLectivo,
		)
	}

	base := AvaliacaoFinalBasePayload{
		BaseEvent:              BaseEvent{AggregateID: e.ID},
		ID:                     uuid.New(),
		CodigoEstudante:        e.CodigoEstudante,
		CodigoAcademia:         codigoAcademia,
		AnoLectivo:             anoLectivo,
		TipoEnsino:             tipoEnsino,
		AnoAcademicoAtual:      anoAcademicoAtual,
		ProximoAnoAcademico:    proximoAnoAcademico,
		CodigoTurma:            codigoTurma,
		CodigosTurmasRemovidas: codigosTurmasRemovidas,
		Aprovado:               aprovado,
		Observacao:             observacao,
		Tipo:                   "avaliacao_final",
		RegisteredAt:           time.Now(),
	}
	var event DomainEvent
	if tipoEnsino == "superior" {
		base.EventType = "AvaliacaoFinalSuperior"
		event = &AvaliacaoFinalSuperiorEvent{AvaliacaoFinalBasePayload: base}
	} else {
		base.EventType = "AvaliacaoFinalEscolar"
		event = &AvaliacaoFinalEscolarEvent{AvaliacaoFinalBasePayload: base}
	}

	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) applyAvaliacaoFinalEscolar(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applyAvaliacaoFinalEscolar: marshal error: %w", err)
	}
	var ev AvaliacaoFinalEscolarEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyAvaliacaoFinalEscolar: unmarshal error: %w", err)
	}
	return e.applyAvaliacaoFinalPayload(ev.AvaliacaoFinalBasePayload)
}

func (e *Estudante) applyAvaliacaoFinalSuperior(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applyAvaliacaoFinalSuperior: marshal error: %w", err)
	}
	var ev AvaliacaoFinalSuperiorEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyAvaliacaoFinalSuperior: unmarshal error: %w", err)
	}
	return e.applyAvaliacaoFinalPayload(ev.AvaliacaoFinalBasePayload)
}

func (e *Estudante) applyAvaliacaoFinalPayload(ev AvaliacaoFinalBasePayload) error {
	// Registrar no mapa de idempotência independentemente de aprovado ou não.
	if e.AvaliacoesPorAno == nil {
		e.AvaliacoesPorAno = make(map[string]bool)
	}
	chave := chaveAvaliacao(ev.AnoLectivo)
	e.AvaliacoesPorAno[chave] = true

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
		// Último ano do ciclo — marcar como finalizado.
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
