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
	BilheteIdentidade          *string
	BilheteIdentidadeResp      *string
	CodigoAcademia             *string
	Status                     string  // inativo, ativo, finalizado
	AnoEscolar                 *string
	AnoSuperior                *string
	CursoMedio                 *string
	CursoSuperior              *string
	StatusEscolar              string  // inativo, em_andamento, finalizado
	StatusSuperior             string  // inativo, em_andamento, finalizado
	CreatedAt                  time.Time
	
	Notas                      []RegistroNotas
	Faltas                     []RegistroFaltas
	Inscricoes                 []Inscricao
}

type RegistroNotas struct {
	CodigoAcademia string
	AnoLectivo     string
	Periodo        string
	Materias       []Materia
	RegisteredAt   time.Time
}

type RegistroFaltas struct {
	CodigoAcademia string
	AnoLectivo     string
	Periodo        string
	Materias       []MateriaFaltas
	RegisteredAt   time.Time
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

type Materia struct {
	Nome string
	Nota float64
}

type MateriaFaltas struct {
	Nome   string
	Faltas int
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
		Notas:          []RegistroNotas{},
		Faltas:         []RegistroFaltas{},
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
	default:
		return fmt.Errorf("tipo de evento desconhecido: %s", event.GetEventType())
	}
}

// Comandos

