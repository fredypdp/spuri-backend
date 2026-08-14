---
criado: 2026-08-13 00:00
origem: Solicitação direta do dono do produto (orquestrado via Claude, execução via Codex)
status: pendente
repositorio: fredypdp/spuri-backend (branch main)
---

# Adicionar `periodo` em Falta, com as mesmas características do período de Nota (pendente)

## Prompt recomendado para executar a atualização

Implemente o campo `periodo` em `Falta` reproduzindo, característica por característica, o que já existe para `periodo` em `Nota` (`internal/domain/aggregates/estudante_notas.go`, `internal/handlers/notas_handlers.go`, `internal/projections/notas_projection.go`): mesmo conjunto fixo de valores (`1_trimestre`/`2_trimestre`/`3_trimestre` para tipo escolar, subconjunto de períodos do curso — hoje `1_semestre`/`2_semestre` — para tipo superior, com o período da falta obrigatoriamente igual ao período fixo já configurado na matéria quando a matéria for do tipo superior), mesma obrigatoriedade no registro, mesmo papel na chave de deduplicação/unicidade, e mesmo comportamento de filtro em consultas. Reaproveite as funções já existentes e compartilhadas entre notas e faltas — `resolverPeriodosValidos`, `inferirTipoLetivoMateria`, `validarPeriodoComLista` (mesmo pacote `aggregates`) — em vez de duplicar lógica. Trate isto como mudança de contrato público breaking (campo passa a ser obrigatório em `POST /academia/faltas-aluno` e em `POST /academia/faltas-aluno/async`), consistente com a política já adotada neste repositório de não manter aliases, wrappers de compatibilidade ou fallback temporário para o comportamento antigo. Ao final, crie a migração de banco necessária (incluindo estratégia explícita e justificada para as faltas já existentes sem `periodo`), atualize testes e `Documentação da API.md`, e corrija a nota da documentação atual que descreve o filtro `periodo` de faltas como "período da matéria disciplinar" — isso deixa de ser verdade após esta tarefa.

## Contexto

`Nota` já tem `periodo` como campo de primeira classe, com todas estas características (todas em `internal/domain/aggregates/estudante_notas.go` e `internal/handlers/notas_handlers.go`):

- obrigatório no registro (`Periodo string \`json:"periodo\" binding:"required"\`` em `RegistrarNota`, handler);
- validado contra uma lista de períodos válidos resolvida por tipo (`resolverPeriodosValidos(c, tipo, cursoID)`):
  - tipo `escolar` → sempre `aggregates.PeriodosEscolar` = `["1_trimestre", "2_trimestre", "3_trimestre"]`;
  - tipo `superior` → `cursoDTO.Periodos` (períodos configurados no curso ao qual a matéria pertence), e, adicionalmente, o período informado deve ser exatamente igual a `*materiaDTO.Periodo` quando a matéria tiver um período fixo definido (`if req.Periodo != *materiaDTO.Periodo { ... }`);
- parte da chave de deduplicação em memória do agregado (`chaveNota`, que inclui `periodo`) e da constraint `uq_nota_unica` no banco (`codigo_estudante, codigo_academia, ano_lectivo, periodo, materia_disciplinar_id, tipo, categoria`, migration `057_unicidade_notas_com_codigo_academia.sql`);
- imutável na correção: `CorrigirNota` deriva `periodo` do registro original (`nota.Periodo`), nunca do corpo da requisição de correção;
- filtrável diretamente em `GET /notas` e `GET /notas-estudante/:codigo` via `matchesFiltroString(filtros.periodos, nota.Periodo)`.

`Falta` (`internal/domain/aggregates/estudante_falta.go`) **não tem** esse campo hoje. O registro de falta só guarda `data` (uma data específica) e é vinculado a `materia_disciplinar_id`; não existe nenhuma noção explícita de trimestre/semestre no próprio registro de falta. Como consequência:

