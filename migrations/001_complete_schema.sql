-- Migration unificada gerada a partir de todas as migrations existentes
-- Ordem de aplicação preservada (sort lexicográfico dos arquivos).

-- ===== BEGIN 001_complete_schema.sql =====
-- ============================================
-- SPURI EVENT SOURCING - SCHEMA COMPLETO
-- Versão: 3.0.0 (Nova Estrutura Notas/Faltas)
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
CREATE TABLE IF NOT EXISTS projection_estudantes (
    id UUID PRIMARY KEY,
    nome VARCHAR(255) NOT NULL,
    codigo_estudante VARCHAR(7) UNIQUE,
    senha_hash VARCHAR(255) NOT NULL,
    email VARCHAR(255),
    telefone VARCHAR(20),
    email_verificado BOOLEAN DEFAULT FALSE,
    bilhete_identidade VARCHAR(50),
    bilhete_identidade_responsavel VARCHAR(50),
    codigo_academia VARCHAR(50),
    status VARCHAR(20) DEFAULT 'inativo' CHECK (status IN ('inativo', 'ativo', 'finalizado')),
    ano_escolar VARCHAR(50),
    ano_superior VARCHAR(50),
    curso_medio VARCHAR(255),
    curso_superior VARCHAR(255),
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
CREATE INDEX IF NOT EXISTS idx_proj_estudante_email ON projection_estudantes(email);
CREATE INDEX IF NOT EXISTS idx_proj_estudante_codigo_academia ON projection_estudantes(codigo_academia);
CREATE INDEX IF NOT EXISTS idx_proj_estudante_bilhete ON projection_estudantes(bilhete_identidade);
CREATE INDEX IF NOT EXISTS idx_proj_estudante_bilhete_resp ON projection_estudantes(bilhete_identidade_responsavel);
CREATE INDEX IF NOT EXISTS idx_proj_estudante_status ON projection_estudantes(status);

-- Projeção: Academias
CREATE TABLE IF NOT EXISTS projection_academias (
    id UUID PRIMARY KEY,
    nivel VARCHAR(20) NOT NULL CHECK (nivel IN ('escola', 'superior')),
    nome VARCHAR(255) NOT NULL,
    codigo_academia VARCHAR(50) UNIQUE NOT NULL,
    senha_hash VARCHAR(255) NOT NULL,
    provincia VARCHAR(3) NOT NULL,
    endereco TEXT NOT NULL,
    numero_telefone VARCHAR(20),
    email VARCHAR(100),
    email_verificado BOOLEAN DEFAULT FALSE,
    website VARCHAR(255),
    nivel_escolar VARCHAR(20) CHECK (nivel_escolar IN ('fundamental', 'medio', 'misto')),
    status VARCHAR(20) DEFAULT 'inativo' CHECK (status IN ('ativo', 'inativo')),
    cursos JSONB DEFAULT '[]',
    version INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_event_id UUID,
    total_estudantes INTEGER DEFAULT 0,
    total_inscricoes_pendentes INTEGER DEFAULT 0,
    
    -- Constraint: nivel_escolar obrigatório para escolas, NULL para superior
    CONSTRAINT check_nivel_escolar_tipo CHECK (
        (nivel = 'escola' AND nivel_escolar IN ('fundamental', 'medio', 'misto')) 
        OR 
        (nivel = 'superior' AND nivel_escolar IS NULL)
    )
);

CREATE INDEX IF NOT EXISTS idx_proj_academia_provincia ON projection_academias(provincia);
CREATE INDEX IF NOT EXISTS idx_proj_academia_codigo ON projection_academias(codigo_academia);
CREATE INDEX IF NOT EXISTS idx_proj_academia_email ON projection_academias(email);
CREATE INDEX IF NOT EXISTS idx_proj_academia_nivel_tipo ON projection_academias(nivel);
CREATE INDEX IF NOT EXISTS idx_proj_academia_status ON projection_academias(status);
CREATE INDEX IF NOT EXISTS idx_proj_academia_nivel ON projection_academias(nivel_escolar);

-- Projeção: Admins
CREATE TABLE IF NOT EXISTS projection_admins (
    id UUID PRIMARY KEY,
    nome VARCHAR(255) NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    senha_hash VARCHAR(255) NOT NULL,
    role VARCHAR(20) NOT NULL CHECK (role IN ('fpp', 'adm', 'gerente')),
    status VARCHAR(20) DEFAULT 'ativo' CHECK (status IN ('ativo', 'inativo')),
    email_verificado BOOLEAN DEFAULT FALSE,
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

-- Projeção: Cursos
CREATE TABLE IF NOT EXISTS projection_cursos (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    nome VARCHAR(255) NOT NULL,
    type VARCHAR(20) NOT NULL CHECK (type IN ('medio', 'superior')),
    nivel JSONB NOT NULL,
    codigo_academia VARCHAR(50) NOT NULL,
    status VARCHAR(20) DEFAULT 'ativo' CHECK (status IN ('ativo', 'inativo')),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    version INTEGER NOT NULL DEFAULT 0,
    last_event_id UUID,
    
    FOREIGN KEY (codigo_academia) 
        REFERENCES projection_academias(codigo_academia) 
        ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_cursos_academia ON projection_cursos(codigo_academia);
CREATE INDEX IF NOT EXISTS idx_cursos_type ON projection_cursos(type);
CREATE INDEX IF NOT EXISTS idx_cursos_status ON projection_cursos(status);

-- Projeção: Matérias
CREATE TABLE IF NOT EXISTS projection_materias (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    nome VARCHAR(255) NOT NULL,
    type VARCHAR(20) NOT NULL CHECK (type IN ('fundamental', 'medio', 'superior')),
    nivel JSONB,
    codigo_academia VARCHAR(50) NOT NULL,
    curso_id UUID,
    status VARCHAR(20) DEFAULT 'ativo' CHECK (status IN ('ativo', 'inativo')),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    version INTEGER NOT NULL DEFAULT 0,
    last_event_id UUID,
    
    FOREIGN KEY (codigo_academia) 
        REFERENCES projection_academias(codigo_academia) 
        ON DELETE CASCADE,
    FOREIGN KEY (curso_id) 
        REFERENCES projection_cursos(id) 
        ON DELETE CASCADE,
    
    CONSTRAINT check_fundamental_sem_curso CHECK (
        (type = 'fundamental' AND curso_id IS NULL) OR
        (type IN ('medio', 'superior') AND curso_id IS NOT NULL)
    )
);

CREATE INDEX IF NOT EXISTS idx_materias_academia ON projection_materias(codigo_academia);
CREATE INDEX IF NOT EXISTS idx_materias_curso ON projection_materias(curso_id);
CREATE INDEX IF NOT EXISTS idx_materias_type ON projection_materias(type);
CREATE INDEX IF NOT EXISTS idx_materias_status ON projection_materias(status);

-- ============================================
-- 🔥 NOVA ESTRUTURA - NOTAS (v3.0)
-- ============================================

CREATE TABLE IF NOT EXISTS projection_notas (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    
    -- Identificadores
    codigo_estudante VARCHAR(7) NOT NULL,
    codigo_academia VARCHAR(50) NOT NULL,
    
    -- Período Acadêmico
    ano_lectivo VARCHAR(20) NOT NULL,
    periodo VARCHAR(20) NOT NULL CHECK (periodo IN (
        '1_trimestre', '2_trimestre', '3_trimestre',
        '1_semestre', '2_semestre'
    )),
    
    -- Matéria e Nota
    materia_disciplinar_id UUID NOT NULL,
    nota DECIMAL(5,2) NOT NULL CHECK (nota >= 0 AND nota <= 20),
    observacao TEXT,
    
    -- Metadados
    registered_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    event_id UUID NOT NULL,
    version INTEGER NOT NULL,
    
    -- Foreign Keys
    FOREIGN KEY (materia_disciplinar_id) 
        REFERENCES projection_materias(id) 
        ON DELETE CASCADE,
    
    -- Constraint: Única nota por estudante/academia/período/matéria
    UNIQUE(codigo_estudante, codigo_academia, ano_lectivo, periodo, materia_disciplinar_id)
);

CREATE INDEX IF NOT EXISTS idx_notas_estudante ON projection_notas(codigo_estudante);
CREATE INDEX IF NOT EXISTS idx_notas_academia ON projection_notas(codigo_academia);
CREATE INDEX IF NOT EXISTS idx_notas_materia ON projection_notas(materia_disciplinar_id);
CREATE INDEX IF NOT EXISTS idx_notas_periodo ON projection_notas(ano_lectivo, periodo);
CREATE INDEX IF NOT EXISTS idx_notas_registered ON projection_notas(registered_at DESC);

-- ============================================
-- 🔥 NOVA ESTRUTURA - FALTAS (v3.0)
-- ============================================

CREATE TABLE IF NOT EXISTS projection_faltas (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    
    -- Identificadores
    codigo_estudante VARCHAR(7) NOT NULL,
    codigo_academia VARCHAR(50) NOT NULL,
    
    -- Período Acadêmico
    ano_lectivo VARCHAR(20) NOT NULL,
    data DATE NOT NULL,
    
    -- Matéria e Quantidade
    materia_disciplinar_id UUID NOT NULL,
    quantidade INTEGER NOT NULL DEFAULT 1 CHECK (quantidade > 0),
    observacao TEXT,
    
    -- Metadados
    registered_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    event_id UUID NOT NULL,
    version INTEGER NOT NULL,
    
    -- Foreign Keys
    FOREIGN KEY (materia_disciplinar_id) 
        REFERENCES projection_materias(id) 
        ON DELETE CASCADE,
    
    -- Constraint: Única falta por estudante/academia/data/matéria
    UNIQUE(codigo_estudante, codigo_academia, data, materia_disciplinar_id)
);

CREATE INDEX IF NOT EXISTS idx_faltas_estudante ON projection_faltas(codigo_estudante);
CREATE INDEX IF NOT EXISTS idx_faltas_academia ON projection_faltas(codigo_academia);
CREATE INDEX IF NOT EXISTS idx_faltas_materia ON projection_faltas(materia_disciplinar_id);
CREATE INDEX IF NOT EXISTS idx_faltas_data ON projection_faltas(data DESC);
CREATE INDEX IF NOT EXISTS idx_faltas_ano ON projection_faltas(ano_lectivo);
CREATE INDEX IF NOT EXISTS idx_faltas_registered ON projection_faltas(registered_at DESC);

-- Projeção: Inscrições
CREATE TABLE IF NOT EXISTS projection_inscricoes (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    estudante_id UUID NOT NULL,
    codigo_estudante VARCHAR(7),
    academia_id UUID NOT NULL,
    codigo_academia VARCHAR(50),
    tipo VARCHAR(20) NOT NULL CHECK (tipo IN ('escola', 'universidade')),
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

-- ============================================
-- TOKENS DE AUTENTICAÇÃO
-- ============================================

CREATE TABLE IF NOT EXISTS auth_tokens (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL,
    user_type VARCHAR(20) NOT NULL CHECK (user_type IN ('estudante', 'academia', 'admin')),
    token VARCHAR(64) UNIQUE NOT NULL,
    tipo VARCHAR(20) NOT NULL CHECK (tipo IN ('verificacao_email', 'recuperacao_senha')),
    email VARCHAR(255) NOT NULL,
    usado BOOLEAN DEFAULT FALSE,
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    usado_em TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_tokens_token ON auth_tokens(token);
CREATE INDEX IF NOT EXISTS idx_tokens_user ON auth_tokens(user_id, user_type);
CREATE INDEX IF NOT EXISTS idx_tokens_expires ON auth_tokens(expires_at) WHERE NOT usado;

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
    ('inscricoes', 0, CURRENT_TIMESTAMP),
    ('cursos', 0, CURRENT_TIMESTAMP),
    ('materias', 0, CURRENT_TIMESTAMP)
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

-- Limpar tokens expirados
CREATE OR REPLACE FUNCTION cleanup_expired_tokens()
RETURNS void AS $$
BEGIN
    DELETE FROM auth_tokens 
    WHERE expires_at < CURRENT_TIMESTAMP 
    AND NOT usado;
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

-- 🔥 Obter média de notas do estudante
CREATE OR REPLACE FUNCTION get_media_estudante(
    p_codigo_estudante VARCHAR(7),
    p_ano_lectivo VARCHAR(20),
    p_periodo VARCHAR(20) DEFAULT NULL
) RETURNS DECIMAL(5,2) AS $$
DECLARE
    v_media DECIMAL(5,2);
BEGIN
    IF p_periodo IS NULL THEN
        SELECT AVG(nota) INTO v_media
        FROM projection_notas
        WHERE codigo_estudante = p_codigo_estudante
          AND ano_lectivo = p_ano_lectivo;
    ELSE
        SELECT AVG(nota) INTO v_media
        FROM projection_notas
        WHERE codigo_estudante = p_codigo_estudante
          AND ano_lectivo = p_ano_lectivo
          AND periodo = p_periodo;
    END IF;
    
    RETURN COALESCE(v_media, 0);
END;
$$ LANGUAGE plpgsql;

-- 🔥 Contar faltas do estudante
CREATE OR REPLACE FUNCTION get_total_faltas_estudante(
    p_codigo_estudante VARCHAR(7),
    p_ano_lectivo VARCHAR(20),
    p_materia_id UUID DEFAULT NULL
) RETURNS INTEGER AS $$
DECLARE
    v_total INTEGER;
BEGIN
    IF p_materia_id IS NULL THEN
        SELECT COALESCE(SUM(quantidade), 0) INTO v_total
        FROM projection_faltas
        WHERE codigo_estudante = p_codigo_estudante
          AND ano_lectivo = p_ano_lectivo;
    ELSE
        SELECT COALESCE(SUM(quantidade), 0) INTO v_total
        FROM projection_faltas
        WHERE codigo_estudante = p_codigo_estudante
          AND ano_lectivo = p_ano_lectivo
          AND materia_disciplinar_id = p_materia_id;
    END IF;
    
    RETURN v_total;
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

DROP TRIGGER IF EXISTS trigger_update_curso_timestamp ON projection_cursos;
CREATE TRIGGER trigger_update_curso_timestamp
    BEFORE UPDATE ON projection_cursos
    FOR EACH ROW EXECUTE FUNCTION update_projection_timestamp();

DROP TRIGGER IF EXISTS trigger_update_materia_timestamp ON projection_materias;
CREATE TRIGGER trigger_update_materia_timestamp
    BEFORE UPDATE ON projection_materias
    FOR EACH ROW EXECUTE FUNCTION update_projection_timestamp();

-- ============================================
-- VIEWS
-- ============================================

CREATE OR REPLACE VIEW v_estudante_completo AS
SELECT 
    e.*,
    (SELECT json_agg(n.*) FROM projection_notas n WHERE n.codigo_estudante = e.codigo_estudante) as notas,
    (SELECT json_agg(f.*) FROM projection_faltas f WHERE f.codigo_estudante = e.codigo_estudante) as faltas,
    (SELECT json_agg(i.*) FROM projection_inscricoes i WHERE i.estudante_id = e.id) as inscricoes
FROM projection_estudantes e;

-- 🔥 View: Notas com nome da matéria
CREATE OR REPLACE VIEW v_notas_completas AS
SELECT 
    n.id,
    n.codigo_estudante,
    e.nome as estudante_nome,
    n.codigo_academia,
    a.nome as academia_nome,
    n.ano_lectivo,
    n.periodo,
    m.nome as materia_nome,
    m.type as materia_type,
    n.nota,
    n.observacao,
    n.registered_at
FROM projection_notas n
LEFT JOIN projection_estudantes e ON n.codigo_estudante = e.codigo_estudante
LEFT JOIN projection_academias a ON n.codigo_academia = a.codigo_academia
LEFT JOIN projection_materias m ON n.materia_disciplinar_id = m.id;

-- 🔥 View: Faltas com nome da matéria
CREATE OR REPLACE VIEW v_faltas_completas AS
SELECT 
    f.id,
    f.codigo_estudante,
    e.nome as estudante_nome,
    f.codigo_academia,
    a.nome as academia_nome,
    f.ano_lectivo,
    f.data,
    m.nome as materia_nome,
    m.type as materia_type,
    f.quantidade,
    f.observacao,
    f.registered_at
FROM projection_faltas f
LEFT JOIN projection_estudantes e ON f.codigo_estudante = e.codigo_estudante
LEFT JOIN projection_academias a ON f.codigo_academia = a.codigo_academia
LEFT JOIN projection_materias m ON f.materia_disciplinar_id = m.id;

-- 🔥 View: Resumo de faltas por estudante/matéria
CREATE OR REPLACE VIEW v_resumo_faltas AS
SELECT 
    codigo_estudante,
    codigo_academia,
    ano_lectivo,
    materia_disciplinar_id,
    SUM(quantidade) as total_faltas,
    COUNT(*) as total_registros,
    MIN(data) as primeira_falta,
    MAX(data) as ultima_falta
FROM projection_faltas
GROUP BY codigo_estudante, codigo_academia, ano_lectivo, materia_disciplinar_id;

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
        'version', '3.0.0',
        'name', 'Spuri Event Sourcing',
        'features', json_build_array(
            'event_sourcing',
            'cqrs',
            'codigo_estudante',
            'codigo_academia',
            'admin_system',
            'role_hierarchy',
            'nivel_misto',
            'cursos_materias',
            'email_verification',
            'password_recovery',
            'auth_tokens',
            'notas_relacionais',
            'faltas_com_data'
        )
    ),
    jsonb_build_object('created_by', 'migration_completa_v3.0'),
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
COMMENT ON TABLE projection_cursos IS 'Projeção de cursos (médio/superior)';
COMMENT ON TABLE projection_materias IS 'Projeção de matérias/disciplinas';
COMMENT ON TABLE projection_notas IS 'Notas individuais por matéria - estrutura relacional v3.0';
COMMENT ON TABLE projection_faltas IS 'Faltas individuais por matéria com data específica v3.0';
COMMENT ON TABLE auth_tokens IS 'Tokens de verificação de email e recuperação de senha';
COMMENT ON TABLE admin_action_log IS 'Log de todas as ações administrativas';

COMMENT ON COLUMN projection_estudantes.codigo_estudante IS 'Código único do estudante (formato: AAA1234)';
COMMENT ON COLUMN projection_estudantes.email IS 'Email do estudante (opcional)';
COMMENT ON COLUMN projection_estudantes.telefone IS 'Telefone do estudante (opcional)';
COMMENT ON COLUMN projection_estudantes.email_verificado IS 'Se o email do estudante foi verificado';
COMMENT ON COLUMN projection_estudantes.codigo_academia IS 'Código da academia à qual o estudante pertence';
COMMENT ON COLUMN projection_estudantes.status IS 'Status geral: inativo, ativo, finalizado';
COMMENT ON COLUMN projection_estudantes.status_escolar IS 'Status ensino escolar: inativo, em_andamento, finalizado';
COMMENT ON COLUMN projection_estudantes.status_superior IS 'Status ensino superior: inativo, em_andamento, finalizado';

COMMENT ON COLUMN projection_academias.email_verificado IS 'Se o email da academia foi verificado';
COMMENT ON COLUMN projection_academias.nivel_escolar IS 'Nível escolar: fundamental, medio, misto (obrigatório para nivel=escola)';
COMMENT ON COLUMN projection_academias.status IS 'Status da academia (ativo/inativo) - academias iniciam inativas';
COMMENT ON COLUMN projection_academias.cursos IS 'Array JSON com lista de nomes de cursos oferecidos';

COMMENT ON COLUMN projection_cursos.nivel IS 'Array JSON com anos do curso: ["primeiro_medio","segundo_medio","terceiro_medio"]';
COMMENT ON COLUMN projection_materias.curso_id IS 'NULL para fundamental, FK para medio/superior';
COMMENT ON COLUMN projection_materias.nivel IS 'Apenas para fundamental: ["1_fundamental","2_fundamental",...]';

COMMENT ON COLUMN projection_notas.periodo IS 'Período: 1_trimestre, 2_trimestre, 3_trimestre, 1_semestre, 2_semestre';
COMMENT ON COLUMN projection_notas.nota IS 'Nota de 0 a 20';
COMMENT ON COLUMN projection_notas.materia_disciplinar_id IS 'FK para projection_materias';
COMMENT ON COLUMN projection_notas.observacao IS 'Detalhes sobre a nota (texto opcional)';

COMMENT ON COLUMN projection_faltas.data IS 'Data específica da falta';
COMMENT ON COLUMN projection_faltas.quantidade IS 'Quantidade de faltas (geralmente 1)';
COMMENT ON COLUMN projection_faltas.materia_disciplinar_id IS 'FK para projection_materias';
COMMENT ON COLUMN projection_faltas.observacao IS 'Detalhes sobre a falta (texto opcional)';

COMMENT ON COLUMN projection_admins.role IS 'Hierarquia: fpp > adm > gerente';
COMMENT ON COLUMN projection_admins.email_verificado IS 'Se o email do admin foi verificado';
COMMENT ON COLUMN projection_inscricoes.status_usado IS 'Se a inscrição aprovada já foi usada para vincular';

COMMENT ON COLUMN auth_tokens.tipo IS 'Tipo do token: verificacao_email ou recuperacao_senha';
COMMENT ON COLUMN auth_tokens.usado IS 'Se o token já foi utilizado';
COMMENT ON COLUMN auth_tokens.expires_at IS 'Data de expiração do token';

COMMENT ON FUNCTION generate_codigo_estudante IS 'Gera código único AAA1234 para estudantes';
COMMENT ON FUNCTION registrar_acao_admin IS 'Registra uma ação administrativa no log';
COMMENT ON FUNCTION cleanup_expired_tokens IS 'Remove tokens expirados do banco de dados';
COMMENT ON FUNCTION verify_hash_chain IS 'Verifica integridade da cadeia de hashes de um agregado';
COMMENT ON FUNCTION get_media_estudante IS 'Calcula média de notas do estudante por período';
COMMENT ON FUNCTION get_total_faltas_estudante IS 'Soma total de faltas do estudante';

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
    RAISE NOTICE '║ SCHEMA CRIADO COM SUCESSO! v3.0.0 ║';
    RAISE NOTICE '╚═══════════════════════════════════╝';
    RAISE NOTICE 'Total de eventos: %', v_total_eventos;
    RAISE NOTICE 'Total de checkpoints: %', v_total_checkpoints;
    RAISE NOTICE 'Total de admins: %', v_total_admins;
    RAISE NOTICE '🔥 NOVIDADES v3.0.0:';
    RAISE NOTICE '   ✅ Estrutura relacional de Notas';
    RAISE NOTICE '   ✅ Estrutura relacional de Faltas';
    RAISE NOTICE '   ✅ Relacionamento com Matérias';
    RAISE NOTICE '   ✅ Data específica para Faltas';
    RAISE NOTICE '   ✅ Campo observação em Notas/Faltas';
    RAISE NOTICE '   ✅ Funções de cálculo (média, total)';
    RAISE NOTICE '   ✅ Views auxiliares completas';
    RAISE NOTICE '╚═══════════════════════════════════╝';
END $$;
-- ===== END 001_complete_schema.sql =====

-- ===== BEGIN 002_add_email_verificado_safe.sql =====
-- ============================================
-- MIGRATION SEGURA - Adicionar email_verificado
-- Versão: 002
-- Data: 25-01-2026
-- SAFE para produção com dados existentes
-- ============================================

-- 1. Adicionar coluna email_verificado em projection_admins (se não existir)
DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'projection_admins' 
        AND column_name = 'email_verificado'
    ) THEN
        ALTER TABLE projection_admins 
        ADD COLUMN email_verificado BOOLEAN DEFAULT FALSE;
        
        RAISE NOTICE '✅ Coluna email_verificado adicionada em projection_admins';
    ELSE
        RAISE NOTICE 'ℹ️  Coluna email_verificado já existe em projection_admins';
    END IF;
END $$;

-- 2. Definir FALSE para todos os registros existentes (garantir consistência)
UPDATE projection_admins 
SET email_verificado = FALSE 
WHERE email_verificado IS NULL;

DO $$ BEGIN
    RAISE NOTICE '✅ Registros existentes atualizados com email_verificado = FALSE';
END $$;

-- 3. Adicionar NOT NULL constraint depois de preencher valores
ALTER TABLE projection_admins 
ALTER COLUMN email_verificado SET NOT NULL;

DO $$ BEGIN
    RAISE NOTICE '✅ Constraint NOT NULL adicionada';
END $$;

-- 4. Adicionar comentário
COMMENT ON COLUMN projection_admins.email_verificado IS 'Indica se o email do admin foi verificado';

-- 5. Verificação final
DO $$ 
DECLARE
    v_total_admins INTEGER;
    v_admins_nao_verificados INTEGER;
BEGIN
    SELECT COUNT(*) INTO v_total_admins FROM projection_admins;
    SELECT COUNT(*) INTO v_admins_nao_verificados 
    FROM projection_admins 
    WHERE email_verificado = FALSE;
    
    RAISE NOTICE '══════════════════════════════════════════';
    RAISE NOTICE '✅ MIGRATION 002 CONCLUÍDA COM SUCESSO';
    RAISE NOTICE '══════════════════════════════════════════';
    RAISE NOTICE 'Total de admins: %', v_total_admins;
    RAISE NOTICE 'Admins não verificados: %', v_admins_nao_verificados;
    RAISE NOTICE '══════════════════════════════════════════';
END $$;
-- ===== END 002_add_email_verificado_safe.sql =====

-- ===== BEGIN 003_add_aprovacao_ano.sql =====
-- ============================================
-- MIGRATION 003 - Sistema de Aprovação de Ano Letivo
-- ============================================

-- Tabela de projeção para aprovações/reprovações
CREATE TABLE IF NOT EXISTS projection_aprovacao_ano (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    
    -- Identificadores
    codigo_estudante VARCHAR(7) NOT NULL,
    codigo_academia VARCHAR(50) NOT NULL,
    
    -- Período
    ano_lectivo VARCHAR(20) NOT NULL,
    
    -- Níveis
    nivel_atual VARCHAR(50) NOT NULL,
    nivel_seguinte VARCHAR(50),
    
    -- Resultado
    avancar_ano BOOLEAN NOT NULL DEFAULT FALSE,
    observacao TEXT,
    
    -- Metadados
    registered_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    event_id UUID NOT NULL,
    version INTEGER NOT NULL,
    
    -- Constraint: única aprovação por estudante/academia/ano
    UNIQUE(codigo_estudante, codigo_academia, ano_lectivo)
);

-- Índices
CREATE INDEX IF NOT EXISTS idx_aprovacao_estudante ON projection_aprovacao_ano(codigo_estudante);
CREATE INDEX IF NOT EXISTS idx_aprovacao_academia ON projection_aprovacao_ano(codigo_academia);
CREATE INDEX IF NOT EXISTS idx_aprovacao_ano ON projection_aprovacao_ano(ano_lectivo);
CREATE INDEX IF NOT EXISTS idx_aprovacao_registered ON projection_aprovacao_ano(registered_at DESC);

-- Checkpoint
INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at) 
VALUES ('aprovacao_ano', 0, CURRENT_TIMESTAMP)
ON CONFLICT (projection_name) DO NOTHING;

-- Comentários
COMMENT ON TABLE projection_aprovacao_ano IS 'Histórico de aprovações/reprovações de ano letivo';
COMMENT ON COLUMN projection_aprovacao_ano.avancar_ano IS 'Se TRUE, estudante avança para próximo ano';
COMMENT ON COLUMN projection_aprovacao_ano.nivel_seguinte IS 'Próximo nível (NULL se último ano)';

-- View auxiliar
CREATE OR REPLACE VIEW v_aprovacoes_completas AS
SELECT 
    a.id,
    a.codigo_estudante,
    e.nome as estudante_nome,
    a.codigo_academia,
    ac.nome as academia_nome,
    a.ano_lectivo,
    a.nivel_atual,
    a.nivel_seguinte,
    a.avancar_ano,
    a.observacao,
    a.registered_at,
    CASE 
        WHEN a.avancar_ano THEN 'APROVADO'
        ELSE 'REPROVADO'
    END as resultado
FROM projection_aprovacao_ano a
LEFT JOIN projection_estudantes e ON a.codigo_estudante = e.codigo_estudante
LEFT JOIN projection_academias ac ON a.codigo_academia = ac.codigo_academia;

-- Função auxiliar para próximo nível
CREATE OR REPLACE FUNCTION get_proximo_nivel(
    p_nivel_atual VARCHAR,
    p_tipo VARCHAR  -- 'escolar' ou 'superior'
) RETURNS VARCHAR AS $$
DECLARE
    v_niveis_escolar TEXT[] := ARRAY[
        '1_fundamental', 'segundo_fundamental', 'terceiro_fundamental',
        'quarto_fundamental', 'quinto_fundamental', 'sexto_fundamental',
        'setimo_fundamental', 'oitavo_fundamental', 'nono_fundamental',
        'primeiro_medio', 'segundo_medio', 'terceiro_medio', 'quarto_medio'
    ];
    v_niveis_superior TEXT[] := ARRAY[
        'primeiro_ano', 'segundo_ano', 'terceiro_ano',
        'quarto_ano', 'quinto_ano', 'sexto_ano'
    ];
    v_idx INTEGER;
BEGIN
    IF p_tipo = 'escolar' THEN
        v_idx := array_position(v_niveis_escolar, p_nivel_atual);
        IF v_idx IS NULL OR v_idx = array_length(v_niveis_escolar, 1) THEN
            RETURN NULL; -- Último ano
        END IF;
        RETURN v_niveis_escolar[v_idx + 1];
    ELSIF p_tipo = 'superior' THEN
        v_idx := array_position(v_niveis_superior, p_nivel_atual);
        IF v_idx IS NULL OR v_idx = array_length(v_niveis_superior, 1) THEN
            RETURN NULL; -- Último ano
        END IF;
        RETURN v_niveis_superior[v_idx + 1];
    END IF;
    
    RETURN NULL;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 003 CONCLUÍDA - Sistema de Aprovação de Ano';
END $$;
-- ===== END 003_add_aprovacao_ano.sql =====

-- ===== BEGIN 004_cursos_uuid.sql =====
-- ============================================
-- MIGRATION 004 - Alterar curso_medio/curso_superior para UUID
-- ============================================

-- 1. Adicionar novas colunas UUID
ALTER TABLE projection_estudantes 
  ADD COLUMN IF NOT EXISTS curso_medio_id UUID,
  ADD COLUMN IF NOT EXISTS curso_superior_id UUID;

-- 2. Adicionar FKs para cursos
ALTER TABLE projection_estudantes
  ADD CONSTRAINT fk_estudante_curso_medio 
    FOREIGN KEY (curso_medio_id) 
    REFERENCES projection_cursos(id) 
    ON DELETE SET NULL;

ALTER TABLE projection_estudantes
  ADD CONSTRAINT fk_estudante_curso_superior 
    FOREIGN KEY (curso_superior_id) 
    REFERENCES projection_cursos(id) 
    ON DELETE SET NULL;

-- 3. Atualizar tabela de inscrições
ALTER TABLE projection_inscricoes 
  DROP COLUMN IF EXISTS curso CASCADE;

ALTER TABLE projection_inscricoes 
  ADD COLUMN IF NOT EXISTS curso_id UUID;

ALTER TABLE projection_inscricoes
  ADD CONSTRAINT fk_inscricao_curso 
    FOREIGN KEY (curso_id) 
    REFERENCES projection_cursos(id) 
    ON DELETE SET NULL;

-- 4. Remover colunas antigas (SOMENTE após migração de dados)
-- CUIDADO: Execute isso DEPOIS de migrar os dados existentes!
-- ALTER TABLE projection_estudantes DROP COLUMN IF EXISTS curso_medio CASCADE;
-- ALTER TABLE projection_estudantes DROP COLUMN IF EXISTS curso_superior CASCADE;

-- 5. Comentários
COMMENT ON COLUMN projection_estudantes.curso_medio_id IS 'FK para curso de ensino médio (UUID)';
COMMENT ON COLUMN projection_estudantes.curso_superior_id IS 'FK para curso de ensino superior (UUID)';
COMMENT ON COLUMN projection_inscricoes.curso_id IS 'FK para curso da inscrição (UUID)';

-- 6. Índices
CREATE INDEX IF NOT EXISTS idx_estudante_curso_medio ON projection_estudantes(curso_medio_id);
CREATE INDEX IF NOT EXISTS idx_estudante_curso_superior ON projection_estudantes(curso_superior_id);
CREATE INDEX IF NOT EXISTS idx_inscricao_curso ON projection_inscricoes(curso_id);

-- 7. View auxiliar com nomes dos cursos
CREATE OR REPLACE VIEW v_estudantes_com_cursos AS
SELECT 
    e.id,
    e.nome,
    e.codigo_estudante,
    e.email,
    e.codigo_academia,
    e.status,
    e.status_escolar,
    e.status_superior,
    e.ano_escolar,
    e.ano_superior,
    cm.id as curso_medio_id,
    cm.nome as curso_medio_nome,
    cs.id as curso_superior_id,
    cs.nome as curso_superior_nome,
    e.created_at,
    e.updated_at
FROM projection_estudantes e
LEFT JOIN projection_cursos cm ON e.curso_medio_id = cm.id
LEFT JOIN projection_cursos cs ON e.curso_superior_id = cs.id;

-- 8. View de inscrições com cursos
CREATE OR REPLACE VIEW v_inscricoes_com_cursos AS
SELECT 
    i.id,
    i.estudante_id,
    i.codigo_estudante,
    e.nome as estudante_nome,
    i.academia_id,
    i.codigo_academia,
    a.nome as academia_nome,
    i.tipo,
    i.ano_inscricao,
    i.curso_id,
    c.nome as curso_nome,
    c.type as curso_type,
    i.status,
    i.status_usado,
    i.created_at,
    i.updated_at
FROM projection_inscricoes i
LEFT JOIN projection_estudantes e ON i.estudante_id = e.id
LEFT JOIN projection_academias a ON i.academia_id = a.id
LEFT JOIN projection_cursos c ON i.curso_id = c.id;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 004 CONCLUÍDA - Cursos agora são UUID com FKs';
    RAISE NOTICE '⚠️  LEMBRE-SE: Migre os dados existentes antes de dropar as colunas antigas';
END $$;
-- ===== END 004_cursos_uuid.sql =====

-- ===== BEGIN 005_add_sistema_config.sql =====
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
-- ===== END 005_add_sistema_config.sql =====

-- ===== BEGIN 006_add_tipo_categoria_notas.sql =====
-- ============================================
-- MIGRATION 006 - Tipo e Categoria em Notas
-- ============================================

-- 1. Remover constraint UNIQUE antiga
ALTER TABLE projection_notas
    DROP CONSTRAINT IF EXISTS projection_notas_codigo_estudante_codigo_academia_ano_lectivo_key;

-- 2. Adicionar colunas tipo e categoria
ALTER TABLE projection_notas
    ADD COLUMN IF NOT EXISTS tipo VARCHAR(20) NOT NULL DEFAULT 'escolar'
        CHECK (tipo IN ('escolar', 'superior')),
    ADD COLUMN IF NOT EXISTS categoria VARCHAR(100) NOT NULL DEFAULT 'nota_escola';

-- 3. Remover o DEFAULT após adicionar (evita dados inválidos futuros sem valor)
ALTER TABLE projection_notas
    ALTER COLUMN tipo DROP DEFAULT,
    ALTER COLUMN categoria DROP DEFAULT;

-- 4. Nova UNIQUE constraint incluindo tipo e categoria
ALTER TABLE projection_notas
    ADD CONSTRAINT uq_nota_unica
        UNIQUE (codigo_estudante, ano_lectivo, periodo, materia_disciplinar_id, tipo, categoria);

-- 5. Índices para os novos campos
CREATE INDEX IF NOT EXISTS idx_notas_tipo      ON projection_notas(tipo);
CREATE INDEX IF NOT EXISTS idx_notas_categoria ON projection_notas(categoria);

-- ============================================
-- Tabela de Categorias Customizadas (Superior)
-- ============================================

CREATE TABLE IF NOT EXISTS projection_categorias_nota (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    codigo_academia VARCHAR(50)  NOT NULL,
    nome            VARCHAR(100) NOT NULL, -- formato: "nota_[nome]"
    descricao       TEXT,
    status          VARCHAR(20)  NOT NULL DEFAULT 'ativo'
                        CHECK (status IN ('ativo', 'inativo')),
    created_at      TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    event_id        UUID         NOT NULL,
    version         INTEGER      NOT NULL,

    FOREIGN KEY (codigo_academia)
        REFERENCES projection_academias(codigo_academia)
        ON DELETE CASCADE,

    -- Uma academia não pode ter duas categorias com o mesmo nome
    UNIQUE (codigo_academia, nome)
);

CREATE INDEX IF NOT EXISTS idx_cat_nota_academia ON projection_categorias_nota(codigo_academia);
CREATE INDEX IF NOT EXISTS idx_cat_nota_status   ON projection_categorias_nota(status);

-- ============================================
-- Checkpoint para nova projeção
-- ============================================

INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at)
VALUES ('categorias_nota', 0, CURRENT_TIMESTAMP)
ON CONFLICT (projection_name) DO NOTHING;

-- ============================================
-- Comentários
-- ============================================

COMMENT ON COLUMN projection_notas.tipo IS
    'Tipo da nota: escolar | superior';

COMMENT ON COLUMN projection_notas.categoria IS
    'Categoria da nota. Escolar: nota_escola | nota_professor. '
    'Superior fixas: nota_pp1 | nota_pp2 | nota_exame. '
    'Superior adicionais: nota_[nome] (definidas pela academia)';

COMMENT ON TABLE projection_categorias_nota IS
    'Categorias de nota adicionais criadas por academias do tipo superior';

COMMENT ON COLUMN projection_categorias_nota.nome IS
    'Sempre no formato nota_[nome], ex: nota_trabalho';
-- ===== END 006_add_tipo_categoria_notas.sql =====

-- ===== BEGIN 007_add_turmas_genero.sql =====
-- ============================================
-- MIGRATION 007 - Turmas + Gênero do Estudante
-- ============================================

-- 1. Adicionar campo genero em projection_estudantes
ALTER TABLE projection_estudantes
    ADD COLUMN IF NOT EXISTS genero VARCHAR(10) CHECK (genero IN ('masculino', 'feminino'));

COMMENT ON COLUMN projection_estudantes.genero IS 'Gênero do estudante: masculino ou feminino';

-- 2. Criar tabela projection_turmas
CREATE TABLE IF NOT EXISTS projection_turmas (
    id              UUID PRIMARY KEY,
    codigo_turma    VARCHAR(50) NOT NULL,
    codigo_academia VARCHAR(50) NOT NULL,
    nivel           VARCHAR(50) NOT NULL,
    curso_id        UUID,
    turno           VARCHAR(10) NOT NULL CHECK (turno IN ('manha', 'tarde', 'noite')),
    estudantes      JSONB NOT NULL DEFAULT '[]',
    status          VARCHAR(20) NOT NULL DEFAULT 'ativo' CHECK (status IN ('ativo', 'inativo')),
    created_at      TIMESTAMP NOT NULL,
    updated_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    version         INTEGER NOT NULL DEFAULT 0,
    last_event_id   UUID,

    UNIQUE (codigo_turma, codigo_academia),

    FOREIGN KEY (codigo_academia)
        REFERENCES projection_academias(codigo_academia)
        ON DELETE CASCADE,

    FOREIGN KEY (curso_id)
        REFERENCES projection_cursos(id)
        ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_turmas_academia ON projection_turmas(codigo_academia);
CREATE INDEX IF NOT EXISTS idx_turmas_nivel    ON projection_turmas(nivel);
CREATE INDEX IF NOT EXISTS idx_turmas_turno    ON projection_turmas(turno);
CREATE INDEX IF NOT EXISTS idx_turmas_status   ON projection_turmas(status);
CREATE INDEX IF NOT EXISTS idx_turmas_curso    ON projection_turmas(curso_id);

-- 3. Checkpoint para a projeção
INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at)
VALUES ('turmas', 0, CURRENT_TIMESTAMP)
ON CONFLICT (projection_name) DO NOTHING;

-- Comentários
COMMENT ON TABLE  projection_turmas                IS 'Projeção de turmas das academias';
COMMENT ON COLUMN projection_turmas.codigo_turma   IS 'Identificador único da turma dentro da academia';
COMMENT ON COLUMN projection_turmas.nivel          IS 'Ano escolar/superior da turma';
COMMENT ON COLUMN projection_turmas.curso_id       IS 'FK para curso (apenas médio/superior)';
COMMENT ON COLUMN projection_turmas.turno          IS 'Turno: manha, tarde, noite';
COMMENT ON COLUMN projection_turmas.estudantes     IS 'Array JSON com códigos dos estudantes da turma';

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 007 CONCLUÍDA - Turmas criadas e gênero adicionado ao estudante';
END $$;
-- ===== END 007_add_turmas_genero.sql =====

-- ===== BEGIN 008_status_escolar_split_aprovacao.sql =====
-- ============================================
-- MIGRATION 008 - Split status_escolar + Aprovação revisada
-- Anos dinâmicos por curso, aprovação manual pela academia
-- CORRIGIDA: dropa views dependentes antes de dropar status_escolar
-- ============================================

BEGIN;

-- ============================================
-- 1. Split status_escolar → fundamental + medio
-- ============================================

ALTER TABLE projection_estudantes
    ADD COLUMN IF NOT EXISTS status_escolar_fundamental VARCHAR(20)
        NOT NULL DEFAULT 'inativo'
        CHECK (status_escolar_fundamental IN ('inativo', 'em_andamento', 'finalizado')),
    ADD COLUMN IF NOT EXISTS status_escolar_medio VARCHAR(20)
        NOT NULL DEFAULT 'inativo'
        CHECK (status_escolar_medio IN ('inativo', 'em_andamento', 'finalizado'));

-- Migrar dados existentes: status_escolar atual vai para fundamental
UPDATE projection_estudantes
SET status_escolar_fundamental = status_escolar;

-- Dropar views dependentes ANTES de remover a coluna
DROP VIEW IF EXISTS v_estudante_completo CASCADE;
DROP VIEW IF EXISTS v_estudantes_com_cursos CASCADE;

-- Remover coluna antiga
ALTER TABLE projection_estudantes
    DROP COLUMN IF EXISTS status_escolar;

COMMENT ON COLUMN projection_estudantes.status_escolar_fundamental IS
    'Status ensino fundamental: inativo | em_andamento | finalizado';
COMMENT ON COLUMN projection_estudantes.status_escolar_medio IS
    'Status ensino médio: inativo | em_andamento | finalizado';

CREATE INDEX IF NOT EXISTS idx_estudante_status_fund  ON projection_estudantes(status_escolar_fundamental);
CREATE INDEX IF NOT EXISTS idx_estudante_status_medio ON projection_estudantes(status_escolar_medio);

-- ============================================
-- 2. Reestruturar projection_aprovacao_ano
-- ============================================

ALTER TABLE projection_aprovacao_ano
    DROP CONSTRAINT IF EXISTS projection_aprovacao_ano_codigo_estudante_codigo_academia_ano_key;

ALTER TABLE projection_aprovacao_ano
    RENAME COLUMN avancar_ano TO aprovado;

ALTER TABLE projection_aprovacao_ano
    RENAME COLUMN nivel_seguinte TO proximo_nivel;

ALTER TABLE projection_aprovacao_ano
    ADD COLUMN IF NOT EXISTS tipo_ensino VARCHAR(20)
        NOT NULL DEFAULT 'fundamental'
        CHECK (tipo_ensino IN ('fundamental', 'medio', 'superior'));

ALTER TABLE projection_aprovacao_ano
    ALTER COLUMN tipo_ensino DROP DEFAULT;

COMMENT ON COLUMN projection_aprovacao_ano.aprovado IS
    'TRUE = aprovado (avança ou finaliza); FALSE = reprovado (fica no mesmo nível)';
COMMENT ON COLUMN projection_aprovacao_ano.proximo_nivel IS
    'Próximo nível definido pela academia. NULL se reprovado ou último ano do curso';
COMMENT ON COLUMN projection_aprovacao_ano.tipo_ensino IS
    'Tipo de ensino: fundamental | medio | superior';

CREATE INDEX IF NOT EXISTS idx_aprovacao_tipo_ensino ON projection_aprovacao_ano(tipo_ensino);
CREATE INDEX IF NOT EXISTS idx_aprovacao_aprovado     ON projection_aprovacao_ano(aprovado);

-- ============================================
-- 3. Remover função de próximo nível
-- ============================================

DROP FUNCTION IF EXISTS get_proximo_nivel(VARCHAR, VARCHAR);

-- ============================================
-- 4. Recriar views
-- ============================================

DROP VIEW IF EXISTS v_aprovacoes_completas;
CREATE OR REPLACE VIEW v_aprovacoes_completas AS
SELECT
    a.id,
    a.codigo_estudante,
    e.nome                                                       AS estudante_nome,
    a.codigo_academia,
    ac.nome                                                      AS academia_nome,
    a.ano_lectivo,
    a.tipo_ensino,
    a.nivel_atual,
    a.proximo_nivel,
    a.aprovado,
    a.observacao,
    a.registered_at,
    CASE WHEN a.aprovado THEN 'APROVADO' ELSE 'REPROVADO' END   AS resultado
FROM projection_aprovacao_ano a
LEFT JOIN projection_estudantes e  ON a.codigo_estudante = e.codigo_estudante
LEFT JOIN projection_academias  ac ON a.codigo_academia  = ac.codigo_academia;

CREATE OR REPLACE VIEW v_estudantes_com_cursos AS
SELECT
    e.id,
    e.nome,
    e.codigo_estudante,
    e.email,
    e.genero,
    e.codigo_academia,
    e.status,
    e.status_escolar_fundamental,
    e.status_escolar_medio,
    e.status_superior,
    e.ano_escolar,
    e.ano_superior,
    cm.id   AS curso_medio_id,
    cm.nome AS curso_medio_nome,
    cs.id   AS curso_superior_id,
    cs.nome AS curso_superior_nome,
    e.created_at,
    e.updated_at
FROM projection_estudantes e
LEFT JOIN projection_cursos cm ON e.curso_medio_id   = cm.id
LEFT JOIN projection_cursos cs ON e.curso_superior_id = cs.id;

CREATE OR REPLACE VIEW v_estudante_completo AS
SELECT
    e.*,
    (SELECT json_agg(n.*) FROM projection_notas n WHERE n.codigo_estudante = e.codigo_estudante) AS notas,
    (SELECT json_agg(f.*) FROM projection_faltas f WHERE f.codigo_estudante = e.codigo_estudante) AS faltas,
    (SELECT json_agg(i.*) FROM projection_inscricoes i WHERE i.estudante_id = e.id) AS inscricoes
FROM projection_estudantes e;

COMMIT;

DO $$ BEGIN RAISE NOTICE '✅ MIGRATION 008 CONCLUÍDA - status_escolar dividido + aprovação revisada'; END $$;
-- ===== END 008_status_escolar_split_aprovacao.sql =====

-- ===== BEGIN 009_ano_escolar_medio_reprovacoes.sql =====
-- ============================================================================
-- MIGRATION 009 - Corrigir ano_escolar_medio + Tabela de reprovações dedicada
-- ============================================================================

BEGIN;

-- ============================================================================
-- 1. Adicionar ano_escolar_medio à projeção de estudantes
-- ============================================================================

ALTER TABLE projection_estudantes
    ADD COLUMN IF NOT EXISTS ano_escolar_medio VARCHAR(100);

COMMENT ON COLUMN projection_estudantes.ano_escolar IS
    'Ano atual no ciclo fundamental (definido pela academia)';
COMMENT ON COLUMN projection_estudantes.ano_escolar_medio IS
    'Ano atual no ciclo médio (definido pela academia dentro do curso)';

-- ============================================================================
-- 2. Criar tabela dedicada de reprovações (log explícito)
-- ============================================================================

CREATE TABLE IF NOT EXISTS projection_reprovacoes (
    id               UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    codigo_estudante VARCHAR(7)  NOT NULL,
    codigo_academia  VARCHAR(50) NOT NULL,
    ano_lectivo      VARCHAR(20) NOT NULL,
    tipo_ensino      VARCHAR(20) NOT NULL
        CHECK (tipo_ensino IN ('fundamental', 'medio', 'superior')),
    nivel_reprovado  VARCHAR(50) NOT NULL,
    observacao       TEXT,
    registered_at    TIMESTAMP   NOT NULL DEFAULT CURRENT_TIMESTAMP,
    event_id         UUID        NOT NULL,
    version          INTEGER     NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_reprov_estudante ON projection_reprovacoes(codigo_estudante);
CREATE INDEX IF NOT EXISTS idx_reprov_academia  ON projection_reprovacoes(codigo_academia);
CREATE INDEX IF NOT EXISTS idx_reprov_ano       ON projection_reprovacoes(ano_lectivo);
CREATE INDEX IF NOT EXISTS idx_reprov_tipo      ON projection_reprovacoes(tipo_ensino);

COMMENT ON TABLE projection_reprovacoes IS
    'Log de reprovações por ano letivo. Complementa projection_aprovacao_ano (aprovado=false).';

INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at)
VALUES ('reprovacoes', 0, CURRENT_TIMESTAMP)
ON CONFLICT (projection_name) DO NOTHING;

-- ============================================================================
-- 3. Atualizar view v_estudantes_com_cursos para incluir ano_escolar_medio
-- ============================================================================

DROP VIEW IF EXISTS v_estudantes_com_cursos;
CREATE OR REPLACE VIEW v_estudantes_com_cursos AS
SELECT
    e.id,
    e.nome,
    e.codigo_estudante,
    e.email,
    e.genero,
    e.codigo_academia,
    e.status,
    e.status_escolar_fundamental,
    e.status_escolar_medio,
    e.status_superior,
    e.ano_escolar,
    e.ano_escolar_medio,
    e.ano_superior,
    cm.id   AS curso_medio_id,
    cm.nome AS curso_medio_nome,
    cs.id   AS curso_superior_id,
    cs.nome AS curso_superior_nome,
    e.created_at,
    e.updated_at
FROM projection_estudantes e
LEFT JOIN projection_cursos cm ON e.curso_medio_id   = cm.id
LEFT JOIN projection_cursos cs ON e.curso_superior_id = cs.id;

-- ============================================================================
-- 4. Recriar v_estudante_completo (depende de v_estudantes_com_cursos)
-- ============================================================================

DROP VIEW IF EXISTS v_estudante_completo;
CREATE OR REPLACE VIEW v_estudante_completo AS
SELECT
    e.*,
    (SELECT json_agg(n.*) FROM projection_notas n WHERE n.codigo_estudante = e.codigo_estudante) AS notas,
    (SELECT json_agg(f.*) FROM projection_faltas f WHERE f.codigo_estudante = e.codigo_estudante) AS faltas,
    (SELECT json_agg(i.*) FROM projection_inscricoes i WHERE i.estudante_id = e.id) AS inscricoes
FROM projection_estudantes e;

-- ============================================================================
-- 5. View de reprovações completas
-- ============================================================================

CREATE OR REPLACE VIEW v_reprovacoes_completas AS
SELECT
    r.id,
    r.codigo_estudante,
    e.nome                AS estudante_nome,
    r.codigo_academia,
    ac.nome               AS academia_nome,
    r.ano_lectivo,
    r.tipo_ensino,
    r.nivel_reprovado,
    r.observacao,
    r.registered_at
FROM projection_reprovacoes r
LEFT JOIN projection_estudantes e  ON r.codigo_estudante = e.codigo_estudante
LEFT JOIN projection_academias  ac ON r.codigo_academia  = ac.codigo_academia;

-- ============================================================================
-- INSTRUÇÃO: Em internal/db/safe_queries.go, adicionar à validTables:
--   "projection_reprovacoes": true,
-- ============================================================================

COMMIT;

DO $$ BEGIN RAISE NOTICE '✅ MIGRATION 009 - ano_escolar_medio + projection_reprovacoes criados'; END $$;
-- ===== END 009_ano_escolar_medio_reprovacoes.sql =====

-- ===== BEGIN 010_academia_anos_academicos.sql =====
-- ============================================================================
-- MIGRATION 010 - AnosAcademicos na Academia
-- CORRIGIDA: preenche anos_academicos nas academias existentes antes da constraint
-- ============================================================================

BEGIN;

ALTER TABLE projection_academias
    ADD COLUMN IF NOT EXISTS anos_academicos JSONB;

COMMENT ON COLUMN projection_academias.anos_academicos IS
    'Array JSON com anos fundamentais oferecidos pela academia. '
    'Obrigatório para nivel_escolar IN (''fundamental'', ''misto''). '
    'Exemplo: ["1_fundamental","segundo_fundamental","terceiro_fundamental"]';

-- Preencher academias fundamental/misto existentes com array vazio
-- para não violar a constraint NOT NULL que será adicionada abaixo.
-- O admin deve atualizar os valores corretos depois.
UPDATE projection_academias
SET anos_academicos = '[]'::jsonb
WHERE nivel_escolar IN ('fundamental', 'misto')
  AND anos_academicos IS NULL;

-- Constraint: anos_academicos obrigatório (NOT NULL) para fundamental/misto
-- Versão permissiva: aceita array vazio para não bloquear dados existentes.
-- Se quiser obrigar array não-vazio, troque para:
--   AND jsonb_array_length(anos_academicos) > 0
ALTER TABLE projection_academias
    DROP CONSTRAINT IF EXISTS check_anos_academicos_nivel;

ALTER TABLE projection_academias
    ADD CONSTRAINT check_anos_academicos_nivel CHECK (
        (nivel_escolar IN ('fundamental', 'misto') AND anos_academicos IS NOT NULL)
        OR
        (nivel_escolar NOT IN ('fundamental', 'misto') OR nivel_escolar IS NULL)
    );

CREATE INDEX IF NOT EXISTS idx_academia_anos_academicos
    ON projection_academias USING GIN (anos_academicos)
    WHERE anos_academicos IS NOT NULL;

COMMIT;

DO $$ BEGIN RAISE NOTICE '✅ MIGRATION 010 - anos_academicos adicionado a projection_academias'; END $$;
-- ===== END 010_academia_anos_academicos.sql =====

-- ===== BEGIN 011_cursos_nivel_to_anos_academicos.sql =====
-- ============================================================================
-- MIGRATION 011 - Renomear 'nivel' → 'anos_academicos' em projection_cursos
-- CORRIGIDA: usa bloco DO condicional para evitar erro se coluna já foi renomeada
-- ============================================================================

BEGIN;

-- 1. Renomear coluna somente se ainda existir como 'nivel'
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'projection_cursos'
          AND column_name = 'nivel'
    ) THEN
        ALTER TABLE projection_cursos RENAME COLUMN nivel TO anos_academicos;
        RAISE NOTICE 'Coluna nivel renomeada para anos_academicos';
    ELSE
        RAISE NOTICE 'Coluna nivel não existe (já renomeada ou inexistente), pulando';
    END IF;
