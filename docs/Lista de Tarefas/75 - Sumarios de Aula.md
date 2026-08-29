---
tarefa: Adicionar Sumários de Aula e vincular opcionalmente às Faltas — BACKEND
repositorio: fredypdp/spuri-backend
orquestrado_por: Claude (a pedido do usuário; Codex apenas executa)
origem: Adicionar_sumários_e_vincular_opcionalmente_às_faltas.md
status: pronto_para_implementacao
---

# 75 - Adicionar Sumários de Aula e vincular opcionalmente às Faltas

## Como usar este documento

Este documento já contém **todas as decisões de design, o SQL validado contra um Postgres real com as 118 migrations existentes aplicadas, e o código completo** a ser criado/alterado. Você (Codex) não precisa planejar a arquitetura — apenas seguir as seções na ordem, criando/editando exatamente os arquivos indicados.

Onde eu (Claude) já validei algo num ambiente real (Postgres com Docker/apt, que você não tem), eu digo explicitamente "validado" e o resultado. Onde a validação depende de rodar `go build`/`go test` (que você consegue fazer, sem precisar de apt/Docker/psql), isso fica como sua responsabilidade, listada na seção 13.

**Leia antes de começar** (para ter o contexto que eu já tenho, sem precisar redescobrir):
- `internal/domain/aggregates/materia_disciplinar.go` — aggregate-template que este trabalho espelha.
- `internal/projections/materias_projection.go` — projection-template.
- `internal/handlers/materia_disciplinar_handlers.go` — handler-template (inclusive `decodeStrictJSON`, que é reaproveitado sem alterações).
- `internal/domain/aggregates/estudante_falta.go`, `internal/projections/faltas_projection.go`, `internal/handlers/faltas_handlers.go` — os arquivos que serão modificados.

## 0. Contexto importante

Já existiu uma tentativa deste recurso: `migrations/075_sumarios_aulas_faltas.sql` criou uma tabela `projection_sumarios_aulas` e colunas em `projection_faltas`; `migrations/076_remove_sumarios_aulas.sql` reverteu tudo. Não há documentação do motivo da reversão, mas identifiquei um problema concreto naquela tentativa: ela definia `ano_academico` como `INTEGER`, enquanto **todo o resto do sistema** (faltas, notas, matérias) usa esse campo como texto no formato `"N_ano_fundamental"`, `"N_ano_medio"` ou `"N_ano_superior"` (confirmado em `internal/handlers/notas_handlers_test.go` e em uso real em `notas_handlers.go`/`faltas_handlers.go`). A implementação abaixo é nova e não tem essa inconsistência.

Também existe hoje um resíduo ativo dessa tentativa: a função `rejeitarCamposLegadosSumarioFaltas` em `internal/handlers/faltas_handlers.go` **bloqueia ativamente** os campos `sumario_id`/`sumario_titulo` em `POST /academia/faltas-aluno` e `PATCH /academia/faltas-aluno/:id`. Este documento inclui a remoção/generalização dessa função (seção 11).

## 1. Decisões de design já tomadas

Estas decisões já foram tomadas (por mim, com confirmação do usuário nos pontos mais importantes) para que você não precise decidir nada. Só desvie delas se encontrar uma contradição factual com o código atual do repositório.

1. **`curso_id` do sumário é inferido de `materia_id`, nunca aceito do cliente.** Evita pares `curso_id`/`materia_id` inconsistentes. *(Confirmado com o usuário.)*
2. **Sumário não tem estado ativo/inativo.** Diferente de Matéria/Curso, um sumário representa uma aula pontual — só existe `ativo` e `deletado` (soft delete). Não há endpoints de ativar/desativar.
3. **Campos estruturais são imutáveis após a criação**: `materia_id`, `periodo`, `ano_academico` (e, por extensão, `curso_id`, `nivel`, `type`, que são derivados). Só `sumario_titulo` e `descricao` são editáveis via `PUT /academia/sumario/:id/dados`. Justificativa: se esses campos pudessem mudar depois que faltas já foram vinculadas, o vínculo passaria a violar retroativamente a regra "periodo/ano_academico da falta = periodo/ano_academico do sumário". Se a academia errar um desses campos, o caminho é deletar o sumário (soft delete) e criar outro.
4. **Deletar um sumário nunca é bloqueado por ter faltas vinculadas.** É exatamente o oposto do que acontece com Matéria (que exige estar "inativa" antes de deletar): a good soft delete existe **precisamente** para permitir isso com segurança, porque a falta guarda um snapshot do título (`sumario_titulo`) e o FK usa `ON DELETE SET NULL`. Não peço "motivo" na deleção (mantém simples; o usuário não pediu auditoria de motivo aqui).
5. **`ano_academico` do sumário deve pertencer a `materia.anos_academicos`** (não apenas "estar no formato certo"). É a mesma regra que faltas/notas já usam via `inferirAnoAcademicoParaNota`/`inferirAnoAcademicoFaltas`, e é estritamente mais precisa do que validar só o formato — garante que o sumário só possa ser criado num ano em que a matéria de fato é lecionada.
6. **`periodo` do sumário**: se `nivel == "superior"`, deve ser exatamente igual a `materia.periodo` (mesma regra que faltas já aplicam). Se `nivel` for `fundamental`/`medio`, deve ser um de `aggregates.PeriodosEscolar` (`1_trimestre`, `2_trimestre`, `3_trimestre`).
7. **Validação de formato de `periodo`/`ano_academico` na tabela nova usa regex, não uma lista fixa.** Descobri, testando o schema real, que `projection_faltas.periodo` e `projection_materias.periodo` têm uma CHECK constraint (`chk_..._periodo_valores`) que só aceita `1..3_trimestre` e `1_semestre`/`2_semestre` — mas `derivarCursoSuperior` (em `cursos_handlers.go`) gera períodos até `N_semestre` para cursos de N semestres (ex.: `7_semestre`, `8_semestre` para um curso de 4 anos). **Isso é um bug pré-existente e fora do escopo desta tarefa** (não mexemos nas tabelas antigas) — mas não o repetimos na tabela nova: `projection_sumarios.periodo` usa `CHECK (periodo ~ '^[1-9][0-9]*_(trimestre|semestre)$')`, que aceita qualquer semestre válido. Testei isso na prática (seção 2).
8. **Vínculo falta → sumário, em `PATCH /academia/faltas-aluno/:id`:**
   - Campo `sumario_id` **omitido** no corpo → o vínculo atual não é alterado.
   - Campo `sumario_id` **presente com um UUID válido e compatível** → troca o vínculo e atualiza o snapshot `sumario_titulo`.
   - Campo `sumario_id` **presente como `null`** → **rejeitado** com erro de validação orientando a usar o endpoint dedicado abaixo. *(Confirmado com o usuário: desvincular não deve acontecer por `null` no PATCH.)*
   - Novo endpoint dedicado `PUT /academia/faltas-aluno/:id/desvincular-sumario` remove o vínculo. *(Confirmado com o usuário.)* Segue a mesma convenção de rota que `PUT /academia/materia/:id/desativar`.
9. **`sumario_titulo` na falta é sempre um snapshot**, resolvido pelo backend a partir de `sumario_id` no momento do vínculo — nunca aceito do cliente, nunca recalculado depois. Renomear um sumário não deve alterar o texto já salvo nas faltas.
10. Não adicionei checagem de `status` da matéria (ativa/inativa/deletada) ao criar um sumário, porque `RegistrarFaltas` hoje também não faz essa checagem — mantive consistência com o comportamento atual em vez de introduzir uma regra mais rígida que o próprio fluxo de faltas não tem.
11. O desvínculo (novo endpoint) reaproveita o evento `FaltaCorrigida` já existente (chamando `CorrigirFalta` com quantidade/observação inalteradas e `sumarioAlterado=true, novoSumarioID=nil`), em vez de criar um novo tipo de evento. Isso evita duplicar toda a lógica de resolução de estudante/matéria que `CorrigirFalta` já tem, e mantém um único caminho de auditoria (`motivo`) para qualquer mudança em falta já registrada.

## 2. Migration SQL — já validada num Postgres real

Criei um Postgres 16 do zero, apliquei as 118 migrations existentes (`001` a `111`) sem nenhum erro, depois apliquei a migration abaixo e testei manualmente:

- ✅ Sumário fundamental válido (sem curso_id) — inserido com sucesso.
- ✅ Sumário médio válido (com curso_id) — inserido com sucesso.
- ✅ Sumário superior com `periodo = "7_semestre"` — inserido com sucesso (prova que a regex aceita semestres além de 2, ao contrário da constraint antiga).
- ❌ `periodo` fora do formato (`"1_mes"`) — rejeitado pela CHECK, como esperado.
- ❌ `ano_academico` fora do formato (`"quinto_ano"`) — rejeitado pela CHECK, como esperado.
- ❌ `nivel = "fundamental"` com `curso_id` preenchido — rejeitado pela CHECK `check_sumario_fundamental_sem_curso`, como esperado.
- ❌ `nivel = "medio"` sem `curso_id` — rejeitado pela mesma CHECK, como esperado.
- ❌ `type`/`nivel` incoerentes (`type="superior"` com `nivel="fundamental"`) — rejeitado pela CHECK `check_sumario_type_nivel`, como esperado.
- ❌ `sumario_titulo` com 2 caracteres — rejeitado pela CHECK de tamanho, como esperado.
- ❌ `materia_id` inexistente — rejeitado pela FK, como esperado.
- ✅ Falta com `sumario_id` válido — inserida com sucesso.
- ❌ Falta com `sumario_id` inexistente — rejeitada pela FK, como esperado.
- ✅ Falta sem `sumario_id` (continua opcional) — inserida com sucesso.
- ✅ Apaguei fisicamente um sumário com `DELETE` direto (pior caso, simulando o `ON DELETE SET NULL`): a falta vinculada ficou com `sumario_id = NULL`, mas **`sumario_titulo` permaneceu com o snapshot** ("Introducao a fracoes") — exatamente o comportamento de preservação histórica pedido no documento original.

