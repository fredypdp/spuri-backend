-- ============================================
-- MIGRATION 007 - Turmas + Gênero do Estudante
-- ============================================

-- 1. Adicionar campo genero em projection_estudantes
ALTER TABLE projection_estudantes
    ADD COLUMN IF NOT EXISTS genero VARCHAR(10) CHECK (genero IN ('masculino', 'feminino'));

COMMENT ON COLUMN projection_estudantes.genero IS 'Gênero do estudante: masculino ou feminino';

-- 2. Criar tabela projection_turmas
CREATE TABLE IF NOT EXISTS projection_turmas (
    id              UUID PRIMARY KEY,
    codigo_turma    VARCHAR(50) NOT NULL,
    codigo_academia VARCHAR(50) NOT NULL,
    nivel           VARCHAR(50) NOT NULL,
    curso_id        UUID,
    turno           VARCHAR(10) NOT NULL CHECK (turno IN ('manha', 'tarde', 'noite')),
    estudantes      JSONB NOT NULL DEFAULT '[]',
    status          VARCHAR(20) NOT NULL DEFAULT 'ativo' CHECK (status IN ('ativo', 'inativo')),
    created_at      TIMESTAMP NOT NULL,
    updated_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    version         INTEGER NOT NULL DEFAULT 0,
    last_event_id   UUID,

    UNIQUE (codigo_turma, codigo_academia),

    FOREIGN KEY (codigo_academia)
        REFERENCES projection_academias(codigo_academia)
        ON DELETE CASCADE,

    FOREIGN KEY (curso_id)
        REFERENCES projection_cursos(id)
        ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_turmas_academia ON projection_turmas(codigo_academia);
CREATE INDEX IF NOT EXISTS idx_turmas_nivel    ON projection_turmas(nivel);
CREATE INDEX IF NOT EXISTS idx_turmas_turno    ON projection_turmas(turno);
CREATE INDEX IF NOT EXISTS idx_turmas_status   ON projection_turmas(status);
CREATE INDEX IF NOT EXISTS idx_turmas_curso    ON projection_turmas(curso_id);

-- 3. Checkpoint para a projeção
INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at)
VALUES ('turmas', 0, CURRENT_TIMESTAMP)
ON CONFLICT (projection_name) DO NOTHING;

-- Comentários
COMMENT ON TABLE  projection_turmas                IS 'Projeção de turmas das academias';
COMMENT ON COLUMN projection_turmas.codigo_turma   IS 'Identificador único da turma dentro da academia';
COMMENT ON COLUMN projection_turmas.nivel          IS 'Ano escolar/superior da turma';
COMMENT ON COLUMN projection_turmas.curso_id       IS 'FK para curso (apenas médio/superior)';
COMMENT ON COLUMN projection_turmas.turno          IS 'Turno: manha, tarde, noite';
COMMENT ON COLUMN projection_turmas.estudantes     IS 'Array JSON com códigos dos estudantes da turma';

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 007 CONCLUÍDA - Turmas criadas e gênero adicionado ao estudante';
END $$;