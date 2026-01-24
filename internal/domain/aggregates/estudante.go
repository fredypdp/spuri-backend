package aggregates

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
)

type Estudante struct {
	BaseAggregate
	
	Nome                       string
	CodigoEstudante            string
	SenhaHash                  string
	Email                      *string
	Telefone                   *string
	BilheteIdentidade          *string
	BilheteIdentidadeResp      *string
	CodigoAcademia             *string
	Status                     string
	AnoEscolar                 *string
	AnoSuperior                *string
	CursoMedio                 *string
	CursoSuperior              *string
	StatusEscolar              string
	StatusSuperior             string
	CreatedAt                  time.Time
	
	Inscricoes                 []Inscricao
}

type Inscricao struct {
	ID             uuid.UUID
	CodigoAcademia string
	Tipo           string
	AnoInscricao   string
	Curso          *string
	Status         string
	StatusUsado    bool
	CreatedAt      time.Time
}

func NewEstudante() *Estudante {
	log.Printf("[DEBUG] Criando novo agregado Estudante")
	return &Estudante{
		BaseAggregate: BaseAggregate{
			ID:                uuid.New(),
			Version:           0,
			UncommittedEvents: []DomainEvent{},
		},
		Status:         "inativo",
		StatusEscolar:  "inativo",
		StatusSuperior: "inativo",
		Inscricoes:     []Inscricao{},
	}
}

func (e *Estudante) GetType() string {
	return "Estudante"
}

func (e *Estudante) Apply(event DomainEvent) error {
	log.Printf("[DEBUG] Aplicando evento %s ao Estudante %s", event.GetEventType(), e.ID)
	
	switch event.GetEventType() {
	case "EstudanteCriado":
		return e.applyEstudanteCriado(event)
	case "NotasRegistradas":
		return e.applyNotasRegistradas(event)
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
	default:
		log.Printf("[ERROR] Tipo de evento desconhecido: %s", event.GetEventType())
		return fmt.Errorf("tipo de evento desconhecido: %s", event.GetEventType())
	}
}

