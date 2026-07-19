-- MIGRATION 068 — remove telefone extra e promove telefones nativos

ALTER TABLE projection_estudantes
    ADD COLUMN IF NOT EXISTS telefone_verificado BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS telefone_encarregado VARCHAR(20),
    ADD COLUMN IF NOT EXISTS telefone_encarregado_verificado BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE projection_estudantes
    ADD CONSTRAINT projection_estudantes_telefone_minimo_check CHECK (
        NULLIF(btrim(COALESCE(telefone, '')), '') IS NOT NULL
        OR NULLIF(btrim(COALESCE(telefone_encarregado, '')), '') IS NOT NULL
    );

ALTER TABLE projection_estudantes
    ADD CONSTRAINT projection_estudantes_telefones_diferentes_check CHECK (
        telefone IS NULL OR telefone_encarregado IS NULL OR telefone <> telefone_encarregado
    );

-- Em bases novas (ex.: Aiven criado do zero), a migration 001 já cria
-- projection_academias.telefone. Em bases antigas, ainda pode existir
-- numero_telefone. Normaliza os dois cenários sem falhar por coluna ausente.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = 'projection_academias'
          AND column_name = 'numero_telefone'
    ) THEN
        IF NOT EXISTS (
            SELECT 1
            FROM information_schema.columns
            WHERE table_schema = 'public'
              AND table_name = 'projection_academias'
              AND column_name = 'telefone'
        ) THEN
            ALTER TABLE projection_academias
                RENAME COLUMN numero_telefone TO telefone;
        ELSE
            UPDATE projection_academias
            SET telefone = COALESCE(telefone, numero_telefone)
            WHERE telefone IS NULL AND numero_telefone IS NOT NULL;

            ALTER TABLE projection_academias
                DROP COLUMN numero_telefone;
        END IF;
    ELSE
        ALTER TABLE projection_academias
            ADD COLUMN IF NOT EXISTS telefone VARCHAR(20);
    END IF;
END $$;

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
    ON projection_estudantes (telefone_encarregado) WHERE telefone_encarregado_verificado = TRUE AND telefone_encarregado IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_telefone_verificado_academia
    ON projection_academias (telefone) WHERE telefone_verificado = TRUE AND telefone IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_telefone_verificado_admin
    ON projection_admins (telefone) WHERE telefone_verificado = TRUE AND telefone IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_estudante_telefone_encarregado_unico
    ON projection_estudantes (telefone_encarregado) WHERE telefone_encarregado IS NOT NULL;
