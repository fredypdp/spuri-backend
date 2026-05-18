-- ============================================================
-- MIGRATION 064 - Permitir reutilização de código de turma deletada
-- ============================================================
-- Contexto:
--   A aplicação consulta turmas por (codigo_turma, codigo_academia)
--   filtrando deleted_at IS NULL, permitindo conceitualmente reutilizar o
--   código de uma turma removida na mesma academia.
--
--   Porém, a constraint legada UNIQUE (codigo_turma, codigo_academia) ainda
--   considerava registros soft-deletados e bloqueava a recriação.
--
-- Solução:
--   Substituir a constraint global por um índice único parcial, válido apenas
--   para turmas não deletadas.

BEGIN;

ALTER TABLE projection_turmas
    DROP CONSTRAINT IF EXISTS projection_turmas_codigo_turma_codigo_academia_key;

CREATE UNIQUE INDEX IF NOT EXISTS idx_turmas_codigo_academia_ativas_unico
    ON projection_turmas (codigo_turma, codigo_academia)
    WHERE deleted_at IS NULL;

COMMENT ON INDEX idx_turmas_codigo_academia_ativas_unico IS
    'Garante codigo_turma único por academia apenas para turmas não deletadas, permitindo reutilização após soft delete.';

COMMIT;

DO $$ BEGIN RAISE NOTICE '✅ MIGRATION 064 CONCLUÍDA - unicidade de turmas limitada a registros ativos'; END $$;