- `GetFaltasEstudante` (`internal/handlers/faltas_handlers.go`) simula um filtro por período indiretamente, buscando o período **fixo da matéria** (`materiaMetaAtual.periodo`, via `getMateriaMeta`) — o que só funciona de forma útil para matérias do tipo superior (que têm um único período fixo). Para matérias escolares com três trimestres, essa aproximação não reflete em qual trimestre a falta realmente ocorreu.
- A documentação atual (`Documentação da API.md`, seção 14, `GET /faltas`) já registra essa limitação explicitamente: *"Em faltas, `periodo` filtra o período da matéria disciplinar."* — esta frase deve ser corrigida como parte desta tarefa.

Importante — **não confundir dois conceitos com nomes parecidos que já coexistem no código**:

1. **"período letivo" (janela de datas do ano letivo)**: validado por `validarDataNoPeriodoLetivo` e mencionado na seção *"Validação de faltas pelo período letivo"* da documentação — isso já existe, é sobre a data cair dentro do ano letivo ativo, e **não muda** nesta tarefa.
2. **"periodo" (trimestre/semestre) da Nota**, que é o conceito a ser replicado em Falta nesta tarefa.

O método `FaltasProjection.GetByPeriodo(codigoEstudante, anoLectivo string, dataInicio, dataFim time.Time)` (`internal/projections/faltas_projection.go`) também usa o nome "período" mas se refere a um **intervalo de datas** de consulta, não ao novo campo `periodo`. Não renomeie essa função como parte desta tarefa (está fora de escopo), mas tome cuidado para não confundir os dois conceitos ao adicionar a nova coluna/campo — nomeie o novo campo exatamente `periodo` (igual a Nota) mesmo que isso conviva com o nome dessa função pré-existente.

`inferirTipoLetivoMateria(materiaDTO.Type)` (`internal/handlers/ano_letivo_helpers.go`) já devolve exatamente `"escolar"` ou `"superior"` a partir do tipo da matéria e já é usado em `RegistrarFaltas` para outra finalidade (`validarDataNoPeriodoLetivo`). Esse mesmo valor pode alimentar diretamente `resolverPeriodosValidos(c, tipoLetivo, materiaDTO.CursoID)`, sem necessidade de nova lógica de inferência de tipo.

## Resumo executivo

| Item | Decisão | Resultado esperado |
| --- | --- | --- |
| Campo novo | `periodo` (string, obrigatório) em `Falta`, mesmo enum de `Nota` | Mesma validação, mesma UX de API |
| Validação | Reaproveita `resolverPeriodosValidos` + regra de período fixo da matéria (tipo superior) | Nenhuma lógica de validação duplicada |
| Unicidade | `periodo` entra na chave em memória (`chaveFalta`) e na constraint `uq_falta_unica` | Simetria com `uq_nota_unica` |
| Correção (`PATCH`) | `periodo` é imutável, derivado do registro original | Simetria com `CorrigirNota` |
| Filtro em consultas | `GET /faltas` e `GET /faltas-estudante/:codigo` passam a filtrar pelo `periodo` do próprio registro | Corrige a limitação hoje documentada |
| Migração | Nova coluna + backfill explícito para dados existentes + `CHECK` + `UNIQUE` | Sem linhas órfãs, sem downtime não planejado |
| Contrato público | `periodo` passa a ser obrigatório em `POST /academia/faltas-aluno` e `POST /academia/faltas-aluno/async` | Mudança breaking documentada e intencional |

---

# 1. Domínio — agregado `Estudante` (faltas)

## Objetivo

Adicionar `Periodo` como parte do comando `RegistrarFalta` e do evento `FaltasRegistradasEvent`, com a mesma validação e o mesmo papel na deduplicação que já existem em `Nota`.

## Escopo obrigatório — `internal/domain/aggregates/estudante_falta.go`

