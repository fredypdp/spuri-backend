CREATE TABLE IF NOT EXISTS projection_solicitacoes_status_academico (
    id UUID PRIMARY KEY,
    codigo_solicitacao VARCHAR(32) UNIQUE NOT NULL,
    codigo_estudante VARCHAR(20) NOT NULL,
    codigo_academia VARCHAR(20) NOT NULL,
    tipo VARCHAR(20) NOT NULL CHECK (tipo IN ('interrupcao', 'desvinculacao', 'revinculacao')),
    status VARCHAR(20) NOT NULL CHECK (status IN ('pendente', 'aprovada', 'reprovada')),
    motivo TEXT NOT NULL CHECK (btrim(motivo) <> ''),
    tipo_ensino VARCHAR(20) CHECK (tipo_ensino IS NULL OR tipo_ensino IN ('fundamental', 'medio', 'superior', '')),
    curso_medio_id UUID,
    curso_superior_id UUID,
    observacao_academia TEXT,
    motivo_reprovacao TEXT,
    solicitada_por UUID NOT NULL,
    decidida_por UUID,
    decidida_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_solicitacoes_status_academico_pendente
    ON projection_solicitacoes_status_academico (codigo_estudante, codigo_academia, tipo)
    WHERE status = 'pendente';
CREATE INDEX IF NOT EXISTS idx_solicitacoes_status_academico_academia_status
    ON projection_solicitacoes_status_academico (codigo_academia, status);
