-- Correções de notas/faltas são eventos compensatórios; estas colunas guardam
-- o estado vigente e a auditoria imediata da última correção na projeção.
BEGIN;

ALTER TABLE projection_notas
    ADD COLUMN IF NOT EXISTS registrado_por UUID,
    ADD COLUMN IF NOT EXISTS valor_anterior DECIMAL(5,2),
    ADD COLUMN IF NOT EXISTS motivo_correcao TEXT,
    ADD COLUMN IF NOT EXISTS corrigido_por UUID,
    ADD COLUMN IF NOT EXISTS corrigido_em TIMESTAMP;

ALTER TABLE projection_faltas
    ADD COLUMN IF NOT EXISTS registrado_por UUID,
    ADD COLUMN IF NOT EXISTS valor_anterior INTEGER,
    ADD COLUMN IF NOT EXISTS motivo_correcao TEXT,
    ADD COLUMN IF NOT EXISTS corrigido_por UUID,
    ADD COLUMN IF NOT EXISTS corrigido_em TIMESTAMP;

CREATE INDEX IF NOT EXISTS idx_notas_corrigido_em ON projection_notas (corrigido_em) WHERE corrigido_em IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_faltas_corrigido_em ON projection_faltas (corrigido_em) WHERE corrigido_em IS NOT NULL;

COMMIT;
