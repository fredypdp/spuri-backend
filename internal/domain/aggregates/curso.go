package aggregates

import (
	"encoding/json"
	"fmt"
	"log"
	"spuri/internal/utils"
	"time"

	"github.com/google/uuid"
)

// periodosAceitos é o conjunto global de períodos aceitos pelo sistema.
// Renomeado de periodosValidos para evitar shadowing com parâmetros de função
// no mesmo pacote.
var periodosAceitos = map[string]bool{
	"1_trimestre": true,
	"2_trimestre": true,
	"3_trimestre": true,
	"1_semestre":  true,
	"2_semestre":  true,
}

type Curso struct {
	BaseAggregate

	Nome           string
	Type           string   // "medio" ou "superior" — imutável após criação
	AnosAcademicos []string // Anos do curso definidos pela academia
	// Periodos define os períodos letivos do curso.
	// Obrigatório para type="superior"; NULL/vazio para "medio".
	// Deve ser subconjunto de: 1_trimestre, 2_trimestre, 3_trimestre, 1_semestre, 2_semestre.
	Periodos       []string
	CodigoAcademia string
	Status         string
	CreatedAt      time.Time
}

func NewCurso() *Curso {
	log.Printf("[DEBUG] Criando novo agregado Curso")
	return &Curso{
		BaseAggregate: BaseAggregate{
			ID:                uuid.New(),
			Version:           0,
			UncommittedEvents: []DomainEvent{},
		},
		Status:         "ativo",
		AnosAcademicos: []string{},
		Periodos:       []string{},
	}
}

func (c *Curso) GetType() string {
	return "Curso"
}

func (c *Curso) Apply(event DomainEvent) error {
	log.Printf("[DEBUG] Aplicando evento %s ao Curso %s", event.GetEventType(), c.ID)

	switch event.GetEventType() {
	case "CursoCriado":
		return c.applyCursoCriado(event)
	case "CursoAtivado":
		return c.applyCursoAtivado(event)
	case "CursoDesativado":
		return c.applyCursoDesativado(event)
	case "CursoDadosAtualizados":
		return c.applyCursoDadosAtualizados(event)
	default:
		log.Printf("[ERROR] Tipo de evento desconhecido: %s", event.GetEventType())
		return fmt.Errorf("tipo de evento desconhecido: %s", event.GetEventType())
	}
}

// ============================================================================
// Eventos
// ============================================================================

type CursoCriadoEvent struct {
	BaseEvent
	Nome           string
	Type           string
	AnosAcademicos []string
	// Periodos: obrigatório para superior, vazio para medio.
	Periodos       []string
	CodigoAcademia string
	CreatedAt      time.Time
}

func (e *CursoCriadoEvent) GetPayload() interface{} { return e }

type CursoAtivadoEvent struct {
	BaseEvent
	ActivatedAt time.Time
}

func (e *CursoAtivadoEvent) GetPayload() interface{} { return e }

type CursoDesativadoEvent struct {
	BaseEvent
	DeactivatedAt time.Time
}

func (e *CursoDesativadoEvent) GetPayload() interface{} { return e }

// CursoDadosAtualizadosEvent — Type foi removido: o tipo do curso é imutável após criação.
// Periodos: nil = não alterar; lista não vazia = atualizar periodos.
type CursoDadosAtualizadosEvent struct {
	BaseEvent
	Nome           *string
	AnosAcademicos []string
	Periodos       *[]string
	UpdatedAt      time.Time
}

func (e *CursoDadosAtualizadosEvent) GetPayload() interface{} { return e }

// ============================================================================
// Commands
// ============================================================================

// Criar registra o evento de criação do curso.
//
// Para type="superior": periodos é OBRIGATÓRIO e deve ser um subconjunto
// não vazio de {1_trimestre, 2_trimestre, 3_trimestre, 1_semestre, 2_semestre}.
// Para type="medio": periodos deve ser nil ou vazio.
func (c *Curso) Criar(
	nome string,
	tipo string,
	anosAcademicos []string,
	periodos []string,
	codigoAcademia string,
) error {
	log.Printf("[DEBUG] Criando curso: nome=%s, tipo=%s, anosAcademicos=%v, periodos=%v, academia=%s",
		nome, tipo, anosAcademicos, periodos, codigoAcademia)

	if nome == "" {
		return fmt.Errorf("nome é obrigatório")
	}
	if tipo != "medio" && tipo != "superior" {
		return fmt.Errorf("tipo deve ser 'medio' ou 'superior'")
	}
	if len(anosAcademicos) == 0 {
		return fmt.Errorf("anos_academicos é obrigatório")
	}
	if codigoAcademia == "" {
		return fmt.Errorf("código da academia é obrigatório")
	}

	if err := utils.ValidateAnosCurso(tipo, anosAcademicos); err != nil {
		return err
	}

	if err := validarPeriodosCurso(tipo, periodos); err != nil {
		return err
	}

	event := &CursoCriadoEvent{
		BaseEvent: BaseEvent{
			EventType:   "CursoCriado",
			AggregateID: c.ID,
		},
		Nome:           nome,
		Type:           tipo,
		AnosAcademicos: anosAcademicos,
		Periodos:       normalizarPeriodos(tipo, periodos),
		CodigoAcademia: codigoAcademia,
		CreatedAt:      time.Now(),
	}

	log.Printf("[DEBUG] Evento CursoCriado criado para curso %s", c.ID)
	c.RaiseEvent(event)
	return c.Apply(event)
}

