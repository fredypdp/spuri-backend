-- MIGRATION 100 — Remover coluna legada projection_estudantes.ano_escolar
--
-- CONTEXTO:
--   Desde a migration 034, a aplicação usa exclusivamente
--   ano_escolar_fundamental. A coluna "ano_escolar" está órfã e não é mais
--   escrita por nenhum código Go.
--
-- O QUE ESTA MIGRATION FAZ:
--   1. Recria v_estudantes_com_cursos e v_estudante_completo sem a coluna
--      legada.
--   2. Remove projection_estudantes.ano_escolar.

BEGIN;

DROP VIEW IF EXISTS v_estudantes_com_cursos CASCADE;
DROP VIEW IF EXISTS v_estudante_completo;

ALTER TABLE projection_estudantes DROP COLUMN IF EXISTS ano_escolar;

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
    e.ano_escolar_fundamental,
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

CREATE VIEW v_estudante_completo AS
SELECT
    e.*,
    (SELECT json_agg(n.*) FROM projection_notas     n WHERE n.codigo_estudante = e.codigo_estudante) AS notas,
    (SELECT json_agg(f.*) FROM projection_faltas     f WHERE f.codigo_estudante = e.codigo_estudante) AS faltas,
    (SELECT json_agg(i.*) FROM projection_inscricoes i WHERE i.estudante_id     = e.id)               AS inscricoes
FROM projection_estudantes e;

COMMIT;
