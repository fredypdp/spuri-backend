-- ============================================================================
-- MIGRATION 011 - Renomear 'nivel' → 'anos_academicos' em projection_cursos
--
-- MOTIVO: O requisito define o campo como 'AnosAcademicos' tanto na Academia
-- quanto em cada Curso. O aggregate Curso usava 'Nivel' internamente, mas para
-- conformidade semântica com o spec, todos os layers (aggregate, eventos,
-- projeção, API) passam a usar 'anos_academicos'.
-- ============================================================================

BEGIN;

-- 1. Renomear coluna na projection_cursos
ALTER TABLE projection_cursos
    RENAME COLUMN nivel TO anos_academicos;

COMMENT ON COLUMN projection_cursos.anos_academicos IS
    'Array JSON com os anos/níveis do curso definidos pela academia. '
    'Obrigatório para cursos de médio e superior. '
    'Exemplo: ["7ano","8ano","9ano"] ou ["1ano","2ano","3ano","4ano","5ano"]';

-- 2. Atualizar índice GIN se existir
DROP INDEX IF EXISTS idx_cursos_nivel;
CREATE INDEX IF NOT EXISTS idx_cursos_anos_academicos
    ON projection_cursos USING GIN (anos_academicos)
    WHERE anos_academicos IS NOT NULL;

-- 3. Garantir constraint NOT NULL (os cursos existentes já devem ter valor)
ALTER TABLE projection_cursos
    ALTER COLUMN anos_academicos SET NOT NULL;

-- 4. Atualizar view v_estudantes_com_cursos (inclui nome dos cursos, sem expor nivel/anos)
-- Essa view não expõe a coluna anos_academicos diretamente, então nenhuma mudança é necessária lá.

-- 5. Atualizar ValidateTableName em safe_queries.go se projection_cursos já estiver lá.
--    Nenhuma alteração de nome de tabela — apenas de coluna — então não há impacto.

COMMIT;

DO $$ BEGIN RAISE NOTICE '✅ MIGRATION 011 - nivel → anos_academicos em projection_cursos'; END $$;
