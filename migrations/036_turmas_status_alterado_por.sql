-- ============================================================================
-- MIGRATION 036 — Auditoria de ativação/desativação de turmas
--
-- PROBLEMA CORRIGIDO:
--   Os eventos TurmaAtivada e TurmaDesativada gravam AlteradoPor no payload
--   do ledger, mas a projection_turmas não tinha colunas para expor essa
--   informação. A pergunta "quem desativou esta turma?" exigia inspecionar
--   o spuri_ledger manualmente.
--
-- O QUE ESTA MIGRATION FAZ:
--   1. Adiciona status_alterado_por UUID (quem ativou/desativou)
--   2. Adiciona status_alterado_em TIMESTAMP (quando ocorreu)
--   3. Cria índice para auditoria por encarregado
--
-- AÇÃO REQUERIDA APÓS ESTA MIGRATION:
--   Execute um rebuild da projeção "turmas" via:
--     POST /admin/rebuild-projection/turmas
--   Isso vai reprocessar todos os eventos do ledger e popular
--   corretamente as novas colunas.
-- ============================================================================

BEGIN;

-- 1. Adicionar colunas de auditoria de status
ALTER TABLE projection_turmas
    ADD COLUMN IF NOT EXISTS status_alterado_por UUID DEFAULT NULL;

ALTER TABLE projection_turmas
    ADD COLUMN IF NOT EXISTS status_alterado_em TIMESTAMP DEFAULT NULL;

COMMENT ON COLUMN projection_turmas.status_alterado_por IS
    'UUID da academia que realizou a última ativação ou desativação da turma. '
    'Preenchido via eventos TurmaAtivada e TurmaDesativada — campo AlteradoPor do payload.';

COMMENT ON COLUMN projection_turmas.status_alterado_em IS
    'Timestamp da última ativação ou desativação da turma. '
    'Preenchido via eventos TurmaAtivada e TurmaDesativada — campo OccurredAt do evento.';

-- 2. Índice para auditoria: "quais turmas foram alteradas por esta academia?"
CREATE INDEX IF NOT EXISTS idx_turmas_status_alterado_por
    ON projection_turmas (status_alterado_por)
    WHERE status_alterado_por IS NOT NULL;

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 036 CONCLUÍDA — status_alterado_por e status_alterado_em adicionados a projection_turmas';
    RAISE NOTICE 'Execute POST /admin/rebuild-projection/turmas para popular as novas colunas.';
END $$;
