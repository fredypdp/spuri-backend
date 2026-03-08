-- ============================================================================
-- MIGRATION 010 - AnosAcademicos na Academia
-- CORRIGIDA: preenche anos_academicos nas academias existentes antes da constraint
-- ============================================================================

BEGIN;

ALTER TABLE projection_academias
    ADD COLUMN IF NOT EXISTS anos_academicos JSONB;

COMMENT ON COLUMN projection_academias.anos_academicos IS
    'Array JSON com anos fundamentais oferecidos pela academia. '
    'Obrigatório para nivel_escolar IN (''fundamental'', ''misto''). '
    'Exemplo: ["1_fundamental","segundo_fundamental","terceiro_fundamental"]';

-- Preencher academias fundamental/misto existentes com array vazio
-- para não violar a constraint NOT NULL que será adicionada abaixo.
-- O admin deve atualizar os valores corretos depois.
UPDATE projection_academias
SET anos_academicos = '[]'::jsonb
WHERE nivel_escolar IN ('fundamental', 'misto')
  AND anos_academicos IS NULL;

-- Constraint: anos_academicos obrigatório (NOT NULL) para fundamental/misto
-- Versão permissiva: aceita array vazio para não bloquear dados existentes.
-- Se quiser obrigar array não-vazio, troque para:
--   AND jsonb_array_length(anos_academicos) > 0
ALTER TABLE projection_academias
    DROP CONSTRAINT IF EXISTS check_anos_academicos_nivel;

ALTER TABLE projection_academias
    ADD CONSTRAINT check_anos_academicos_nivel CHECK (
        (nivel_escolar IN ('fundamental', 'misto') AND anos_academicos IS NOT NULL)
        OR
        (nivel_escolar NOT IN ('fundamental', 'misto') OR nivel_escolar IS NULL)
    );

CREATE INDEX IF NOT EXISTS idx_academia_anos_academicos
    ON projection_academias USING GIN (anos_academicos)
    WHERE anos_academicos IS NOT NULL;

COMMIT;

DO $$ BEGIN RAISE NOTICE '✅ MIGRATION 010 - anos_academicos adicionado a projection_academias'; END $$;