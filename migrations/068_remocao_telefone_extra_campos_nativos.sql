-- MIGRATION 068 — remove telefone extra e promove telefones nativos

ALTER TABLE projection_estudantes
    ADD COLUMN IF NOT EXISTS telefone_verificado BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS telefone_responsavel VARCHAR(20),
    ADD COLUMN IF NOT EXISTS telefone_responsavel_verificado BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE projection_estudantes
    ADD CONSTRAINT projection_estudantes_telefone_minimo_check CHECK (
        NULLIF(btrim(COALESCE(telefone, '')), '') IS NOT NULL
        OR NULLIF(btrim(COALESCE(telefone_responsavel, '')), '') IS NOT NULL
    );

ALTER TABLE projection_estudantes
    ADD CONSTRAINT projection_estudantes_telefones_diferentes_check CHECK (
        telefone IS NULL OR telefone_responsavel IS NULL OR telefone <> telefone_responsavel
    );

ALTER TABLE projection_academias
    RENAME COLUMN numero_telefone TO telefone;
ALTER TABLE projection_academias
    ADD COLUMN IF NOT EXISTS telefone_verificado BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE projection_admins
    ADD COLUMN IF NOT EXISTS telefone VARCHAR(20),
    ADD COLUMN IF NOT EXISTS telefone_verificado BOOLEAN NOT NULL DEFAULT FALSE;

DROP TABLE IF EXISTS projection_telefones_extra;
DELETE FROM projection_checkpoints WHERE projection_name = 'telefones_extra';

CREATE UNIQUE INDEX IF NOT EXISTS idx_telefone_verificado_estudante
    ON projection_estudantes (telefone) WHERE telefone_verificado = TRUE AND telefone IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_telefone_resp_verificado_estudante
    ON projection_estudantes (telefone_responsavel) WHERE telefone_responsavel_verificado = TRUE AND telefone_responsavel IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_telefone_verificado_academia
    ON projection_academias (telefone) WHERE telefone_verificado = TRUE AND telefone IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_telefone_verificado_admin
    ON projection_admins (telefone) WHERE telefone_verificado = TRUE AND telefone IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_estudante_telefone_responsavel_unico
    ON projection_estudantes (telefone_responsavel) WHERE telefone_responsavel IS NOT NULL;