// Comandos

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
	cursoMedio *string,
	cursoSuperior *string,
	statusEscolar *string,
	statusSuperior *string,
) error {
	log.Printf("[DEBUG] Criando estudante: nome=%s, codigo=%s", nome, codigoEstudante)
	
	if nome == "" || codigoEstudante == "" || senhaHash == "" {
		log.Printf("[ERROR] Campos obrigatórios faltando")
		return fmt.Errorf("nome, codigo_estudante e senha são obrigatórios")
	}
	
	if bilhete == nil && bilheteResp == nil {
		log.Printf("[ERROR] Nenhum bilhete de identidade fornecido")
		return fmt.Errorf("pelo menos um bilhete de identidade é obrigatório")
	}

	statusEsc := "inativo"
	statusSup := "inativo"
	
	if statusEscolar != nil {
		if *statusEscolar != "inativo" && *statusEscolar != "em_andamento" && *statusEscolar != "finalizado" {
			log.Printf("[ERROR] Status escolar inválido: %s", *statusEscolar)
			return fmt.Errorf("status_escolar inválido")
		}
		statusEsc = *statusEscolar
	}
	
	if statusSuperior != nil {
		if *statusSuperior != "inativo" && *statusSuperior != "em_andamento" && *statusSuperior != "finalizado" {
			log.Printf("[ERROR] Status superior inválido: %s", *statusSuperior)
			return fmt.Errorf("status_superior inválido")
		}
		statusSup = *statusSuperior
	}
	
	if statusSup == "em_andamento" && statusEsc != "finalizado" {
		log.Printf("[ERROR] Status superior em_andamento sem status escolar finalizado")
		return fmt.Errorf("status_superior só pode ser 'em_andamento' se status_escolar for 'finalizado'")
	}
	if statusSup == "finalizado" && statusEsc != "finalizado" {
		log.Printf("[ERROR] Status superior finalizado sem status escolar finalizado")
		return fmt.Errorf("status_superior só pode ser 'finalizado' se status_escolar for 'finalizado'")
	}
	
	if statusEsc == "em_andamento" && statusSup == "em_andamento" {
		log.Printf("[ERROR] Ambos status em_andamento simultaneamente")
		return fmt.Errorf("status_escolar e status_superior não podem estar ambos 'em_andamento'")
	}

	event := &EstudanteCriadoEvent{
		BaseEvent: BaseEvent{
			EventType:   "EstudanteCriado",
			AggregateID: e.ID,
		},
		Nome:                  nome,
		CodigoEstudante:       codigoEstudante,
		SenhaHash:             senhaHash,
		Email:                 email,
		Telefone:              telefone,
		BilheteIdentidade:     bilhete,
		BilheteIdentidadeResp: bilheteResp,
		AnoEscolar:            anoEscolar,
		AnoSuperior:           anoSuperior,
		CursoMedio:            cursoMedio,
		CursoSuperior:         cursoSuperior,
		StatusEscolar:         statusEsc,
		StatusSuperior:        statusSup,
		CreatedAt:             time.Now(),
	}

	log.Printf("[DEBUG] Evento EstudanteCriado criado para estudante %s", e.ID)
	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) RegistrarNota(
	codigoAcademia string,
	anoLectivo string,
	periodo string,
	materiaDisciplinarID uuid.UUID,
	nota float64,
	observacao *string,
) error {
	log.Printf("[DEBUG] Registrando nota: estudante=%s, materia=%s, nota=%.2f", 
		e.CodigoEstudante, materiaDisciplinarID, nota)
	
	if e.CodigoAcademia == nil || *e.CodigoAcademia != codigoAcademia {
		log.Printf("[ERROR] Estudante não pertence a esta academia")
		return fmt.Errorf("estudante não pertence a esta academia")
	}
	
	periodosValidos := []string{
		"1_trimestre", "2_trimestre", "3_trimestre",
		"1_semestre", "2_semestre",
	}
	periodoValido := false
	for _, p := range periodosValidos {
		if periodo == p {
			periodoValido = true
			break
		}
	}
	if !periodoValido {
		log.Printf("[ERROR] Período inválido: %s", periodo)
		return fmt.Errorf("período inválido: %s", periodo)
	}
	
	if nota < 0 || nota > 20 {
		log.Printf("[ERROR] Nota fora do intervalo válido: %.2f", nota)
		return fmt.Errorf("nota deve estar entre 0 e 20")
	}

	event := &NotasRegistradasEvent{
		BaseEvent: BaseEvent{
			EventType:   "NotasRegistradas",
			AggregateID: e.ID,
		},
		CodigoEstudante:      e.CodigoEstudante,
		CodigoAcademia:       codigoAcademia,
		AnoLectivo:           anoLectivo,
		Periodo:              periodo,
		MateriaDisciplinarID: materiaDisciplinarID,
		Nota:                 nota,
		Observacao:           observacao,
		RegisteredAt:         time.Now(),
	}

	log.Printf("[DEBUG] Evento NotasRegistradas criado")
	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) RegistrarFalta(
	codigoAcademia string,
	anoLectivo string,
	data time.Time,
	materiaDisciplinarID uuid.UUID,
	quantidade int,
	observacao *string,
) error {
	log.Printf("[DEBUG] Registrando falta: estudante=%s, materia=%s, quantidade=%d", 
		e.CodigoEstudante, materiaDisciplinarID, quantidade)
	
	if e.CodigoAcademia == nil || *e.CodigoAcademia != codigoAcademia {
		log.Printf("[ERROR] Estudante não pertence a esta academia")
		return fmt.Errorf("estudante não pertence a esta academia")
	}
	
	if quantidade <= 0 {
		log.Printf("[ERROR] Quantidade inválida: %d", quantidade)
		return fmt.Errorf("quantidade deve ser maior que zero")
	}

	event := &FaltasRegistradasEvent{
		BaseEvent: BaseEvent{
			EventType:   "FaltasRegistradas",
			AggregateID: e.ID,
		},
		CodigoEstudante:      e.CodigoEstudante,
		CodigoAcademia:       codigoAcademia,
		AnoLectivo:           anoLectivo,
		Data:                 data,
		MateriaDisciplinarID: materiaDisciplinarID,
		Quantidade:           quantidade,
		Observacao:           observacao,
		RegisteredAt:         time.Now(),
	}

	log.Printf("[DEBUG] Evento FaltasRegistradas criado")
	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) SolicitarInscricao(
	codigoAcademia string,
	tipo string,
	anoInscricao string,
	curso *string,
) error {
	log.Printf("[DEBUG] Solicitando inscrição: academia=%s, tipo=%s", codigoAcademia, tipo)
	
	if tipo != "escola" && tipo != "universidade" {
		log.Printf("[ERROR] Tipo de inscrição inválido: %s", tipo)
		return fmt.Errorf("tipo deve ser 'escola' ou 'universidade'")
	}

	if e.CodigoAcademia != nil && *e.CodigoAcademia == codigoAcademia {
		log.Printf("[ERROR] Estudante já matriculado nesta academia")
		return fmt.Errorf("você já está matriculado nesta academia")
	}

	for _, inscricao := range e.Inscricoes {
		if inscricao.CodigoAcademia == codigoAcademia && inscricao.Status == "espera" {
			log.Printf("[ERROR] Já existe inscrição pendente para esta academia")
			return fmt.Errorf("você já possui uma inscrição pendente nesta academia")
		}
	}

	inscricaoID := uuid.New()
	event := &EstudanteInscritoEvent{
		BaseEvent: BaseEvent{
			EventType:   "EstudanteInscrito",
			AggregateID: e.ID,
		},
		InscricaoID:    inscricaoID,
		CodigoAcademia: codigoAcademia,
		Tipo:           tipo,
		AnoInscricao:   anoInscricao,
		Curso:          curso,
		CreatedAt:      time.Now(),
	}

	log.Printf("[DEBUG] Evento EstudanteInscrito criado: inscricaoID=%s", inscricaoID)
	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) AprovarInscricao(
	codigoAcademia string,
	inscricaoID uuid.UUID,
) error {
	log.Printf("[DEBUG] Aprovando inscrição: academia=%s, inscricaoID=%s", codigoAcademia, inscricaoID)
	
	var inscricaoPendente *Inscricao
	for i := range e.Inscricoes {
		if e.Inscricoes[i].CodigoAcademia == codigoAcademia && e.Inscricoes[i].Status == "espera" {
			inscricaoPendente = &e.Inscricoes[i]
			break
		}
	}

	if inscricaoPendente == nil {
		log.Printf("[ERROR] Nenhuma inscrição pendente encontrada")
		return fmt.Errorf("nenhuma inscrição pendente encontrada")
	}

	event := &InscricaoAprovadaEvent{
		BaseEvent: BaseEvent{
			EventType:   "InscricaoAprovada",
			AggregateID: e.ID,
		},
		InscricaoID:    inscricaoID,
		CodigoAcademia: codigoAcademia,
		Tipo:           inscricaoPendente.Tipo,
		AnoInscricao:   inscricaoPendente.AnoInscricao,
		Curso:          inscricaoPendente.Curso,
	}

	log.Printf("[DEBUG] Evento InscricaoAprovada criado")
	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) ReprovarInscricao(
	codigoAcademia string,
	inscricaoID uuid.UUID,
) error {
	log.Printf("[DEBUG] Reprovando inscrição: academia=%s, inscricaoID=%s", codigoAcademia, inscricaoID)
	
	var inscricaoPendente *Inscricao
	for i := range e.Inscricoes {
		if e.Inscricoes[i].CodigoAcademia == codigoAcademia && e.Inscricoes[i].Status == "espera" {
			inscricaoPendente = &e.Inscricoes[i]
			break
		}
	}

	if inscricaoPendente == nil {
		log.Printf("[ERROR] Nenhuma inscrição pendente encontrada")
		return fmt.Errorf("nenhuma inscrição pendente encontrada")
	}

	event := &InscricaoReprovadaEvent{
		BaseEvent: BaseEvent{
			EventType:   "InscricaoReprovada",
			AggregateID: e.ID,
		},
		InscricaoID:    inscricaoID,
		CodigoAcademia: codigoAcademia,
	}

	log.Printf("[DEBUG] Evento InscricaoReprovada criado")
	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) VincularAcademia(inscricaoID uuid.UUID) error {
	log.Printf("[DEBUG] Vinculando estudante a academia via inscricaoID=%s", inscricaoID)
	
	var inscricao *Inscricao
	for i := range e.Inscricoes {
		if e.Inscricoes[i].ID == inscricaoID {
			inscricao = &e.Inscricoes[i]
			break
		}
	}

	if inscricao == nil {
		log.Printf("[ERROR] Inscrição não encontrada")
		return fmt.Errorf("inscrição não encontrada")
	}

	if inscricao.Status != "aprovado" {
		log.Printf("[ERROR] Inscrição não foi aprovada: status=%s", inscricao.Status)
		return fmt.Errorf("inscrição não foi aprovada")
	}

	if inscricao.StatusUsado {
		log.Printf("[ERROR] Inscrição já foi utilizada")
		return fmt.Errorf("esta inscrição já foi utilizada")
	}

	event := &EstudanteVinculadoEvent{
		BaseEvent: BaseEvent{
			EventType:   "EstudanteVinculado",
			AggregateID: e.ID,
		},
		InscricaoID:    inscricaoID,
		CodigoAcademia: inscricao.CodigoAcademia,
		VinculadoAt:    time.Now(),
	}

	log.Printf("[DEBUG] Evento EstudanteVinculado criado: academia=%s", inscricao.CodigoAcademia)
	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) AtualizarStatusEscolar(novoStatus string) error {
	log.Printf("[DEBUG] Atualizando status escolar: %s -> %s", e.StatusEscolar, novoStatus)
	
	validStatus := map[string]bool{"inativo": true, "em_andamento": true, "finalizado": true}
	if !validStatus[novoStatus] {
		log.Printf("[ERROR] Status inválido: %s", novoStatus)
		return fmt.Errorf("status inválido")
	}

	if novoStatus == "inativo" && e.StatusSuperior != "inativo" {
		log.Printf("[ERROR] Não pode inativar status escolar com status superior ativo")
		return fmt.Errorf("não pode inativar status_escolar enquanto status_superior está ativo")
	}

	event := &StatusEscolarAtualizadoEvent{
		BaseEvent: BaseEvent{
			EventType:   "StatusEscolarAtualizado",
			AggregateID: e.ID,
		},
		NovoStatus: novoStatus,
		UpdatedAt:  time.Now(),
	}

	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) AtualizarStatusSuperior(novoStatus string) error {
	log.Printf("[DEBUG] Atualizando status superior: %s -> %s", e.StatusSuperior, novoStatus)
	
	validStatus := map[string]bool{"inativo": true, "em_andamento": true, "finalizado": true}
	if !validStatus[novoStatus] {
		log.Printf("[ERROR] Status inválido: %s", novoStatus)
		return fmt.Errorf("status inválido")
	}

	if (novoStatus == "em_andamento" || novoStatus == "finalizado") && e.StatusEscolar != "finalizado" {
		log.Printf("[ERROR] Status superior requer status escolar finalizado")
		return fmt.Errorf("status_superior só pode ser atualizado se status_escolar for 'finalizado'")
	}

	event := &StatusSuperiorAtualizadoEvent{
		BaseEvent: BaseEvent{
			EventType:   "StatusSuperiorAtualizado",
			AggregateID: e.ID,
		},
		NovoStatus: novoStatus,
		UpdatedAt:  time.Now(),
	}

	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) AtualizarDadosPessoais(
	nome *string,
	email *string,
	telefone *string,
	bilheteIdentidade *string,
	bilheteIdentidadeResp *string,
) error {
	log.Printf("[DEBUG] Atualizando dados pessoais do estudante %s", e.ID)
	
	if e.Status != "ativo" && e.Status != "inativo" {
		log.Printf("[ERROR] Estudante com status inválido para atualização: %s", e.Status)
		return fmt.Errorf("não é possível atualizar dados de estudante finalizado")
	}

	if nome == nil && email == nil && telefone == nil && bilheteIdentidade == nil && bilheteIdentidadeResp == nil {
		log.Printf("[ERROR] Nenhum campo para atualizar")
		return fmt.Errorf("nenhum campo para atualizar")
	}

	if nome != nil && *nome == "" {
		log.Printf("[ERROR] Nome não pode ser vazio")
		return fmt.Errorf("nome não pode ser vazio")
	}

	event := &DadosPessoaisAtualizadosEvent{
		BaseEvent: BaseEvent{
			EventType:   "DadosPessoaisAtualizados",
			AggregateID: e.ID,
		},
		Nome:                  nome,
		Email:                 email,
		Telefone:              telefone,
		BilheteIdentidade:     bilheteIdentidade,
		BilheteIdentidadeResp: bilheteIdentidadeResp,
		EmailAlterado:         email != nil && (e.Email == nil || *e.Email != *email),
		UpdatedAt:             time.Now(),
	}

	log.Printf("[DEBUG] Evento DadosPessoaisAtualizados criado")
	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) AtualizarDadosAcademicos(
	anoEscolar *string,
	anoSuperior *string,
	cursoMedio *string,
	cursoSuperior *string,
) error {
	log.Printf("[DEBUG] Atualizando dados acadêmicos do estudante %s", e.ID)
	
	if e.Status != "ativo" && e.Status != "inativo" {
		log.Printf("[ERROR] Estudante com status inválido para atualização: %s", e.Status)
		return fmt.Errorf("não é possível atualizar dados de estudante finalizado")
	}

	if anoEscolar == nil && anoSuperior == nil && cursoMedio == nil && cursoSuperior == nil {
		log.Printf("[ERROR] Nenhum campo para atualizar")
		return fmt.Errorf("nenhum campo para atualizar")
	}

	event := &DadosAcademicosAtualizadosEvent{
		BaseEvent: BaseEvent{
			EventType:   "DadosAcademicosAtualizados",
			AggregateID: e.ID,
		},
		AnoEscolar:    anoEscolar,
		AnoSuperior:   anoSuperior,
		CursoMedio:    cursoMedio,
		CursoSuperior: cursoSuperior,
		UpdatedAt:     time.Now(),
	}

	log.Printf("[DEBUG] Evento DadosAcademicosAtualizados criado")
	e.RaiseEvent(event)
	return e.Apply(event)
}