1. Adicionar `Periodo string` a `FaltasRegistradasEvent` e a `FaltaCorrigidaEvent` (mesma posição conceitual de `NotasRegistradasEvent.Periodo` e `NotaCorrigidaEvent.Periodo`).
2. Atualizar `chaveFalta` para incluir `periodo` na composição da chave, mantendo a ordem coerente com a nova constraint do banco (ver seção 4):
   ```go
   func chaveFalta(codigoEstudante, codigoAcademia, anoLectivo, periodo string, data time.Time, materiaID uuid.UUID) string
   ```
3. Atualizar a assinatura de `RegistrarFalta` para receber `periodo string` e `periodosValidos []string` (mesmo padrão de `RegistrarNota`), e validar com `validarPeriodoComLista(periodo, periodosValidos)` — função já existente em `estudante_notas.go`, no mesmo pacote `aggregates`, reaproveitável diretamente sem duplicação.
4. Atualizar `CorrigirFalta` para receber `periodo string` (valor derivado do registro original pelo handler, nunca da requisição de correção) e usá-lo na reconstrução da chave para localizar a falta original a corrigir.
5. Atualizar `applyFaltasRegistradas` para persistir `periodo` em `FaltasRegistradasPorChave` (o mapa de deduplicação em memória do agregado), com a nova composição de chave.
6. Atualizar `FaltaCorrigidaEvent`/`applyFaltaCorrigida` para carregar `Periodo` no payload por completude de auditoria, mesmo que a projeção não precise alterar a coluna `periodo` numa correção (ver seção 3).

---

# 2. Handler — `internal/handlers/faltas_handlers.go`

## Objetivo

Expor `periodo` como campo obrigatório em `POST /academia/faltas-aluno`, validá-lo com a mesma regra usada em `POST /academia/notas-aluno`, e propagá-lo corretamente para o agregado e para a resposta.

## Escopo obrigatório

### 2.1 `RegistrarFaltas`

1. Adicionar `Periodo string \`json:"periodo" binding:"required"\`` à struct de request.
2. Incluir `periodo` na validação de campos obrigatórios já existente (mensagem de erro atualizada: `"dados obrigatórios: codigo_estudante, data, periodo, materia_disciplinar_id e quantidade"`).
3. Depois de `materiaDTO` ser carregado e `tipoLetivo` (`inferirTipoLetivoMateria(materiaDTO.Type)`) já calculado — reaproveitando exatamente a variável já existente nesse handler — chamar `resolverPeriodosValidos(c, tipoLetivo, materiaDTO.CursoID)` (função já definida em `notas_handlers.go`, mesmo pacote `handlers`, reaproveitável diretamente).
4. Quando `tipoLetivo == aggregates.TipoSuperior` e `materiaDTO.Periodo != nil`, exigir `req.Periodo == *materiaDTO.Periodo`, replicando a mensagem de erro já usada em `RegistrarNota` ("periodo '%s' invalido para a materia '%s'. Periodo definido: '%s'").
5. Passar `req.Periodo` e `periodosValidos` para `estudante.RegistrarFalta(...)`.
6. Incluir `periodo` e `periodos_validos` na resposta `201`, no mesmo padrão já usado por `RegistrarNota`.

### 2.2 `CorrigirFalta`

1. Depois de carregar `falta` via `getFaltasProjection(c).GetByID(faltaID.String())`, usar `falta.Periodo` (novo campo do DTO, ver seção 3) como o `periodo` passado a `estudante.CorrigirFalta(...)` — nunca aceitar `periodo` no corpo da requisição de correção.
2. Rejeitar explicitamente, com erro de validação, qualquer tentativa de enviar `periodo` no corpo de `PATCH /academia/faltas-aluno/:id` (mesmo padrão defensivo já usado por `rejeitarCamposLegadosSumarioFaltas` para outros campos não suportados nesta rota — pode reaproveitar ou estender essa função).

### 2.3 `GetFaltasEstudante`

Substituir a lógica atual (linhas ~302–320 do arquivo) que resolve `periodo` indiretamente via `getMateriaMeta`/`materiaMetaAtual.periodo` pelo filtro direto no campo do próprio registro, no mesmo padrão de `GetNotasEstudante`:

