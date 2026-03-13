-- ============================================
-- MIGRATION SEGURA - Adicionar email_verificado
-- Versão: 002
-- Data: 25-01-2026
-- SAFE para produção com dados existentes
-- ============================================

-- 1. Adicionar coluna email_verificado em projection_admins (se não existir)
DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'projection_admins' 
        AND column_name = 'email_verificado'
    ) THEN
        ALTER TABLE projection_admins 
        ADD COLUMN email_verificado BOOLEAN DEFAULT FALSE;
        
        RAISE NOTICE '✅ Coluna email_verificado adicionada em projection_admins';
    ELSE
        RAISE NOTICE 'ℹ️  Coluna email_verificado já existe em projection_admins';
    END IF;
END $$;

-- 2. Definir FALSE para todos os registros existentes (garantir consistência)
UPDATE projection_admins 
SET email_verificado = FALSE 
WHERE email_verificado IS NULL;

DO $$ BEGIN
    RAISE NOTICE '✅ Registros existentes atualizados com email_verificado = FALSE';
END $$;

-- 3. Adicionar NOT NULL constraint depois de preencher valores
ALTER TABLE projection_admins 
ALTER COLUMN email_verificado SET NOT NULL;

DO $$ BEGIN
    RAISE NOTICE '✅ Constraint NOT NULL adicionada';
END $$;

-- 4. Adicionar comentário
COMMENT ON COLUMN projection_admins.email_verificado IS 'Indica se o email do admin foi verificado';

-- 5. Verificação final
DO $$ 
DECLARE
    v_total_admins INTEGER;
    v_admins_nao_verificados INTEGER;
BEGIN
    SELECT COUNT(*) INTO v_total_admins FROM projection_admins;
    SELECT COUNT(*) INTO v_admins_nao_verificados 
    FROM projection_admins 
    WHERE email_verificado = FALSE;
    
    RAISE NOTICE '══════════════════════════════════════════';
    RAISE NOTICE '✅ MIGRATION 002 CONCLUÍDA COM SUCESSO';
    RAISE NOTICE '══════════════════════════════════════════';
    RAISE NOTICE 'Total de admins: %', v_total_admins;
    RAISE NOTICE 'Admins não verificados: %', v_admins_nao_verificados;
    RAISE NOTICE '══════════════════════════════════════════';
END $$;