END $$;

COMMENT ON COLUMN projection_cursos.anos_academicos IS
    'Array JSON com os anos/níveis do curso definidos pela academia. '
    'Obrigatório para cursos de médio e superior. '
    'Exemplo: ["7ano","8ano","9ano"] ou ["1ano","2ano","3ano","4ano","5ano"]';

-- 2. Atualizar índice GIN
DROP INDEX IF EXISTS idx_cursos_nivel;
CREATE INDEX IF NOT EXISTS idx_cursos_anos_academicos
    ON projection_cursos USING GIN (anos_academicos)
    WHERE anos_academicos IS NOT NULL;

-- 3. NOT NULL apenas se todos os registros estiverem preenchidos
-- Verificar antes: SELECT COUNT(*) FROM projection_cursos WHERE anos_academicos IS NULL;
-- Se retornar 0, pode ativar:
-- ALTER TABLE projection_cursos ALTER COLUMN anos_academicos SET NOT NULL;

COMMIT;

DO $$ BEGIN RAISE NOTICE '✅ MIGRATION 011 - nivel → anos_academicos em projection_cursos'; END $$;
-- ===== END 011_cursos_nivel_to_anos_academicos.sql =====

-- ===== BEGIN 012_ano_academico.sql =====
-- ============================================
-- MIGRATION 012 - Adicionar ano_academico em notas e faltas
-- Representa o ano do estudante (ex: "1_fundamental", "segundo_medio")
-- ============================================

