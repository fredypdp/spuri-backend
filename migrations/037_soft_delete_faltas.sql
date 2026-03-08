-- Migration 037: soft delete em projection_faltas
-- projection_notas já possui deleted_at (migration anterior).
-- Idempotente via IF NOT EXISTS.

ALTER TABLE projection_faltas
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP DEFAULT NULL;

CREATE INDEX IF NOT EXISTS idx_faltas_estudante_ativo
    ON projection_faltas (codigo_estudante) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_faltas_academia_ativo
    ON projection_faltas (codigo_academia) WHERE deleted_at IS NULL;