func (e *Estudante) Criar(
	nome string,
	codigoEstudante string,
	senhaHash string,
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
	
	// 🔥 VALIDAÇÃO: Pelo menos um bilhete obrigatório
	if bilhete == nil && bilheteResp == nil {
		return fmt.Errorf("pelo menos um bilhete de identidade é obrigatório")
	}

	// 🔥 VALIDAÇÃO: Status escolar/superior
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
	
	// 🔥 REGRA: Superior só pode estar ativo se escolar finalizado
	if statusSup == "em_andamento" && statusEsc != "finalizado" {
		return fmt.Errorf("status_superior só pode ser 'em_andamento' se status_escolar for 'finalizado'")
	}
	if statusSup == "finalizado" && statusEsc != "finalizado" {
		return fmt.Errorf("status_superior só pode ser 'finalizado' se status_escolar for 'finalizado'")
	}
	
	// 🔥 REGRA: Nunca ambos em andamento
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

func (e *Estudante) RegistrarNotas(
	codigoAcademia string,
	anoLectivo string,
	periodo string,
	materias []Materia,
) error {
	if e.CodigoAcademia == nil || *e.CodigoAcademia != codigoAcademia {
		return fmt.Errorf("estudante não pertence a esta academia")
	}
	if len(materias) == 0 {
		return fmt.Errorf("materias não pode estar vazio")
	}

	event := &NotasRegistradasEvent{
		BaseEvent: BaseEvent{
			EventType:   "NotasRegistradas",
			AggregateID: e.ID,
		},
		CodigoAcademia: codigoAcademia,
		AnoLectivo:     anoLectivo,
		Periodo:        periodo,
		Materias:       materias,
		RegisteredAt:   time.Now(),
	}

	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) RegistrarFaltas(
	codigoAcademia string,
	anoLectivo string,
	periodo string,
	materias []MateriaFaltas,
) error {
	if e.CodigoAcademia == nil || *e.CodigoAcademia != codigoAcademia {
		return fmt.Errorf("estudante não pertence a esta academia")
	}
	if len(materias) == 0 {
		return fmt.Errorf("materias não pode estar vazio")
	}

	event := &FaltasRegistradasEvent{
		BaseEvent: BaseEvent{
			EventType:   "FaltasRegistradas",
			AggregateID: e.ID,
		},
		CodigoAcademia: codigoAcademia,
		AnoLectivo:     anoLectivo,
		Periodo:        periodo,
		Materias:       materias,
		RegisteredAt:   time.Now(),
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

	// 🔥 VALIDAÇÃO: Não pode se inscrever se já vinculado
	if e.CodigoAcademia != nil && *e.CodigoAcademia == codigoAcademia {
		return fmt.Errorf("você já está matriculado nesta academia")
	}

	// 🔥 VALIDAÇÃO: Não pode ter inscrição pendente
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

// 🔥 NOVO: Vincular estudante à academia usando inscrição aprovada
func (e *Estudante) VincularAcademia(inscricaoID uuid.UUID) error {
	// Buscar inscrição aprovada não usada
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

	// 🔥 Estudante torna-se ATIVO ao vincular
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

// 🔥 NOVO: Atualizar status escolar
func (e *Estudante) AtualizarStatusEscolar(novoStatus string) error {
	validStatus := map[string]bool{"inativo": true, "em_andamento": true, "finalizado": true}
	if !validStatus[novoStatus] {
		return fmt.Errorf("status inválido")
	}

	// 🔥 REGRA: Se mudar escolar para inativo, superior também deve ficar inativo
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

// 🔥 NOVO: Atualizar status superior
func (e *Estudante) AtualizarStatusSuperior(novoStatus string) error {
	validStatus := map[string]bool{"inativo": true, "em_andamento": true, "finalizado": true}
	if !validStatus[novoStatus] {
		return fmt.Errorf("status inválido")
	}

	// 🔥 REGRA: Superior só pode estar ativo se escolar finalizado
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
	e.BilheteIdentidade = ev.BilheteIdentidade
	e.BilheteIdentidadeResp = ev.BilheteIdentidadeResp
	e.AnoEscolar = ev.AnoEscolar
	e.AnoSuperior = ev.AnoSuperior
	e.CursoMedio = ev.CursoMedio
	e.CursoSuperior = ev.CursoSuperior
	e.Status = "inativo" // 🔥 Sempre inativo ao criar
	e.StatusEscolar = ev.StatusEscolar
	e.StatusSuperior = ev.StatusSuperior
	e.CreatedAt = ev.CreatedAt

	return nil
}

func (e *Estudante) applyNotasRegistradas(event DomainEvent) error {
	payload := event.GetPayload()
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	var ev NotasRegistradasEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	e.Notas = append(e.Notas, RegistroNotas{
		CodigoAcademia: ev.CodigoAcademia,
		AnoLectivo:     ev.AnoLectivo,
		Periodo:        ev.Periodo,
		Materias:       ev.Materias,
		RegisteredAt:   ev.RegisteredAt,
	})

	return nil
}

func (e *Estudante) applyFaltasRegistradas(event DomainEvent) error {
	payload := event.GetPayload()
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	var ev FaltasRegistradasEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	e.Faltas = append(e.Faltas, RegistroFaltas{
		CodigoAcademia: ev.CodigoAcademia,
		AnoLectivo:     ev.AnoLectivo,
		Periodo:        ev.Periodo,
		Materias:       ev.Materias,
		RegisteredAt:   ev.RegisteredAt,
	})

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

	// Atualizar status da inscrição para aprovado (NÃO vincular ainda)
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

// 🔥 NOVO
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

	// Vincular à academia
	e.CodigoAcademia = &ev.CodigoAcademia
	
	// 🔥 Estudante torna-se ATIVO
	e.Status = "ativo"

	// Marcar inscrição como usada
	for i := range e.Inscricoes {
		if e.Inscricoes[i].ID == ev.InscricaoID {
			e.Inscricoes[i].StatusUsado = true
			break
		}
	}

	return nil
}

// 🔥 NOVO
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
	
	// 🔥 Se escolar vira inativo, superior também
	if ev.NovoStatus == "inativo" {
		e.StatusSuperior = "inativo"
	}

	return nil
}

// 🔥 NOVO
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

// Eventos

type EstudanteCriadoEvent struct {
	BaseEvent
	Nome                  string
	CodigoEstudante       string
	SenhaHash             string
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
	CodigoAcademia string
	AnoLectivo     string
	Periodo        string
	Materias       []Materia
	RegisteredAt   time.Time
}

func (e *NotasRegistradasEvent) GetPayload() interface{} {
	return e
}

type FaltasRegistradasEvent struct {
	BaseEvent
	CodigoAcademia string
	AnoLectivo     string
	Periodo        string
	Materias       []MateriaFaltas
	RegisteredAt   time.Time
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

// 🔥 NOVO
type EstudanteVinculadoEvent struct {
	BaseEvent
	InscricaoID    uuid.UUID
	CodigoAcademia string
	VinculadoAt    time.Time
}

func (e *EstudanteVinculadoEvent) GetPayload() interface{} {
	return e
}

// 🔥 NOVO
type StatusEscolarAtualizadoEvent struct {
	BaseEvent
	NovoStatus string
	UpdatedAt  time.Time
}

func (e *StatusEscolarAtualizadoEvent) GetPayload() interface{} {
	return e
}

// 🔥 NOVO
type StatusSuperiorAtualizadoEvent struct {
	BaseEvent
	NovoStatus string
	UpdatedAt  time.Time
}

func (e *StatusSuperiorAtualizadoEvent) GetPayload() interface{} {
	return e
}