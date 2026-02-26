// ============================================================================
// Lógica de aprovação/reprovação de ano letivo.
// Anos são dinâmicos e definidos por cada academia/curso.
// A academia é responsável por validar e informar o proximo_nivel correto —
// o domínio apenas garante a consistência do aggregate.
// ============================================================================

package aggregates

import (
	"encoding/json"
	"fmt"
	"time"
)

// ============================================================================
// Comandos do Aggregate
// ============================================================================

// RegistrarAprovacaoAno registra a decisão da academia sobre o ano letivo do estudante.
//
// Regras:
//   - Se aprovado e proximoNivel != nil → estudante avança para o próximo nível.
//   - Se aprovado e proximoNivel == nil → estudante está no último ano do ciclo;
//     o status correspondente (fundamental/medio/superior) é marcado como "finalizado".
//   - Se reprovado → apenas registra o evento; nenhum estado é alterado.
//
// O handler (camada de aplicação) é responsável por validar que nivelAtual e
// proximoNivel são anos válidos para o curso do estudante antes de chamar aqui.
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

// AtualizarStatusEscolarFundamental permite atualizar manualmente o status
// do ciclo fundamental (usado por academia/admin fora do fluxo de aprovação).
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

// AtualizarStatusEscolarMedio permite atualizar manualmente o status
// do ciclo médio (usado por academia/admin fora do fluxo de aprovação).
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
// (registrar no switch Apply() de estudante.go — ver guia de alterações)
// ============================================================================

func (e *Estudante) applyAprovacaoAnoRegistrada(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return err
	}

	var ev AprovacaoAnoRegistradaEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	if !ev.Aprovado {
		// Reprovado: nenhum estado do aggregate é alterado
		return nil
	}

	if ev.ProximoNivel != nil {
		// Aprovado com próximo nível: avança o ano do estudante
		switch ev.TipoEnsino {
		case "fundamental", "medio":
			e.AnoEscolar = ev.ProximoNivel
		case "superior":
			e.AnoSuperior = ev.ProximoNivel
		}
		return nil
	}

	// Aprovado sem próximo nível: último ano do ciclo → finaliza
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
// Este evento é imutável e constitui o registro histórico completo —
// tanto aprovações quanto reprovações geram este evento.
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

// StatusEscolarFundamentalAtualizadoEvent representa atualização manual
// do status do ciclo fundamental (fora do fluxo de aprovação de ano).
type StatusEscolarFundamentalAtualizadoEvent struct {
	BaseEvent
	NovoStatus string
	UpdatedAt  time.Time
}

func (e *StatusEscolarFundamentalAtualizadoEvent) GetPayload() interface{} { return e }

// StatusEscolarMedioAtualizadoEvent representa atualização manual
// do status do ciclo médio (fora do fluxo de aprovação de ano).
type StatusEscolarMedioAtualizadoEvent struct {
	BaseEvent
	NovoStatus string
	UpdatedAt  time.Time
}

func (e *StatusEscolarMedioAtualizadoEvent) GetPayload() interface{} { return e }