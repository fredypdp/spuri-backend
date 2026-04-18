-- MIGRATION 055 - Adicionar campo obrigatório "type" em academias (public/private)
-- -----------------------------------------------------------------------------
-- Premissa desta migration:
--   - banco vazio (sem necessidade de compatibilidade com dados legados)
-- Objetivo:
--   1. Adicionar a coluna obrigatória `type` em projection_academias
--   2. Garantir domínio restrito: 'public' | 'private'

ALTER TABLE projection_academias
    ADD COLUMN type VARCHAR(20) NOT NULL;

ALTER TABLE projection_academias
    ADD CONSTRAINT check_academia_type_public_private CHECK (type IN ('public', 'private'));

CREATE INDEX IF NOT EXISTS idx_proj_academia_type_public_private
    ON projection_academias(type);

COMMENT ON COLUMN projection_academias.type IS
    'Natureza da academia: public ou private';

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 055 - campo obrigatório projection_academias.type adicionado (public/private)';
END $$;