BEGIN;

ALTER TABLE projection_notas
    ADD COLUMN IF NOT EXISTS ano_academico VARCHAR(50);

ALTER TABLE projection_faltas
    ADD COLUMN IF NOT EXISTS ano_academico VARCHAR(50);

COMMENT ON COLUMN projection_notas.ano_academico IS
    'Ano acadêmico do estudante no momento do registro (ex: 1_fundamental, segundo_medio, terceiro_ano)';

COMMENT ON COLUMN projection_faltas.ano_academico IS
    'Ano acadêmico do estudante no momento do registro (ex: 1_fundamental, segundo_medio, terceiro_ano)';

CREATE INDEX IF NOT EXISTS idx_notas_ano_academico   ON projection_notas(ano_academico);
CREATE INDEX IF NOT EXISTS idx_faltas_ano_academico  ON projection_faltas(ano_academico);

COMMIT;
-- ===== END 012_ano_academico.sql =====

-- ===== BEGIN 013_anos_academicos_materia.sql =====
-- ============================================================================
-- MIGRATION 013 — Atualização semântica de anos_academicos em projection_materias
-- ============================================================================
--
-- ATUALIZAÇÃO 1:
--   O campo "nivel" (JSONB) em projection_materias passa a armazenar
--   AnosAcademicos com as seguintes regras:
--     • fundamental : array com 1–9 itens (1_fundamental…nono_fundamental)
--     • medio       : array com EXATAMENTE 1 item (ano do curso da matéria)
--     • superior    : array com EXATAMENTE 1 item (ano do curso da matéria)
--
--   Nenhuma alteração estrutural é necessária no banco — a coluna "nivel" JSONB
--   já comporta os dados corretamente. Esta migration apenas:
--     1. Atualiza o COMMENT da coluna para refletir as novas regras.
--     2. Verifica inconsistências nos dados existentes antes que a migration 014
--        renomeie a coluna de nivel → anos_academicos.
--
-- ATENÇÃO: Execute com cuidado em produção se houver matérias de medio/superior
--   com mais de 1 item em nivel — corrija os dados antes de aplicar o CHECK.
-- ============================================================================

BEGIN;

-- 1. Atualizar comentário da coluna
COMMENT ON COLUMN projection_materias.nivel IS
    'AnosAcademicos da matéria disciplinar. '
    'fundamental: 1–9 itens (1_fundamental…nono_fundamental). '
    'medio/superior: exatamente 1 item — ano do curso ao qual a matéria pertence. '
    'Armazenado como JSONB array de strings.';

-- 2. Verificar se existem matérias de medio/superior com nivel com mais de 1 item
--    (caso existam, CORRIJA antes de aplicar o CHECK abaixo)
DO $$
DECLARE
    v_count INTEGER;
BEGIN
    SELECT COUNT(*)
    INTO v_count
    FROM projection_materias
    WHERE type IN ('medio', 'superior')
      AND jsonb_array_length(COALESCE(nivel, '[]'::jsonb)) != 1;

    IF v_count > 0 THEN
        RAISE WARNING
            '⚠️  Existem % matérias de medio/superior com nivel diferente de 1 item. '
            'Corrija os dados antes de ativar a CHECK CONSTRAINT.',
            v_count;
    ELSE
        RAISE NOTICE '✅ Nenhuma inconsistência encontrada em nivel de medio/superior.';
    END IF;
END $$;

-- 3. Adicionar CHECK CONSTRAINT (descomente após verificar que não há inconsistências)
-- ALTER TABLE projection_materias
--     ADD CONSTRAINT chk_nivel_cardinalidade CHECK (
--         (type = 'fundamental')
--         OR
--         (type IN ('medio', 'superior') AND jsonb_array_length(COALESCE(nivel, '[]'::jsonb)) = 1)
--     );

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 013 — semântica de anos_academicos em projection_materias aplicada.';
END $$;
-- ===== END 013_anos_academicos_materia.sql =====

-- ===== BEGIN 014_materias_nivel_to_anos_academicos.sql =====
-- ============================================================================
-- MIGRATION 014 - Renomear 'nivel' → 'anos_academicos' em projection_materias
-- ============================================================================

BEGIN;

-- 1. Renomear coluna somente se ainda existir como 'nivel'
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'projection_materias'
          AND column_name = 'nivel'
    ) THEN
        ALTER TABLE projection_materias RENAME COLUMN nivel TO anos_academicos;
        RAISE NOTICE 'Coluna nivel renomeada para anos_academicos em projection_materias';
    ELSE
        RAISE NOTICE 'Coluna nivel não existe (já renomeada ou inexistente), pulando';
    END IF;
END $$;

-- 2. Atualizar comentário da coluna
COMMENT ON COLUMN projection_materias.anos_academicos IS
    'Apenas para type=fundamental: array JSON com os anos académicos '
    'que esta matéria cobre. Ex: ["1_fundamental","segundo_fundamental"]. '
    'NULL para medio e superior (que usam CursoID).';

-- 3. Atualizar índice GIN (caso exista baseado no nome antigo)
DROP INDEX IF EXISTS idx_materias_nivel;
CREATE INDEX IF NOT EXISTS idx_materias_anos_academicos
    ON projection_materias USING GIN (anos_academicos)
    WHERE anos_academicos IS NOT NULL;

COMMIT;

DO $$ BEGIN RAISE NOTICE '✅ MIGRATION 014 - nivel → anos_academicos em projection_materias'; END $$;
-- ===== END 014_materias_nivel_to_anos_academicos.sql =====

-- ===== BEGIN 015_add_periodos_to_cursos.sql =====
-- ============================================================================
-- MIGRATION 015 — Adicionar campo "periodos" em projection_cursos
--
-- REGRA DE NEGÓCIO:
--   • type = 'superior' → periodos é OBRIGATÓRIO (array JSON com 1+ itens)
--   • type = 'medio'    → periodos é NULL (escolas usam trimestres fixos)
--
-- Valores permitidos por item: 1_trimestre | 2_trimestre | 3_trimestre |
--                              1_semestre  | 2_semestre
-- ============================================================================

BEGIN;

