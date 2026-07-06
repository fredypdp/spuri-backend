-- MIGRATION 084 — Fixar anos acadêmicos dos cursos médios por modelo
-- Idempotente: corrige projeções existentes para que a duração do médio seja
-- sempre consequência do modelo pedagógico persistido.

UPDATE projection_cursos
SET anos_academicos = '["1_ano_medio", "2_ano_medio", "3_ano_medio"]'::jsonb
WHERE type = 'medio'
  AND modelo = 'liceu'
  AND anos_academicos IS DISTINCT FROM '["1_ano_medio", "2_ano_medio", "3_ano_medio"]'::jsonb;

UPDATE projection_cursos
SET anos_academicos = '["1_ano_medio", "2_ano_medio", "3_ano_medio", "4_ano_medio"]'::jsonb
WHERE type = 'medio'
  AND modelo = 'tecnico'
  AND anos_academicos IS DISTINCT FROM '["1_ano_medio", "2_ano_medio", "3_ano_medio", "4_ano_medio"]'::jsonb;

COMMENT ON COLUMN projection_cursos.anos_academicos IS
  'Array JSON com anos do curso. Para médio é derivado e fixo por modelo: liceu=1º-3º, tecnico=1º-4º. Para superior é derivado de periodos.';

DO $$ BEGIN RAISE NOTICE '✅ MIGRATION 084 — anos acadêmicos médios fixados por modelo'; END $$;
