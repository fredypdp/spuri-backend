ALTER TABLE projection_academias
    ADD COLUMN IF NOT EXISTS documentos_obrigatorios JSONB NOT NULL DEFAULT '{"declaracao": [], "certificado": []}'::jsonb;

CREATE TABLE IF NOT EXISTS projection_solicitacoes_matricula (
    id UUID PRIMARY KEY,
    codigo_solicitacao VARCHAR(11) UNIQUE NOT NULL,
    codigo_academia VARCHAR(50) NOT NULL,
    nome VARCHAR(255) NOT NULL,
    genero VARCHAR(20) NOT NULL CHECK (genero IN ('masculino', 'feminino')),
    data_nascimento DATE NOT NULL,
    email VARCHAR(255),
    telefone VARCHAR(20),
    bilhete_identidade VARCHAR(50),
    bilhete_identidade_responsavel VARCHAR(50),
    ano_escolar_fundamental VARCHAR(50),
    ano_escolar_medio VARCHAR(50),
    curso_medio_id UUID,
    ano_superior VARCHAR(50),
    curso_superior_id UUID,
    status VARCHAR(20) NOT NULL CHECK (status IN ('pendente', 'aprovada', 'reprovada')),
    motivo_reprovacao TEXT,
    documentos JSONB NOT NULL DEFAULT '{}'::jsonb,
    codigo_estudante_gerado VARCHAR(7),
    aprovada_por UUID,
    reprovada_por UUID,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    version INTEGER NOT NULL DEFAULT 0,
    last_event_id UUID
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_solicitacoes_matricula_codigo ON projection_solicitacoes_matricula(codigo_solicitacao);
CREATE INDEX IF NOT EXISTS idx_solicitacoes_matricula_academia ON projection_solicitacoes_matricula(codigo_academia);
CREATE INDEX IF NOT EXISTS idx_solicitacoes_matricula_status ON projection_solicitacoes_matricula(status);
CREATE INDEX IF NOT EXISTS idx_solicitacoes_matricula_created_at ON projection_solicitacoes_matricula(created_at DESC);
