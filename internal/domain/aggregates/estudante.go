// ============================================================================
// ARQUIVO: internal/domain/aggregates/estudante.go
// ATUALIZADO: curso_medio_id e curso_superior_id agora são UUID
// ============================================================================

package aggregates

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Estudante struct {
	BaseAggregate
	
	Nome                  string
	CodigoEstudante       string
	SenhaHash             string
	Email                 *string
	Telefone              *string
	EmailVerificado       bool
	BilheteIdentidade     *string
	BilheteIdentidadeResp *string
	CodigoAcademia        *string
	Status                string
	AnoEscolar            *string
	AnoSuperior           *string
	CursoMedioID          *uuid.UUID // 🔥 MUDOU: agora é UUID
	CursoSuperiorID       *uuid.UUID // 🔥 MUDOU: agora é UUID
	StatusEscolar         string
	StatusSuperior        string
	CreatedAt             time.Time
	
	Inscricoes []Inscricao
}

type Inscricao struct {
	ID             uuid.UUID
	CodigoAcademia string
	Tipo           string
	AnoInscricao   string
	CursoID        *uuid.UUID // 🔥 MUDOU: agora é UUID
	Status         string
	StatusUsado    bool
	CreatedAt      time.Time
}

func NewEstudante() *Estudante {
	return &Estudante{
		BaseAggregate: BaseAggregate{
			ID:                uuid.New(),
			Version:           0,
			UncommittedEvents: []DomainEvent{},
		},
		Status:          "inativo",
		StatusEscolar:   "inativo",
		StatusSuperior:  "inativo",
		EmailVerificado: false,
		Inscricoes:      []Inscricao{},
	}
}

func (e *Estudante) GetType() string {
	return "Estudante"
}

func (e *Estudante) Apply(event DomainEvent) error {
	switch event.GetEventType() {
	case "EstudanteCriado":
		return e.applyEstudanteCriado(event)
	case "NotasRegistradas":
		return e.applyNotasRegistradas(event)
	case "NotaAtualizada":
		return e.applyNotaAtualizada(event)
	case "FaltasRegistradas":
		return e.applyFaltasRegistradas(event)
	case "EstudanteInscrito":
		return e.applyEstudanteInscrito(event)
	case "InscricaoAprovada":
		return e.applyInscricaoAprovada(event)
	case "InscricaoReprovada":
		return e.applyInscricaoReprovada(event)
	case "EstudanteVinculado":
		return e.applyEstudanteVinculado(event)
	case "StatusEscolarAtualizado":
		return e.applyStatusEscolarAtualizado(event)
	case "StatusSuperiorAtualizado":
		return e.applyStatusSuperiorAtualizado(event)
	case "DadosPessoaisAtualizados":
		return e.applyDadosPessoaisAtualizados(event)
	case "DadosAcademicosAtualizados":
		return e.applyDadosAcademicosAtualizados(event)
	case "EmailVerificado":
		return e.applyEmailVerificado(event)
	case "AprovacaoAnoRegistrada":
		return e.applyAprovacaoAnoRegistrada(event)
	case "CursoAlterado":
		return e.applyCursoAlterado(event)
	case "EstudanteCriadoComVinculo":
		return e.applyEstudanteCriadoComVinculo(event)
	default:
		return fmt.Errorf("tipo de evento desconhecido: %s", event.GetEventType())
	}
}

