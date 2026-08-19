-- Fase 5 do financeiro: remoção de configurações já definidas, respeitando
-- event sourcing. Nenhuma tabela de fatos existente é alterada ou perde
-- linhas; cada remoção é um NOVO fato imutável em uma tabela dedicada,
-- nunca um DELETE/UPDATE sobre o histórico já gravado no ledger.
--
-- Padrão adotado (consistente com o restante do módulo financeiro):
--   1. financeiro_<recurso>_remocoes guarda o fato "removido em X".
--   2. financeiro_<recurso>_atual é uma VIEW que resolve o estado vigente
--      combinando a versão mais recente com a remoção mais recente,
--      sem nunca apagar linha nenhuma das tabelas de origem.
--   3. Consultas que hoje leem "a config atual" (não uma data histórica)
--      passam a ler da VIEW; consultas que resolvem PREÇO HISTÓRICO por
--      data continuam lendo a tabela de origem e comparam o timestamp da
--      remoção com o vigente_em da versão resolvida (implementado em Go,
--      não nesta migration), preservando 100% do histórico já cobrado.

BEGIN;

-- 1) Credenciais AppyPay: recurso "estado atual" simples (não é versionado
-- por data), então a remoção apaga a linha da projeção (a mesma tabela já é
-- inteiramente reconstruída a partir do ledger em Rebuild()). O cofre de
-- segredos (financeiro_segredos_appypay) é limpo pelo Service no mesmo
-- comando, fora do replay do ledger, exatamente como ConfigureCredential já
-- faz para gravar segredos.
-- (Nenhuma alteração de schema é necessária aqui: a tabela já existe.)

