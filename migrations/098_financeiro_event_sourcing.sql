-- Financeiro Event Sourcing/CQRS: ledger é a fonte de verdade; tabelas financeiro_* são projeções.
CREATE TABLE IF NOT EXISTS financeiro_segredos_appypay (
    credential_id UUID NOT NULL,
    secret_version INTEGER NOT NULL DEFAULT 1,
    secret_type VARCHAR(64) NOT NULL,
    application_id VARCHAR(128),
    ciphertext TEXT NOT NULL,
    key_id VARCHAR(128) NOT NULL DEFAULT 'FINANCE_ENCRYPTION_KEY',
    algorithm VARCHAR(64) NOT NULL DEFAULT 'AES-256-GCM',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    rotated_at TIMESTAMP,
    revoked_at TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_financeiro_segredos_appypay_version
    ON financeiro_segredos_appypay (credential_id, secret_type, COALESCE(application_id, ''), secret_version);

COMMENT ON TABLE financeiro_credenciais_appypay IS 'Projeção/read model de credenciais AppyPay. O histórico primário é o spuri_ledger (aggregate_type=Financeiro).';
COMMENT ON TABLE financeiro_cobrancas IS 'Projeção/read model de cobranças financeiras e índice de idempotência reconstruível a partir do spuri_ledger.';
COMMENT ON TABLE financeiro_webhooks_recebidos IS 'Projeção/índice operacional de idempotência de webhooks reconstruível por replay dos eventos financeiros.';
COMMENT ON TABLE financeiro_modalidade_pagamento IS 'Projeção singleton da modalidade de pagamento ativa, reconstruível por replay do spuri_ledger.';
COMMENT ON TABLE financeiro_segredos_appypay IS 'Armazenamento operacional de segredos cifrados AppyPay; segredos em claro nunca são gravados no ledger.';
