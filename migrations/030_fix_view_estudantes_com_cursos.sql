-- ============================================================================
-- MIGRATION 030 — Recriar view v_estudantes_com_cursos (CORRIGIDA)
-- ============================================================================
--
-- PROBLEMA ORIGINAL:
--   A migration 030 usava CREATE OR REPLACE VIEW omitindo a coluna `genero`.
--   O PostgreSQL não permite CREATE OR REPLACE quando isso renomearia colunas
--   existentes — a coluna 5 era `genero` (desde migration 007/008/009) e
--   passaria a ser `codigo_academia`, gerando:
--   "cannot change name of view column genero to codigo_academia"
--
-- CORREÇÃO:
--   1. DROP VIEW ... CASCADE (derruba também v_estudante_completo que depende)
--   2. CREATE VIEW com todas as colunas corretas, incluindo `genero`
--   3. Recriar v_estudante_completo (derrubada pelo CASCADE)
--
-- ESTADO DA VIEW APÓS MIGRATION 009 (última a tocá-la antes desta):
--   id, nome, codigo_estudante, email, genero, codigo_academia, status,
--   status_escolar_fundamental, status_escolar_medio, status_superior,
--   ano_escolar, ano_escolar_medio, ano_superior,
--   curso_medio_id, curso_medio_nome, curso_superior_id, curso_superior_nome,
--   created_at, updated_at
-- ============================================================================

-- 1. Dropar views dependentes (CASCADE remove v_estudante_completo também)
DROP VIEW IF EXISTS v_estudantes_com_cursos CASCADE;

-- 2. Recriar v_estudantes_com_cursos com colunas completas e corretas
CREATE VIEW v_estudantes_com_cursos AS
SELECT
    e.id,
    e.nome,
    e.codigo_estudante,
    e.email,
    e.genero,                          -- adicionado na migration 007, omitido erroneamente na 030 original
    e.codigo_academia,
    e.status,
    e.status_escolar_fundamental,      -- dividido na migration 008
    e.status_escolar_medio,            -- dividido na migration 008
    e.status_superior,
    e.ano_escolar,
    e.ano_escolar_medio,               -- adicionado na migration 009
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
    'Recriada na migration 030 (corrigida) com DROP CASCADE + CREATE. '
    'Inclui: genero (mig 007), status_escolar_fundamental/medio (mig 008), '
    'ano_escolar_medio (mig 009), curso_*_type (mig 030).';

-- 3. Recriar v_estudante_completo (derrubada pelo CASCADE acima)
--    Definição idêntica à migration 009 / 008
CREATE OR REPLACE VIEW v_estudante_completo AS
SELECT
    e.*,
    (SELECT json_agg(n.*) FROM projection_notas      n WHERE n.codigo_estudante = e.codigo_estudante) AS notas,
    (SELECT json_agg(f.*) FROM projection_faltas      f WHERE f.codigo_estudante = e.codigo_estudante) AS faltas,
    (SELECT json_agg(i.*) FROM projection_inscricoes  i WHERE i.estudante_id     = e.id)               AS inscricoes
FROM projection_estudantes e;

COMMENT ON VIEW v_estudante_completo IS
    'View completa do estudante com notas, faltas e inscrições em JSON. '
    'Recriada na migration 030 após CASCADE do DROP de v_estudantes_com_cursos.';

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 030 — v_estudantes_com_cursos recriada com DROP CASCADE + CREATE';
    RAISE NOTICE '   Fix: genero incluído, CREATE OR REPLACE substituído por DROP+CREATE';
    RAISE NOTICE '   Fix: v_estudante_completo recriada após CASCADE';
    RAISE NOTICE '   Colunas: id, nome, codigo_estudante, email, genero, codigo_academia,';
    RAISE NOTICE '            status, status_escolar_fundamental, status_escolar_medio,';
    RAISE NOTICE '            status_superior, ano_escolar, ano_escolar_medio, ano_superior,';
    RAISE NOTICE '            email_verificado, curso_medio_*, curso_superior_*,';
    RAISE NOTICE '            created_at, updated_at';
END $$;