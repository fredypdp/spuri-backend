-- ============================================
-- MIGRATION 023 - Suporte ao evento AdminSenhaAlterada
-- Data: 2026
-- ============================================
-- 
-- Contexto:
-- AlterarSenha e ResetarSenha faziam UPDATE direto em projection_admins,
-- bypassando o event sourcing. Esta migration suporta a correção:
-- agora ambos emitem o evento AdminSenhaAlterada que é gravado no ledger
-- e processado pela projeção, garantindo rastreabilidade completa e
-- que o rebuild restaure a senha correta.
--
-- Não há alterações de schema nesta migration porque:
-- 1. spuri_ledger já aceita qualquer event_type (a validação é no Go)
-- 2. projection_admins já tem a coluna senha_hash
-- 3. A whitelist de eventos é controlada em safe_queries.go (Go)
-- ============================================

BEGIN;

-- ============================================
-- 1. Garantir que projection_checkpoints tem
--    entrada para admins (idempotente)
-- ============================================

INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at)
VALUES ('admins', 0, CURRENT_TIMESTAMP)
ON CONFLICT (projection_name) DO NOTHING;

-- ============================================
-- 2. Comentário informativo na tabela
-- ============================================

COMMENT ON TABLE projection_admins IS
    'Projeção de leitura para administradores. '
    'Atualizada por eventos: AdminCriado, AdminAtivado, AdminDesativado, '
    'AcaoAdminRegistrada, AdminDadosAtualizados, AdminRoleAtualizado, '
    'EmailVerificado, AdminSenhaAlterada (adicionado migration 023).';

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 023 - AdminSenhaAlterada habilitado no pipeline de events';
    RAISE NOTICE '   Ação necessária no Go: adicionar "AdminSenhaAlterada": true em safe_queries.go';
END $$;