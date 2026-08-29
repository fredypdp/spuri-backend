-- MIGRATION 113 - Deleção auditável (autodeleção) de estudantes via event sourcing
--
-- Espelha exatamente o padrão da migration 110 (academia). A projeção nunca é
-- fisicamente apagada (nenhum DELETE FROM), apenas marcada com status
-- 'deletado' + deleted_at + deletado_por. O evento EstudanteDeletado é gravado
-- no spuri_ledger e preserva o histórico completo. Notas, faltas e avaliações
-- do estudante permanecem intactas (nenhuma FK em cascata para essas tabelas).
--
-- IMPORTANTE (achado durante a investigação, ver documento da tarefa):
-- codigo_academia NUNCA é limpo quando um estudante se desvincula de uma
-- academia (handleEstudanteDesvinculadoDaAcademia só muda o status) —
-- permanece preenchido para fins históricos. Por isso "vinculado a uma
-- academia" é definido por status IN ('ativo', 'pendente_documentos'), nunca
-- por codigo_academia IS NULL. A regra de negócio ("apenas se não estiver
-- vinculado a nenhuma academia") é validada em Go em Estudante.Deletar
-- verificando e.Status == "inativo".

ALTER TABLE projection_estudantes
    DROP CONSTRAINT IF EXISTS projection_estudantes_status_check;

ALTER TABLE projection_estudantes
    ADD CONSTRAINT projection_estudantes_status_check
    CHECK (status IN ('ativo', 'inativo', 'pendente_documentos', 'deletado'));

ALTER TABLE projection_estudantes
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP,
    ADD COLUMN IF NOT EXISTS deletado_por UUID;

COMMENT ON COLUMN projection_estudantes.deleted_at IS
    'Data/hora em que o estudante foi marcado como deletado via evento EstudanteDeletado';
COMMENT ON COLUMN projection_estudantes.deletado_por IS
    'Sempre o próprio estudante (autodeleção) — mesmo valor do id da projection_estudantes';

CREATE INDEX IF NOT EXISTS idx_proj_estudante_deleted_at
    ON projection_estudantes(deleted_at)
    WHERE status = 'deletado';

DO $$ BEGIN RAISE NOTICE '✅ MIGRATION 113 - deleção event-sourced (autodeleção) de estudantes'; END $$;
