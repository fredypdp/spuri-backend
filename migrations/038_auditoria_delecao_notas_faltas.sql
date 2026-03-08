-- ============================================================================
-- MIGRATION 038 — Auditoria de deleção de notas e faltas
--
-- PROBLEMA CORRIGIDO (ambas as tabelas):
--   Os eventos NotaDeletada e FaltaDeletada gravam DeletadoPor e Motivo no
--   payload do ledger, mas as projeções não tinham colunas para expor essa
--   informação. A pergunta "quem deletou e por qual motivo?" exigia inspecionar
--   o spuri_ledger manualmente.
--
--   Adicionalmente, os handlers usavam NOW() em vez de ler DeletedAt do
--   payload — num rebuild, o deleted_at gravado refletiria o momento do
--   rebuild e não o momento real da deleção.
--
-- O QUE ESTA MIGRATION FAZ:
--   1. projection_notas  — adiciona deletado_por UUID e motivo_exclusao TEXT
--   2. projection_faltas — idem
--   3. Índices de auditoria em ambas as tabelas
--
-- AÇÃO REQUERIDA APÓS ESTA MIGRATION:
--   POST /admin/rebuild-projection/notas
--   POST /admin/rebuild-projection/faltas
-- ============================================================================

BEGIN;

-- ── projection_notas ──────────────────────────────────────────────────────

ALTER TABLE projection_notas
    ADD COLUMN IF NOT EXISTS deletado_por    UUID DEFAULT NULL;

ALTER TABLE projection_notas
    ADD COLUMN IF NOT EXISTS motivo_exclusao TEXT DEFAULT NULL;

COMMENT ON COLUMN projection_notas.deletado_por IS
    'UUID da academia que realizou o soft delete. '
    'Preenchido via evento NotaDeletada — campo DeletadoPor do payload. '
    'NULL enquanto a nota não tiver sido deletada.';

COMMENT ON COLUMN projection_notas.motivo_exclusao IS
    'Justificativa obrigatória informada ao deletar a nota. '
    'Preenchido via evento NotaDeletada — campo Motivo do payload. '
    'NULL enquanto a nota não tiver sido deletada.';

CREATE INDEX IF NOT EXISTS idx_notas_deletado_por
    ON projection_notas (deletado_por)
    WHERE deletado_por IS NOT NULL;

-- ── projection_faltas ─────────────────────────────────────────────────────

ALTER TABLE projection_faltas
    ADD COLUMN IF NOT EXISTS deletado_por    UUID DEFAULT NULL;

ALTER TABLE projection_faltas
    ADD COLUMN IF NOT EXISTS motivo_exclusao TEXT DEFAULT NULL;

COMMENT ON COLUMN projection_faltas.deletado_por IS
    'UUID da academia que realizou o soft delete. '
    'Preenchido via evento FaltaDeletada — campo DeletadoPor do payload. '
    'NULL enquanto a falta não tiver sido deletada.';

COMMENT ON COLUMN projection_faltas.motivo_exclusao IS
    'Justificativa obrigatória informada ao deletar a falta. '
    'Preenchido via evento FaltaDeletada — campo Motivo do payload. '
    'NULL enquanto a falta não tiver sido deletada.';

CREATE INDEX IF NOT EXISTS idx_faltas_deletado_por
    ON projection_faltas (deletado_por)
    WHERE deletado_por IS NOT NULL;

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 038 CONCLUÍDA';
    RAISE NOTICE '   → deletado_por e motivo_exclusao adicionados a projection_notas e projection_faltas';
    RAISE NOTICE '   → Execute POST /admin/rebuild-projection/notas';
    RAISE NOTICE '   → Execute POST /admin/rebuild-projection/faltas';
END $$;