// 🔥 ATUALIZADO
func (e *Estudante) Criar(
	nome string,
	codigoEstudante string,
	senhaHash string,
	email *string,
	telefone *string,
	bilhete *string,
	bilheteResp *string,
	anoEscolar *string,
	anoSuperior *string,
	cursoMedioID *uuid.UUID,
	cursoSuperiorID *uuid.UUID,
	statusEscolar *string,
	statusSuperior *string,
) error {
	if nome == "" || codigoEstudante == "" || senhaHash == "" {
		return fmt.Errorf("campos obrigatórios vazios")
	}
	
	if bilhete == nil && bilheteResp == nil {
		return fmt.Errorf("pelo menos um bilhete de identidade do estudante é obrigatório")
	}

	// Validar bilhetes diferentes
	if bilhete != nil && bilheteResp != nil && *bilhete == *bilheteResp {
		return fmt.Errorf("bilhete de identidade do estudante e bilhete do responsável não podem ser iguais")
	}

	statusEsc := "inativo"
	statusSup := "inativo"
	
	if statusEscolar != nil {
		if *statusEscolar != "inativo" && *statusEscolar != "em_andamento" && *statusEscolar != "finalizado" {
			return fmt.Errorf("status_escolar inválido")
		}
		statusEsc = *statusEscolar
	}
	
	if statusSuperior != nil {
		if *statusSuperior != "inativo" && *statusSuperior != "em_andamento" && *statusSuperior != "finalizado" {
			return fmt.Errorf("status_superior inválido")
		}
		statusSup = *statusSuperior
	}

	event := &EstudanteCriadoEvent{
		BaseEvent:             BaseEvent{EventType: "EstudanteCriado", AggregateID: e.ID},
		Nome:                  nome,
		CodigoEstudante:       codigoEstudante,
		SenhaHash:             senhaHash,
		Email:                 email,
		Telefone:              telefone,
		BilheteIdentidade:     bilhete,
		BilheteIdentidadeResp: bilheteResp,
		AnoEscolar:            anoEscolar,
		AnoSuperior:           anoSuperior,
		CursoMedioID:          cursoMedioID,      // 🔥 MUDOU
		CursoSuperiorID:       cursoSuperiorID,   // 🔥 MUDOU
		StatusEscolar:         statusEsc,
		StatusSuperior:        statusSup,
		CreatedAt:             time.Now(),
	}

	e.RaiseEvent(event)
	return e.Apply(event)
}

