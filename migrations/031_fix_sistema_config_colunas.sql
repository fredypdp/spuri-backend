-- ============================================================================
-- MIGRATION 031 — Adicionar colunas ausentes em projection_sistema_config
--
-- PROBLEMA CORRIGIDO (P3-12):
--   A migration 005 criou projection_sistema_config apenas com as colunas:
--     chave, valor, updated_by, updated_at, version, last_event_id
--
--   O handler handleAnoLetivoDefinido em sistema_config_projection.go
--   tenta fazer INSERT/UPDATE com as colunas:
--     ano_letivo_atual, data_inicio, data_fim, definido_por, observacao, event_id
--
--   Essas colunas não existiam, causando erro:
--     "column 'ano_letivo_atual' does not exist"
--   em TODA execução de handleAnoLetivoDefinido — bloqueando a definição
--   do ano letivo no sistema inteiro.
--
-- O QUE ESTA MIGRATION FAZ:
--   Adiciona as colunas faltantes de forma idempotente (ADD COLUMN IF NOT EXISTS).
--   Adiciona checkpoint para a projeção sistema_config (se não existir).
--   Não destrói dados existentes.
-- ============================================================================

BEGIN;

-- 1. Adicionar colunas que o handler precisa e a migration 005 não criou
ALTER TABLE projection_sistema_config
    ADD COLUMN IF NOT EXISTS ano_letivo_atual VARCHAR(20),
    ADD COLUMN IF NOT EXISTS data_inicio      TIMESTAMP,
    ADD COLUMN IF NOT EXISTS data_fim         TIMESTAMP,
    ADD COLUMN IF NOT EXISTS definido_por     UUID REFERENCES projection_admins(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS observacao       TEXT,
    ADD COLUMN IF NOT EXISTS event_id         UUID;

-- 2. Remover coluna updated_by (substituída por definido_por com mesmo propósito)
--    ATENÇÃO: só executar se não houver dependências externas nessa coluna.
--    A coluna updated_by era o campo original da migration 005.
--    definido_por é o nome correto alinhado com o evento AnoLetivoDefinidoEvent.
--    Mantemos updated_by para compatibilidade retroativa (pode ser removida futuramente).

-- 3. Comentários
COMMENT ON COLUMN projection_sistema_config.ano_letivo_atual IS
    'Valor do ano letivo atual (redundante com valor, facilita queries diretas)';
COMMENT ON COLUMN projection_sistema_config.data_inicio IS
    'Data de início do ano letivo. NULL para eventos legados sem este campo.';
COMMENT ON COLUMN projection_sistema_config.data_fim IS
    'Data de fim do ano letivo. NULL para eventos legados sem este campo.';
COMMENT ON COLUMN projection_sistema_config.definido_por IS
    'UUID do admin que definiu o ano letivo.';
COMMENT ON COLUMN projection_sistema_config.observacao IS
    'Observação opcional sobre a definição do ano letivo.';
COMMENT ON COLUMN projection_sistema_config.event_id IS
    'UUID do último evento que atualizou esta linha.';

-- 4. Garantir checkpoint para sistema_config
INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at)
VALUES ('sistema_config', 0, CURRENT_TIMESTAMP)
ON CONFLICT (projection_name) DO NOTHING;

-- 5. Checkpoint para avaliacao_final (se não existir — criado em 016 mas pode ter pulado)
INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at)
VALUES ('avaliacao_final', 0, CURRENT_TIMESTAMP)
ON CONFLICT (projection_name) DO NOTHING;

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 031 — projection_sistema_config: colunas ano_letivo_atual, data_inicio, data_fim, definido_por, observacao, event_id adicionadas.';
    RAISE NOTICE '   Ação recomendada: POST /admin/rebuild-projection/sistema_config';
END $$;
