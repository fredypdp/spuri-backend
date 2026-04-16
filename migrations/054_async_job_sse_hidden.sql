CREATE TABLE IF NOT EXISTS async_job_sse_hidden (
    user_id    UUID        NOT NULL,
    job_id     UUID        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, job_id),
    CONSTRAINT fk_async_job_sse_hidden_job
        FOREIGN KEY (job_id) REFERENCES async_jobs(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_async_job_sse_hidden_user_id
    ON async_job_sse_hidden (user_id);
