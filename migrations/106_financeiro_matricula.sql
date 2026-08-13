-- Fase 4: configuração e ciclo de cobrança de matrícula por solicitação.
CREATE TABLE IF NOT EXISTS financeiro_matricula_configuracoes (
    event_id UUID PRIMARY KEY,
    aggregate_id UUID NOT NULL,
    codigo_academia TEXT NOT NULL,
    nivel TEXT NOT NULL CHECK (nivel IN ('fundamental', 'medio', 'superior')),
    ano_academico TEXT NOT NULL,
    curso_id UUID NULL,
    valor NUMERIC(14,2) NOT NULL CHECK (valor > 0),
    metodos_pagamento TEXT[] NOT NULL,
    vigente_em TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_fin_matricula_configuracoes_resolucao
    ON financeiro_matricula_configuracoes (codigo_academia, nivel, ano_academico, curso_id, vigente_em DESC);

ALTER TABLE projection_solicitacoes_matricula
    ADD COLUMN IF NOT EXISTS valor_matricula NUMERIC(14,2) NULL,
    ADD COLUMN IF NOT EXISTS metodos_pagamento_matricula TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[];

ALTER TABLE projection_solicitacoes_matricula
    DROP CONSTRAINT IF EXISTS projection_solicitacoes_matricula_status_check;
ALTER TABLE projection_solicitacoes_matricula
    ADD CONSTRAINT projection_solicitacoes_matricula_status_check
    CHECK (status IN ('pendente', 'aprovada', 'reprovada', 'cancelada', 'aprovada_pendente_pagamento_matricula'));

CREATE INDEX IF NOT EXISTS idx_solicitacoes_matricula_busca_publica
    ON projection_solicitacoes_matricula (telefone, telefone_encarregado, email, bilhete_identidade, bilhete_identidade_encarregado);
