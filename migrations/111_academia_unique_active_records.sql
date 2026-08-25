-- MIGRATION 111 - Unicidade operacional de academias ativas
--
-- A projeção de deleção não pode prefixar o NIF para liberar a unicidade:
-- `nif` é VARCHAR(10) e deve continuar contendo exatamente dez dígitos.
-- Os índices parciais preservam os dados auditáveis e permitem reutilizar
-- NIF/e-mail depois que uma academia é logicamente deletada.

ALTER TABLE projection_academias
    DROP CONSTRAINT IF EXISTS projection_academias_email_key;

DROP INDEX IF EXISTS idx_projection_academias_nif_unique;

CREATE UNIQUE INDEX IF NOT EXISTS idx_projection_academias_nif_active_unique
    ON projection_academias(nif)
    WHERE status IS DISTINCT FROM 'deletado';

CREATE UNIQUE INDEX IF NOT EXISTS idx_projection_academias_email_active_unique
    ON projection_academias(email)
    WHERE email IS NOT NULL AND status IS DISTINCT FROM 'deletado';

COMMENT ON INDEX idx_projection_academias_nif_active_unique IS
    'NIF único somente entre academias não deletadas.';
COMMENT ON INDEX idx_projection_academias_email_active_unique IS
    'E-mail único somente entre academias não deletadas.';

DO $$ BEGIN RAISE NOTICE '✅ MIGRATION 111 - unicidade de academia limitada a registros ativos'; END $$;
