-- ============================================================================
-- MIGRATION 043 — Adicionar campo data_nascimento em projection_estudantes
--
-- REGRA DE NEGÓCIO:
--   • data_nascimento é opcional no cadastro.
--   • O valor nunca pode ser >= CURRENT_DATE (a pessoa deve ter nascido no passado).
--   • A constraint no banco é a barreira definitiva contra datas inválidas.
--
-- AÇÃO REQUERIDA APÓS ESTA MIGRATION:
--   POST /admin/rebuild-projection/estudantes
-- ============================================================================

BEGIN;

-- 1. Adicionar coluna (nullable — estudantes existentes não têm data_nascimento)
ALTER TABLE projection_estudantes
    ADD COLUMN IF NOT EXISTS data_nascimento DATE DEFAULT NULL;

-- 2. Constraint: data_nascimento deve ser menor que a data atual (passado)
ALTER TABLE projection_estudantes
    DROP CONSTRAINT IF EXISTS chk_estudante_data_nascimento;

ALTER TABLE projection_estudantes
    ADD CONSTRAINT chk_estudante_data_nascimento CHECK (
        data_nascimento IS NULL
        OR data_nascimento < CURRENT_DATE
    );

-- 3. Índice para buscas por data de nascimento
CREATE INDEX IF NOT EXISTS idx_estudante_data_nascimento
    ON projection_estudantes (data_nascimento)
    WHERE data_nascimento IS NOT NULL;

-- 4. Comentário
COMMENT ON COLUMN projection_estudantes.data_nascimento IS
    'Data de nascimento do estudante. Deve ser anterior à data atual (passado). '
    'Opcional — NULL para estudantes cadastrados antes desta migration.';

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 043 — data_nascimento adicionada em projection_estudantes';
    RAISE NOTICE '   Constraint: data_nascimento < CURRENT_DATE (passado obrigatório)';
    RAISE NOTICE '   Execute POST /admin/rebuild-projection/estudantes se necessário.';
END $$;