func (c *Curso) Ativar() error {
	log.Printf("[DEBUG] Ativando curso %s (status atual: %s)", c.ID, c.Status)

	if c.Status == "ativo" {
		return fmt.Errorf("curso já está ativo")
	}

	event := &CursoAtivadoEvent{
		BaseEvent: BaseEvent{
			EventType:   "CursoAtivado",
			AggregateID: c.ID,
		},
		ActivatedAt: time.Now(),
	}

	c.RaiseEvent(event)
	return c.Apply(event)
}

func (c *Curso) Desativar() error {
	log.Printf("[DEBUG] Desativando curso %s (status atual: %s)", c.ID, c.Status)

	if c.Status == "inativo" {
		return fmt.Errorf("curso já está inativo")
	}

	event := &CursoDesativadoEvent{
		BaseEvent: BaseEvent{
			EventType:   "CursoDesativado",
			AggregateID: c.ID,
		},
		DeactivatedAt: time.Now(),
	}

	c.RaiseEvent(event)
	return c.Apply(event)
}

// AtualizarDados atualiza nome, anos_academicos e/ou periodos do curso.
// O tipo do curso é IMUTÁVEL após a criação.
// Passe nil para não alterar os respectivos campos.
// periodos=nil → não altera; periodos=&[]string{...} → atualiza.
func (c *Curso) AtualizarDados(nome *string, anosAcademicos []string, periodos *[]string) error {
	if nome == nil && anosAcademicos == nil && periodos == nil {
		return fmt.Errorf("nenhum campo para atualizar")
	}

	if anosAcademicos != nil {
		if err := utils.ValidateAnosCurso(c.Type, anosAcademicos); err != nil {
			return err
		}
	}

	if periodos != nil {
		if err := validarPeriodosCurso(c.Type, *periodos); err != nil {
			return err
		}
		normalized := normalizarPeriodos(c.Type, *periodos)
		periodos = &normalized
	}

	event := &CursoDadosAtualizadosEvent{
		BaseEvent: BaseEvent{
			EventType:   "CursoDadosAtualizados",
			AggregateID: c.ID,
		},
		Nome:           nome,
		AnosAcademicos: anosAcademicos,
		Periodos:       periodos,
		UpdatedAt:      time.Now(),
	}

	c.RaiseEvent(event)
	return c.Apply(event)
}

// ============================================================================
// Apply handlers
// ============================================================================

func (c *Curso) applyCursoCriado(event DomainEvent) error {
	payload := event.GetPayload()

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("erro ao serializar evento CursoCriado: %w", err)
	}

	var p CursoCriadoEvent
	if err := json.Unmarshal(data, &p); err != nil {
		return fmt.Errorf("erro ao desserializar evento CursoCriado: %w", err)
	}

	c.Nome = p.Nome
	c.Type = p.Type
	c.AnosAcademicos = p.AnosAcademicos
	c.Periodos = p.Periodos
	c.CodigoAcademia = p.CodigoAcademia
	c.Status = "ativo"
	c.CreatedAt = p.CreatedAt

	log.Printf("[DEBUG] applyCursoCriado: curso=%s tipo=%s periodos=%v", c.Nome, c.Type, c.Periodos)
	return nil
}

func (c *Curso) applyCursoAtivado(event DomainEvent) error {
	c.Status = "ativo"
	log.Printf("[DEBUG] applyCursoAtivado: curso=%s", c.Nome)
	return nil
}

func (c *Curso) applyCursoDesativado(event DomainEvent) error {
	c.Status = "inativo"
	log.Printf("[DEBUG] applyCursoDesativado: curso=%s", c.Nome)
	return nil
}

func (c *Curso) applyCursoDadosAtualizados(event DomainEvent) error {
	payload := event.GetPayload()

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("erro ao serializar evento CursoDadosAtualizados: %w", err)
	}

	var p CursoDadosAtualizadosEvent
	if err := json.Unmarshal(data, &p); err != nil {
		return fmt.Errorf("erro ao desserializar evento CursoDadosAtualizados: %w", err)
	}

	if p.Nome != nil {
		c.Nome = *p.Nome
	}
	if p.AnosAcademicos != nil {
		c.AnosAcademicos = p.AnosAcademicos
	}
	if p.Periodos != nil {
		c.Periodos = *p.Periodos
	}

	log.Printf("[DEBUG] applyCursoDadosAtualizados: curso=%s periodos=%v", c.Nome, c.Periodos)
	return nil
}

// ============================================================================
// Helpers internos
// ============================================================================

// validarPeriodosCurso valida os periodos conforme o tipo do curso:
//   - superior → obrigatório, ≥1 item, todos do enum global
//   - medio    → deve ser nil ou vazio
func validarPeriodosCurso(tipo string, periodos []string) error {
	switch tipo {
	case "superior":
		if len(periodos) == 0 {
			return fmt.Errorf("periodos é obrigatório para cursos do tipo 'superior'")
		}
		seen := make(map[string]bool, len(periodos))
		for _, p := range periodos {
			if !periodosAceitos[p] {
				return fmt.Errorf(
					"período '%s' inválido. Aceitos: 1_trimestre, 2_trimestre, 3_trimestre, 1_semestre, 2_semestre",
					p,
				)
			}
			if seen[p] {
				return fmt.Errorf("período duplicado: '%s'", p)
			}
			seen[p] = true
		}
	case "medio":
		if len(periodos) > 0 {
			return fmt.Errorf("cursos do tipo 'medio' não devem ter periodos definidos (são fixos no sistema: 1_trimestre, 2_trimestre, 3_trimestre)")
		}
	}
	return nil
}

// normalizarPeriodos retorna slice vazio para medio e a lista para superior.
func normalizarPeriodos(tipo string, periodos []string) []string {
	if tipo == "medio" {
		return []string{}
	}
	return periodos
}