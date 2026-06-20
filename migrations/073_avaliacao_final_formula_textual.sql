BEGIN;

DO $$
DECLARE
    regra_formula_type text;
    snapshot_type text;
BEGIN
    SELECT data_type INTO regra_formula_type
    FROM information_schema.columns
    WHERE table_name = 'projection_regras_avaliacao_final'
      AND column_name = 'formula';

    IF regra_formula_type = 'jsonb' THEN
        ALTER TABLE projection_regras_avaliacao_final
            ALTER COLUMN formula TYPE TEXT USING CASE
                WHEN jsonb_typeof(formula) = 'string' THEN trim(both '"' from formula::text)
                ELSE formula::text
            END;
    ELSIF regra_formula_type IS NOT NULL AND regra_formula_type <> 'text' THEN
        ALTER TABLE projection_regras_avaliacao_final
            ALTER COLUMN formula TYPE TEXT USING formula::text;
    END IF;

    SELECT data_type INTO snapshot_type
    FROM information_schema.columns
    WHERE table_name = 'projection_avaliacao_final'
      AND column_name = 'formula_snapshot';

    IF snapshot_type = 'jsonb' THEN
        ALTER TABLE projection_avaliacao_final
            ALTER COLUMN formula_snapshot TYPE TEXT USING CASE
                WHEN formula_snapshot IS NULL THEN NULL
                WHEN jsonb_typeof(formula_snapshot) = 'string' THEN trim(both '"' from formula_snapshot::text)
                ELSE formula_snapshot::text
            END;
    ELSIF snapshot_type IS NOT NULL AND snapshot_type <> 'text' THEN
        ALTER TABLE projection_avaliacao_final
            ALTER COLUMN formula_snapshot TYPE TEXT USING formula_snapshot::text;
    END IF;
END $$;

COMMENT ON COLUMN projection_regras_avaliacao_final.formula IS
    'Fórmula textual declarativa de avaliação final no formato [categoria,periodo] com operadores + - * / e parênteses; validada por parser próprio.';

COMMENT ON COLUMN projection_avaliacao_final.formula_snapshot IS
    'Snapshot textual da fórmula de avaliação final usada no momento do cálculo.';

COMMIT;
