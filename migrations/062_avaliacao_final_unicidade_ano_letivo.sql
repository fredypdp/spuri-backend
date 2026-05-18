-- ============================================================
-- MIGRATION 062 - Unicidade de avaliação final por ano letivo
-- ============================================================

BEGIN;

-- A avaliação final é a decisão única do ano letivo para o estudante.
-- Caso existam duplicidades históricas na projeção, mantém o primeiro registro
-- projetado e remove os demais para permitir a nova garantia de unicidade.
WITH duplicadas AS (
    SELECT
        id,
        ROW_NUMBER() OVER (
            PARTITION BY codigo_estudante, codigo_academia, ano_lectivo
            ORDER BY registered_at ASC, event_id ASC
        ) AS rn
    FROM projection_avaliacao_final
)
DELETE FROM projection_avaliacao_final avf
USING duplicadas d
WHERE avf.id = d.id
  AND d.rn > 1;

CREATE UNIQUE INDEX IF NOT EXISTS idx_avf_unico_estudante_ano_letivo
    ON projection_avaliacao_final (codigo_estudante, codigo_academia, ano_lectivo);

COMMENT ON INDEX idx_avf_unico_estudante_ano_letivo IS
    'Impede aprovação/reprovação duplicada do mesmo estudante no mesmo ano letivo.';

COMMIT;

DO $$ BEGIN RAISE NOTICE '✅ MIGRATION 062 CONCLUÍDA - avaliação final única por estudante/ano letivo'; END $$;
