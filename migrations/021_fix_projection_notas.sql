-- ============================================================================
-- MIGRATION 021 — Alinhamento da projection_notas com o padrão do sistema
--
-- PROBLEMA CORRIGIDO:
--   A NotasProjection estava escutando eventos inexistentes ("NotaRegistrada",
--   "NotaCorrigida", "NotaEliminada") em vez dos eventos reais emitidos pelo
--   aggregate ("NotasRegistradas", "NotaAtualizada"). Resultado: a tabela
--   projection_notas nunca era atualizada.
--
-- TAMBÉM CORRIGIDO:
--   O antigo handleNotaEliminada usava DELETE físico. Esta migration garante
--   que a tabela tem coluna deleted_at para futuros soft-deletes, consistente
--   com o restante do sistema.
--
-- AÇÃO REQUERIDA APÓS ESTA MIGRATION:
--   Execute um rebuild da projeção "notas" via:
--     POST /admin/rebuild-projection/notas
--   Isso vai reprocessar todos os eventos "NotasRegistradas" e "NotaAtualizada"
--   do ledger e popular corretamente a tabela projection_notas.
-- ============================================================================

BEGIN;

-- 1. Adicionar coluna deleted_at (soft delete) se não existir
ALTER TABLE projection_notas
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP DEFAULT NULL;

COMMENT ON COLUMN projection_notas.deleted_at IS
    'Soft delete — preenchido quando a nota é removida logicamente. '
    'Registros com deleted_at != NULL não aparecem nas queries de leitura padrão.';

-- 2. Índice para queries de notas ativas (sem soft delete)
CREATE INDEX IF NOT EXISTS idx_notas_estudante_ativo
    ON projection_notas (codigo_estudante)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_notas_academia_ativo
    ON projection_notas (codigo_academia)
    WHERE deleted_at IS NULL;

-- 3. Verificar e reportar dados existentes na tabela
DO $$
DECLARE
    v_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO v_count FROM projection_notas;

    IF v_count = 0 THEN
        RAISE NOTICE
            '✅ projection_notas está vazia — consistente com o bug corrigido. '
            'Execute POST /admin/rebuild-projection/notas para reprocessar o ledger.';
    ELSE
        RAISE WARNING
            '⚠️  projection_notas contém % registros existentes. '
            'Verifique se foram inseridos manualmente ou por outro mecanismo. '
            'Considere executar POST /admin/rebuild-projection/notas para garantir consistência.',
            v_count;
    END IF;
END $$;

-- 4. Atualizar checkpoint da projeção notas para 0,
--    forçando reprocessamento completo no próximo rebuild.
--    (O rebuild via HTTP já faz TRUNCATE + reprocessamento, mas este
--     UPDATE garante que o polling também reprocesse do início se necessário.)
UPDATE projection_checkpoints
SET last_processed_event_id = 0,
    last_processed_at       = CURRENT_TIMESTAMP
WHERE projection_name = 'notas';

-- Se não existir o checkpoint, criar
INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at, events_processed)
VALUES ('notas', 0, CURRENT_TIMESTAMP, 0)
ON CONFLICT (projection_name) DO NOTHING;

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 021 — projection_notas alinhada. Execute rebuild da projeção notas.';
END $$;
