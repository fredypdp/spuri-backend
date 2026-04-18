-- MIGRATION 055 - Adicionar campo "type" em academias (public/private)
-- -----------------------------------------------------------------------------
-- Objetivo:
--   1. Adicionar a coluna `type` em projection_academias
--   2. Garantir domínio restrito: 'public' | 'private'
--   3. Definir valor padrão para dados legados

ALTER TABLE projection_academias
    ADD COLUMN IF NOT EXISTS type VARCHAR(20);

UPDATE projection_academias
SET type = 'private'
WHERE type IS NULL;

ALTER TABLE projection_academias
    ALTER COLUMN type SET NOT NULL;

ALTER TABLE projection_academias
    ALTER COLUMN type SET DEFAULT 'private';

ALTER TABLE projection_academias
    DROP CONSTRAINT IF EXISTS check_academia_type_public_private;

ALTER TABLE projection_academias
    ADD CONSTRAINT check_academia_type_public_private CHECK (type IN ('public', 'private'));

CREATE INDEX IF NOT EXISTS idx_proj_academia_type_public_private
    ON projection_academias(type);

COMMENT ON COLUMN projection_academias.type IS
    'Natureza da academia: public ou private';

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 055 - campo projection_academias.type adicionado (public/private)';
END $$;
