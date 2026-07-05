-- MIGRATION 083 — modelo pedagógico dos cursos médios
ALTER TABLE projection_cursos
    ADD COLUMN IF NOT EXISTS modelo VARCHAR(20);

UPDATE projection_cursos
SET modelo = 'liceu'
WHERE type = 'medio'
  AND (modelo IS NULL OR modelo = '');

ALTER TABLE projection_cursos
    DROP CONSTRAINT IF EXISTS projection_cursos_modelo_medio_check;

ALTER TABLE projection_cursos
    ADD CONSTRAINT projection_cursos_modelo_medio_check CHECK (
        (type = 'medio' AND modelo IN ('liceu', 'tecnico')) OR
        (type <> 'medio' AND modelo IS NULL)
    );

CREATE INDEX IF NOT EXISTS idx_cursos_modelo_medio
    ON projection_cursos(modelo)
    WHERE type = 'medio' AND deleted_at IS NULL;

COMMENT ON COLUMN projection_cursos.modelo IS
    'Modelo pedagógico/curricular exclusivo e obrigatório para cursos type=medio: liceu ou tecnico. Backfill inicial usa liceu para cursos médios existentes.';
