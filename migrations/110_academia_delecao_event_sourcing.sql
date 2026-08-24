-- MIGRATION 110 - Deleção auditável de academias via event sourcing
-- Mantém histórico no ledger e permite reutilizar dados únicos em novos cadastros.

ALTER TABLE projection_academias
    DROP CONSTRAINT IF EXISTS projection_academias_status_check;

ALTER TABLE projection_academias
    ADD CONSTRAINT projection_academias_status_check
    CHECK (status IN ('ativo', 'inativo', 'deletado'));

ALTER TABLE projection_academias
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP,
    ADD COLUMN IF NOT EXISTS deletado_por UUID;

COMMENT ON COLUMN projection_academias.deleted_at IS
    'Data/hora em que a academia foi marcada como deletada via evento AcademiaDeletada';
COMMENT ON COLUMN projection_academias.deletado_por IS
    'Administrador FPP que executou a deleção lógica da academia';

CREATE INDEX IF NOT EXISTS idx_proj_academia_deleted_at
    ON projection_academias(deleted_at)
    WHERE status = 'deletado';

DO $$ BEGIN RAISE NOTICE '✅ MIGRATION 110 - deleção event-sourced de academias'; END $$;