```go
if !matchesFiltroString(filtros.anoLectivos, falta.AnoLectivo) ||
    !matchesFiltroString(filtros.anoAcademicos, falta.AnoAcademico) ||
    !matchesFiltroString(filtros.periodos, falta.Periodo) ||
    !matchesFiltroString(filtros.materiasDisciplinares, falta.MateriaDisciplinarID) ||
    !matchesFiltroString(filtros.codigosAcademia, falta.CodigoAcademia) {
    continue
}

if len(filtros.cursoIDs) > 0 {
    // mantém a resolução via materiaMeta apenas para curso_id, que continua
    // sendo um atributo da matéria e não do registro de falta
    ...
}
```

`filtros.cursoIDs` continua precisando de `getMateriaMeta` (curso não é um atributo do registro de falta); apenas `filtros.periodos` deixa de depender da matéria.

---

# 3. Projeção — `internal/projections/faltas_projection.go`

## Objetivo

Persistir e expor `periodo` em `projection_faltas`, em paridade total com `projection_notas`.

## Escopo obrigatório

1. `handleFaltasRegistradasTx`: adicionar `Periodo string \`json:"Periodo"\`` ao struct de payload, incluir `periodo` na lista de colunas do `INSERT` e no `ON CONFLICT ON CONSTRAINT uq_falta_unica` (constraint atualizada na migração, seção 4).
2. `FaltaDTO`: adicionar `Periodo string \`json:"periodo"\`` (posição equivalente a `NotaDTO.Periodo`, se existir um tipo análogo em `notas_projection.go` — usar a mesma convenção de nomenclatura de campo/tag JSON).
3. `scanFaltas`: incluir `&f.Periodo` no `Scan`, na posição correspondente à nova coluna.
4. Atualizar o `SELECT` de **todas** as funções de leitura para incluir `f.periodo`: `GetByID`, `GetByEstudante`, `GetByAcademia`, `GetByPeriodo` (a função de intervalo de datas — apenas adicionar a coluna à projeção retornada, sem alterar sua assinatura nem seu propósito) e `GetAll`.
5. `handleFaltaCorrigida`: não precisa alterar a coluna `periodo` (ela é imutável na correção, igual a `nota.Periodo`); apenas garantir que o `UPDATE` continue funcionando sem tocar nessa coluna.
6. `Rebuild()`: nenhuma mudança na query de eventos (já seleciona `FaltasRegistradas` e `FaltaCorrigida`); o `payload` recém-ampliado já traz `Periodo` automaticamente ao ser reprocessado.

## Escopo obrigatório — `internal/handlers/registros_handlers.go` (consulta global `GET /faltas`)

1. Adicionar `Periodo string \`json:"periodo"\`` a `FaltaRegistroResponse`.
2. Incluir `f.periodo` no `SELECT` de `baseQuery` (dentro da função que atende `GET /faltas`) e no `Scan` correspondente.
3. Trocar a chamada `filtros.buildWhereSQL("f", false)` (linha ~231) para `filtros.buildWhereSQL("f", true)` — o parâmetro `includePeriodoRegistro` já existe exatamente para diferenciar "filtrar pelo período do próprio registro" (`true`, hoje só usado por notas) de "filtrar pelo período da matéria" (`false`, comportamento atual de faltas que deixa de ser necessário). Isso alinha faltas ao mesmo mecanismo de filtro já usado por notas, sem necessidade de lógica nova em `buildWhereSQL`.

---

# 4. Migração de banco de dados

## Objetivo

Criar a migração `107_periodo_faltas.sql` (próximo número disponível após `106_financeiro_matricula.sql`), adicionando a coluna `periodo` a `projection_faltas` com o mesmo formato/`CHECK` de `projection_notas.periodo` (migration `001_complete_schema.sql`, linhas ~326-329, e `018_materia_periodo.sql`), e atualizando a constraint de unicidade.

## Escopo obrigatório

### 4.1 Nova coluna

```sql
ALTER TABLE projection_faltas
    ADD COLUMN IF NOT EXISTS periodo VARCHAR(20);
```

