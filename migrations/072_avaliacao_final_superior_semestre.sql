BEGIN;

ALTER TABLE projection_avaliacao_final
    ADD COLUMN IF NOT EXISTS semestre_atual INTEGER,
    ADD COLUMN IF NOT EXISTS proximo_semestre_atual INTEGER,
    ADD COLUMN IF NOT EXISTS ano_superior_antes VARCHAR(50),
    ADD COLUMN IF NOT EXISTS ano_superior_depois VARCHAR(50);

COMMENT ON COLUMN projection_avaliacao_final.semestre_atual IS
    'Semestre sequencial avaliado em avaliações finais do ensino superior.';
COMMENT ON COLUMN projection_avaliacao_final.proximo_semestre_atual IS
    'Próximo semestre sequencial calculado em aprovação superior intermediária.';
COMMENT ON COLUMN projection_avaliacao_final.ano_superior_antes IS
    'Ano superior derivado antes da avaliação final superior.';
COMMENT ON COLUMN projection_avaliacao_final.ano_superior_depois IS
    'Ano superior derivado após a avaliação final superior.';

COMMIT;
