BEGIN;

CREATE TABLE IF NOT EXISTS projection_regras_avaliacao_final (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    codigo_academia VARCHAR(50) NOT NULL REFERENCES projection_academias(codigo_academia) ON DELETE CASCADE,
    type VARCHAR(50) NOT NULL DEFAULT 'normal',
    nome VARCHAR(120) NOT NULL,
    descricao TEXT,
    tipo_ensino VARCHAR(20) NOT NULL CHECK (tipo_ensino IN ('fundamental','medio','superior')),
    anos_academicos JSONB NOT NULL DEFAULT '[]'::jsonb,
    nota_minima_aprovacao NUMERIC(10,2) NOT NULL CHECK (nota_minima_aprovacao > 0),
    categorias_envolvidas JSONB NOT NULL DEFAULT '[]'::jsonb,
    formula TEXT NOT NULL,
    aplica_se_reprovado_em_type VARCHAR(50),
    status VARCHAR(20) NOT NULL DEFAULT 'ativo' CHECK (status IN ('ativo','inativo')),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    version INTEGER NOT NULL DEFAULT 1,
    CHECK (aplica_se_reprovado_em_type IS NULL OR aplica_se_reprovado_em_type <> type)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_regra_avaliacao_final_ativa
ON projection_regras_avaliacao_final (codigo_academia, type, tipo_ensino)
WHERE status = 'ativo';

ALTER TABLE projection_avaliacao_final
    ADD COLUMN IF NOT EXISTS type VARCHAR(50) NOT NULL DEFAULT 'normal',
    ADD COLUMN IF NOT EXISTS nota_final NUMERIC(10,2),
    ADD COLUMN IF NOT EXISTS nota_minima_aprovacao NUMERIC(10,2),
    ADD COLUMN IF NOT EXISTS regra_avaliacao_final_id UUID,
    ADD COLUMN IF NOT EXISTS formula_snapshot TEXT,
    ADD COLUMN IF NOT EXISTS aplica_se_reprovado_em_type VARCHAR(50);

DROP INDEX IF EXISTS uq_avaliacao_final_estudante_ano_letivo;
ALTER TABLE projection_avaliacao_final DROP CONSTRAINT IF EXISTS projection_avaliacao_final_codigo_estudante_codigo_academia_ano_key;
ALTER TABLE projection_avaliacao_final DROP CONSTRAINT IF EXISTS projection_avaliacao_final_codigo_estudante_codigo_academia_a_key;
CREATE UNIQUE INDEX IF NOT EXISTS uq_avaliacao_final_estudante_ano_tipo
ON projection_avaliacao_final (codigo_estudante, codigo_academia, ano_lectivo, tipo_ensino, ano_academico_atual, type);

CREATE INDEX IF NOT EXISTS idx_avf_type ON projection_avaliacao_final(type);
CREATE INDEX IF NOT EXISTS idx_regras_avf_academia ON projection_regras_avaliacao_final(codigo_academia, status);

COMMIT;