### 4.2 Estratégia de backfill para faltas já existentes — decisão obrigatória

Antes de tornar a coluna `NOT NULL`, definir e documentar explicitamente, no próprio corpo da migração e no PR, qual das estratégias abaixo (mesmo cardápio já usado em `docs/Tarefas feitas/Adicionar campo modelo aos cursos medios.md`) será aplicada:

1. **Backfill determinístico quando possível**: para faltas de matéria `type='superior'`, o período é derivável diretamente de `projection_materias.periodo` (valor único e fixo por matéria) — preencher via `UPDATE ... FROM projection_materias WHERE materia_disciplinar_id = materias.id`. Para faltas de matéria escolar (`fundamental`/`medio`), avaliar se `migrations/086_periodos_letivos_fixos_imutaveis.sql` define uma tabela de janelas de datas por período/ano letivo que permita mapear `data` → `periodo` de forma determinística; se sim, usar essa tabela para o backfill.
2. Para qualquer falta que não puder ser resolvida deterministicamente pela opção 1, **não inventar um valor de período** silenciosamente. Escolher e justificar no PR uma das opções abaixo:
   - manter a coluna `NULL`-ável para as linhas legadas não resolvidas, aplicar a constraint `CHECK` permitindo `NULL` apenas para linhas anteriores à migração (ex.: via constraint condicional ou coluna auxiliar), e tratar isso como dívida técnica explícita a ser resolvida por um script operacional separado; ou
   - exigir revisão manual/script de saneamento antes de aplicar `NOT NULL`, documentando o procedimento.
3. Registros novos, criados a partir da entrada em vigor desta migração, sempre terão `periodo` obrigatório pela aplicação (seção 2), então o problema de backfill é estritamente sobre dados históricos.

### 4.3 `CHECK` e `NOT NULL` (após backfill/saneamento definido em 4.2)

```sql
ALTER TABLE projection_faltas
    DROP CONSTRAINT IF EXISTS chk_faltas_periodo_valores;

ALTER TABLE projection_faltas
    ADD CONSTRAINT chk_faltas_periodo_valores CHECK (
        periodo IN ('1_trimestre', '2_trimestre', '3_trimestre', '1_semestre', '2_semestre')
    );
```

Aplicar `NOT NULL` somente se a estratégia escolhida em 4.2 permitir (isto é, se todas as linhas já tiverem valor válido no momento da migração).

### 4.4 Unicidade

**Importante — verificado diretamente no banco/migrations, corrige uma suposição anterior**: a constraint `uq_falta_unica` **nunca foi alterada** desde que foi recriada pela migration `053_restaurar_unicidade_faltas.sql`; nenhuma migration posterior (incluindo `088_remover_soft_delete_notas_faltas.sql`) voltou a tocá-la. A definição atual, real, é:

```sql
UNIQUE (codigo_estudante, codigo_academia, data, materia_disciplinar_id)
```

Ou seja, **`ano_lectivo` não faz parte da constraint hoje** — apesar de `chaveFalta` (a chave de deduplicação em memória do agregado) já incluir `anoLectivo`. Essa assimetria entre a chave em memória (5 campos) e a constraint do banco (4 campos) é uma característica pré-existente do código, não introduzida por esta tarefa — não a corrija como efeito colateral aqui (fora de escopo); apenas **adicione `periodo` à constraint existente**, preservando a base atual:

```sql
ALTER TABLE projection_faltas
    DROP CONSTRAINT IF EXISTS uq_falta_unica;

ALTER TABLE projection_faltas
    ADD CONSTRAINT uq_falta_unica
        UNIQUE (codigo_estudante, codigo_academia, data, materia_disciplinar_id, periodo);
```

Como o nome da constraint é estável e conhecido (diferente do cenário que a migration 053 enfrentou, em que o nome variava e por isso precisou de busca dinâmica via `pg_constraint`), `DROP CONSTRAINT IF EXISTS uq_falta_unica` direto é suficiente — não é necessário reproduzir a busca dinâmica por nome.

