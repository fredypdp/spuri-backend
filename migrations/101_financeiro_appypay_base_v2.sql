-- Módulo financeiro/AppyPay: read models reconstruíveis e cofre separado.
-- Nenhum segredo ou ciphertext é mantido nos payloads das projeções.
CREATE TABLE IF NOT EXISTS financeiro_credenciais_appypay (
    id UUID PRIMARY KEY,
    contexto_tipo VARCHAR(16) NOT NULL CHECK (contexto_tipo IN ('spuri','academia')),
    codigo_academia TEXT,
    ambiente VARCHAR(8) NOT NULL CHECK (ambiente IN ('test','prod')),
    payload JSONB NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK ((contexto_tipo = 'spuri' AND codigo_academia IS NULL) OR
           (contexto_tipo = 'academia' AND codigo_academia IS NOT NULL))
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_financeiro_credenciais_contexto
 ON financeiro_credenciais_appypay (contexto_tipo, COALESCE(codigo_academia, ''), ambiente);

CREATE TABLE IF NOT EXISTS financeiro_segredos_appypay (
    -- Não há FK intencionalmente: o cofre operacional sobrevive ao rebuild da
    -- projeção de metadados, pois ciphertexts nunca podem vir do ledger.
    credential_id UUID NOT NULL,
    secret_type VARCHAR(64) NOT NULL,
    ciphertext TEXT NOT NULL,
    key_id VARCHAR(128) NOT NULL DEFAULT 'FINANCE_ENCRYPTION_KEY',
    algorithm VARCHAR(64) NOT NULL DEFAULT 'AES-256-GCM',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (credential_id, secret_type)
);

CREATE TABLE IF NOT EXISTS financeiro_cobrancas (
    id UUID PRIMARY KEY,
    provider_charge_id TEXT,
    merchant_transaction_id VARCHAR(15) NOT NULL UNIQUE,
    contexto_tipo VARCHAR(16) NOT NULL,
    codigo_academia TEXT,
    payload JSONB NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_financeiro_cobrancas_provider_id
 ON financeiro_cobrancas(provider_charge_id) WHERE provider_charge_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS financeiro_webhooks_recebidos (
    event_id TEXT PRIMARY KEY,
    metodo VARCHAR(8) NOT NULL CHECK (metodo IN ('GPO','REF')),
    received_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

COMMENT ON TABLE financeiro_segredos_appypay IS 'Cofre operacional AES-256-GCM. Nunca é reconstruído do ledger.';
