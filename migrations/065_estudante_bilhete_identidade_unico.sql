-- Garante unicidade do bilhete de identidade principal entre estudantes.
-- bilhete_identidade_encarregado continua podendo repetir.

BEGIN;

CREATE UNIQUE INDEX IF NOT EXISTS uq_estudante_bilhete_identidade_normalizado
    ON projection_estudantes (lower(btrim(bilhete_identidade)))
    WHERE bilhete_identidade IS NOT NULL
      AND btrim(bilhete_identidade) <> '';

COMMIT;
