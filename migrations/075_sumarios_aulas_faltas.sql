-- Migration 075: sumários/aulas e vínculo opcional nas faltas
CREATE TABLE IF NOT EXISTS projection_sumarios_aulas (
  id UUID PRIMARY KEY,
  academia_id UUID NOT NULL,
  codigo_academia VARCHAR(50) NOT NULL,
  sumario_titulo TEXT NOT NULL CHECK (char_length(trim(sumario_titulo)) BETWEEN 3 AND 200),
  descricao TEXT,
  periodo TEXT NOT NULL,
  ano_academico INTEGER NOT NULL,
  nivel TEXT NOT NULL,
  type TEXT NOT NULL,
  curso_id UUID NULL REFERENCES projection_cursos(id) ON DELETE SET NULL,
  materia_id UUID NOT NULL REFERENCES projection_materias(id) ON DELETE RESTRICT,
  criado_por UUID,
  criado_em TIMESTAMPTZ NOT NULL,
  atualizado_em TIMESTAMPTZ NOT NULL,
  deleted_at TIMESTAMPTZ NULL,
  event_id UUID NOT NULL,
  version INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sumarios_academia_periodo ON projection_sumarios_aulas (codigo_academia, periodo) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_sumarios_contexto ON projection_sumarios_aulas (codigo_academia, ano_academico, curso_id, materia_id) WHERE deleted_at IS NULL;
ALTER TABLE projection_faltas ADD COLUMN IF NOT EXISTS sumario_id UUID NULL REFERENCES projection_sumarios_aulas(id) ON DELETE SET NULL;
ALTER TABLE projection_faltas ADD COLUMN IF NOT EXISTS sumario_titulo TEXT NULL;
CREATE INDEX IF NOT EXISTS idx_faltas_sumario ON projection_faltas(sumario_id) WHERE deleted_at IS NULL;
COMMENT ON TABLE projection_sumarios_aulas IS 'Sumários/aulas ministradas por academia, matéria e período, com remoção lógica para preservar vínculos históricos.';
COMMENT ON COLUMN projection_faltas.sumario_titulo IS 'Snapshot do título do sumário no momento do vínculo histórico da falta.';
