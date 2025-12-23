-- ============================================
-- GenesisDB - Event Sourcing Ledger
-- Imutável, auditável, com hash chain
-- ============================================

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ============================================
-- GENESIS LEDGER - Event Store Principal
-- ============================================

CREATE TABLE genesis_ledger (
    -- Identificação
    id BIGSERIAL PRIMARY KEY,
    event_id UUID UNIQUE NOT NULL DEFAULT uuid_generate_v4(),
    
    -- Agregado
    aggregate_id UUID NOT NULL,
    aggregate_type VARCHAR(50) NOT NULL,
    
    -- Evento
    event_type VARCHAR(100) NOT NULL,
    event_version INTEGER NOT NULL,
    
    -- Dados
    payload JSONB NOT NULL,
    metadata JSONB DEFAULT '{}',
    
    -- Timestamps
    occurred_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    recorded_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    
    -- Hash Chain (imutabilidade garantida)
    ledger_hash VARCHAR(64) NOT NULL,
    previous_hash VARCHAR(64),
    
    -- Constraint: versão única por agregado
    UNIQUE(aggregate_id, event_version)
);

-- ============================================
-- FUNÇÃO PARA GERAR HASH
-- ============================================

CREATE OR REPLACE FUNCTION generate_ledger_hash(
    p_event_id UUID,
    p_aggregate_id UUID,
    p_event_type VARCHAR,
    p_payload JSONB,
    p_previous_hash VARCHAR
) RETURNS VARCHAR AS $$
BEGIN
    RETURN encode(
        digest(
            p_event_id::text || 
            p_aggregate_id::text || 
            p_event_type || 
            p_payload::text || 
            COALESCE(p_previous_hash, ''),
            'sha256'
        ),
        'hex'
    );
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- ============================================
-- TRIGGER PARA AUTO-GERAR HASH
-- ============================================

CREATE OR REPLACE FUNCTION auto_generate_ledger_hash()
RETURNS TRIGGER AS $$
DECLARE
    v_previous_hash VARCHAR(64);
BEGIN
    -- Obter hash anterior do mesmo agregado
    SELECT ledger_hash INTO v_previous_hash
    FROM genesis_ledger
    WHERE aggregate_id = NEW.aggregate_id
    ORDER BY event_version DESC
    LIMIT 1;
    
    -- Gerar hash do novo evento
    NEW.ledger_hash := generate_ledger_hash(
        NEW.event_id,
        NEW.aggregate_id,
        NEW.event_type,
        NEW.payload,
        v_previous_hash
    );
    
    -- Armazenar hash anterior
    NEW.previous_hash := v_previous_hash;
    
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_generate_ledger_hash
    BEFORE INSERT ON genesis_ledger
    FOR EACH ROW
    EXECUTE FUNCTION auto_generate_ledger_hash();

-- ============================================
-- TRIGGER PARA PREVENIR UPDATES/DELETES
-- ============================================

CREATE OR REPLACE FUNCTION prevent_ledger_modification()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'Genesis Ledger é imutável. Operação % não permitida.', TG_OP;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER prevent_update_ledger
    BEFORE UPDATE ON genesis_ledger
    FOR EACH ROW
    EXECUTE FUNCTION prevent_ledger_modification();

CREATE TRIGGER prevent_delete_ledger
    BEFORE DELETE ON genesis_ledger
    FOR EACH ROW
    EXECUTE FUNCTION prevent_ledger_modification();

-- ============================================
-- ÍNDICES PARA PERFORMANCE
-- ============================================

CREATE INDEX idx_genesis_aggregate ON genesis_ledger(aggregate_id, event_version);
CREATE INDEX idx_genesis_type ON genesis_ledger(event_type);
CREATE INDEX idx_genesis_occurred ON genesis_ledger(occurred_at);
CREATE INDEX idx_genesis_aggregate_type ON genesis_ledger(aggregate_type);

-- Índice GIN para busca rápida em payload JSONB
CREATE INDEX idx_genesis_payload ON genesis_ledger USING GIN (payload);

-- ============================================
-- TABELA DE SNAPSHOTS (Otimização)
-- ============================================

