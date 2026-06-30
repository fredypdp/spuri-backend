-- ============================================================================
-- MIGRATION 077 — Adicionar pendencia_permitida em projection_materias
--
-- REGRAS:
--   • Indica se uma matéria pode ficar pendente para conclusão futura do ciclo.
--   • Disponível apenas para matérias do type='medio' ou type='superior'.
--   • Matérias fundamental permanecem com pendencia_permitida=false.
-- ============================================================================

BEGIN;

ALTER TABLE projection_materias
    ADD COLUMN IF NOT EXISTS pendencia_permitida BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE projection_materias
SET pendencia_permitida = FALSE
WHERE type = 'fundamental';

ALTER TABLE projection_materias
    DROP CONSTRAINT IF EXISTS chk_materia_pendencia_permitida_tipo;

ALTER TABLE projection_materias
    ADD CONSTRAINT chk_materia_pendencia_permitida_tipo CHECK (
        type IN ('medio', 'superior')
        OR pendencia_permitida = FALSE
    );

COMMENT ON COLUMN projection_materias.pendencia_permitida IS
    'Indica se a matéria pode ficar como pendência para aprovação futura. '
    'Disponível apenas para matérias do type=medio ou type=superior.';

CREATE INDEX IF NOT EXISTS idx_materias_pendencia_permitida
    ON projection_materias(pendencia_permitida)
    WHERE pendencia_permitida = TRUE;

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 077 — campo pendencia_permitida adicionado em projection_materias';
END $$;