-- 1. Adicionar coluna periodos (nullable — medio não usa)
ALTER TABLE projection_cursos
    ADD COLUMN IF NOT EXISTS periodos JSONB DEFAULT NULL;

COMMENT ON COLUMN projection_cursos.periodos IS
    'Apenas para type=superior: array JSON com os períodos letivos do curso. '
    'Ex: ["1_semestre","2_semestre"]. NULL para tipo medio.';

-- 2. Criar índice GIN para buscas por período
CREATE INDEX IF NOT EXISTS idx_cursos_periodos
    ON projection_cursos USING GIN (periodos)
    WHERE periodos IS NOT NULL;

-- 3. Verificar cursos superiores sem periodos (dados existentes — atenção!)
DO $$
DECLARE
    v_count INTEGER;
BEGIN
    SELECT COUNT(*)
    INTO v_count
    FROM projection_cursos
    WHERE type = 'superior'
      AND (periodos IS NULL OR periodos = 'null'::jsonb OR jsonb_array_length(COALESCE(periodos, '[]'::jsonb)) = 0);

    IF v_count > 0 THEN
        RAISE WARNING
            '⚠️  Existem % cursos superiores sem periodos definidos. '
            'Atualize-os via PUT /academia/cursos/:id antes de registrar novas notas.',
            v_count;
    ELSE
        RAISE NOTICE '✅ Nenhum curso superior sem periodos encontrado.';
    END IF;
END $$;

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 015 — periodos adicionado em projection_cursos.';
END $$;
-- ===== END 015_add_periodos_to_cursos.sql =====

-- ===== BEGIN 016_avaliacao_final.sql =====
-- ============================================================
-- MIGRATION 016 - projection_avaliacao_final
-- Substitui projection_aprovacao_ano + projection_reprovacoes
-- ============================================================

BEGIN;

CREATE TABLE IF NOT EXISTS projection_avaliacao_final (
    id                    UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    event_id              UUID        NOT NULL,

    codigo_estudante      VARCHAR(7)  NOT NULL,
    codigo_academia       VARCHAR(50) NOT NULL,

    ano_lectivo           VARCHAR(20) NOT NULL,
    tipo_ensino           VARCHAR(20) NOT NULL
                              CHECK (tipo_ensino IN ('fundamental', 'medio', 'superior')),

    ano_academico_atual   VARCHAR(50) NOT NULL,
    proximo_ano_academico VARCHAR(50),          -- NULL = último ano ou reprovado

    aprovado              BOOLEAN     NOT NULL DEFAULT FALSE,
    observacao            TEXT,

    registered_at         TIMESTAMP   NOT NULL DEFAULT CURRENT_TIMESTAMP,
    version               INTEGER     NOT NULL,

    -- Uma avaliação por estudante/academia/ano_lectivo/tipo_ensino
    UNIQUE (codigo_estudante, codigo_academia, ano_lectivo, tipo_ensino),

    FOREIGN KEY (codigo_academia)
        REFERENCES projection_academias(codigo_academia)
        ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_avf_estudante  ON projection_avaliacao_final(codigo_estudante);
CREATE INDEX IF NOT EXISTS idx_avf_academia   ON projection_avaliacao_final(codigo_academia);
CREATE INDEX IF NOT EXISTS idx_avf_ano        ON projection_avaliacao_final(ano_lectivo);
CREATE INDEX IF NOT EXISTS idx_avf_tipo       ON projection_avaliacao_final(tipo_ensino);
CREATE INDEX IF NOT EXISTS idx_avf_aprovado   ON projection_avaliacao_final(aprovado);

INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at)
VALUES ('avaliacao_final', 0, CURRENT_TIMESTAMP)
ON CONFLICT (projection_name) DO NOTHING;

COMMENT ON TABLE  projection_avaliacao_final IS
    'Avaliações finais de ano acadêmico — substitui projection_aprovacao_ano e projection_reprovacoes';
COMMENT ON COLUMN projection_avaliacao_final.aprovado IS
    'TRUE = aprovado (avança ou finaliza ciclo); FALSE = reprovado (só registra)';
COMMENT ON COLUMN projection_avaliacao_final.proximo_ano_academico IS
    'Próximo nível. NULL se último ano do ciclo ou se reprovado.';
COMMENT ON COLUMN projection_avaliacao_final.observacao IS
    'Justificativa obrigatória para forçar aprovação com notas ausentes.';

COMMIT;
-- ===== END 016_avaliacao_final.sql =====

-- ===== BEGIN 017_avaliacao_turma.sql =====
-- ============================================================
-- MIGRATION 017 - Adicionar campo codigo_turma em projection_avaliacao_final
-- ============================================================

BEGIN;

ALTER TABLE projection_avaliacao_final
    ADD COLUMN IF NOT EXISTS codigo_turma VARCHAR(50);

COMMENT ON COLUMN projection_avaliacao_final.codigo_turma IS
    'Código da turma do estudante no momento da avaliação. NULL se não estava em nenhuma turma.';

CREATE INDEX IF NOT EXISTS idx_avf_turma ON projection_avaliacao_final(codigo_turma);

COMMIT;

DO $$ BEGIN RAISE NOTICE '✅ MIGRATION 017 CONCLUÍDA - codigo_turma adicionado em projection_avaliacao_final'; END $$;
-- ===== END 017_avaliacao_turma.sql =====

-- ===== BEGIN 018_materia_periodo.sql =====
-- ============================================================================
-- MIGRATION 018 — Adicionar campo 'periodo' em projection_materias
--
-- REGRAS:
--   • Apenas matérias do type='superior' usam este campo.
--   • Valores aceitos: 1_semestre, 2_semestre (subconjunto dos periodos do curso).
--   • NULL para type='fundamental' e type='medio'.
--   • Matérias superior são criadas como 'inativo' e só podem ser ativadas
--     quando periodo estiver preenchido (regra aplicada no domínio Go).
-- ============================================================================

BEGIN;

-- 1. Adicionar coluna
ALTER TABLE projection_materias
    ADD COLUMN IF NOT EXISTS periodo VARCHAR(20) DEFAULT NULL;

-- 2. Constraint: período só pode ser um dos valores válidos (ou NULL)
ALTER TABLE projection_materias
    DROP CONSTRAINT IF EXISTS chk_materia_periodo_valores;

ALTER TABLE projection_materias
    ADD CONSTRAINT chk_materia_periodo_valores CHECK (
        periodo IS NULL
        OR periodo IN ('1_trimestre', '2_trimestre', '3_trimestre', '1_semestre', '2_semestre')
    );

-- 3. Constraint: apenas matérias superior podem ter período preenchido
ALTER TABLE projection_materias
    DROP CONSTRAINT IF EXISTS chk_materia_periodo_tipo;

ALTER TABLE projection_materias
    ADD CONSTRAINT chk_materia_periodo_tipo CHECK (
        (type = 'superior')
        OR
        (type IN ('fundamental', 'medio') AND periodo IS NULL)
    );

-- 4. Comentário
COMMENT ON COLUMN projection_materias.periodo IS
    'Período letivo da matéria. Obrigatório para ativar matérias do type=superior. '
    'Deve ser um dos períodos definidos no curso vinculado. '
    'NULL para type=fundamental e type=medio.';

-- 5. Índice
CREATE INDEX IF NOT EXISTS idx_materias_periodo
    ON projection_materias(periodo)
    WHERE periodo IS NOT NULL;

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 018 — campo periodo adicionado em projection_materias';
END $$;
-- ===== END 018_materia_periodo.sql =====

-- ===== BEGIN 019_soft_delete_auditavel.sql =====
-- ============================================================
-- MIGRATION 019 — Suporte a deleção auditável (soft delete)
-- Afeta: projection_turmas, projection_cursos, projection_materias
-- ============================================================
--
-- CONTEXTO:
--   Deleção física (DELETE FROM) é incompatível com event sourcing imutável:
--   o ledger guarda eventos para sempre, então a projeção deve refletir
--   o soft delete via campo deleted_at, mantendo o registro para rebuild.
--
-- O QUE ESTA MIGRATION FAZ:
--   1. projection_turmas  — adiciona deleted_at, atualiza CHECK de status
--      para incluir 'deletado', cria índice parcial.
--   2. projection_cursos  — idem.
--   3. projection_materias — idem. (Anteriormente usava DELETE físico;
--      esta migration converte para soft delete para garantir consistência
--      de rebuild via evento MateriaDeletada.)
-- ============================================================

-- ── projection_turmas ──────────────────────────────────────────────────────

ALTER TABLE projection_turmas
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP DEFAULT NULL;

-- Adicionar 'deletado' como valor permitido no CHECK de status
ALTER TABLE projection_turmas
    DROP CONSTRAINT IF EXISTS projection_turmas_status_check;

ALTER TABLE projection_turmas
    ADD CONSTRAINT projection_turmas_status_check
        CHECK (status IN ('ativo', 'inativo', 'deletado'));

-- Índice para filtrar registros não-deletados eficientemente
CREATE INDEX IF NOT EXISTS idx_turmas_not_deleted
    ON projection_turmas (codigo_academia)
    WHERE deleted_at IS NULL;

COMMENT ON COLUMN projection_turmas.deleted_at IS
    'Preenchido via evento TurmaDeletada — registro mantido para auditoria';

-- ── projection_cursos ──────────────────────────────────────────────────────

ALTER TABLE projection_cursos
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP DEFAULT NULL;

ALTER TABLE projection_cursos
    DROP CONSTRAINT IF EXISTS projection_cursos_status_check;

ALTER TABLE projection_cursos
    ADD CONSTRAINT projection_cursos_status_check
        CHECK (status IN ('ativo', 'inativo', 'deletado'));

CREATE INDEX IF NOT EXISTS idx_cursos_not_deleted
    ON projection_cursos (codigo_academia)
    WHERE deleted_at IS NULL;

COMMENT ON COLUMN projection_cursos.deleted_at IS
    'Preenchido via evento CursoDeletado — registro mantido para auditoria';

-- ── projection_materias — convertida de DELETE físico para soft delete ─────
--
-- NOTA (ERRO-MIG-03 FIX): O comentário original desta migration dizia
-- "projection_materias já tem suporte via DELETE físico" e tratava a
-- conversão para soft delete como opcional. Isso era contraditório pois
-- o corpo da migration adiciona deleted_at e atualiza o CHECK de status.
-- Soft delete em projection_materias é OBRIGATÓRIO para que o evento
-- MateriaDeletada possa ser reprocessado corretamente em um rebuild.

ALTER TABLE projection_materias
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP DEFAULT NULL;

ALTER TABLE projection_materias
    DROP CONSTRAINT IF EXISTS projection_materias_status_check;

ALTER TABLE projection_materias
    ADD CONSTRAINT projection_materias_status_check
        CHECK (status IN ('ativo', 'inativo', 'deletado'));

CREATE INDEX IF NOT EXISTS idx_materias_not_deleted
    ON projection_materias (codigo_academia)
    WHERE deleted_at IS NULL;

COMMENT ON COLUMN projection_materias.deleted_at IS
    'Preenchido via evento MateriaDeletada — registro mantido para auditoria';

-- ── Checkpoints existentes não precisam de nova entrada ────────────────────

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 019 CONCLUÍDA — soft delete auditável para turmas, cursos e matérias';
END $$;
-- ===== END 019_soft_delete_auditavel.sql =====

-- ===== BEGIN 020_fix_verify_hash_chain.sql =====
-- ============================================
-- MIGRATION 020 - Corrigir verify_hash_chain e remover StatusEscolarAtualizado
-- ============================================
-- 
-- Contexto:
-- 1. A função verify_hash_chain já existe na migration 001, mas o Go lê
--    broken_at_version como *int via Scan — a função retorna INTEGER ok.
--    Recriamos aqui para garantir a assinatura correta (idempotente).
-- 2. Remove da lógica qualquer referência ao evento StatusEscolarAtualizado
--    (removido na migration 008 em favor de StatusEscolarFundamentalAtualizado
--    e StatusEscolarMedioAtualizado).
-- ============================================

BEGIN;

-- ============================================
-- 1. Recriar verify_hash_chain com assinatura explícita
--    Recalcula cada ledger_hash a partir dos dados originais e valida.
--    A função Go VerifyLedgerIntegrity chama:
--      SELECT is_valid, broken_at_version, message FROM verify_hash_chain('uuid')
-- ============================================

CREATE OR REPLACE FUNCTION verify_hash_chain(p_aggregate_id UUID)
RETURNS TABLE(
    is_valid          BOOLEAN,
    broken_at_version INTEGER,
    message           TEXT
) AS $$
DECLARE
    v_current_hash  VARCHAR(64);
    v_event         RECORD;
    v_expected_hash VARCHAR(64);
BEGIN
    FOR v_event IN
        SELECT *
        FROM spuri_ledger
        WHERE aggregate_id = p_aggregate_id
        ORDER BY event_version ASC
    LOOP
        -- Recalcula o hash esperado com os dados armazenados
        v_expected_hash := generate_ledger_hash(
            v_event.event_id,
            v_event.aggregate_id,
            v_event.event_type,
            v_event.payload,
            v_event.previous_hash
        );

        -- Hash armazenado difere do recalculado → payload adulterado
        IF v_event.ledger_hash != v_expected_hash THEN
            is_valid          := FALSE;
            broken_at_version := v_event.event_version;
            message           := format(
                'Hash inválido no evento version=%s (event_id=%s): esperado=%s armazenado=%s',
                v_event.event_version, v_event.event_id, v_expected_hash, v_event.ledger_hash
            );
            RETURN NEXT;
            RETURN;
        END IF;

        -- Cadeia quebrada: previous_hash do evento atual ≠ ledger_hash do anterior
        IF v_event.event_version > 1 AND v_event.previous_hash IS DISTINCT FROM v_current_hash THEN
            is_valid          := FALSE;
            broken_at_version := v_event.event_version;
            message           := format(
                'Cadeia de hashes quebrada no event_version=%s: previous_hash=%s esperado=%s',
                v_event.event_version, v_event.previous_hash, v_current_hash
            );
            RETURN NEXT;
            RETURN;
        END IF;

        v_current_hash := v_event.ledger_hash;
    END LOOP;

    -- Sem eventos ou todos válidos
    is_valid          := TRUE;
    broken_at_version := NULL;
    message           := 'Cadeia de hashes íntegra';
    RETURN NEXT;
END;
$$ LANGUAGE plpgsql STABLE;

COMMENT ON FUNCTION verify_hash_chain(UUID) IS
    'Verifica integridade do ledger para um agregado: recalcula cada ledger_hash
     e valida a cadeia de previous_hash. Detecta adulteração de payload mesmo que
     o hash tenha sido reescrito junto.';

-- ============================================
-- 2. Garantir que projection_checkpoints tem
--    entrada para todas as projeções ativas
-- ============================================

INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at)
VALUES
    ('estudantes',       0, CURRENT_TIMESTAMP),
    ('academias',        0, CURRENT_TIMESTAMP),
    ('admins',           0, CURRENT_TIMESTAMP),
    ('notas',            0, CURRENT_TIMESTAMP),
    ('faltas',           0, CURRENT_TIMESTAMP),
    ('inscricoes',       0, CURRENT_TIMESTAMP),
    ('cursos',           0, CURRENT_TIMESTAMP),
    ('materias',         0, CURRENT_TIMESTAMP),
    ('turmas',           0, CURRENT_TIMESTAMP),
    ('reprovacoes',      0, CURRENT_TIMESTAMP),
    ('aprovacao_ano',    0, CURRENT_TIMESTAMP),
    ('categorias_nota',  0, CURRENT_TIMESTAMP)
ON CONFLICT (projection_name) DO NOTHING;

COMMIT;

DO $$ BEGIN RAISE NOTICE '✅ MIGRATION 020 - verify_hash_chain recriada com recálculo de hash real'; END $$;
-- ===== END 020_fix_verify_hash_chain.sql =====

-- ===== BEGIN 021_fix_projection_notas.sql =====
-- ============================================================================
-- MIGRATION 021 — Alinhamento da projection_notas com o padrão do sistema
--
-- PROBLEMA CORRIGIDO:
--   A NotasProjection estava escutando eventos inexistentes ("NotaRegistrada",
--   "NotaCorrigida", "NotaEliminada") em vez dos eventos reais emitidos pelo
--   aggregate ("NotasRegistradas", "NotaAtualizada"). Resultado: a tabela
--   projection_notas nunca era atualizada.
--
-- TAMBÉM CORRIGIDO:
--   O antigo handleNotaEliminada usava DELETE físico. Esta migration garante
--   que a tabela tem coluna deleted_at para futuros soft-deletes, consistente
--   com o restante do sistema.
--
-- AÇÃO REQUERIDA APÓS ESTA MIGRATION:
--   Execute um rebuild da projeção "notas" via:
--     POST /admin/rebuild-projection/notas
--   Isso vai reprocessar todos os eventos "NotasRegistradas" e "NotaAtualizada"
--   do ledger e popular corretamente a tabela projection_notas.
-- ============================================================================

BEGIN;

-- 1. Adicionar coluna deleted_at (soft delete) se não existir
ALTER TABLE projection_notas
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP DEFAULT NULL;

COMMENT ON COLUMN projection_notas.deleted_at IS
    'Soft delete — preenchido quando a nota é removida logicamente. '
    'Registros com deleted_at != NULL não aparecem nas queries de leitura padrão.';

-- 2. Índice para queries de notas ativas (sem soft delete)
CREATE INDEX IF NOT EXISTS idx_notas_estudante_ativo
    ON projection_notas (codigo_estudante)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_notas_academia_ativo
    ON projection_notas (codigo_academia)
    WHERE deleted_at IS NULL;

-- 3. Verificar e reportar dados existentes na tabela
DO $$
DECLARE
    v_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO v_count FROM projection_notas;

    IF v_count = 0 THEN
        RAISE NOTICE
            '✅ projection_notas está vazia — consistente com o bug corrigido. '
            'Execute POST /admin/rebuild-projection/notas para reprocessar o ledger.';
    ELSE
        RAISE WARNING
            '⚠️  projection_notas contém % registros existentes. '
            'Verifique se foram inseridos manualmente ou por outro mecanismo. '
            'Considere executar POST /admin/rebuild-projection/notas para garantir consistência.',
            v_count;
    END IF;
END $$;

-- 4. Atualizar checkpoint da projeção notas para 0,
--    forçando reprocessamento completo no próximo rebuild.
--    (O rebuild via HTTP já faz TRUNCATE + reprocessamento, mas este
--     UPDATE garante que o polling também reprocesse do início se necessário.)
UPDATE projection_checkpoints
SET last_processed_event_id = 0,
    last_processed_at       = CURRENT_TIMESTAMP
WHERE projection_name = 'notas';

-- Se não existir o checkpoint, criar
INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at, events_processed)
VALUES ('notas', 0, CURRENT_TIMESTAMP, 0)
ON CONFLICT (projection_name) DO NOTHING;

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 021 — projection_notas alinhada. Execute rebuild da projeção notas.';
END $$;
-- ===== END 021_fix_projection_notas.sql =====

-- ===== BEGIN 022_reforcar_anos_academicos_constraint.sql =====
-- ============================================================================
-- MIGRATION 022 - Reforçar constraint anos_academicos em projection_academias
-- ============================================================================

BEGIN;

-- 1. PRIMEIRO: remover a constraint antiga ANTES dos UPDATEs
--    (a constraint antiga bloqueia SET anos_academicos = NULL em misto/fundamental)
ALTER TABLE projection_academias
    DROP CONSTRAINT IF EXISTS check_anos_academicos_nivel;

-- 2. Limpar dados inválidos: fundamental/misto com NULL ou array vazio → NULL
UPDATE projection_academias
SET anos_academicos = NULL
WHERE nivel_escolar IN ('fundamental', 'misto')
  AND (
      anos_academicos IS NULL
      OR jsonb_array_length(anos_academicos) = 0
  );

-- 3. Limpar dados inválidos: medio/superior com anos_academicos preenchido → NULL
UPDATE projection_academias
SET anos_academicos = NULL
WHERE (nivel_escolar = 'medio' OR nivel_escolar IS NULL)
  AND anos_academicos IS NOT NULL;

-- 4. Auditoria: avisar quantas academias ficaram sem anos válidos
DO $$
DECLARE
    v_count INTEGER;
BEGIN
    SELECT COUNT(*)
    INTO v_count
    FROM projection_academias
    WHERE nivel_escolar IN ('fundamental', 'misto')
      AND anos_academicos IS NULL;

    IF v_count > 0 THEN
        RAISE WARNING
            '⚠️  % academia(s) fundamental/misto sem anos_academicos válidos. '
            'O admin deve corrigir via AtualizarAnosAcademicos.',
            v_count;
    END IF;
END $$;

