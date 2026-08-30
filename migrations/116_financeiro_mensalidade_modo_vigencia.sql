BEGIN;

ALTER TABLE financeiro_mensalidade_configuracoes
    ADD COLUMN sequencia BIGINT,
    ADD COLUMN modo_vigencia TEXT NOT NULL DEFAULT 'a_partir_da_atualizacao'
        CHECK (modo_vigencia IN ('a_partir_da_atualizacao', 'cobrancas_pendentes'));

UPDATE financeiro_mensalidade_configuracoes c
SET sequencia = l.id
FROM spuri_ledger l
WHERE l.event_id = c.event_id;

ALTER TABLE financeiro_mensalidade_configuracoes
    ALTER COLUMN sequencia SET NOT NULL;

CREATE INDEX idx_fin_mensalidade_config_sequencia
    ON financeiro_mensalidade_configuracoes (codigo_academia, nivel, ano_academico, curso_id, sequencia DESC);

DROP VIEW financeiro_mensalidade_configuracoes_atual;
CREATE VIEW financeiro_mensalidade_configuracoes_atual AS
SELECT c.codigo_academia, c.nivel, c.ano_academico, c.curso_id, c.valor,
       c.mes_fim_cobranca, c.metodos_pagamento, c.vigente_em, c.modo_vigencia
FROM (
    SELECT DISTINCT ON (codigo_academia, nivel, ano_academico, curso_id)
        codigo_academia, nivel, ano_academico, curso_id, valor,
        mes_fim_cobranca, metodos_pagamento, vigente_em, modo_vigencia, sequencia
    FROM financeiro_mensalidade_configuracoes
    ORDER BY codigo_academia, nivel, ano_academico, curso_id, sequencia DESC
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

COMMIT;

COMMENT ON COLUMN financeiro_mensalidade_configuracoes.sequencia IS
    'Ordem real de criação (copiada de spuri_ledger.id no momento da projeção) — event_id é um UUID aleatório e não serve para isso. Usada por Service.ultimaConfiguracaoMensalidade (mensalidade_vigencia.go) para decidir qual foi a versão mais recente, independente de vigente_em.';
COMMENT ON COLUMN financeiro_mensalidade_configuracoes.modo_vigencia IS
    'Escolha feita pelo chamador em ConfigureMensalidade: a_partir_da_atualizacao (default, comportamento histórico) ou cobrancas_pendentes (a versão mais recente com este modo passa a valer para qualquer obrigação ainda pendente, de qualquer mês — ver resolveConfiguracaoEfetiva).';