// CriarComVinculo cria estudante já vinculado a uma academia (usado por academias)
func (e *Estudante) CriarComVinculo(
	nome string,
	codigoEstudante string,
	senhaHash string,
	email *string,
	telefone *string,
	bilhete *string,
	bilheteResp *string,
	anoEscolar *string,
	anoSuperior *string,
	cursoMedioID *uuid.UUID,
	cursoSuperiorID *uuid.UUID,
	statusEscolar *string,
	statusSuperior *string,
	codigoAcademia string, // 🔥 DIFERENÇA: já recebe o código da academia
) error {
	if nome == "" || codigoEstudante == "" || senhaHash == "" {
		return fmt.Errorf("campos obrigatórios vazios")
	}
	
	if bilhete == nil && bilheteResp == nil {
		return fmt.Errorf("pelo menos um bilhete de identidade do estudante  é obrigatório")
	}

	if bilhete != nil && bilheteResp != nil && *bilhete == *bilheteResp {
		return fmt.Errorf("bilhete de identidade  do estudante e bilhete do responsável não podem ser iguais")
	}

	if codigoAcademia == "" {
		return fmt.Errorf("código da academia é obrigatório")
	}

	statusEsc := "inativo"
	statusSup := "inativo"
	
	if statusEscolar != nil {
		if *statusEscolar != "inativo" && *statusEscolar != "em_andamento" && *statusEscolar != "finalizado" {
			return fmt.Errorf("status_escolar inválido")
		}
		statusEsc = *statusEscolar
	}
	
	if statusSuperior != nil {
		if *statusSuperior != "inativo" && *statusSuperior != "em_andamento" && *statusSuperior != "finalizado" {
			return fmt.Errorf("status_superior inválido")
		}
		statusSup = *statusSuperior
	}

	// 🔥 LOG PARA DEBUG
	log.Printf("[DEBUG] CriarComVinculo - Recebido: AnoEscolar=%v, CursoMedioID=%v", anoEscolar, cursoMedioID)

	event := &EstudanteCriadoComVinculoEvent{
		BaseEvent:             BaseEvent{EventType: "EstudanteCriadoComVinculo", AggregateID: e.ID},
		Nome:                  nome,
		CodigoEstudante:       codigoEstudante,
		SenhaHash:             senhaHash,
		Email:                 email,
		Telefone:              telefone,
		BilheteIdentidade:     bilhete,
		BilheteIdentidadeResp: bilheteResp,
		AnoEscolar:            anoEscolar,            // 🔥 Deve propagar
		AnoSuperior:           anoSuperior,          // 🔥 Deve propagar
		CursoMedioID:          cursoMedioID,         // 🔥 Deve propagar
		CursoSuperiorID:       cursoSuperiorID,      // 🔥 Deve propagar
		StatusEscolar:         statusEsc,
		StatusSuperior:        statusSup,
		CodigoAcademia:        codigoAcademia, // 🔥 JÁ VINCULADO
		CreatedAt:             time.Now(),
	}

	// 🔥 LOG DO EVENTO CRIADO
	log.Printf("[DEBUG] Evento criado - AnoEscolar=%v, CursoMedioID=%v", event.AnoEscolar, event.CursoMedioID)

	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) VerificarEmail() error {
	if e.EmailVerificado {
		return fmt.Errorf("email já verificado")
	}

	event := &EmailVerificadoEvent{
		BaseEvent:  BaseEvent{EventType: "EmailVerificado", AggregateID: e.ID},
		VerifiedAt: time.Now(),
	}

	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) RegistrarFalta(codigoAcademia string, anoLectivo string, data time.Time, materiaDisciplinarID uuid.UUID, quantidade int, observacao *string) error {
	if e.CodigoAcademia == nil || *e.CodigoAcademia != codigoAcademia {
		return fmt.Errorf("estudante não pertence a esta academia")
	}
	
	if quantidade <= 0 {
		return fmt.Errorf("quantidade deve ser maior que zero")
	}

	event := &FaltasRegistradasEvent{
		BaseEvent:            BaseEvent{EventType: "FaltasRegistradas", AggregateID: e.ID},
		CodigoEstudante:      e.CodigoEstudante,
		CodigoAcademia:       codigoAcademia,
		AnoLectivo:           anoLectivo,
		Data:                 data,
		MateriaDisciplinarID: materiaDisciplinarID,
		Quantidade:           quantidade,
		Observacao:           observacao,
		RegisteredAt:         time.Now(),
	}

	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) RegistrarAprovacaoAno(
	codigoAcademia string,
	anoLectivo string,
	nivelAtual string,
	avancarAno bool,
	observacao *string,
) error {
	if e.CodigoAcademia == nil || *e.CodigoAcademia != codigoAcademia {
		return fmt.Errorf("estudante não pertence a esta academia")
	}

	tipo := "escolar"
	if strings.Contains(nivelAtual, "_ano") {
		tipo = "superior"
	}

	var nivelSeguinte *string
	if avancarAno {
		proximo := getProximoNivel(nivelAtual, tipo)
		if proximo != "" {
			nivelSeguinte = &proximo
		}
	}

	event := &AprovacaoAnoRegistradaEvent{
		BaseEvent:       BaseEvent{EventType: "AprovacaoAnoRegistrada", AggregateID: e.ID},
		CodigoEstudante: e.CodigoEstudante,
		CodigoAcademia:  codigoAcademia,
		AnoLectivo:      anoLectivo,
		NivelAtual:      nivelAtual,
		NivelSeguinte:   nivelSeguinte,
		AvancarAno:      avancarAno,
		Observacao:      observacao,
		RegisteredAt:    time.Now(),
	}

	e.RaiseEvent(event)
	return e.Apply(event)
}

// 🔥 ATUALIZADO
func (e *Estudante) SolicitarInscricao(codigoAcademia string, tipo string, anoInscricao string, cursoID *uuid.UUID) error {
	if tipo != "escola" && tipo != "universidade" {
		return fmt.Errorf("tipo deve ser 'escola' ou 'universidade'")
	}

	if e.CodigoAcademia != nil && *e.CodigoAcademia == codigoAcademia {
		return fmt.Errorf("você já está matriculado nesta academia")
	}

	for _, inscricao := range e.Inscricoes {
		if inscricao.CodigoAcademia == codigoAcademia && inscricao.Status == "espera" {
			return fmt.Errorf("você já possui uma inscrição pendente nesta academia")
		}
	}

	inscricaoID := uuid.New()
	event := &EstudanteInscritoEvent{
		BaseEvent:      BaseEvent{EventType: "EstudanteInscrito", AggregateID: e.ID},
		InscricaoID:    inscricaoID,
		CodigoAcademia: codigoAcademia,
		Tipo:           tipo,
		AnoInscricao:   anoInscricao,
		CursoID:        cursoID, // 🔥 MUDOU
		CreatedAt:      time.Now(),
	}

	e.RaiseEvent(event)
	return e.Apply(event)
}

