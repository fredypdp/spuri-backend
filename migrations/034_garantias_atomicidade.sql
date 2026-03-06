-- ============================================================================
-- MIGRATION 034 — Garantias de atomicidade retroativas
--
-- DB-12 FIX (auditoria-etapa2-db.md):
--   A migration 001_complete_schema.sql não estava envolvida em BEGIN/COMMIT.
--   Esta migration não pode reescrever a 001 retroativamente (ela já foi
--   aplicada em produção), mas garante que o estado do banco está íntegro
--   verificando os objetos essenciais criados pela 001.
--   Futuros ambientes devem aplicar a 001 em uma conexão com autocommit=off
--   ou dentro de um script de inicialização transacional.
--
-- DB-15 FIX (auditoria-etapa2-db.md):
--   A migration 024_remove_inscricoes_sistema.sql não estava envolvida em
--   BEGIN/COMMIT. Os DROP INDEX e o INSERT INTO projection_checkpoints eram
--   statements independentes. Esta migration garante que os checkpoints
--   esperados pela 024 existem, tornando o estado consistente mesmo que
--   a 024 tenha falhado parcialmente no INSERT.
-- ============================================================================

BEGIN;

-- ============================================================================
-- DB-12: Verificar integridade dos objetos essenciais da migration 001
-- ============================================================================

DO $$
BEGIN
    -- Verificar tabela principal do ledger
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.tables
        WHERE table_name = 'spuri_ledger'
    ) THEN
        RAISE EXCEPTION
            '❌ spuri_ledger não existe — migration 001 pode ter falhado parcialmente. '
            'Execute manualmente: psql -f migrations/001_complete_schema.sql';
    END IF;

    -- Verificar função de hash chain (recriada na migration 020 com assinatura correta)
    IF NOT EXISTS (
        SELECT 1 FROM pg_proc p
        JOIN pg_namespace n ON p.pronamespace = n.oid
        WHERE n.nspname = 'public' AND p.proname = 'verify_hash_chain'
    ) THEN
        RAISE EXCEPTION
            '❌ verify_hash_chain não existe — migration 020 pode não ter sido aplicada. '
            'Execute manualmente: psql -f migrations/020_fix_verify_hash_chain.sql';
    END IF;

    -- Verificar trigger de hash automático
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger
        WHERE tgname = 'trigger_generate_ledger_hash'
    ) THEN
        RAISE EXCEPTION
            '❌ trigger_generate_ledger_hash não existe — migration 001 está incompleta.';
    END IF;

    RAISE NOTICE '✅ Objetos essenciais da migration 001 verificados com sucesso.';
END $$;

-- ============================================================================
-- DB-15: Garantir checkpoints da migration 024 (idempotente)
-- ============================================================================

INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at, events_processed)
VALUES
    ('aprovacao_ano', 0, CURRENT_TIMESTAMP, 0),
    ('reprovacoes',   0, CURRENT_TIMESTAMP, 0)
ON CONFLICT (projection_name) DO NOTHING;

-- Garantir também sistema_config e avaliacao_final (podem ter sido perdidos
-- se migrations 031 e 016 falharam parcialmente)
INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at, events_processed)
VALUES
    ('sistema_config',   0, CURRENT_TIMESTAMP, 0),
    ('avaliacao_final',  0, CURRENT_TIMESTAMP, 0),
    ('categorias_nota',  0, CURRENT_TIMESTAMP, 0),
    ('turmas',           0, CURRENT_TIMESTAMP, 0)
ON CONFLICT (projection_name) DO NOTHING;

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 034 — verificações de integridade e checkpoints garantidos.';
    RAISE NOTICE '   DB-12: objetos da migration 001 verificados.';
    RAISE NOTICE '   DB-15: checkpoints aprovacao_ano, reprovacoes e demais garantidos.';
END $$;