// Event Handlers

func (e *Estudante) applyEstudanteCriado(event DomainEvent) error {
	log.Printf("[DEBUG] Aplicando EstudanteCriado ao agregado %s", event.GetAggregateID())
	
	payload := event.GetPayload()
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[ERROR] Erro ao serializar payload: %v", err)
		return err
	}

	var ev EstudanteCriadoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		log.Printf("[ERROR] Erro ao deserializar evento: %v", err)
		return err
	}

	e.ID = event.GetAggregateID()
	e.Nome = ev.Nome
	e.CodigoEstudante = ev.CodigoEstudante
	e.SenhaHash = ev.SenhaHash
	e.Email = ev.Email
	e.Telefone = ev.Telefone
	e.BilheteIdentidade = ev.BilheteIdentidade
	e.BilheteIdentidadeResp = ev.BilheteIdentidadeResp
	e.AnoEscolar = ev.AnoEscolar
	e.AnoSuperior = ev.AnoSuperior
	e.CursoMedio = ev.CursoMedio
	e.CursoSuperior = ev.CursoSuperior
	e.Status = "inativo"
	e.StatusEscolar = ev.StatusEscolar
	e.StatusSuperior = ev.StatusSuperior
	e.CreatedAt = ev.CreatedAt

	log.Printf("[DEBUG] Estudante criado: %s (%s)", e.Nome, e.CodigoEstudante)
	return nil
}

