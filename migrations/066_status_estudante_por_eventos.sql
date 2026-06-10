-- MIGRATION 066 - Status do estudante controlados por acontecimentos de domínio
-- Remove o valor legado 'finalizado' do status geral do estudante e adiciona
-- 'arquivado' para preservar histórico após desvinculação da academia.

BEGIN;

UPDATE projection_estudantes
SET status = 'arquivado'
WHERE status = 'finalizado';

DO $$
DECLARE
    constraint_name text;
BEGIN
    SELECT conname INTO constraint_name
    FROM pg_constraint
    WHERE conrelid = 'projection_estudantes'::regclass
      AND contype = 'c'
      AND pg_get_constraintdef(oid) LIKE '%status%'
      AND pg_get_constraintdef(oid) LIKE '%finalizado%'
      AND pg_get_constraintdef(oid) NOT LIKE '%status_escolar%'
      AND pg_get_constraintdef(oid) NOT LIKE '%status_superior%'
    LIMIT 1;

    IF constraint_name IS NOT NULL THEN
        EXECUTE format('ALTER TABLE projection_estudantes DROP CONSTRAINT %I', constraint_name);
    END IF;
END $$;

ALTER TABLE projection_estudantes
    ADD CONSTRAINT projection_estudantes_status_check
        CHECK (status IN ('inativo', 'ativo', 'arquivado'));

COMMENT ON COLUMN projection_estudantes.status IS
    'Status geral do estudante: inativo | ativo | arquivado. O valor finalizado pertence apenas aos status escolares.';

COMMIT;