// 🔥 ATUALIZADO
func (e *Estudante) AprovarInscricao(codigoAcademia string, inscricaoID uuid.UUID) error {
	var inscricaoPendente *Inscricao
	for i := range e.Inscricoes {
		if e.Inscricoes[i].CodigoAcademia == codigoAcademia && e.Inscricoes[i].Status == "espera" {
			inscricaoPendente = &e.Inscricoes[i]
			break
		}
	}

	if inscricaoPendente == nil {
		return fmt.Errorf("nenhuma inscrição pendente encontrada")
	}

	event := &InscricaoAprovadaEvent{
		BaseEvent:      BaseEvent{EventType: "InscricaoAprovada", AggregateID: e.ID},
		InscricaoID:    inscricaoID,
		CodigoAcademia: codigoAcademia,
		Tipo:           inscricaoPendente.Tipo,
		AnoInscricao:   inscricaoPendente.AnoInscricao,
		CursoID:        inscricaoPendente.CursoID, // 🔥 MUDOU
	}

	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) ReprovarInscricao(codigoAcademia string, inscricaoID uuid.UUID) error {
	var inscricaoPendente *Inscricao
	for i := range e.Inscricoes {
		if e.Inscricoes[i].CodigoAcademia == codigoAcademia && e.Inscricoes[i].Status == "espera" {
			inscricaoPendente = &e.Inscricoes[i]
			break
		}
	}

	if inscricaoPendente == nil {
		return fmt.Errorf("nenhuma inscrição pendente encontrada")
	}

	event := &InscricaoReprovadaEvent{
		BaseEvent:      BaseEvent{EventType: "InscricaoReprovada", AggregateID: e.ID},
		InscricaoID:    inscricaoID,
		CodigoAcademia: codigoAcademia,
	}

	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) VincularAcademia(inscricaoID uuid.UUID) error {
	var inscricao *Inscricao
	for i := range e.Inscricoes {
		if e.Inscricoes[i].ID == inscricaoID {
			inscricao = &e.Inscricoes[i]
			break
		}
	}

	if inscricao == nil {
		return fmt.Errorf("inscrição não encontrada")
	}

	if inscricao.Status != "aprovado" {
		return fmt.Errorf("inscrição não foi aprovada")
	}

	if inscricao.StatusUsado {
		return fmt.Errorf("esta inscrição já foi utilizada")
	}

	event := &EstudanteVinculadoEvent{
		BaseEvent:      BaseEvent{EventType: "EstudanteVinculado", AggregateID: e.ID},
		InscricaoID:    inscricaoID,
		CodigoAcademia: inscricao.CodigoAcademia,
		VinculadoAt:    time.Now(),
	}

	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) AtualizarStatusEscolar(novoStatus string) error {
	validStatus := map[string]bool{"inativo": true, "em_andamento": true, "finalizado": true}
	if !validStatus[novoStatus] {
		return fmt.Errorf("status inválido")
	}

	if novoStatus == "inativo" && e.StatusSuperior != "inativo" {
		return fmt.Errorf("não pode inativar status_escolar enquanto status_superior está ativo")
	}

	event := &StatusEscolarAtualizadoEvent{
		BaseEvent:  BaseEvent{EventType: "StatusEscolarAtualizado", AggregateID: e.ID},
		NovoStatus: novoStatus,
		UpdatedAt:  time.Now(),
	}

	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) AtualizarStatusSuperior(novoStatus string) error {
	validStatus := map[string]bool{"inativo": true, "em_andamento": true, "finalizado": true}
	if !validStatus[novoStatus] {
		return fmt.Errorf("status inválido")
	}

	if (novoStatus == "em_andamento" || novoStatus == "finalizado") && e.StatusEscolar != "finalizado" {
		return fmt.Errorf("status_superior só pode ser atualizado se status_escolar for 'finalizado'")
	}

	event := &StatusSuperiorAtualizadoEvent{
		BaseEvent:  BaseEvent{EventType: "StatusSuperiorAtualizado", AggregateID: e.ID},
		NovoStatus: novoStatus,
		UpdatedAt:  time.Now(),
	}

	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) AtualizarDadosPessoais(nome *string, email *string, telefone *string, bilheteIdentidade *string, bilheteIdentidadeResp *string) error {
	if nome == nil && email == nil && telefone == nil && bilheteIdentidade == nil && bilheteIdentidadeResp == nil {
		return fmt.Errorf("nenhum campo para atualizar")
	}

	emailAlterado := email != nil && (e.Email == nil || *e.Email != *email)

	event := &DadosPessoaisAtualizadosEvent{
		BaseEvent:             BaseEvent{EventType: "DadosPessoaisAtualizados", AggregateID: e.ID},
		Nome:                  nome,
		Email:                 email,
		Telefone:              telefone,
		BilheteIdentidade:     bilheteIdentidade,
		BilheteIdentidadeResp: bilheteIdentidadeResp,
		EmailAlterado:         emailAlterado,
		UpdatedAt:             time.Now(),
	}

	e.RaiseEvent(event)
	return e.Apply(event)
}

