-- Persistência do módulo financeiro base AppyPay.
CREATE TABLE IF NOT EXISTS financeiro_credenciais_appypay (
    id UUID PRIMARY KEY,
    payload JSONB NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS financeiro_cobrancas (
    id UUID PRIMARY KEY,
    idempotency_key TEXT UNIQUE NOT NULL,
    payload JSONB NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS financeiro_webhooks_recebidos (
    event_id TEXT PRIMARY KEY,
    received_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS financeiro_modalidade_pagamento (
    id TEXT PRIMARY KEY DEFAULT 'default',
    payload JSONB NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT financeiro_modalidade_pagamento_singleton CHECK (id = 'default')
);

CREATE INDEX IF NOT EXISTS idx_financeiro_credenciais_payload_contexto
    ON financeiro_credenciais_appypay ((payload->>'ContextoTipo'), (payload->>'CodigoAcademia'));

CREATE INDEX IF NOT EXISTS idx_financeiro_cobrancas_payload_contexto
    ON financeiro_cobrancas ((payload->>'ContextoTipo'), (payload->>'CodigoAcademia'));
