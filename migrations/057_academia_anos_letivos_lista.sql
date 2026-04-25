-- MIGRATION 057 — Histórico de anos letivos por academia
-- Objetivo:
-- 1) Adicionar coluna com lista de anos letivos já definidos pela academia
-- 2) Garantir formato JSONB de array, sem null

ALTER TABLE projection_academias
    ADD COLUMN IF NOT EXISTS anos_letivos_lista JSONB NOT NULL DEFAULT '[]'::jsonb;

COMMENT ON COLUMN projection_academias.anos_letivos_lista IS
    'Lista histórica de anos letivos já definidos pela academia (sem duplicar ano_letivo).';

DO $$
BEGIN
    RAISE NOTICE '✅ MIGRATION 057 — projection_academias.anos_letivos_lista adicionada';
END $$;
