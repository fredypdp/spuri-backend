-- MIGRATION 052 — Notas sem teto máximo e faltas sem unicidade por data+matéria
--
-- Objetivo:
--   1) Permitir notas com qualquer valor >= 0 (remover teto <= 20).
--   2) Permitir múltiplos registros de falta na mesma combinação
--      (estudante, academia, data, matéria).
--
-- Impacto:
--   - projection_notas.nota continua obrigatória e não-negativa.
--   - projection_faltas deixa de impor UNIQUE por data+matéria.

BEGIN;

-- 1) Notas: remover check antigo e manter apenas nota >= 0
ALTER TABLE projection_notas
    DROP CONSTRAINT IF EXISTS projection_notas_nota_check;

ALTER TABLE projection_notas
    ADD CONSTRAINT projection_notas_nota_check
        CHECK (nota >= 0);

COMMENT ON COLUMN projection_notas.nota IS
    'Nota sem teto máximo; valor deve ser maior ou igual a 0.';

-- 2) Faltas: remover UNIQUE(codigo_estudante, codigo_academia, data, materia_disciplinar_id)
DO $$
DECLARE
    r RECORD;
BEGIN
    FOR r IN
        SELECT c.conname
        FROM pg_constraint c
        JOIN pg_class t ON t.oid = c.conrelid
        WHERE t.relname = 'projection_faltas'
          AND c.contype = 'u'
          AND (
              SELECT array_agg(a.attname ORDER BY u.ord)
              FROM unnest(c.conkey) WITH ORDINALITY AS u(attnum, ord)
              JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = u.attnum
          ) = ARRAY['codigo_estudante','codigo_academia','data','materia_disciplinar_id']
    LOOP
        EXECUTE format('ALTER TABLE projection_faltas DROP CONSTRAINT %I', r.conname);
        RAISE NOTICE '✅ Constraint removida de projection_faltas: %', r.conname;
    END LOOP;
END
$$;

COMMIT;
