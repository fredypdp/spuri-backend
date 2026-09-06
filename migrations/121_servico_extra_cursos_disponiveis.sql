-- ============================================================================
-- MIGRATION 121 — Adiciona cursos_disponiveis a projection_servicos_extras
-- ============================================================================
--
-- Cada item usa o formato "<curso_id>|<ano_academico>". A validação de
-- formato ocorre no aggregate; a posse e consistência do curso no handler.
-- A lista é combinável com anos_academicos_disponiveis sem reinterpretar
-- registros existentes.

BEGIN;

ALTER TABLE projection_servicos_extras
    ADD COLUMN cursos_disponiveis TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[];

COMMENT ON COLUMN projection_servicos_extras.cursos_disponiveis IS
    'Cada item no formato "<curso_id>|<ano_academico>" (ano_academico termina em _ano_medio ou _ano_superior — nunca _ano_fundamental). Lista vazia junto com anos_academicos_disponiveis vazio = disponível para todos os anos/cursos. Combinável com anos_academicos_disponiveis (migration 118). Adicionada na migration 121.';

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 121 — cursos_disponiveis adicionada a projection_servicos_extras';
END $$;
