CREATE TABLE IF NOT EXISTS projection_solicitacoes_edicao_dados_estudante (
    id UUID PRIMARY KEY,
    codigo_solicitacao VARCHAR(32) UNIQUE NOT NULL,
    codigo_estudante VARCHAR(20) NOT NULL,
    codigo_academia VARCHAR(20) NOT NULL,
    campo VARCHAR(64) NOT NULL CHECK (campo IN ('nome','bilhete_identidade','bilhete_identidade_encarregado','data_nascimento')),
    valor_atual TEXT NOT NULL,
    valor_solicitado TEXT NOT NULL,
    documento_temporario_path TEXT NOT NULL,
    documento_temporario_url TEXT,
    status VARCHAR(20) NOT NULL CHECK (status IN ('pendente','aprovada','reprovada')),
    motivo_reprovacao TEXT,
    solicitado_por VARCHAR(20) NOT NULL,
    decidido_por TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    version INTEGER NOT NULL DEFAULT 1,
    last_event_id BIGINT
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_solicitacoes_edicao_dados_estudante_pendente
    ON projection_solicitacoes_edicao_dados_estudante (codigo_estudante, campo)
    WHERE status = 'pendente';
CREATE INDEX IF NOT EXISTS idx_solicitacoes_edicao_dados_estudante_academia_status
    ON projection_solicitacoes_edicao_dados_estudante (codigo_academia, status);
CREATE INDEX IF NOT EXISTS idx_solicitacoes_edicao_dados_estudante_estudante_status
    ON projection_solicitacoes_edicao_dados_estudante (codigo_estudante, status);
