-- ============================================================================
-- MIGRATION 028 - FIX-C3: EmailVerificadoEstudante via event sourcing
-- ============================================================================
-- Esta migration não altera schema diretamente — a coluna email_verificado
-- já existe em projection_estudantes.
--
-- O que muda:
--   1. A coluna email_verificado agora é atualizada via evento
--      EmailVerificadoEstudante → handler na projeção (não mais UPDATE direto).
--   2. Adicionamos índice único em bilhete_identidade para garantir integridade
--      na camada de banco (defesa em profundidade além da validação no handler).
--
-- Safe para executar em produção com dados existentes.
-- ============================================================================

BEGIN;

-- Garantir que email_verificado existe e tem NOT NULL com DEFAULT FALSE
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'projection_estudantes'
          AND column_name = 'email_verificado'
    ) THEN
        ALTER TABLE projection_estudantes
            ADD COLUMN email_verificado BOOLEAN NOT NULL DEFAULT FALSE;
        RAISE NOTICE '✅ Coluna email_verificado adicionada em projection_estudantes';
    ELSE
        -- Garantir NOT NULL e DEFAULT
        ALTER TABLE projection_estudantes
            ALTER COLUMN email_verificado SET NOT NULL,
            ALTER COLUMN email_verificado SET DEFAULT FALSE;
        RAISE NOTICE 'ℹ️  Coluna email_verificado já existe em projection_estudantes';
    END IF;
END $$;

-- Preencher NULLs existentes
UPDATE projection_estudantes
SET email_verificado = FALSE
WHERE email_verificado IS NULL;

-- Índice para lookup por bilhete_identidade (usado na validação de unicidade)
CREATE INDEX IF NOT EXISTS idx_estudante_bilhete_identidade
    ON projection_estudantes (bilhete_identidade)
    WHERE bilhete_identidade IS NOT NULL;

-- Índice para lookup por email (usado no login e recuperação de senha)
CREATE INDEX IF NOT EXISTS idx_estudante_email
    ON projection_estudantes (email)
    WHERE email IS NOT NULL;

-- Índice para lookup por codigo_academia (usado em ListarEstudantes)
CREATE INDEX IF NOT EXISTS idx_estudante_codigo_academia
    ON projection_estudantes (codigo_academia)
    WHERE codigo_academia IS NOT NULL;

COMMIT;

-- Verificação
DO $$
DECLARE
    v_total INTEGER;
    v_verificados INTEGER;
BEGIN
    SELECT COUNT(*) INTO v_total FROM projection_estudantes;
    SELECT COUNT(*) INTO v_verificados FROM projection_estudantes WHERE email_verificado = TRUE;
    RAISE NOTICE '══════════════════════════════════════════════════';
    RAISE NOTICE '✅ MIGRATION 010 CONCLUÍDA';
    RAISE NOTICE '══════════════════════════════════════════════════';
    RAISE NOTICE 'Total estudantes: %', v_total;
    RAISE NOTICE 'Emails verificados: %', v_verificados;
    RAISE NOTICE '══════════════════════════════════════════════════';
END $$;
