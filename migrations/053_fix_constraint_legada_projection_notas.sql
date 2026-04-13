-- ==========================================================================
-- MIGRATION 053 — Remover constraint UNIQUE legada de projection_notas
--
-- Problema observado em produção:
--   duplicate key value violates unique constraint
--   "projection_notas_codigo_estudante_codigo_academia_ano_lecti_key"
--
-- Causa:
--   Alguns ambientes antigos podem manter uma UNIQUE legada baseada em:
--   (codigo_estudante, codigo_academia, ano_lectivo, periodo, materia_disciplinar_id)
--   Essa restrição conflita com o modelo atual (uq_nota_unica) que inclui
--   (tipo, categoria) e não inclui codigo_academia.
--
-- Correção:
--   1) Remove qualquer UNIQUE legada com o conjunto exato de colunas antigas,
--      independentemente do nome real da constraint (incluindo nomes truncados).
--   2) Garante a existência da uq_nota_unica com o conjunto de colunas atual.
-- ==========================================================================

BEGIN;

DO $$
DECLARE
    r RECORD;
BEGIN
    -- Remover todas as UNIQUE legadas por assinatura de colunas (nome-agnóstico)
    FOR r IN
        SELECT c.conname
        FROM pg_constraint c
        JOIN pg_class t ON t.oid = c.conrelid
        WHERE t.relname = 'projection_notas'
          AND c.contype = 'u'
          AND (
              SELECT array_agg(a.attname ORDER BY u.ord)
              FROM unnest(c.conkey) WITH ORDINALITY AS u(attnum, ord)
              JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = u.attnum
          ) = ARRAY[
              'codigo_estudante',
              'codigo_academia',
              'ano_lectivo',
              'periodo',
              'materia_disciplinar_id'
          ]::name[]
    LOOP
        EXECUTE format('ALTER TABLE projection_notas DROP CONSTRAINT %I', r.conname);
        RAISE NOTICE '✅ Constraint UNIQUE legada removida de projection_notas: %', r.conname;
    END LOOP;
END
$$;

-- Garantir constraint atual de idempotência
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint c
        JOIN pg_class t ON t.oid = c.conrelid
        WHERE t.relname = 'projection_notas'
          AND c.conname = 'uq_nota_unica'
          AND c.contype = 'u'
    ) THEN
        ALTER TABLE projection_notas
            ADD CONSTRAINT uq_nota_unica
                UNIQUE (codigo_estudante, ano_lectivo, periodo, materia_disciplinar_id, tipo, categoria);

        RAISE NOTICE '✅ Constraint uq_nota_unica criada em projection_notas';
    ELSE
        RAISE NOTICE 'ℹ️ Constraint uq_nota_unica já existe em projection_notas';
    END IF;
END
$$;

-- Checkpoint técnico da migration
INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at, events_processed)
VALUES ('migration_053', 0, CURRENT_TIMESTAMP, 0)
ON CONFLICT (projection_name) DO NOTHING;

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 053 CONCLUÍDA';
    RAISE NOTICE '   → UNIQUE legada de projection_notas removida (se existia)';
    RAISE NOTICE '   → uq_nota_unica garantida';
    RAISE NOTICE '   → Recomendado: POST /admin/rebuild-projection/notas';
END $$;
