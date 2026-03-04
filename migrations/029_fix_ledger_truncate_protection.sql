-- ============================================================================
-- MIGRATION 029 — Proteção de spuri_ledger contra TRUNCATE
-- ============================================================================
--
-- CONTEXTO (ERRO-MIG-04 da auditoria-etapa2-db.md):
--   Os triggers prevent_update_ledger e prevent_delete_ledger cobrem apenas
--   UPDATE e DELETE (FOR EACH ROW). TRUNCATE em PostgreSQL não dispara triggers
--   FOR EACH ROW — requer trigger FOR EACH STATEMENT com a cláusula TRUNCATE.
--   Sem este trigger, qualquer usuário com permissão TRUNCATE pode apagar
--   todo o ledger sem ser interceptado pelos controles de imutabilidade.
--
-- O QUE ESTA MIGRATION FAZ:
--   1. Cria a função prevent_ledger_truncate() que lança exceção.
--   2. Cria o trigger prevent_truncate_ledger em BEFORE TRUNCATE FOR EACH STATEMENT.
--
-- Idempotente: usa CREATE OR REPLACE para a função e DROP IF EXISTS para o trigger.
-- ============================================================================

BEGIN;

-- ============================================================================
-- 1. Função que bloqueia TRUNCATE
-- ============================================================================

CREATE OR REPLACE FUNCTION prevent_ledger_truncate()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION
        'Spuri Ledger é imutável. TRUNCATE não permitido. '
        'Use rebuild de projeção se precisar reprocessar eventos.';
    -- RETURN NULL é necessário sintaticamente mas nunca é atingido.
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION prevent_ledger_truncate() IS
    'Bloqueia qualquer tentativa de TRUNCATE em spuri_ledger. '
    'Complementa os triggers prevent_update_ledger e prevent_delete_ledger '
    'que cobrem apenas UPDATE e DELETE por linha.';

-- ============================================================================
-- 2. Trigger BEFORE TRUNCATE — nível de statement (único modo possível para TRUNCATE)
-- ============================================================================

DROP TRIGGER IF EXISTS prevent_truncate_ledger ON spuri_ledger;

CREATE TRIGGER prevent_truncate_ledger
    BEFORE TRUNCATE ON spuri_ledger
    FOR EACH STATEMENT
    EXECUTE FUNCTION prevent_ledger_truncate();

COMMENT ON TRIGGER prevent_truncate_ledger ON spuri_ledger IS
    'Impede TRUNCATE no ledger imutável. '
    'Junto com prevent_update_ledger e prevent_delete_ledger, '
    'garante que nenhuma operação DML pode modificar ou apagar eventos gravados.';

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 029 — Proteção TRUNCATE adicionada ao spuri_ledger';
    RAISE NOTICE '   Trigger: prevent_truncate_ledger (BEFORE TRUNCATE FOR EACH STATEMENT)';
    RAISE NOTICE '   Agora UPDATE, DELETE e TRUNCATE são todos bloqueados no ledger.';
END $$;