-- 5. Adicionar constraint restritiva:
--    - fundamental/misto: NULL (legado pendente de correção) OU array não-vazio
--    - medio: anos_academicos deve ser NULL
--    - superior (nivel_escolar IS NULL): anos_academicos deve ser NULL
ALTER TABLE projection_academias
    ADD CONSTRAINT check_anos_academicos_nivel CHECK (
        (
            nivel_escolar IN ('fundamental', 'misto')
            AND (
                anos_academicos IS NULL
                OR jsonb_array_length(anos_academicos) > 0
            )
        )
        OR
        (
            nivel_escolar = 'medio'
            AND anos_academicos IS NULL
        )
        OR
        (
            nivel_escolar IS NULL
            AND anos_academicos IS NULL
        )
    );

-- 6. Recriar índice GIN
DROP INDEX IF EXISTS idx_academia_anos_academicos;
CREATE INDEX IF NOT EXISTS idx_academia_anos_academicos
    ON projection_academias USING GIN (anos_academicos)
    WHERE anos_academicos IS NOT NULL;

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 022 - Constraint anos_academicos reforçada com sucesso';
END $$;
-- ===== END 022_reforcar_anos_academicos_constraint.sql =====

-- ===== BEGIN 023_admin_senha_alterada.sql =====
-- ============================================
-- MIGRATION 023 - Suporte ao evento AdminSenhaAlterada
-- Data: 2026
-- ============================================
-- 
-- Contexto:
-- AlterarSenha e ResetarSenha faziam UPDATE direto em projection_admins,
-- bypassando o event sourcing. Esta migration suporta a correção:
-- agora ambos emitem o evento AdminSenhaAlterada que é gravado no ledger
-- e processado pela projeção, garantindo rastreabilidade completa e
-- que o rebuild restaure a senha correta.
--
-- Não há alterações de schema nesta migration porque:
-- 1. spuri_ledger já aceita qualquer event_type (a validação é no Go)
-- 2. projection_admins já tem a coluna senha_hash
-- 3. A whitelist de eventos é controlada em safe_queries.go (Go)
-- ============================================

BEGIN;

-- ============================================
-- 1. Garantir que projection_checkpoints tem
--    entrada para admins (idempotente)
-- ============================================

INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at)
VALUES ('admins', 0, CURRENT_TIMESTAMP)
ON CONFLICT (projection_name) DO NOTHING;

-- ============================================
-- 2. Comentário informativo na tabela
-- ============================================

COMMENT ON TABLE projection_admins IS
    'Projeção de leitura para administradores. '
    'Atualizada por eventos: AdminCriado, AdminAtivado, AdminDesativado, '
    'AcaoAdminRegistrada, AdminDadosAtualizados, AdminRoleAtualizado, '
    'EmailVerificado, AdminSenhaAlterada (adicionado migration 023).';

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 023 - AdminSenhaAlterada habilitado no pipeline de events';
    RAISE NOTICE '   Ação necessária no Go: adicionar "AdminSenhaAlterada": true em safe_queries.go';
END $$;
-- ===== END 023_admin_senha_alterada.sql =====

-- ===== BEGIN 024_remove_inscricoes_sistema.sql =====
-- ============================================================================
-- MIGRATION 024: Remove sistema de inscrição de estudante em academia
-- ============================================================================
-- CONTEXTO:
--   O sistema passou a cadastrar estudantes DIRETAMENTE vinculados a uma
--   academia (via POST /academia/estudante/register + CriarComVinculo).
--   O fluxo de inscrição (SolicitarInscricao → AprovarInscricao → Vincular)
--   foi removido do código — handlers, aggregate methods e rotas eliminados.
--
-- O QUE ESTA MIGRATION FAZ:
--   1. Preserva a tabela projection_inscricoes (dados históricos do ledger)
--   2. Adiciona comentário de depreciação
--   3. Remove índices de busca frequente (não há mais novas inscrições)
--   4. Garante que anos_academicos existe em projection_academias (scanAcademia)
-- ============================================================================

-- 1. Garantir que a coluna anos_academicos existe em projection_academias
--    (pode já existir em migrations anteriores — IF NOT EXISTS é idempotente)
ALTER TABLE projection_academias
    ADD COLUMN IF NOT EXISTS anos_academicos JSONB DEFAULT '[]'::jsonb;

-- 2. Comentário de depreciação na tabela de inscrições
COMMENT ON TABLE projection_inscricoes IS
    '[DEPRECIADO desde migration 024] '
    'O sistema de inscrição foi removido. '
    'Estudantes são cadastrados diretamente vinculados à academia via EstudanteCriadoComVinculo. '
    'Esta tabela é mantida apenas para preservar dados históricos do ledger.';

-- 3. Remover índices de busca frequente de inscrições (não haverá novas escritas)
--    Usa IF EXISTS para ser idempotente
DROP INDEX IF EXISTS idx_projection_inscricoes_status;
DROP INDEX IF EXISTS idx_projection_inscricoes_academia;
DROP INDEX IF EXISTS idx_projection_inscricoes_estudante;

-- 4. Checkpoint: garantir que aprovacao_ano e reprovacoes estão registrados
INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at, events_processed)
VALUES
    ('aprovacao_ano', 0, CURRENT_TIMESTAMP, 0),
    ('reprovacoes',   0, CURRENT_TIMESTAMP, 0)
ON CONFLICT (projection_name) DO NOTHING;

-- ============================================================================
-- FIM DA MIGRATION 024
-- ============================================================================
-- ===== END 024_remove_inscricoes_sistema.sql =====

-- ===== BEGIN 025_admin_email_unique_index.sql =====
-- ============================================================================
-- MIGRATION 025 — Constraint única para Bootstrap FPP
--
-- PROBLEMA CORRIGIDO (Issue #3 — auditoria Março 2026):
--   Race condition TOCTOU no BootstrapAdminFPP: duas requisições simultâneas
--   podiam ambas ver `len(admins) == 0` e criar dois admins FPP distintos.
--
-- SOLUÇÃO:
--   1. Unique partial index: no máximo 1 admin com role='fpp' E created_by IS NULL.
--      Isso garante que apenas o admin FPP de bootstrap (sem criador) seja único.
--      Admins FPP criados via RegisterAdmin têm created_by != NULL e não são
--      afetados por este índice.
--
--   2. O advisory lock em Go (pg_advisory_lock) já serializa as requisições.
--      Este índice é a segunda linha de defesa (defense in depth).
--
-- NOTA: Admins FPP criados via /admin/register têm created_by preenchido
--       e NÃO são afetados por este índice — podem existir múltiplos.
-- ============================================================================

BEGIN;

-- Constraint: apenas 1 admin com role='fpp' e sem criador (bootstrap)
CREATE UNIQUE INDEX IF NOT EXISTS idx_bootstrap_fpp_unique
    ON projection_admins (role)
    WHERE created_by IS NULL;

COMMENT ON INDEX idx_bootstrap_fpp_unique IS
    'Garante que apenas um admin FPP de bootstrap (created_by IS NULL) exista. '
    'Segunda linha de defesa contra race condition em BootstrapAdminFPP.';

-- Checkpoint da migration
INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at, events_processed)
VALUES ('migration_025', 0, CURRENT_TIMESTAMP, 0)
ON CONFLICT (projection_name) DO NOTHING;

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 025 — Constraint bootstrap FPP aplicada com sucesso.';
END $$;
-- ===== END 025_admin_email_unique_index.sql =====

-- ===== BEGIN 026_academia_motivo_desativacao.sql =====
-- ============================================================================
-- MIGRATION 026: Adicionar motivo_desativacao em projection_academias
-- ============================================================================
-- CONTEXTO (FIX E-10):
--   O evento AcademiaDesativada contém o campo Motivo, mas a projeção não
--   persistia esse valor. Para consultar o motivo era necessário inspecionar
--   o ledger diretamente.
--
--   Esta migration adiciona a coluna motivo_desativacao para que o handler
--   handleAcademiaDesativada possa persistir o motivo na projeção.
-- ============================================================================

BEGIN;

ALTER TABLE projection_academias
    ADD COLUMN IF NOT EXISTS motivo_desativacao TEXT;

COMMENT ON COLUMN projection_academias.motivo_desativacao IS
    'Motivo fornecido pelo admin ao desativar a academia. '
    'NULL quando ativa. Populado pelo evento AcademiaDesativada (migration 026).';

-- Índice opcional para auditoria de desativações
CREATE INDEX IF NOT EXISTS idx_academia_motivo_desativacao
    ON projection_academias(id)
    WHERE motivo_desativacao IS NOT NULL;

-- Garantir checkpoint da projeção academias
INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at, events_processed)
VALUES ('academias', 0, CURRENT_TIMESTAMP, 0)
ON CONFLICT (projection_name) DO NOTHING;

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 026 - motivo_desativacao adicionado a projection_academias';
    RAISE NOTICE '   Ação necessária: executar rebuild da projeção academias para repopular o campo';
    RAISE NOTICE '   POST /admin/rebuild-projection/academias';
END $$;
-- ===== END 026_academia_motivo_desativacao.sql =====

-- ===== BEGIN 027_academia_senha_alterada.sql =====
-- ============================================================================
-- MIGRATION 027 - Suporte ao evento AcademiaSenhaAlterada
-- Data: 2026
-- ============================================================================
--
-- Contexto (FIX C1 da auditoria spuri-backend):
--   AlterarSenha e ResetarSenha faziam UPDATE direto em projection_academias,
--   bypassando o event sourcing. Esta migration suporta a correção:
--   agora ambos emitem o evento AcademiaSenhaAlterada que é gravado no ledger
--   e processado pela AcademiaProjection, garantindo:
--     1. Rastreabilidade completa no ledger (quem/quando/por quê)
--     2. Rebuild restaura a senha correta (não volta à senha original)
--     3. Consistência com AdminSenhaAlterada (migration 023)
--
-- Não há alterações de schema nesta migration porque:
--   1. spuri_ledger já aceita qualquer event_type (validação é no Go)
--   2. projection_academias já tem a coluna senha_hash
--   3. A whitelist de eventos é controlada em safe_queries.go (Go)
-- ============================================================================

BEGIN;

-- ============================================================================
-- 1. Garantir que projection_checkpoints tem
--    entrada para academias (idempotente)
-- ============================================================================

INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at)
VALUES ('academias', 0, CURRENT_TIMESTAMP)
ON CONFLICT (projection_name) DO NOTHING;

-- ============================================================================
-- 2. Comentário informativo na tabela
-- ============================================================================

COMMENT ON TABLE projection_academias IS
    'Projeção de leitura para academias. '
    'Atualizada por eventos: AcademiaCriada, AcademiaAtivada, AcademiaDesativada, '
    'AcademiaDadosAtualizados, CursosAtualizados, EmailVerificado, '
    'AcademiaSenhaAlterada (adicionado migration 027 — FIX C1).';

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 027 - AcademiaSenhaAlterada habilitado no pipeline de eventos';
    RAISE NOTICE '   Whitelist atualizada em safe_queries.go: "AcademiaSenhaAlterada": true';
    RAISE NOTICE '   Handler adicionado em academia_projection.go: handleAcademiaSenhaAlterada';
    RAISE NOTICE '   Rebuild recomendado para garantir consistência:';
    RAISE NOTICE '   POST /admin/rebuild-projection/academias';
END $$;
-- ===== END 027_academia_senha_alterada.sql =====

-- ===== BEGIN 028_fix_estudante_email_verificado.sql =====
-- ============================================================================
-- MIGRATION 028 - FIX-C3: EmailVerificadoEstudante via event sourcing
-- ============================================================================
-- Esta migration não altera schema diretamente — a coluna email_verificado
-- já existe em projection_estudantes.
--
-- O que muda:
--   1. A coluna email_verificado agora é atualizada via evento
--      EmailVerificadoEstudante → handler na projeção (não mais UPDATE direto).
--   2. Adicionamos índice único em bilhete_identidade para garantir integridade
--      na camada de banco (defesa em profundidade além da validação no handler).
--
-- Safe para executar em produção com dados existentes.
-- ============================================================================

BEGIN;

-- Garantir que email_verificado existe e tem NOT NULL com DEFAULT FALSE
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'projection_estudantes'
          AND column_name = 'email_verificado'
    ) THEN
        ALTER TABLE projection_estudantes
            ADD COLUMN email_verificado BOOLEAN NOT NULL DEFAULT FALSE;
        RAISE NOTICE '✅ Coluna email_verificado adicionada em projection_estudantes';
    ELSE
        -- Garantir NOT NULL e DEFAULT
        ALTER TABLE projection_estudantes
            ALTER COLUMN email_verificado SET NOT NULL,
            ALTER COLUMN email_verificado SET DEFAULT FALSE;
        RAISE NOTICE 'ℹ️  Coluna email_verificado já existe em projection_estudantes';
    END IF;
END $$;

-- Preencher NULLs existentes
UPDATE projection_estudantes
SET email_verificado = FALSE
WHERE email_verificado IS NULL;

-- Índice para lookup por bilhete_identidade (usado na validação de unicidade)
CREATE INDEX IF NOT EXISTS idx_estudante_bilhete_identidade
    ON projection_estudantes (bilhete_identidade)
    WHERE bilhete_identidade IS NOT NULL;

-- Índice para lookup por email (usado no login e recuperação de senha)
CREATE INDEX IF NOT EXISTS idx_estudante_email
    ON projection_estudantes (email)
    WHERE email IS NOT NULL;

-- Índice para lookup por codigo_academia (usado em ListarEstudantes)
CREATE INDEX IF NOT EXISTS idx_estudante_codigo_academia
    ON projection_estudantes (codigo_academia)
    WHERE codigo_academia IS NOT NULL;

COMMIT;

-- Verificação
DO $$
DECLARE
    v_total INTEGER;
    v_verificados INTEGER;
BEGIN
    SELECT COUNT(*) INTO v_total FROM projection_estudantes;
    SELECT COUNT(*) INTO v_verificados FROM projection_estudantes WHERE email_verificado = TRUE;
    RAISE NOTICE '══════════════════════════════════════════════════';
    RAISE NOTICE '✅ MIGRATION 028 CONCLUÍDA';
    RAISE NOTICE '══════════════════════════════════════════════════';
    RAISE NOTICE 'Total estudantes: %', v_total;
    RAISE NOTICE 'Emails verificados: %', v_verificados;
    RAISE NOTICE '══════════════════════════════════════════════════';
END $$;
-- ===== END 028_fix_estudante_email_verificado.sql =====

-- ===== BEGIN 029_fix_ledger_truncate_protection.sql =====
-- ============================================================================
-- MIGRATION 029 — Proteção de spuri_ledger contra TRUNCATE
-- ============================================================================
--
-- CONTEXTO (ERRO-MIG-04 da auditoria-etapa2-db.md):
--   Os triggers prevent_update_ledger e prevent_delete_ledger cobrem apenas
--   UPDATE e DELETE (FOR EACH ROW). TRUNCATE em PostgreSQL não dispara triggers
--   FOR EACH ROW — requer trigger FOR EACH STATEMENT com a cláusula TRUNCATE.
--   Sem este trigger, qualquer usuário com permissão TRUNCATE pode apagar
--   todo o ledger sem ser interceptado pelos controles de imutabilidade.
--
-- O QUE ESTA MIGRATION FAZ:
--   1. Cria a função prevent_ledger_truncate() que lança exceção.
--   2. Cria o trigger prevent_truncate_ledger em BEFORE TRUNCATE FOR EACH STATEMENT.
--
-- Idempotente: usa CREATE OR REPLACE para a função e DROP IF EXISTS para o trigger.
-- ============================================================================

BEGIN;

-- ============================================================================
-- 1. Função que bloqueia TRUNCATE
-- ============================================================================

CREATE OR REPLACE FUNCTION prevent_ledger_truncate()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION
        'Spuri Ledger é imutável. TRUNCATE não permitido. '
        'Use rebuild de projeção se precisar reprocessar eventos.';
    -- RETURN NULL é necessário sintaticamente mas nunca é atingido.
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION prevent_ledger_truncate() IS
    'Bloqueia qualquer tentativa de TRUNCATE em spuri_ledger. '
    'Complementa os triggers prevent_update_ledger e prevent_delete_ledger '
    'que cobrem apenas UPDATE e DELETE por linha.';

-- ============================================================================
-- 2. Trigger BEFORE TRUNCATE — nível de statement (único modo possível para TRUNCATE)
-- ============================================================================

DROP TRIGGER IF EXISTS prevent_truncate_ledger ON spuri_ledger;

CREATE TRIGGER prevent_truncate_ledger
    BEFORE TRUNCATE ON spuri_ledger
    FOR EACH STATEMENT
    EXECUTE FUNCTION prevent_ledger_truncate();

COMMENT ON TRIGGER prevent_truncate_ledger ON spuri_ledger IS
    'Impede TRUNCATE no ledger imutável. '
    'Junto com prevent_update_ledger e prevent_delete_ledger, '
    'garante que nenhuma operação DML pode modificar ou apagar eventos gravados.';

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 029 — Proteção TRUNCATE adicionada ao spuri_ledger';
    RAISE NOTICE '   Trigger: prevent_truncate_ledger (BEFORE TRUNCATE FOR EACH STATEMENT)';
    RAISE NOTICE '   Agora UPDATE, DELETE e TRUNCATE são todos bloqueados no ledger.';
END $$;
-- ===== END 029_fix_ledger_truncate_protection.sql =====

-- ===== BEGIN 030_fix_view_estudantes_com_cursos.sql =====
-- ============================================================================
-- MIGRATION 030 — Recriar view v_estudantes_com_cursos (CORRIGIDA)
-- ============================================================================
--
-- CONTEXTO (ERRO-MIG-01 da auditoria-etapa2-db.md):
--   A migration 004 criou a view v_estudantes_com_cursos referenciando a coluna
--   status_escolar de projection_estudantes.
--   A migration 008 fez DROP VIEW v_estudantes_com_cursos CASCADE (para poder
--   remover a coluna status_escolar) e adicionou status_escolar_fundamental e
--   status_escolar_medio, mas não recriou a view.
--   Desde migration 008, a view não existe — qualquer query ou código que
--   dependa dela falha com "relation v_estudantes_com_cursos does not exist".
--
-- CORREÇÃO DO BUG DE DEPLOY:
--   A versão original desta migration usava CREATE OR REPLACE VIEW omitindo
--   a coluna `genero`. O PostgreSQL não permite CREATE OR REPLACE quando isso
--   altera a posição ou nome de colunas existentes na view:
--
--     Estado da view após migration 009 (última a tocá-la):
--       pos.5 = genero, pos.6 = codigo_academia, ...
--     O que CREATE OR REPLACE tentava impor:
--       pos.5 = codigo_academia, ...  ← genero ausente
--
--   Resultado: "cannot change name of view column genero to codigo_academia"
--
-- CORREÇÃO APLICADA:
--   1. DROP VIEW ... CASCADE (remove também v_estudante_completo que depende)
--   2. DROP VIEW IF EXISTS v_estudante_completo explícito — garante remoção
--      mesmo que v_estudantes_com_cursos não existisse (CASCADE não dispara)
--   3. CREATE VIEW com todas as colunas corretas, incluindo `genero`
--   4. Recriar v_estudante_completo
-- ============================================================================

BEGIN;

-- 1. Dropar views dependentes
--    CASCADE remove v_estudante_completo automaticamente se a view existir
DROP VIEW IF EXISTS v_estudantes_com_cursos CASCADE;

-- 2. Drop explícito de v_estudante_completo — necessário quando
--    v_estudantes_com_cursos não existia e o CASCADE não foi disparado
DROP VIEW IF EXISTS v_estudante_completo;

-- 3. Recriar v_estudantes_com_cursos com todas as colunas corretas
CREATE VIEW v_estudantes_com_cursos AS
SELECT
    e.id,
    e.nome,
    e.codigo_estudante,
    e.email,
    e.genero,                        -- adicionado na migration 007; omitido na versão anterior desta migration
    e.codigo_academia,
    e.status,
    e.status_escolar_fundamental,    -- dividido na migration 008
    e.status_escolar_medio,          -- dividido na migration 008
    e.status_superior,
    e.ano_escolar,
    e.ano_escolar_medio,             -- adicionado na migration 009
    e.ano_superior,
    e.email_verificado,
    -- Curso médio vinculado (nullable)
    cm.id   AS curso_medio_id,
    cm.nome AS curso_medio_nome,
    cm.type AS curso_medio_type,
    -- Curso superior vinculado (nullable)
    cs.id   AS curso_superior_id,
    cs.nome AS curso_superior_nome,
    cs.type AS curso_superior_type,
    e.created_at,
    e.updated_at
FROM projection_estudantes e
LEFT JOIN projection_cursos cm ON e.curso_medio_id    = cm.id AND cm.deleted_at IS NULL
LEFT JOIN projection_cursos cs ON e.curso_superior_id = cs.id AND cs.deleted_at IS NULL;

COMMENT ON VIEW v_estudantes_com_cursos IS
    'View auxiliar de estudantes com cursos médio e superior vinculados. '
    'Recriada na migration 030 (corrigida) com DROP CASCADE em vez de CREATE OR REPLACE. '
    'Inclui: genero (mig 007), status_escolar_fundamental/medio (mig 008), '
    'ano_escolar_medio (mig 009), curso_*_type (mig 030). '
    'Filtra cursos soft-deleted (deleted_at IS NULL, mig 019).';

