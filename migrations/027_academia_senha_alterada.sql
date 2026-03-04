-- ============================================================================
-- MIGRATION 027 - Suporte ao evento AcademiaSenhaAlterada
-- Data: 2026
-- ============================================================================
--
-- Contexto (FIX C1 da auditoria spuri-backend):
--   AlterarSenha e ResetarSenha faziam UPDATE direto em projection_academias,
--   bypassando o event sourcing. Esta migration suporta a correção:
--   agora ambos emitem o evento AcademiaSenhaAlterada que é gravado no ledger
--   e processado pela AcademiaProjection, garantindo:
--     1. Rastreabilidade completa no ledger (quem/quando/por quê)
--     2. Rebuild restaura a senha correta (não volta à senha original)
--     3. Consistência com AdminSenhaAlterada (migration 023)
--
-- Não há alterações de schema nesta migration porque:
--   1. spuri_ledger já aceita qualquer event_type (validação é no Go)
--   2. projection_academias já tem a coluna senha_hash
--   3. A whitelist de eventos é controlada em safe_queries.go (Go)
-- ============================================================================

BEGIN;

-- ============================================================================
-- 1. Garantir que projection_checkpoints tem
--    entrada para academias (idempotente)
-- ============================================================================

INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at)
VALUES ('academias', 0, CURRENT_TIMESTAMP)
ON CONFLICT (projection_name) DO NOTHING;

-- ============================================================================
-- 2. Comentário informativo na tabela
-- ============================================================================

COMMENT ON TABLE projection_academias IS
    'Projeção de leitura para academias. '
    'Atualizada por eventos: AcademiaCriada, AcademiaAtivada, AcademiaDesativada, '
    'AcademiaDadosAtualizados, CursosAtualizados, EmailVerificado, '
    'AcademiaSenhaAlterada (adicionado migration 027 — FIX C1).';

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 027 - AcademiaSenhaAlterada habilitado no pipeline de eventos';
    RAISE NOTICE '   Whitelist atualizada em safe_queries.go: "AcademiaSenhaAlterada": true';
    RAISE NOTICE '   Handler adicionado em academia_projection.go: handleAcademiaSenhaAlterada';
    RAISE NOTICE '   Rebuild recomendado para garantir consistência:';
    RAISE NOTICE '   POST /admin/rebuild-projection/academias';
END $$;
