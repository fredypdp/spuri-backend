-- ============================================================================
-- MIGRATION 010 - AnosAcademicos na Academia
--
-- MOTIVO: Escolas do tipo fundamental/misto precisam declarar quais anos do
-- fundamental oferecem (subconjunto de 1º–9º). Esse campo é obrigatório para
-- nivel_escolar IN ('fundamental', 'misto') e proibido para 'medio' e 'superior'.
-- ============================================================================

BEGIN;

ALTER TABLE projection_academias
    ADD COLUMN IF NOT EXISTS anos_academicos JSONB;

COMMENT ON COLUMN projection_academias.anos_academicos IS
    'Array JSON com anos fundamentais oferecidos pela academia. '
    'Obrigatório para nivel_escolar IN (''fundamental'', ''misto''). '
    'Exemplo: ["primeiro_fundamental","segundo_fundamental","terceiro_fundamental"]';

-- Constraint: anos_academicos obrigatório para fundamental/misto, NULL para o resto
ALTER TABLE projection_academias
    ADD CONSTRAINT check_anos_academicos_nivel CHECK (
        (nivel_escolar IN ('fundamental', 'misto') AND anos_academicos IS NOT NULL AND jsonb_array_length(anos_academicos) > 0)
        OR
        (nivel_escolar NOT IN ('fundamental', 'misto') OR nivel_escolar IS NULL)
    );

CREATE INDEX IF NOT EXISTS idx_academia_anos_academicos
    ON projection_academias USING GIN (anos_academicos)
    WHERE anos_academicos IS NOT NULL;

COMMIT;

DO $$ BEGIN RAISE NOTICE '✅ MIGRATION 010 - anos_academicos adicionado a projection_academias'; END $$;
