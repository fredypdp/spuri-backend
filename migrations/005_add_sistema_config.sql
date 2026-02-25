-- ============================================
-- MIGRATION 005 - Projeção de Configuração do Sistema
-- ============================================

CREATE TABLE IF NOT EXISTS projection_sistema_config (
    chave         VARCHAR(100) PRIMARY KEY,
    valor         TEXT NOT NULL,
    updated_by    UUID REFERENCES projection_admins(id) ON DELETE SET NULL,
    updated_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    version       INTEGER NOT NULL DEFAULT 0,
    last_event_id UUID
);

CREATE INDEX IF NOT EXISTS idx_sistema_config_chave ON projection_sistema_config(chave);

-- Checkpoint para a projeção
INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at)
VALUES ('sistema_config', 0, CURRENT_TIMESTAMP)
ON CONFLICT (projection_name) DO NOTHING;

-- Comentários
COMMENT ON TABLE projection_sistema_config IS 'Projeção de configurações globais do sistema';
COMMENT ON COLUMN projection_sistema_config.chave IS 'Chave única da configuração (ex: ano_letivo_atual)';
COMMENT ON COLUMN projection_sistema_config.valor IS 'Valor atual da configuração';
COMMENT ON COLUMN projection_sistema_config.updated_by IS 'Admin que definiu o valor';