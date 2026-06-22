CREATE TABLE IF NOT EXISTS projection_anos_letivos_configuracoes (
  type TEXT PRIMARY KEY CHECK (type IN ('escolar','superior')),
  periodo VARCHAR(5) NOT NULL CHECK (periodo ~ '^(0?[1-9]|1[0-2])_(0?[1-9]|1[0-2])$'),
  updated_by UUID,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO projection_anos_letivos_configuracoes (type, periodo)
VALUES ('escolar','09_07'), ('superior','02_12')
ON CONFLICT (type) DO NOTHING;

CREATE TABLE IF NOT EXISTS projection_anos_letivos_academia_finalizacoes (
  academia_id UUID NOT NULL,
  codigo_academia VARCHAR(20) NOT NULL,
  type TEXT NOT NULL CHECK (type IN ('escolar','superior')),
  ano_letivo VARCHAR(20) NOT NULL CHECK (ano_letivo ~ '^\d{4}_\d{4}$'),
  finalizado BOOLEAN NOT NULL DEFAULT TRUE,
  finalizado_por UUID,
  finalizado_em TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  observacao TEXT,
  PRIMARY KEY (academia_id, type, ano_letivo)
);

CREATE INDEX IF NOT EXISTS idx_anos_letivos_finalizacoes_type_ano
  ON projection_anos_letivos_academia_finalizacoes (type, ano_letivo, finalizado);
