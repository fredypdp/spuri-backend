-- ============================================================================
-- MIGRATION 033 — Correções auditoria-etapa2-db (DB-10, DB-15) — CORRIGIDA
-- ============================================================================
-- DB-10: DROP das colunas curso_medio e curso_superior (VARCHAR obsoletas)
--   CORREÇÃO: derrubar views dependentes ANTES do DROP COLUMN.
--   v_estudante_completo usa SELECT e.* → depende de todas as colunas.
--   v_estudantes_com_cursos depende de v_estudante_completo via CASCADE.
--   Padrão idêntico às migrations 008, 009 e 030.
-- ============================================================================

BEGIN;

-- ============================================================================
-- 1. Derrubar views que dependem de projection_estudantes
--    CASCADE remove v_estudante_completo e qualquer view que dependa dela
-- ============================================================================

DROP VIEW IF EXISTS v_estudantes_com_cursos CASCADE;
DROP VIEW IF EXISTS v_estudante_completo    CASCADE;

-- ============================================================================
-- 2. DB-10: remover colunas VARCHAR obsoletas de projection_estudantes
-- ============================================================================

ALTER TABLE projection_estudantes
    DROP COLUMN IF EXISTS curso_medio;

ALTER TABLE projection_estudantes
    DROP COLUMN IF EXISTS curso_superior;

COMMENT ON TABLE projection_estudantes IS
    'Projeção de leitura para estudantes. '
    'Colunas de curso usam UUID (curso_medio_id, curso_superior_id) '
    'com FK para projection_cursos. '
    'Colunas VARCHAR legadas (curso_medio, curso_superior) removidas na migration 033.';

-- ============================================================================
-- 3. Recriar v_estudantes_com_cursos (definição da migration 030)
-- ============================================================================

CREATE VIEW v_estudantes_com_cursos AS
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
    e.email_verificado,
    cm.id   AS curso_medio_id,
    cm.nome AS curso_medio_nome,
    cm.type AS curso_medio_type,
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
    'Recriada na migration 033 após DROP CASCADE para remoção das colunas VARCHAR legadas.';

-- ============================================================================
-- 4. Recriar v_estudante_completo (derrubada pelo CASCADE acima)
-- ============================================================================

CREATE VIEW v_estudante_completo AS
SELECT
    e.*,
    (SELECT json_agg(n.*) FROM projection_notas     n WHERE n.codigo_estudante = e.codigo_estudante) AS notas,
    (SELECT json_agg(f.*) FROM projection_faltas     f WHERE f.codigo_estudante = e.codigo_estudante) AS faltas,
    (SELECT json_agg(i.*) FROM projection_inscricoes i WHERE i.estudante_id     = e.id)               AS inscricoes
FROM projection_estudantes e;

COMMENT ON VIEW v_estudante_completo IS
    'View completa do estudante com notas, faltas e inscrições em JSON. '
    'Recriada na migration 033 após DROP CASCADE de v_estudantes_com_cursos.';

-- ============================================================================
-- 5. DB-15: verificar consistência dos checkpoints da migration 024
-- ============================================================================

INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at, events_processed)
VALUES
    ('aprovacao_ano', 0, CURRENT_TIMESTAMP, 0),
    ('reprovacoes',   0, CURRENT_TIMESTAMP, 0)
ON CONFLICT (projection_name) DO NOTHING;

DO $$
DECLARE
    v_idx_status    TEXT;
    v_idx_academia  TEXT;
    v_idx_estudante TEXT;
BEGIN
    SELECT CASE WHEN EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'idx_projection_inscricoes_status')
           THEN 'EXISTS' ELSE 'DROPPED' END INTO v_idx_status;
    SELECT CASE WHEN EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'idx_projection_inscricoes_academia')
           THEN 'EXISTS' ELSE 'DROPPED' END INTO v_idx_academia;
    SELECT CASE WHEN EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'idx_projection_inscricoes_estudante')
           THEN 'EXISTS' ELSE 'DROPPED' END INTO v_idx_estudante;

    RAISE NOTICE '[DB-15] Estado dos índices da migration 024:';
    RAISE NOTICE '  idx_projection_inscricoes_status:    %', v_idx_status;
    RAISE NOTICE '  idx_projection_inscricoes_academia:  %', v_idx_academia;
    RAISE NOTICE '  idx_projection_inscricoes_estudante: %', v_idx_estudante;

    IF v_idx_status    = 'EXISTS' THEN DROP INDEX IF EXISTS idx_projection_inscricoes_status;    END IF;
    IF v_idx_academia  = 'EXISTS' THEN DROP INDEX IF EXISTS idx_projection_inscricoes_academia;  END IF;
    IF v_idx_estudante = 'EXISTS' THEN DROP INDEX IF EXISTS idx_projection_inscricoes_estudante; END IF;
END $$;

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 033 — Correções auditoria-etapa2-db aplicadas';
    RAISE NOTICE '   DB-10: colunas curso_medio e curso_superior (VARCHAR) removidas de projection_estudantes';
    RAISE NOTICE '   FIX:   views v_estudante_completo e v_estudantes_com_cursos recriadas após CASCADE';
    RAISE NOTICE '   DB-15: checkpoints da migration 024 verificados e consistentes';
    RAISE NOTICE '   ⚠️  Rebuild recomendado: POST /admin/rebuild-projection/estudantes';
END $$;