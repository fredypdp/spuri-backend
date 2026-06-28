package aggregates

import (
	"encoding/json"
	"fmt"
	"log"
	"spuri/internal/utils"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Curso — aggregate raiz para cursos de médio e superior.
//
// Regras de períodos:
//   - type="medio"    → períodos FIXOS no sistema: 1_trimestre, 2_trimestre, 3_trimestre.
//                       A academia NÃO configura; o campo Periodos fica vazio no aggregate.
//   - type="superior" → períodos DINÂMICOS definidos pela academia na criação.
//                       Formato obrigatório: [número]_semestre (ex.: 1_semestre, 2_semestre).
//                       Número deve ser inteiro ≥ 1; sem duplicatas.

type Curso struct {
	BaseAggregate

	Nome           string
	Type           string   // "medio" ou "superior" — imutável após criação
	AnosAcademicos []string // Anos do curso definidos pela academia
	// Periodos define os períodos letivos do curso.
	// Obrigatório para type="superior"; vazio para "medio" (fixos no sistema).
	Periodos       []string
	CodigoAcademia string
	Status         string
	CreatedAt      time.Time
	DeletedAt      *time.Time
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

func (c *Curso) GetType() string { return "Curso" }

// ============================================================================
// Apply dispatcher
// ============================================================================

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
	case "CursoDeletado":
		return c.applyCursoDeletado(event)
	default:
		log.Printf("[ERROR] Tipo de evento desconhecido: %s", event.GetEventType())
		return fmt.Errorf("tipo de evento desconhecido: %s", event.GetEventType())
	}
}

// ============================================================================
// Eventos
// FIX C-01: ToJSON() adicionado a todos os eventos concretos do Curso.
// Sem essa sobrescrita, json.Marshal(e) no BaseEvent serializa apenas os campos
// do BaseEvent, gravando payload nulo no ledger e impossibilitando o rebuild.
// ============================================================================

type CursoCriadoEvent struct {
	BaseEvent
	Nome           string
	Type           string
	AnosAcademicos []string
	Periodos       []string
	CodigoAcademia string
	CreatedAt      time.Time
}

func (e *CursoCriadoEvent) GetPayload() interface{} { return e }
func (e *CursoCriadoEvent) ToJSON() ([]byte, error) { return json.Marshal(e) } // FIX C-01

type CursoAtivadoEvent struct {
	BaseEvent
	AtivadoPor  uuid.UUID
	ActivatedAt time.Time
}

func (e *CursoAtivadoEvent) GetPayload() interface{} { return e }
func (e *CursoAtivadoEvent) ToJSON() ([]byte, error) { return json.Marshal(e) } // FIX C-01

type CursoDesativadoEvent struct {
	BaseEvent
	DesativadoPor uuid.UUID
	DeactivatedAt time.Time
}

func (e *CursoDesativadoEvent) GetPayload() interface{} { return e }
func (e *CursoDesativadoEvent) ToJSON() ([]byte, error) { return json.Marshal(e) } // FIX C-01

// CursoDadosAtualizadosEvent — Type é imutável após criação.
// Periodos: nil = não alterar; ponteiro para lista = atualizar.
// FIX C-02: AtualizadoPor adicionado para auditoria self-contained.
type CursoDadosAtualizadosEvent struct {
	BaseEvent
	Nome           *string
	AnosAcademicos []string
	Periodos       *[]string
	UpdatedAt      time.Time
	// FIX C-02: UUID do usuário que atualizou. uuid.Nil = legado/não preenchido.
	AtualizadoPor uuid.UUID
}

func (e *CursoDadosAtualizadosEvent) GetPayload() interface{} { return e }
func (e *CursoDadosAtualizadosEvent) ToJSON() ([]byte, error) { return json.Marshal(e) } // FIX C-01

type CursoDeletadoEvent struct {
	BaseEvent
	DeletadoPor uuid.UUID
	Motivo      string
	DeletedAt   time.Time
}

func (e *CursoDeletadoEvent) GetPayload() interface{} { return e }
func (e *CursoDeletadoEvent) ToJSON() ([]byte, error) { return json.Marshal(e) } // FIX C-01

// ============================================================================
// Commands
// ============================================================================

