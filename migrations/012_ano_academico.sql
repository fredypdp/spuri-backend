-- ============================================
-- MIGRATION 012 - Adicionar ano_academico em notas e faltas
-- Representa o ano do estudante (ex: "primeiro_fundamental", "segundo_medio")
-- ============================================

BEGIN;

ALTER TABLE projection_notas
    ADD COLUMN IF NOT EXISTS ano_academico VARCHAR(50);

ALTER TABLE projection_faltas
    ADD COLUMN IF NOT EXISTS ano_academico VARCHAR(50);

COMMENT ON COLUMN projection_notas.ano_academico IS
    'Ano acadêmico do estudante no momento do registro (ex: primeiro_fundamental, segundo_medio, terceiro_ano)';

COMMENT ON COLUMN projection_faltas.ano_academico IS
    'Ano acadêmico do estudante no momento do registro (ex: primeiro_fundamental, segundo_medio, terceiro_ano)';

CREATE INDEX IF NOT EXISTS idx_notas_ano_academico   ON projection_notas(ano_academico);
CREATE INDEX IF NOT EXISTS idx_faltas_ano_academico  ON projection_faltas(ano_academico);

COMMIT;
