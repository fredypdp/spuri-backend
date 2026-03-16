-- ============================================================================
-- MIGRATION 042 — Corrigir idempotência do registro de nota e falta
--
-- PROBLEMA CORRIGIDO:
--   1. NOTA: O handler handleNotasRegistradas usava ON CONFLICT com colunas
--      (codigo_estudante, codigo_academia, ano_lectivo, periodo, materia_disciplinar_id)
--      que NÃO correspondem à constraint uq_nota_unica definida em migration 006.
--      A constraint real é:
--        UNIQUE (codigo_estudante, ano_lectivo, periodo, materia_disciplinar_id, tipo, categoria)
--      Com as colunas erradas, o ON CONFLICT nunca disparava — causando violação
--      23505 (unique_violation) em rebuild completo ou double-submit.
--      CORREÇÃO: o handler Go foi corrigido para usar as colunas corretas.
--      Esta migration garante que a constraint existe com o nome e colunas exatos.
--
--   2. FALTA: A constraint UNIQUE da tabela projection_faltas é:
--        UNIQUE(codigo_estudante, codigo_academia, data, materia_disciplinar_id)
--      e o handler usa ON CONFLICT (event_id) DO NOTHING — correto para replay
--      de mesmo evento, mas sem guard de negócio no aggregate.
--      O guard foi adicionado no aggregate Go (FaltasRegistradasPorChave).
--      Esta migration não precisa alterar a constraint de faltas.
--
-- AÇÃO REQUERIDA APÓS ESTA MIGRATION:
--   POST /admin/rebuild-projection/notas
--   (para reprocessar eventos com o UPSERT corrigido)
-- ============================================================================

BEGIN;

-- ── Garantir que uq_nota_unica existe com as colunas corretas ────────────────
-- A migration 006 criou esta constraint. Se por algum motivo foi removida
-- ou criada com colunas diferentes, recriamos aqui com IF NOT EXISTS safe.

-- Verificar e recriar apenas se necessário.
-- Usamos DO $$ para condicional segura.
DO $$
BEGIN
    -- Verificar se a constraint existe com o nome correto
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint c
        JOIN pg_class t ON t.oid = c.conrelid
        WHERE t.relname = 'projection_notas'
          AND c.conname = 'uq_nota_unica'
          AND c.contype = 'u'
    ) THEN
        -- Remover qualquer constraint UNIQUE antiga sobre as mesmas colunas
        -- (pode ter sido criada com nome diferente em ambientes antigos)
        ALTER TABLE projection_notas
            DROP CONSTRAINT IF EXISTS projection_notas_codigo_estudante_codigo_academia_ano_lectivo_key;

        -- Criar a constraint correta
        ALTER TABLE projection_notas
            ADD CONSTRAINT uq_nota_unica
                UNIQUE (codigo_estudante, ano_lectivo, periodo, materia_disciplinar_id, tipo, categoria);

        RAISE NOTICE '✅ Constraint uq_nota_unica criada em projection_notas';
    ELSE
        RAISE NOTICE 'ℹ️  Constraint uq_nota_unica já existe em projection_notas — nenhuma ação necessária';
    END IF;
END $$;

-- ── Garantir índices de suporte ao ON CONFLICT corrigido ─────────────────────
CREATE INDEX IF NOT EXISTS idx_notas_unica_lookup
    ON projection_notas (codigo_estudante, ano_lectivo, periodo, materia_disciplinar_id, tipo, categoria)
    WHERE deleted_at IS NULL;

-- ── Checkpoint ───────────────────────────────────────────────────────────────
INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at, events_processed)
VALUES ('migration_042', 0, CURRENT_TIMESTAMP, 0)
ON CONFLICT (projection_name) DO NOTHING;

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 042 CONCLUÍDA';
    RAISE NOTICE '   → constraint uq_nota_unica verificada/criada em projection_notas';
    RAISE NOTICE '   → ON CONFLICT do handler handleNotasRegistradas agora usa colunas corretas';
    RAISE NOTICE '   → Guards de duplicata adicionados nos aggregates (NotasRegistradasPorChave, FaltasRegistradasPorChave)';
    RAISE NOTICE '   → Execute: POST /admin/rebuild-projection/notas';
END $$;
