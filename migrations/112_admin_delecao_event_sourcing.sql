-- MIGRATION 112 - Deleção auditável de administradores via event sourcing
--
-- Espelha exatamente o padrão da migration 110 (academia): a projeção nunca é
-- fisicamente apagada (nenhum DELETE FROM), apenas marcada com status
-- 'deletado' + deleted_at + deletado_por. O evento AdminDeletado é gravado no
-- spuri_ledger e preserva o histórico completo (quem, quando, motivo).
--
-- A hierarquia de quem pode deletar quem (FPP > ADM > Gerente, mesmo cargo não
-- deleta mesmo cargo, Gerente não deleta ninguém) NÃO é validada neste schema
-- — é responsabilidade de Admin.ValidatePermission em Go (já usada hoje por
-- AtivarAdmin/DesativarAdmin), reaproveitada pelo novo DeletarAdmin.

ALTER TABLE projection_admins
    DROP CONSTRAINT IF EXISTS projection_admins_status_check;

ALTER TABLE projection_admins
    ADD CONSTRAINT projection_admins_status_check
    CHECK (status IN ('ativo', 'inativo', 'deletado'));

ALTER TABLE projection_admins
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP,
    ADD COLUMN IF NOT EXISTS deletado_por UUID;

COMMENT ON COLUMN projection_admins.deleted_at IS
    'Data/hora em que o administrador foi marcado como deletado via evento AdminDeletado';
COMMENT ON COLUMN projection_admins.deletado_por IS
    'Administrador que executou a deleção lógica (hierarquia validada em Admin.ValidatePermission)';

CREATE INDEX IF NOT EXISTS idx_proj_admin_deleted_at
    ON projection_admins(deleted_at)
    WHERE status = 'deletado';

DO $$ BEGIN RAISE NOTICE '✅ MIGRATION 112 - deleção event-sourced de administradores'; END $$;
