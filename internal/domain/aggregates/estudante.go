// ============================================================================
// ARQUIVO: internal/domain/aggregates/estudante.go
// ATUALIZADO: Usar CodigoAcademia em vez de IDAcademia
// ============================================================================

package aggregates

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Estudante agregado raiz
type Estudante struct {
	BaseAggregate
	
	// Estado
	Nome                       string
	CodigoEstudante            string     // Código único (ex: KAF7392)
	SenhaHash                  string
	BilheteIdentidade          *string
	BilheteIdentidadeResp      *string
	CodigoAcademia             *string    // 🔥 MUDOU: string em vez de UUID
	AnoEscolar                 *string
	AnoSuperior                *string
	CursoMedio                 *string
	CursoSuperior              *string
	StatusEscolar              *string
	StatusSuperior             *string
	CreatedAt                  time.Time
	
	// Histórico (reconstruído de eventos)
	Notas                      []RegistroNotas
	Faltas                     []RegistroFaltas
	Inscricoes                 []Inscricao
}

// RegistroNotas representa notas do estudante
type RegistroNotas struct {
	CodigoAcademia string    // 🔥 MUDOU
	AnoLectivo     string
	Periodo        string
	Materias       []Materia
	RegisteredAt   time.Time
}

// RegistroFaltas representa faltas do estudante
type RegistroFaltas struct {
	CodigoAcademia string          // 🔥 MUDOU
	AnoLectivo     string
	Periodo        string
	Materias       []MateriaFaltas
	RegisteredAt   time.Time
}

// Inscricao representa uma inscrição
type Inscricao struct {
	CodigoAcademia string    // 🔥 MUDOU
	Tipo           string
	AnoInscricao   string
	Curso          *string
	Status         string
	CreatedAt      time.Time
}

// Materia representa uma matéria com nota
type Materia struct {
	Nome string
	Nota float64
}

// MateriaFaltas representa uma matéria com faltas
type MateriaFaltas struct {
	Nome   string
	Faltas int
}

// NewEstudante cria um novo agregado Estudante
func NewEstudante() *Estudante {
	return &Estudante{
		BaseAggregate: BaseAggregate{
			ID:                uuid.New(),
			Version:           0,
			UncommittedEvents: []DomainEvent{},
		},
		Notas:      []RegistroNotas{},
		Faltas:     []RegistroFaltas{},
		Inscricoes: []Inscricao{},
	}
}

// GetType implementa Aggregate
func (e *Estudante) GetType() string {
	return "Estudante"
}

// Apply aplica eventos ao agregado
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
	case "VinculoAtualizado":
		return e.applyVinculoAtualizado(event)
	default:
		return fmt.Errorf("tipo de evento desconhecido: %s", event.GetEventType())
	}
}

// Comandos - geram eventos

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
	// Validações
	if nome == "" {
		return fmt.Errorf("nome é obrigatório")
	}
	if codigoEstudante == "" {
		return fmt.Errorf("codigo_estudante é obrigatório")
	}
	if senhaHash == "" {
		return fmt.Errorf("senha é obrigatória")
	}
	if bilhete == nil && bilheteResp == nil {
		return fmt.Errorf("pelo menos um bilhete é obrigatório")
	}

	// Criar evento
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
		StatusEscolar:         statusEscolar,
		StatusSuperior:        statusSuperior,
		CreatedAt:             time.Now(),
	}

	e.RaiseEvent(event)
	return e.Apply(event)
}

