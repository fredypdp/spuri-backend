-- MIGRATION 059
-- Adiciona semestre_atual no estudante e validação de consistência para cursos superiores.

ALTER TABLE projection_estudantes
    ADD COLUMN IF NOT EXISTS semestre_atual INTEGER;

ALTER TABLE projection_estudantes
    ADD CONSTRAINT check_projection_estudantes_semestre_atual
        CHECK (semestre_atual IS NULL OR semestre_atual >= 1);

COMMENT ON COLUMN projection_estudantes.semestre_atual IS
    'Semestre sequencial atual do estudante superior (1..N).';
