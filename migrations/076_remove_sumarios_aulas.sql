-- Migration 076: remove definitivamente sumários/aulas e vínculo nas faltas
DROP INDEX IF EXISTS idx_faltas_sumario;
ALTER TABLE projection_faltas DROP COLUMN IF EXISTS sumario_titulo;
ALTER TABLE projection_faltas DROP COLUMN IF EXISTS sumario_id;
DROP INDEX IF EXISTS idx_sumarios_contexto;
DROP INDEX IF EXISTS idx_sumarios_academia_periodo;
DROP TABLE IF EXISTS projection_sumarios_aulas;
DELETE FROM projection_checkpoints WHERE projection_name = 'sumarios';
