-- ============================================================================
-- MIGRATION 039 - Índice de telefone em projection_estudantes
-- ============================================================================
-- Necessário para o login universal (sem campo "type"):
-- GetAuthByIdentificador faz lookup por codigo_estudante OR email OR telefone.
-- Sem este índice, a query faria full scan na coluna telefone.
--
-- Os índices de codigo_estudante e email já existem:
--   - codigo_estudante é PRIMARY KEY (índice automático)
--   - idx_estudante_email foi criado na migration 028
--
-- Safe para executar em produção com dados existentes (CREATE INDEX IF NOT EXISTS).
-- ============================================================================

BEGIN;

CREATE INDEX IF NOT EXISTS idx_estudante_telefone
    ON projection_estudantes (telefone)
    WHERE telefone IS NOT NULL;

COMMIT;

-- Verificação
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_indexes
        WHERE tablename = 'projection_estudantes'
          AND indexname  = 'idx_estudante_telefone'
    ) THEN
        RAISE NOTICE '✅ MIGRATION 039 CONCLUÍDA — idx_estudante_telefone criado';
    ELSE
        RAISE NOTICE '❌ MIGRATION 039 FALHOU — índice não encontrado';
    END IF;
END $$;
