---
tarefa: Corrigir CHECK de periodo para suportar cursos superiores com mais de 2 semestres
repositorio: fredypdp/spuri-backend
status: pronto_para_implementacao
---

# 76 - Corrigir período de matérias/faltas/notas do ensino superior

## Como usar este documento

Esta é uma correção pequena e já validada — só a migration abaixo, sem nenhuma alteração de código Go. Já testei num Postgres real com as 123 migrations atuais do repositório (incluindo a feature de Sumários, já mesclada) e confirmei o resultado. Aplique a migration exatamente como está.

## Contexto

`derivarCursoSuperior` (`internal/handlers/cursos_handlers.go`) gera períodos `1_semestre`..`N_semestre` para um curso de N semestres, sem limite superior — um curso de 4 anos gera até `8_semestre`. Isso já está e sempre esteve correto.

O problema está em três CHECK constraints do banco, criadas antes dessa lógica existir, que só aceitam 5 valores fixos (`1_trimestre`, `2_trimestre`, `3_trimestre`, `1_semestre`, `2_semestre`):

- `chk_materia_periodo_valores` em `projection_materias` (migration `018_materia_periodo.sql`)
- `chk_faltas_periodo_valores` em `projection_faltas` (migration `107_periodo_faltas.sql`)
- `projection_notas_periodo_check` em `projection_notas` (inline em `001_complete_schema.sql`) — **encontrei este terceiro caso ao testar a correção**; é a mesma causa raiz, não uma tarefa separada.

Resultado prático: qualquer matéria, falta ou nota de um curso superior com mais de 1 ano (mais de 2 semestres) é aceita pela aplicação, gravada no ledger (fonte da verdade, imutável) — e depois **falha silenciosamente** ao tentar materializar a linha em `projection_materias`/`projection_faltas`/`projection_notas`. Não há erro visível para o usuário; a leitura fica permanentemente desatualizada para aquele registro específico.

Confirmei que a camada de aplicação em Go **já está correta**: `internal/utils/validation.go` tem uma função `ValidatePeriodo` que já aceita `[número]_semestre` com qualquer número ≥ 1 (comentário no código já cita `"10_semestre"` como exemplo válido), e `resolverPeriodosValidos` (`internal/handlers/notas_handlers.go`) já retorna a lista de períodos do próprio curso (`cursoDTO.Periodos`) sem truncar. Isso bate com o que você mencionou já ter atualizado — a lacuna que restava era exclusivamente esta constraint do banco.

## A migration (já testada)

Crie o arquivo `migrations/115_corrigir_periodo_semestre_superior.sql` com o conteúdo exato abaixo (`115` é o próximo número livre; confirme isso antes de aplicar, caso outra migration tenha sido mesclada entre a criação deste documento e a execução desta tarefa):