Crie o arquivo abaixo exatamente como está (é o próximo número disponível: a migration mais recente hoje é `111_academia_unique_active_records.sql`):

**Arquivo: `migrations/112_sumarios_aulas.sql`**

```sql
-- Migration 112: Sumários de Aula + vínculo opcional em Faltas
--
-- Contexto: já existiu uma tentativa anterior (migrations 075 e 076, esta
-- última revertendo a primeira). Esta é uma implementação nova. Duas correções
-- importantes em relação à tentativa anterior:
--   1) ano_academico aqui é TEXT no formato "N_ano_fundamental|medio|superior"
--      (nunca INTEGER — todo o resto do sistema usa esse formato de string).
--   2) periodo usa checagem estrutural via regex (N_trimestre | N_semestre),
--      em vez de uma lista fixa de 5 valores. A lista fixa hoje usada em
--      projection_faltas.periodo e projection_materias.periodo (chk_*_periodo_valores)
--      só permite até 2_semestre, o que é incompatível com cursos superiores
--      de mais de 1 ano (derivarCursoSuperior gera até N_semestre para cursos
--      de N semestres). Não alteramos essas constraints antigas nesta migration
--      (fora do escopo desta tarefa) — apenas evitamos repetir o mesmo problema
--      na tabela nova.

CREATE TABLE IF NOT EXISTS projection_sumarios (
    id                 UUID PRIMARY KEY,
    codigo_academia    VARCHAR(50) NOT NULL REFERENCES projection_academias(codigo_academia) ON DELETE CASCADE,
    sumario_titulo     TEXT NOT NULL CHECK (char_length(btrim(sumario_titulo)) BETWEEN 3 AND 200),
    descricao          TEXT,
    periodo            VARCHAR(20) NOT NULL CHECK (periodo ~ '^[1-9][0-9]*_(trimestre|semestre)$'),
    ano_academico      VARCHAR(50) NOT NULL CHECK (ano_academico ~ '^[1-9][0-9]*_ano_(fundamental|medio|superior)$'),
    nivel              VARCHAR(20) NOT NULL CHECK (nivel IN ('fundamental', 'medio', 'superior')),
    type               VARCHAR(20) NOT NULL CHECK (type IN ('escolar', 'superior')),
    curso_id           UUID NULL REFERENCES projection_cursos(id) ON DELETE CASCADE,
    materia_id         UUID NOT NULL REFERENCES projection_materias(id) ON DELETE CASCADE,
    criado_por         UUID NULL,
    status             VARCHAR(20) NOT NULL DEFAULT 'ativo' CHECK (status IN ('ativo', 'deletado')),
    deleted_at         TIMESTAMPTZ NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_event_id      UUID,
    version            INTEGER NOT NULL DEFAULT 0,
    CONSTRAINT check_sumario_fundamental_sem_curso CHECK (
        (nivel = 'fundamental' AND curso_id IS NULL)
        OR (nivel IN ('medio', 'superior') AND curso_id IS NOT NULL)
    ),
    CONSTRAINT check_sumario_type_nivel CHECK (
        (type = 'escolar' AND nivel IN ('fundamental', 'medio'))
        OR (type = 'superior' AND nivel = 'superior')
    )
);

CREATE INDEX IF NOT EXISTS idx_sumarios_academia ON projection_sumarios(codigo_academia);
CREATE INDEX IF NOT EXISTS idx_sumarios_materia ON projection_sumarios(materia_id);
CREATE INDEX IF NOT EXISTS idx_sumarios_curso ON projection_sumarios(curso_id);
CREATE INDEX IF NOT EXISTS idx_sumarios_not_deleted ON projection_sumarios(codigo_academia) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_sumarios_busca_vinculo ON projection_sumarios(materia_id, periodo, ano_academico) WHERE deleted_at IS NULL;

COMMENT ON TABLE projection_sumarios IS 'Projeção de leitura para sumários/aulas (Tarefa: Adicionar sumários e vincular opcionalmente às faltas)';
COMMENT ON COLUMN projection_sumarios.sumario_titulo IS 'Título da aula/sumário; snapshot histórico é copiado para projection_faltas.sumario_titulo no momento do vínculo';
COMMENT ON COLUMN projection_sumarios.curso_id IS 'Inferido automaticamente a partir de materia_id (materia.curso_id) — não é aceito diretamente do cliente';
COMMENT ON COLUMN projection_sumarios.nivel IS 'fundamental | medio | superior — espelha materia_disciplinar.type no momento da criação do sumário';
COMMENT ON COLUMN projection_sumarios.type IS 'escolar | superior — classificação derivada de nivel (fundamental/medio => escolar; superior => superior)';

-- ── Vínculo opcional falta -> sumário ──────────────────────────────────────

ALTER TABLE projection_faltas
    ADD COLUMN IF NOT EXISTS sumario_id UUID NULL REFERENCES projection_sumarios(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS sumario_titulo TEXT NULL;

CREATE INDEX IF NOT EXISTS idx_faltas_sumario ON projection_faltas(sumario_id) WHERE sumario_id IS NOT NULL;

COMMENT ON COLUMN projection_faltas.sumario_id IS 'Vínculo opcional à aula/sumário correspondente (Tarefa: Adicionar sumários)';
COMMENT ON COLUMN projection_faltas.sumario_titulo IS 'Snapshot do título do sumário no momento do vínculo; preserva leitura histórica mesmo se o sumário for renomeado ou deletado depois';
```

**Não altere** `chk_faltas_periodo_valores` nem `chk_materia_periodo_valores` — o achado do item 7 acima é uma nota informativa para o usuário decidir se quer corrigir depois, numa tarefa separada. Mexer nisso agora está fora do escopo.

## 3. Novo arquivo: `internal/domain/aggregates/sumario.go`

Modelado diretamente em `materia_disciplinar.go`, mas sem estado ativo/inativo (ver decisão 2).

