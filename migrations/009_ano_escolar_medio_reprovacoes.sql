-- ============================================================================
-- MIGRATION 009 - Corrigir ano_escolar_medio + Tabela de reprovações dedicada
-- ============================================================================

BEGIN;

-- ============================================================================
-- 1. Adicionar ano_escolar_medio à projeção de estudantes
--
-- MOTIVO: O campo ano_escolar era (incorretamente) usado para fundamental E
--   médio ao mesmo tempo. Agora são campos distintos:
--     ano_escolar       → ano atual no ciclo fundamental
--     ano_escolar_medio → ano atual no ciclo médio
-- ============================================================================

ALTER TABLE projection_estudantes
    ADD COLUMN IF NOT EXISTS ano_escolar_medio VARCHAR(100);

COMMENT ON COLUMN projection_estudantes.ano_escolar IS
    'Ano atual no ciclo fundamental (definido pela academia)';
COMMENT ON COLUMN projection_estudantes.ano_escolar_medio IS
    'Ano atual no ciclo médio (definido pela academia dentro do curso)';

-- ============================================================================
-- 2. Criar tabela dedicada de reprovações (log explícito)
--
-- MOTIVO: O requisito pede registro explícito de reprovações.
--   projection_aprovacao_ano já armazena reprovações (aprovado=false),
--   mas uma tabela dedicada facilita queries, auditorias e relatórios
--   sem depender de filtro na tabela unificada.
--
--   IMPORTANTE: Não remove projection_aprovacao_ano — ambas coexistem.
--   A projeção de aprovação continua sendo o log completo (aprovados + reprovados).
--   projection_reprovacoes é uma view materializada explícita das reprovações.
-- ============================================================================

CREATE TABLE IF NOT EXISTS projection_reprovacoes (
    id               UUID         PRIMARY KEY DEFAULT uuid_generate_v4(),

    -- Referência ao evento de origem (rastreabilidade event sourcing)
    event_id         UUID         NOT NULL,

    -- Identificadores
    codigo_estudante VARCHAR(7)   NOT NULL,
    codigo_academia  VARCHAR(50)  NOT NULL,

    -- Dados do ano reprovado
    ano_lectivo      VARCHAR(20)  NOT NULL,
    tipo_ensino      VARCHAR(20)  NOT NULL
        CHECK (tipo_ensino IN ('fundamental', 'medio', 'superior')),
    nivel_reprovado  VARCHAR(100) NOT NULL,

    -- Justificativa (opcional, preenchida pela academia)
    observacao       TEXT,

    -- Metadados
    registered_at    TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    version          INTEGER      NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_reprovacoes_estudante   ON projection_reprovacoes(codigo_estudante);
CREATE INDEX IF NOT EXISTS idx_reprovacoes_academia    ON projection_reprovacoes(codigo_academia);
CREATE INDEX IF NOT EXISTS idx_reprovacoes_tipo_ensino ON projection_reprovacoes(tipo_ensino);
CREATE INDEX IF NOT EXISTS idx_reprovacoes_ano         ON projection_reprovacoes(ano_lectivo);

COMMENT ON TABLE projection_reprovacoes IS
    'Log explícito de reprovações. Complementa projection_aprovacao_ano (aprovado=false).';

-- Checkpoint para a nova projeção
INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at)
VALUES ('reprovacoes', 0, CURRENT_TIMESTAMP)
ON CONFLICT (projection_name) DO NOTHING;

-- ============================================================================
-- 3. Atualizar view v_estudantes_com_cursos para incluir ano_escolar_medio
-- ============================================================================

DROP VIEW IF EXISTS v_estudantes_com_cursos;
CREATE OR REPLACE VIEW v_estudantes_com_cursos AS
SELECT
    e.id,
    e.nome,
    e.codigo_estudante,
    e.email,
    e.genero,
    e.codigo_academia,
    e.status,
    e.status_escolar_fundamental,
    e.status_escolar_medio,
    e.status_superior,
    e.ano_escolar,
    e.ano_escolar_medio,
    e.ano_superior,
    cm.id   AS curso_medio_id,
    cm.nome AS curso_medio_nome,
    cs.id   AS curso_superior_id,
    cs.nome AS curso_superior_nome,
    e.created_at,
    e.updated_at
FROM projection_estudantes e
LEFT JOIN projection_cursos cm ON e.curso_medio_id   = cm.id
LEFT JOIN projection_cursos cs ON e.curso_superior_id = cs.id;

-- ============================================================================
-- 4. View de reprovações completas
-- ============================================================================

CREATE OR REPLACE VIEW v_reprovacoes_completas AS
SELECT
    r.id,
    r.codigo_estudante,
    e.nome                AS estudante_nome,
    r.codigo_academia,
    ac.nome               AS academia_nome,
    r.ano_lectivo,
    r.tipo_ensino,
    r.nivel_reprovado,
    r.observacao,
    r.registered_at
FROM projection_reprovacoes r
LEFT JOIN projection_estudantes e  ON r.codigo_estudante = e.codigo_estudante
LEFT JOIN projection_academias  ac ON r.codigo_academia  = ac.codigo_academia;

-- ============================================================================
-- 5. Adicionar projection_reprovacoes à whitelist de ValidateTableName
--    (ajustar em internal/db/safe_queries.go)
-- ============================================================================
-- INSTRUÇÃO: Em internal/db/safe_queries.go, adicionar à validTables:
--   "projection_reprovacoes": true,

COMMIT;

DO $$ BEGIN RAISE NOTICE '✅ MIGRATION 009 - ano_escolar_medio + projection_reprovacoes criados'; END $$;
