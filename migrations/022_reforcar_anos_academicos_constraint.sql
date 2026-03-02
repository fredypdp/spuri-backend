-- ============================================================================
-- MIGRATION 022 - Reforçar constraint anos_academicos em projection_academias
--
-- Problema da migration 010: a constraint era "permissiva" e aceitava arrays
-- vazios para escolas de nivel_escolar 'fundamental' ou 'misto'.
-- Anos academicos é obrigatório e não pode estar vazio para esses níveis.
--
-- Esta migration:
--   1. Preenche eventuais NULLs com valor sentinela (para não quebrar a query)
--   2. Remove a constraint antiga
--   3. Adiciona constraint mais restritiva: NOT NULL + array não vazio
-- ============================================================================

BEGIN;

-- 1. Garantir que não existam NULLs em academias fundamental/misto
--    (registros legados que possam ter escapado da constraint permissiva)
UPDATE projection_academias
SET anos_academicos = '[]'::jsonb
WHERE nivel_escolar IN ('fundamental', 'misto')
  AND anos_academicos IS NULL;

-- 2. Remover constraints antigas (permissiva da migration 010 e qualquer variante)
ALTER TABLE projection_academias
    DROP CONSTRAINT IF EXISTS check_anos_academicos_nivel;

-- 3. Adicionar constraint restritiva:
--    - Para fundamental/misto: anos_academicos deve ser NOT NULL E ter ao menos 1 elemento
--    - Para medio/superior/NULL: anos_academicos deve ser NULL (não faz sentido ter anos fundamentais)
ALTER TABLE projection_academias
    ADD CONSTRAINT check_anos_academicos_nivel CHECK (
        (
            nivel_escolar IN ('fundamental', 'misto')
            AND anos_academicos IS NOT NULL
            AND jsonb_array_length(anos_academicos) > 0
        )
        OR
        (
            (nivel_escolar = 'medio' OR nivel_escolar IS NULL)
            AND (anos_academicos IS NULL OR jsonb_array_length(anos_academicos) = 0)
        )
    );

-- 4. Garantir índice GIN para buscas eficientes
DROP INDEX IF EXISTS idx_academia_anos_academicos;
CREATE INDEX IF NOT EXISTS idx_academia_anos_academicos
    ON projection_academias USING GIN (anos_academicos)
    WHERE anos_academicos IS NOT NULL;

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 022 - Constraint anos_academicos reforçada: obrigatório e não-vazio para fundamental/misto';
END $$;
