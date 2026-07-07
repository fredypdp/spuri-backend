-- MIGRATION 086 — Fixar períodos imutáveis dos anos letivos por tipo.
-- A regra de negócio deixa de tratar `periodo` como configuração editável:
-- escolar => 09_07; superior => 10_07.

UPDATE projection_anos_letivos_configuracoes
SET periodo = CASE type
  WHEN 'escolar' THEN '09_07'
  WHEN 'superior' THEN '10_07'
END,
updated_at = NOW()
WHERE type IN ('escolar', 'superior')
  AND periodo IS DISTINCT FROM CASE type
    WHEN 'escolar' THEN '09_07'
    WHEN 'superior' THEN '10_07'
  END;

INSERT INTO projection_anos_letivos_configuracoes (type, periodo)
VALUES ('escolar', '09_07'), ('superior', '10_07')
ON CONFLICT (type) DO UPDATE
SET periodo = EXCLUDED.periodo,
    updated_at = NOW();

ALTER TABLE projection_anos_letivos_configuracoes
  DROP CONSTRAINT IF EXISTS chk_anos_letivos_periodos_fixos;

ALTER TABLE projection_anos_letivos_configuracoes
  ADD CONSTRAINT chk_anos_letivos_periodos_fixos CHECK (
    (type = 'escolar' AND periodo = '09_07')
    OR (type = 'superior' AND periodo = '10_07')
  );
