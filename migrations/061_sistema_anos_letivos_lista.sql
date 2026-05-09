-- MIGRATION 061 — Histórico de anos letivos globais do sistema
--
-- Adiciona coluna com a lista histórica de anos letivos já definidos pelo
-- admin na configuração global do sistema.

ALTER TABLE projection_sistema_config
    ADD COLUMN IF NOT EXISTS anos_letivos_lista JSONB NOT NULL DEFAULT '[]'::jsonb;

COMMENT ON COLUMN projection_sistema_config.anos_letivos_lista IS
    'Lista histórica de anos letivos globais definidos pelo admin (sem duplicar ano_letivo).';

DO $$
BEGIN
    RAISE NOTICE '✅ MIGRATION 061 — projection_sistema_config.anos_letivos_lista adicionada';
END $$;
