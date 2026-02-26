// ============================================================================
// CORREÇÃO: internal/domain/aggregates/estudante_aprovacao.go
//
// PROBLEMA 1: applyAprovacaoAnoRegistrada usava e.AnoEscolar para AMBOS
//   fundamental e médio — avançar no médio sobrescrevia o campo do fundamental.
//
// SOLUÇÃO: O aggregate Estudante precisa de um campo separado para o ano do
//   médio. Adicionamos AnoEscolarMedio *string ao struct Estudante.
//   - fundamental → e.AnoEscolar      (mantém nome existente, semântica = ano atual no fundamental)
//   - medio       → e.AnoEscolarMedio (novo campo)
//   - superior    → e.AnoSuperior     (sem alteração)
//
// PROBLEMA 2: Ao reprovar, o estado do aggregate não é alterado — correto.
//   Mas é preciso garantir que o evento registrado na projeção capture
//   adequadamente a reprovação (aprovado=false + sem proximo_nivel).
//   O evento já tem esses campos; a projeção cuida do registro.
//
// IMPACTO NAS MIGRATIONS: A migration 008 usa `ano_escolar` na projeção para
//   ambos os ciclos. Uma migration adicional (009) deve adicionar a coluna
//   `ano_escolar_medio` na projection_estudantes (ver migration_009.sql).
// ============================================================================

package aggregates

import (
	"encoding/json"
	"fmt"
	"time"
)

// RegistrarAprovacaoAno registra a decisão da academia sobre o ano letivo.
//
// Regras de negócio:
//   - aprovado=true  + proximoNivel!=nil  → avança para o próximo nível
//   - aprovado=true  + proximoNivel==nil  → último ano do ciclo; status=finalizado
//   - aprovado=false + proximoNivel==nil  → reprovado; nenhum estado alterado;
//     evento registrado como log auditável
func (e *Estudante) RegistrarAprovacaoAno(
	codigoAcademia string,
	anoLectivo string,
	tipoEnsino string, // "fundamental" | "medio" | "superior"
	nivelAtual string,
	proximoNivel *string, // nil quando reprovado ou último ano
	aprovado bool,
	observacao *string,
) error {
	if e.CodigoAcademia == nil || *e.CodigoAcademia != codigoAcademia {
		return fmt.Errorf("estudante não pertence a esta academia")
	}

	tiposValidos := map[string]bool{"fundamental": true, "medio": true, "superior": true}
	if !tiposValidos[tipoEnsino] {
		return fmt.Errorf("tipo_ensino inválido: deve ser fundamental, medio ou superior")
	}

	if nivelAtual == "" {
		return fmt.Errorf("nivel_atual é obrigatório")
	}

	if !aprovado && proximoNivel != nil {
		return fmt.Errorf("estudante reprovado não deve ter proximo_nivel definido")
	}

	event := &AprovacaoAnoRegistradaEvent{
		BaseEvent:       BaseEvent{EventType: "AprovacaoAnoRegistrada", AggregateID: e.ID},
		CodigoEstudante: e.CodigoEstudante,
		CodigoAcademia:  codigoAcademia,
		AnoLectivo:      anoLectivo,
		TipoEnsino:      tipoEnsino,
		NivelAtual:      nivelAtual,
		ProximoNivel:    proximoNivel,
		Aprovado:        aprovado,
		Observacao:      observacao,
		RegisteredAt:    time.Now(),
	}

	e.RaiseEvent(event)
	return e.Apply(event)
}

// AtualizarStatusEscolarFundamental permite atualização manual do status do ciclo fundamental.
func (e *Estudante) AtualizarStatusEscolarFundamental(novoStatus string) error {
	validos := map[string]bool{"inativo": true, "em_andamento": true, "finalizado": true}
	if !validos[novoStatus] {
		return fmt.Errorf("status inválido: deve ser inativo, em_andamento ou finalizado")
	}

	event := &StatusEscolarFundamentalAtualizadoEvent{
		BaseEvent:  BaseEvent{EventType: "StatusEscolarFundamentalAtualizado", AggregateID: e.ID},
		NovoStatus: novoStatus,
		UpdatedAt:  time.Now(),
	}

	e.RaiseEvent(event)
	return e.Apply(event)
}

