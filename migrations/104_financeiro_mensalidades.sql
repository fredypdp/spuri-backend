-- Fase 2 do financeiro: projeções derivadas para mensalidades/propinas.
-- Todos os fatos destas tabelas são reconstruíveis a partir do spuri_ledger.

CREATE TABLE financeiro_mensalidade_configuracoes (
    event_id UUID PRIMARY KEY,
    aggregate_id UUID NOT NULL,
    codigo_academia TEXT NOT NULL,
    nivel VARCHAR(16) NOT NULL CHECK (nivel IN ('fundamental', 'medio', 'superior')),
    ano_academico TEXT NOT NULL,
    curso_id UUID NULL,
    valor NUMERIC(14,2) NOT NULL CHECK (valor > 0),
    mes_fim_cobranca SMALLINT NOT NULL CHECK (mes_fim_cobranca IN (6, 7)),
    vigente_em TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_fin_mensalidade_config_lookup
    ON financeiro_mensalidade_configuracoes
       (codigo_academia, nivel, ano_academico, curso_id, vigente_em DESC);

CREATE TABLE financeiro_mensalidade_inicio_cobranca (
    event_id UUID PRIMARY KEY,
    aggregate_id UUID NOT NULL,
    codigo_academia TEXT NOT NULL,
    ano_letivo VARCHAR(9) NOT NULL,
    mes_inicio SMALLINT NOT NULL CHECK (mes_inicio BETWEEN 1 AND 12),
    definido_em TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_fin_mensalidade_inicio_lookup
    ON financeiro_mensalidade_inicio_cobranca (codigo_academia, ano_letivo, definido_em DESC);

CREATE TABLE financeiro_mensalidade_obrigacoes_eventos (
    event_id UUID PRIMARY KEY,
    aggregate_id UUID NOT NULL,
    codigo_estudante TEXT NOT NULL,
    codigo_academia TEXT NOT NULL,
    ano_letivo VARCHAR(9) NOT NULL,
    mes SMALLINT NOT NULL CHECK (mes BETWEEN 1 AND 12),
    tipo VARCHAR(16) NOT NULL CHECK (tipo IN ('anulada', 'reativada', 'paga')),
    motivo TEXT NULL,
    ocorrido_em TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_fin_mensalidade_obrigacao_lookup
    ON financeiro_mensalidade_obrigacoes_eventos
       (codigo_estudante, codigo_academia, ano_letivo, mes, ocorrido_em DESC);

COMMENT ON TABLE financeiro_mensalidade_configuracoes IS
    'Histórico imutável de MensalidadeConfigurada; a versão vigente é resolvida pela data do mês devido.';
COMMENT ON TABLE financeiro_mensalidade_obrigacoes_eventos IS
    'Eventos de anulação, reativação e pagamento por estudante, academia, ano letivo e mês.';