// Criar registra o evento de criação do curso.
//
// Para type="medio":    periodos deve ser nil ou vazio — fixos no sistema
//
//	(1_trimestre, 2_trimestre, 3_trimestre).
//
// Para type="superior": periodos é OBRIGATÓRIO; formato [número]_semestre
//
//	(ex.: 1_semestre, 2_semestre); número inteiro ≥ 1;
//	sem duplicatas.
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

	if err := validarPeriodosCurso(tipo, periodos, len(anosAcademicos)); err != nil {
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

func (c *Curso) Ativar(ativadoPor uuid.UUID) error {
	log.Printf("[DEBUG] Ativando curso %s (status atual: %s)", c.ID, c.Status)

	if c.Status == "ativo" {
		return fmt.Errorf("curso já está ativo")
	}

	event := &CursoAtivadoEvent{
		BaseEvent: BaseEvent{
			EventType:   "CursoAtivado",
			AggregateID: c.ID,
		},
		AtivadoPor:  ativadoPor,
		ActivatedAt: time.Now(),
	}

	c.RaiseEvent(event)
	return c.Apply(event)
}

func (c *Curso) Desativar(desativadoPor uuid.UUID) error {
	log.Printf("[DEBUG] Desativando curso %s (status atual: %s)", c.ID, c.Status)

	if c.Status == "inativo" {
		return fmt.Errorf("curso já está inativo")
	}

	event := &CursoDesativadoEvent{
		BaseEvent: BaseEvent{
			EventType:   "CursoDesativado",
			AggregateID: c.ID,
		},
		DesativadoPor: desativadoPor,
		DeactivatedAt: time.Now(),
	}

	c.RaiseEvent(event)
	return c.Apply(event)
}

// AtualizarDados atualiza nome, anos_academicos e/ou periodos do curso.
// O tipo do curso é IMUTÁVEL após a criação.
// Passe nil para não alterar os respectivos campos.
// periodos=nil → não altera; periodos=&[]string{...} → atualiza.
func (c *Curso) AtualizarDados(nome *string, anosAcademicos []string, periodos *[]string, atualizadoPor uuid.UUID) error {
	if nome == nil && anosAcademicos == nil && periodos == nil {
		return fmt.Errorf("nenhum campo para atualizar")
	}

	if anosAcademicos != nil {
		if err := utils.ValidateAnosCurso(c.Type, anosAcademicos); err != nil {
			return err
		}
	}

	if periodos != nil {
		if err := validarPeriodosCurso(c.Type, *periodos, len(c.AnosAcademicos)); err != nil {
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
		AtualizadoPor:  atualizadoPor,
	}

	c.RaiseEvent(event)
	return c.Apply(event)
}

// Deletar emite CursoDeletado.
// A validação de dependências (estudantes matriculados, matérias ativas)
// é feita no handler ANTES de chamar este método.
// Pré-condição aqui: curso deve estar inativo.
//
// NOTA C-03: a validação de estudantes matriculados é responsabilidade do
// handler (requer acesso à projeção). O aggregate só pode validar seu próprio
// estado (status).
func (c *Curso) Deletar(deletadoPor uuid.UUID, motivo string) error {
	if c.Status == "deletado" {
		return fmt.Errorf("curso já está deletado")
	}
	if c.Status == "ativo" {
		return fmt.Errorf("desative o curso antes de deletá-lo")
	}

	event := &CursoDeletadoEvent{
		BaseEvent:   BaseEvent{EventType: "CursoDeletado", AggregateID: c.ID},
		DeletadoPor: deletadoPor,
		Motivo:      motivo,
		DeletedAt:   time.Now(),
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

func (c *Curso) applyCursoAtivado(_ DomainEvent) error {
	c.Status = "ativo"
	log.Printf("[DEBUG] applyCursoAtivado: curso=%s", c.Nome)
	return nil
}

func (c *Curso) applyCursoDesativado(_ DomainEvent) error {
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

func (c *Curso) applyCursoDeletado(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return err
	}
	var ev CursoDeletadoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}
	c.Status = "deletado"
	c.DeletedAt = &ev.DeletedAt
	return nil
}

// ============================================================================
// Helpers internos
// ============================================================================

// isSemestreValido verifica se o período segue o formato [número]_semestre
// onde número é um inteiro ≥ 1. Ex.: "1_semestre", "2_semestre", "10_semestre".
// Usa apenas strings/strconv — sem import de regexp.
func isSemestreValido(p string) bool {
	const sufixo = "_semestre"
	if !strings.HasSuffix(p, sufixo) {
		return false
	}
	numStr := strings.TrimSuffix(p, sufixo)
	if len(numStr) == 0 {
		return false
	}
	n, err := strconv.Atoi(numStr)
	return err == nil && n >= 1
}

// validarPeriodosCurso valida os períodos de acordo com o tipo do curso.
//
//   - "medio":    não deve ter períodos — são fixos no sistema
//     (1_trimestre, 2_trimestre, 3_trimestre).
//   - "superior": obrigatório; cada item deve seguir [número]_semestre;
//     sem duplicatas.
func validarPeriodosCurso(tipo string, periodos []string, totalAnos int) error {
	switch tipo {
	case "superior":
		if len(periodos) == 0 {
			return fmt.Errorf("periodos é obrigatório para cursos do tipo 'superior' (formato: 1_semestre, 2_semestre, ...)")
		}
		// Consistência: cada ano acadêmico deve ter entre 1 e 2 semestres.
		// Como anos acadêmicos do superior são sequenciais e únicos, isso implica:
		// total_semestres >= total_anos e total_semestres <= total_anos*2.
		if totalAnos > 0 {
			if len(periodos) < totalAnos || len(periodos) > totalAnos*2 {
				return fmt.Errorf("cursos superiores devem respeitar: total_semestres >= total_anos e total_semestres <= total_anos*2")
			}
		}
		seen := make(map[string]bool, len(periodos))
		for i, p := range periodos {
			if !isSemestreValido(p) {
				return fmt.Errorf(
					"período '%s' inválido para curso superior. "+
						"Use o formato [número]_semestre (ex.: 1_semestre, 2_semestre)",
					p,
				)
			}
			if seen[p] {
				return fmt.Errorf("período duplicado: '%s'", p)
			}
			seen[p] = true
			esperado := fmt.Sprintf("%d_semestre", i+1)
			if p != esperado {
				return fmt.Errorf("periodos de curso superior devem ser sequenciais de 1_semestre até N_semestre")
			}
		}
		totalAnosEsperado := (len(periodos) + 1) / 2
		if totalAnos > 0 && totalAnos != totalAnosEsperado {
			return fmt.Errorf("anos_academicos de curso superior devem ser derivados de periodos: esperado %d ano(s)", totalAnosEsperado)
		}

	case "medio":
		if len(periodos) > 0 {
			return fmt.Errorf(
				"cursos do tipo 'medio' não devem ter periodos definidos — " +
					"são fixos no sistema: 1_trimestre, 2_trimestre, 3_trimestre",
			)
		}
	}
	return nil
}

// normalizarPeriodos garante que médio nunca persista períodos no aggregate
// (os trimestres são constantes do sistema, não dados do curso).
func normalizarPeriodos(tipo string, periodos []string) []string {
	if tipo == "medio" {
		return []string{}
	}
	return periodos
}