// 🔥 ATUALIZADO
func (e *Estudante) AtualizarDadosAcademicos(anoEscolar *string, anoSuperior *string, cursoMedioID *uuid.UUID, cursoSuperiorID *uuid.UUID) error {
	if anoEscolar == nil && anoSuperior == nil && cursoMedioID == nil && cursoSuperiorID == nil {
		return fmt.Errorf("nenhum campo para atualizar")
	}

	event := &DadosAcademicosAtualizadosEvent{
		BaseEvent:       BaseEvent{EventType: "DadosAcademicosAtualizados", AggregateID: e.ID},
		AnoEscolar:      anoEscolar,
		AnoSuperior:     anoSuperior,
		CursoMedioID:    cursoMedioID,    // 🔥 MUDOU
		CursoSuperiorID: cursoSuperiorID, // 🔥 MUDOU
		UpdatedAt:       time.Now(),
	}

	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) AlterarCurso(tipoEnsino string, cursoID uuid.UUID) error {
	if e.CodigoAcademia == nil {
		return fmt.Errorf("estudante não está vinculado a nenhuma academia")
	}

	if tipoEnsino != "medio" && tipoEnsino != "superior" {
		return fmt.Errorf("tipo de ensino deve ser 'medio' ou 'superior'")
	}

	if cursoID == uuid.Nil {
		return fmt.Errorf("curso_id inválido")
	}

	// Validar se pode alterar curso baseado no status
	if tipoEnsino == "medio" {
		if e.StatusEscolar != "em_andamento" {
			return fmt.Errorf("só pode alterar curso médio se status_escolar for 'em_andamento'")
		}
	} else {
		if e.StatusSuperior != "em_andamento" {
			return fmt.Errorf("só pode alterar curso superior se status_superior for 'em_andamento'")
		}
	}

	event := &CursoAlteradoEvent{
		BaseEvent:   BaseEvent{EventType: "CursoAlterado", AggregateID: e.ID},
		TipoEnsino:  tipoEnsino,
		CursoID:     cursoID,
		AlteredAt:   time.Now(),
	}

	e.RaiseEvent(event)
	return e.Apply(event)
}

// Event Handlers
func (e *Estudante) applyEstudanteCriado(event DomainEvent) error {
	payload := event.GetPayload()
	data, _ := json.Marshal(payload)
	var ev EstudanteCriadoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	e.ID = event.GetAggregateID()
	e.Nome = ev.Nome
	e.CodigoEstudante = ev.CodigoEstudante
	e.SenhaHash = ev.SenhaHash
	e.Email = ev.Email
	e.Telefone = ev.Telefone
	e.EmailVerificado = false
	e.BilheteIdentidade = ev.BilheteIdentidade
	e.BilheteIdentidadeResp = ev.BilheteIdentidadeResp
	e.AnoEscolar = ev.AnoEscolar
	e.AnoSuperior = ev.AnoSuperior
	e.CursoMedioID = ev.CursoMedioID       // 🔥 MUDOU
	e.CursoSuperiorID = ev.CursoSuperiorID // 🔥 MUDOU
	e.Status = "inativo"
	e.StatusEscolar = ev.StatusEscolar
	e.StatusSuperior = ev.StatusSuperior
	e.CreatedAt = ev.CreatedAt
	return nil
}

