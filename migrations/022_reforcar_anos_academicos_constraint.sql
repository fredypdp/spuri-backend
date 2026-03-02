-- ============================================================================
-- MIGRATION 022 - Reforçar constraint anos_academicos em projection_academias
-- ============================================================================

BEGIN;

-- 1. PRIMEIRO: remover a constraint antiga ANTES dos UPDATEs
--    (a constraint antiga bloqueia SET anos_academicos = NULL em misto/fundamental)
ALTER TABLE projection_academias
    DROP CONSTRAINT IF EXISTS check_anos_academicos_nivel;

-- 2. Limpar dados inválidos: fundamental/misto com NULL ou array vazio → NULL
UPDATE projection_academias
SET anos_academicos = NULL
WHERE nivel_escolar IN ('fundamental', 'misto')
  AND (
      anos_academicos IS NULL
      OR jsonb_array_length(anos_academicos) = 0
  );

-- 3. Limpar dados inválidos: medio/superior com anos_academicos preenchido → NULL
UPDATE projection_academias
SET anos_academicos = NULL
WHERE (nivel_escolar = 'medio' OR nivel_escolar IS NULL)
  AND anos_academicos IS NOT NULL;

-- 4. Auditoria: avisar quantas academias ficaram sem anos válidos
DO $$
DECLARE
    v_count INTEGER;
BEGIN
    SELECT COUNT(*)
    INTO v_count
    FROM projection_academias
    WHERE nivel_escolar IN ('fundamental', 'misto')
      AND anos_academicos IS NULL;

    IF v_count > 0 THEN
        RAISE WARNING
            '⚠️  % academia(s) fundamental/misto sem anos_academicos válidos. '
            'O admin deve corrigir via AtualizarAnosAcademicos.',
            v_count;
    END IF;
END $$;

-- 5. Adicionar constraint restritiva:
--    - fundamental/misto: NULL (legado pendente de correção) OU array não-vazio
--    - medio: anos_academicos deve ser NULL
--    - superior (nivel_escolar IS NULL): anos_academicos deve ser NULL
ALTER TABLE projection_academias
    ADD CONSTRAINT check_anos_academicos_nivel CHECK (
        (
            nivel_escolar IN ('fundamental', 'misto')
            AND (
                anos_academicos IS NULL
                OR jsonb_array_length(anos_academicos) > 0
            )
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

-- 6. Recriar índice GIN
DROP INDEX IF EXISTS idx_academia_anos_academicos;
CREATE INDEX IF NOT EXISTS idx_academia_anos_academicos
    ON projection_academias USING GIN (anos_academicos)
    WHERE anos_academicos IS NOT NULL;

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 022 - Constraint anos_academicos reforçada com sucesso';
END $$;