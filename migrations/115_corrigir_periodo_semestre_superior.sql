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