-- 2) Configuração de mensalidade (propina)
CREATE TABLE financeiro_mensalidade_configuracoes_remocoes (
    event_id UUID PRIMARY KEY,
    aggregate_id UUID NOT NULL,
    codigo_academia TEXT NOT NULL,
    nivel VARCHAR(16) NOT NULL CHECK (nivel IN ('fundamental', 'medio', 'superior')),
    ano_academico TEXT NOT NULL,
    curso_id UUID NULL,
    removido_em TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_fin_mensalidade_config_remocoes_lookup
    ON financeiro_mensalidade_configuracoes_remocoes
       (codigo_academia, nivel, ano_academico, curso_id, removido_em DESC);

CREATE VIEW financeiro_mensalidade_configuracoes_atual AS
SELECT c.codigo_academia, c.nivel, c.ano_academico, c.curso_id, c.valor,
       c.mes_fim_cobranca, c.metodos_pagamento, c.vigente_em
FROM (
    SELECT DISTINCT ON (codigo_academia, nivel, ano_academico, curso_id)
        codigo_academia, nivel, ano_academico, curso_id, valor,
        mes_fim_cobranca, metodos_pagamento, vigente_em
    FROM financeiro_mensalidade_configuracoes
    ORDER BY codigo_academia, nivel, ano_academico, curso_id, vigente_em DESC, event_id DESC
) c
LEFT JOIN LATERAL (
    SELECT removido_em FROM financeiro_mensalidade_configuracoes_remocoes r
    WHERE r.codigo_academia = c.codigo_academia AND r.nivel = c.nivel
      AND r.ano_academico = c.ano_academico
      AND r.curso_id IS NOT DISTINCT FROM c.curso_id
      AND r.removido_em >= c.vigente_em
    ORDER BY r.removido_em DESC LIMIT 1
) rm ON true
WHERE rm.removido_em IS NULL;

-- 3) Mês de início de cobrança
CREATE TABLE financeiro_mensalidade_inicio_cobranca_remocoes (
    event_id UUID PRIMARY KEY,
    aggregate_id UUID NOT NULL,
    codigo_academia TEXT NOT NULL,
    ano_letivo VARCHAR(9) NOT NULL,
    removido_em TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_fin_mensalidade_inicio_remocoes_lookup
    ON financeiro_mensalidade_inicio_cobranca_remocoes (codigo_academia, ano_letivo, removido_em DESC);

CREATE VIEW financeiro_mensalidade_inicio_cobranca_atual AS
SELECT c.codigo_academia, c.ano_letivo, c.mes_inicio, c.definido_em
FROM (
    SELECT DISTINCT ON (codigo_academia, ano_letivo)
        codigo_academia, ano_letivo, mes_inicio, definido_em
    FROM financeiro_mensalidade_inicio_cobranca
    ORDER BY codigo_academia, ano_letivo, definido_em DESC, event_id DESC
) c
LEFT JOIN LATERAL (
    SELECT removido_em FROM financeiro_mensalidade_inicio_cobranca_remocoes r
    WHERE r.codigo_academia = c.codigo_academia AND r.ano_letivo = c.ano_letivo
      AND r.removido_em >= c.definido_em
    ORDER BY r.removido_em DESC LIMIT 1
) rm ON true
WHERE rm.removido_em IS NULL;

-- 4) Configuração de matrícula
CREATE TABLE financeiro_matricula_configuracoes_remocoes (
    event_id UUID PRIMARY KEY,
    aggregate_id UUID NOT NULL,
    codigo_academia TEXT NOT NULL,
    nivel TEXT NOT NULL CHECK (nivel IN ('fundamental', 'medio', 'superior')),
    ano_academico TEXT NOT NULL,
    curso_id UUID NULL,
    removido_em TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_fin_matricula_config_remocoes_lookup
    ON financeiro_matricula_configuracoes_remocoes
       (codigo_academia, nivel, ano_academico, curso_id, removido_em DESC);

CREATE VIEW financeiro_matricula_configuracoes_atual AS
SELECT c.codigo_academia, c.nivel, c.ano_academico, c.curso_id, c.valor,
       c.metodos_pagamento, c.vigente_em
FROM (
    SELECT DISTINCT ON (codigo_academia, nivel, ano_academico, curso_id)
        codigo_academia, nivel, ano_academico, curso_id, valor,
        metodos_pagamento, vigente_em
    FROM financeiro_matricula_configuracoes
    ORDER BY codigo_academia, nivel, ano_academico, curso_id, vigente_em DESC, event_id DESC
) c
LEFT JOIN LATERAL (
    SELECT removido_em FROM financeiro_matricula_configuracoes_remocoes r
    WHERE r.codigo_academia = c.codigo_academia AND r.nivel = c.nivel
      AND r.ano_academico = c.ano_academico
      AND r.curso_id IS NOT DISTINCT FROM c.curso_id
      AND r.removido_em >= c.vigente_em
    ORDER BY r.removido_em DESC LIMIT 1
) rm ON true
WHERE rm.removido_em IS NULL;

COMMIT;

COMMENT ON TABLE financeiro_mensalidade_configuracoes_remocoes IS
    'Fatos imutáveis de MensalidadeConfiguracaoRemovida. Nunca apaga linhas de financeiro_mensalidade_configuracoes; apenas registra a partir de quando o escopo deixou de ter configuração ativa.';
COMMENT ON VIEW financeiro_mensalidade_configuracoes_atual IS
    'Configuração vigente por escopo (ou nenhuma linha, se a última remoção for igual/posterior à última versão). Não usar para preço histórico por data — ver Service.resolveConfiguracao em Go.';
COMMENT ON TABLE financeiro_mensalidade_inicio_cobranca_remocoes IS
    'Fatos imutáveis de MesInicioCobrancaRemovido. A ausência de linha vigente em financeiro_mensalidade_inicio_cobranca_atual faz o backend usar o mês natural padrão do ano letivo.';
COMMENT ON TABLE financeiro_matricula_configuracoes_remocoes IS
    'Fatos imutáveis de MatriculaConfiguracaoRemovida. Nunca apaga linhas de financeiro_matricula_configuracoes.';