func (e *Estudante) applyNotasRegistradas(event DomainEvent) error {
	log.Printf("[DEBUG] Aplicando NotasRegistradas (sem estado) ao agregado %s", event.GetAggregateID())
	return nil
}

func (e *Estudante) applyFaltasRegistradas(event DomainEvent) error {
	log.Printf("[DEBUG] Aplicando FaltasRegistradas (sem estado) ao agregado %s", event.GetAggregateID())
	return nil
}

func (e *Estudante) applyEstudanteInscrito(event DomainEvent) error {
	log.Printf("[DEBUG] Aplicando EstudanteInscrito ao agregado %s", event.GetAggregateID())
	
	payload := event.GetPayload()
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[ERROR] Erro ao serializar payload: %v", err)
		return err
	}

	var ev EstudanteInscritoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		log.Printf("[ERROR] Erro ao deserializar evento: %v", err)
		return err
	}

	e.Inscricoes = append(e.Inscricoes, Inscricao{
		ID:             ev.InscricaoID,
		CodigoAcademia: ev.CodigoAcademia,
		Tipo:           ev.Tipo,
		AnoInscricao:   ev.AnoInscricao,
		Curso:          ev.Curso,
		Status:         "espera",
		StatusUsado:    false,
		CreatedAt:      ev.CreatedAt,
	})

	log.Printf("[DEBUG] Inscrição adicionada: %s (academia: %s)", ev.InscricaoID, ev.CodigoAcademia)
	return nil
}

