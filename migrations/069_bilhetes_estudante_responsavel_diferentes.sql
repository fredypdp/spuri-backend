-- MIGRATION 069 - Impede BI do estudante igual ao BI do responsável
-- A regra vale para cadastro direto de estudante e solicitações públicas de matrícula.
-- NOT VALID preserva dados legados; PostgreSQL passa a validar novos INSERT/UPDATE.

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'projection_estudantes_bilhetes_diferentes_check'
          AND conrelid = 'projection_estudantes'::regclass
    ) THEN
        ALTER TABLE projection_estudantes
            ADD CONSTRAINT projection_estudantes_bilhetes_diferentes_check
            CHECK (
                bilhete_identidade IS NULL
                OR bilhete_identidade_responsavel IS NULL
                OR btrim(bilhete_identidade) = ''
                OR btrim(bilhete_identidade_responsavel) = ''
                OR lower(btrim(bilhete_identidade)) <> lower(btrim(bilhete_identidade_responsavel))
            ) NOT VALID;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'projection_solicitacoes_matricula_bilhetes_diferentes_check'
          AND conrelid = 'projection_solicitacoes_matricula'::regclass
    ) THEN
        ALTER TABLE projection_solicitacoes_matricula
            ADD CONSTRAINT projection_solicitacoes_matricula_bilhetes_diferentes_check
            CHECK (
                bilhete_identidade IS NULL
                OR bilhete_identidade_responsavel IS NULL
                OR btrim(bilhete_identidade) = ''
                OR btrim(bilhete_identidade_responsavel) = ''
                OR lower(btrim(bilhete_identidade)) <> lower(btrim(bilhete_identidade_responsavel))
            ) NOT VALID;
    END IF;
END $$;

COMMENT ON CONSTRAINT projection_estudantes_bilhetes_diferentes_check ON projection_estudantes IS
    'Impede que bilhete_identidade e bilhete_identidade_responsavel do mesmo estudante sejam iguais.';
COMMENT ON CONSTRAINT projection_solicitacoes_matricula_bilhetes_diferentes_check ON projection_solicitacoes_matricula IS
    'Impede que bilhete_identidade e bilhete_identidade_responsavel da mesma solicitação sejam iguais.';
