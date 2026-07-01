-- MIGRATION 080 — matérias-chave por ano em cursos médios
ALTER TABLE projection_cursos
    ADD COLUMN IF NOT EXISTS materias_chave JSONB NOT NULL DEFAULT '[]'::jsonb;

CREATE INDEX IF NOT EXISTS idx_cursos_materias_chave
    ON projection_cursos USING GIN (materias_chave);

COMMENT ON COLUMN projection_cursos.materias_chave IS
    'Configuração de matérias-chave por ano_academico exclusiva de cursos type=medio.';
