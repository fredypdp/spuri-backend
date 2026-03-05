-- ============================================================================
-- MIGRATION 014 - Renomear 'nivel' → 'anos_academicos' em projection_materias
-- ============================================================================

BEGIN;

-- 1. Renomear coluna somente se ainda existir como 'nivel'
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'projection_materias'
          AND column_name = 'nivel'
    ) THEN
        ALTER TABLE projection_materias RENAME COLUMN nivel TO anos_academicos;
        RAISE NOTICE 'Coluna nivel renomeada para anos_academicos em projection_materias';
    ELSE
        RAISE NOTICE 'Coluna nivel não existe (já renomeada ou inexistente), pulando';
    END IF;
END $$;

-- 2. Atualizar comentário da coluna
COMMENT ON COLUMN projection_materias.anos_academicos IS
    'Apenas para type=fundamental: array JSON com os anos académicos '
    'que esta matéria cobre. Ex: ["primeiro_fundamental","segundo_fundamental"]. '
    'NULL para medio e superior (que usam CursoID).';

-- 3. Atualizar índice GIN (caso exista baseado no nome antigo)
DROP INDEX IF EXISTS idx_materias_nivel;
CREATE INDEX IF NOT EXISTS idx_materias_anos_academicos
    ON projection_materias USING GIN (anos_academicos)
    WHERE anos_academicos IS NOT NULL;

COMMIT;

DO $$ BEGIN RAISE NOTICE '✅ MIGRATION 014 - nivel → anos_academicos em projection_materias'; END $$;
