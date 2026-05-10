-- MIGRATION 034 - Padroniza coluna ano_escolar -> ano_escolar_fundamental

ALTER TABLE projection_estudantes
    ADD COLUMN IF NOT EXISTS ano_escolar_fundamental VARCHAR(50);

-- Backfill inicial a partir da coluna legada
UPDATE projection_estudantes
SET ano_escolar_fundamental = ano_escolar
WHERE ano_escolar_fundamental IS NULL
  AND ano_escolar IS NOT NULL;

-- Mantém compatibilidade temporária: coluna legada recebe o mesmo valor
UPDATE projection_estudantes
SET ano_escolar = ano_escolar_fundamental
WHERE ano_escolar IS DISTINCT FROM ano_escolar_fundamental;

COMMENT ON COLUMN projection_estudantes.ano_escolar_fundamental IS
    'Ano escolar do ensino fundamental (formato [1-9]_ano_fundamental).';

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 034 - coluna ano_escolar_fundamental padronizada';
END $$;
