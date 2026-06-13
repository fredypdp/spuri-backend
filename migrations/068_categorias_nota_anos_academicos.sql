-- MIGRATION 068 — Anos acadêmicos nas categorias de nota
--
-- Cada categoria de nota (fixa/obrigatória ou adicional) passa a declarar
-- explicitamente os anos acadêmicos nos quais pode ser usada. Categorias sem
-- anos configurados não podem receber registros de nota pela aplicação.

ALTER TABLE projection_categorias_nota
    ADD COLUMN IF NOT EXISTS anos_academicos JSONB NOT NULL DEFAULT '[]'::jsonb;

COMMENT ON COLUMN projection_categorias_nota.anos_academicos IS
    'Lista de anos acadêmicos nos quais esta categoria de nota pode ser registrada. Lista vazia bloqueia novos registros nesta categoria.';

CREATE INDEX IF NOT EXISTS idx_cat_nota_anos_academicos
    ON projection_categorias_nota USING GIN (anos_academicos);

DO $$ BEGIN RAISE NOTICE '✅ MIGRATION 068 — anos_academicos adicionados a projection_categorias_nota'; END $$;
