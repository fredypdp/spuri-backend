-- MIGRATION 080 - NIF obrigatório e único para academias
ALTER TABLE projection_academias
    ADD COLUMN IF NOT EXISTS nif VARCHAR(10);

UPDATE projection_academias
SET nif = LPAD(SUBSTRING(REGEXP_REPLACE(codigo_academia, '[^0-9]', '', 'g') FROM 1 FOR 10), 10, '0')
WHERE nif IS NULL;

DO $$ BEGIN
    ALTER TABLE projection_academias
        ADD CONSTRAINT check_academia_nif_10_digits CHECK (nif ~ '^[0-9]{10}$');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS idx_projection_academias_nif_unique
    ON projection_academias(nif);

ALTER TABLE projection_academias
    ALTER COLUMN nif SET NOT NULL;

COMMENT ON COLUMN projection_academias.nif IS 'NIF da academia: string obrigatória, única e com exatamente 10 dígitos';
