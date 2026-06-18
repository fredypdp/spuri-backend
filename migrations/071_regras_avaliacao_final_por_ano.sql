BEGIN;

-- A migration 070 deixava apenas uma regra ativa por academia/type/tipo_ensino,
-- mesmo que os anos acadêmicos fossem disjuntos. A tarefa 08 exige regras por
-- ano acadêmico; a validação de sobreposição por ano fica na aplicação porque
-- anos_academicos é JSONB.
DROP INDEX IF EXISTS uq_regra_avaliacao_final_ativa;
CREATE INDEX IF NOT EXISTS idx_regras_avf_lookup
ON projection_regras_avaliacao_final (codigo_academia, tipo_ensino, type, status);
CREATE INDEX IF NOT EXISTS idx_regras_avf_anos
ON projection_regras_avaliacao_final USING GIN (anos_academicos);

COMMIT;