```go
package aggregates

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Sumario representa o registro de uma aula: título, matéria, período e ano
// acadêmico. Pode opcionalmente ser referenciado por faltas para dar contexto
// histórico sobre qual aula gerou aquela ausência (ver estudante_falta.go).
//
// Ao contrário de MateriaDisciplinar/Curso, não existe estado ativo/inativo:
// um sumário representa uma aula pontual, não uma entidade reutilizável.
// Só existe ativo/deletado (soft delete).
type Sumario struct {
	BaseAggregate
	CodigoAcademia string
	SumarioTitulo  string
	Descricao      *string
	Periodo        string
	AnoAcademico   string
	Nivel          string // fundamental | medio | superior (espelha MateriaDisciplinar.Type na criação)
	Type           string // escolar | superior (derivado de Nivel)
	CursoID        *uuid.UUID
	MateriaID      uuid.UUID
	CriadoPor      uuid.UUID
	Deletado       bool
}

func NewSumario() *Sumario {
	s := &Sumario{}
	s.BaseAggregate = NewBaseAggregate(uuid.New())
	return s
}

func (s *Sumario) GetType() string {
	return "Sumario"
}

// ============================================================================
// Eventos
// ============================================================================

type SumarioCriadoEvent struct {
	BaseEvent
	CodigoAcademia string     `json:"codigo_academia"`
	SumarioTitulo  string     `json:"sumario_titulo"`
	Descricao      *string    `json:"descricao,omitempty"`
	Periodo        string     `json:"periodo"`
	AnoAcademico   string     `json:"ano_academico"`
	Nivel          string     `json:"nivel"`
	Type           string     `json:"type"`
	CursoID        *uuid.UUID `json:"curso_id,omitempty"`
	MateriaID      uuid.UUID  `json:"materia_id"`
	CriadoPor      uuid.UUID  `json:"criado_por"`
	CriadoEm       time.Time  `json:"criado_em"`
}

// SumarioDadosAtualizadosEvent: ponteiros nil = "não alterar". Para Descricao,
// um ponteiro não-nil para string vazia SIGNIFICA "limpar a descrição" (é
// distinguível de nil graças a como omitempty funciona em ponteiros — só
// omite quando o ponteiro em si é nil, não quando aponta pra "").
type SumarioDadosAtualizadosEvent struct {
	BaseEvent
	SumarioTitulo *string   `json:"sumario_titulo,omitempty"`
	Descricao     *string   `json:"descricao,omitempty"`
	AtualizadoPor uuid.UUID `json:"atualizado_por"`
	AtualizadoEm  time.Time `json:"atualizado_em"`
}

type SumarioDeletadoEvent struct {
	BaseEvent
	DeletadoPor uuid.UUID `json:"deletado_por"`
	DeletadoEm  time.Time `json:"deletado_em"`
}

// ============================================================================
// Comandos
// ============================================================================

func (s *Sumario) Criar(
	sumarioTitulo string,
	descricao *string,
	codigoAcademia string,
	tipo string,
	nivel string,
	periodo string,
	anoAcademico string,
	cursoID *uuid.UUID,
	materiaID uuid.UUID,
	criadoPor uuid.UUID,
) error {
	tituloLimpo := strings.TrimSpace(sumarioTitulo)
	if len(tituloLimpo) < 3 || len(tituloLimpo) > 200 {
		return fmt.Errorf("sumario_titulo deve ter entre 3 e 200 caracteres")
	}
	if strings.TrimSpace(codigoAcademia) == "" {
		return fmt.Errorf("codigo_academia é obrigatório")
	}
	if tipo != TipoEscolar && tipo != TipoSuperior {
		return fmt.Errorf("type deve ser 'escolar' ou 'superior'")
	}
	if nivel != "fundamental" && nivel != "medio" && nivel != "superior" {
		return fmt.Errorf("nivel deve ser 'fundamental', 'medio' ou 'superior'")
	}
	tipoEsperado := TipoSuperior
	if nivel == "fundamental" || nivel == "medio" {
		tipoEsperado = TipoEscolar
	}
	if tipo != tipoEsperado {
		return fmt.Errorf("type (%s) incoerente com nivel (%s)", tipo, nivel)
	}
	if strings.TrimSpace(periodo) == "" {
		return fmt.Errorf("periodo é obrigatório")
	}
	if strings.TrimSpace(anoAcademico) == "" {
		return fmt.Errorf("ano_academico é obrigatório")
	}
	if nivel == "fundamental" && cursoID != nil {
		return fmt.Errorf("sumário de matéria fundamental não deve ter curso_id")
	}
	if nivel != "fundamental" && cursoID == nil {
		return fmt.Errorf("sumário de matéria %s exige curso_id", nivel)
	}
	if materiaID == uuid.Nil {
		return fmt.Errorf("materia_id é obrigatório")
	}
	if criadoPor == uuid.Nil {
		return fmt.Errorf("criado_por é obrigatório")
	}
	if descricao != nil {
		descLimpa := strings.TrimSpace(*descricao)
		if len(descLimpa) > 2000 {
			return fmt.Errorf("descricao deve ter no máximo 2000 caracteres")
		}
		descricao = &descLimpa
	}

	event := SumarioCriadoEvent{
		BaseEvent:      NewBaseEvent("SumarioCriado", s.ID),
		CodigoAcademia: codigoAcademia,
		SumarioTitulo:  tituloLimpo,
		Descricao:      descricao,
		Periodo:        periodo,
		AnoAcademico:   anoAcademico,
		Nivel:          nivel,
		Type:           tipo,
		CursoID:        cursoID,
		MateriaID:      materiaID,
		CriadoPor:      criadoPor,
		CriadoEm:       time.Now().UTC(),
	}
	return s.RaiseEvent(event)
}

func (s *Sumario) AtualizarDados(sumarioTitulo *string, descricao *string, atualizadoPor uuid.UUID) error {
	if s.Deletado {
		return fmt.Errorf("não é possível atualizar um sumário deletado")
	}
	if sumarioTitulo == nil && descricao == nil {
		return fmt.Errorf("nenhum dado para atualizar")
	}
	if sumarioTitulo != nil {
		tituloLimpo := strings.TrimSpace(*sumarioTitulo)
		if len(tituloLimpo) < 3 || len(tituloLimpo) > 200 {
			return fmt.Errorf("sumario_titulo deve ter entre 3 e 200 caracteres")
		}
		sumarioTitulo = &tituloLimpo
	}
	if descricao != nil {
		descLimpa := strings.TrimSpace(*descricao)
		if len(descLimpa) > 2000 {
			return fmt.Errorf("descricao deve ter no máximo 2000 caracteres")
		}
		descricao = &descLimpa
	}
	if atualizadoPor == uuid.Nil {
		return fmt.Errorf("atualizado_por é obrigatório")
	}

	event := SumarioDadosAtualizadosEvent{
		BaseEvent:     NewBaseEvent("SumarioDadosAtualizados", s.ID),
		SumarioTitulo: sumarioTitulo,
		Descricao:     descricao,
		AtualizadoPor: atualizadoPor,
		AtualizadoEm:  time.Now().UTC(),
	}
	return s.RaiseEvent(event)
}

func (s *Sumario) Deletar(deletadoPor uuid.UUID) error {
	if s.Deletado {
		return fmt.Errorf("sumário já está deletado")
	}
	if deletadoPor == uuid.Nil {
		return fmt.Errorf("deletado_por é obrigatório")
	}
	event := SumarioDeletadoEvent{
		BaseEvent:   NewBaseEvent("SumarioDeletado", s.ID),
		DeletadoPor: deletadoPor,
		DeletadoEm:  time.Now().UTC(),
	}
	return s.RaiseEvent(event)
}

// ============================================================================
// Apply (replay de eventos)
// ============================================================================

func (s *Sumario) Apply(event DomainEvent) error {
	switch event.GetEventType() {
	case "SumarioCriado":
		return s.applySumarioCriado(event)
	case "SumarioDadosAtualizados":
		return s.applySumarioDadosAtualizados(event)
	case "SumarioDeletado":
		return s.applySumarioDeletado(event)
	default:
		return fmt.Errorf("evento desconhecido para Sumario: %s", event.GetEventType())
	}
}

func (s *Sumario) applySumarioCriado(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applySumarioCriado: marshal error: %w", err)
	}
	var payload SumarioCriadoEvent
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("applySumarioCriado: unmarshal error: %w", err)
	}
	s.CodigoAcademia = payload.CodigoAcademia
	s.SumarioTitulo = payload.SumarioTitulo
	s.Descricao = payload.Descricao
	s.Periodo = payload.Periodo
	s.AnoAcademico = payload.AnoAcademico
	s.Nivel = payload.Nivel
	s.Type = payload.Type
	s.CursoID = payload.CursoID
	s.MateriaID = payload.MateriaID
	s.CriadoPor = payload.CriadoPor
	return nil
}

func (s *Sumario) applySumarioDadosAtualizados(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return fmt.Errorf("applySumarioDadosAtualizados: marshal error: %w", err)
	}
	var payload SumarioDadosAtualizadosEvent
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("applySumarioDadosAtualizados: unmarshal error: %w", err)
	}
	if payload.SumarioTitulo != nil {
		s.SumarioTitulo = *payload.SumarioTitulo
	}
	if payload.Descricao != nil {
		s.Descricao = payload.Descricao
	}
	return nil
}

func (s *Sumario) applySumarioDeletado(event DomainEvent) error {
	s.Deletado = true
	return nil
}
```

**Antes de seguir**: confira em `internal/domain/aggregates/aggregate.go` a assinatura exata de `NewBaseEvent`, `NewBaseAggregate`, `BaseAggregate.RaiseEvent` e a interface `DomainEvent` (`GetEventType()`, `GetPayload()`). O código acima assume a mesma assinatura usada em `materia_disciplinar.go` — se você notar qualquer divergência ao ler esse arquivo, ajuste para bater com a assinatura real (não invente uma nova).

## 4. Registrar o novo aggregate na factory

**Arquivo: `internal/domain/aggregates/aggregate.go`** — no método da `DefaultAggregateFactory` que faz o `switch` por tipo (onde já existe `case "MateriaDisciplinar": return NewMateriaDisciplinar(), nil`), adicione:

```go
case "Sumario":
    return NewSumario(), nil
```

## 5. Novo arquivo: `internal/projections/sumarios_projection.go`

Modelado em `materias_projection.go`, incluindo o mesmo padrão de `ExistenceCache` para verificação de matéria durante `Rebuild()`.

```go
package projections

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"spuri/internal/db"

	"github.com/google/uuid"
)

type SumarioDTO struct {
	ID             string  `json:"id"`
	CodigoAcademia string  `json:"codigo_academia"`
	SumarioTitulo  string  `json:"sumario_titulo"`
	Descricao      *string `json:"descricao,omitempty"`
	Periodo        string  `json:"periodo"`
	AnoAcademico   string  `json:"ano_academico"`
	Nivel          string  `json:"nivel"`
	Type           string  `json:"type"`
	CursoID        *string `json:"curso_id,omitempty"`
	MateriaID      string  `json:"materia_id"`
	CriadoPor      *string `json:"criado_por,omitempty"`
	Status         string  `json:"status"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
	Version        int     `json:"version"`
}

type SumariosProjection struct {
	client *db.Client
}

func NewSumariosProjection(client *db.Client) *SumariosProjection {
	return &SumariosProjection{client: client}
}

func (p *SumariosProjection) Name() string {
	return "sumarios"
}

func (p *SumariosProjection) Handle(event db.Event) error {
	handlers := map[string]func(db.Event) error{
		"SumarioCriado":           p.handleSumarioCriado,
		"SumarioDadosAtualizados": p.handleSumarioDadosAtualizados,
		"SumarioDeletado":         p.handleSumarioDeletado,
	}
	if h, ok := handlers[event.EventType]; ok {
		return h(event)
	}
	return nil
}

