BEGIN;

CREATE TABLE IF NOT EXISTS projection_mensagens_comunicacao (
    id UUID PRIMARY KEY,
    destinatario VARCHAR(20) NOT NULL,
    conteudo TEXT NOT NULL,
    provedor_tentado_1 VARCHAR(10) NOT NULL,
    provedor_tentado_2 VARCHAR(10),
    provedor_utilizado VARCHAR(10),
    status VARCHAR(20) NOT NULL,
    mensagem_externa_id VARCHAR(200),
    detalhes_tentativas JSONB NOT NULL,
    enviado_por UUID NOT NULL,
    enviado_por_tipo VARCHAR(10) NOT NULL,
    codigo_academia VARCHAR(50),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    version INTEGER NOT NULL DEFAULT 0,
    last_event_id UUID,
    CONSTRAINT chk_mensagens_comunicacao_provedor_1 CHECK (provedor_tentado_1 IN ('GOSMS', 'ZIETT')),
    CONSTRAINT chk_mensagens_comunicacao_provedor_2 CHECK (provedor_tentado_2 IS NULL OR provedor_tentado_2 IN ('GOSMS', 'ZIETT')),
    CONSTRAINT chk_mensagens_comunicacao_provedor_utilizado CHECK (provedor_utilizado IS NULL OR provedor_utilizado IN ('GOSMS', 'ZIETT')),
    CONSTRAINT chk_mensagens_comunicacao_status CHECK (status IN ('enviada', 'falhou')),
    CONSTRAINT chk_mensagens_comunicacao_enviado_por_tipo CHECK (enviado_por_tipo IN ('admin', 'academia')),
    CONSTRAINT chk_mensagens_comunicacao_codigo_academia CHECK (
        (enviado_por_tipo = 'academia' AND codigo_academia IS NOT NULL) OR
        (enviado_por_tipo = 'admin' AND codigo_academia IS NULL)
    )
);

CREATE INDEX IF NOT EXISTS idx_mensagens_comunicacao_created_at ON projection_mensagens_comunicacao(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_mensagens_comunicacao_codigo_academia ON projection_mensagens_comunicacao(codigo_academia);
CREATE INDEX IF NOT EXISTS idx_mensagens_comunicacao_enviado_por ON projection_mensagens_comunicacao(enviado_por);

COMMIT;