func (e *Estudante) applyInscricaoAprovada(event DomainEvent) error {
	log.Printf("[DEBUG] Aplicando InscricaoAprovada ao agregado %s", event.GetAggregateID())
	
	payload := event.GetPayload()
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[ERROR] Erro ao serializar payload: %v", err)
		return err
	}

	var ev InscricaoAprovadaEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		log.Printf("[ERROR] Erro ao deserializar evento: %v", err)
		return err
	}

	for i := range e.Inscricoes {
		if e.Inscricoes[i].CodigoAcademia == ev.CodigoAcademia && e.Inscricoes[i].Status == "espera" {
			e.Inscricoes[i].Status = "aprovado"
			log.Printf("[DEBUG] Inscrição aprovada: academia=%s", ev.CodigoAcademia)
			break
		}
	}

	return nil
}

func (e *Estudante) applyInscricaoReprovada(event DomainEvent) error {
	log.Printf("[DEBUG] Aplicando InscricaoReprovada ao agregado %s", event.GetAggregateID())
	
	payload := event.GetPayload()
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[ERROR] Erro ao serializar payload: %v", err)
		return err
	}

	var ev InscricaoReprovadaEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		log.Printf("[ERROR] Erro ao deserializar evento: %v", err)
		return err
	}

	for i := range e.Inscricoes {
		if e.Inscricoes[i].CodigoAcademia == ev.CodigoAcademia && e.Inscricoes[i].Status == "espera" {
			e.Inscricoes[i].Status = "reprovado"
			log.Printf("[DEBUG] Inscrição reprovada: academia=%s", ev.CodigoAcademia)
			break
		}
	}

	return nil
}

