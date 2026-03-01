-- ============================================================
-- MIGRATION 017 - Adicionar campo codigo_turma em projection_avaliacao_final
-- ============================================================

BEGIN;

ALTER TABLE projection_avaliacao_final
    ADD COLUMN IF NOT EXISTS codigo_turma VARCHAR(50);

COMMENT ON COLUMN projection_avaliacao_final.codigo_turma IS
    'Código da turma do estudante no momento da avaliação. NULL se não estava em nenhuma turma.';

CREATE INDEX IF NOT EXISTS idx_avf_turma ON projection_avaliacao_final(codigo_turma);

COMMIT;

DO $$ BEGIN RAISE NOTICE '✅ MIGRATION 017 CONCLUÍDA - codigo_turma adicionado em projection_avaliacao_final'; END $$;