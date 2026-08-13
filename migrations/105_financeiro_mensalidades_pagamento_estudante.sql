-- Fase 3: métodos habilitados para propina e vínculo imutável cobrança/mês.
ALTER TABLE financeiro_mensalidade_configuracoes
    ADD COLUMN IF NOT EXISTS metodos_pagamento TEXT[] NOT NULL DEFAULT '{}';

ALTER TABLE financeiro_mensalidade_obrigacoes_eventos
    ADD COLUMN IF NOT EXISTS charge_id UUID NULL;

ALTER TABLE financeiro_mensalidade_obrigacoes_eventos
    DROP CONSTRAINT IF EXISTS financeiro_mensalidade_obrigacoes_eventos_pkey;
ALTER TABLE financeiro_mensalidade_obrigacoes_eventos
    ADD PRIMARY KEY (event_id, codigo_estudante, codigo_academia, ano_letivo, mes);
CREATE UNIQUE INDEX IF NOT EXISTS uq_fin_mensalidade_pagamento_por_cobranca
    ON financeiro_mensalidade_obrigacoes_eventos (charge_id, codigo_estudante, codigo_academia, ano_letivo, mes)
    WHERE charge_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS financeiro_mensalidade_cobrancas (
    charge_id UUID NOT NULL REFERENCES financeiro_cobrancas(id),
    codigo_estudante TEXT NOT NULL,
    codigo_academia TEXT NOT NULL,
    ano_letivo VARCHAR(9) NOT NULL,
    mes SMALLINT NOT NULL CHECK (mes BETWEEN 1 AND 12),
    PRIMARY KEY (charge_id, codigo_estudante, codigo_academia, ano_letivo, mes)
);
CREATE INDEX IF NOT EXISTS idx_fin_mensalidade_cobrancas_mes
    ON financeiro_mensalidade_cobrancas (codigo_estudante, codigo_academia, ano_letivo, mes);
