-- Garante que os períodos de anos letivos sejam regra fixa de sistema,
-- derivada exclusivamente do tipo de ensino.
INSERT INTO projection_anos_letivos_configuracoes (type, periodo, updated_at)
VALUES ('escolar','09_07', NOW()), ('superior','10_07', NOW())
ON CONFLICT (type) DO UPDATE
  SET periodo = EXCLUDED.periodo,
      updated_at = NOW(),
      updated_by = NULL;

ALTER TABLE projection_anos_letivos_configuracoes
  DROP CONSTRAINT IF EXISTS projection_anos_letivos_configuracoes_periodo_fixo_chk;

ALTER TABLE projection_anos_letivos_configuracoes
  ADD CONSTRAINT projection_anos_letivos_configuracoes_periodo_fixo_chk
  CHECK (
    (type = 'escolar' AND periodo = '09_07') OR
    (type = 'superior' AND periodo = '10_07')
  );
