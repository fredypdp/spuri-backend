BEGIN;

CREATE TABLE IF NOT EXISTS projection_documentos_extra (
    id UUID PRIMARY KEY,
    codigo_academia VARCHAR(50) NOT NULL,
    rotulo VARCHAR(150) NOT NULL,
    tipo VARCHAR(10) NOT NULL CHECK (tipo IN ('pdf', 'jpg')),
    obrigatorio BOOLEAN NOT NULL DEFAULT false,
    nivel VARCHAR(20) NOT NULL CHECK (nivel IN ('fundamental', 'medio', 'superior')),
    ano_academico VARCHAR(30) NOT NULL,
    ativo BOOLEAN NOT NULL DEFAULT true,
    criado_por UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    version INTEGER NOT NULL DEFAULT 0,
    last_event_id UUID
);

CREATE INDEX IF NOT EXISTS idx_documentos_extra_academia ON projection_documentos_extra(codigo_academia, ativo);

-- Uma academia não pode ter dois documentos extra ativos com o mesmo rótulo
-- para o mesmo ano acadêmico (mesma proteção usada em categorias de serviço,
-- ux_categorias_servico_nome_ativo, para os mesmos motivos: evitar ambiguidade
-- de nomes visíveis ao estudante/encarregado no momento do upload).
CREATE UNIQUE INDEX IF NOT EXISTS ux_documentos_extra_rotulo_ano_ativo
    ON projection_documentos_extra (codigo_academia, ano_academico, lower(rotulo))
    WHERE ativo = true;

COMMENT ON TABLE projection_documentos_extra IS
    'Catálogo de documentos adicionais que cada academia pode exigir (ou apenas oferecer) no cadastro direto e na solicitação de matrícula do estudante, além dos documentos fixos do sistema (BI, cédula, declaração, certificados). Cada linha é uma definição reutilizada por todos os estudantes daquele ano_academico; o arquivo enviado por um estudante específico fica em projection_estudantes.documentos, na chave documento_extra.<id>.';
COMMENT ON COLUMN projection_documentos_extra.tipo IS
    'Formato de arquivo exigido para este documento: pdf ou jpg. Reaproveita o mesmo limite de tamanho MaxPDFUploadBytes (10MB) já usado pelos documentos fixos.';
COMMENT ON COLUMN projection_documentos_extra.nivel IS
    'Nível acadêmico do ano informado em ano_academico (derivado no backend, nunca aceito diretamente do payload) — fundamental | medio | superior.';
COMMENT ON COLUMN projection_documentos_extra.ano_academico IS
    'Ano acadêmico canônico ao qual esta exigência se aplica (ex.: 6_ano_fundamental, 2_ano_medio, 1_ano_superior) — mesmo domínio validado por utils.ValidateAnoFundamental/ValidateAnoMedio/ValidateAnoSuperior. Uma exigência vale para um único ano; para cobrir vários anos, a academia cria uma definição por ano.';

COMMIT;