CREATE TABLE aggregate_snapshots (
    aggregate_id UUID PRIMARY KEY,
    aggregate_type VARCHAR(50) NOT NULL,
    version INTEGER NOT NULL,
    state JSONB NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_snapshots_type ON aggregate_snapshots(aggregate_type);

-- Trigger para atualizar updated_at
CREATE OR REPLACE FUNCTION update_snapshot_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_update_snapshot_timestamp
    BEFORE UPDATE ON aggregate_snapshots
    FOR EACH ROW
    EXECUTE FUNCTION update_snapshot_timestamp();

-- ============================================
-- VIEWS DE AUDITORIA
-- ============================================

-- View de auditoria completa
CREATE VIEW v_event_audit AS
SELECT 
    event_id,
    aggregate_id,
    aggregate_type,
    event_type,
    event_version,
    occurred_at,
    recorded_at,
    payload,
    metadata,
    ledger_hash,
    previous_hash
FROM genesis_ledger
ORDER BY recorded_at DESC;

-- View de timeline por agregado
CREATE VIEW v_aggregate_timeline AS
SELECT 
    aggregate_id,
    aggregate_type,
    COUNT(*) as total_events,
    MIN(occurred_at) as first_event,
    MAX(occurred_at) as last_event,
    MAX(event_version) as current_version
FROM genesis_ledger
GROUP BY aggregate_id, aggregate_type;

-- View de estatísticas por tipo de evento
CREATE VIEW v_event_statistics AS
SELECT 
    event_type,
    COUNT(*) as total,
    MIN(occurred_at) as first_occurrence,
    MAX(occurred_at) as last_occurrence
FROM genesis_ledger
GROUP BY event_type
ORDER BY total DESC;

-- ============================================
-- FUNÇÕES DE VERIFICAÇÃO
-- ============================================

-- Verificar integridade da cadeia de hashes
CREATE OR REPLACE FUNCTION verify_hash_chain(p_aggregate_id UUID)
RETURNS TABLE(
    is_valid BOOLEAN,
    broken_at_version INTEGER,
    message TEXT
) AS $$
DECLARE
    v_current_hash VARCHAR(64);
    v_previous_hash VARCHAR(64);
    v_expected_hash VARCHAR(64);
    v_event RECORD;
BEGIN
    FOR v_event IN 
        SELECT * FROM genesis_ledger 
        WHERE aggregate_id = p_aggregate_id 
        ORDER BY event_version ASC
    LOOP
        -- Calcular hash esperado
        v_expected_hash := generate_ledger_hash(
            v_event.event_id,
            v_event.aggregate_id,
            v_event.event_type,
            v_event.payload,
            v_event.previous_hash
        );
        
        -- Verificar se hash bate
        IF v_event.ledger_hash != v_expected_hash THEN
            is_valid := FALSE;
            broken_at_version := v_event.event_version;
            message := 'Hash inválido detectado';
            RETURN NEXT;
            RETURN;
        END IF;
        
        -- Verificar se previous_hash bate com hash anterior
        IF v_event.event_version > 1 AND v_event.previous_hash != v_current_hash THEN
            is_valid := FALSE;
            broken_at_version := v_event.event_version;
            message := 'Cadeia de hashes quebrada';
            RETURN NEXT;
            RETURN;
        END IF;
        
        v_current_hash := v_event.ledger_hash;
    END LOOP;
    
    is_valid := TRUE;
    broken_at_version := NULL;
    message := 'Cadeia de hashes íntegra';
    RETURN NEXT;
END;
$$ LANGUAGE plpgsql;

-- Estatísticas do ledger
CREATE OR REPLACE FUNCTION ledger_stats()
RETURNS TABLE(
    total_events BIGINT,
    total_aggregates BIGINT,
    oldest_event TIMESTAMP,
    newest_event TIMESTAMP,
    avg_events_per_aggregate NUMERIC
) AS $$
BEGIN
    RETURN QUERY
    SELECT 
        COUNT(*)::BIGINT,
        COUNT(DISTINCT aggregate_id)::BIGINT,
        MIN(occurred_at),
        MAX(occurred_at),
        ROUND(COUNT(*)::NUMERIC / NULLIF(COUNT(DISTINCT aggregate_id), 0), 2)
    FROM genesis_ledger;
END;
$$ LANGUAGE plpgsql;

-- ============================================
-- COMENTÁRIOS
-- ============================================

COMMENT ON TABLE genesis_ledger IS 
'Event Store principal do sistema. Imutável, auditável, com hash chain para garantir integridade.';

COMMENT ON COLUMN genesis_ledger.ledger_hash IS 
'Hash SHA-256 do evento, garantindo imutabilidade.';

COMMENT ON COLUMN genesis_ledger.previous_hash IS 
'Hash do evento anterior, formando uma blockchain interna.';

COMMENT ON COLUMN genesis_ledger.event_version IS 
'Versão do evento dentro do agregado. Garante ordenação.';

COMMENT ON FUNCTION verify_hash_chain IS 
'Verifica a integridade da cadeia de hashes de um agregado.';

-- ============================================
-- DADOS INICIAIS
-- ============================================

-- Log de criação do schema
INSERT INTO genesis_ledger (
    aggregate_id,
    aggregate_type,
    event_type,
    event_version,
    payload,
    metadata,
    occurred_at
) VALUES (
    uuid_generate_v4(),
    'System',
    'SchemaCreated',
    1,
    '{"version": "1.0.0", "name": "GenesisDB Event Sourcing"}'::jsonb,
    '{"created_by": "migration"}'::jsonb,
    CURRENT_TIMESTAMP
);