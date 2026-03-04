-- ============================================================================
-- MIGRATION 030 — Recriar view v_estudantes_com_cursos
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
-- O QUE ESTA MIGRATION FAZ:
--   Recria v_estudantes_com_cursos usando as colunas atuais de
--   projection_estudantes (status_escolar_fundamental, status_escolar_medio).
--   Não inclui campos sensíveis (senha_hash, bilhete_identidade_responsavel).
--   Usa CREATE OR REPLACE VIEW — idempotente.
-- ============================================================================

-- Recriar view usando CREATE OR REPLACE (não requer DROP prévio)
CREATE OR REPLACE VIEW v_estudantes_com_cursos AS
SELECT
    e.id,
    e.nome,
    e.codigo_estudante,
    e.email,
    e.codigo_academia,
    e.status,
    -- status_escolar foi dividido em 008 em dois campos:
    e.status_escolar_fundamental,
    e.status_escolar_medio,
    e.status_superior,
    e.ano_escolar,
    e.ano_escolar_medio,
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
LEFT JOIN projection_cursos cm ON e.curso_medio_id   = cm.id AND cm.deleted_at IS NULL
LEFT JOIN projection_cursos cs ON e.curso_superior_id = cs.id AND cs.deleted_at IS NULL;

COMMENT ON VIEW v_estudantes_com_cursos IS
    'View auxiliar de estudantes com nomes dos cursos médio e superior vinculados. '
    'Recriada na migration 030 para substituir a versão da migration 004 '
    '(que referenciava status_escolar removida na migration 008). '
    'Usa status_escolar_fundamental e status_escolar_medio conforme migration 008.';

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 030 — v_estudantes_com_cursos recriada com colunas corretas';
    RAISE NOTICE '   Substituídos: status_escolar → status_escolar_fundamental, status_escolar_medio';
    RAISE NOTICE '   Adicionados: ano_escolar_medio, curso_medio_type, curso_superior_type';
END $$;
