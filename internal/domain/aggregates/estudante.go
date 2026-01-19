package aggregates

import (
	"encoding/json"
	"fmt"
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
	if nome == "" || codigoEstudante == "" || senhaHash == "" {
		return fmt.Errorf("nome, codigo_estudante e senha são obrigatórios")
	}
	
	if bilhete == nil && bilheteResp == nil {
		return fmt.Errorf("pelo menos um bilhete de identidade é obrigatório")
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
	
	if statusSup == "em_andamento" && statusEsc != "finalizado" {
		return fmt.Errorf("status_superior só pode ser 'em_andamento' se status_escolar for 'finalizado'")
	}
	if statusSup == "finalizado" && statusEsc != "finalizado" {
		return fmt.Errorf("status_superior só pode ser 'finalizado' se status_escolar for 'finalizado'")
	}
	
	if statusEsc == "em_andamento" && statusSup == "em_andamento" {
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

	e.RaiseEvent(event)
	return e.Apply(event)
}

// 🔥 NOVO v3.0: Registrar nota individual
func (e *Estudante) RegistrarNota(
	codigoAcademia string,
	anoLectivo string,
	periodo string,
	materiaDisciplinarID uuid.UUID,
	nota float64,
	observacao *string,
) error {
	if e.CodigoAcademia == nil || *e.CodigoAcademia != codigoAcademia {
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
		return fmt.Errorf("período inválido: %s", periodo)
	}
	
	if nota < 0 || nota > 20 {
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

	e.RaiseEvent(event)
	return e.Apply(event)
}

// 🔥 NOVO v3.0: Registrar falta individual
func (e *Estudante) RegistrarFalta(
	codigoAcademia string,
	anoLectivo string,
	data time.Time,
	materiaDisciplinarID uuid.UUID,
	quantidade int,
	observacao *string,
) error {
	if e.CodigoAcademia == nil || *e.CodigoAcademia != codigoAcademia {
		return fmt.Errorf("estudante não pertence a esta academia")
	}
	
	if quantidade <= 0 {
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

	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) SolicitarInscricao(
	codigoAcademia string,
	tipo string,
	anoInscricao string,
	curso *string,
) error {
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

	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) AprovarInscricao(
	codigoAcademia string,
	inscricaoID uuid.UUID,
) error {
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

	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) ReprovarInscricao(
	codigoAcademia string,
	inscricaoID uuid.UUID,
) error {
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
		BaseEvent: BaseEvent{
			EventType:   "InscricaoReprovada",
			AggregateID: e.ID,
		},
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
		BaseEvent: BaseEvent{
			EventType:   "EstudanteVinculado",
			AggregateID: e.ID,
		},
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
	validStatus := map[string]bool{"inativo": true, "em_andamento": true, "finalizado": true}
	if !validStatus[novoStatus] {
		return fmt.Errorf("status inválido")
	}

	if (novoStatus == "em_andamento" || novoStatus == "finalizado") && e.StatusEscolar != "finalizado" {
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
	if e.Status != "ativo" && e.Status != "inativo" {
		return fmt.Errorf("não é possível atualizar dados de estudante finalizado")
	}

	if nome == nil && email == nil && telefone == nil && bilheteIdentidade == nil && bilheteIdentidadeResp == nil {
		return fmt.Errorf("nenhum campo para atualizar")
	}

	if nome != nil && *nome == "" {
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

	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) AtualizarDadosAcademicos(
	anoEscolar *string,
	anoSuperior *string,
	cursoMedio *string,
	cursoSuperior *string,
) error {
	if e.Status != "ativo" && e.Status != "inativo" {
		return fmt.Errorf("não é possível atualizar dados de estudante finalizado")
	}

	if anoEscolar == nil && anoSuperior == nil && cursoMedio == nil && cursoSuperior == nil {
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

	e.RaiseEvent(event)
	return e.Apply(event)
}

// Event Handlers

func (e *Estudante) applyEstudanteCriado(event DomainEvent) error {
	payload := event.GetPayload()
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

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

	return nil
}

// 🔥 v3.0: Agregado não mantém histórico individual
func (e *Estudante) applyNotasRegistradas(event DomainEvent) error {
	return nil
}

func (e *Estudante) applyFaltasRegistradas(event DomainEvent) error {
	return nil
}

func (e *Estudante) applyEstudanteInscrito(event DomainEvent) error {
	payload := event.GetPayload()
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	var ev EstudanteInscritoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
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

	return nil
}

func (e *Estudante) applyInscricaoAprovada(event DomainEvent) error {
	payload := event.GetPayload()
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

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
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

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
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

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
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

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
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	var ev StatusSuperiorAtualizadoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	e.StatusSuperior = ev.NovoStatus
	return nil
}

func (e *Estudante) applyDadosPessoaisAtualizados(event DomainEvent) error {
	payload := event.GetPayload()
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	var ev DadosPessoaisAtualizadosEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	if ev.Nome != nil {
		e.Nome = *ev.Nome
	}
	if ev.Email != nil {
		e.Email = ev.Email
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
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

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
	if ev.CursoMedio != nil {
		e.CursoMedio = ev.CursoMedio
	}
	if ev.CursoSuperior != nil {
		e.CursoSuperior = ev.CursoSuperior
	}

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

// 🔥 NOVO v3.0
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

// 🔥 NOVO v3.0
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