func (e *Estudante) applyEstudanteCriadoComVinculo(event DomainEvent) error {
	payload := event.GetPayload()
	data, _ := json.Marshal(payload)
	var ev EstudanteCriadoComVinculoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	e.ID = event.GetAggregateID()
	e.Nome = ev.Nome
	e.CodigoEstudante = ev.CodigoEstudante
	e.SenhaHash = ev.SenhaHash
	e.Email = ev.Email
	e.Telefone = ev.Telefone
	e.EmailVerificado = false
	e.BilheteIdentidade = ev.BilheteIdentidade
	e.BilheteIdentidadeResp = ev.BilheteIdentidadeResp
	e.AnoEscolar = ev.AnoEscolar
	e.AnoSuperior = ev.AnoSuperior
	e.CursoMedioID = ev.CursoMedioID
	e.CursoSuperiorID = ev.CursoSuperiorID
	e.CodigoAcademia = &ev.CodigoAcademia // 🔥 JÁ VINCULADO
	e.Status = "ativo"                     // 🔥 JÁ ATIVO
	e.StatusEscolar = ev.StatusEscolar
	e.StatusSuperior = ev.StatusSuperior
	e.CreatedAt = ev.CreatedAt
	return nil
}

func (e *Estudante) applyEmailVerificado(event DomainEvent) error {
	e.EmailVerificado = true
	return nil
}

func (e *Estudante) applyFaltasRegistradas(event DomainEvent) error {
	return nil
}

func (e *Estudante) applyAprovacaoAnoRegistrada(event DomainEvent) error {
	payload := event.GetPayload()
	data, _ := json.Marshal(payload)
	var ev AprovacaoAnoRegistradaEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	if ev.AvancarAno && ev.NivelSeguinte != nil {
		if strings.Contains(ev.NivelAtual, "medio") || strings.Contains(ev.NivelAtual, "fundamental") {
			e.AnoEscolar = ev.NivelSeguinte
			
			if ev.NivelAtual == "quarto_medio" {
				e.StatusEscolar = "finalizado"
			}
		} else {
			e.AnoSuperior = ev.NivelSeguinte
			
			if ev.NivelAtual == "sexto_ano" {
				e.StatusSuperior = "finalizado"
			}
		}
	}

	return nil
}

func (e *Estudante) applyEstudanteInscrito(event DomainEvent) error {
	payload := event.GetPayload()
	data, _ := json.Marshal(payload)
	var ev EstudanteInscritoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	e.Inscricoes = append(e.Inscricoes, Inscricao{
		ID:             ev.InscricaoID,
		CodigoAcademia: ev.CodigoAcademia,
		Tipo:           ev.Tipo,
		AnoInscricao:   ev.AnoInscricao,
		CursoID:        ev.CursoID, // 🔥 MUDOU
		Status:         "espera",
		StatusUsado:    false,
		CreatedAt:      ev.CreatedAt,
	})
	return nil
}

func (e *Estudante) applyInscricaoAprovada(event DomainEvent) error {
	payload := event.GetPayload()
	data, _ := json.Marshal(payload)
	var ev InscricaoAprovadaEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	for i := range e.Inscricoes {
		if e.Inscricoes[i].CodigoAcademia == ev.CodigoAcademia && e.Inscricoes[i].Status == "espera" {
			e.Inscricoes[i].Status = "aprovado"
			break
		}
	}
	return nil
}

func (e *Estudante) applyInscricaoReprovada(event DomainEvent) error {
	payload := event.GetPayload()
	data, _ := json.Marshal(payload)
	var ev InscricaoReprovadaEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	for i := range e.Inscricoes {
		if e.Inscricoes[i].CodigoAcademia == ev.CodigoAcademia && e.Inscricoes[i].Status == "espera" {
			e.Inscricoes[i].Status = "reprovado"
			break
		}
	}
	return nil
}

func (e *Estudante) applyEstudanteVinculado(event DomainEvent) error {
	payload := event.GetPayload()
	data, _ := json.Marshal(payload)
	var ev EstudanteVinculadoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	e.CodigoAcademia = &ev.CodigoAcademia
	e.Status = "ativo"

	for i := range e.Inscricoes {
		if e.Inscricoes[i].ID == ev.InscricaoID {
			e.Inscricoes[i].StatusUsado = true
			break
		}
	}
	return nil
}

