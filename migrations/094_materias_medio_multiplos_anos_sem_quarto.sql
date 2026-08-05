-- MIGRATION 094 — Documentar matérias médias multi-ano sem 4º ano
-- A regra é aplicada no domínio/handlers: matérias do médio aceitam múltiplos
-- anos_academicos, mas não podem incluir 4_ano_medio.

COMMENT ON COLUMN projection_materias.anos_academicos IS
    'AnosAcademicos da matéria disciplinar. Fundamental: um ou mais anos. Médio: um ou mais anos do 1º ao 3º ano, nunca 4_ano_medio. Superior: período/ano acadêmico associado ao curso.';

DO $$ BEGIN RAISE NOTICE '✅ MIGRATION 094 — comentário de anos_academicos das matérias atualizado'; END $$;
