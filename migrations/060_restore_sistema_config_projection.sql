-- MIGRATION 060
-- Restaura a projeção de configurações globais do sistema.
--
-- A migration 041 removeu projection_sistema_config quando o ano letivo era
-- tratado apenas por academia. O endpoint canônico POST /admin/sistema/ano-letivo
-- voltou a persistir o ano letivo oficial global nessa projeção, portanto a
-- tabela precisa existir em ambientes que já executaram a 041.

CREATE TABLE IF NOT EXISTS projection_sistema_config (
    chave             VARCHAR(100) PRIMARY KEY,
    valor             TEXT NOT NULL,
    ano_letivo_atual  VARCHAR(20),
    data_inicio       TIMESTAMP,
    data_fim          TIMESTAMP,
    definido_por      UUID REFERENCES projection_admins(id) ON DELETE SET NULL,
    observacao        TEXT,
    updated_by        UUID REFERENCES projection_admins(id) ON DELETE SET NULL,
    updated_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    version           INTEGER NOT NULL DEFAULT 0,
    event_id          UUID,
    last_event_id     UUID
);

ALTER TABLE projection_sistema_config
    ADD COLUMN IF NOT EXISTS valor             TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS ano_letivo_atual  VARCHAR(20),
    ADD COLUMN IF NOT EXISTS data_inicio       TIMESTAMP,
    ADD COLUMN IF NOT EXISTS data_fim          TIMESTAMP,
    ADD COLUMN IF NOT EXISTS definido_por      UUID REFERENCES projection_admins(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS observacao        TEXT,
    ADD COLUMN IF NOT EXISTS updated_by        UUID REFERENCES projection_admins(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS updated_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ADD COLUMN IF NOT EXISTS version           INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS event_id          UUID,
    ADD COLUMN IF NOT EXISTS last_event_id     UUID;

CREATE INDEX IF NOT EXISTS idx_sistema_config_chave
    ON projection_sistema_config(chave);

INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at)
VALUES ('sistema_config', 0, CURRENT_TIMESTAMP)
ON CONFLICT (projection_name) DO NOTHING;

COMMENT ON TABLE projection_sistema_config IS
    'Projeção de configurações globais do sistema';
COMMENT ON COLUMN projection_sistema_config.chave IS
    'Chave única da configuração (ex: ano_letivo_atual)';
COMMENT ON COLUMN projection_sistema_config.valor IS
    'Valor atual da configuração';
COMMENT ON COLUMN projection_sistema_config.ano_letivo_atual IS
    'Valor do ano letivo oficial global atual';
COMMENT ON COLUMN projection_sistema_config.definido_por IS
    'UUID do admin que definiu a configuração';
