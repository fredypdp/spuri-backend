-- ============================================
-- MIGRATION 008 - Split status_escolar + Aprovação revisada
-- Anos dinâmicos por curso, aprovação manual pela academia
-- ============================================

BEGIN;

-- ============================================
-- 1. Split status_escolar → fundamental + medio
-- ============================================

ALTER TABLE projection_estudantes
    ADD COLUMN IF NOT EXISTS status_escolar_fundamental VARCHAR(20)
        NOT NULL DEFAULT 'inativo'
        CHECK (status_escolar_fundamental IN ('inativo', 'em_andamento', 'finalizado')),
    ADD COLUMN IF NOT EXISTS status_escolar_medio VARCHAR(20)
        NOT NULL DEFAULT 'inativo'
        CHECK (status_escolar_medio IN ('inativo', 'em_andamento', 'finalizado'));

-- Migrar dados existentes: status_escolar atual vai para fundamental
-- (comportamento conservador; admin pode ajustar depois)
UPDATE projection_estudantes
SET status_escolar_fundamental = status_escolar;

-- Remover coluna antiga
ALTER TABLE projection_estudantes
    DROP COLUMN IF EXISTS status_escolar;

COMMENT ON COLUMN projection_estudantes.status_escolar_fundamental IS
    'Status ensino fundamental: inativo | em_andamento | finalizado';
COMMENT ON COLUMN projection_estudantes.status_escolar_medio IS
    'Status ensino médio: inativo | em_andamento | finalizado';

CREATE INDEX IF NOT EXISTS idx_estudante_status_fund  ON projection_estudantes(status_escolar_fundamental);
CREATE INDEX IF NOT EXISTS idx_estudante_status_medio ON projection_estudantes(status_escolar_medio);

-- ============================================
-- 2. Reestruturar projection_aprovacao_ano
-- ============================================

-- Remover constraint única: aprovação agora é log auditável (N registros por ano)
ALTER TABLE projection_aprovacao_ano
    DROP CONSTRAINT IF EXISTS projection_aprovacao_ano_codigo_estudante_codigo_academia_ano_key;

-- Renomear colunas para refletir nova semântica
ALTER TABLE projection_aprovacao_ano
    RENAME COLUMN avancar_ano   TO aprovado;

ALTER TABLE projection_aprovacao_ano
    RENAME COLUMN nivel_seguinte TO proximo_nivel;

-- Adicionar tipo_ensino
ALTER TABLE projection_aprovacao_ano
    ADD COLUMN IF NOT EXISTS tipo_ensino VARCHAR(20)
        NOT NULL DEFAULT 'fundamental'
        CHECK (tipo_ensino IN ('fundamental', 'medio', 'superior'));

ALTER TABLE projection_aprovacao_ano
    ALTER COLUMN tipo_ensino DROP DEFAULT;

COMMENT ON COLUMN projection_aprovacao_ano.aprovado IS
    'TRUE = aprovado (avança ou finaliza); FALSE = reprovado (fica no mesmo nível)';
COMMENT ON COLUMN projection_aprovacao_ano.proximo_nivel IS
    'Próximo nível definido pela academia. NULL se reprovado ou último ano do curso';
COMMENT ON COLUMN projection_aprovacao_ano.tipo_ensino IS
    'Tipo de ensino: fundamental | medio | superior';

CREATE INDEX IF NOT EXISTS idx_aprovacao_tipo_ensino ON projection_aprovacao_ano(tipo_ensino);
CREATE INDEX IF NOT EXISTS idx_aprovacao_aprovado     ON projection_aprovacao_ano(aprovado);

-- ============================================
-- 3. Remover função de próximo nível (anos agora são dinâmicos por curso)
-- ============================================

DROP FUNCTION IF EXISTS get_proximo_nivel(VARCHAR, VARCHAR);

-- ============================================
-- 4. Atualizar views
-- ============================================

DROP VIEW IF EXISTS v_aprovacoes_completas;
CREATE OR REPLACE VIEW v_aprovacoes_completas AS
SELECT
    a.id,
    a.codigo_estudante,
    e.nome                                                       AS estudante_nome,
    a.codigo_academia,
    ac.nome                                                      AS academia_nome,
    a.ano_lectivo,
    a.tipo_ensino,
    a.nivel_atual,
    a.proximo_nivel,
    a.aprovado,
    a.observacao,
    a.registered_at,
    CASE WHEN a.aprovado THEN 'APROVADO' ELSE 'REPROVADO' END   AS resultado
FROM projection_aprovacao_ano a
LEFT JOIN projection_estudantes e  ON a.codigo_estudante = e.codigo_estudante
LEFT JOIN projection_academias  ac ON a.codigo_academia  = ac.codigo_academia;

-- Atualizar view de estudantes para novos status
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

COMMIT;

DO $$ BEGIN RAISE NOTICE '✅ MIGRATION 008 CONCLUÍDA - status_escolar dividido + aprovação revisada'; END $$;