func (e *Estudante) applyStatusEscolarAtualizado(event DomainEvent) error {
	payload := event.GetPayload()
	data, _ := json.Marshal(payload)
	var ev StatusEscolarAtualizadoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	e.StatusEscolar = ev.NovoStatus
	if ev.NovoStatus == "inativo" {
		e.StatusSuperior = "inativo"
	}
	return nil
}

func (e *Estudante) applyStatusSuperiorAtualizado(event DomainEvent) error {
	payload := event.GetPayload()
	data, _ := json.Marshal(payload)
	var ev StatusSuperiorAtualizadoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	e.StatusSuperior = ev.NovoStatus
	return nil
}

func (e *Estudante) applyDadosPessoaisAtualizados(event DomainEvent) error {
	payload := event.GetPayload()
	data, _ := json.Marshal(payload)
	var ev DadosPessoaisAtualizadosEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	if ev.Nome != nil {
		e.Nome = *ev.Nome
	}
	if ev.Email != nil {
		e.Email = ev.Email
		if ev.EmailAlterado {
			e.EmailVerificado = false
		}
	}
	if ev.Telefone != nil {
		e.Telefone = ev.Telefone
	}
	if ev.BilheteIdentidade != nil {
		e.BilheteIdentidade = ev.BilheteIdentidade
	}
	if ev.BilheteIdentidadeResp != nil {
		e.BilheteIdentidadeResp = ev.BilheteIdentidadeResp
	}
	return nil
}

func (e *Estudante) applyDadosAcademicosAtualizados(event DomainEvent) error {
	payload := event.GetPayload()
	data, _ := json.Marshal(payload)
	var ev DadosAcademicosAtualizadosEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	if ev.AnoEscolar != nil {
		e.AnoEscolar = ev.AnoEscolar
	}
	if ev.AnoSuperior != nil {
		e.AnoSuperior = ev.AnoSuperior
	}
	if ev.CursoMedioID != nil {    // 🔥 MUDOU
		e.CursoMedioID = ev.CursoMedioID
	}
	if ev.CursoSuperiorID != nil { // 🔥 MUDOU
		e.CursoSuperiorID = ev.CursoSuperiorID
	}
	return nil
}

func (e *Estudante) applyCursoAlterado(event DomainEvent) error {
	payload := event.GetPayload()
	data, _ := json.Marshal(payload)
	var ev CursoAlteradoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	if ev.TipoEnsino == "medio" {
		e.CursoMedioID = &ev.CursoID
	} else {
		e.CursoSuperiorID = &ev.CursoID
	}
	
	return nil
}

func (e *Estudante) validarCursoComAno(cursoID uuid.UUID, ano string, tipo string, cursosProj interface{}) error {
	// Esta validação deve ser feita no handler, pois precisa acessar a projeção de cursos
	// Ver correção em auth_handlers.go
	return nil
}

func getProximoNivel(nivelAtual, tipo string) string {
	niveisEscolar := []string{
		"primeiro_fundamental", "segundo_fundamental", "terceiro_fundamental",
		"quarto_fundamental", "quinto_fundamental", "sexto_fundamental",
		"setimo_fundamental", "oitavo_fundamental", "nono_fundamental",
		"primeiro_medio", "segundo_medio", "terceiro_medio", "quarto_medio",
	}
	
	niveisSuperior := []string{
		"primeiro_ano", "segundo_ano", "terceiro_ano",
		"quarto_ano", "quinto_ano", "sexto_ano",
	}

	var niveis []string
	if tipo == "escolar" {
		niveis = niveisEscolar
	} else {
		niveis = niveisSuperior
	}

	for i, n := range niveis {
		if n == nivelAtual && i < len(niveis)-1 {
			return niveis[i+1]
		}
	}

	return ""
}

// 🔥 EVENTOS ATUALIZADOS

type EstudanteCriadoEvent struct {
	BaseEvent
	Nome                  string
	CodigoEstudante       string
	SenhaHash             string
	Email                 *string
	Telefone              *string
	BilheteIdentidade     *string
	BilheteIdentidadeResp *string
	AnoEscolar            *string
	AnoSuperior           *string
	CursoMedioID          *uuid.UUID // 🔥 MUDOU
	CursoSuperiorID       *uuid.UUID // 🔥 MUDOU
	StatusEscolar         string
	StatusSuperior        string
	CreatedAt             time.Time
}

func (e *EstudanteCriadoEvent) GetPayload() interface{} { return e }

