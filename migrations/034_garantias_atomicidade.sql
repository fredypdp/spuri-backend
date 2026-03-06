-- ============================================================================
-- MIGRATION 034 — FIX DB-13: Garantir NOT NULL em tipo_ensino
-- ============================================================================
--
-- CONTEXTO (DB-13 da auditoria-etapa2-db.md):
--   A migration 008 executou:
--     ALTER TABLE projection_aprovacao_ano
--       ADD COLUMN tipo_ensino VARCHAR(20) NOT NULL DEFAULT 'fundamental' ...
--     ALTER TABLE projection_aprovacao_ano
--       ALTER COLUMN tipo_ensino DROP DEFAULT;
--
--   O DROP DEFAULT não remove o NOT NULL. Porém, em ambientes onde a coluna
--   foi criada com execução parcial (sem BEGIN/COMMIT na migration 006/008),
--   a coluna pode ter ficado nullable sem default.
--
--   Além disso, projection_reprovacoes (criada na migration 009) define
--   tipo_ensino como NOT NULL CHECK (...) mas sem DEFAULT — seguro pois
--   toda inserção deve fornecer o valor explicitamente.
--
-- O QUE ESTA MIGRATION FAZ:
--   1. Garante NOT NULL em projection_aprovacao_ano.tipo_ensino.
--   2. Garante NOT NULL em projection_reprovacoes.tipo_ensino.
--   3. Garante NOT NULL em projection_avaliacao_final.tipo_ensino.
--   4. Preenche NULLs existentes com 'fundamental' antes de impor NOT NULL
--      (defesa contra execução parcial de migrations anteriores).
--   5. Garante que os CHECKs de domínio estão presentes em todas as tabelas.
--
-- Idempotente: usa DO $$ BEGIN...END $$ com IF EXISTS/verificações condicionais.
-- ============================================================================

BEGIN;

-- ============================================================================
-- 1. projection_aprovacao_ano.tipo_ensino
-- ============================================================================

-- Preencher NULLs antes de impor NOT NULL
UPDATE projection_aprovacao_ano
SET tipo_ensino = 'fundamental'
WHERE tipo_ensino IS NULL;

-- Garantir NOT NULL
ALTER TABLE projection_aprovacao_ano
    ALTER COLUMN tipo_ensino SET NOT NULL;

-- Garantir CHECK de domínio (idempotente via DROP IF EXISTS + ADD)
ALTER TABLE projection_aprovacao_ano
    DROP CONSTRAINT IF EXISTS check_aprovacao_tipo_ensino;

ALTER TABLE projection_aprovacao_ano
    ADD CONSTRAINT check_aprovacao_tipo_ensino
        CHECK (tipo_ensino IN ('fundamental', 'medio', 'superior'));

COMMENT ON COLUMN projection_aprovacao_ano.tipo_ensino IS
    'Ciclo de ensino da aprovação: fundamental | medio | superior. NOT NULL garantido pela migration 034.';

-- ============================================================================
-- 2. projection_reprovacoes.tipo_ensino
-- ============================================================================

-- Preencher NULLs antes de impor NOT NULL (defesa contra execução parcial)
UPDATE projection_reprovacoes
SET tipo_ensino = 'fundamental'
WHERE tipo_ensino IS NULL;

-- Garantir NOT NULL
ALTER TABLE projection_reprovacoes
    ALTER COLUMN tipo_ensino SET NOT NULL;

-- Garantir CHECK de domínio
ALTER TABLE projection_reprovacoes
    DROP CONSTRAINT IF EXISTS check_reprov_tipo_ensino;

ALTER TABLE projection_reprovacoes
    ADD CONSTRAINT check_reprov_tipo_ensino
        CHECK (tipo_ensino IN ('fundamental', 'medio', 'superior'));

COMMENT ON COLUMN projection_reprovacoes.tipo_ensino IS
    'Ciclo de ensino da reprovação: fundamental | medio | superior. NOT NULL garantido pela migration 034.';

-- ============================================================================
-- 3. projection_avaliacao_final.tipo_ensino
-- ============================================================================

-- Preencher NULLs antes de impor NOT NULL
UPDATE projection_avaliacao_final
SET tipo_ensino = 'fundamental'
WHERE tipo_ensino IS NULL;

-- Garantir NOT NULL
ALTER TABLE projection_avaliacao_final
    ALTER COLUMN tipo_ensino SET NOT NULL;

-- Garantir CHECK de domínio
ALTER TABLE projection_avaliacao_final
    DROP CONSTRAINT IF EXISTS check_avf_tipo_ensino;

ALTER TABLE projection_avaliacao_final
    ADD CONSTRAINT check_avf_tipo_ensino
        CHECK (tipo_ensino IN ('fundamental', 'medio', 'superior'));

COMMENT ON COLUMN projection_avaliacao_final.tipo_ensino IS
    'Ciclo de ensino da avaliação: fundamental | medio | superior. NOT NULL garantido pela migration 034.';

COMMIT;

-- ============================================================================
-- Verificação
-- ============================================================================

DO $$
DECLARE
    v_aprovacao_nulls  INTEGER;
    v_reprovacoes_nulls INTEGER;
    v_avaliacao_nulls  INTEGER;
BEGIN
    SELECT COUNT(*) INTO v_aprovacao_nulls
    FROM projection_aprovacao_ano WHERE tipo_ensino IS NULL;

    SELECT COUNT(*) INTO v_reprovacoes_nulls
    FROM projection_reprovacoes WHERE tipo_ensino IS NULL;

    SELECT COUNT(*) INTO v_avaliacao_nulls
    FROM projection_avaliacao_final WHERE tipo_ensino IS NULL;

    IF v_aprovacao_nulls > 0 OR v_reprovacoes_nulls > 0 OR v_avaliacao_nulls > 0 THEN
        RAISE WARNING '⚠️  tipo_ensino ainda tem NULLs após migration 034: aprovacao=%, reprovacoes=%, avaliacao=%',
            v_aprovacao_nulls, v_reprovacoes_nulls, v_avaliacao_nulls;
    ELSE
        RAISE NOTICE '✅ MIGRATION 034 — tipo_ensino NOT NULL verificado em todas as tabelas (0 NULLs)';
    END IF;
END $$;