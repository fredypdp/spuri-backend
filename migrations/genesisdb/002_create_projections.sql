-- ============================================
-- PROJEÇÕES (Read Models)
-- Tabelas otimizadas para leitura
-- ============================================

-- ============================================
-- PROJEÇÃO: ESTUDANTES
-- ============================================

CREATE TABLE projection_estudantes (
    id UUID PRIMARY KEY,
    nome VARCHAR(255) NOT NULL,
    senha_hash VARCHAR(255) NOT NULL, -- 🔥 ADICIONAR
    bilhete_identidade VARCHAR(50),
    bilhete_identidade_responsavel VARCHAR(50),
    id_academia UUID,
    ano_escolar VARCHAR(50),
    ano_superior VARCHAR(50),
    curso_medio VARCHAR(255),
    curso_superior VARCHAR(255),
    status_escolar VARCHAR(20),
    status_superior VARCHAR(20),
    
    -- Metadados de projeção
    version INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_event_id UUID,
    
    -- Para reconstrução rápida
    total_notas INTEGER DEFAULT 0,
    total_faltas INTEGER DEFAULT 0,
    total_inscricoes INTEGER DEFAULT 0
);

CREATE INDEX idx_proj_estudante_academia ON projection_estudantes(id_academia);
CREATE INDEX idx_proj_estudante_bilhete ON projection_estudantes(bilhete_identidade);
CREATE INDEX idx_proj_estudante_bilhete_resp ON projection_estudantes(bilhete_identidade_responsavel);

-- ============================================
-- PROJEÇÃO: ACADEMIAS
-- ============================================

CREATE TABLE projection_academias (
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
    status VARCHAR(20) DEFAULT 'ativo',
    cursos JSONB DEFAULT '[]',
    
    -- Metadados de projeção
    version INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_event_id UUID,
    
    -- Estatísticas
    total_estudantes INTEGER DEFAULT 0,
    total_inscricoes_pendentes INTEGER DEFAULT 0
);

CREATE INDEX idx_proj_academia_provincia ON projection_academias(provincia);
CREATE INDEX idx_proj_academia_codigo ON projection_academias(codigo_academia);
CREATE INDEX idx_proj_academia_type ON projection_academias(type);

-- ============================================
-- PROJEÇÃO: NOTAS
-- ============================================

CREATE TABLE projection_notas (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    estudante_id UUID NOT NULL,
    id_academia UUID NOT NULL,
    ano_lectivo VARCHAR(20) NOT NULL,
    periodo VARCHAR(50) NOT NULL,
    materias JSONB NOT NULL,
    
    -- Metadados
    registered_at TIMESTAMP NOT NULL,
    event_id UUID NOT NULL,
    version INTEGER NOT NULL,
    
    FOREIGN KEY (estudante_id) REFERENCES projection_estudantes(id) ON DELETE CASCADE
);

CREATE INDEX idx_proj_notas_estudante ON projection_notas(estudante_id);
CREATE INDEX idx_proj_notas_academia ON projection_notas(id_academia);
CREATE INDEX idx_proj_notas_ano ON projection_notas(ano_lectivo);

-- ============================================
-- PROJEÇÃO: FALTAS
-- ============================================

CREATE TABLE projection_faltas (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    estudante_id UUID NOT NULL,
    id_academia UUID NOT NULL,
    ano_lectivo VARCHAR(20) NOT NULL,
    periodo VARCHAR(50) NOT NULL,
    materias JSONB NOT NULL,
    
    -- Metadados
    registered_at TIMESTAMP NOT NULL,
    event_id UUID NOT NULL,
    version INTEGER NOT NULL,
    
    FOREIGN KEY (estudante_id) REFERENCES projection_estudantes(id) ON DELETE CASCADE
);

CREATE INDEX idx_proj_faltas_estudante ON projection_faltas(estudante_id);
CREATE INDEX idx_proj_faltas_academia ON projection_faltas(id_academia);
CREATE INDEX idx_proj_faltas_ano ON projection_faltas(ano_lectivo);

-- ============================================
-- PROJEÇÃO: INSCRIÇÕES
-- ============================================

CREATE TABLE projection_inscricoes (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    estudante_id UUID NOT NULL,
    academia_id UUID NOT NULL,
    tipo VARCHAR(20) NOT NULL,
    ano_inscricao VARCHAR(50) NOT NULL,
    curso VARCHAR(255),
    status VARCHAR(20) NOT NULL,
    
    -- Metadados
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    event_id UUID NOT NULL,
    version INTEGER NOT NULL,
    
    FOREIGN KEY (estudante_id) REFERENCES projection_estudantes(id) ON DELETE CASCADE,
    FOREIGN KEY (academia_id) REFERENCES projection_academias(id) ON DELETE CASCADE
);

CREATE INDEX idx_proj_inscricoes_estudante ON projection_inscricoes(estudante_id);
CREATE INDEX idx_proj_inscricoes_academia ON projection_inscricoes(academia_id);
CREATE INDEX idx_proj_inscricoes_status ON projection_inscricoes(status);

-- ============================================
-- TABELA DE CONTROLE DE PROJEÇÕES
-- ============================================

CREATE TABLE projection_checkpoints (
    projection_name VARCHAR(100) PRIMARY KEY,
    last_processed_event_id BIGINT NOT NULL,
    last_processed_at TIMESTAMP NOT NULL,
    events_processed BIGINT DEFAULT 0,
    is_rebuilding BOOLEAN DEFAULT FALSE,
    rebuild_started_at TIMESTAMP,
    error_count INTEGER DEFAULT 0,
    last_error TEXT,
    last_error_at TIMESTAMP
);

