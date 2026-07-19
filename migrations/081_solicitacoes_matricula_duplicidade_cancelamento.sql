ALTER TABLE projection_solicitacoes_matricula
    ADD COLUMN IF NOT EXISTS solicitacoes_semelhantes TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[];

ALTER TABLE projection_solicitacoes_matricula
    DROP CONSTRAINT IF EXISTS projection_solicitacoes_matricula_status_check;

ALTER TABLE projection_solicitacoes_matricula
    ADD CONSTRAINT projection_solicitacoes_matricula_status_check
    CHECK (status IN ('pendente', 'aprovada', 'reprovada', 'cancelada'));

CREATE INDEX IF NOT EXISTS idx_solicitacoes_matricula_bi_normalizado
    ON projection_solicitacoes_matricula (lower(btrim(bilhete_identidade)))
    WHERE bilhete_identidade IS NOT NULL AND btrim(bilhete_identidade) <> '';
