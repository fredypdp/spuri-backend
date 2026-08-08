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
-- Instalações que executaram uma versão anterior do módulo podem manter as
-- tabelas 097/098 mesmo tendo a migration 099 marcada como aplicada. CREATE
-- TABLE IF NOT EXISTS não altera essas tabelas; portanto compatibilizamos o
-- esquema antes de criar os índices novos.
ALTER TABLE financeiro_credenciais_appypay
    ADD COLUMN IF NOT EXISTS contexto_tipo VARCHAR(16),
    ADD COLUMN IF NOT EXISTS codigo_academia TEXT,
    ADD COLUMN IF NOT EXISTS ambiente VARCHAR(8);
UPDATE financeiro_credenciais_appypay
SET contexto_tipo = COALESCE(NULLIF(payload->>'contexto_tipo', ''), NULLIF(payload->>'ContextoTipo', ''), 'spuri'),
    codigo_academia = COALESCE(NULLIF(payload->>'codigo_academia', ''), NULLIF(payload->>'CodigoAcademia', '')),
    ambiente = COALESCE(NULLIF(payload->>'ambiente', ''), NULLIF(payload->>'Ambiente', ''), 'test')
WHERE contexto_tipo IS NULL OR ambiente IS NULL;
CREATE INDEX IF NOT EXISTS idx_financeiro_credenciais_contexto_v2
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
ALTER TABLE financeiro_cobrancas
    ADD COLUMN IF NOT EXISTS provider_charge_id TEXT,
    ADD COLUMN IF NOT EXISTS merchant_transaction_id VARCHAR(15),
    ADD COLUMN IF NOT EXISTS contexto_tipo VARCHAR(16),
    ADD COLUMN IF NOT EXISTS codigo_academia TEXT;
UPDATE financeiro_cobrancas
SET provider_charge_id = COALESCE(NULLIF(payload->>'provider_charge_id', ''), NULLIF(payload->>'ProviderChargeID', '')),
    merchant_transaction_id = COALESCE(NULLIF(payload->>'merchant_transaction_id', ''), NULLIF(payload->>'MerchantTransactionID', ''), 'L' || SUBSTRING(REPLACE(id::text, '-', '') FROM 1 FOR 14)),
    contexto_tipo = COALESCE(NULLIF(payload->>'contexto_tipo', ''), NULLIF(payload->>'ContextoTipo', ''), 'spuri'),
    codigo_academia = COALESCE(NULLIF(payload->>'codigo_academia', ''), NULLIF(payload->>'CodigoAcademia', ''))
WHERE merchant_transaction_id IS NULL OR contexto_tipo IS NULL;
CREATE INDEX IF NOT EXISTS idx_financeiro_cobrancas_merchant_id_v2
 ON financeiro_cobrancas(merchant_transaction_id);
CREATE INDEX IF NOT EXISTS idx_financeiro_cobrancas_provider_id_v2
 ON financeiro_cobrancas(provider_charge_id) WHERE provider_charge_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS financeiro_webhooks_recebidos (
    event_id TEXT PRIMARY KEY,
    metodo VARCHAR(8) NOT NULL CHECK (metodo IN ('GPO','REF')),
    received_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
ALTER TABLE financeiro_webhooks_recebidos
    ADD COLUMN IF NOT EXISTS metodo VARCHAR(8);
UPDATE financeiro_webhooks_recebidos SET metodo = 'REF' WHERE metodo IS NULL;

COMMENT ON TABLE financeiro_segredos_appypay IS 'Cofre operacional AES-256-GCM. Nunca é reconstruído do ledger.';
