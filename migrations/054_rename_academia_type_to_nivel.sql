-- ==========================================================================
-- MIGRATION 054 - Renomear campo type de academias para nivel
-- ==========================================================================

BEGIN;

-- IMPORTANTE:
-- O campo `type` passou a representar a natureza da academia
-- (public/private), enquanto `nivel` representa o nível de ensino
-- (escola/superior). Portanto, não devemos mais renomear `type` -> `nivel`.
-- Esta migration agora apenas reforça estrutura/constraints de `nivel`.

CREATE INDEX IF NOT EXISTS idx_proj_academia_nivel_tipo
    ON projection_academias(nivel);

ALTER TABLE projection_academias
    DROP CONSTRAINT IF EXISTS check_nivel_escolar_tipo;

ALTER TABLE projection_academias
    ADD CONSTRAINT check_nivel_escolar_tipo CHECK (
        (nivel = 'escola' AND nivel_escolar IN ('fundamental', 'medio', 'misto'))
        OR
        (nivel = 'superior' AND nivel_escolar IS NULL)
    );

COMMENT ON COLUMN projection_academias.nivel IS
    'Nível da academia: escola | superior.';

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 054 - estrutura/constraints de projection_academias.nivel reforçadas';
END $$;
