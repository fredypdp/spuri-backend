-- MIGRATION 089 - Permitir estudante pendente de documentos na projeção
-- O cadastro assíncrono sem arquivo cria o estudante com status geral
-- 'pendente_documentos' até que os PDFs obrigatórios sejam enviados depois.

BEGIN;

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
        CHECK (status IN ('inativo', 'ativo', 'arquivado', 'pendente_documentos'));

COMMENT ON COLUMN projection_estudantes.status IS
    'Status geral do estudante: inativo | ativo | arquivado | pendente_documentos. O valor pendente_documentos representa cadastro sem arquivos aguardando conclusão documental.';

COMMIT;