func (e *Estudante) applyEstudanteVinculado(event DomainEvent) error {
	log.Printf("[DEBUG] Aplicando EstudanteVinculado ao agregado %s", event.GetAggregateID())
	
	payload := event.GetPayload()
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[ERROR] Erro ao serializar payload: %v", err)
		return err
	}

	var ev EstudanteVinculadoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		log.Printf("[ERROR] Erro ao deserializar evento: %v", err)
		return err
	}

	e.CodigoAcademia = &ev.CodigoAcademia
	e.Status = "ativo"

	for i := range e.Inscricoes {
		if e.Inscricoes[i].ID == ev.InscricaoID {
			e.Inscricoes[i].StatusUsado = true
			log.Printf("[DEBUG] Estudante vinculado à academia: %s", ev.CodigoAcademia)
			break
		}
	}

	return nil
}

func (e *Estudante) applyStatusEscolarAtualizado(event DomainEvent) error {
	log.Printf("[DEBUG] Aplicando StatusEscolarAtualizado ao agregado %s", event.GetAggregateID())
	
	payload := event.GetPayload()
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[ERROR] Erro ao serializar payload: %v", err)
		return err
	}

	var ev StatusEscolarAtualizadoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		log.Printf("[ERROR] Erro ao deserializar evento: %v", err)
		return err
	}

	e.StatusEscolar = ev.NovoStatus
	
	if ev.NovoStatus == "inativo" {
		e.StatusSuperior = "inativo"
		log.Printf("[DEBUG] Status superior também definido como inativo")
	}

	log.Printf("[DEBUG] Status escolar atualizado: %s", ev.NovoStatus)
	return nil
}