-- 4. Recriar v_estudante_completo
CREATE VIEW v_estudante_completo AS
SELECT
    e.*,
    (SELECT json_agg(n.*) FROM projection_notas     n WHERE n.codigo_estudante = e.codigo_estudante) AS notas,
    (SELECT json_agg(f.*) FROM projection_faltas     f WHERE f.codigo_estudante = e.codigo_estudante) AS faltas,
    (SELECT json_agg(i.*) FROM projection_inscricoes i WHERE i.estudante_id     = e.id)               AS inscricoes
FROM projection_estudantes e;

COMMENT ON VIEW v_estudante_completo IS
    'View completa do estudante com notas, faltas e inscrições em JSON. '
    'Recriada na migration 030 após DROP CASCADE de v_estudantes_com_cursos.';

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 030 — v_estudantes_com_cursos recriada corretamente';
    RAISE NOTICE '   FIX: DROP CASCADE + CREATE VIEW (não CREATE OR REPLACE)';
    RAISE NOTICE '   FIX: DROP explícito de v_estudante_completo antes de recriar';
    RAISE NOTICE '   FIX: coluna genero incluída na posição correta (pos.5)';
    RAISE NOTICE '   FIX: v_estudante_completo recriada após CASCADE';
    RAISE NOTICE '   Colunas: id, nome, codigo_estudante, email, genero,';
    RAISE NOTICE '            codigo_academia, status, status_escolar_fundamental,';
    RAISE NOTICE '            status_escolar_medio, status_superior, ano_escolar,';
    RAISE NOTICE '            ano_escolar_medio, ano_superior, email_verificado,';
    RAISE NOTICE '            curso_medio_*, curso_superior_*, created_at, updated_at';
END $$;
-- ===== END 030_fix_view_estudantes_com_cursos.sql =====

-- ===== BEGIN 031_fix_sistema_config_colunas.sql =====
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
-- ===== END 031_fix_sistema_config_colunas.sql =====

-- ===== BEGIN 032_add_adicionado_por_categoria_nota.sql =====
-- ============================================================================
-- MIGRATION 032 — Adicionar coluna adicionado_por em projection_categorias_nota
--
-- PROBLEMA CORRIGIDO (P3-09):
--   O handler handleCategoriaAdicionada em categorias_nota_projection.go
--   não lia nem persistia o campo AdicionadoPor (uuid.UUID) do evento
--   CategoriaNotaAdicionadaEvent. A tabela também não tinha a coluna.
--
--   Impacto: impossível auditar quem criou cada categoria de nota
--   sem inspecionar o ledger diretamente — violando o princípio de
--   auditabilidade do sistema.
--
-- O QUE ESTA MIGRATION FAZ:
--   Adiciona coluna adicionado_por à tabela projection_categorias_nota.
--   Garante checkpoint para categorias_nota.
-- ============================================================================

BEGIN;

-- 1. Adicionar coluna adicionado_por
ALTER TABLE projection_categorias_nota
    ADD COLUMN IF NOT EXISTS adicionado_por UUID;

COMMENT ON COLUMN projection_categorias_nota.adicionado_por IS
    'UUID do admin/academia que adicionou a categoria. '
    'Permite auditoria sem inspecionar o ledger diretamente.';

-- 2. Índice para busca por quem adicionou (auditoria)
CREATE INDEX IF NOT EXISTS idx_cat_nota_adicionado_por
    ON projection_categorias_nota(adicionado_por)
    WHERE adicionado_por IS NOT NULL;

-- 3. Garantir checkpoint
INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at)
VALUES ('categorias_nota', 0, CURRENT_TIMESTAMP)
ON CONFLICT (projection_name) DO NOTHING;

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 032 — projection_categorias_nota.adicionado_por adicionada.';
    RAISE NOTICE '   Ação recomendada: POST /admin/rebuild-projection/categorias_nota';
END $$;
-- ===== END 032_add_adicionado_por_categoria_nota.sql =====

-- ===== BEGIN 033_remove_curso_legado_varchar.sql =====
-- ============================================================================
-- MIGRATION 033 — Correções auditoria-etapa2-db (DB-10, DB-15) — CORRIGIDA
-- ============================================================================
-- DB-10: DROP das colunas curso_medio e curso_superior (VARCHAR obsoletas)
--   CORREÇÃO: derrubar views dependentes ANTES do DROP COLUMN.
--   v_estudante_completo usa SELECT e.* → depende de todas as colunas.
--   v_estudantes_com_cursos depende de v_estudante_completo via CASCADE.
--   Padrão idêntico às migrations 008, 009 e 030.
-- ============================================================================

BEGIN;

-- ============================================================================
-- 1. Derrubar views que dependem de projection_estudantes
--    CASCADE remove v_estudante_completo e qualquer view que dependa dela
-- ============================================================================

DROP VIEW IF EXISTS v_estudantes_com_cursos CASCADE;
DROP VIEW IF EXISTS v_estudante_completo    CASCADE;

-- ============================================================================
-- 2. DB-10: remover colunas VARCHAR obsoletas de projection_estudantes
-- ============================================================================

ALTER TABLE projection_estudantes
    DROP COLUMN IF EXISTS curso_medio;

ALTER TABLE projection_estudantes
    DROP COLUMN IF EXISTS curso_superior;

COMMENT ON TABLE projection_estudantes IS
    'Projeção de leitura para estudantes. '
    'Colunas de curso usam UUID (curso_medio_id, curso_superior_id) '
    'com FK para projection_cursos. '
    'Colunas VARCHAR legadas (curso_medio, curso_superior) removidas na migration 033.';

-- ============================================================================
-- 3. Recriar v_estudantes_com_cursos (definição da migration 030)
-- ============================================================================

CREATE VIEW v_estudantes_com_cursos AS
SELECT
    e.id,
    e.nome,
    e.codigo_estudante,
    e.email,
    e.genero,
    e.codigo_academia,
    e.status,
    e.status_escolar_fundamental,
    e.status_escolar_medio,
    e.status_superior,
    e.ano_escolar,
    e.ano_escolar_medio,
    e.ano_superior,
    e.email_verificado,
    cm.id   AS curso_medio_id,
    cm.nome AS curso_medio_nome,
    cm.type AS curso_medio_type,
    cs.id   AS curso_superior_id,
    cs.nome AS curso_superior_nome,
    cs.type AS curso_superior_type,
    e.created_at,
    e.updated_at
FROM projection_estudantes e
LEFT JOIN projection_cursos cm ON e.curso_medio_id    = cm.id AND cm.deleted_at IS NULL
LEFT JOIN projection_cursos cs ON e.curso_superior_id = cs.id AND cs.deleted_at IS NULL;

COMMENT ON VIEW v_estudantes_com_cursos IS
    'View auxiliar de estudantes com cursos médio e superior vinculados. '
    'Recriada na migration 033 após DROP CASCADE para remoção das colunas VARCHAR legadas.';

-- ============================================================================
-- 4. Recriar v_estudante_completo (derrubada pelo CASCADE acima)
-- ============================================================================

CREATE VIEW v_estudante_completo AS
SELECT
    e.*,
    (SELECT json_agg(n.*) FROM projection_notas     n WHERE n.codigo_estudante = e.codigo_estudante) AS notas,
    (SELECT json_agg(f.*) FROM projection_faltas     f WHERE f.codigo_estudante = e.codigo_estudante) AS faltas,
    (SELECT json_agg(i.*) FROM projection_inscricoes i WHERE i.estudante_id     = e.id)               AS inscricoes
FROM projection_estudantes e;

COMMENT ON VIEW v_estudante_completo IS
    'View completa do estudante com notas, faltas e inscrições em JSON. '
    'Recriada na migration 033 após DROP CASCADE de v_estudantes_com_cursos.';

-- ============================================================================
-- 5. DB-15: verificar consistência dos checkpoints da migration 024
-- ============================================================================

INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at, events_processed)
VALUES
    ('aprovacao_ano', 0, CURRENT_TIMESTAMP, 0),
    ('reprovacoes',   0, CURRENT_TIMESTAMP, 0)
ON CONFLICT (projection_name) DO NOTHING;

DO $$
DECLARE
    v_idx_status    TEXT;
    v_idx_academia  TEXT;
    v_idx_estudante TEXT;
BEGIN
    SELECT CASE WHEN EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'idx_projection_inscricoes_status')
           THEN 'EXISTS' ELSE 'DROPPED' END INTO v_idx_status;
    SELECT CASE WHEN EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'idx_projection_inscricoes_academia')
           THEN 'EXISTS' ELSE 'DROPPED' END INTO v_idx_academia;
    SELECT CASE WHEN EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'idx_projection_inscricoes_estudante')
           THEN 'EXISTS' ELSE 'DROPPED' END INTO v_idx_estudante;

    RAISE NOTICE '[DB-15] Estado dos índices da migration 024:';
    RAISE NOTICE '  idx_projection_inscricoes_status:    %', v_idx_status;
    RAISE NOTICE '  idx_projection_inscricoes_academia:  %', v_idx_academia;
    RAISE NOTICE '  idx_projection_inscricoes_estudante: %', v_idx_estudante;

    IF v_idx_status    = 'EXISTS' THEN DROP INDEX IF EXISTS idx_projection_inscricoes_status;    END IF;
    IF v_idx_academia  = 'EXISTS' THEN DROP INDEX IF EXISTS idx_projection_inscricoes_academia;  END IF;
    IF v_idx_estudante = 'EXISTS' THEN DROP INDEX IF EXISTS idx_projection_inscricoes_estudante; END IF;
END $$;

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 033 — Correções auditoria-etapa2-db aplicadas';
    RAISE NOTICE '   DB-10: colunas curso_medio e curso_superior (VARCHAR) removidas de projection_estudantes';
    RAISE NOTICE '   FIX:   views v_estudante_completo e v_estudantes_com_cursos recriadas após CASCADE';
    RAISE NOTICE '   DB-15: checkpoints da migration 024 verificados e consistentes';
    RAISE NOTICE '   ⚠️  Rebuild recomendado: POST /admin/rebuild-projection/estudantes';
END $$;
-- ===== END 033_remove_curso_legado_varchar.sql =====

-- ===== BEGIN 034_garantias_atomicidade.sql =====
-- ============================================================================
-- MIGRATION 034 — FIX DB-13: Garantir NOT NULL em tipo_ensino
-- ============================================================================
--
-- CONTEXTO (DB-13 da auditoria-etapa2-db.md):
--   A migration 008 executou:
--     ALTER TABLE projection_aprovacao_ano
--       ADD COLUMN tipo_ensino VARCHAR(20) NOT NULL DEFAULT 'fundamental' ...
--     ALTER TABLE projection_aprovacao_ano
--       ALTER COLUMN tipo_ensino DROP DEFAULT;
--
--   O DROP DEFAULT não remove o NOT NULL. Porém, em ambientes onde a coluna
--   foi criada com execução parcial (sem BEGIN/COMMIT na migration 006/008),
--   a coluna pode ter ficado nullable sem default.
--
--   Além disso, projection_reprovacoes (criada na migration 009) define
--   tipo_ensino como NOT NULL CHECK (...) mas sem DEFAULT — seguro pois
--   toda inserção deve fornecer o valor explicitamente.
--
-- O QUE ESTA MIGRATION FAZ:
--   1. Garante NOT NULL em projection_aprovacao_ano.tipo_ensino.
--   2. Garante NOT NULL em projection_reprovacoes.tipo_ensino.
--   3. Garante NOT NULL em projection_avaliacao_final.tipo_ensino.
--   4. Preenche NULLs existentes com 'fundamental' antes de impor NOT NULL
--      (defesa contra execução parcial de migrations anteriores).
--   5. Garante que os CHECKs de domínio estão presentes em todas as tabelas.
--
-- Idempotente: usa DO $$ BEGIN...END $$ com IF EXISTS/verificações condicionais.
-- ============================================================================

BEGIN;

-- ============================================================================
-- 1. projection_aprovacao_ano.tipo_ensino
-- ============================================================================

-- Preencher NULLs antes de impor NOT NULL
UPDATE projection_aprovacao_ano
SET tipo_ensino = 'fundamental'
WHERE tipo_ensino IS NULL;

-- Garantir NOT NULL
ALTER TABLE projection_aprovacao_ano
    ALTER COLUMN tipo_ensino SET NOT NULL;

-- Garantir CHECK de domínio (idempotente via DROP IF EXISTS + ADD)
ALTER TABLE projection_aprovacao_ano
    DROP CONSTRAINT IF EXISTS check_aprovacao_tipo_ensino;

ALTER TABLE projection_aprovacao_ano
    ADD CONSTRAINT check_aprovacao_tipo_ensino
        CHECK (tipo_ensino IN ('fundamental', 'medio', 'superior'));

COMMENT ON COLUMN projection_aprovacao_ano.tipo_ensino IS
    'Ciclo de ensino da aprovação: fundamental | medio | superior. NOT NULL garantido pela migration 034.';

-- ============================================================================
-- 2. projection_reprovacoes.tipo_ensino
-- ============================================================================

-- Preencher NULLs antes de impor NOT NULL (defesa contra execução parcial)
UPDATE projection_reprovacoes
SET tipo_ensino = 'fundamental'
WHERE tipo_ensino IS NULL;

-- Garantir NOT NULL
ALTER TABLE projection_reprovacoes
    ALTER COLUMN tipo_ensino SET NOT NULL;

-- Garantir CHECK de domínio
ALTER TABLE projection_reprovacoes
    DROP CONSTRAINT IF EXISTS check_reprov_tipo_ensino;

ALTER TABLE projection_reprovacoes
    ADD CONSTRAINT check_reprov_tipo_ensino
        CHECK (tipo_ensino IN ('fundamental', 'medio', 'superior'));

COMMENT ON COLUMN projection_reprovacoes.tipo_ensino IS
    'Ciclo de ensino da reprovação: fundamental | medio | superior. NOT NULL garantido pela migration 034.';

-- ============================================================================
-- 3. projection_avaliacao_final.tipo_ensino
-- ============================================================================

-- Preencher NULLs antes de impor NOT NULL
UPDATE projection_avaliacao_final
SET tipo_ensino = 'fundamental'
WHERE tipo_ensino IS NULL;

-- Garantir NOT NULL
ALTER TABLE projection_avaliacao_final
    ALTER COLUMN tipo_ensino SET NOT NULL;

-- Garantir CHECK de domínio
ALTER TABLE projection_avaliacao_final
    DROP CONSTRAINT IF EXISTS check_avf_tipo_ensino;

ALTER TABLE projection_avaliacao_final
    ADD CONSTRAINT check_avf_tipo_ensino
        CHECK (tipo_ensino IN ('fundamental', 'medio', 'superior'));

COMMENT ON COLUMN projection_avaliacao_final.tipo_ensino IS
    'Ciclo de ensino da avaliação: fundamental | medio | superior. NOT NULL garantido pela migration 034.';

COMMIT;

-- ============================================================================
-- Verificação
-- ============================================================================

DO $$
DECLARE
    v_aprovacao_nulls  INTEGER;
    v_reprovacoes_nulls INTEGER;
    v_avaliacao_nulls  INTEGER;
BEGIN
    SELECT COUNT(*) INTO v_aprovacao_nulls
    FROM projection_aprovacao_ano WHERE tipo_ensino IS NULL;

    SELECT COUNT(*) INTO v_reprovacoes_nulls
    FROM projection_reprovacoes WHERE tipo_ensino IS NULL;

    SELECT COUNT(*) INTO v_avaliacao_nulls
    FROM projection_avaliacao_final WHERE tipo_ensino IS NULL;

    IF v_aprovacao_nulls > 0 OR v_reprovacoes_nulls > 0 OR v_avaliacao_nulls > 0 THEN
        RAISE WARNING '⚠️  tipo_ensino ainda tem NULLs após migration 034: aprovacao=%, reprovacoes=%, avaliacao=%',
            v_aprovacao_nulls, v_reprovacoes_nulls, v_avaliacao_nulls;
    ELSE
        RAISE NOTICE '✅ MIGRATION 034 — tipo_ensino NOT NULL verificado em todas as tabelas (0 NULLs)';
    END IF;
END $$;
-- ===== END 034_garantias_atomicidade.sql =====

-- ===== BEGIN 035_spuri_generate_codigo_academia.sql =====
-- ============================================================================
-- MIGRATION 035 — Criar função spuri_generate_codigo_academia
-- ============================================================================
--
-- CONTEXTO:
--   O handler RegisterAcademia chama `SELECT spuri_generate_codigo_academia($1)`
--   desde a migration 001, mas a função nunca foi definida em nenhuma migration.
--   Qualquer cadastro de academia caia silenciosamente no fallback hash do Go,
--   gerando códigos no formato "{PROV}XXXXXXXX" (8 dígitos aleatórios) em vez
--   do formato correto.
--
-- FORMATO CORRETO: {PROVINCIA}{ANO}{SEQUENCIAL}
--   Exemplos:
--     LDA20261  →  1ª academia cadastrada em Luanda no ano de 2026
--     LDA20262  →  2ª academia cadastrada em Luanda no ano de 2026
--     BGU20261  →  1ª academia cadastrada em Benguela no ano de 2026
--     HUI20268  →  8ª academia cadastrada em Huila no ano de 2026
--
--   A sequência é reiniciada por província a cada ano-calendário.
--   O número sequencial não tem zero-padding — cresce naturalmente (1, 2, … N).
--
-- GARANTIA DE UNICIDADE:
--   A função conta academias existentes com o prefixo "{PROV}{ANO}" e incrementa.
--   Um loop WHILE protege contra race conditions em cadastros simultâneos na
--   mesma província/ano: se o código já existir, tenta o próximo.
--   O UNIQUE constraint em projection_academias.codigo_academia é a barreira
--   definitiva — a função minimiza colisões, o constraint as elimina.
--
-- Idempotente: CREATE OR REPLACE.
-- ============================================================================

BEGIN;

CREATE OR REPLACE FUNCTION spuri_generate_codigo_academia(p_provincia_code VARCHAR)
RETURNS VARCHAR AS $$
DECLARE
    v_ano    INTEGER;
    v_prefix VARCHAR;
    v_seq    INTEGER;
    v_codigo VARCHAR;
BEGIN
    v_ano    := EXTRACT(YEAR FROM CURRENT_TIMESTAMP)::INTEGER;
    v_prefix := p_provincia_code || v_ano::TEXT;

    -- Ponto de partida: quantas academias já existem com este prefixo?
    -- +1 para obter a próxima posição disponível.
    SELECT COUNT(*) + 1
      INTO v_seq
      FROM projection_academias
     WHERE codigo_academia LIKE v_prefix || '%';

    v_codigo := v_prefix || v_seq::TEXT;

    -- Loop de proteção contra race condition:
    -- se dois cadastros simultâneos chegarem ao mesmo seq, um deles tenta o próximo.
    WHILE EXISTS (
        SELECT 1 FROM projection_academias WHERE codigo_academia = v_codigo
    ) LOOP
        v_seq    := v_seq + 1;
        v_codigo := v_prefix || v_seq::TEXT;
    END LOOP;

    RETURN v_codigo;
END;
$$ LANGUAGE plpgsql VOLATILE;

COMMENT ON FUNCTION spuri_generate_codigo_academia(VARCHAR) IS
    'Gera um código único para academia no formato {PROVINCIA}{ANO}{SEQUENCIAL}. '
    'Exemplo: LDA20261 = 1ª academia de Luanda em 2026. '
    'A sequência reinicia por província a cada ano-calendário. '
    'O loop WHILE protege contra race conditions em cadastros simultâneos.';

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 035 — spuri_generate_codigo_academia criada';
    RAISE NOTICE '   Formato: {PROVINCIA}{ANO}{SEQ} — ex: LDA20261, LDA20262, BGU20261';
    RAISE NOTICE '   Sequência por (província, ano), sem zero-padding no sequencial.';
END $$;
-- ===== END 035_spuri_generate_codigo_academia.sql =====

-- ===== BEGIN 036_turmas_status_alterado_por.sql =====
-- ============================================================================
-- MIGRATION 036 — Auditoria de ativação/desativação de turmas
--
-- PROBLEMA CORRIGIDO:
--   Os eventos TurmaAtivada e TurmaDesativada gravam AlteradoPor no payload
--   do ledger, mas a projection_turmas não tinha colunas para expor essa
--   informação. A pergunta "quem desativou esta turma?" exigia inspecionar
--   o spuri_ledger manualmente.
--
-- O QUE ESTA MIGRATION FAZ:
--   1. Adiciona status_alterado_por UUID (quem ativou/desativou)
--   2. Adiciona status_alterado_em TIMESTAMP (quando ocorreu)
--   3. Cria índice para auditoria por responsável
--
-- AÇÃO REQUERIDA APÓS ESTA MIGRATION:
--   Execute um rebuild da projeção "turmas" via:
--     POST /admin/rebuild-projection/turmas
--   Isso vai reprocessar todos os eventos do ledger e popular
--   corretamente as novas colunas.
-- ============================================================================

