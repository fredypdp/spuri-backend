-- ==========================================================================
-- MIGRATION 054 - Renomear campo type de academias para nivel
-- ==========================================================================

BEGIN;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = 'projection_academias'
          AND column_name = 'type'
    )
    AND NOT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = 'projection_academias'
          AND column_name = 'nivel'
    ) THEN
        ALTER TABLE projection_academias
            RENAME COLUMN type TO nivel;
    END IF;
END $$;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM pg_indexes
        WHERE indexname = 'idx_proj_academia_type'
    ) THEN
        ALTER INDEX idx_proj_academia_type RENAME TO idx_proj_academia_nivel_tipo;
    END IF;
END $$;

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
    RAISE NOTICE '✅ MIGRATION 054 - campo projection_academias.type renomeado para nivel';
END $$;