**Não é necessário nenhum passo de consolidação de duplicatas aqui** (diferente do que a migration 053 precisou fazer): adicionar uma coluna a uma `UNIQUE` composta só pode tornar a restrição mais permissiva, nunca menos — qualquer conjunto de linhas que já era único pelas 4 colunas atuais continua trivialmente único ao acrescentar uma 5ª coluna (`periodo`). Não há, portanto, risco de a nova constraint falhar por dados pré-existentes, independentemente de qual estratégia de backfill for escolhida na seção 4.2.

**Também não use `WHERE deleted_at IS NULL`** em nenhuma lógica desta migration — a coluna `deleted_at` de `projection_faltas` foi removida pela migration `088_remover_soft_delete_notas_faltas.sql` (faltas não têm mais exclusão lógica; são um recurso somente de criação/leitura/correção). Referências a esse padrão, presentes na migration 053 por ser anterior à 088, não se aplicam mais ao schema atual.

### 4.5 Índice

Hoje `projection_faltas` tem apenas dois índices simples de consulta, criados pela migration 088: `idx_faltas_estudante_lookup (codigo_estudante)` e `idx_faltas_academia_lookup (codigo_academia)` — não existe um índice composto equivalente a `idx_notas_unica_lookup`. Como `periodo` passa a ser filtro direto em `GET /faltas` e `GET /faltas-estudante/:codigo`, avaliar a criação de um índice composto (ex.: `idx_faltas_periodo_lookup ON projection_faltas (codigo_estudante, codigo_academia, periodo)` ou variação que melhor sirva os filtros combinados já suportados por `filtrosRegistros`) — decisão de performance, não bloqueante para a funcionalidade.

### 4.6 Checkpoint de projeção

Resetar o checkpoint da projeção `faltas` ao final da migração, no mesmo padrão já usado tanto por `057_unicidade_notas_com_codigo_academia.sql` (`INSERT INTO projection_checkpoints (...) VALUES ('notas', 0, CURRENT_TIMESTAMP) ON CONFLICT (projection_name) DO NOTHING`) quanto por `088_remover_soft_delete_notas_faltas.sql` — este é um padrão já estabelecido e consistente no projeto para migrações estruturais deste tipo, não apenas uma possibilidade a confirmar.

---

# 5. Testes obrigatórios

1. registrar falta tipo escolar com `periodo="1_trimestre"` — sucesso;
2. registrar falta tipo escolar com `periodo` fora do conjunto fixo (ex.: `"4_trimestre"`) — `400`;
3. registrar falta tipo escolar sem `periodo` — `400`, mensagem listando o campo como obrigatório;
4. registrar falta tipo superior com `periodo` igual ao período fixo já configurado na matéria — sucesso;
5. registrar falta tipo superior com `periodo` diferente do período fixo da matéria — `400`, mesma mensagem/padrão usado em `RegistrarNota`;
6. `PATCH /academia/faltas-aluno/:id` enviando `periodo` no corpo — rejeitado (campo não suportado nesta rota);
7. `PATCH /academia/faltas-aluno/:id` sem enviar `periodo` — sucesso, `periodo` do registro permanece o do lançamento original;
8. `GET /faltas` com filtro `?periodo=1_trimestre` — retorna somente faltas cujo **próprio registro** tem esse período, não mais inferido pela matéria;
9. `GET /faltas-estudante/:codigo?periodo=2_semestre` — mesmo comportamento acima, para o endpoint por estudante;
10. rebuild da projeção `faltas` (`Rebuild()`) preserva `periodo` corretamente a partir do ledger;
11. duas faltas do mesmo estudante/matéria/data (as quatro colunas que já compõem `uq_falta_unica` hoje) mas com `periodo` diferente — como `periodo` passa a integrar a constraint, ambos os registros são aceitos (cenário de borda, teoricamente raro dado que `periodo` é determinado pela combinação data+matéria; documentar o comportamento esperado no teste mesmo assim);
12. migração `107_periodo_faltas.sql` aplicada sobre uma base com faltas pré-existentes — nenhuma linha órfã/inconsistente ao final, conforme a estratégia de backfill escolhida na seção 4.2;
13. teste de regressão confirmando que `GetByPeriodo` (intervalo de datas) continua funcionando sem alteração de comportamento, apenas retornando `periodo` a mais em cada `FaltaDTO`.