BEGIN;

-- 1. Adicionar colunas de auditoria de status
ALTER TABLE projection_turmas
    ADD COLUMN IF NOT EXISTS status_alterado_por UUID DEFAULT NULL;

ALTER TABLE projection_turmas
    ADD COLUMN IF NOT EXISTS status_alterado_em TIMESTAMP DEFAULT NULL;

COMMENT ON COLUMN projection_turmas.status_alterado_por IS
    'UUID da academia que realizou a última ativação ou desativação da turma. '
    'Preenchido via eventos TurmaAtivada e TurmaDesativada — campo AlteradoPor do payload.';

COMMENT ON COLUMN projection_turmas.status_alterado_em IS
    'Timestamp da última ativação ou desativação da turma. '
    'Preenchido via eventos TurmaAtivada e TurmaDesativada — campo OccurredAt do evento.';

-- 2. Índice para auditoria: "quais turmas foram alteradas por esta academia?"
CREATE INDEX IF NOT EXISTS idx_turmas_status_alterado_por
    ON projection_turmas (status_alterado_por)
    WHERE status_alterado_por IS NOT NULL;

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 036 CONCLUÍDA — status_alterado_por e status_alterado_em adicionados a projection_turmas';
    RAISE NOTICE 'Execute POST /admin/rebuild-projection/turmas para popular as novas colunas.';
END $$;
-- ===== END 036_turmas_status_alterado_por.sql =====

-- ===== BEGIN 037_soft_delete_faltas.sql =====
-- Migration 037: soft delete em projection_faltas
-- projection_notas já possui deleted_at (migration anterior).
-- Idempotente via IF NOT EXISTS.

ALTER TABLE projection_faltas
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP DEFAULT NULL;

CREATE INDEX IF NOT EXISTS idx_faltas_estudante_ativo
    ON projection_faltas (codigo_estudante) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_faltas_academia_ativo
    ON projection_faltas (codigo_academia) WHERE deleted_at IS NULL;
-- ===== END 037_soft_delete_faltas.sql =====

-- ===== BEGIN 038_auditoria_delecao_notas_faltas.sql =====
-- ============================================================================
-- MIGRATION 038 — Auditoria de deleção de notas e faltas
--
-- PROBLEMA CORRIGIDO (ambas as tabelas):
--   Os eventos NotaDeletada e FaltaDeletada gravam DeletadoPor e Motivo no
--   payload do ledger, mas as projeções não tinham colunas para expor essa
--   informação. A pergunta "quem deletou e por qual motivo?" exigia inspecionar
--   o spuri_ledger manualmente.
--
--   Adicionalmente, os handlers usavam NOW() em vez de ler DeletedAt do
--   payload — num rebuild, o deleted_at gravado refletiria o momento do
--   rebuild e não o momento real da deleção.
--
-- O QUE ESTA MIGRATION FAZ:
--   1. projection_notas  — adiciona deletado_por UUID e motivo_exclusao TEXT
--   2. projection_faltas — idem
--   3. Índices de auditoria em ambas as tabelas
--
-- AÇÃO REQUERIDA APÓS ESTA MIGRATION:
--   POST /admin/rebuild-projection/notas
--   POST /admin/rebuild-projection/faltas
-- ============================================================================

BEGIN;

-- ── projection_notas ──────────────────────────────────────────────────────

ALTER TABLE projection_notas
    ADD COLUMN IF NOT EXISTS deletado_por    UUID DEFAULT NULL;

ALTER TABLE projection_notas
    ADD COLUMN IF NOT EXISTS motivo_exclusao TEXT DEFAULT NULL;

COMMENT ON COLUMN projection_notas.deletado_por IS
    'UUID da academia que realizou o soft delete. '
    'Preenchido via evento NotaDeletada — campo DeletadoPor do payload. '
    'NULL enquanto a nota não tiver sido deletada.';

COMMENT ON COLUMN projection_notas.motivo_exclusao IS
    'Justificativa obrigatória informada ao deletar a nota. '
    'Preenchido via evento NotaDeletada — campo Motivo do payload. '
    'NULL enquanto a nota não tiver sido deletada.';

CREATE INDEX IF NOT EXISTS idx_notas_deletado_por
    ON projection_notas (deletado_por)
    WHERE deletado_por IS NOT NULL;

-- ── projection_faltas ─────────────────────────────────────────────────────

ALTER TABLE projection_faltas
    ADD COLUMN IF NOT EXISTS deletado_por    UUID DEFAULT NULL;

ALTER TABLE projection_faltas
    ADD COLUMN IF NOT EXISTS motivo_exclusao TEXT DEFAULT NULL;

COMMENT ON COLUMN projection_faltas.deletado_por IS
    'UUID da academia que realizou o soft delete. '
    'Preenchido via evento FaltaDeletada — campo DeletadoPor do payload. '
    'NULL enquanto a falta não tiver sido deletada.';

COMMENT ON COLUMN projection_faltas.motivo_exclusao IS
    'Justificativa obrigatória informada ao deletar a falta. '
    'Preenchido via evento FaltaDeletada — campo Motivo do payload. '
    'NULL enquanto a falta não tiver sido deletada.';

CREATE INDEX IF NOT EXISTS idx_faltas_deletado_por
    ON projection_faltas (deletado_por)
    WHERE deletado_por IS NOT NULL;

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 038 CONCLUÍDA';
    RAISE NOTICE '   → deletado_por e motivo_exclusao adicionados a projection_notas e projection_faltas';
    RAISE NOTICE '   → Execute POST /admin/rebuild-projection/notas';
    RAISE NOTICE '   → Execute POST /admin/rebuild-projection/faltas';
END $$;
-- ===== END 038_auditoria_delecao_notas_faltas.sql =====

-- ===== BEGIN 039_idx_estudante_telefone.sql =====
-- ============================================================================
-- MIGRATION 039 - Índice de telefone em projection_estudantes
-- ============================================================================
-- Necessário para o login universal (sem campo "type"):
-- GetAuthByIdentificador faz lookup por codigo_estudante OR email OR telefone.
-- Sem este índice, a query faria full scan na coluna telefone.
--
-- Os índices de codigo_estudante e email já existem:
--   - codigo_estudante é PRIMARY KEY (índice automático)
--   - idx_estudante_email foi criado na migration 028
--
-- Safe para executar em produção com dados existentes (CREATE INDEX IF NOT EXISTS).
-- ============================================================================

BEGIN;

CREATE INDEX IF NOT EXISTS idx_estudante_telefone
    ON projection_estudantes (telefone)
    WHERE telefone IS NOT NULL;

COMMIT;

-- Verificação
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_indexes
        WHERE tablename = 'projection_estudantes'
          AND indexname  = 'idx_estudante_telefone'
    ) THEN
        RAISE NOTICE '✅ MIGRATION 039 CONCLUÍDA — idx_estudante_telefone criado';
    ELSE
        RAISE NOTICE '❌ MIGRATION 039 FALHOU — índice não encontrado';
    END IF;
END $$;
-- ===== END 039_idx_estudante_telefone.sql =====

-- ===== BEGIN 040_projection_errors.sql =====
-- ============================================================================
-- MIGRATION 040 - Tabela projection_errors
-- Data: 2026
-- ============================================================================
--
-- Contexto:
--   O sistema de projeções tenta registrar falhas permanentes na tabela
--   projection_errors (ver log: "Erro ao registrar falha de projeção academias:
--   pq: relation 'projection_errors' does not exist").
--   Esta migration cria a tabela que estava ausente, eliminando o erro secundário
--   que ocorre após falhas de projeção.
--
--   A coluna occurred_at é exigida pelo INSERT em manager.go:
--     INSERT INTO projection_errors (projection_name, error_message, occurred_at)
--   e já está incluída nesta versão unificada (originalmente ausente e corrigida
--   pela migration 041).
--
-- A tabela é apenas de diagnóstico — não afeta o fluxo principal do event sourcing.
-- Permite auditoria de eventos que falharam permanentemente após todas as tentativas.
-- ============================================================================

BEGIN;

CREATE TABLE IF NOT EXISTS projection_errors (
    id                  BIGSERIAL PRIMARY KEY,
    projection_name     VARCHAR(100)  NOT NULL,
    event_id            BIGINT,
    aggregate_id        UUID,
    aggregate_type      VARCHAR(100),
    event_type          VARCHAR(100),
    error_message       TEXT          NOT NULL,
    attempts            INTEGER       NOT NULL DEFAULT 1,
    occurred_at         TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    first_failed_at     TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_failed_at      TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    resolved            BOOLEAN       NOT NULL DEFAULT FALSE,
    resolved_at         TIMESTAMP,
    resolved_by         VARCHAR(255),
    notes               TEXT
);

-- Índices para consulta operacional
CREATE INDEX IF NOT EXISTS idx_projection_errors_projection
    ON projection_errors (projection_name, resolved);

CREATE INDEX IF NOT EXISTS idx_projection_errors_event
    ON projection_errors (event_id);

CREATE INDEX IF NOT EXISTS idx_projection_errors_unresolved
    ON projection_errors (projection_name, last_failed_at DESC)
    WHERE resolved = FALSE;

COMMENT ON TABLE projection_errors IS
    'Registro de eventos que falharam permanentemente em todas as tentativas de '
    'processamento pelas projeções. Usado para diagnóstico e monitoramento operacional. '
    'Não afeta o fluxo principal do event sourcing.';

COMMENT ON COLUMN projection_errors.event_id        IS 'ID sequencial do evento no spuri_ledger (opcional)';
COMMENT ON COLUMN projection_errors.projection_name IS 'Nome da projeção que falhou (ex: academias, estudantes)';
COMMENT ON COLUMN projection_errors.occurred_at     IS 'Timestamp de quando o erro foi registrado pelo manager.go';
COMMENT ON COLUMN projection_errors.attempts        IS 'Número de tentativas realizadas antes de registrar falha permanente';
COMMENT ON COLUMN projection_errors.resolved        IS 'TRUE quando a falha foi resolvida manualmente (ex: após rebuild)';
COMMENT ON COLUMN projection_errors.resolved_by     IS 'Identificador de quem/o que resolveu a falha (ex: admin UUID, rebuild)';

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 040 - projection_errors criada com sucesso';
    RAISE NOTICE '   Inclui coluna occurred_at exigida pelo manager.go.';
    RAISE NOTICE '   Consulte: SELECT * FROM projection_errors WHERE resolved = FALSE ORDER BY last_failed_at DESC;';
END $$;
-- ===== END 040_projection_errors.sql =====

-- ===== BEGIN 041_ano_letivo_academia.sql =====
BEGIN;

-- 1. Remover tabela obsoleta do ano letivo global
DROP TABLE IF EXISTS projection_sistema_config CASCADE;

-- 2. Remover checkpoint obsoleto
DELETE FROM projection_checkpoints WHERE projection_name = 'sistema_config';

-- 3. Adicionar colunas de ano letivo na tabela de academias
ALTER TABLE projection_academias
    ADD COLUMN IF NOT EXISTS ano_letivo              VARCHAR(20),
    ADD COLUMN IF NOT EXISTS tipo_ano_letivo         VARCHAR(20),
    ADD COLUMN IF NOT EXISTS ano_letivo_ativado_em   TIMESTAMP,
    ADD COLUMN IF NOT EXISTS ano_letivo_ativado_por  UUID
        REFERENCES projection_academias(id) ON DELETE SET NULL;

COMMENT ON COLUMN projection_academias.ano_letivo IS
    'Ano letivo ativo da academia (ex: 2025_2026). NULL = sem ano letivo definido.';
COMMENT ON COLUMN projection_academias.tipo_ano_letivo IS
    'Tipo do ano letivo: escola ou superior.';
COMMENT ON COLUMN projection_academias.ano_letivo_ativado_em IS
    'Data/hora em que o ano letivo foi definido pela última vez.';

COMMIT;
-- ===== END 041_ano_letivo_academia.sql =====

-- ===== BEGIN 042_fix_idempotencia_notas_faltas.sql =====
-- ============================================================================
-- MIGRATION 042 — Corrigir idempotência do registro de nota e falta
--
-- PROBLEMA CORRIGIDO:
--   1. NOTA: O handler handleNotasRegistradas usava ON CONFLICT com colunas
--      (codigo_estudante, codigo_academia, ano_lectivo, periodo, materia_disciplinar_id)
--      que NÃO correspondem à constraint uq_nota_unica definida em migration 006.
--      A constraint real é:
--        UNIQUE (codigo_estudante, ano_lectivo, periodo, materia_disciplinar_id, tipo, categoria)
--      Com as colunas erradas, o ON CONFLICT nunca disparava — causando violação
--      23505 (unique_violation) em rebuild completo ou double-submit.
--      CORREÇÃO: o handler Go foi corrigido para usar as colunas corretas.
--      Esta migration garante que a constraint existe com o nome e colunas exatos.
--
--   2. FALTA: A constraint UNIQUE da tabela projection_faltas é:
--        UNIQUE(codigo_estudante, codigo_academia, data, materia_disciplinar_id)
--      e o handler usa ON CONFLICT (event_id) DO NOTHING — correto para replay
--      de mesmo evento, mas sem guard de negócio no aggregate.
--      O guard foi adicionado no aggregate Go (FaltasRegistradasPorChave).
--      Esta migration não precisa alterar a constraint de faltas.
--
-- AÇÃO REQUERIDA APÓS ESTA MIGRATION:
--   POST /admin/rebuild-projection/notas
--   (para reprocessar eventos com o UPSERT corrigido)
-- ============================================================================

BEGIN;

-- ── Garantir que uq_nota_unica existe com as colunas corretas ────────────────
-- A migration 006 criou esta constraint. Se por algum motivo foi removida
-- ou criada com colunas diferentes, recriamos aqui com IF NOT EXISTS safe.

-- Verificar e recriar apenas se necessário.
-- Usamos DO $$ para condicional segura.
DO $$
BEGIN
    -- Verificar se a constraint existe com o nome correto
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint c
        JOIN pg_class t ON t.oid = c.conrelid
        WHERE t.relname = 'projection_notas'
          AND c.conname = 'uq_nota_unica'
          AND c.contype = 'u'
    ) THEN
        -- Remover qualquer constraint UNIQUE antiga sobre as mesmas colunas
        -- (pode ter sido criada com nome diferente em ambientes antigos)
        ALTER TABLE projection_notas
            DROP CONSTRAINT IF EXISTS projection_notas_codigo_estudante_codigo_academia_ano_lectivo_key;

        -- Criar a constraint correta
        ALTER TABLE projection_notas
            ADD CONSTRAINT uq_nota_unica
                UNIQUE (codigo_estudante, ano_lectivo, periodo, materia_disciplinar_id, tipo, categoria);

        RAISE NOTICE '✅ Constraint uq_nota_unica criada em projection_notas';
    ELSE
        RAISE NOTICE 'ℹ️  Constraint uq_nota_unica já existe em projection_notas — nenhuma ação necessária';
    END IF;
END $$;

-- ── Garantir índices de suporte ao ON CONFLICT corrigido ─────────────────────
CREATE INDEX IF NOT EXISTS idx_notas_unica_lookup
    ON projection_notas (codigo_estudante, ano_lectivo, periodo, materia_disciplinar_id, tipo, categoria)
    WHERE deleted_at IS NULL;

-- ── Checkpoint ───────────────────────────────────────────────────────────────
INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at, events_processed)
VALUES ('migration_042', 0, CURRENT_TIMESTAMP, 0)
ON CONFLICT (projection_name) DO NOTHING;

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 042 CONCLUÍDA';
    RAISE NOTICE '   → constraint uq_nota_unica verificada/criada em projection_notas';
    RAISE NOTICE '   → ON CONFLICT do handler handleNotasRegistradas agora usa colunas corretas';
    RAISE NOTICE '   → Guards de duplicata adicionados nos aggregates (NotasRegistradasPorChave, FaltasRegistradasPorChave)';
    RAISE NOTICE '   → Execute: POST /admin/rebuild-projection/notas';
END $$;
-- ===== END 042_fix_idempotencia_notas_faltas.sql =====

-- ===== BEGIN 043_add_data_nascimento_estudante.sql =====
-- ============================================================================
-- MIGRATION 043 — Adicionar campo data_nascimento em projection_estudantes
--
-- REGRA DE NEGÓCIO:
--   • data_nascimento é opcional no cadastro.
--   • O valor nunca pode ser >= CURRENT_DATE (a pessoa deve ter nascido no passado).
--   • A constraint no banco é a barreira definitiva contra datas inválidas.
--
-- AÇÃO REQUERIDA APÓS ESTA MIGRATION:
--   POST /admin/rebuild-projection/estudantes
-- ============================================================================

BEGIN;

-- 1. Adicionar coluna (nullable — estudantes existentes não têm data_nascimento)
ALTER TABLE projection_estudantes
    ADD COLUMN IF NOT EXISTS data_nascimento DATE DEFAULT NULL;

-- 2. Constraint: data_nascimento deve ser menor que a data atual (passado)
ALTER TABLE projection_estudantes
    DROP CONSTRAINT IF EXISTS chk_estudante_data_nascimento;

ALTER TABLE projection_estudantes
    ADD CONSTRAINT chk_estudante_data_nascimento CHECK (
        data_nascimento IS NULL
        OR data_nascimento < CURRENT_DATE
    );

-- 3. Índice para buscas por data de nascimento
CREATE INDEX IF NOT EXISTS idx_estudante_data_nascimento
    ON projection_estudantes (data_nascimento)
    WHERE data_nascimento IS NOT NULL;

-- 4. Comentário
COMMENT ON COLUMN projection_estudantes.data_nascimento IS
    'Data de nascimento do estudante. Deve ser anterior à data atual (passado). '
    'Opcional — NULL para estudantes cadastrados antes desta migration.';

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 043 — data_nascimento adicionada em projection_estudantes';
    RAISE NOTICE '   Constraint: data_nascimento < CURRENT_DATE (passado obrigatório)';
    RAISE NOTICE '   Execute POST /admin/rebuild-projection/estudantes se necessário.';
END $$;
-- ===== END 043_add_data_nascimento_estudante.sql =====

-- ===== BEGIN 044_telefone_extra.sql =====
-- ============================================================================
-- MIGRATION 044 — Tabela numero_telefone_extra
--
-- REGRAS DE NEGÓCIO:
--   • Qualquer usuário (estudante, academia ou admin) pode cadastrar um
--     número de telefone extra.
--   • O mesmo número pode ser cadastrado por múltiplos usuários enquanto
--     nenhum deles o verificou.
--   • Quando um usuário verifica o número, os demais NÃO podem verificá-lo.
--   • Se um número já está verificado por alguém, ninguém mais pode
--     cadastrá-lo (o cadastro é bloqueado na camada de aplicação e pelo
--     índice parcial de unicidade abaixo).
--   • Um usuário pode ter múltiplos telefones extras, mas não pode cadastrar
--     o mesmo número duas vezes.
--
-- ESTRATÉGIA DE UNICIDADE:
--   1. UNIQUE parcial em (numero_telefone) WHERE verificado = TRUE:
--      garante que no máximo um registro verificado exista para cada número.
--   2. UNIQUE em (id_user, tipo_user, numero_telefone):
--      impede que o mesmo usuário cadastre o mesmo número duas vezes.
--   3. A restrição "número verificado não pode ser cadastrado de novo"
--      é enforçada na camada Go (aggregate) consultando a projeção antes
--      de emitir o evento.
-- ============================================================================

BEGIN;

CREATE TABLE IF NOT EXISTS projection_telefones_extra (
    id              UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    id_user         UUID        NOT NULL,
    tipo_user       VARCHAR(20) NOT NULL
                        CHECK (tipo_user IN ('estudante', 'academia', 'admin')),
    numero_telefone VARCHAR(30) NOT NULL,
    verificado      BOOLEAN     NOT NULL DEFAULT FALSE,
    registered_at   TIMESTAMP   NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP   NOT NULL DEFAULT CURRENT_TIMESTAMP,
    event_id        UUID        NOT NULL,
    version         INTEGER     NOT NULL DEFAULT 1,

    -- Um usuário não pode cadastrar o mesmo número duas vezes
    UNIQUE (id_user, tipo_user, numero_telefone)
);

-- Garante que no máximo 1 registro verificado exista por número de telefone.
-- Índice parcial: afeta apenas linhas onde verificado = TRUE.
-- Isso permite múltiplos cadastros não verificados do mesmo número,
-- mas bloqueia uma segunda verificação.
CREATE UNIQUE INDEX IF NOT EXISTS idx_telefone_extra_verificado_unico
    ON projection_telefones_extra (numero_telefone)
    WHERE verificado = TRUE;

-- Índices operacionais
CREATE INDEX IF NOT EXISTS idx_telefone_extra_user
    ON projection_telefones_extra (id_user, tipo_user);

