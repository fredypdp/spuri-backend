-- ============================================================================
-- MIGRATION 078 — Adicionar pendencia_nivel_conclusao em projection_materias
--
-- REGRAS:
--   • Define o ano acadêmico/semestre máximo em que o estudante pode chegar
--     mantendo pendências desta matéria.
--   • Disponível apenas para matérias do type='medio' ou type='superior'.
--   • NULL para matérias fundamental.
-- ============================================================================

BEGIN;

ALTER TABLE projection_materias
    ADD COLUMN IF NOT EXISTS pendencia_nivel_conclusao VARCHAR(50) DEFAULT NULL;

UPDATE projection_materias
SET pendencia_nivel_conclusao = NULL
WHERE type = 'fundamental';

ALTER TABLE projection_materias
    DROP CONSTRAINT IF EXISTS chk_materia_pendencia_nivel_conclusao_tipo;

ALTER TABLE projection_materias
    ADD CONSTRAINT chk_materia_pendencia_nivel_conclusao_tipo CHECK (
        pendencia_nivel_conclusao IS NULL
        OR type IN ('medio', 'superior')
    );

COMMENT ON COLUMN projection_materias.pendencia_nivel_conclusao IS
    'Ano acadêmico/semestre máximo em que o estudante pode chegar com pendências desta matéria. '
    'Disponível apenas para matérias do type=medio ou type=superior.';

CREATE INDEX IF NOT EXISTS idx_materias_pendencia_nivel_conclusao
    ON projection_materias(pendencia_nivel_conclusao)
    WHERE pendencia_nivel_conclusao IS NOT NULL;

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 078 — campo pendencia_nivel_conclusao adicionado em projection_materias';
END $$;
