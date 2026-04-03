BEGIN;

CREATE TABLE IF NOT EXISTS async_jobs (
    id           UUID         PRIMARY KEY,
    type         VARCHAR(64)  NOT NULL,
    status       VARCHAR(20)  NOT NULL DEFAULT 'pending'
                              CHECK (status IN ('pending','processing','done','failed')),
    user_id      UUID         NOT NULL,
    user_type    VARCHAR(20)  NOT NULL,
    payload      JSONB        NOT NULL,
    results      JSONB        NOT NULL DEFAULT '[]',
    total_items  INT          NOT NULL DEFAULT 0,
    done_items   INT          NOT NULL DEFAULT 0,
    fail_items   INT          NOT NULL DEFAULT 0,
    error        TEXT,
    created_at   TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at   TIMESTAMP,
    completed_at TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_async_jobs_user_id    ON async_jobs (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_async_jobs_status     ON async_jobs (status) WHERE status IN ('pending','processing');
CREATE INDEX IF NOT EXISTS idx_async_jobs_cleanup    ON async_jobs (completed_at) WHERE status IN ('done','failed');

COMMENT ON TABLE async_jobs IS
    'Jobs assíncronos de operações em lote. Resultados parciais são gravados durante o processamento.';

COMMIT;