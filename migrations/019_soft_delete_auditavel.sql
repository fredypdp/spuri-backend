-- ============================================================
-- MIGRATION 019 — Suporte a deleção auditável (soft delete)
-- Afeta: projection_turmas, projection_cursos, projection_materias
-- ============================================================
--
-- CONTEXTO:
--   Deleção física (DELETE FROM) é incompatível com event sourcing imutável:
--   o ledger guarda eventos para sempre, então a projeção deve refletir
--   o soft delete via campo deleted_at, mantendo o registro para rebuild.
--
-- O QUE ESTA MIGRATION FAZ:
--   1. projection_turmas  — adiciona deleted_at, atualiza CHECK de status
--      para incluir 'deletado', cria índice parcial.
--   2. projection_cursos  — idem.
--   3. projection_materias — idem. (Anteriormente usava DELETE físico;
--      esta migration converte para soft delete para garantir consistência
--      de rebuild via evento MateriaDeletada.)
-- ============================================================

-- ── projection_turmas ──────────────────────────────────────────────────────

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

-- ── projection_cursos ──────────────────────────────────────────────────────

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

-- ── projection_materias — convertida de DELETE físico para soft delete ─────
--
-- NOTA (ERRO-MIG-03 FIX): O comentário original desta migration dizia
-- "projection_materias já tem suporte via DELETE físico" e tratava a
-- conversão para soft delete como opcional. Isso era contraditório pois
-- o corpo da migration adiciona deleted_at e atualiza o CHECK de status.
-- Soft delete em projection_materias é OBRIGATÓRIO para que o evento
-- MateriaDeletada possa ser reprocessado corretamente em um rebuild.

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

-- ── Checkpoints existentes não precisam de nova entrada ────────────────────

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 019 CONCLUÍDA — soft delete auditável para turmas, cursos e matérias';
END $$;