// AtualizarStatusEscolarMedio permite atualização manual do status do ciclo médio.
// Requer que status_escolar_fundamental seja "finalizado" para ativar/finalizar médio.
func (e *Estudante) AtualizarStatusEscolarMedio(novoStatus string) error {
	validos := map[string]bool{"inativo": true, "em_andamento": true, "finalizado": true}
	if !validos[novoStatus] {
		return fmt.Errorf("status inválido: deve ser inativo, em_andamento ou finalizado")
	}

	if (novoStatus == "em_andamento" || novoStatus == "finalizado") &&
		e.StatusEscolarFundamental != "finalizado" {
		return fmt.Errorf("status_escolar_medio só pode ser ativado/finalizado se status_escolar_fundamental for 'finalizado'")
	}

	event := &StatusEscolarMedioAtualizadoEvent{
		BaseEvent:  BaseEvent{EventType: "StatusEscolarMedioAtualizado", AggregateID: e.ID},
		NovoStatus: novoStatus,
		UpdatedAt:  time.Now(),
	}

	e.RaiseEvent(event)
	return e.Apply(event)
}

// ============================================================================
// Apply Handlers
// ============================================================================

// applyAprovacaoAnoRegistrada — CORRIGIDO.
//
// Antes: usava e.AnoEscolar para "fundamental" e "medio" (campo único compartilhado).
// Agora: usa campos segregados:
//   - fundamental → e.AnoEscolar
//   - medio       → e.AnoEscolarMedio  ← NOVO CAMPO no struct Estudante
//   - superior    → e.AnoSuperior
func (e *Estudante) applyAprovacaoAnoRegistrada(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return err
	}

	var ev AprovacaoAnoRegistradaEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	// Reprovado: nenhum estado é alterado — evento serve apenas como log
	if !ev.Aprovado {
		return nil
	}

	if ev.ProximoNivel != nil {
		// Aprovado com próximo nível: avança o ano no campo correto
		switch ev.TipoEnsino {
		case "fundamental":
			e.AnoEscolar = ev.ProximoNivel
		case "medio":
			e.AnoEscolarMedio = ev.ProximoNivel // ← CORRIGIDO (era e.AnoEscolar)
		case "superior":
			e.AnoSuperior = ev.ProximoNivel
		}
		return nil
	}

	// Aprovado sem próximo nível: último ano do ciclo → finaliza status
	switch ev.TipoEnsino {
	case "fundamental":
		e.StatusEscolarFundamental = "finalizado"
	case "medio":
		e.StatusEscolarMedio = "finalizado"
	case "superior":
		e.StatusSuperior = "finalizado"
	}

	return nil
}

func (e *Estudante) applyStatusEscolarFundamentalAtualizado(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return err
	}

	var ev StatusEscolarFundamentalAtualizadoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	e.StatusEscolarFundamental = ev.NovoStatus
	return nil
}

func (e *Estudante) applyStatusEscolarMedioAtualizado(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return err
	}

	var ev StatusEscolarMedioAtualizadoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	e.StatusEscolarMedio = ev.NovoStatus
	return nil
}

// ============================================================================
// Eventos
// ============================================================================

// AprovacaoAnoRegistradaEvent representa a decisão da academia sobre um ano letivo.
// Imutável — tanto aprovações quanto reprovações geram este evento.
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

// StatusEscolarFundamentalAtualizadoEvent — atualização manual do ciclo fundamental.
type StatusEscolarFundamentalAtualizadoEvent struct {
	BaseEvent
	NovoStatus string
	UpdatedAt  time.Time
}

func (e *StatusEscolarFundamentalAtualizadoEvent) GetPayload() interface{} { return e }

// StatusEscolarMedioAtualizadoEvent — atualização manual do ciclo médio.
type StatusEscolarMedioAtualizadoEvent struct {
	BaseEvent
	NovoStatus string
	UpdatedAt  time.Time
}

func (e *StatusEscolarMedioAtualizadoEvent) GetPayload() interface{} { return e }