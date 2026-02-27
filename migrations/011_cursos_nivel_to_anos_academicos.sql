-- ============================================================================
-- MIGRATION 011 - Renomear 'nivel' → 'anos_academicos' em projection_cursos
-- CORRIGIDA: usa bloco DO condicional para evitar erro se coluna já foi renomeada
-- ============================================================================

BEGIN;

-- 1. Renomear coluna somente se ainda existir como 'nivel'
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'projection_cursos'
          AND column_name = 'nivel'
    ) THEN
        ALTER TABLE projection_cursos RENAME COLUMN nivel TO anos_academicos;
        RAISE NOTICE 'Coluna nivel renomeada para anos_academicos';
    ELSE
        RAISE NOTICE 'Coluna nivel não existe (já renomeada ou inexistente), pulando';
    END IF;
END $$;

COMMENT ON COLUMN projection_cursos.anos_academicos IS
    'Array JSON com os anos/níveis do curso definidos pela academia. '
    'Obrigatório para cursos de médio e superior. '
    'Exemplo: ["7ano","8ano","9ano"] ou ["1ano","2ano","3ano","4ano","5ano"]';

-- 2. Atualizar índice GIN
DROP INDEX IF EXISTS idx_cursos_nivel;
CREATE INDEX IF NOT EXISTS idx_cursos_anos_academicos
    ON projection_cursos USING GIN (anos_academicos)
    WHERE anos_academicos IS NOT NULL;

-- 3. NOT NULL apenas se todos os registros estiverem preenchidos
-- Verificar antes: SELECT COUNT(*) FROM projection_cursos WHERE anos_academicos IS NULL;
-- Se retornar 0, pode ativar:
-- ALTER TABLE projection_cursos ALTER COLUMN anos_academicos SET NOT NULL;

COMMIT;

DO $$ BEGIN RAISE NOTICE '✅ MIGRATION 011 - nivel → anos_academicos em projection_cursos'; END $$;