-- ============================================================================
-- MIGRATION 030 — Recriar view v_estudantes_com_cursos (CORRIGIDA)
-- ============================================================================
--
-- CONTEXTO (ERRO-MIG-01 da auditoria-etapa2-db.md):
--   A migration 004 criou a view v_estudantes_com_cursos referenciando a coluna
--   status_escolar de projection_estudantes.
--   A migration 008 fez DROP VIEW v_estudantes_com_cursos CASCADE (para poder
--   remover a coluna status_escolar) e adicionou status_escolar_fundamental e
--   status_escolar_medio, mas não recriou a view.
--   Desde migration 008, a view não existe — qualquer query ou código que
--   dependa dela falha com "relation v_estudantes_com_cursos does not exist".
--
-- CORREÇÃO DO BUG DE DEPLOY:
--   A versão original desta migration usava CREATE OR REPLACE VIEW omitindo
--   a coluna `genero`. O PostgreSQL não permite CREATE OR REPLACE quando isso
--   altera a posição ou nome de colunas existentes na view:
--
--     Estado da view após migration 009 (última a tocá-la):
--       pos.5 = genero, pos.6 = codigo_academia, ...
--     O que CREATE OR REPLACE tentava impor:
--       pos.5 = codigo_academia, ...  ← genero ausente
--
--   Resultado: "cannot change name of view column genero to codigo_academia"
--
-- CORREÇÃO APLICADA:
--   1. DROP VIEW ... CASCADE (remove também v_estudante_completo que depende)
--   2. DROP VIEW IF EXISTS v_estudante_completo explícito — garante remoção
--      mesmo que v_estudantes_com_cursos não existisse (CASCADE não dispara)
--   3. CREATE VIEW com todas as colunas corretas, incluindo `genero`
--   4. Recriar v_estudante_completo
-- ============================================================================

BEGIN;

-- 1. Dropar views dependentes
--    CASCADE remove v_estudante_completo automaticamente se a view existir
DROP VIEW IF EXISTS v_estudantes_com_cursos CASCADE;

-- 2. Drop explícito de v_estudante_completo — necessário quando
--    v_estudantes_com_cursos não existia e o CASCADE não foi disparado
DROP VIEW IF EXISTS v_estudante_completo;

-- 3. Recriar v_estudantes_com_cursos com todas as colunas corretas
CREATE VIEW v_estudantes_com_cursos AS
SELECT
    e.id,
    e.nome,
    e.codigo_estudante,
    e.email,
    e.genero,                        -- adicionado na migration 007; omitido na versão anterior desta migration
    e.codigo_academia,
    e.status,
    e.status_escolar_fundamental,    -- dividido na migration 008
    e.status_escolar_medio,          -- dividido na migration 008
    e.status_superior,
    e.ano_escolar,
    e.ano_escolar_medio,             -- adicionado na migration 009
    e.ano_superior,
    e.email_verificado,
    -- Curso médio vinculado (nullable)
    cm.id   AS curso_medio_id,
    cm.nome AS curso_medio_nome,
    cm.type AS curso_medio_type,
    -- Curso superior vinculado (nullable)
    cs.id   AS curso_superior_id,
    cs.nome AS curso_superior_nome,
    cs.type AS curso_superior_type,
    e.created_at,
    e.updated_at
FROM projection_estudantes e
LEFT JOIN projection_cursos cm ON e.curso_medio_id    = cm.id AND cm.deleted_at IS NULL
LEFT JOIN projection_cursos cs ON e.curso_superior_id = cs.id AND cs.deleted_at IS NULL;

COMMENT ON VIEW v_estudantes_com_cursos IS
    'View auxiliar de estudantes com cursos médio e superior vinculados. '
    'Recriada na migration 030 (corrigida) com DROP CASCADE em vez de CREATE OR REPLACE. '
    'Inclui: genero (mig 007), status_escolar_fundamental/medio (mig 008), '
    'ano_escolar_medio (mig 009), curso_*_type (mig 030). '
    'Filtra cursos soft-deleted (deleted_at IS NULL, mig 019).';

-- 4. Recriar v_estudante_completo
CREATE VIEW v_estudante_completo AS
SELECT
    e.*,
    (SELECT json_agg(n.*) FROM projection_notas     n WHERE n.codigo_estudante = e.codigo_estudante) AS notas,
    (SELECT json_agg(f.*) FROM projection_faltas     f WHERE f.codigo_estudante = e.codigo_estudante) AS faltas,
    (SELECT json_agg(i.*) FROM projection_inscricoes i WHERE i.estudante_id     = e.id)               AS inscricoes
FROM projection_estudantes e;

COMMENT ON VIEW v_estudante_completo IS
    'View completa do estudante com notas, faltas e inscrições em JSON. '
    'Recriada na migration 030 após DROP CASCADE de v_estudantes_com_cursos.';

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 030 — v_estudantes_com_cursos recriada corretamente';
    RAISE NOTICE '   FIX: DROP CASCADE + CREATE VIEW (não CREATE OR REPLACE)';
    RAISE NOTICE '   FIX: DROP explícito de v_estudante_completo antes de recriar';
    RAISE NOTICE '   FIX: coluna genero incluída na posição correta (pos.5)';
    RAISE NOTICE '   FIX: v_estudante_completo recriada após CASCADE';
    RAISE NOTICE '   Colunas: id, nome, codigo_estudante, email, genero,';
    RAISE NOTICE '            codigo_academia, status, status_escolar_fundamental,';
    RAISE NOTICE '            status_escolar_medio, status_superior, ano_escolar,';
    RAISE NOTICE '            ano_escolar_medio, ano_superior, email_verificado,';
    RAISE NOTICE '            curso_medio_*, curso_superior_*, created_at, updated_at';
END $$;