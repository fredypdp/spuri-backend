-- Módulo financeiro/AppyPay: read models reconstruíveis e cofre separado.
-- Nenhum segredo ou ciphertext é mantido nos payloads das projeções.
CREATE TABLE IF NOT EXISTS financeiro_credenciais_appypay (
    id UUID PRIMARY KEY,
    payload JSONB NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
ALTER TABLE financeiro_credenciais_appypay
    ADD COLUMN IF NOT EXISTS contexto_tipo VARCHAR(16),
    ADD COLUMN IF NOT EXISTS codigo_academia TEXT,
    ADD COLUMN IF NOT EXISTS ambiente VARCHAR(8);
UPDATE financeiro_credenciais_appypay
   SET contexto_tipo = COALESCE(NULLIF(contexto_tipo, ''), NULLIF(payload->>'contexto_tipo', ''), NULLIF(payload->>'ContextoTipo', ''), 'spuri'),
       codigo_academia = COALESCE(NULLIF(codigo_academia, ''), NULLIF(payload->>'codigo_academia', ''), NULLIF(payload->>'CodigoAcademia', '')),
       ambiente = COALESCE(NULLIF(ambiente, ''), NULLIF(payload->>'ambiente', ''), NULLIF(payload->>'Ambiente', ''), 'test');
ALTER TABLE financeiro_credenciais_appypay
    ALTER COLUMN contexto_tipo SET NOT NULL,
    ALTER COLUMN ambiente SET NOT NULL,
    ADD CONSTRAINT financeiro_credenciais_appypay_contexto_tipo_check CHECK (contexto_tipo IN ('spuri','academia')),
    ADD CONSTRAINT financeiro_credenciais_appypay_ambiente_check CHECK (ambiente IN ('test','prod')),
    ADD CONSTRAINT financeiro_credenciais_appypay_contexto_codigo_check CHECK ((contexto_tipo = 'spuri' AND codigo_academia IS NULL) OR (contexto_tipo = 'academia' AND codigo_academia IS NOT NULL));
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
    payload JSONB NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
ALTER TABLE financeiro_cobrancas
    ADD COLUMN IF NOT EXISTS provider_charge_id TEXT,
    ADD COLUMN IF NOT EXISTS merchant_transaction_id VARCHAR(15),
    ADD COLUMN IF NOT EXISTS contexto_tipo VARCHAR(16),
    ADD COLUMN IF NOT EXISTS codigo_academia TEXT;
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
          FROM information_schema.columns
         WHERE table_schema = 'public'
           AND table_name = 'financeiro_cobrancas'
           AND column_name = 'idempotency_key'
    ) THEN
        UPDATE financeiro_cobrancas
           SET provider_charge_id = COALESCE(NULLIF(provider_charge_id, ''), NULLIF(payload->>'provider_charge_id', ''), NULLIF(payload->>'ProviderChargeID', '')),
               merchant_transaction_id = COALESCE(NULLIF(merchant_transaction_id, ''), NULLIF(payload->>'merchant_transaction_id', ''), NULLIF(payload->>'MerchantTransactionID', ''), NULLIF(payload->>'idempotency_key', ''), idempotency_key),
               contexto_tipo = COALESCE(NULLIF(contexto_tipo, ''), NULLIF(payload->>'contexto_tipo', ''), NULLIF(payload->>'ContextoTipo', ''), 'spuri'),
               codigo_academia = COALESCE(NULLIF(codigo_academia, ''), NULLIF(payload->>'codigo_academia', ''), NULLIF(payload->>'CodigoAcademia', ''));
    ELSE
        UPDATE financeiro_cobrancas
           SET provider_charge_id = COALESCE(NULLIF(provider_charge_id, ''), NULLIF(payload->>'provider_charge_id', ''), NULLIF(payload->>'ProviderChargeID', '')),
               merchant_transaction_id = COALESCE(NULLIF(merchant_transaction_id, ''), NULLIF(payload->>'merchant_transaction_id', ''), NULLIF(payload->>'MerchantTransactionID', ''), NULLIF(payload->>'idempotency_key', '')),
               contexto_tipo = COALESCE(NULLIF(contexto_tipo, ''), NULLIF(payload->>'contexto_tipo', ''), NULLIF(payload->>'ContextoTipo', ''), 'spuri'),
               codigo_academia = COALESCE(NULLIF(codigo_academia, ''), NULLIF(payload->>'codigo_academia', ''), NULLIF(payload->>'CodigoAcademia', ''));
    END IF;
END $$;
ALTER TABLE financeiro_cobrancas
    ALTER COLUMN merchant_transaction_id SET NOT NULL,
    ALTER COLUMN contexto_tipo SET NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS ux_financeiro_cobrancas_merchant_transaction_id
 ON financeiro_cobrancas(merchant_transaction_id);
CREATE UNIQUE INDEX IF NOT EXISTS ux_financeiro_cobrancas_provider_id
 ON financeiro_cobrancas(provider_charge_id) WHERE provider_charge_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS financeiro_webhooks_recebidos (
    event_id TEXT PRIMARY KEY,
    received_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
ALTER TABLE financeiro_webhooks_recebidos
    ADD COLUMN IF NOT EXISTS metodo VARCHAR(8);
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
          FROM information_schema.columns
         WHERE table_schema = 'public'
           AND table_name = 'financeiro_webhooks_recebidos'
           AND column_name = 'payload'
    ) THEN
        UPDATE financeiro_webhooks_recebidos
           SET metodo = COALESCE(NULLIF(metodo, ''), NULLIF(payload->>'metodo', ''), NULLIF(payload->>'Metodo', ''), 'GPO');
    END IF;
END $$;
UPDATE financeiro_webhooks_recebidos
   SET metodo = COALESCE(NULLIF(metodo, ''), 'GPO');
ALTER TABLE financeiro_webhooks_recebidos
    ALTER COLUMN metodo SET NOT NULL,
    ADD CONSTRAINT financeiro_webhooks_recebidos_metodo_check CHECK (metodo IN ('GPO','REF'));

COMMENT ON TABLE financeiro_segredos_appypay IS 'Cofre operacional AES-256-GCM. Nunca é reconstruído do ledger.';
