BEGIN;

CREATE TABLE IF NOT EXISTS projection_categorias_servico (
    id UUID PRIMARY KEY,
    codigo_academia VARCHAR(50) NOT NULL,
    nome VARCHAR(100) NOT NULL,
    ativo BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    version INTEGER NOT NULL DEFAULT 0,
    last_event_id UUID
);
CREATE INDEX IF NOT EXISTS idx_categorias_servico_academia ON projection_categorias_servico(codigo_academia, ativo);
CREATE UNIQUE INDEX IF NOT EXISTS ux_categorias_servico_nome_ativo
    ON projection_categorias_servico (codigo_academia, lower(nome))
    WHERE ativo = true;

ALTER TABLE projection_servicos_extras ADD COLUMN categoria_servico_id UUID NULL;
ALTER TABLE projection_servicos_extras DROP COLUMN categoria;

COMMIT;