func (e *Estudante) applyStatusSuperiorAtualizado(event DomainEvent) error {
	log.Printf("[DEBUG] Aplicando StatusSuperiorAtualizado ao agregado %s", event.GetAggregateID())
	
	payload := event.GetPayload()
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[ERROR] Erro ao serializar payload: %v", err)
		return err
	}

	var ev StatusSuperiorAtualizadoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		log.Printf("[ERROR] Erro ao deserializar evento: %v", err)
		return err
	}

	e.StatusSuperior = ev.NovoStatus
	log.Printf("[DEBUG] Status superior atualizado: %s", ev.NovoStatus)
	return nil
}

func (e *Estudante) applyDadosPessoaisAtualizados(event DomainEvent) error {
	log.Printf("[DEBUG] Aplicando DadosPessoaisAtualizados ao agregado %s", event.GetAggregateID())
	
	payload := event.GetPayload()
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[ERROR] Erro ao serializar payload: %v", err)
		return err
	}

	var ev DadosPessoaisAtualizadosEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		log.Printf("[ERROR] Erro ao deserializar evento: %v", err)
		return err
	}

	if ev.Nome != nil {
		log.Printf("[DEBUG] Atualizando nome: %s -> %s", e.Nome, *ev.Nome)
		e.Nome = *ev.Nome
	}
	if ev.Email != nil {
		log.Printf("[DEBUG] Atualizando email")
		e.Email = ev.Email
	}
	if ev.Telefone != nil {
		log.Printf("[DEBUG] Atualizando telefone")
		e.Telefone = ev.Telefone
	}
	if ev.BilheteIdentidade != nil {
		log.Printf("[DEBUG] Atualizando bilhete de identidade")
		e.BilheteIdentidade = ev.BilheteIdentidade
	}
	if ev.BilheteIdentidadeResp != nil {
		log.Printf("[DEBUG] Atualizando bilhete do responsável")
		e.BilheteIdentidadeResp = ev.BilheteIdentidadeResp
	}

	log.Printf("[DEBUG] Dados pessoais atualizados com sucesso")
	return nil
}