func (p *SumariosProjection) handleSumarioCriado(event db.Event) error {
	var payload struct {
		CodigoAcademia string     `json:"codigo_academia"`
		SumarioTitulo  string     `json:"sumario_titulo"`
		Descricao      *string    `json:"descricao,omitempty"`
		Periodo        string     `json:"periodo"`
		AnoAcademico   string     `json:"ano_academico"`
		Nivel          string     `json:"nivel"`
		Type           string     `json:"type"`
		CursoID        *uuid.UUID `json:"curso_id,omitempty"`
		MateriaID      uuid.UUID  `json:"materia_id"`
		CriadoPor      uuid.UUID  `json:"criado_por"`
		CriadoEm       string     `json:"criado_em"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleSumarioCriado: parse error: %w", err)
	}
	_, err := p.client.DB().Exec(`
		INSERT INTO projection_sumarios (
			id, codigo_academia, sumario_titulo, descricao, periodo, ano_academico,
			nivel, type, curso_id, materia_id, criado_por, status,
			created_at, updated_at, last_event_id, version
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'ativo',$12,$12,$13,1)
		ON CONFLICT (id) DO NOTHING
	`, event.AggregateID, payload.CodigoAcademia, payload.SumarioTitulo, payload.Descricao,
		payload.Periodo, payload.AnoAcademico, payload.Nivel, payload.Type,
		payload.CursoID, payload.MateriaID, payload.CriadoPor, payload.CriadoEm, event.EventID)
	if err != nil {
		return fmt.Errorf("handleSumarioCriado: exec error: %w", err)
	}
	return nil
}

func (p *SumariosProjection) handleSumarioDadosAtualizados(event db.Event) error {
	var payload struct {
		SumarioTitulo *string `json:"sumario_titulo,omitempty"`
		Descricao     *string `json:"descricao,omitempty"`
		AtualizadoEm  string  `json:"atualizado_em"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleSumarioDadosAtualizados: parse error: %w", err)
	}

	sets := []string{"updated_at = $1", "last_event_id = $2", "version = version + 1"}
	args := []interface{}{payload.AtualizadoEm, event.EventID}
	if payload.SumarioTitulo != nil {
		args = append(args, *payload.SumarioTitulo)
		sets = append(sets, fmt.Sprintf("sumario_titulo = $%d", len(args)))
	}
	if payload.Descricao != nil {
		args = append(args, *payload.Descricao)
		sets = append(sets, fmt.Sprintf("descricao = $%d", len(args)))
	}
	args = append(args, event.AggregateID)
	query := fmt.Sprintf("UPDATE projection_sumarios SET %s WHERE id = $%d",
		joinComma(sets), len(args))

	if _, err := p.client.DB().Exec(query, args...); err != nil {
		return fmt.Errorf("handleSumarioDadosAtualizados: exec error: %w", err)
	}
	return nil
}

