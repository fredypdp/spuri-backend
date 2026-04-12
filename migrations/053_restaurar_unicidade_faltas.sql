-- MIGRATION 053 — Restaurar unicidade de faltas por ano+data+matéria
--
-- Contexto:
--   A migration 052 removeu a UNIQUE de projection_faltas.
--   Esta migration restaura a regra de unicidade para faltas,
--   mantendo a quantidade sem teto máximo (apenas > 0).

BEGIN;

-- 1) Consolidar duplicatas ativas (deleted_at IS NULL) para permitir recriar UNIQUE.
--    Estratégia: manter o menor id e somar quantidades dos duplicados.
WITH duplicados AS (
    SELECT
        codigo_estudante,
        codigo_academia,
        data,
        materia_disciplinar_id,
        MIN(id) AS keep_id,
        SUM(quantidade) AS total_qtd,
        ARRAY_AGG(id) AS ids
    FROM projection_faltas
    WHERE deleted_at IS NULL
    GROUP BY codigo_estudante, codigo_academia, data, materia_disciplinar_id
    HAVING COUNT(*) > 1
), atualiza_keep AS (
    UPDATE projection_faltas f
    SET quantidade = d.total_qtd
    FROM duplicados d
    WHERE f.id = d.keep_id
    RETURNING f.id
)
DELETE FROM projection_faltas f
USING duplicados d
WHERE f.id = ANY(d.ids)
  AND f.id <> d.keep_id;

-- 2) Remover constraint UNIQUE antiga (se existir com nome variável)
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
          ) = ARRAY['codigo_estudante','codigo_academia','data','materia_disciplinar_id']::name[]
    LOOP
        EXECUTE format('ALTER TABLE projection_faltas DROP CONSTRAINT %I', r.conname);
        RAISE NOTICE 'ℹ️ Constraint antiga removida: %', r.conname;
    END LOOP;
END
$$;

-- 3) Recriar UNIQUE canônica
ALTER TABLE projection_faltas
    ADD CONSTRAINT uq_falta_unica
        UNIQUE (codigo_estudante, codigo_academia, data, materia_disciplinar_id);

COMMIT;
