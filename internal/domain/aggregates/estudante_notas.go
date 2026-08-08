package aggregates

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// Constantes de tipo e categoria
// ============================================================================

const (
	TipoEscolar  = "escolar"
	TipoSuperior = "superior"
)

// PeriodosEscolar são os períodos fixos para notas do tipo escolar.
var PeriodosEscolar = []string{"1_trimestre", "2_trimestre", "3_trimestre"}

// ============================================================================
// Eventos
// ============================================================================

// NotasRegistradasEvent — emitido ao registrar uma nota pela primeira vez.
// EventType: "NotasRegistradas" (canônico — a projeção escuta exatamente este string).
// AnoAcademico é sempre preenchido pelo back end (nunca pelo cliente).
//
// FIX E-06: campo RegistradoPor adicionado para auditoria self-contained.
type NotasRegistradasEvent struct {
	BaseEvent
	CodigoEstudante      string
	CodigoAcademia       string
	AnoLectivo           string
	AnoAcademico         string // inferido pelo back end
	Periodo              string
	MateriaDisciplinarID uuid.UUID
	Tipo                 string // "escolar" | "superior"
	Categoria            string
	Nota                 float64
	Observacao           *string
	RegisteredAt         time.Time
	// FIX E-06: UUID do usuário que registrou a nota. uuid.Nil = legado/não preenchido.
	RegistradoPor uuid.UUID
}

