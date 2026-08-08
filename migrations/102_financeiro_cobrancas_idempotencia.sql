-- Reserve merchantTransactionId before appending an event. The reservation
-- prevents concurrent retries from creating orphan ledger aggregates.
CREATE TABLE IF NOT EXISTS financeiro_cobrancas_reservas (
    merchant_transaction_id VARCHAR(15) PRIMARY KEY,
    charge_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
