-- Alinha projection_estudantes ao schema usado pelo código atual.
-- Bancos que já tinham a baseline 000 aplicada não recebiam migrations
-- incrementais porque o runner executava apenas a baseline quando ela existia.
-- Por isso, algumas colunas esperadas por /estudantes e pela projeção podiam
-- faltar em produção (ex.: status_escolar_fundamental).

ALTER TABLE projection_estudantes
    ADD COLUMN IF NOT EXISTS genero VARCHAR(20),
    ADD COLUMN IF NOT EXISTS data_nascimento DATE,
    ADD COLUMN IF NOT EXISTS status_escolar_fundamental VARCHAR(20) DEFAULT 'inativo',
    ADD COLUMN IF NOT EXISTS status_escolar_medio VARCHAR(20) DEFAULT 'inativo',
    ADD COLUMN IF NOT EXISTS ano_escolar_medio VARCHAR(50),
    ADD COLUMN IF NOT EXISTS semestre_atual INTEGER,
    ADD COLUMN IF NOT EXISTS curso_medio_id UUID,
    ADD COLUMN IF NOT EXISTS curso_superior_id UUID;

-- Preserva dados de instalações antigas que usavam a coluna genérica
-- status_escolar para o ensino fundamental.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = 'projection_estudantes'
          AND column_name = 'status_escolar'
    ) THEN
        UPDATE projection_estudantes
        SET status_escolar_fundamental = COALESCE(status_escolar_fundamental, status_escolar, 'inativo'),
            status_escolar_medio = COALESCE(status_escolar_medio, 'inativo'),
            status_superior = COALESCE(status_superior, 'inativo')
        WHERE status_escolar_fundamental IS NULL
           OR status_escolar_medio IS NULL
           OR status_superior IS NULL;
    ELSE
        UPDATE projection_estudantes
        SET status_escolar_fundamental = COALESCE(status_escolar_fundamental, 'inativo'),
            status_escolar_medio = COALESCE(status_escolar_medio, 'inativo'),
            status_superior = COALESCE(status_superior, 'inativo')
        WHERE status_escolar_fundamental IS NULL
           OR status_escolar_medio IS NULL
           OR status_superior IS NULL;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'projection_estudantes_genero_check'
          AND conrelid = 'projection_estudantes'::regclass
    ) THEN
        ALTER TABLE projection_estudantes
            ADD CONSTRAINT projection_estudantes_genero_check
            CHECK (genero IS NULL OR genero IN ('masculino', 'feminino'));
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'projection_estudantes_status_fundamental_check'
          AND conrelid = 'projection_estudantes'::regclass
    ) THEN
        ALTER TABLE projection_estudantes
            ADD CONSTRAINT projection_estudantes_status_fundamental_check
            CHECK (status_escolar_fundamental IN ('inativo', 'em_andamento', 'finalizado'));
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'projection_estudantes_status_medio_check'
          AND conrelid = 'projection_estudantes'::regclass
    ) THEN
        ALTER TABLE projection_estudantes
            ADD CONSTRAINT projection_estudantes_status_medio_check
            CHECK (status_escolar_medio IN ('inativo', 'em_andamento', 'finalizado'));
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_proj_estudante_curso_medio_id ON projection_estudantes(curso_medio_id);
CREATE INDEX IF NOT EXISTS idx_proj_estudante_curso_superior_id ON projection_estudantes(curso_superior_id);
