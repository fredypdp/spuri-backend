-- ============================================================================
-- MIGRATION 024: Remove sistema de inscrição de estudante em academia
-- ============================================================================
-- CONTEXTO:
--   O sistema passou a cadastrar estudantes DIRETAMENTE vinculados a uma
--   academia (via POST /academia/estudante/register + CriarComVinculo).
--   O fluxo de inscrição (SolicitarInscricao → AprovarInscricao → Vincular)
--   foi removido do código — handlers, aggregate methods e rotas eliminados.
--
-- O QUE ESTA MIGRATION FAZ:
--   1. Preserva a tabela projection_inscricoes (dados históricos do ledger)
--   2. Adiciona comentário de depreciação
--   3. Remove índices de busca frequente (não há mais novas inscrições)
--   4. Garante que anos_academicos existe em projection_academias (scanAcademia)
-- ============================================================================

-- 1. Garantir que a coluna anos_academicos existe em projection_academias
--    (pode já existir em migrations anteriores — IF NOT EXISTS é idempotente)
ALTER TABLE projection_academias
    ADD COLUMN IF NOT EXISTS anos_academicos JSONB DEFAULT '[]'::jsonb;

-- 2. Comentário de depreciação na tabela de inscrições
COMMENT ON TABLE projection_inscricoes IS
    '[DEPRECIADO desde migration 024] '
    'O sistema de inscrição foi removido. '
    'Estudantes são cadastrados diretamente vinculados à academia via EstudanteCriadoComVinculo. '
    'Esta tabela é mantida apenas para preservar dados históricos do ledger.';

-- 3. Remover índices de busca frequente de inscrições (não haverá novas escritas)
--    Usa IF EXISTS para ser idempotente
DROP INDEX IF EXISTS idx_projection_inscricoes_status;
DROP INDEX IF EXISTS idx_projection_inscricoes_academia;
DROP INDEX IF EXISTS idx_projection_inscricoes_estudante;

-- 4. Checkpoint: garantir que aprovacao_ano e reprovacoes estão registrados
INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at, events_processed)
VALUES
    ('aprovacao_ano', 0, CURRENT_TIMESTAMP, 0),
    ('reprovacoes',   0, CURRENT_TIMESTAMP, 0)
ON CONFLICT (projection_name) DO NOTHING;

-- ============================================================================
-- FIM DA MIGRATION 024
-- ============================================================================