-- Inserir checkpoints iniciais
INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at) VALUES
    ('estudantes', 0, CURRENT_TIMESTAMP),
    ('academias', 0, CURRENT_TIMESTAMP),
    ('notas', 0, CURRENT_TIMESTAMP),
    ('faltas', 0, CURRENT_TIMESTAMP),
    ('inscricoes', 0, CURRENT_TIMESTAMP);

-- ============================================
-- VIEWS DE CONSULTA RÁPIDA
-- ============================================

-- Histórico completo do estudante
CREATE VIEW v_estudante_completo AS
SELECT 
    e.*,
    (SELECT json_agg(n.*) FROM projection_notas n WHERE n.estudante_id = e.id) as notas,
    (SELECT json_agg(f.*) FROM projection_faltas f WHERE f.estudante_id = e.id) as faltas,
    (SELECT json_agg(i.*) FROM projection_inscricoes i WHERE i.estudante_id = e.id) as inscricoes
FROM projection_estudantes e;

-- Estudantes por academia
CREATE VIEW v_estudantes_por_academia AS
SELECT 
    a.id as academia_id,
    a.nome as academia_nome,
    COUNT(e.id) as total_estudantes,
    COUNT(CASE WHEN e.status_escolar = 'ativo' OR e.status_superior = 'ativo' THEN 1 END) as estudantes_ativos
FROM projection_academias a
LEFT JOIN projection_estudantes e ON e.id_academia = a.id
GROUP BY a.id, a.nome;

-- Inscrições pendentes por academia
CREATE VIEW v_inscricoes_pendentes AS
SELECT 
    a.id as academia_id,
    a.nome as academia_nome,
    e.id as estudante_id,
    e.nome as estudante_nome,
    i.tipo,
    i.ano_inscricao,
    i.curso,
    i.created_at
FROM projection_inscricoes i
JOIN projection_academias a ON i.academia_id = a.id
JOIN projection_estudantes e ON i.estudante_id = e.id
WHERE i.status = 'espera'
ORDER BY i.created_at ASC;

-- Performance das academias
CREATE VIEW v_performance_academias AS
SELECT 
    a.id as academia_id,
    a.nome as academia_nome,
    a.provincia,
    COUNT(DISTINCT e.id) as total_estudantes,
    COUNT(DISTINCT n.id) as total_registros_notas,
    COUNT(DISTINCT f.id) as total_registros_faltas
FROM projection_academias a
LEFT JOIN projection_estudantes e ON e.id_academia = a.id
LEFT JOIN projection_notas n ON n.id_academia = a.id
LEFT JOIN projection_faltas f ON f.id_academia = a.id
GROUP BY a.id, a.nome, a.provincia;

-- ============================================
-- FUNÇÕES DE RECONSTRUÇÃO
-- ============================================

-- Marca projeção para reconstrução
CREATE OR REPLACE FUNCTION mark_projection_for_rebuild(p_projection_name VARCHAR)
RETURNS VOID AS $$
BEGIN
    UPDATE projection_checkpoints
    SET 
        is_rebuilding = TRUE,
        rebuild_started_at = CURRENT_TIMESTAMP,
        last_processed_event_id = 0
    WHERE projection_name = p_projection_name;
END;
$$ LANGUAGE plpgsql;

-- Finaliza reconstrução de projeção
CREATE OR REPLACE FUNCTION finish_projection_rebuild(p_projection_name VARCHAR, p_last_event_id BIGINT)
RETURNS VOID AS $$
BEGIN
    UPDATE projection_checkpoints
    SET 
        is_rebuilding = FALSE,
        rebuild_started_at = NULL,
        last_processed_event_id = p_last_event_id,
        last_processed_at = CURRENT_TIMESTAMP
    WHERE projection_name = p_projection_name;
END;
$$ LANGUAGE plpgsql;

-- Registra erro em projeção
CREATE OR REPLACE FUNCTION log_projection_error(
    p_projection_name VARCHAR,
    p_error TEXT
)
RETURNS VOID AS $$
BEGIN
    UPDATE projection_checkpoints
    SET 
        error_count = error_count + 1,
        last_error = p_error,
        last_error_at = CURRENT_TIMESTAMP
    WHERE projection_name = p_projection_name;
END;
$$ LANGUAGE plpgsql;

-- ============================================
-- TRIGGER PARA AUTO-UPDATE DE updated_at
-- ============================================

CREATE OR REPLACE FUNCTION update_projection_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_update_estudante_timestamp
    BEFORE UPDATE ON projection_estudantes
    FOR EACH ROW EXECUTE FUNCTION update_projection_timestamp();

CREATE TRIGGER trigger_update_academia_timestamp
    BEFORE UPDATE ON projection_academias
    FOR EACH ROW EXECUTE FUNCTION update_projection_timestamp();

CREATE TRIGGER trigger_update_inscricao_timestamp
    BEFORE UPDATE ON projection_inscricoes
    FOR EACH ROW EXECUTE FUNCTION update_projection_timestamp();

-- ============================================
-- COMENTÁRIOS
-- ============================================

COMMENT ON TABLE projection_estudantes IS 
'Projeção de leitura para estudantes. Reconstruída a partir do Genesis Ledger.';

COMMENT ON TABLE projection_academias IS 
'Projeção de leitura para academias. Reconstruída a partir do Genesis Ledger.';

COMMENT ON TABLE projection_checkpoints IS 
'Controle de posição de processamento das projeções. Permite reconstrução incremental.';

COMMENT ON VIEW v_estudante_completo IS 
'View agregada com histórico completo do estudante (notas, faltas, inscrições).';

COMMENT ON FUNCTION mark_projection_for_rebuild IS 
'Marca uma projeção para ser reconstruída do zero a partir dos eventos.';