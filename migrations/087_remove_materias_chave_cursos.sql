-- Remove definitivamente a persistência ativa de materias_chave em cursos.
DROP INDEX IF EXISTS idx_cursos_materias_chave;
ALTER TABLE projection_cursos
    DROP COLUMN IF EXISTS materias_chave;
