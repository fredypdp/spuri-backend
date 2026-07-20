-- MIGRATION 066 - Status do estudante controlados por acontecimentos de domínio
-- Remove o valor legado 'finalizado' do status geral do estudante e adiciona
-- 'inativo' para preservar histórico após desvinculação da academia.

BEGIN;

UPDATE projection_estudantes
SET status = 'inativo'
WHERE status = 'finalizado';

DO $$
DECLARE
    constraint_record record;
BEGIN
    FOR constraint_record IN
        SELECT conname
        FROM pg_constraint
        WHERE conrelid = 'projection_estudantes'::regclass
          AND contype = 'c'
          AND pg_get_constraintdef(oid) ~ '\mstatus\M'
          AND pg_get_constraintdef(oid) !~ '\mstatus_escolar\M'
          AND pg_get_constraintdef(oid) !~ '\mstatus_superior\M'
    LOOP
        EXECUTE format('ALTER TABLE projection_estudantes DROP CONSTRAINT %I', constraint_record.conname);
    END LOOP;
END $$;

ALTER TABLE projection_estudantes
    ADD CONSTRAINT projection_estudantes_status_check
        CHECK (status IN ('inativo', 'ativo'));

COMMENT ON COLUMN projection_estudantes.status IS
    'Status geral do estudante: inativo | ativo. O valor finalizado pertence apenas aos status escolares.';

COMMIT;
