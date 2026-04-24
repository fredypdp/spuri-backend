-- MIGRATION 056 — Garantir projection_faltas.data como DATE
--
-- Objetivo:
--   Garantir semanticamente que a data da falta permaneça date-only
--   no banco, sem componente de hora.
--
-- Estratégia:
--   - Se a coluna já for DATE, não faz nada.
--   - Caso esteja em outro tipo temporal, converte para DATE com cast.

DO $$
DECLARE
    col_type text;
BEGIN
    SELECT data_type
      INTO col_type
      FROM information_schema.columns
     WHERE table_schema = 'public'
       AND table_name = 'projection_faltas'
       AND column_name = 'data';

    IF col_type IS NULL THEN
        RAISE EXCEPTION 'Coluna projection_faltas.data não encontrada';
    END IF;

    IF col_type <> 'date' THEN
        ALTER TABLE projection_faltas
            ALTER COLUMN data TYPE DATE
            USING data::date;

        RAISE NOTICE '✅ projection_faltas.data convertido para DATE (tipo anterior: %)', col_type;
    ELSE
        RAISE NOTICE '✅ projection_faltas.data já está em DATE (sem alterações)';
    END IF;
END $$;

COMMENT ON COLUMN projection_faltas.data IS
'Data da falta (date-only, sem hora), no formato ISO AAAA-MM-DD.';
