-- Módulo financeiro/AppyPay (fase 1). O ledger continua sendo a fonte de verdade;
-- estas tabelas são projections/read models e o cofre operacional de segredos.
CREATE TABLE financeiro_credenciais_appypay (
    id UUID PRIMARY KEY,
    contexto_tipo VARCHAR(16) NOT NULL CHECK (contexto_tipo IN ('spuri', 'academia')),
    -- String vazia representa a conta da própria plataforma; assim a chave
    -- única também protege o escopo Spuri (NULLs seriam distintos no PostgreSQL).
    codigo_academia VARCHAR(64) NOT NULL DEFAULT '',
    ambiente VARCHAR(8) NOT NULL CHECK (ambiente IN ('TEST', 'PROD')),
    client_id_mascarado TEXT NOT NULL,
    resource_mascarado TEXT NOT NULL,
    gpo_method_mascarado TEXT NOT NULL,
    ref_method_mascarado TEXT NOT NULL,
    webhook_auth_type VARCHAR(16),
    webhook_username_mascarado TEXT,
    webhook_secret_mascarado TEXT,
    version INTEGER NOT NULL DEFAULT 1,
    last_event_id UUID,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (contexto_tipo, codigo_academia, ambiente),
    CHECK ((contexto_tipo = 'spuri' AND codigo_academia = '') OR
           (contexto_tipo = 'academia' AND codigo_academia <> ''))
);

CREATE TABLE financeiro_segredos_appypay (
    credential_id UUID NOT NULL REFERENCES financeiro_credenciais_appypay(id) ON DELETE CASCADE,
    secret_version INTEGER NOT NULL,
    secret_type VARCHAR(64) NOT NULL,
    ciphertext TEXT NOT NULL,
    key_id VARCHAR(128) NOT NULL DEFAULT 'FINANCE_ENCRYPTION_KEY',
    algorithm VARCHAR(64) NOT NULL DEFAULT 'AES-256-GCM',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    revoked_at TIMESTAMP,
    PRIMARY KEY (credential_id, secret_type, secret_version)
);

CREATE TABLE financeiro_cobrancas (
    id UUID PRIMARY KEY,
    credential_id UUID NOT NULL REFERENCES financeiro_credenciais_appypay(id),
    contexto_tipo VARCHAR(16) NOT NULL,
    codigo_academia VARCHAR(64),
    merchant_transaction_id VARCHAR(15) NOT NULL UNIQUE,
    appypay_charge_id TEXT UNIQUE,
    metodo VARCHAR(8) NOT NULL CHECK (metodo IN ('GPO', 'REF')),
    status TEXT,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    response JSONB NOT NULL DEFAULT '{}'::jsonb,
    version INTEGER NOT NULL DEFAULT 1,
    last_event_id UUID,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE financeiro_webhooks_recebidos (
    event_key TEXT PRIMARY KEY,
    metodo VARCHAR(8) NOT NULL CHECK (metodo IN ('GPO', 'REF')),
    credential_id UUID,
    cobranca_id UUID,
    payload JSONB NOT NULL,
    received_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_financeiro_credenciais_contexto ON financeiro_credenciais_appypay(contexto_tipo, codigo_academia, ambiente);
CREATE INDEX idx_financeiro_cobrancas_contexto ON financeiro_cobrancas(contexto_tipo, codigo_academia);
