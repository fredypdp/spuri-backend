-- ============================================================================
-- MIGRATION 026: Adicionar motivo_desativacao em projection_academias
-- ============================================================================
-- CONTEXTO (FIX E-10):
--   O evento AcademiaDesativada contém o campo Motivo, mas a projeção não
--   persistia esse valor. Para consultar o motivo era necessário inspecionar
--   o ledger diretamente.
--
--   Esta migration adiciona a coluna motivo_desativacao para que o handler
--   handleAcademiaDesativada possa persistir o motivo na projeção.
-- ============================================================================

BEGIN;

ALTER TABLE projection_academias
    ADD COLUMN IF NOT EXISTS motivo_desativacao TEXT;

COMMENT ON COLUMN projection_academias.motivo_desativacao IS
    'Motivo fornecido pelo admin ao desativar a academia. '
    'NULL quando ativa. Populado pelo evento AcademiaDesativada (migration 026).';

-- Índice opcional para auditoria de desativações
CREATE INDEX IF NOT EXISTS idx_academia_motivo_desativacao
    ON projection_academias(id)
    WHERE motivo_desativacao IS NOT NULL;

-- Garantir checkpoint da projeção academias
INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at, events_processed)
VALUES ('academias', 0, CURRENT_TIMESTAMP, 0)
ON CONFLICT (projection_name) DO NOTHING;

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 026 - motivo_desativacao adicionado a projection_academias';
    RAISE NOTICE '   Ação necessária: executar rebuild da projeção academias para repopular o campo';
    RAISE NOTICE '   POST /admin/rebuild-projection/academias';
END $$;
