BEGIN;

CREATE TABLE IF NOT EXISTS projection_remetentes_comunicacao (
    id UUID PRIMARY KEY,
    provedor VARCHAR(10) NOT NULL,
    identificador VARCHAR(100) NOT NULL,
    token_api_cifrado TEXT NOT NULL,
    configurado_por UUID NOT NULL,
    configurado_por_tipo VARCHAR(10) NOT NULL DEFAULT 'admin',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    version INTEGER NOT NULL DEFAULT 0,
    last_event_id UUID,
    CONSTRAINT chk_remetentes_comunicacao_provedor CHECK (provedor IN ('GOSMS', 'ZIETT')),
    CONSTRAINT chk_remetentes_comunicacao_por_tipo CHECK (configurado_por_tipo = 'admin'),
    CONSTRAINT uq_remetentes_comunicacao_provedor UNIQUE (provedor)
);

COMMIT;
