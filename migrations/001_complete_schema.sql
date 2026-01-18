-- ============================================
-- SPURI EVENT SOURCING - SCHEMA COMPLETO
-- Versão: 2.3.0 (com status e status_usado)
-- ============================================

-- ============================================
-- EXTENSÕES
-- ============================================
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ============================================
-- Spuri LEDGER - Event Store Principal
-- ============================================

CREATE TABLE IF NOT EXISTS spuri_ledger (
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

-- Funções para hash
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

CREATE OR REPLACE FUNCTION auto_generate_ledger_hash()
RETURNS TRIGGER AS $$
DECLARE
    v_previous_hash VARCHAR(64);
BEGIN
    SELECT ledger_hash INTO v_previous_hash
    FROM spuri_ledger
    WHERE aggregate_id = NEW.aggregate_id
    ORDER BY event_version DESC
    LIMIT 1;
    
    NEW.ledger_hash := generate_ledger_hash(
        NEW.event_id,
        NEW.aggregate_id,
        NEW.event_type,
        NEW.payload,
        v_previous_hash
    );
    
    NEW.previous_hash := v_previous_hash;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trigger_generate_ledger_hash ON spuri_ledger;
CREATE TRIGGER trigger_generate_ledger_hash
    BEFORE INSERT ON spuri_ledger
    FOR EACH ROW
    EXECUTE FUNCTION auto_generate_ledger_hash();

-- Prevenir modificações
CREATE OR REPLACE FUNCTION prevent_ledger_modification()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'Spuri Ledger é imutável. Operação % não permitida.', TG_OP;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS prevent_update_ledger ON spuri_ledger;
CREATE TRIGGER prevent_update_ledger
    BEFORE UPDATE ON spuri_ledger
    FOR EACH ROW
    EXECUTE FUNCTION prevent_ledger_modification();

DROP TRIGGER IF EXISTS prevent_delete_ledger ON spuri_ledger;
CREATE TRIGGER prevent_delete_ledger
    BEFORE DELETE ON spuri_ledger
    FOR EACH ROW
    EXECUTE FUNCTION prevent_ledger_modification();

-- Índices
CREATE INDEX IF NOT EXISTS idx_spuri_aggregate ON spuri_ledger(aggregate_id, event_version);
CREATE INDEX IF NOT EXISTS idx_spuri_type ON spuri_ledger(event_type);
CREATE INDEX IF NOT EXISTS idx_spuri_occurred ON spuri_ledger(occurred_at);
CREATE INDEX IF NOT EXISTS idx_spuri_aggregate_type ON spuri_ledger(aggregate_type);
CREATE INDEX IF NOT EXISTS idx_spuri_payload ON spuri_ledger USING GIN (payload);

-- Snapshots
CREATE TABLE IF NOT EXISTS aggregate_snapshots (
    aggregate_id UUID PRIMARY KEY,
    aggregate_type VARCHAR(50) NOT NULL,
    version INTEGER NOT NULL,
    state JSONB NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_snapshots_type ON aggregate_snapshots(aggregate_type);

-- ============================================
-- PROJEÇÕES
-- ============================================

-- Projeção: Estudantes
-- 🔥 ATUALIZADO: codigo_academia VARCHAR(50) + status + constraints
CREATE TABLE IF NOT EXISTS projection_estudantes (
    id UUID PRIMARY KEY,
    nome VARCHAR(255) NOT NULL,
    codigo_estudante VARCHAR(7) UNIQUE NOT NULL,
    senha_hash VARCHAR(255) NOT NULL,
    bilhete_identidade VARCHAR(50),
    bilhete_identidade_responsavel VARCHAR(50),
    codigo_academia VARCHAR(50),
    ano_escolar VARCHAR(50),
    ano_superior VARCHAR(50),
    curso_medio VARCHAR(255),
    curso_superior VARCHAR(255),
    status VARCHAR(20) DEFAULT 'inativo' CHECK (status IN ('inativo', 'ativo', 'finalizado')),
    status_escolar VARCHAR(20) DEFAULT 'inativo' CHECK (status_escolar IN ('inativo', 'em_andamento', 'finalizado')),
    status_superior VARCHAR(20) DEFAULT 'inativo' CHECK (status_superior IN ('inativo', 'em_andamento', 'finalizado')),
    version INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_event_id UUID,
    total_notas INTEGER DEFAULT 0,
    total_faltas INTEGER DEFAULT 0,
    total_inscricoes INTEGER DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_proj_estudante_codigo ON projection_estudantes(codigo_estudante);
CREATE INDEX IF NOT EXISTS idx_proj_estudante_codigo_academia ON projection_estudantes(codigo_academia);
CREATE INDEX IF NOT EXISTS idx_proj_estudante_bilhete ON projection_estudantes(bilhete_identidade);
CREATE INDEX IF NOT EXISTS idx_proj_estudante_bilhete_resp ON projection_estudantes(bilhete_identidade_responsavel);
CREATE INDEX IF NOT EXISTS idx_proj_estudante_status ON projection_estudantes(status);

-- Projeção: Academias
CREATE TABLE IF NOT EXISTS projection_academias (
    id UUID PRIMARY KEY,
    type VARCHAR(20) NOT NULL,
    nome VARCHAR(255) NOT NULL,
    codigo_academia VARCHAR(50) UNIQUE NOT NULL,
    senha_hash VARCHAR(255) NOT NULL,
    provincia VARCHAR(3) NOT NULL,
    endereco TEXT NOT NULL,
    numero_telefone VARCHAR(20),
    email VARCHAR(100),
    website VARCHAR(255),
    nivel_escolar VARCHAR(20),
    status VARCHAR(20) DEFAULT 'inativo' CHECK (status IN ('ativo', 'inativo')),
    cursos JSONB DEFAULT '[]',
    version INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_event_id UUID,
    total_estudantes INTEGER DEFAULT 0,
    total_inscricoes_pendentes INTEGER DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_proj_academia_provincia ON projection_academias(provincia);
CREATE INDEX IF NOT EXISTS idx_proj_academia_codigo ON projection_academias(codigo_academia);
CREATE INDEX IF NOT EXISTS idx_proj_academia_type ON projection_academias(type);
CREATE INDEX IF NOT EXISTS idx_proj_academia_status ON projection_academias(status);

-- Projeção: Admins
CREATE TABLE IF NOT EXISTS projection_admins (
    id UUID PRIMARY KEY,
    nome VARCHAR(255) NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    senha_hash VARCHAR(255) NOT NULL,
    role VARCHAR(20) NOT NULL CHECK (role IN ('fpp', 'adm', 'gerente')),
    status VARCHAR(20) DEFAULT 'ativo' CHECK (status IN ('ativo', 'inativo')),
    created_by UUID REFERENCES projection_admins(id) ON DELETE SET NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    version INTEGER NOT NULL DEFAULT 0,
    last_event_id UUID,
    total_acoes_realizadas INTEGER DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_proj_admins_email ON projection_admins(email);
CREATE INDEX IF NOT EXISTS idx_proj_admins_role ON projection_admins(role);
CREATE INDEX IF NOT EXISTS idx_proj_admins_status ON projection_admins(status);
CREATE INDEX IF NOT EXISTS idx_proj_admins_created_by ON projection_admins(created_by);

-- Projeção: Notas
CREATE TABLE IF NOT EXISTS projection_notas (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    estudante_id UUID NOT NULL,
    codigo_academia VARCHAR(50) NOT NULL,
    ano_lectivo VARCHAR(20) NOT NULL,
    periodo VARCHAR(50) NOT NULL,
    materias JSONB NOT NULL,
    registered_at TIMESTAMP NOT NULL,
    event_id UUID NOT NULL,
    version INTEGER NOT NULL,
    FOREIGN KEY (estudante_id) REFERENCES projection_estudantes(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_proj_notas_estudante ON projection_notas(estudante_id);
CREATE INDEX IF NOT EXISTS idx_proj_notas_codigo_academia ON projection_notas(codigo_academia);
CREATE INDEX IF NOT EXISTS idx_proj_notas_ano ON projection_notas(ano_lectivo);

-- Projeção: Faltas
CREATE TABLE IF NOT EXISTS projection_faltas (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    estudante_id UUID NOT NULL,
    codigo_academia VARCHAR(50) NOT NULL,
    ano_lectivo VARCHAR(20) NOT NULL,
    periodo VARCHAR(50) NOT NULL,
    materias JSONB NOT NULL,
    registered_at TIMESTAMP NOT NULL,
    event_id UUID NOT NULL,
    version INTEGER NOT NULL,
    FOREIGN KEY (estudante_id) REFERENCES projection_estudantes(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_proj_faltas_estudante ON projection_faltas(estudante_id);
CREATE INDEX IF NOT EXISTS idx_proj_faltas_codigo_academia ON projection_faltas(codigo_academia);
CREATE INDEX IF NOT EXISTS idx_proj_faltas_ano ON projection_faltas(ano_lectivo);

-- Projeção: Inscrições
-- 🔥 ATUALIZADO: campo status_usado adicionado
CREATE TABLE IF NOT EXISTS projection_inscricoes (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    estudante_id UUID NOT NULL,
    codigo_estudante VARCHAR(7),
    academia_id UUID NOT NULL,
    codigo_academia VARCHAR(50),
    tipo VARCHAR(20) NOT NULL,
    ano_inscricao VARCHAR(50) NOT NULL,
    curso VARCHAR(255),
    status VARCHAR(20) NOT NULL CHECK (status IN ('espera', 'aprovado', 'reprovado')),
    status_usado BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    event_id UUID NOT NULL,
    version INTEGER NOT NULL,
    FOREIGN KEY (estudante_id) REFERENCES projection_estudantes(id) ON DELETE CASCADE,
    FOREIGN KEY (academia_id) REFERENCES projection_academias(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_proj_inscricoes_estudante ON projection_inscricoes(estudante_id);
CREATE INDEX IF NOT EXISTS idx_proj_inscricoes_academia ON projection_inscricoes(academia_id);
CREATE INDEX IF NOT EXISTS idx_proj_inscricoes_status ON projection_inscricoes(status);
CREATE INDEX IF NOT EXISTS idx_proj_inscricoes_codigo_estudante ON projection_inscricoes(codigo_estudante);
CREATE INDEX IF NOT EXISTS idx_proj_inscricoes_codigo_academia ON projection_inscricoes(codigo_academia);
CREATE INDEX IF NOT EXISTS idx_proj_inscricoes_status_usado ON projection_inscricoes(status_usado);

-- ============================================
-- LOG DE AÇÕES ADMINISTRATIVAS
-- ============================================

CREATE TABLE IF NOT EXISTS admin_action_log (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    admin_id UUID NOT NULL REFERENCES projection_admins(id) ON DELETE CASCADE,
    acao VARCHAR(100) NOT NULL,
    detalhes JSONB,
    target_type VARCHAR(50),
    target_id UUID,
    ip_address VARCHAR(45),
    user_agent TEXT,
    performed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_admin_log_admin ON admin_action_log(admin_id);
CREATE INDEX IF NOT EXISTS idx_admin_log_acao ON admin_action_log(acao);
CREATE INDEX IF NOT EXISTS idx_admin_log_target ON admin_action_log(target_type, target_id);
CREATE INDEX IF NOT EXISTS idx_admin_log_performed ON admin_action_log(performed_at DESC);

-- ============================================
-- CHECKPOINTS
-- ============================================

CREATE TABLE IF NOT EXISTS projection_checkpoints (
    projection_name VARCHAR(100) PRIMARY KEY,
    last_processed_event_id BIGINT NOT NULL DEFAULT 0,
    last_processed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    events_processed BIGINT DEFAULT 0,
    is_rebuilding BOOLEAN DEFAULT FALSE,
    rebuild_started_at TIMESTAMP,
    error_count INTEGER DEFAULT 0,
    last_error TEXT,
    last_error_at TIMESTAMP
);

INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at) 
VALUES
    ('estudantes', 0, CURRENT_TIMESTAMP),
    ('academias', 0, CURRENT_TIMESTAMP),
    ('admins', 0, CURRENT_TIMESTAMP),
    ('notas', 0, CURRENT_TIMESTAMP),
    ('faltas', 0, CURRENT_TIMESTAMP),
    ('inscricoes', 0, CURRENT_TIMESTAMP)
ON CONFLICT (projection_name) DO NOTHING;

-- ============================================
-- FUNÇÕES AUXILIARES
-- ============================================

-- Gerar código estudante
CREATE OR REPLACE FUNCTION generate_codigo_estudante()
RETURNS VARCHAR AS $$
DECLARE
    v_codigo VARCHAR(7);
    v_exists BOOLEAN;
    v_letters TEXT := 'ABCDEFGHIJKLMNOPQRSTUVWXYZ';
    v_counter INTEGER := 0;
BEGIN
    LOOP
        v_codigo := 
            substring(v_letters from (floor(random() * 26) + 1)::int for 1) ||
            substring(v_letters from (floor(random() * 26) + 1)::int for 1) ||
            substring(v_letters from (floor(random() * 26) + 1)::int for 1);
        
        v_codigo := v_codigo || lpad(floor(random() * 10000)::text, 4, '0');
        
        SELECT EXISTS(
            SELECT 1 FROM projection_estudantes WHERE codigo_estudante = v_codigo
        ) INTO v_exists;
        
        IF NOT v_exists THEN
            RETURN v_codigo;
        END IF;
        
        v_counter := v_counter + 1;
        IF v_counter > 100 THEN
            RAISE EXCEPTION 'Não foi possível gerar código único após 100 tentativas';
        END IF;
    END LOOP;
END;
$$ LANGUAGE plpgsql VOLATILE;

-- Registrar ação administrativa
CREATE OR REPLACE FUNCTION registrar_acao_admin(
    p_admin_id UUID,
    p_acao VARCHAR(100),
    p_detalhes JSONB DEFAULT NULL,
    p_target_type VARCHAR(50) DEFAULT NULL,
    p_target_id UUID DEFAULT NULL
) RETURNS UUID AS $$
DECLARE
    v_log_id UUID;
BEGIN
    v_log_id := uuid_generate_v4();
    
    INSERT INTO admin_action_log (
        id, admin_id, acao, detalhes,
        target_type, target_id, performed_at
    ) VALUES (
        v_log_id, p_admin_id, p_acao, p_detalhes,
        p_target_type, p_target_id, CURRENT_TIMESTAMP
    );
    
    RETURN v_log_id;
END;
$$ LANGUAGE plpgsql;

-- Verificar hash chain
CREATE OR REPLACE FUNCTION verify_hash_chain(p_aggregate_id UUID)
RETURNS TABLE(
    is_valid BOOLEAN,
    broken_at_version INTEGER,
    message TEXT
) AS $$
DECLARE
    v_current_hash VARCHAR(64);
    v_expected_hash VARCHAR(64);
    v_event RECORD;
BEGIN
    FOR v_event IN 
        SELECT * FROM spuri_ledger 
        WHERE aggregate_id = p_aggregate_id 
        ORDER BY event_version ASC
    LOOP
        v_expected_hash := generate_ledger_hash(
            v_event.event_id,
            v_event.aggregate_id,
            v_event.event_type,
            v_event.payload,
            v_event.previous_hash
        );
        
        IF v_event.ledger_hash != v_expected_hash THEN
            is_valid := FALSE;
            broken_at_version := v_event.event_version;
            message := 'Hash inválido detectado';
            RETURN NEXT;
            RETURN;
        END IF;
        
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

-- ============================================
-- TRIGGERS DE ATUALIZAÇÃO
-- ============================================

CREATE OR REPLACE FUNCTION update_projection_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trigger_update_estudante_timestamp ON projection_estudantes;
CREATE TRIGGER trigger_update_estudante_timestamp
    BEFORE UPDATE ON projection_estudantes
    FOR EACH ROW EXECUTE FUNCTION update_projection_timestamp();

DROP TRIGGER IF EXISTS trigger_update_academia_timestamp ON projection_academias;
CREATE TRIGGER trigger_update_academia_timestamp
    BEFORE UPDATE ON projection_academias
    FOR EACH ROW EXECUTE FUNCTION update_projection_timestamp();

DROP TRIGGER IF EXISTS trigger_update_admin_timestamp ON projection_admins;
CREATE TRIGGER trigger_update_admin_timestamp
    BEFORE UPDATE ON projection_admins
    FOR EACH ROW EXECUTE FUNCTION update_projection_timestamp();

DROP TRIGGER IF EXISTS trigger_update_inscricao_timestamp ON projection_inscricoes;
CREATE TRIGGER trigger_update_inscricao_timestamp
    BEFORE UPDATE ON projection_inscricoes
    FOR EACH ROW EXECUTE FUNCTION update_projection_timestamp();

-- ============================================
-- VIEWS
-- ============================================

CREATE OR REPLACE VIEW v_estudante_completo AS
SELECT 
    e.*,
    (SELECT json_agg(n.*) FROM projection_notas n WHERE n.estudante_id = e.id) as notas,
    (SELECT json_agg(f.*) FROM projection_faltas f WHERE f.estudante_id = e.id) as faltas,
    (SELECT json_agg(i.*) FROM projection_inscricoes i WHERE i.estudante_id = e.id) as inscricoes
FROM projection_estudantes e;

CREATE OR REPLACE VIEW v_admin_actions_recent AS
SELECT 
    l.id,
    l.acao,
    l.detalhes,
    l.target_type,
    l.target_id,
    l.performed_at,
    a.nome as admin_nome,
    a.email as admin_email,
    a.role as admin_role
FROM admin_action_log l
JOIN projection_admins a ON l.admin_id = a.id
ORDER BY l.performed_at DESC
LIMIT 100;

-- ============================================
-- LOG INICIAL DO SISTEMA
-- ============================================

INSERT INTO spuri_ledger (
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
    jsonb_build_object(
        'version', '2.3.0',
        'name', 'Spuri Event Sourcing',
        'features', json_build_array(
            'event_sourcing',
            'cqrs',
            'codigo_estudante',
            'codigo_academia',
            'admin_system',
            'role_hierarchy',
            'status_validation',
            'inscricao_vinculacao'
        )
    ),
    jsonb_build_object('created_by', 'migration_completa_v2.3'),
    CURRENT_TIMESTAMP
)
ON CONFLICT (aggregate_id, event_version) DO NOTHING;

-- ============================================
-- COMENTÁRIOS
-- ============================================

COMMENT ON TABLE spuri_ledger IS 'Event Store principal - Imutável com hash chain';
COMMENT ON TABLE projection_estudantes IS 'Projeção de leitura para estudantes';
COMMENT ON TABLE projection_academias IS 'Projeção de leitura para academias';
COMMENT ON TABLE projection_admins IS 'Projeção de leitura para administradores';
COMMENT ON TABLE admin_action_log IS 'Log de todas as ações administrativas';

COMMENT ON COLUMN projection_estudantes.codigo_estudante IS 'Código único do estudante (formato: AAA1234)';
COMMENT ON COLUMN projection_estudantes.codigo_academia IS 'Código da academia à qual o estudante pertence';
COMMENT ON COLUMN projection_estudantes.status IS 'Status geral: inativo (sem vínculo), ativo (vinculado), finalizado';
COMMENT ON COLUMN projection_estudantes.status_escolar IS 'Status ensino médio: inativo, em_andamento, finalizado';
COMMENT ON COLUMN projection_estudantes.status_superior IS 'Status ensino superior: inativo, em_andamento, finalizado';

COMMENT ON COLUMN projection_academias.status IS 'Status da academia: inativo (ao criar), ativo (aprovada por admin)';

COMMENT ON COLUMN projection_inscricoes.status_usado IS 'Se esta inscrição já foi usada para vincular o estudante';

COMMENT ON COLUMN projection_notas.codigo_academia IS 'Código da academia que registrou as notas';
COMMENT ON COLUMN projection_faltas.codigo_academia IS 'Código da academia que registrou as faltas';
COMMENT ON COLUMN projection_admins.role IS 'Hierarquia: fpp > adm > gerente';

COMMENT ON FUNCTION generate_codigo_estudante IS 'Gera código único AAA1234 para estudantes';
COMMENT ON FUNCTION registrar_acao_admin IS 'Registra uma ação administrativa no log';
COMMENT ON FUNCTION verify_hash_chain IS 'Verifica integridade da cadeia de hashes de um agregado';

-- ============================================
-- VERIFICAÇÃO FINAL
-- ============================================

DO $$
DECLARE
    v_total_eventos BIGINT;
    v_total_checkpoints INT;
    v_total_admins INT;
BEGIN
    SELECT COUNT(*) INTO v_total_eventos FROM spuri_ledger;
    SELECT COUNT(*) INTO v_total_checkpoints FROM projection_checkpoints;
    SELECT COUNT(*) INTO v_total_admins FROM projection_admins;
    
    RAISE NOTICE '╔═══════════════════════════════════╗';
    RAISE NOTICE '║ SCHEMA CRIADO COM SUCESSO! v2.3.0 ║';
    RAISE NOTICE '╚═══════════════════════════════════╝';
    RAISE NOTICE 'Total de eventos: %', v_total_eventos;
    RAISE NOTICE 'Total de checkpoints: %', v_total_checkpoints;
    RAISE NOTICE 'Total de admins: %', v_total_admins;
    RAISE NOTICE '🔥 NOVIDADES v2.3.0:';
    RAISE NOTICE '   ✅ Campo status em estudantes';
    RAISE NOTICE '   ✅ Campo status_usado em inscrições';
    RAISE NOTICE '   ✅ Academia inicia com status inativo';
    RAISE NOTICE '   ✅ Constraints de validação de status';
    RAISE NOTICE '╚═══════════════════════════════════╝';
END $$;