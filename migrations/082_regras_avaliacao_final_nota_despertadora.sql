BEGIN;

ALTER TABLE projection_regras_avaliacao_final
    ADD COLUMN IF NOT EXISTS nota_despertadora VARCHAR(80);

COMMENT ON COLUMN projection_regras_avaliacao_final.nota_despertadora IS
    'Código da categoria de nota que desperta automaticamente apenas uma regra raiz de avaliação final. Regras descendentes permanecem nulas e são acionadas por reprovação ancestral.';

CREATE INDEX IF NOT EXISTS idx_regras_avf_nota_despertadora_raiz
ON projection_regras_avaliacao_final (codigo_academia, nivel, nota_despertadora, status)
WHERE aplica_se_reprovado_em_type IS NULL AND nota_despertadora IS NOT NULL;

COMMIT;