type EstudanteCriadoComVinculoEvent struct {
	BaseEvent
	Nome                  string
	CodigoEstudante       string
	SenhaHash             string
	Email                 *string
	Telefone              *string
	BilheteIdentidade     *string
	BilheteIdentidadeResp *string
	AnoEscolar            *string    // 🔥 IMPORTANTE: ponteiro
	AnoSuperior           *string    // 🔥 IMPORTANTE: ponteiro
	CursoMedioID          *uuid.UUID // 🔥 IMPORTANTE: ponteiro
	CursoSuperiorID       *uuid.UUID // 🔥 IMPORTANTE: ponteiro
	StatusEscolar         string
	StatusSuperior        string
	CodigoAcademia        string // 🔥 DIFERENÇA
	CreatedAt             time.Time
}

func (e *EstudanteCriadoComVinculoEvent) GetPayload() interface{} { return e }

type FaltasRegistradasEvent struct {
	BaseEvent
	CodigoEstudante      string
	CodigoAcademia       string
	AnoLectivo           string
	Data                 time.Time
	MateriaDisciplinarID uuid.UUID
	Quantidade           int
	Observacao           *string
	RegisteredAt         time.Time
}

func (e *FaltasRegistradasEvent) GetPayload() interface{} { return e }

type AprovacaoAnoRegistradaEvent struct {
	BaseEvent
	CodigoEstudante string
	CodigoAcademia  string
	AnoLectivo      string
	NivelAtual      string
	NivelSeguinte   *string
	AvancarAno      bool
	Observacao      *string
	RegisteredAt    time.Time
}

func (e *AprovacaoAnoRegistradaEvent) GetPayload() interface{} { return e }

type EstudanteInscritoEvent struct {
	BaseEvent
	InscricaoID    uuid.UUID
	CodigoAcademia string
	Tipo           string
	AnoInscricao   string
	CursoID        *uuid.UUID // 🔥 MUDOU
	CreatedAt      time.Time
}

func (e *EstudanteInscritoEvent) GetPayload() interface{} { return e }

type InscricaoAprovadaEvent struct {
	BaseEvent
	InscricaoID    uuid.UUID
	CodigoAcademia string
	Tipo           string
	AnoInscricao   string
	CursoID        *uuid.UUID // 🔥 MUDOU
}

func (e *InscricaoAprovadaEvent) GetPayload() interface{} { return e }

type InscricaoReprovadaEvent struct {
	BaseEvent
	InscricaoID    uuid.UUID
	CodigoAcademia string
}

func (e *InscricaoReprovadaEvent) GetPayload() interface{} { return e }

type EstudanteVinculadoEvent struct {
	BaseEvent
	InscricaoID    uuid.UUID
	CodigoAcademia string
	VinculadoAt    time.Time
}

func (e *EstudanteVinculadoEvent) GetPayload() interface{} { return e }

type StatusEscolarAtualizadoEvent struct {
	BaseEvent
	NovoStatus string
	UpdatedAt  time.Time
}

func (e *StatusEscolarAtualizadoEvent) GetPayload() interface{} { return e }

type StatusSuperiorAtualizadoEvent struct {
	BaseEvent
	NovoStatus string
	UpdatedAt  time.Time
}

func (e *StatusSuperiorAtualizadoEvent) GetPayload() interface{} { return e }

type DadosPessoaisAtualizadosEvent struct {
	BaseEvent
	Nome                  *string
	Email                 *string
	Telefone              *string
	BilheteIdentidade     *string
	BilheteIdentidadeResp *string
	EmailAlterado         bool
	UpdatedAt             time.Time
}

func (e *DadosPessoaisAtualizadosEvent) GetPayload() interface{} { return e }

type DadosAcademicosAtualizadosEvent struct {
	BaseEvent
	AnoEscolar      *string
	AnoSuperior     *string
	CursoMedioID    *uuid.UUID // 🔥 MUDOU
	CursoSuperiorID *uuid.UUID // 🔥 MUDOU
	UpdatedAt       time.Time
}

type CursoAlteradoEvent struct {
	BaseEvent
	TipoEnsino string
	CursoID    uuid.UUID
	AlteredAt  time.Time
}

func (e *DadosAcademicosAtualizadosEvent) GetPayload() interface{} { return e }