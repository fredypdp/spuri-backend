-- MIGRATION 051 — Histórico de estudantes por ano letivo em turmas
--
-- Objetivos:
-- 1) Permitir que cada turma mantenha um histórico por ano letivo dos
--    estudantes que já fizeram parte dela.
-- 2) Suportar remoção atômica na projeção de turmas a partir do evento
--    AvaliacaoFinalAnoAcademico.

ALTER TABLE projection_turmas
    ADD COLUMN IF NOT EXISTS historico_estudantes_ano_letivo JSONB NOT NULL DEFAULT '{}'::jsonb;

COMMENT ON COLUMN projection_turmas.historico_estudantes_ano_letivo IS
    'Mapa JSONB: ano_letivo -> lista de codigo_estudante que já fizeram parte da turma no ano.';

DO $$
BEGIN
    RAISE NOTICE '✅ MIGRATION 051 — histórico por ano letivo adicionado em projection_turmas';
END $$;
