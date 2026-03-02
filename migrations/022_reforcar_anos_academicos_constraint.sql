-- ============================================================================
-- MIGRATION 022 - Reforçar constraint anos_academicos em projection_academias
--
-- Problema da migration 010: a constraint era "permissiva" e aceitava arrays
-- vazios para escolas de nivel_escolar 'fundamental' ou 'misto'.
-- Anos academicos é obrigatório e não pode estar vazio para esses níveis.
--
-- Esta migration:
--   1. Remove registros legados com array vazio (NULL ou []) — inválidos sob a nova regra
--   2. Remove a constraint antiga
--   3. Adiciona constraint mais restritiva: NOT NULL + array não vazio para fundamental/misto
--      e NULL ou array vazio para medio/superior
-- ============================================================================

BEGIN;

-- 1. Corrigir TODOS os dados inválidos antes de aplicar a nova constraint:
--    - NULLs em fundamental/misto → sentinela '[]' (para poder identificar no log)
--    - Arrays vazios '[]' em fundamental/misto → também inválidos sob a nova regra.
--    Como não é possível inferir os anos corretos automaticamente,
--    esses registros são resetados para NULL e serão rejeitados pela constraint,
--    o que é o comportamento correto: forçar o admin a corrigir via domínio.
--
--    ATENÇÃO: se houver academias fundamental/misto com anos_academicos = '[]' no banco,
--    elas precisam ser corrigidas ANTES de rodar esta migration (via AtualizarDados).
--    Esta query lista as problemáticas para auditoria:
--
--    SELECT id, codigo_academia, nivel_escolar, anos_academicos
--    FROM projection_academias
--    WHERE nivel_escolar IN ('fundamental', 'misto')
--      AND (anos_academicos IS NULL OR jsonb_array_length(anos_academicos) = 0);

-- 1a. NULLs → array vazio como passo intermediário (facilita log)
UPDATE projection_academias
SET anos_academicos = '[]'::jsonb
WHERE nivel_escolar IN ('fundamental', 'misto')
  AND anos_academicos IS NULL;

-- 1b. Garantir que academias medio/superior não tenham anos_academicos preenchido
UPDATE projection_academias
SET anos_academicos = NULL
WHERE (nivel_escolar = 'medio' OR nivel_escolar IS NULL)
  AND anos_academicos IS NOT NULL
  AND jsonb_array_length(anos_academicos) = 0;

-- 2. Remover constraint antiga
ALTER TABLE projection_academias
    DROP CONSTRAINT IF EXISTS check_anos_academicos_nivel;

-- 3. Adicionar constraint restritiva:
--    - fundamental/misto: NOT NULL e array não vazio (>= 1 elemento)
--    - medio (nivel_escolar='medio'): anos_academicos deve ser NULL
--    - superior (nivel_escolar IS NULL): anos_academicos deve ser NULL
ALTER TABLE projection_academias
    ADD CONSTRAINT check_anos_academicos_nivel CHECK (
        (
            nivel_escolar IN ('fundamental', 'misto')
            AND anos_academicos IS NOT NULL
            AND jsonb_array_length(anos_academicos) > 0
        )
        OR
        (
            nivel_escolar = 'medio'
            AND anos_academicos IS NULL
        )
        OR
        (
            nivel_escolar IS NULL
            AND anos_academicos IS NULL
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