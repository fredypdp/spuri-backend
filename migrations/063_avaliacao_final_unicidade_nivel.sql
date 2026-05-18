-- ============================================================
-- MIGRATION 063 - Unicidade de avaliação final por nível
-- ============================================================

BEGIN;

-- Além da decisão única por ano letivo, garante explicitamente que a mesma
-- Avaliação Final Escolar/Superior não seja duplicada para o mesmo nível.
WITH duplicadas AS (
    SELECT
        id,
        ROW_NUMBER() OVER (
            PARTITION BY
                codigo_estudante,
                codigo_academia,
                ano_lectivo,
                CASE WHEN tipo_ensino = 'superior' THEN 'superior' ELSE 'escolar' END,
                ano_academico_atual
            ORDER BY registered_at ASC, event_id ASC
        ) AS rn
    FROM projection_avaliacao_final
)
DELETE FROM projection_avaliacao_final avf
USING duplicadas d
WHERE avf.id = d.id
  AND d.rn > 1;

CREATE UNIQUE INDEX IF NOT EXISTS idx_avf_unico_estudante_ano_letivo_grupo_nivel
    ON projection_avaliacao_final (
        codigo_estudante,
        codigo_academia,
        ano_lectivo,
        (CASE WHEN tipo_ensino = 'superior' THEN 'superior' ELSE 'escolar' END),
        ano_academico_atual
    );

COMMENT ON INDEX idx_avf_unico_estudante_ano_letivo_grupo_nivel IS
    'Impede Avaliação Final Escolar/Superior duplicada do mesmo estudante no mesmo nível.';

COMMIT;

DO $$ BEGIN RAISE NOTICE '✅ MIGRATION 063 CONCLUÍDA - avaliação final única por nível'; END $$;
