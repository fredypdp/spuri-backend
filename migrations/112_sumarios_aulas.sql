-- Migration 112: Sumários de Aula + vínculo opcional em Faltas
--
-- Contexto: já existiu uma tentativa anterior (migrations 075 e 076, esta
-- última revertendo a primeira). Esta é uma implementação nova. Duas correções
-- importantes em relação à tentativa anterior:
--   1) ano_academico aqui é TEXT no formato "N_ano_fundamental|medio|superior"
--      (nunca INTEGER — todo o resto do sistema usa esse formato de string).
--   2) periodo usa checagem estrutural via regex (N_trimestre | N_semestre),
--      em vez de uma lista fixa de 5 valores. A lista fixa hoje usada em
--      projection_faltas.periodo e projection_materias.periodo (chk_*_periodo_valores)
--      só permite até 2_semestre, o que é incompatível com cursos superiores
--      de mais de 1 ano (derivarCursoSuperior gera até N_semestre para cursos
--      de N semestres). Não alteramos essas constraints antigas nesta migration
--      (fora do escopo desta tarefa) — apenas evitamos repetir o mesmo problema
--      na tabela nova.

CREATE TABLE IF NOT EXISTS projection_sumarios (
    id                 UUID PRIMARY KEY,
    codigo_academia    VARCHAR(50) NOT NULL REFERENCES projection_academias(codigo_academia) ON DELETE CASCADE,
    sumario_titulo     TEXT NOT NULL CHECK (char_length(btrim(sumario_titulo)) BETWEEN 3 AND 200),
    descricao          TEXT,
    periodo            VARCHAR(20) NOT NULL CHECK (periodo ~ '^[1-9][0-9]*_(trimestre|semestre)$'),
    ano_academico      VARCHAR(50) NOT NULL CHECK (ano_academico ~ '^[1-9][0-9]*_ano_(fundamental|medio|superior)$'),
    nivel              VARCHAR(20) NOT NULL CHECK (nivel IN ('fundamental', 'medio', 'superior')),
    type               VARCHAR(20) NOT NULL CHECK (type IN ('escolar', 'superior')),
    curso_id           UUID NULL REFERENCES projection_cursos(id) ON DELETE CASCADE,
    materia_id         UUID NOT NULL REFERENCES projection_materias(id) ON DELETE CASCADE,
    criado_por         UUID NULL,
    status             VARCHAR(20) NOT NULL DEFAULT 'ativo' CHECK (status IN ('ativo', 'deletado')),
    deleted_at         TIMESTAMPTZ NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_event_id      UUID,
    version            INTEGER NOT NULL DEFAULT 0,
    CONSTRAINT check_sumario_fundamental_sem_curso CHECK (
        (nivel = 'fundamental' AND curso_id IS NULL)
        OR (nivel IN ('medio', 'superior') AND curso_id IS NOT NULL)
    ),
    CONSTRAINT check_sumario_type_nivel CHECK (
        (type = 'escolar' AND nivel IN ('fundamental', 'medio'))
        OR (type = 'superior' AND nivel = 'superior')
    )
);

CREATE INDEX IF NOT EXISTS idx_sumarios_academia ON projection_sumarios(codigo_academia);
CREATE INDEX IF NOT EXISTS idx_sumarios_materia ON projection_sumarios(materia_id);
CREATE INDEX IF NOT EXISTS idx_sumarios_curso ON projection_sumarios(curso_id);
CREATE INDEX IF NOT EXISTS idx_sumarios_not_deleted ON projection_sumarios(codigo_academia) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_sumarios_busca_vinculo ON projection_sumarios(materia_id, periodo, ano_academico) WHERE deleted_at IS NULL;

COMMENT ON TABLE projection_sumarios IS 'Projeção de leitura para sumários/aulas (Tarefa: Adicionar sumários e vincular opcionalmente às faltas)';
COMMENT ON COLUMN projection_sumarios.sumario_titulo IS 'Título da aula/sumário; snapshot histórico é copiado para projection_faltas.sumario_titulo no momento do vínculo';
COMMENT ON COLUMN projection_sumarios.curso_id IS 'Inferido automaticamente a partir de materia_id (materia.curso_id) — não é aceito diretamente do cliente';
COMMENT ON COLUMN projection_sumarios.nivel IS 'fundamental | medio | superior — espelha materia_disciplinar.type no momento da criação do sumário';
COMMENT ON COLUMN projection_sumarios.type IS 'escolar | superior — classificação derivada de nivel (fundamental/medio => escolar; superior => superior)';

-- ── Vínculo opcional falta -> sumário ──────────────────────────────────────

ALTER TABLE projection_faltas
    ADD COLUMN IF NOT EXISTS sumario_id UUID NULL REFERENCES projection_sumarios(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS sumario_titulo TEXT NULL;

CREATE INDEX IF NOT EXISTS idx_faltas_sumario ON projection_faltas(sumario_id) WHERE sumario_id IS NOT NULL;

COMMENT ON COLUMN projection_faltas.sumario_id IS 'Vínculo opcional à aula/sumário correspondente (Tarefa: Adicionar sumários)';
COMMENT ON COLUMN projection_faltas.sumario_titulo IS 'Snapshot do título do sumário no momento do vínculo; preserva leitura histórica mesmo se o sumário for renomeado ou deletado depois';