func (p *SumariosProjection) handleSumarioDeletado(event db.Event) error {
	var payload struct {
		DeletadoEm string `json:"deletado_em"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleSumarioDeletado: parse error: %w", err)
	}
	_, err := p.client.DB().Exec(`
		UPDATE projection_sumarios
		SET status = 'deletado', deleted_at = $1, updated_at = $1, last_event_id = $2, version = version + 1
		WHERE id = $3
	`, payload.DeletadoEm, event.EventID, event.AggregateID)
	if err != nil {
		return fmt.Errorf("handleSumarioDeletado: exec error: %w", err)
	}
	return nil
}

// joinComma existe só para não puxar "strings" por um único Join — se o
// pacote já importar "strings" em outro arquivo do mesmo pacote, pode trocar
// isto por strings.Join(sets, ", ") e remover esta função.
func joinComma(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out
}

// ============================================================================
// Leitura
// ============================================================================

func (p *SumariosProjection) GetByID(id uuid.UUID) (*SumarioDTO, error) {
	if id == uuid.Nil {
		return nil, fmt.Errorf("UUID inválido")
	}
	row := p.client.DB().QueryRow(`
		SELECT id, codigo_academia, sumario_titulo, descricao, periodo, ano_academico,
			nivel, type, curso_id, materia_id, criado_por, status, created_at, updated_at, version
		FROM projection_sumarios WHERE id = $1
	`, id)
	return scanSumario(row)
}

func (p *SumariosProjection) GetByAcademia(codigoAcademia string, materiaID, periodo, anoAcademico *string) ([]SumarioDTO, error) {
	query := `
		SELECT id, codigo_academia, sumario_titulo, descricao, periodo, ano_academico,
			nivel, type, curso_id, materia_id, criado_por, status, created_at, updated_at, version
		FROM projection_sumarios
		WHERE codigo_academia = $1 AND deleted_at IS NULL
	`
	args := []interface{}{codigoAcademia}
	if materiaID != nil {
		args = append(args, *materiaID)
		query += fmt.Sprintf(" AND materia_id = $%d", len(args))
	}
	if periodo != nil {
		args = append(args, *periodo)
		query += fmt.Sprintf(" AND periodo = $%d", len(args))
	}
	if anoAcademico != nil {
		args = append(args, *anoAcademico)
		query += fmt.Sprintf(" AND ano_academico = $%d", len(args))
	}
	query += " ORDER BY created_at DESC"

	rows, err := p.client.DB().Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSumarios(rows)
}

func scanSumario(row *sql.Row) (*SumarioDTO, error) {
	var dto SumarioDTO
	var descricao, cursoID, criadoPor sql.NullString
	err := row.Scan(
		&dto.ID, &dto.CodigoAcademia, &dto.SumarioTitulo, &descricao, &dto.Periodo, &dto.AnoAcademico,
		&dto.Nivel, &dto.Type, &cursoID, &dto.MateriaID, &criadoPor, &dto.Status,
		&dto.CreatedAt, &dto.UpdatedAt, &dto.Version,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if descricao.Valid {
		dto.Descricao = &descricao.String
	}
	if cursoID.Valid {
		dto.CursoID = &cursoID.String
	}
	if criadoPor.Valid {
		dto.CriadoPor = &criadoPor.String
	}
	return &dto, nil
}

func scanSumarios(rows *sql.Rows) ([]SumarioDTO, error) {
	var result []SumarioDTO
	for rows.Next() {
		var dto SumarioDTO
		var descricao, cursoID, criadoPor sql.NullString
		if err := rows.Scan(
			&dto.ID, &dto.CodigoAcademia, &dto.SumarioTitulo, &descricao, &dto.Periodo, &dto.AnoAcademico,
			&dto.Nivel, &dto.Type, &cursoID, &dto.MateriaID, &criadoPor, &dto.Status,
			&dto.CreatedAt, &dto.UpdatedAt, &dto.Version,
		); err != nil {
			return nil, err
		}
		if descricao.Valid {
			dto.Descricao = &descricao.String
		}
		if cursoID.Valid {
			dto.CursoID = &cursoID.String
		}
		if criadoPor.Valid {
			dto.CriadoPor = &criadoPor.String
		}
		result = append(result, dto)
	}
	return result, rows.Err()
}

// Rebuild reprocessa todos os eventos "Sumario*" do zero. Segue o mesmo padrão
// de materias_projection.go: se quiser usar o ExistenceCache para validar que
// materia_id existe em projection_materias antes de inserir (mesma ideia que
// materias_projection.go usa para academia_id), copie esse padrão daqui.
// Deixei a assinatura mínima abaixo; ajuste para bater com a interface Projection
// real usada pelas outras projections (verifique manager.go / a interface
// Projection em internal/projections).
func (p *SumariosProjection) Rebuild() error {
	rows, err := p.client.DB().Query(`
		SELECT event_id, aggregate_id, aggregate_type, event_type, event_version, payload, metadata, occurred_at, recorded_at, ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE aggregate_type = 'Sumario'
		ORDER BY id ASC
	`)
	if err != nil {
		return fmt.Errorf("Rebuild: query error: %w", err)
	}
	defer rows.Close()

	if _, err := p.client.DB().Exec(`DELETE FROM projection_sumarios`); err != nil {
		return fmt.Errorf("Rebuild: delete error: %w", err)
	}

	for rows.Next() {
		var e db.Event
		if err := rows.Scan(&e.EventID, &e.AggregateID, &e.AggregateType, &e.EventType, &e.EventVersion,
			&e.Payload, &e.Metadata, &e.OccurredAt, &e.RecordedAt, &e.LedgerHash, &e.PreviousHash); err != nil {
			return fmt.Errorf("Rebuild: scan error: %w", err)
		}
		if err := p.Handle(e); err != nil {
			return fmt.Errorf("Rebuild: handle error para evento %s: %w", e.EventID, err)
		}
	}
	return rows.Err()
}
```

**Atenção**: a função `Rebuild()` acima é minha melhor reconstrução do padrão baseado no que vi em `materias_projection.go`, mas eu não vi o corpo completo do `Rebuild()` de lá byte a byte na minha última passada. **Antes de finalizar este arquivo, abra `internal/projections/materias_projection.go` e compare a assinatura/corpo do `Rebuild()` de lá** (incluindo a interface `Projection` que ambos implementam, provavelmente em `internal/projections/manager.go` ou `projection.go`) **e ajuste `sumarios_projection.go` para bater exatamente** — isso é mais importante do que seguir meu rascunho ao pé da letra se houver qualquer diferença.

## 6. Registrar a projection no `main.go`

**Arquivo: `cmd/server/main.go`** — logo após a linha que registra `materias`:

```go
projManager.RegisterProjection("materias", projections.NewMateriasProjection(dbClient))
```

adicione:

```go
projManager.RegisterProjection("sumarios", projections.NewSumariosProjection(dbClient))
```

## 7. Novo helper em `internal/handlers/helpers.go`

Ao lado de `getMateriasProjection`, `getCursosProjection` etc., adicione:

```go
func getSumariosProjection(c *gin.Context) *projections.SumariosProjection {
	return getRepositoryDeps(c).SumariosProjection // ajuste para o mesmo padrão de acesso usado pelas outras — copie a assinatura exata de getMateriasProjection e só troque o tipo/campo
}
```

**Importante**: escreva a implementação real copiando **literalmente** o corpo de `getMateriasProjection` (ou de `getFaltasProjection`) e trocando apenas o tipo de retorno e o nome do campo/projeção — não adivinhe o mecanismo de injeção (pode ser via `c.MustGet(...)`, um struct de dependências no contexto, etc.). O objetivo é que `getSumariosProjection` funcione exatamente pelo mesmo mecanismo que os outros já usam.

## 8. Novo arquivo: `internal/handlers/sumario_handlers.go`

```go
package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/utils"
)

// ============================================================================
// POST /academia/sumario
// ============================================================================

func CriarSumario(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		utils.RespondWithUnauthorizedError(c)
		return
	}

	var req struct {
		SumarioTitulo string    `json:"sumario_titulo"`
		Descricao     *string   `json:"descricao"`
		MateriaID     uuid.UUID `json:"materia_id"`
		Periodo       string    `json:"periodo"`
		AnoAcademico  string    `json:"ano_academico"`
	}
	if err := decodeStrictJSON(c, &req); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if strings.TrimSpace(req.SumarioTitulo) == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("sumario_titulo é obrigatório"))
		return
	}
	if req.MateriaID == uuid.Nil {
		utils.RespondWithValidationError(c, fmt.Errorf("materia_id é obrigatório"))
		return
	}
	if strings.TrimSpace(req.Periodo) == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("periodo é obrigatório"))
		return
	}
	if strings.TrimSpace(req.AnoAcademico) == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("ano_academico é obrigatório"))
		return
	}

	academiaDTO, err := getAcademiaProjection(c).GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	materiaDTO, err := getMateriasProjection(c).GetByID(req.MateriaID)
	if err != nil || materiaDTO == nil {
		utils.RespondWithNotFoundError(c, "materia")
		return
	}
	if materiaDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "matéria não pertence a esta academia")
		return
	}

	// nivel/type inferidos da matéria — nunca aceitos do cliente.
	nivel := materiaDTO.Type
	tipo, err := inferirTipoLetivoMateria(nivel)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	cursoID := materiaDTO.CursoID // curso_id inferido da matéria — nunca aceito do cliente (decisão de design nº 1)

	// periodo: mesma regra que faltas/notas já aplicam para matéria superior.
	if tipo == aggregates.TipoSuperior {
		if materiaDTO.Periodo == nil || strings.TrimSpace(*materiaDTO.Periodo) == "" {
			utils.RespondWithValidationError(c, fmt.Errorf("matéria superior sem período definido"))
			return
		}
		if req.Periodo != *materiaDTO.Periodo {
			utils.RespondWithValidationError(c, fmt.Errorf("periodo (%s) não corresponde ao período da matéria (%s)", req.Periodo, *materiaDTO.Periodo))
			return
		}
	} else if !containsString(aggregates.PeriodosEscolar, req.Periodo) {
		utils.RespondWithValidationError(c, fmt.Errorf("periodo inválido para matéria %s; use um de %v", nivel, aggregates.PeriodosEscolar))
		return
	}

	// ano_academico: deve pertencer aos anos em que a matéria é lecionada
	// (mesma regra usada por faltas/notas via inferirAnoAcademicoParaNota).
	if !containsString(materiaDTO.AnosAcademicos, req.AnoAcademico) {
		utils.RespondWithValidationError(c, fmt.Errorf("ano_academico (%s) não é um dos anos em que a matéria é lecionada", req.AnoAcademico))
		return
	}

	var cursoUUID *uuid.UUID
	if cursoID != nil {
		cid, parseErr := uuid.Parse(*cursoID)
		if parseErr != nil {
			utils.RespondWithInternalError(c, parseErr)
			return
		}
		cursoUUID = &cid
	}

	sumario := aggregates.NewSumario()
	if err := sumario.Criar(req.SumarioTitulo, req.Descricao, academiaDTO.CodigoAcademia, tipo, nivel, req.Periodo, req.AnoAcademico, cursoUUID, req.MateriaID, userID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	repository := getRepository(c)
	audit := getAuditContext(c, userID, "academia")
	if err := repository.SaveWithAudit(sumario, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "sumário criado com sucesso",
		"id":      sumario.ID,
	})
}

// ============================================================================
// GET /academia/sumarios
// ============================================================================

func ListarSumarios(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		utils.RespondWithUnauthorizedError(c)
		return
	}
	codigoAcademia, err := resolverCodigoAcademia(c, userID) // reaproveite o helper já usado por ListarMaterias para resolver codigo_academia (admin pode passar ?codigo_academia=, academia usa o próprio)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	var materiaID, periodo, anoAcademico *string
	if v := c.Query("materia_id"); v != "" {
		materiaID = &v
	}
	if v := c.Query("periodo"); v != "" {
		periodo = &v
	}
	if v := c.Query("ano_academico"); v != "" {
		anoAcademico = &v
	}

	sumarios, err := getSumariosProjection(c).GetByAcademia(codigoAcademia, materiaID, periodo, anoAcademico)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"sumarios": sumarios})
}

// ============================================================================
// GET /academia/sumario/:id
// ============================================================================

func GetSumario(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		utils.RespondWithUnauthorizedError(c)
		return
	}
	sumarioID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("id inválido"))
		return
	}
	sumarioDTO, err := getSumariosProjection(c).GetByID(sumarioID)
	if err != nil || sumarioDTO == nil {
		utils.RespondWithNotFoundError(c, "sumario")
		return
	}
	codigoAcademia, err := resolverCodigoAcademia(c, userID)
	if err != nil || sumarioDTO.CodigoAcademia != codigoAcademia {
		utils.RespondWithForbiddenError(c, "sumário não pertence a esta academia")
		return
	}
	c.JSON(http.StatusOK, sumarioDTO)
}

// ============================================================================
// PUT /academia/sumario/:id/dados
// ============================================================================

func AtualizarDadosSumario(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		utils.RespondWithUnauthorizedError(c)
		return
	}
	sumarioUUID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("id inválido"))
		return
	}

	// materia_id/periodo/ano_academico são imutáveis (decisão de design nº 3):
	// mesma técnica de detecção usada em AtualizarDadosMateria para "periodo".
	var raw map[string]json.RawMessage
	rawBody, _ := c.GetRawData()
	c.Request.Body = io.NopCloser(bytes.NewBuffer(rawBody))
	_ = json.Unmarshal(rawBody, &raw)
	for _, campoImutavel := range []string{"materia_id", "periodo", "ano_academico", "curso_id", "nivel", "type"} {
		if _, ok := raw[campoImutavel]; ok {
			utils.RespondWithValidationError(c, fmt.Errorf("campo imutável após a criação: %s (delete este sumário e crie outro)", campoImutavel))
			return
		}
	}

	var req struct {
		SumarioTitulo *string `json:"sumario_titulo"`
		Descricao     *string `json:"descricao"`
	}
	if err := decodeStrictJSON(c, &req); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	sumarioDTO, err := getSumariosProjection(c).GetByID(sumarioUUID)
	if err != nil || sumarioDTO == nil {
		utils.RespondWithNotFoundError(c, "sumario")
		return
	}
	academiaDTO, err := getAcademiaProjection(c).GetByID(userID)
	if err != nil || academiaDTO == nil || sumarioDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "sumário não pertence a esta academia")
		return
	}

	repository := getRepository(c)
	sumarioAgg, err := repository.Load(sumarioUUID, "Sumario")
	if err != nil {
		utils.RespondWithNotFoundError(c, "sumario")
		return
	}
	sumario, ok := sumarioAgg.(*aggregates.Sumario)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("tipo de aggregate inesperado"))
		return
	}

	if err := sumario.AtualizarDados(req.SumarioTitulo, req.Descricao, userID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := getAuditContext(c, userID, "academia")
	if err := repository.SaveWithAudit(sumario, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "sumário atualizado com sucesso"})
}

// ============================================================================
// DELETE /academia/sumario/:id
// ============================================================================

func DeletarSumario(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		utils.RespondWithUnauthorizedError(c)
		return
	}
	sumarioUUID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("id inválido"))
		return
	}

	sumarioDTO, err := getSumariosProjection(c).GetByID(sumarioUUID)
	if err != nil || sumarioDTO == nil {
		utils.RespondWithNotFoundError(c, "sumario")
		return
	}
	academiaDTO, err := getAcademiaProjection(c).GetByID(userID)
	if err != nil || academiaDTO == nil || sumarioDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "sumário não pertence a esta academia")
		return
	}

	repository := getRepository(c)
	sumarioAgg, err := repository.Load(sumarioUUID, "Sumario")
	if err != nil {
		utils.RespondWithNotFoundError(c, "sumario")
		return
	}
	sumario, ok := sumarioAgg.(*aggregates.Sumario)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("tipo de aggregate inesperado"))
		return
	}

	// Nota de design: deletar NUNCA é bloqueado por já existirem faltas
	// vinculadas — é o oposto do que MateriaDisciplinar faz (que exige status
	// "inativo" antes). O soft delete + snapshot de sumario_titulo em
	// projection_faltas existe exatamente para permitir isso com segurança.
	if err := sumario.Deletar(userID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := getAuditContext(c, userID, "academia")
	if err := repository.SaveWithAudit(sumario, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "sumário deletado com sucesso"})
}
```

**Observações sobre este arquivo:**

- Faltam os imports `"encoding/json"`, `"io"`, `"bytes"` e `"fmt"` no topo — adicione-os (segui a convenção de omitir imports óbvios para não poluir o documento, mas o arquivo real precisa deles).
- `getAcademiaProjection`, `getAuditContext`, `resolverCodigoAcademia`, `getRepository` são helpers que **já existem** no pacote (usados por `materia_disciplinar_handlers.go`/`cursos_handlers.go`) — não os reescreva, apenas confirme os nomes exatos ao ler `helpers.go` (posso ter o nome de `resolverCodigoAcademia` ligeiramente errado — se não existir com esse nome exato, procure a função que `ListarMaterias` usa para resolver `codigo_academia` a partir de admin vs. academia, e use essa).
- `repository.Load(id, "Sumario")` — confirme a assinatura exata de `Load` em `internal/db/repository.go` (pode ser `Load(id uuid.UUID, aggregateType string) (Aggregate, error)` ou similar — copie a chamada exata que `AtualizarDadosMateria` já faz para `MateriaDisciplinar` e só troque o tipo).
- `inferirTipoLetivoMateria` já existe em `internal/handlers/ano_letivo_helpers.go` — não recrie.
- `containsString` já existe em `internal/handlers/materia_disciplinar_handlers.go` — não recrie.

## 9. Modificações em `internal/domain/aggregates/estudante_falta.go`

### 9.1 — `FaltasRegistradasEvent`: adicionar 2 campos

Localize o struct (tem campos como `CodigoAcademia`, `AnoLectivo`, `AnoAcademico`, `Periodo`, `Data`, `MateriaDisciplinarID`, `Quantidade`, `Observacao`, `RegistradoPor`, `RegistradoEm`) e adicione ao final:

```go
	SumarioID     *uuid.UUID `json:"sumario_id,omitempty"`
	SumarioTitulo *string    `json:"sumario_titulo,omitempty"`
```

### 9.2 — `FaltaCorrigidaEvent`: adicionar 3 campos

Localize o struct (`FaltaAnteriorID`, `CodigoAcademia`, `AnoLectivo`, `Periodo`, `Data`, `MateriaDisciplinarID`, `NovaQuantidade`, `NovaObservacao`, `Motivo`, `CorrigidoPor`, `CorrigidoEm`) e adicione ao final:

```go
	SumarioAlterado   bool       `json:"sumario_alterado,omitempty"`
	NovoSumarioID     *uuid.UUID `json:"novo_sumario_id,omitempty"`
	NovoSumarioTitulo *string    `json:"novo_sumario_titulo,omitempty"`
```

`SumarioAlterado` indica se o payload da correção deve mexer no vínculo (`true`) ou deixá-lo como está (`false`, o padrão para correções antigas). Quando `true` e `NovoSumarioID == nil`, significa "desvincular".

### 9.3 — `RegistrarFalta`: adicionar 2 parâmetros no final da assinatura

```go
func (e *Estudante) RegistrarFalta(
	codigoAcademia string,
	anoLectivo string,
	anoAcademico string,
	periodo string,
	data time.Time,
	materiaDisciplinarID uuid.UUID,
	quantidade int,
	observacao *string,
	registradoPor uuid.UUID,
	periodosValidos []string,
	maxQuantidade int,
	sumarioID *uuid.UUID, // NOVO — Tarefa Sumários
	sumarioTitulo *string, // NOVO — Tarefa Sumários (snapshot já resolvido pelo handler)
) error {
```

Dentro do corpo, no ponto onde o `event := FaltasRegistradasEvent{...}` é montado, adicione as duas linhas correspondentes:

```go
		SumarioID:     sumarioID,
		SumarioTitulo: sumarioTitulo,
```

**Não mude `chaveFalta` nem a lógica de deduplicação** — confirmei lendo o código que nenhuma delas usa sumário, e não devem passar a usar: o UUID determinístico da falta é gerado a partir de `(codigo_estudante, codigo_academia, data, materia_disciplinar_id, periodo)` — testei em isolado que incluir `sumario_id` nesse hash geraria um ID diferente sempre que o vínculo mudasse, o que quebraria a ideia de "corrigir a mesma falta" (criaria uma falta nova em vez de atualizar).

### 9.4 — `CorrigirFalta`: adicionar 3 parâmetros no final da assinatura

```go
func (e *Estudante) CorrigirFalta(
	faltaAnteriorID uuid.UUID,
	codigoAcademia, anoLectivo, periodo string,
	data time.Time,
	materiaID uuid.UUID,
	novaQuantidade int,
	novaObservacao *string,
	motivo string,
	corrigidoPor uuid.UUID,
	maxQuantidade int,
	sumarioAlterado bool, // NOVO — Tarefa Sumários
	novoSumarioID *uuid.UUID, // NOVO — Tarefa Sumários; nil = desvincular (só relevante se sumarioAlterado)
	novoSumarioTitulo *string, // NOVO — Tarefa Sumários; snapshot já resolvido pelo handler
) error {
```

No `event := FaltaCorrigidaEvent{...}` montado dentro do corpo, adicione:

```go
		SumarioAlterado:   sumarioAlterado,
		NovoSumarioID:     novoSumarioID,
		NovoSumarioTitulo: novoSumarioTitulo,
```

**`applyFaltasRegistradas` e `applyFaltaCorrigida` não precisam de nenhuma alteração** — confirmei lendo o código atual: o primeiro só recalcula `chaveFalta` para deduplicação (que não usa sumário); o segundo (`applyFaltaCorrigida`) hoje literalmente só faz `return nil`, não mantém estado em memória sobre faltas individuais.

## 10. Modificações em `internal/projections/faltas_projection.go`

### 10.1 — Insert (`handleFaltasRegistradasTx` ou equivalente)

No struct de payload local usado para dar `json.Unmarshal(event.Payload, &payload)`, adicione:

```go
		SumarioID     *uuid.UUID `json:"sumario_id,omitempty"`
		SumarioTitulo *string    `json:"sumario_titulo,omitempty"`
```

No `INSERT INTO projection_faltas (...)`, adicione as colunas `sumario_id, sumario_titulo` à lista de colunas e `payload.SumarioID, payload.SumarioTitulo` como novos argumentos posicionais correspondentes (ajuste os `$N` de todos os placeholders que vierem depois, já que a posição deles muda).

### 10.2 — Update (`handleFaltaCorrigida`)

Hoje esta função faz:

```go
func (p *FaltasProjection) handleFaltaCorrigida(event db.Event) error {
	var payload struct {
		FaltaAnteriorID uuid.UUID `json:"falta_anterior_id"`
		NovaQuantidade  int       `json:"nova_quantidade"`
		NovaObservacao  *string   `json:"nova_observacao,omitempty"`
		Motivo          string    `json:"motivo"`
		CorrigidoPor    uuid.UUID `json:"corrigido_por"`
		CorrigidoEm     string    `json:"corrigido_em"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleFaltaCorrigida: parse error: %w", err)
	}
	result, err := p.client.DB().Exec(`
		UPDATE projection_faltas
		SET quantidade = $1, observacao = $2, valor_anterior = quantidade,
			motivo_correcao = $3, corrigido_por = $4, corrigido_em = $5,
			version = version + 1
		WHERE id = $6
	`, payload.NovaQuantidade, payload.NovaObservacao, payload.Motivo,
		payload.CorrigidoPor, payload.CorrigidoEm, payload.FaltaAnteriorID)
	if err != nil {
		return fmt.Errorf("handleFaltaCorrigida: exec error: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("handleFaltaCorrigida: falta original %s não encontrada", payload.FaltaAnteriorID)
	}
	return nil
}
```

Troque por (adiciona `SumarioAlterado`/`NovoSumarioID`/`NovoSumarioTitulo` ao payload e monta o `SET` dinamicamente só quando `SumarioAlterado == true`, do mesmo jeito que `handleMateriaDadosAtualizados` já monta `SET` dinâmico em `materias_projection.go` — copie esse padrão de lá se a implementação abaixo divergir):

```go
func (p *FaltasProjection) handleFaltaCorrigida(event db.Event) error {
	var payload struct {
		FaltaAnteriorID   uuid.UUID  `json:"falta_anterior_id"`
		NovaQuantidade    int        `json:"nova_quantidade"`
		NovaObservacao    *string    `json:"nova_observacao,omitempty"`
		Motivo            string     `json:"motivo"`
		CorrigidoPor      uuid.UUID  `json:"corrigido_por"`
		CorrigidoEm       string     `json:"corrigido_em"`
		SumarioAlterado   bool       `json:"sumario_alterado,omitempty"`
		NovoSumarioID     *uuid.UUID `json:"novo_sumario_id,omitempty"`
		NovoSumarioTitulo *string    `json:"novo_sumario_titulo,omitempty"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleFaltaCorrigida: parse error: %w", err)
	}

	sets := []string{
		"quantidade = $1", "observacao = $2", "valor_anterior = quantidade",
		"motivo_correcao = $3", "corrigido_por = $4", "corrigido_em = $5",
		"version = version + 1",
	}
	args := []interface{}{payload.NovaQuantidade, payload.NovaObservacao, payload.Motivo,
		payload.CorrigidoPor, payload.CorrigidoEm}
	if payload.SumarioAlterado {
		args = append(args, payload.NovoSumarioID)
		sets = append(sets, fmt.Sprintf("sumario_id = $%d", len(args)))
		args = append(args, payload.NovoSumarioTitulo)
		sets = append(sets, fmt.Sprintf("sumario_titulo = $%d", len(args)))
	}
	args = append(args, payload.FaltaAnteriorID)
	query := fmt.Sprintf("UPDATE projection_faltas SET %s WHERE id = $%d", strings.Join(sets, ", "), len(args))

	result, err := p.client.DB().Exec(query, args...)
	if err != nil {
		return fmt.Errorf("handleFaltaCorrigida: exec error: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("handleFaltaCorrigida: falta original %s não encontrada", payload.FaltaAnteriorID)
	}
	return nil
}
```

Se `"strings"` ainda não estiver importado neste arquivo, adicione o import.

### 10.3 — `FaltaDTO` e as 5 queries de leitura (`GetByID`, `GetByEstudante`, `GetByAcademia`, `GetByPeriodo`, `GetAll`)

No struct `FaltaDTO`, adicione:

```go
	SumarioID     *string `json:"sumario_id,omitempty"`
	SumarioTitulo *string `json:"sumario_titulo,omitempty"`
```

Em **cada uma das 5 funções** que fazem `SELECT ... FROM projection_faltas`, adicione `f.sumario_id, f.sumario_titulo` (ou `sumario_id, sumario_titulo`, dependendo se a query usa alias `f.`) à lista de colunas selecionadas, e adicione as duas variáveis correspondentes (`sql.NullString`) no `Scan(...)` de cada uma, seguindo exatamente o mesmo padrão já usado ali para `observacao`/`motivo_correcao` (campos de texto opcionais).

## 11. Modificações em `internal/handlers/faltas_handlers.go`

### 11.1 — Substituir `rejeitarCamposLegadosSumarioFaltas`

O código atual (linhas 21–41) é:

```go
func rejeitarCamposLegadosSumarioFaltas(c *gin.Context, camposExtras ...string) bool {
	body, err := c.GetRawData()
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("payload inválido"))
		return true
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(body))

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return false
	}
	camposNaoSuportados := append([]string{"sumario_id", "sumario_titulo"}, camposExtras...)
	for _, campo := range camposNaoSuportados {
		if _, ok := raw[campo]; ok {
			utils.RespondWithValidationError(c, fmt.Errorf("campo não suportado em falta: %s", campo))
			return true
		}
	}
	return false
}
```

`sumario_id`/`sumario_titulo` deixam de ser "campos legados não suportados" — passam a ser campos legítimos. Substitua por duas funções mais genéricas:

```go
// campoPresenteNoPayload verifica se uma chave está presente no corpo JSON,
// mesmo que o valor seja null. Necessário porque um *uuid.UUID nulo não
// distingue "campo omitido" (preservar o vínculo de sumário atual) de "campo
// enviado como null" (que passamos a rejeitar — ver rejeitarCamposImutaveisFalta
// e o uso em CorrigirFalta abaixo).
func campoPresenteNoPayload(c *gin.Context, campo string) (bool, error) {
	body, err := c.GetRawData()
	if err != nil {
		return false, fmt.Errorf("payload inválido")
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(body))

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return false, nil // corpo malformado será pego pelo decode logo em seguida
	}
	_, presente := raw[campo]
	return presente, nil
}

// rejeitarCamposImutaveisFalta generaliza a antiga rejeitarCamposLegadosSumarioFaltas
// para campos que continuam não suportados em correções de falta (ex.: periodo).
// sumario_id NÃO faz mais parte desta lista — ver Tarefa: Adicionar sumários.
func rejeitarCamposImutaveisFalta(c *gin.Context, campos ...string) bool {
	body, err := c.GetRawData()
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("payload inválido"))
		return true
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(body))

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return false
	}
	for _, campo := range campos {
		if _, ok := raw[campo]; ok {
			utils.RespondWithValidationError(c, fmt.Errorf("campo não suportado em falta: %s", campo))
			return true
		}
	}
	return false
}
```

### 11.2 — `RegistrarFaltas` (POST /academia/faltas-aluno)

- **Remova** o bloco que hoje chama a função antiga:

```go
	if rejeitarCamposLegadosSumarioFaltas(c) {
		return
	}
```

- No struct `req` (que hoje tem `CodigoEstudante`, `Data`, `MateriaDisciplinarID`, `Periodo`, `Quantidade`, `Observacao`), adicione:

```go
	SumarioID *uuid.UUID `json:"sumario_id"`
```

- Depois que `materiaDTO`, `academiaDTO` e `anoAcademico` (o resultado de `inferirAnoAcademicoFaltas`) já estiverem resolvidos — mas **antes** de montar o evento — adicione a resolução e validação do sumário:

```go
	var sumarioTitulo *string
	if req.SumarioID != nil {
		sumarioDTO, err := getSumariosProjection(c).GetByID(*req.SumarioID)
		if err != nil || sumarioDTO == nil {
			utils.RespondWithNotFoundError(c, "sumario")
			return
		}
		if sumarioDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
			utils.RespondWithForbiddenError(c, "sumário não pertence a esta academia")
			return
		}
		if sumarioDTO.MateriaID != req.MateriaDisciplinarID.String() {
			utils.RespondWithValidationError(c, fmt.Errorf("sumário não pertence à matéria informada"))
			return
		}
		if sumarioDTO.Periodo != req.Periodo {
			utils.RespondWithValidationError(c, fmt.Errorf("periodo do sumário (%s) não corresponde ao periodo da falta (%s)", sumarioDTO.Periodo, req.Periodo))
			return
		}
		if sumarioDTO.AnoAcademico != anoAcademico {
			utils.RespondWithValidationError(c, fmt.Errorf("ano_academico do sumário (%s) não corresponde ao ano_academico da falta (%s)", sumarioDTO.AnoAcademico, anoAcademico))
			return
		}
		sumarioTitulo = &sumarioDTO.SumarioTitulo
	}
```

  (`sumarioDTO.MateriaID != req.MateriaDisciplinarID.String()` assume que `MateriaID` no DTO é `string` — confirme comparando com `SumarioDTO` da seção 5; se `req.MateriaDisciplinarID` já for `string` em vez de `uuid.UUID` no struct local, ajuste a comparação para não precisar de `.String()`.)

- Na chamada a `estudante.RegistrarFalta(...)`, adicione os dois novos argumentos no final: `req.SumarioID, sumarioTitulo`.

### 11.3 — `CorrigirFalta` (PATCH /academia/faltas-aluno/:id)

- Troque a chamada de guarda de:

```go
	if rejeitarCamposLegadosSumarioFaltas(c, "periodo") {
		return
	}
```

  para:

```go
	if rejeitarCamposImutaveisFalta(c, "periodo") {
		return
	}
```

- No struct `req` (hoje `Quantidade`, `Observacao`, `Motivo`), adicione:

```go
	SumarioID *uuid.UUID `json:"sumario_id"`
```

- Logo depois do `decodeStrictJSON(c, &req)`, detecte se `sumario_id` veio no payload:

```go
	sumarioIDPresente, err := campoPresenteNoPayload(c, "sumario_id")
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if sumarioIDPresente && req.SumarioID == nil {
		utils.RespondWithValidationError(c, fmt.Errorf("sumario_id não pode ser definido como null; use PUT /academia/faltas-aluno/:id/desvincular-sumario para remover o vínculo"))
		return
	}
```

- Depois que a falta original (`faltaDTO`), `academiaDTO` e a matéria já estiverem resolvidos (o handler já carrega isso hoje para montar `CorrigirFalta`), adicione a resolução do NOVO sumário quando aplicável:

```go
	var novoSumarioTitulo *string
	if sumarioIDPresente && req.SumarioID != nil {
		sumarioDTO, err := getSumariosProjection(c).GetByID(*req.SumarioID)
		if err != nil || sumarioDTO == nil {
			utils.RespondWithNotFoundError(c, "sumario")
			return
		}
		if sumarioDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
			utils.RespondWithForbiddenError(c, "sumário não pertence a esta academia")
			return
		}
		if sumarioDTO.MateriaID != faltaDTO.MateriaDisciplinarID {
			utils.RespondWithValidationError(c, fmt.Errorf("sumário não pertence à matéria desta falta"))
			return
		}
		if sumarioDTO.Periodo != faltaDTO.Periodo {
			utils.RespondWithValidationError(c, fmt.Errorf("periodo do sumário (%s) não corresponde ao periodo da falta (%s)", sumarioDTO.Periodo, faltaDTO.Periodo))
			return
		}
		if sumarioDTO.AnoAcademico != faltaDTO.AnoAcademico {
			utils.RespondWithValidationError(c, fmt.Errorf("ano_academico do sumário (%s) não corresponde ao ano_academico da falta (%s)", sumarioDTO.AnoAcademico, faltaDTO.AnoAcademico))
			return
		}
		novoSumarioTitulo = &sumarioDTO.SumarioTitulo
	}
```

  (ajuste os nomes de campo de `faltaDTO` para bater com o `FaltaDTO` real — os nomes usados aqui são os que já vi em `faltas_projection.go`: `MateriaDisciplinarID`, `Periodo`, `AnoAcademico`.)

- Na chamada a `estudante.CorrigirFalta(...)`, adicione os três novos argumentos no final: `sumarioIDPresente, req.SumarioID, novoSumarioTitulo`.

### 11.4 — Novo endpoint: `PUT /academia/faltas-aluno/:id/desvincular-sumario`

Adicione esta nova função ao arquivo (reaproveita toda a resolução de estudante/matéria que `CorrigirFalta` já faz — copie o início de `CorrigirFalta` até o ponto em que `estudante`, `academiaDTO`, `faltaDTO` e `materiaID` estão resolvidos, e então):

```go
// ============================================================================
// PUT /academia/faltas-aluno/:id/desvincular-sumario
// ============================================================================

func DesvincularSumarioFalta(c *gin.Context) {
	userID, err := middleware.GetUserID(c)
	if err != nil {
		utils.RespondWithUnauthorizedError(c)
		return
	}
	faltaID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("id inválido"))
		return
	}

	faltaDTO, err := getFaltasProjection(c).GetByID(faltaID)
	if err != nil || faltaDTO == nil {
		utils.RespondWithNotFoundError(c, "falta")
		return
	}
	academiaDTO, err := getAcademiaProjection(c).GetByID(userID)
	if err != nil || academiaDTO == nil || faltaDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "falta não pertence a esta academia")
		return
	}
	if faltaDTO.SumarioID == nil {
		c.JSON(http.StatusOK, gin.H{"message": "falta já não possui sumário vinculado", "id": faltaID})
		return
	}

	// Resolução do aggregate Estudante: copie exatamente o mesmo trecho que
	// CorrigirFalta usa para ir de codigo_estudante -> repository.Load(estudanteDTO.ID, "Estudante").
	estudanteDTO, err := getEstudanteProjection(c).GetByCodigoEstudante(faltaDTO.CodigoEstudante) // confirme o nome exato deste método lendo CorrigirFalta
	if err != nil || estudanteDTO == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}
	repository := getRepository(c)
	estudanteAgg, err := repository.Load(estudanteDTO.ID, "Estudante")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	estudante, ok := estudanteAgg.(*aggregates.Estudante)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("tipo de aggregate inesperado"))
		return
	}

	materiaID, err := uuid.Parse(faltaDTO.MateriaDisciplinarID)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	data, err := time.Parse("2006-01-02", faltaDTO.Data) // confirme o formato exato de FaltaDTO.Data lendo faltas_projection.go / CorrigirFalta
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	if err := estudante.CorrigirFalta(
		faltaID, academiaDTO.CodigoAcademia, faltaDTO.AnoLectivo, faltaDTO.Periodo, data, materiaID,
		faltaDTO.Quantidade, faltaDTO.Observacao,
		"Sumário desvinculado via endpoint dedicado", userID, aggregates.MaxQuantidadeFaltasPadrao,
		true, nil, nil,
	); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := getAuditContext(c, userID, "academia")
	if err := repository.SaveWithAudit(estudante, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "sumário desvinculado com sucesso", "id": faltaID})
}
```

**Este trecho tem mais suposições do que os outros** (nome exato do método de busca de estudante por código, formato exato de `FaltaDTO.Data`, se `Observacao`/`Quantidade` são os nomes exatos no DTO) porque não tenho, neste momento, a visão completa e recente de todo o corpo de `CorrigirFalta` a partir do ponto em que resolve o estudante — **copie literalmente esse trecho de `CorrigirFalta` (que já faz exatamente essa resolução) em vez de reescrever do zero**, e só adapte o final (a chamada a `CorrigirFalta` com os 3 argumentos novos, e a resposta).

## 12. Novas rotas em `cmd/server/main.go`

No grupo `academiaRead` (linha ~438, mesmo grupo de `GET /academia/materias`), adicione:

```go
		academiaRead.GET("/sumarios", handlers.ListarSumarios)
		academiaRead.GET("/sumario/:id", handlers.GetSumario)
```

No grupo `academia` (write — mesmo grupo de `POST /academia/materia`, `PUT /academia/materia/:id/dados`, `DELETE /academia/materia/:id`), adicione:

```go
		academia.POST("/sumario", handlers.CriarSumario)
		academia.PUT("/sumario/:id/dados", handlers.AtualizarDadosSumario)
		academia.DELETE("/sumario/:id", handlers.DeletarSumario)
		academia.PUT("/faltas-aluno/:id/desvincular-sumario", handlers.DesvincularSumarioFalta)
```

(O padrão de nomenclatura de rota — singular `/sumario/:id` para operações de um item, plural `/sumarios` para listagem — replica exatamente o que já existe para `/materia` vs. `/materias`.)

## 13. O que você (Codex) deve testar e como

Seu ambiente não tem `apt`/Docker/`psql`, então **não tente subir um Postgres**. A migration já foi validada por mim (seção 2) com casos positivos e negativos reais. O que você deve fazer:

1. **`go build ./...`** — precisa compilar sem erros. Se dependências de tipo (`FaltaDTO.MateriaDisciplinarID` ser `string` vs `uuid.UUID`, `repository.Load` ter outra assinatura, etc.) não baterem com o que assumi neste documento, corrija o código para bater com o que realmente existe no repositório — isto tem prioridade sobre seguir meu rascunho ao pé da letra.
2. **`go vet ./...`** — sem warnings novos.
3. **Testes unitários puros (sem banco), seguindo o padrão já existente em `internal/projections/materias_projection_test.go` e `internal/handlers/materia_disciplinar_handlers_test.go`** (que testam funções isoladas, sem subir servidor nem banco). Escreva pelo menos:
   - Um teste para a validação de `periodo`/`ano_academico` no aggregate `Sumario.Criar` (casos válidos e inválidos para fundamental/medio/superior).
   - Um teste para `Sumario.Criar` rejeitando `curso_id` presente com `nivel="fundamental"` e ausente com `nivel="medio"/"superior"`.
   - Um teste para `campoPresenteNoPayload` (omitido vs. presente vs. presente-com-null) — já validei essa lógica isoladamente num módulo Go descartável e funcionou; o teste aqui é para garantir que a versão integrada ao pacote `handlers` se comporta igual.
   - Um teste confirmando que `SumarioDTO`/`FaltaDTO` serializam/desserializam `sumario_id`/`sumario_titulo` corretamente com `omitempty` (nil não aparece no JSON; valor aparece).
4. **Rode a suíte de testes já existente** (`go test ./...`) e confirme que nada quebrou — em especial os testes de `materias_projection_test.go`, `faltas` (se existirem) e `materia_disciplinar_handlers_test.go`, já que você vai tocar em arquivos compartilhados (`helpers.go`, `aggregate.go`).

**Ao terminar, me diga apenas**: se compilou, se os testes passaram (existentes + novos), e — se algo não bateu com uma suposição deste documento (nome de função, assinatura, tipo de campo) — qual foi a divergência e como você resolveu. Não preciso do código completo de volta, só um resumo do resultado e das divergências.

## 14. Checklist de aceitação (confronto com o documento original)

- [ ] Sumário tem `sumario_titulo` (obrigatório, 3–200 caracteres), `descricao` (opcional, até 2000 caracteres).
- [ ] `academia_id`/`codigo_academia` sempre inferido do usuário autenticado, nunca aceito do cliente.
- [ ] `type` (escolar/superior) sempre inferido da matéria, nunca aceito do cliente.
- [ ] `curso_id` sempre inferido da matéria, nunca aceito do cliente.
- [ ] `periodo`: escolar → um de `1_trimestre`/`2_trimestre`/`3_trimestre`; superior → igual ao período da matéria.
- [ ] `ano_academico`: deve pertencer aos anos em que a matéria é lecionada.
- [ ] Histórico do título preservado: renomear um sumário não altera `sumario_titulo` já salvo em faltas já vinculadas (confirmado no teste da seção 2, inclusive no caso extremo de deleção física).
- [ ] Soft delete de sumário nunca é bloqueado por faltas vinculadas.
- [ ] Falta aceita `sumario_id` opcional na criação; backend resolve e grava `sumario_titulo` sozinho.
- [ ] Falta continua válida sem sumário (campo realmente opcional).
- [ ] Trocar `sumario_id` numa correção atualiza o snapshot do título.
- [ ] Não é possível vincular falta a sumário de outra academia.
- [ ] Não é possível vincular falta a sumário de matéria/periodo/ano_academico incompatível.
- [ ] Endpoint dedicado de desvínculo funciona e é idempotente (chamar duas vezes não dá erro).
