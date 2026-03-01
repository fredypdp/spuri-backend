-- ============================================================
-- MIGRATION 009 — Suporte a deleção auditável (soft delete)
-- Afeta: projection_turmas, projection_cursos
-- (projection_materias já tem suporte via DELETE físico;
--  alterar para soft delete se quiser auditabilidade igual)
-- ============================================================

-- ── projection_turmas ──────────────────────────────────────

ALTER TABLE projection_turmas
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP DEFAULT NULL;

-- Adicionar 'deletado' como valor permitido no CHECK de status
ALTER TABLE projection_turmas
    DROP CONSTRAINT IF EXISTS projection_turmas_status_check;

ALTER TABLE projection_turmas
    ADD CONSTRAINT projection_turmas_status_check
        CHECK (status IN ('ativo', 'inativo', 'deletado'));

-- Índice para filtrar registros não-deletados eficientemente
CREATE INDEX IF NOT EXISTS idx_turmas_not_deleted
    ON projection_turmas (codigo_academia)
    WHERE deleted_at IS NULL;

COMMENT ON COLUMN projection_turmas.deleted_at IS
    'Preenchido via evento TurmaDeletada — registro mantido para auditoria';

-- ── projection_cursos ──────────────────────────────────────

ALTER TABLE projection_cursos
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP DEFAULT NULL;

ALTER TABLE projection_cursos
    DROP CONSTRAINT IF EXISTS projection_cursos_status_check;

ALTER TABLE projection_cursos
    ADD CONSTRAINT projection_cursos_status_check
        CHECK (status IN ('ativo', 'inativo', 'deletado'));

CREATE INDEX IF NOT EXISTS idx_cursos_not_deleted
    ON projection_cursos (codigo_academia)
    WHERE deleted_at IS NULL;

COMMENT ON COLUMN projection_cursos.deleted_at IS
    'Preenchido via evento CursoDeletado — registro mantido para auditoria';

-- ── projection_materias: converter de DELETE físico para soft delete ───────
-- (opcional mas recomendado para consistência com os demais)

ALTER TABLE projection_materias
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP DEFAULT NULL;

ALTER TABLE projection_materias
    DROP CONSTRAINT IF EXISTS projection_materias_status_check;

ALTER TABLE projection_materias
    ADD CONSTRAINT projection_materias_status_check
        CHECK (status IN ('ativo', 'inativo', 'deletado'));

CREATE INDEX IF NOT EXISTS idx_materias_not_deleted
    ON projection_materias (codigo_academia)
    WHERE deleted_at IS NULL;

COMMENT ON COLUMN projection_materias.deleted_at IS
    'Preenchido via evento MateriaDeletada — registro mantido para auditoria';

-- ── Novos checkpoints (se ainda não existirem) ─────────────
-- Os checkpoints de turmas/cursos/materias já existem;
-- nenhuma entrada nova necessária.

RAISE NOTICE '✅ MIGRATION 009 CONCLUÍDA — soft delete auditável para turmas, cursos e matérias';
