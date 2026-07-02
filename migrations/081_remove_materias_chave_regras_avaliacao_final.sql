-- MIGRATION 081 — remove matérias-chave da regra de avaliação final
-- A responsabilidade de materias_chave é exclusiva do curso médio, por ano_academico.
ALTER TABLE projection_regras_avaliacao_final
    DROP COLUMN IF EXISTS materias_chave;
