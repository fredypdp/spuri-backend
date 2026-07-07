-- MIGRATION 085 — Restringir matérias dependentes ao ensino superior
--
-- O padrão avaliativo escolar fixo não permite matérias pendentes/dependentes
-- no ensino médio. Pendências permanecem exclusivas do ensino superior.

UPDATE projection_materias
SET pendencia_permitida = FALSE,
    pendencia_nivel_conclusao = NULL,
    updated_at = CURRENT_TIMESTAMP
WHERE type <> 'superior'
  AND (pendencia_permitida = TRUE OR pendencia_nivel_conclusao IS NOT NULL);

ALTER TABLE projection_materias
    DROP CONSTRAINT IF EXISTS chk_materia_pendencia_permitida_tipo;

ALTER TABLE projection_materias
    ADD CONSTRAINT chk_materia_pendencia_permitida_tipo CHECK (
        type = 'superior' OR pendencia_permitida = FALSE
    );

ALTER TABLE projection_materias
    DROP CONSTRAINT IF EXISTS chk_materia_pendencia_nivel_conclusao_tipo;

ALTER TABLE projection_materias
    ADD CONSTRAINT chk_materia_pendencia_nivel_conclusao_tipo CHECK (
        pendencia_nivel_conclusao IS NULL OR type = 'superior'
    );

COMMENT ON COLUMN projection_materias.pendencia_permitida IS
    'Indica se a matéria pode ficar como pendência para aprovação futura; disponível apenas para superior';

COMMENT ON COLUMN projection_materias.pendencia_nivel_conclusao IS
    'Semestre máximo em que o estudante pode chegar com pendências desta matéria; disponível apenas para superior';

DO $$
BEGIN
    RAISE NOTICE '✅ MIGRATION 085 — matérias pendentes restritas ao ensino superior';
END $$;