```sql
-- Migration 115: Corrigir CHECK de periodo para suportar cursos superiores
-- com mais de 2 semestres (mais de 1 ano).
--
-- Contexto: derivarCursoSuperior (internal/handlers/cursos_handlers.go) gera
-- periodos "1_semestre".."N_semestre" para um curso de N semestres, sem
-- nenhum limite superior. As constraints chk_materia_periodo_valores
-- (migration 018) e chk_faltas_periodo_valores (migration 107) só aceitavam
-- até "2_semestre" — ou seja, qualquer matéria/falta de um curso superior com
-- mais de 1 ano (mais de 2 semestres) falha silenciosamente na camada de
-- projeção: o evento fica gravado no ledger (fonte da verdade, imutável),
-- mas a linha correspondente em projection_materias/projection_faltas nunca
-- é criada/atualizada, deixando a leitura permanentemente desatualizada para
-- aquele registro.
--
-- Correção: trimestre continua fixo em 1..3 (nunca foi derivado de curso —
-- é aggregates.PeriodosEscolar, uma lista fechada). Semestre passa a aceitar
-- qualquer inteiro positivo, o que já bate com o que derivarCursoSuperior
-- gera hoje sem limite. Não há nenhuma alteração de código Go necessária:
-- resolverPeriodosValidos (internal/handlers/notas_handlers.go) já retorna
-- cursoDTO.Periodos diretamente para matérias superiores, então a camada de
-- aplicação já validava corretamente — só a constraint do banco estava
-- defasada.
--
-- Esta migration é puramente aditiva/retrocompatível: só adiciona valores
-- permitidos, nunca remove nenhum dos 5 valores antigos. Nenhuma linha
-- existente pode violar a nova constraint (se já satisfazia a antiga, mais
-- restritiva, satisfaz a nova).
--
-- Ao testar a correção, descobri que projection_notas tem exatamente a mesma
-- constraint restritiva (projection_notas_periodo_check, definida inline em
-- 001_complete_schema.sql), com a mesma causa raiz. Corrigida junto nesta
-- mesma migration por ser o mesmo bug, não um bug separado. Diferença: em
-- projection_notas a coluna periodo é NOT NULL (não precisa do "OR periodo IS NULL").

BEGIN;

ALTER TABLE projection_materias
    DROP CONSTRAINT IF EXISTS chk_materia_periodo_valores;

ALTER TABLE projection_materias
    ADD CONSTRAINT chk_materia_periodo_valores CHECK (
        periodo IS NULL
        OR periodo IN ('1_trimestre', '2_trimestre', '3_trimestre')
        OR periodo ~ '^[1-9][0-9]*_semestre$'
    );

ALTER TABLE projection_faltas
    DROP CONSTRAINT IF EXISTS chk_faltas_periodo_valores;

ALTER TABLE projection_faltas
    ADD CONSTRAINT chk_faltas_periodo_valores CHECK (
        periodo IS NULL
        OR periodo IN ('1_trimestre', '2_trimestre', '3_trimestre')
        OR periodo ~ '^[1-9][0-9]*_semestre$'
    );

COMMENT ON COLUMN projection_materias.periodo IS
    'Período letivo da matéria. Obrigatório para ativar matérias do type=superior. '
    'Trimestre: 1_trimestre..3_trimestre (fixo, ensino escolar). Semestre: '
    'N_semestre, sem limite superior, batendo com o total de períodos definido '
    'no curso vinculado (ver derivarCursoSuperior). NULL para type=fundamental '
    'e type=medio.';

COMMENT ON COLUMN projection_faltas.periodo IS
    'Período do próprio registro de falta. Trimestre: 1_trimestre..3_trimestre '
    '(fixo, ensino escolar). Semestre: N_semestre, sem limite superior, batendo '
    'com o total de períodos definido no curso da matéria. NULL é permitido '
    'somente para registros legados sem período determinístico (ver migration 107).';

ALTER TABLE projection_notas
    DROP CONSTRAINT IF EXISTS projection_notas_periodo_check;

ALTER TABLE projection_notas
    ADD CONSTRAINT projection_notas_periodo_check CHECK (
        periodo IN ('1_trimestre', '2_trimestre', '3_trimestre')
        OR periodo ~ '^[1-9][0-9]*_semestre$'
    );

COMMENT ON COLUMN projection_notas.periodo IS
    'Período da nota. Trimestre: 1_trimestre..3_trimestre (fixo, ensino escolar). '
    'Semestre: N_semestre, sem limite superior, batendo com o total de períodos '
    'definido no curso da matéria.';

COMMIT;
```

## O que eu já validei (Postgres real, do zero, com as 123 migrations atuais)

- ✅ Matéria superior com `periodo = "7_semestre"` (curso de 4 anos/8 semestres) — antes falhava, agora funciona.
- ✅ Falta com `periodo = "8_semestre"` — antes falhava, agora funciona.
- ✅ Nota com `periodo = "8_semestre"` (tabela que eu nem sabia ter o bug até testar) — antes falhava, agora funciona.
- ❌ `periodo = "1_mes"` (formato qualquer) — continua rejeitado nas 3 tabelas, como deveria.
- ❌ `periodo = "4_trimestre"` — continua rejeitado (trimestre continua fechado em 1–3, não veio a mudar).
- ❌ `periodo = "0_semestre"` — continua rejeitado (0 não é inteiro positivo).
- ✅ Os 5 valores originais (`1_trimestre`, `2_trimestre`, `3_trimestre`, `1_semestre`, `2_semestre`) continuam válidos nas 3 tabelas — nenhuma regressão.

## O que você (Codex) precisa fazer

1. Criar o arquivo da migration exatamente como acima.
2. `go build ./...` e `go test ./...` — não deve haver nenhum impacto em código Go, já que a mudança é só no banco e a validação da aplicação já estava correta. Se algum teste existente fixar um valor de período que dependia implicitamente do limite antigo (pouco provável, mas verifique testes de `notas_handlers`, `faltas_handlers` e `cursos_handlers` relacionados a período), ajuste o teste, não a migration.
3. Não é necessário nenhuma alteração em `internal/utils/validation.go`, `internal/handlers/notas_handlers.go` ou `internal/domain/aggregates/curso.go` — já conferi que os três já suportam semestres sem limite.

## Nota à parte, fora do escopo desta tarefa

Reparei que existem hoje dois arquivos de migration com o mesmo número: `112_admin_delecao_event_sourcing.sql` e `112_sumarios_aulas.sql`. Isso não quebra nada tecnicamente (o runner ordena por nome completo do arquivo, então ambos aplicam, só que numa ordem determinada pela ordem alfabética do texto depois do número, não necessariamente a ordem de criação pretendida) — mas pode causar confusão/colisão futura se alguém criar uma nova migration `112_...` sem perceber. Não mexi nisso agora porque é independente do bug de período, só deixo registrado caso você queira renomear um dos dois numa tarefa de organização separada.