---

# 6. Documentação obrigatória

Atualizar `Documentação da API.md`:

- seção **2.11 Falta** e **2.13 Registro de Falta (consulta global)**: incluir `periodo` no modelo de dados;
- seção **14. Faltas**:
  - `POST /academia/faltas-aluno`: `periodo` como campo obrigatório, exemplo de request/response atualizado, regras de validação (mesmo texto usado para `Nota` na seção 13, adaptado);
  - `PATCH /academia/faltas-aluno/:id`: deixar explícito que `periodo` é derivado do registro original e não é aceito no corpo da correção (mesmo texto já usado para `data`/matéria);
  - `GET /faltas`: **corrigir** a frase *"Em faltas, `periodo` filtra o período da matéria disciplinar."* para refletir que agora filtra o período do próprio registro de falta, no mesmo formato usado por notas;
  - `GET /faltas-estudante/:codigo`: mesma correção de semântica do filtro `periodo`;
  - `POST /academia/faltas-aluno/async`: exemplo de item de array incluindo `periodo`.
- Deixar explícito, em nota de changelog ou nota de rodapé da seção 14, que esta é uma mudança de contrato breaking: chamadas existentes a `POST /academia/faltas-aluno` e `POST /academia/faltas-aluno/async` sem `periodo` passam a ser rejeitadas.

---

# Fora de escopo

- Alterar o significado ou o comportamento de "período letivo" (janela de datas do ano letivo, `validarDataNoPeriodoLetivo`) — conceito diferente, não tocado por esta tarefa.
- Renomear `FaltasProjection.GetByPeriodo` (intervalo de datas) — apenas passa a retornar `periodo` a mais em cada item, sem mudança de assinatura ou propósito.
- Implementar a funcionalidade de reprovação por falta com limite percentual configurável (`docs/Lista de Tarefas/09 - Reprovação por falta com limite percentual configurável.md`) — depende de sumários/aulas e é tarefa independente.
- Adicionar novos valores ao enum de período (`1_trimestre`/`2_trimestre`/`3_trimestre`/`1_semestre`/`2_semestre`) além dos já existentes para `Nota`.

# Critérios de aceite

A tarefa só deve ser considerada concluída quando:

1. `periodo` for obrigatório em `POST /academia/faltas-aluno` e `POST /academia/faltas-aluno/async`, validado com o mesmo enum e as mesmas regras por tipo (escolar/superior) já usadas em `Nota`;
2. `periodo` fizer parte da chave de deduplicação em memória do agregado e da constraint `uq_falta_unica` no banco;
3. `PATCH /academia/faltas-aluno/:id` não aceitar `periodo` no corpo e preservar o período original do lançamento;
4. `GET /faltas` e `GET /faltas-estudante/:codigo` filtrarem `periodo` a partir do próprio registro de falta, não mais da matéria;
5. a migração `107_periodo_faltas.sql` existir com estratégia de backfill explícita e justificada para faltas pré-existentes, sem deixar linhas órfãs;
6. os testes da seção 5 estiverem implementados e passando, incluindo o rebuild de projeção;
7. `Documentação da API.md` estiver atualizada conforme a seção 6, incluindo a correção da frase hoje desatualizada sobre o filtro `periodo` de faltas.

## Procedimento de conclusão

Ao finalizar a implementação:

1. atualizar o título interno desta tarefa para `# Adicionar periodo em Falta, com as mesmas caracteristicas do periodo de Nota (feito)`;
2. alterar o front matter para `status: feito`;
3. mover este arquivo para `docs/Tarefas feitas/`.
