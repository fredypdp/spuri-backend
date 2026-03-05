-- ============================================================================
-- MIGRATION 025 — Constraint única para Bootstrap FPP
--
-- PROBLEMA CORRIGIDO (Issue #3 — auditoria Março 2026):
--   Race condition TOCTOU no BootstrapAdminFPP: duas requisições simultâneas
--   podiam ambas ver `len(admins) == 0` e criar dois admins FPP distintos.
--
-- SOLUÇÃO:
--   1. Unique partial index: no máximo 1 admin com role='fpp' E created_by IS NULL.
--      Isso garante que apenas o admin FPP de bootstrap (sem criador) seja único.
--      Admins FPP criados via RegisterAdmin têm created_by != NULL e não são
--      afetados por este índice.
--
--   2. O advisory lock em Go (pg_advisory_lock) já serializa as requisições.
--      Este índice é a segunda linha de defesa (defense in depth).
--
-- NOTA: Admins FPP criados via /admin/register têm created_by preenchido
--       e NÃO são afetados por este índice — podem existir múltiplos.
-- ============================================================================

BEGIN;

-- Constraint: apenas 1 admin com role='fpp' e sem criador (bootstrap)
CREATE UNIQUE INDEX IF NOT EXISTS idx_bootstrap_fpp_unique
    ON projection_admins (role)
    WHERE created_by IS NULL;

COMMENT ON INDEX idx_bootstrap_fpp_unique IS
    'Garante que apenas um admin FPP de bootstrap (created_by IS NULL) exista. '
    'Segunda linha de defesa contra race condition em BootstrapAdminFPP.';

-- Checkpoint da migration
INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at, events_processed)
VALUES ('migration_025', 0, CURRENT_TIMESTAMP, 0)
ON CONFLICT (projection_name) DO NOTHING;

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 025 — Constraint bootstrap FPP aplicada com sucesso.';
END $$;
