ALTER TABLE projection_solicitacoes_servico_extra ADD COLUMN IF NOT EXISTS vinculada_em TIMESTAMPTZ;

CREATE TABLE IF NOT EXISTS financeiro_servico_extra_obrigacoes_eventos (
    id BIGSERIAL PRIMARY KEY, event_id UUID NOT NULL, solicitacao_id UUID NOT NULL,
    tipo_lancamento VARCHAR(20) NOT NULL CHECK (tipo_lancamento IN ('mensalidade','preco_unico')),
    ano INTEGER, mes INTEGER CHECK (mes IS NULL OR (mes >= 1 AND mes <= 12)),
    tipo VARCHAR(10) NOT NULL CHECK (tipo IN ('anulada','reativada','paga')),
    ocorrido_em TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_servico_extra_obrigacoes_busca ON financeiro_servico_extra_obrigacoes_eventos(solicitacao_id, tipo_lancamento, ano, mes);
CREATE UNIQUE INDEX IF NOT EXISTS ux_servico_extra_obrigacoes_event_id ON financeiro_servico_extra_obrigacoes_eventos(event_id);
