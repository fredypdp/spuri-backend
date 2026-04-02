-- ============================================================================
-- MIGRATION 046 — Remover tabelas obsoletas: aprovacao_ano e reprovacoes
-- ============================================================================
--
-- CONTEXTO:
--   O sistema consolidou a avaliação de ano em um único evento:
--   AvaliacaoFinalAnoAcademico → projection_avaliacao_final.
--
--   As tabelas projection_aprovacao_ano e projection_reprovacoes eram
--   populadas pelo evento AprovacaoAnoRegistrada, que foi removido do sistema.
--   A rota POST /academia/aprovacao-ano foi eliminada.
--
--   Com o banco sendo resetado, estas tabelas são simplesmente removidas.
--   A projeção avaliacao_final cobre todos os casos:
--     • aprovado=TRUE  → aprovações
--     • aprovado=FALSE → reprovações
--   Filtros via GET /aprovacoes e GET /reprovacoes continuam funcionando
--   via projection_avaliacao_final.
--
-- AÇÃO REQUERIDA APÓS ESTA MIGRATION:
--   Nenhuma — banco já foi resetado antes desta migration.
-- ============================================================================

BEGIN;

-- 1. Remover views que dependem das tabelas obsoletas (se existirem)
DROP VIEW IF EXISTS v_aprovacoes_completas CASCADE;
DROP VIEW IF EXISTS v_reprovacoes_completas CASCADE;

-- 2. Remover tabelas obsoletas
DROP TABLE IF EXISTS projection_aprovacao_ano CASCADE;
DROP TABLE IF EXISTS projection_reprovacoes CASCADE;

-- 3. Remover checkpoints obsoletos
DELETE FROM projection_checkpoints
WHERE projection_name IN ('aprovacao_ano', 'reprovacoes');

-- 4. Garantir checkpoint para avaliacao_final
INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at)
VALUES ('avaliacao_final', 0, CURRENT_TIMESTAMP)
ON CONFLICT (projection_name) DO NOTHING;

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 046 — projection_aprovacao_ano e projection_reprovacoes removidas';
    RAISE NOTICE '   Avaliações de ano agora gerenciadas exclusivamente por projection_avaliacao_final';
    RAISE NOTICE '   GET /aprovacoes → aprovado=TRUE em projection_avaliacao_final';
    RAISE NOTICE '   GET /reprovacoes → aprovado=FALSE em projection_avaliacao_final';
END $$;
