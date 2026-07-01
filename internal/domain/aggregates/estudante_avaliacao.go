package aggregates

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type AvaliacaoFinalBasePayload struct {
	BaseEvent
	ID                      uuid.UUID                        `json:"id"`
	CodigoEstudante         string                           `json:"codigo_estudante"`
	CodigoAcademia          string                           `json:"codigo_academia"`
	AnoLectivo              string                           `json:"ano_lectivo"`
	TipoEnsino              string                           `json:"nivel"`
	AnoAcademicoAtual       string                           `json:"nivel_ano_academico_atual"`
	ProximoAnoAcademico     *string                          `json:"proximo_ano_academico,omitempty"`
	CodigoTurma             *string                          `json:"codigo_turma,omitempty"`
	CodigosTurmasRemovidas  []string                         `json:"codigos_turmas_removidas,omitempty"`
	Aprovado                bool                             `json:"aprovado"`
	Observacao              *string                          `json:"observacao,omitempty"`
	Type                    string                           `json:"type"`
	NotaFinal               float64                          `json:"nota_final"`
	NotaMinimaAprovacao     float64                          `json:"nota_minima_aprovacao"`
	RegraAvaliacaoFinalID   *uuid.UUID                       `json:"regra_avaliacao_final_id,omitempty"`
	FormulaSnapshot         string                           `json:"formula_snapshot,omitempty"`
	AplicaSeReprovadoEmType *string                          `json:"aplica_se_reprovado_em_type,omitempty"`
	CursoIDSnapshot         *uuid.UUID                       `json:"curso_id_snapshot,omitempty"`
	MateriasChaveSnapshot   []uuid.UUID                      `json:"materias_chave_snapshot,omitempty"`
	SemestreAtualAvaliado   *int                             `json:"semestre_atual,omitempty"`
	ProximoSemestreAtual    *int                             `json:"proximo_semestre_atual,omitempty"`
	AnoSuperiorAntes        *string                          `json:"ano_superior_antes,omitempty"`
	AnoSuperiorDepois       *string                          `json:"ano_superior_depois,omitempty"`
	MotivoProgressao        *string                          `json:"motivo_progressao,omitempty"`
	ResultadosMaterias      []ResultadoMateriaAvaliacaoFinal `json:"resultados_materias,omitempty"`
	AprovadoComPendencia    bool                             `json:"aprovado_com_pendencia,omitempty"`
	PendenciasGeradas       []MateriaPendenteGerada          `json:"pendencias_geradas,omitempty"`
	RegisteredAt            time.Time                        `json:"registered_at"`
}

type ResultadoMateriaAvaliacaoFinal struct {
	MateriaID             uuid.UUID  `json:"materia_id"`
	NotaFinal             float64    `json:"nota_final"`
	Aprovado              bool       `json:"aprovado"`
	RegraAvaliacaoFinalID *uuid.UUID `json:"regra_avaliacao_final_id,omitempty"`
	Type                  string     `json:"type"`
	FormulaSnapshot       string     `json:"formula_snapshot"`
	PendenciaPermitida    bool       `json:"pendencia_permitida"`
}

type MateriaPendenteGerada struct {
	MateriaID uuid.UUID `json:"materia_id"`
	CursoID   uuid.UUID `json:"curso_id"`
	Nivel     string    `json:"nivel"`
	Escopo    string    `json:"escopo"`
}

type AvaliacaoFinalSuperiorProgressao struct {
	SemestreAtualAvaliado *int
	ProximoSemestreAtual  *int
	AnoSuperiorAntes      *string
	AnoSuperiorDepois     *string
}

type AvaliacaoFinalEscolarEvent struct{ AvaliacaoFinalBasePayload }
type AvaliacaoFinalSuperiorEvent struct{ AvaliacaoFinalBasePayload }

