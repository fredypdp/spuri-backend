-- MIGRATION 088 — Remover soft delete operacional de notas e faltas
--
-- Notas e faltas são recursos somente de criação e leitura. Não há mais fluxo
-- público, administrativo, batch ou assíncrono para editar, excluir, restaurar
-- ou ocultar registros por soft delete. Esta migration remove os artefatos de
-- exclusão lógica que eram exclusivos das mutações removidas e recria índices
-- de consulta sem predicado deleted_at.

BEGIN;

DROP INDEX IF EXISTS idx_notas_deletado_por;
DROP INDEX IF EXISTS idx_faltas_deletado_por;
DROP INDEX IF EXISTS idx_notas_unica_lookup;
DROP INDEX IF EXISTS idx_faltas_estudante_ativo;
DROP INDEX IF EXISTS idx_faltas_academia_ativo;

ALTER TABLE projection_notas
    DROP COLUMN IF EXISTS deletado_por,
    DROP COLUMN IF EXISTS motivo_exclusao,
    DROP COLUMN IF EXISTS deleted_at;

ALTER TABLE projection_faltas
    DROP COLUMN IF EXISTS deletado_por,
    DROP COLUMN IF EXISTS motivo_exclusao,
    DROP COLUMN IF EXISTS deleted_at;

CREATE INDEX IF NOT EXISTS idx_notas_unica_lookup
    ON projection_notas (
        codigo_estudante,
        codigo_academia,
        ano_lectivo,
        periodo,
        materia_disciplinar_id,
        tipo,
        categoria
    );

CREATE INDEX IF NOT EXISTS idx_faltas_estudante_lookup
    ON projection_faltas (codigo_estudante);

CREATE INDEX IF NOT EXISTS idx_faltas_academia_lookup
    ON projection_faltas (codigo_academia);

INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at, events_processed)
VALUES ('migration_088_remover_soft_delete_notas_faltas', 0, CURRENT_TIMESTAMP, 0)
ON CONFLICT (projection_name) DO NOTHING;

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 088 CONCLUÍDA — soft delete de notas/faltas removido da superfície operacional';
END $$;
