-- ============================================================================
-- MIGRATION 032 — Adicionar coluna adicionado_por em projection_categorias_nota
--
-- PROBLEMA CORRIGIDO (P3-09):
--   O handler handleCategoriaAdicionada em categorias_nota_projection.go
--   não lia nem persistia o campo AdicionadoPor (uuid.UUID) do evento
--   CategoriaNotaAdicionadaEvent. A tabela também não tinha a coluna.
--
--   Impacto: impossível auditar quem criou cada categoria de nota
--   sem inspecionar o ledger diretamente — violando o princípio de
--   auditabilidade do sistema.
--
-- O QUE ESTA MIGRATION FAZ:
--   Adiciona coluna adicionado_por à tabela projection_categorias_nota.
--   Garante checkpoint para categorias_nota.
-- ============================================================================

BEGIN;

-- 1. Adicionar coluna adicionado_por
ALTER TABLE projection_categorias_nota
    ADD COLUMN IF NOT EXISTS adicionado_por UUID;

COMMENT ON COLUMN projection_categorias_nota.adicionado_por IS
    'UUID do admin/academia que adicionou a categoria. '
    'Permite auditoria sem inspecionar o ledger diretamente.';

-- 2. Índice para busca por quem adicionou (auditoria)
CREATE INDEX IF NOT EXISTS idx_cat_nota_adicionado_por
    ON projection_categorias_nota(adicionado_por)
    WHERE adicionado_por IS NOT NULL;

-- 3. Garantir checkpoint
INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at)
VALUES ('categorias_nota', 0, CURRENT_TIMESTAMP)
ON CONFLICT (projection_name) DO NOTHING;

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 032 — projection_categorias_nota.adicionado_por adicionada.';
    RAISE NOTICE '   Ação recomendada: POST /admin/rebuild-projection/categorias_nota';
END $$;