func (e *AvaliacaoFinalEscolarEvent) GetPayload() interface{}  { return e }
func (e *AvaliacaoFinalEscolarEvent) ToJSON() ([]byte, error)  { return json.Marshal(e) }
func (e *AvaliacaoFinalSuperiorEvent) GetPayload() interface{} { return e }
func (e *AvaliacaoFinalSuperiorEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

// chaveAvaliacaoAnoLetivo retorna a chave de idempotência para avaliações finais
// no mesmo ano letivo.
func chaveAvaliacaoAnoLetivo(anoLectivo, avaliacaoType string) string {
	return "ano_letivo:" + anoLectivo + ":" + avaliacaoType
}

// chaveAvaliacaoNivel retorna a chave de idempotência para avaliações finais
// Escolar/Superior no mesmo nível dentro do mesmo ano letivo.
func chaveAvaliacaoNivel(tipoEnsino, anoLectivo, anoAcademicoAtual, avaliacaoType string) string {
	grupo := "escolar"
	if tipoEnsino == "superior" {
		grupo = "superior"
	}
	return "nivel:" + grupo + ":" + anoLectivo + ":" + anoAcademicoAtual + ":" + avaliacaoType
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
	avaliacaoType string,
	notaFinal float64,
	notaMinimaAprovacao float64,
	regraAvaliacaoFinalID *uuid.UUID,
	formulaSnapshot string,
	aplicaSeReprovadoEmType *string,
	cursoIDSnapshot *uuid.UUID,
	materiasChaveSnapshot []uuid.UUID,
	motivoProgressao *string,
	resultadosMaterias []ResultadoMateriaAvaliacaoFinal,
	aprovadoComPendencia bool,
	pendenciasGeradas []MateriaPendenteGerada,
	progressaoSuperior ...AvaliacaoFinalSuperiorProgressao,
) error {
	if e.CodigoAcademia == nil || *e.CodigoAcademia != codigoAcademia {
		return fmt.Errorf("estudante não pertence a esta academia")
	}

	// Guard de duplicata: impede double-submit e garante idempotência no aggregate.
	// Primeiro bloqueia a duplicidade no mesmo nível de Avaliação Final
	// Escolar/Superior; depois bloqueia qualquer segunda decisão no ano letivo.
	if avaliacaoType == "" {
		avaliacaoType = "normal"
	}
	chaveNivel := chaveAvaliacaoNivel(tipoEnsino, anoLectivo, anoAcademicoAtual, avaliacaoType)
	if e.AvaliacoesPorAno != nil && e.AvaliacoesPorAno[chaveNivel] {
		return fmt.Errorf(
			"avaliação final já registrada para o nível '%s' no ano letivo '%s'",
			anoAcademicoAtual, anoLectivo,
		)
	}

	chaveAnoLetivo := chaveAvaliacaoAnoLetivo(anoLectivo, avaliacaoType)
	if tipoEnsino != "superior" && e.AvaliacoesPorAno != nil && e.AvaliacoesPorAno[chaveAnoLetivo] {
		return fmt.Errorf(
			"avaliação final já registrada no ano letivo '%s'",
			anoLectivo,
		)
	}

	var progressao AvaliacaoFinalSuperiorProgressao
	if len(progressaoSuperior) > 0 {
		progressao = progressaoSuperior[0]
	}

	base := AvaliacaoFinalBasePayload{
		BaseEvent:               BaseEvent{AggregateID: e.ID},
		ID:                      uuid.New(),
		CodigoEstudante:         e.CodigoEstudante,
		CodigoAcademia:          codigoAcademia,
		AnoLectivo:              anoLectivo,
		TipoEnsino:              tipoEnsino,
		AnoAcademicoAtual:       anoAcademicoAtual,
		ProximoAnoAcademico:     proximoAnoAcademico,
		CodigoTurma:             codigoTurma,
		CodigosTurmasRemovidas:  codigosTurmasRemovidas,
		Aprovado:                aprovado,
		Observacao:              observacao,
		Type:                    avaliacaoType,
		NotaFinal:               notaFinal,
		NotaMinimaAprovacao:     notaMinimaAprovacao,
		RegraAvaliacaoFinalID:   regraAvaliacaoFinalID,
		FormulaSnapshot:         formulaSnapshot,
		AplicaSeReprovadoEmType: aplicaSeReprovadoEmType,
		CursoIDSnapshot:         cursoIDSnapshot,
		MateriasChaveSnapshot:   materiasChaveSnapshot,
		SemestreAtualAvaliado:   progressao.SemestreAtualAvaliado,
		ProximoSemestreAtual:    progressao.ProximoSemestreAtual,
		AnoSuperiorAntes:        progressao.AnoSuperiorAntes,
		AnoSuperiorDepois:       progressao.AnoSuperiorDepois,
		MotivoProgressao:        motivoProgressao,
		ResultadosMaterias:      resultadosMaterias,
		AprovadoComPendencia:    aprovadoComPendencia,
		PendenciasGeradas:       pendenciasGeradas,
		RegisteredAt:            time.Now(),
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
	avaliacaoType := ev.Type
	if avaliacaoType == "" {
		avaliacaoType = "normal"
	}
	if ev.TipoEnsino != "superior" {
		e.AvaliacoesPorAno[chaveAvaliacaoAnoLetivo(ev.AnoLectivo, avaliacaoType)] = true
	}
	e.AvaliacoesPorAno[chaveAvaliacaoNivel(ev.TipoEnsino, ev.AnoLectivo, ev.AnoAcademicoAtual, avaliacaoType)] = true

	if !ev.Aprovado {
		return nil
	}

	if ev.ProximoAnoAcademico != nil {
		switch ev.TipoEnsino {
		case "fundamental":
			e.AnoEscolar = ev.ProximoAnoAcademico
			e.StatusEscolarFundamental = "em_andamento"
		case "medio":
			e.AnoEscolarMedio = ev.ProximoAnoAcademico
		case "superior":
			if ev.ProximoSemestreAtual != nil {
				e.SemestreAtual = ev.ProximoSemestreAtual
			}
			if ev.AnoSuperiorDepois != nil {
				e.AnoSuperior = ev.AnoSuperiorDepois
			} else {
				e.AnoSuperior = ev.ProximoAnoAcademico
			}
		}
	} else {
		// Último ano do ciclo — marcar como finalizado.
		switch ev.TipoEnsino {
		case "fundamental":
			e.StatusEscolarFundamental = "finalizado"
		case "medio":
			e.StatusEscolarMedio = "finalizado"
		case "superior":
			if ev.AnoSuperiorDepois != nil {
				e.AnoSuperior = ev.AnoSuperiorDepois
			}
			e.StatusSuperior = "finalizado"
		}
	}

	return nil
}