func (e *Estudante) applyDadosAcademicosAtualizados(event DomainEvent) error {
	log.Printf("[DEBUG] Aplicando DadosAcademicosAtualizados ao agregado %s", event.GetAggregateID())
	
	payload := event.GetPayload()
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[ERROR] Erro ao serializar payload: %v", err)
		return err
	}

	var ev DadosAcademicosAtualizadosEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		log.Printf("[ERROR] Erro ao deserializar evento: %v", err)
		return err
	}

	if ev.AnoEscolar != nil {
		log.Printf("[DEBUG] Atualizando ano escolar")
		e.AnoEscolar = ev.AnoEscolar
	}
	if ev.AnoSuperior != nil {
		log.Printf("[DEBUG] Atualizando ano superior")
		e.AnoSuperior = ev.AnoSuperior
	}
	if ev.CursoMedio != nil {
		log.Printf("[DEBUG] Atualizando curso médio")
		e.CursoMedio = ev.CursoMedio
	}
	if ev.CursoSuperior != nil {
		log.Printf("[DEBUG] Atualizando curso superior")
		e.CursoSuperior = ev.CursoSuperior
	}

	log.Printf("[DEBUG] Dados acadêmicos atualizados com sucesso")
	return nil
}

// Eventos

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
	CursoMedio            *string
	CursoSuperior         *string
	StatusEscolar         string
	StatusSuperior        string
	CreatedAt             time.Time
}

func (e *EstudanteCriadoEvent) GetPayload() interface{} {
	return e
}

type NotasRegistradasEvent struct {
	BaseEvent
	CodigoEstudante      string
	CodigoAcademia       string
	AnoLectivo           string
	Periodo              string
	MateriaDisciplinarID uuid.UUID
	Nota                 float64
	Observacao           *string
	RegisteredAt         time.Time
}

func (e *NotasRegistradasEvent) GetPayload() interface{} {
	return e
}

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

func (e *FaltasRegistradasEvent) GetPayload() interface{} {
	return e
}

type EstudanteInscritoEvent struct {
	BaseEvent
	InscricaoID    uuid.UUID
	CodigoAcademia string
	Tipo           string
	AnoInscricao   string
	Curso          *string
	CreatedAt      time.Time
}

func (e *EstudanteInscritoEvent) GetPayload() interface{} {
	return e
}

type InscricaoAprovadaEvent struct {
	BaseEvent
	InscricaoID    uuid.UUID
	CodigoAcademia string
	Tipo           string
	AnoInscricao   string
	Curso          *string
}

func (e *InscricaoAprovadaEvent) GetPayload() interface{} {
	return e
}

type InscricaoReprovadaEvent struct {
	BaseEvent
	InscricaoID    uuid.UUID
	CodigoAcademia string
}

func (e *InscricaoReprovadaEvent) GetPayload() interface{} {
	return e
}

type EstudanteVinculadoEvent struct {
	BaseEvent
	InscricaoID    uuid.UUID
	CodigoAcademia string
	VinculadoAt    time.Time
}

func (e *EstudanteVinculadoEvent) GetPayload() interface{} {
	return e
}

type StatusEscolarAtualizadoEvent struct {
	BaseEvent
	NovoStatus string
	UpdatedAt  time.Time
}

func (e *StatusEscolarAtualizadoEvent) GetPayload() interface{} {
	return e
}

type StatusSuperiorAtualizadoEvent struct {
	BaseEvent
	NovoStatus string
	UpdatedAt  time.Time
}

func (e *StatusSuperiorAtualizadoEvent) GetPayload() interface{} {
	return e
}

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

func (e *DadosPessoaisAtualizadosEvent) GetPayload() interface{} {
	return e
}

type DadosAcademicosAtualizadosEvent struct {
	BaseEvent
	AnoEscolar    *string
	AnoSuperior   *string
	CursoMedio    *string
	CursoSuperior *string
	UpdatedAt     time.Time
}

func (e *DadosAcademicosAtualizadosEvent) GetPayload() interface{} {
	return e
}