// RegistrarNotas registra notas do estudante
// 🔥 ATUALIZADO: Recebe codigoAcademia em vez de idAcademia
func (e *Estudante) RegistrarNotas(
	codigoAcademia string,
	anoLectivo string,
	periodo string,
	materias []Materia,
) error {
	// Validações
	if e.CodigoAcademia == nil || *e.CodigoAcademia != codigoAcademia {
		return fmt.Errorf("estudante não pertence a esta academia")
	}
	if len(materias) == 0 {
		return fmt.Errorf("materias não pode estar vazio")
	}

	// Criar evento
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

// RegistrarFaltas registra faltas do estudante
// 🔥 ATUALIZADO: Recebe codigoAcademia em vez de idAcademia
func (e *Estudante) RegistrarFaltas(
	codigoAcademia string,
	anoLectivo string,
	periodo string,
	materias []MateriaFaltas,
) error {
	// Validações
	if e.CodigoAcademia == nil || *e.CodigoAcademia != codigoAcademia {
		return fmt.Errorf("estudante não pertence a esta academia")
	}
	if len(materias) == 0 {
		return fmt.Errorf("materias não pode estar vazio")
	}

	// Criar evento
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

// SolicitarInscricao solicita inscrição em uma academia
// 🔥 ATUALIZADO: Recebe codigoAcademia
func (e *Estudante) SolicitarInscricao(
	codigoAcademia string,
	tipo string,
	anoInscricao string,
	curso *string,
) error {
	// Validações
	if tipo != "escola" && tipo != "universidade" {
		return fmt.Errorf("tipo deve ser 'escola' ou 'universidade'")
	}

	// 🔥 VALIDAÇÃO: Não pode se inscrever se já pertence a esta academia
	if e.CodigoAcademia != nil && *e.CodigoAcademia == codigoAcademia {
		return fmt.Errorf("você já está matriculado nesta academia")
	}

	// 🔥 VALIDAÇÃO: Não pode ter inscrição pendente nesta academia
	for _, inscricao := range e.Inscricoes {
		if inscricao.CodigoAcademia == codigoAcademia && inscricao.Status == "espera" {
			return fmt.Errorf("você já possui uma inscrição pendente nesta academia")
		}
	}

	// Criar evento
	event := &EstudanteInscritoEvent{
		BaseEvent: BaseEvent{
			EventType:   "EstudanteInscrito",
			AggregateID: e.ID,
		},
		CodigoAcademia: codigoAcademia,
		Tipo:           tipo,
		AnoInscricao:   anoInscricao,
		Curso:          curso,
		CreatedAt:      time.Now(),
	}

	e.RaiseEvent(event)
	return e.Apply(event)
}

// AprovarInscricao aprova uma inscrição
// 🔥 ATUALIZADO: Recebe codigoAcademia
func (e *Estudante) AprovarInscricao(
	codigoAcademia string,
	inscricaoID uuid.UUID,
) error {
	// Buscar inscrição pendente
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

	// Criar evento
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

// ReprovarInscricao processa a reprovação de uma inscrição
// 🔥 ATUALIZADO: Recebe codigoAcademia
func (e *Estudante) ReprovarInscricao(
	codigoAcademia string,
	inscricaoID uuid.UUID,
) error {
	// Buscar inscrição pendente
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

	// Criar evento
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

// Event Handlers - aplicam eventos ao estado

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
		CodigoAcademia: ev.CodigoAcademia,
		Tipo:           ev.Tipo,
		AnoInscricao:   ev.AnoInscricao,
		Curso:          ev.Curso,
		Status:         "espera",
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

	// Atualizar status da inscrição
	for i := range e.Inscricoes {
		if e.Inscricoes[i].CodigoAcademia == ev.CodigoAcademia && e.Inscricoes[i].Status == "espera" {
			e.Inscricoes[i].Status = "aprovado"
			break
		}
	}

	// Vincular à academia
	e.CodigoAcademia = &ev.CodigoAcademia

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

	// Atualizar status da inscrição
	for i := range e.Inscricoes {
		if e.Inscricoes[i].CodigoAcademia == ev.CodigoAcademia && e.Inscricoes[i].Status == "espera" {
			e.Inscricoes[i].Status = "reprovado"
			break
		}
	}

	return nil
}

func (e *Estudante) applyVinculoAtualizado(event DomainEvent) error {
	payload := event.GetPayload()
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	var ev VinculoAtualizadoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}

	e.CodigoAcademia = &ev.NovoCodigoAcademia

	return nil
}

// Eventos do Estudante

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
	StatusEscolar         *string
	StatusSuperior        *string
	CreatedAt             time.Time
}

func (e *EstudanteCriadoEvent) GetPayload() interface{} {
	return e
}

// 🔥 ATUALIZADO
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

// 🔥 ATUALIZADO
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

// 🔥 ATUALIZADO
type EstudanteInscritoEvent struct {
	BaseEvent
	CodigoAcademia string
	Tipo           string
	AnoInscricao   string
	Curso          *string
	CreatedAt      time.Time
}

func (e *EstudanteInscritoEvent) GetPayload() interface{} {
	return e
}

// 🔥 ATUALIZADO
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

// 🔥 ATUALIZADO
type InscricaoReprovadaEvent struct {
	BaseEvent
	InscricaoID    uuid.UUID
	CodigoAcademia string
}

func (e *InscricaoReprovadaEvent) GetPayload() interface{} {
	return e
}

// 🔥 ATUALIZADO
type VinculoAtualizadoEvent struct {
	BaseEvent
	NovoCodigoAcademia string
}

func (e *VinculoAtualizadoEvent) GetPayload() interface{} {
	return e
}