CREATE INDEX IF NOT EXISTS idx_telefone_extra_numero
    ON projection_telefones_extra (numero_telefone);

CREATE INDEX IF NOT EXISTS idx_telefone_extra_verificado
    ON projection_telefones_extra (verificado);

-- Trigger para atualizar updated_at automaticamente
DROP TRIGGER IF EXISTS trigger_update_telefone_extra_timestamp ON projection_telefones_extra;
CREATE TRIGGER trigger_update_telefone_extra_timestamp
    BEFORE UPDATE ON projection_telefones_extra
    FOR EACH ROW EXECUTE FUNCTION update_projection_timestamp();

-- Checkpoint para a projeção
INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at)
VALUES ('telefones_extra', 0, CURRENT_TIMESTAMP)
ON CONFLICT (projection_name) DO NOTHING;

COMMENT ON TABLE projection_telefones_extra IS
    'Telefones extras de qualquer tipo de usuário (estudante, academia, admin). '
    'Um número só pode estar verificado por um único usuário. '
    'Múltiplos usuários podem cadastrar o mesmo número enquanto não verificado. '
    'Se verificado, nenhum outro usuário pode cadastrá-lo.';

COMMENT ON COLUMN projection_telefones_extra.id_user IS
    'UUID do usuário dono deste telefone (estudante, academia ou admin).';

COMMENT ON COLUMN projection_telefones_extra.tipo_user IS
    'Tipo do usuário: estudante | academia | admin.';

COMMENT ON COLUMN projection_telefones_extra.verificado IS
    'TRUE quando o usuário confirmou a posse do número. '
    'Apenas um usuário pode ter verificado = TRUE para o mesmo numero_telefone.';

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 044 — projection_telefones_extra criada';
    RAISE NOTICE '   Índice de unicidade parcial: um número só pode ser verificado por um usuário.';
END $$;
-- ===== END 044_telefone_extra.sql =====

-- ===== BEGIN 045_fix_codigo_academia_generator.sql =====
-- ============================================================================
-- MIGRATION 045 — Corrigir spuri_generate_codigo_academia
--
-- PROBLEMA CORRIGIDO:
--   A função original consultava projection_academias (projeção assíncrona)
--   para determinar o próximo sequencial. Em cadastros rápidos em sequência,
--   a projeção ainda não materializou o evento anterior quando o próximo
--   chegou — resultado: duas academias recebiam o mesmo código (ex: BGO20261),
--   causando violação de unique constraint na projeção e travamento permanente
--   do pipeline de eventos.
--
-- SOLUÇÃO:
--   Consultar spuri_ledger (fonte de verdade imutável, síncrona com o INSERT)
--   em vez de projection_academias. O payload do evento AcademiaCriada contém
--   CodigoAcademia como campo JSON — extraímos via ->> para comparar o prefixo.
--
-- FORMATO DO CÓDIGO: {PROVINCIA}{ANO}{SEQUENCIAL}
--   Ex: LDA20261, LDA20262, BGO20261
--   O sequencial reinicia por (província, ano). Sem zero-padding.
--
-- Idempotente: CREATE OR REPLACE.
-- ============================================================================

BEGIN;

CREATE OR REPLACE FUNCTION spuri_generate_codigo_academia(p_provincia_code VARCHAR)
RETURNS VARCHAR AS $$
DECLARE
    v_ano    INTEGER;
    v_prefix VARCHAR;
    v_seq    INTEGER;
    v_codigo VARCHAR;
BEGIN
    v_ano    := EXTRACT(YEAR FROM CURRENT_TIMESTAMP)::INTEGER;
    v_prefix := p_provincia_code || v_ano::TEXT;

    -- Contar academias já gravadas no LEDGER com este prefixo.
    -- O ledger é síncrono: o INSERT do evento já ocorreu antes desta função
    -- ser chamada na mesma transação (ou imediatamente antes).
    -- Usar payload->>'CodigoAcademia' para extrair o código do JSON.
    SELECT COUNT(*) + 1
      INTO v_seq
      FROM spuri_ledger
     WHERE event_type = 'AcademiaCriada'
       AND payload->>'CodigoAcademia' LIKE v_prefix || '%';

    v_codigo := v_prefix || v_seq::TEXT;

    -- Loop de proteção: se por alguma race condition o código já existir
    -- no ledger, incrementar até encontrar um livre.
    WHILE EXISTS (
        SELECT 1
        FROM spuri_ledger
        WHERE event_type = 'AcademiaCriada'
          AND payload->>'CodigoAcademia' = v_codigo
    ) LOOP
        v_seq    := v_seq + 1;
        v_codigo := v_prefix || v_seq::TEXT;
    END LOOP;

    RETURN v_codigo;
END;
$$ LANGUAGE plpgsql VOLATILE;

COMMENT ON FUNCTION spuri_generate_codigo_academia(VARCHAR) IS
    'Gera um código único para academia no formato {PROVINCIA}{ANO}{SEQUENCIAL}. '
    'Exemplo: LDA20261 = 1ª academia de Luanda em 2026. '
    'Consulta o spuri_ledger (não a projeção) para garantir unicidade mesmo '
    'em cadastros simultâneos antes da projeção ser materializada. '
    'Corrigido na migration 045: substitui consulta a projection_academias '
    'por consulta ao ledger (fonte de verdade imutável).';

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 045 — spuri_generate_codigo_academia corrigida';
    RAISE NOTICE '   Agora consulta spuri_ledger em vez de projection_academias.';
    RAISE NOTICE '   Elimina race condition em cadastros rápidos em sequência.';
END $$;
-- ===== END 045_fix_codigo_academia_generator.sql =====

-- ===== BEGIN 046_remove_aprovacao_reprovacao.sql =====
-- ============================================================================
-- MIGRATION 046 — Remover tabelas obsoletas: aprovacao_ano e reprovacoes
-- ============================================================================
--
-- CONTEXTO:
--   O sistema consolidou a avaliação de ano em um único evento:
--   AvaliacaoFinalAnoAcademico → projection_avaliacao_final.
--
--   As tabelas projection_aprovacao_ano e projection_reprovacoes eram
--   populadas pelo evento AprovacaoAnoRegistrada, que foi removido do sistema.
--   A rota POST /academia/aprovacao-ano foi eliminada.
--
--   Com o banco sendo resetado, estas tabelas são simplesmente removidas.
--   A projeção avaliacao_final cobre todos os casos:
--     • aprovado=TRUE  → aprovações
--     • aprovado=FALSE → reprovações
--   Filtros via GET /aprovacoes e GET /reprovacoes continuam funcionando
--   via projection_avaliacao_final.
--
-- AÇÃO REQUERIDA APÓS ESTA MIGRATION:
--   Nenhuma — banco já foi resetado antes desta migration.
-- ============================================================================

BEGIN;

-- 1. Remover views que dependem das tabelas obsoletas (se existirem)
DROP VIEW IF EXISTS v_aprovacoes_completas CASCADE;
DROP VIEW IF EXISTS v_reprovacoes_completas CASCADE;

-- 2. Remover tabelas obsoletas
DROP TABLE IF EXISTS projection_aprovacao_ano CASCADE;
DROP TABLE IF EXISTS projection_reprovacoes CASCADE;

-- 3. Remover checkpoints obsoletos
DELETE FROM projection_checkpoints
WHERE projection_name IN ('aprovacao_ano', 'reprovacoes');

-- 4. Garantir checkpoint para avaliacao_final
INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at)
VALUES ('avaliacao_final', 0, CURRENT_TIMESTAMP)
ON CONFLICT (projection_name) DO NOTHING;

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 046 — projection_aprovacao_ano e projection_reprovacoes removidas';
    RAISE NOTICE '   Avaliações de ano agora gerenciadas exclusivamente por projection_avaliacao_final';
    RAISE NOTICE '   GET /aprovacoes → aprovado=TRUE em projection_avaliacao_final';
    RAISE NOTICE '   GET /reprovacoes → aprovado=FALSE em projection_avaliacao_final';
END $$;
-- ===== END 046_remove_aprovacao_reprovacao.sql =====

-- ===== BEGIN 050_create_async_jobs.sql =====
BEGIN;

CREATE TABLE IF NOT EXISTS async_jobs (
    id           UUID         PRIMARY KEY,
    type         VARCHAR(64)  NOT NULL,
    status       VARCHAR(20)  NOT NULL DEFAULT 'pending'
                              CHECK (status IN ('pending','processing','done','failed')),
    user_id      UUID         NOT NULL,
    user_type    VARCHAR(20)  NOT NULL,
    payload      JSONB        NOT NULL,
    results      JSONB        NOT NULL DEFAULT '[]',
    total_items  INT          NOT NULL DEFAULT 0,
    done_items   INT          NOT NULL DEFAULT 0,
    fail_items   INT          NOT NULL DEFAULT 0,
    error        TEXT,
    created_at   TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at   TIMESTAMP,
    completed_at TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_async_jobs_user_id    ON async_jobs (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_async_jobs_status     ON async_jobs (status) WHERE status IN ('pending','processing');
CREATE INDEX IF NOT EXISTS idx_async_jobs_cleanup    ON async_jobs (completed_at) WHERE status IN ('done','failed');

COMMENT ON TABLE async_jobs IS
    'Jobs assíncronos de operações em lote. Resultados parciais são gravados durante o processamento.';

COMMIT;
-- ===== END 050_create_async_jobs.sql =====

-- ===== BEGIN 051_turmas_historico_ano_letivo.sql =====
-- MIGRATION 051 — Histórico de estudantes por ano letivo em turmas
--
-- Objetivos:
-- 1) Permitir que cada turma mantenha um histórico por ano letivo dos
--    estudantes que já fizeram parte dela.
-- 2) Suportar remoção atômica na projeção de turmas a partir do evento
--    AvaliacaoFinalAnoAcademico.

ALTER TABLE projection_turmas
    ADD COLUMN IF NOT EXISTS historico_estudantes_ano_letivo JSONB NOT NULL DEFAULT '{}'::jsonb;

COMMENT ON COLUMN projection_turmas.historico_estudantes_ano_letivo IS
    'Mapa JSONB: ano_letivo -> lista de codigo_estudante que já fizeram parte da turma no ano.';

DO $$
BEGIN
    RAISE NOTICE '✅ MIGRATION 051 — histórico por ano letivo adicionado em projection_turmas';
END $$;
-- ===== END 051_turmas_historico_ano_letivo.sql =====

-- ===== BEGIN 052_notas_sem_teto_faltas_sem_unicidade.sql =====
-- MIGRATION 052 — Notas sem teto máximo e faltas sem unicidade por data+matéria
--
-- Objetivo:
--   1) Permitir notas com qualquer valor >= 0 (remover teto <= 20).
--   2) Permitir múltiplos registros de falta na mesma combinação
--      (estudante, academia, data, matéria).
--
-- Impacto:
--   - projection_notas.nota continua obrigatória e não-negativa.
--   - projection_faltas deixa de impor UNIQUE por data+matéria.

BEGIN;

-- 1) Notas: remover check antigo e manter apenas nota >= 0
ALTER TABLE projection_notas
    DROP CONSTRAINT IF EXISTS projection_notas_nota_check;

ALTER TABLE projection_notas
    ADD CONSTRAINT projection_notas_nota_check
        CHECK (nota >= 0);

COMMENT ON COLUMN projection_notas.nota IS
    'Nota sem teto máximo; valor deve ser maior ou igual a 0.';

-- 2) Faltas: remover UNIQUE(codigo_estudante, codigo_academia, data, materia_disciplinar_id)
DO $$
DECLARE
    r RECORD;
BEGIN
    FOR r IN
        SELECT c.conname
        FROM pg_constraint c
        JOIN pg_class t ON t.oid = c.conrelid
        WHERE t.relname = 'projection_faltas'
          AND c.contype = 'u'
          AND (
              SELECT array_agg(a.attname ORDER BY u.ord)
              FROM unnest(c.conkey) WITH ORDINALITY AS u(attnum, ord)
              JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = u.attnum
          ) = ARRAY['codigo_estudante','codigo_academia','data','materia_disciplinar_id']::name[]
    LOOP
        EXECUTE format('ALTER TABLE projection_faltas DROP CONSTRAINT %I', r.conname);
        RAISE NOTICE '✅ Constraint removida de projection_faltas: %', r.conname;
    END LOOP;
END
$$;

COMMIT;
-- ===== END 052_notas_sem_teto_faltas_sem_unicidade.sql =====

-- ===== BEGIN 053_fix_constraint_legada_projection_notas.sql =====
-- ==========================================================================
-- MIGRATION 053 — Remover constraint UNIQUE legada de projection_notas
--
-- Problema observado em produção:
--   duplicate key value violates unique constraint
--   "projection_notas_codigo_estudante_codigo_academia_ano_lecti_key"
--
-- Causa:
--   Alguns ambientes antigos podem manter uma UNIQUE legada baseada em:
--   (codigo_estudante, codigo_academia, ano_lectivo, periodo, materia_disciplinar_id)
--   Essa restrição conflita com o modelo atual (uq_nota_unica) que inclui
--   (tipo, categoria) e não inclui codigo_academia.
--
-- Correção:
--   1) Remove qualquer UNIQUE legada com o conjunto exato de colunas antigas,
--      independentemente do nome real da constraint (incluindo nomes truncados).
--   2) Garante a existência da uq_nota_unica com o conjunto de colunas atual.
-- ==========================================================================

BEGIN;

DO $$
DECLARE
    r RECORD;
BEGIN
    -- Remover todas as UNIQUE legadas por assinatura de colunas (nome-agnóstico)
    FOR r IN
        SELECT c.conname
        FROM pg_constraint c
        JOIN pg_class t ON t.oid = c.conrelid
        WHERE t.relname = 'projection_notas'
          AND c.contype = 'u'
          AND (
              SELECT array_agg(a.attname ORDER BY u.ord)
              FROM unnest(c.conkey) WITH ORDINALITY AS u(attnum, ord)
              JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = u.attnum
          ) = ARRAY[
              'codigo_estudante',
              'codigo_academia',
              'ano_lectivo',
              'periodo',
              'materia_disciplinar_id'
          ]::name[]
    LOOP
        EXECUTE format('ALTER TABLE projection_notas DROP CONSTRAINT %I', r.conname);
        RAISE NOTICE '✅ Constraint UNIQUE legada removida de projection_notas: %', r.conname;
    END LOOP;
END
$$;

-- Garantir constraint atual de idempotência
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint c
        JOIN pg_class t ON t.oid = c.conrelid
        WHERE t.relname = 'projection_notas'
          AND c.conname = 'uq_nota_unica'
          AND c.contype = 'u'
    ) THEN
        ALTER TABLE projection_notas
            ADD CONSTRAINT uq_nota_unica
                UNIQUE (codigo_estudante, ano_lectivo, periodo, materia_disciplinar_id, tipo, categoria);

        RAISE NOTICE '✅ Constraint uq_nota_unica criada em projection_notas';
    ELSE
        RAISE NOTICE 'ℹ️ Constraint uq_nota_unica já existe em projection_notas';
    END IF;
END
$$;

-- Checkpoint técnico da migration
INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at, events_processed)
VALUES ('migration_053', 0, CURRENT_TIMESTAMP, 0)
ON CONFLICT (projection_name) DO NOTHING;

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 053 CONCLUÍDA';
    RAISE NOTICE '   → UNIQUE legada de projection_notas removida (se existia)';
    RAISE NOTICE '   → uq_nota_unica garantida';
    RAISE NOTICE '   → Recomendado: POST /admin/rebuild-projection/notas';
END $$;
-- ===== END 053_fix_constraint_legada_projection_notas.sql =====

-- ===== BEGIN 053_restaurar_unicidade_faltas.sql =====
-- MIGRATION 053 — Restaurar unicidade de faltas por ano+data+matéria
--
-- Contexto:
--   A migration 052 removeu a UNIQUE de projection_faltas.
--   Esta migration restaura a regra de unicidade para faltas,
--   mantendo a quantidade sem teto máximo (apenas > 0).

BEGIN;

-- 1) Consolidar duplicatas ativas (deleted_at IS NULL) para permitir recriar UNIQUE.
--    Estratégia: manter o registro mais antigo (registered_at, depois id) e somar quantidades dos duplicados.
WITH duplicados AS (
    SELECT
        codigo_estudante,
        codigo_academia,
        data,
        materia_disciplinar_id,
        (ARRAY_AGG(id ORDER BY registered_at ASC, id ASC))[1] AS keep_id,
        SUM(quantidade) AS total_qtd,
        ARRAY_AGG(id) AS ids
    FROM projection_faltas
    WHERE deleted_at IS NULL
    GROUP BY codigo_estudante, codigo_academia, data, materia_disciplinar_id
    HAVING COUNT(*) > 1
), atualiza_keep AS (
    UPDATE projection_faltas f
    SET quantidade = d.total_qtd
    FROM duplicados d
    WHERE f.id = d.keep_id
    RETURNING f.id
)
DELETE FROM projection_faltas f
USING duplicados d
WHERE f.id = ANY(d.ids)
  AND f.id <> d.keep_id;

-- 2) Remover constraint UNIQUE antiga (se existir com nome variável)
DO $$
DECLARE
    r RECORD;
BEGIN
    FOR r IN
        SELECT c.conname
        FROM pg_constraint c
        JOIN pg_class t ON t.oid = c.conrelid
        WHERE t.relname = 'projection_faltas'
          AND c.contype = 'u'
          AND (
              SELECT array_agg(a.attname ORDER BY u.ord)
              FROM unnest(c.conkey) WITH ORDINALITY AS u(attnum, ord)
              JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = u.attnum
          ) = ARRAY['codigo_estudante','codigo_academia','data','materia_disciplinar_id']::name[]
    LOOP
        EXECUTE format('ALTER TABLE projection_faltas DROP CONSTRAINT %I', r.conname);
        RAISE NOTICE 'ℹ️ Constraint antiga removida: %', r.conname;
    END LOOP;
END
$$;

-- 3) Recriar UNIQUE canônica
ALTER TABLE projection_faltas
    ADD CONSTRAINT uq_falta_unica
        UNIQUE (codigo_estudante, codigo_academia, data, materia_disciplinar_id);

COMMIT;
-- ===== END 053_restaurar_unicidade_faltas.sql =====

-- ===== BEGIN 054_async_job_sse_hidden.sql =====
CREATE TABLE IF NOT EXISTS async_job_sse_hidden (
    user_id    UUID        NOT NULL,
    job_id     UUID        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, job_id),
    CONSTRAINT fk_async_job_sse_hidden_job
        FOREIGN KEY (job_id) REFERENCES async_jobs(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_async_job_sse_hidden_user_id
    ON async_job_sse_hidden (user_id);
-- ===== END 054_async_job_sse_hidden.sql =====

-- ===== BEGIN 054_rename_academia_type_to_nivel.sql =====
-- ==========================================================================
-- MIGRATION 054 - Renomear campo type de academias para nivel
-- ==========================================================================

BEGIN;

ALTER TABLE projection_academias
    RENAME COLUMN type TO nivel;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM pg_indexes
        WHERE indexname = 'idx_proj_academia_type'
    ) THEN
        ALTER INDEX idx_proj_academia_type RENAME TO idx_proj_academia_nivel_tipo;
    END IF;
END $$;

ALTER TABLE projection_academias
    DROP CONSTRAINT IF EXISTS check_nivel_escolar_tipo;

ALTER TABLE projection_academias
    ADD CONSTRAINT check_nivel_escolar_tipo CHECK (
        (nivel = 'escola' AND nivel_escolar IN ('fundamental', 'medio', 'misto'))
        OR
        (nivel = 'superior' AND nivel_escolar IS NULL)
    );

COMMENT ON COLUMN projection_academias.nivel IS
    'Nível da academia: escola | superior.';

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 054 - campo projection_academias.type renomeado para nivel';
END $$;
-- ===== END 054_rename_academia_type_to_nivel.sql =====

-- ===== BEGIN 055_add_academia_type_public_private.sql =====
-- MIGRATION 055 - Adicionar campo obrigatório "type" em academias (public/private)
-- -----------------------------------------------------------------------------
-- Premissa desta migration:
--   - banco vazio (sem necessidade de compatibilidade com dados legados)
-- Objetivo:
--   1. Adicionar a coluna obrigatória `type` em projection_academias
--   2. Garantir domínio restrito: 'public' | 'private'

ALTER TABLE projection_academias
    ADD COLUMN type VARCHAR(20) NOT NULL;

ALTER TABLE projection_academias
    ADD CONSTRAINT check_academia_type_public_private CHECK (type IN ('public', 'private'));

CREATE INDEX IF NOT EXISTS idx_proj_academia_type_public_private
    ON projection_academias(type);

COMMENT ON COLUMN projection_academias.type IS
    'Natureza da academia: public ou private';

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 055 - campo obrigatório projection_academias.type adicionado (public/private)';
END $$;
-- ===== END 055_add_academia_type_public_private.sql =====

