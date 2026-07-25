CREATE TABLE IF NOT EXISTS unique_operation_guards (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    scope TEXT NOT NULL,
    key TEXT NOT NULL,
    aggregate_type TEXT,
    aggregate_id UUID,
    user_id TEXT,
    user_type TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL DEFAULT 'reserved',
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    consumed_at TIMESTAMPTZ,
    released_at TIMESTAMPTZ,
    CONSTRAINT unique_operation_guards_status_check CHECK (status IN ('reserved','consumed','released','expired'))
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_unique_operation_guards_active
    ON unique_operation_guards (scope, key)
    WHERE status IN ('reserved','consumed');

CREATE INDEX IF NOT EXISTS idx_unique_operation_guards_expires_at
    ON unique_operation_guards (expires_at)
    WHERE status = 'reserved';