func (e *NotasRegistradasEvent) GetPayload() interface{} { return e }
func (e *NotasRegistradasEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

// NotaCorrigidaEvent preserves the original ledger record and carries the
// audited replacement displayed by the read projection.
type NotaCorrigidaEvent struct {
	BaseEvent
	NotaAnteriorID       uuid.UUID
	CodigoAcademia       string
	AnoLectivo           string
	Periodo              string
	MateriaDisciplinarID uuid.UUID
	Tipo                 string
	Categoria            string
	NovaNota             float64
	NovaObservacao       *string
	Motivo               string
	CorrigidoPor         uuid.UUID
	CorrigidoEm          time.Time
}

func (e *NotaCorrigidaEvent) GetPayload() interface{} { return e }
func (e *NotaCorrigidaEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

// ============================================================================
// Validações internas
// ============================================================================

func validarPeriodoComLista(periodo string, periodosValidos []string) error {
	if periodo == "" {
		return fmt.Errorf("periodo não pode ser vazio")
	}
	for _, p := range periodosValidos {
		if p == periodo {
			return nil
		}
	}
	return fmt.Errorf("periodo '%s' inválido para este contexto. Aceitos: %v", periodo, periodosValidos)
}

// validarCategoria verifica se a categoria é aceita para o tipo de nota.
//
// Regras:
//   - A categoria deve estar configurada pela academia para o ano_academico
//     inferido da nota. Categorias sem anos definidos não chegam aqui.
//
// categoriasAdicionais contém os códigos das categorias configuradas para o
// ano_academico da nota.
func validarCategoria(tipo string, categoria string, categoriasAdicionais []string) error {
	if categoria == "" {
		return fmt.Errorf("categoria não pode ser vazia")
	}

	categoriaConfigurada := false
	for _, c := range categoriasAdicionais {
		if c == categoria {
			categoriaConfigurada = true
			break
		}
	}
	if !categoriaConfigurada {
		return fmt.Errorf("categoria '%s' não está configurada para o ano_academico da nota", categoria)
	}

	switch tipo {
	case TipoEscolar, TipoSuperior:
		return nil
	default:
		return fmt.Errorf("tipo '%s' inválido. Use 'escolar' ou 'superior'", tipo)
	}
}

// chaveNota retorna a chave composta usada para detectar duplicatas de nota no aggregate.
// Formato: "<codigoAcademia>_<anoLectivo>_<periodo>_<materiaID>_<tipo>_<categoria>"
// Deve coincidir exatamente com as colunas da constraint uq_nota_unica do banco.
func chaveNota(codigoAcademia, anoLectivo, periodo string, materiaID uuid.UUID, tipo, categoria string) string {
	return codigoAcademia + "_" + anoLectivo + "_" + periodo + "_" + materiaID.String() + "_" + tipo + "_" + categoria
}

// ============================================================================
// Método de comando: RegistrarNota
// ============================================================================

// RegistrarNota registra uma nota pela primeira vez.
//
// periodosValidos: lista de períodos aceitos para este tipo de nota.
//   - tipo="escolar"  → sempre PeriodosEscolar (handler preenche)
//   - tipo="superior" → curso.Periodos da matéria (handler preenche)
//
// anoAcademico é inferido pelo handler (não vem do request):
//   - estudante no fundamental → estudante.AnoEscolar
//   - estudante no médio/superior → materia.AnosAcademicos[0]
//
// categoriasAdicionais: códigos das categorias configuradas pela academia
// para o ano_academico da nota. Deve ser fornecida para qualquer tipo —
// escolar ou superior.
//
// FIX NOTA-AGG-01: guard de duplicata via NotasRegistradasPorChave antes de
// emitir o evento, evitando double-submit e a violação 23505 na projeção.
func (e *Estudante) RegistrarNota(
	codigoAcademia string,
	anoLectivo string,
	anoAcademico string,
	periodo string,
	materiaDisciplinarID uuid.UUID,
	tipo string,
	categoria string,
	nota float64,
	observacao *string,
	categoriasAdicionais []string,
	periodosValidos []string,
	registradoPor uuid.UUID,
	maxNota float64,
) error {
	if e.CodigoAcademia == nil || *e.CodigoAcademia != codigoAcademia {
		return fmt.Errorf("estudante não pertence a esta academia")
	}
	if strings.TrimSpace(anoAcademico) == "" {
		return fmt.Errorf("ano_academico não pode ser vazio")
	}
	if err := validarPeriodoComLista(periodo, periodosValidos); err != nil {
		return err
	}
	if err := validarCategoria(tipo, categoria, categoriasAdicionais); err != nil {
		return err
	}
	if nota < 0 || nota > maxNota {
		return fmt.Errorf("nota deve estar entre 0 e %.0f", maxNota)
	}

	// FIX NOTA-AGG-01: detectar duplicata via estado do aggregate.
	// Evita double-submit e a violação de unique constraint 23505 na projeção.
	chave := chaveNota(codigoAcademia, anoLectivo, periodo, materiaDisciplinarID, tipo, categoria)
	if e.NotasRegistradasPorChave != nil && e.NotasRegistradasPorChave[chave] {
		return fmt.Errorf(
			"nota já registrada para periodo '%s', materia '%s', tipo '%s', categoria '%s' no ano letivo '%s'",
			periodo, materiaDisciplinarID, tipo, categoria, anoLectivo,
		)
	}

	event := &NotasRegistradasEvent{
		BaseEvent:            BaseEvent{EventType: "NotasRegistradas", AggregateID: e.ID},
		CodigoEstudante:      e.CodigoEstudante,
		CodigoAcademia:       codigoAcademia,
		AnoLectivo:           anoLectivo,
		AnoAcademico:         anoAcademico,
		Periodo:              periodo,
		MateriaDisciplinarID: materiaDisciplinarID,
		Tipo:                 tipo,
		Categoria:            categoria,
		Nota:                 nota,
		Observacao:           observacao,
		RegisteredAt:         time.Now(),
		RegistradoPor:        registradoPor,
	}

	e.RaiseEvent(event)
	return e.Apply(event)
}

func (e *Estudante) CorrigirNota(notaAnteriorID uuid.UUID, codigoAcademia, anoLectivo, periodo string, materiaID uuid.UUID, tipo, categoria string, novaNota float64, novaObservacao *string, motivo string, corrigidoPor uuid.UUID, maxNota float64) error {
	if e.CodigoAcademia == nil || *e.CodigoAcademia != codigoAcademia {
		return fmt.Errorf("estudante não pertence a esta academia")
	}
	if notaAnteriorID == uuid.Nil {
		return fmt.Errorf("id da nota original inválido")
	}
	if strings.TrimSpace(motivo) == "" {
		return fmt.Errorf("motivo da correção é obrigatório")
	}
	if novaNota < 0 || novaNota > maxNota {
		return fmt.Errorf("nota deve estar entre 0 e %.0f", maxNota)
	}
	chave := chaveNota(codigoAcademia, anoLectivo, periodo, materiaID, tipo, categoria)
	if e.NotasRegistradasPorChave == nil || !e.NotasRegistradasPorChave[chave] {
		return fmt.Errorf("nota original não encontrada para correção")
	}
	event := &NotaCorrigidaEvent{BaseEvent: BaseEvent{EventType: "NotaCorrigida", AggregateID: e.ID}, NotaAnteriorID: notaAnteriorID, CodigoAcademia: codigoAcademia, AnoLectivo: anoLectivo, Periodo: periodo, MateriaDisciplinarID: materiaID, Tipo: tipo, Categoria: categoria, NovaNota: novaNota, NovaObservacao: novaObservacao, Motivo: strings.TrimSpace(motivo), CorrigidoPor: corrigidoPor, CorrigidoEm: time.Now()}
	e.RaiseEvent(event)
	return e.Apply(event)
}

// ============================================================================
// Apply handlers
// ============================================================================

// applyNotasRegistradas — FIX NOTA-AGG-01: mantém NotasRegistradasPorChave em
// estado para que RegistrarNota possa detectar duplicatas sem depender da projeção.
// A chave usada aqui é idêntica à constraint uq_nota_unica do banco.
func (e *Estudante) applyNotasRegistradas(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applyNotasRegistradas: marshal error: %w", err)
	}
	var ev NotasRegistradasEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return fmt.Errorf("applyNotasRegistradas: unmarshal error: %w", err)
	}

	if e.NotasRegistradasPorChave == nil {
		e.NotasRegistradasPorChave = make(map[string]bool)
	}
	chave := chaveNota(ev.CodigoAcademia, ev.AnoLectivo, ev.Periodo, ev.MateriaDisciplinarID, ev.Tipo, ev.Categoria)
	e.NotasRegistradasPorChave[chave] = true
	return nil
}

func (e *Estudante) applyNotaCorrigida(event DomainEvent) error { return nil }
