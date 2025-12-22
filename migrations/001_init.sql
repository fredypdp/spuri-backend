-- Spuri Database Schema
-- Event Sourcing Architecture

-- Extensão para UUID
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ============================================
-- TABELAS PRINCIPAIS
-- ============================================

-- Escolas e Universidades
CREATE TABLE escolas_universidades (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    type VARCHAR(20) NOT NULL CHECK (type IN ('escola', 'superior')),
    nome VARCHAR(255) NOT NULL,
    codigo_academia VARCHAR(50) UNIQUE NOT NULL,
    senha_hash VARCHAR(255) NOT NULL,
    endereco TEXT NOT NULL,
    numero_telefone VARCHAR(20),
    email VARCHAR(100),
    website VARCHAR(255),
    nivel_escolar VARCHAR(20) CHECK (nivel_escolar IN ('fundamental', 'medio')),
    status VARCHAR(20) DEFAULT 'ativo' CHECK (status IN ('ativo', 'inativo')),
    cursos JSONB DEFAULT '[]',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Cursos
CREATE TABLE cursos (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    nome VARCHAR(255) NOT NULL,
    type VARCHAR(20) NOT NULL CHECK (type IN ('medio', 'superior')),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Contas Administrativas
CREATE TABLE contas_adm (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    nome VARCHAR(255) NOT NULL,
    senha_hash VARCHAR(255) NOT NULL,
    status VARCHAR(20) DEFAULT 'ativo' CHECK (status IN ('ativo', 'inativo')),
    role VARCHAR(20) NOT NULL CHECK (role IN ('adm', 'gerente', 'fpp')),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Estudantes
CREATE TABLE estudantes (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    nome VARCHAR(255) NOT NULL,
    senha_hash VARCHAR(255) NOT NULL,
    bilhete_identidade VARCHAR(50),
    bilhete_identidade_responsavel VARCHAR(50),
    id_academia UUID REFERENCES escolas_universidades(id),
    ano_escolar VARCHAR(50),
    ano_superior VARCHAR(50),
    curso_medio VARCHAR(255),
    curso_superior VARCHAR(255),
    status_escolar VARCHAR(20) CHECK (status_escolar IN ('ativo', 'inativo', 'finalizado')),
    status_superior VARCHAR(20) CHECK (status_superior IN ('ativo', 'inativo', 'finalizado')),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    -- Constraints para garantir que pelo menos um bilhete existe
    CHECK (
        bilhete_identidade IS NOT NULL OR 
        bilhete_identidade_responsavel IS NOT NULL
    )
);

-- ============================================
-- EVENT STORE (Núcleo do Event Sourcing)
-- ============================================

CREATE TABLE event_store (
    id BIGSERIAL PRIMARY KEY,
    event_id UUID UNIQUE NOT NULL DEFAULT uuid_generate_v4(),
    aggregate_id UUID NOT NULL,
    aggregate_type VARCHAR(50) NOT NULL,
    event_type VARCHAR(100) NOT NULL,
    payload JSONB NOT NULL,
    metadata JSONB DEFAULT '{}',
    occurred_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    
    -- Para ordenação garantida
    version INTEGER NOT NULL DEFAULT 1
);

-- Índices para performance do Event Store
CREATE INDEX idx_event_store_aggregate ON event_store(aggregate_id, version);
CREATE INDEX idx_event_store_type ON event_store(event_type);
CREATE INDEX idx_event_store_occurred ON event_store(occurred_at);

-- ============================================
-- READ MODELS (Projeções para Leitura Rápida)
-- ============================================

-- Registro de Notas (Projeção)
CREATE TABLE registro_notas (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    estudante_id UUID NOT NULL REFERENCES estudantes(id),
    id_academia UUID NOT NULL REFERENCES escolas_universidades(id),
    ano_lectivo VARCHAR(20) NOT NULL,
    periodo VARCHAR(50) NOT NULL,
    materias JSONB NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    event_id UUID REFERENCES event_store(event_id)
);

CREATE INDEX idx_notas_estudante ON registro_notas(estudante_id);
CREATE INDEX idx_notas_academia ON registro_notas(id_academia);

-- Registro de Faltas (Projeção)
CREATE TABLE registro_faltas (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    estudante_id UUID NOT NULL REFERENCES estudantes(id),
    id_academia UUID NOT NULL REFERENCES escolas_universidades(id),
    ano_lectivo VARCHAR(20) NOT NULL,
    periodo VARCHAR(50) NOT NULL,
    materias JSONB NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    event_id UUID REFERENCES event_store(event_id)
);

CREATE INDEX idx_faltas_estudante ON registro_faltas(estudante_id);
CREATE INDEX idx_faltas_academia ON registro_faltas(id_academia);

-- ============================================
-- INSCRIÇÕES
-- ============================================

CREATE TABLE inscricoes (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    estudante_id UUID NOT NULL REFERENCES estudantes(id),
    academia_id UUID NOT NULL REFERENCES escolas_universidades(id),
    tipo VARCHAR(20) NOT NULL CHECK (tipo IN ('escola', 'universidade')),
    ano_inscricao VARCHAR(50) NOT NULL,
    curso VARCHAR(255),
    status VARCHAR(20) DEFAULT 'espera' CHECK (status IN ('espera', 'aprovado', 'reprovado')),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_inscricoes_estudante ON inscricoes(estudante_id);
CREATE INDEX idx_inscricoes_academia ON inscricoes(academia_id);
CREATE INDEX idx_inscricoes_status ON inscricoes(status);

-- ============================================
-- FUNÇÕES AUXILIARES
-- ============================================

-- Trigger para atualizar updated_at
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER update_inscricoes_updated_at 
    BEFORE UPDATE ON inscricoes 
    FOR EACH ROW 
    EXECUTE FUNCTION update_updated_at_column();

-- ============================================
-- DADOS INICIAIS (Opcional)
-- ============================================

-- Inserir alguns cursos padrão
INSERT INTO cursos (nome, type) VALUES
    ('Ciências Físicas e Biológicas', 'medio'),
    ('Ciências Económicas e Jurídicas', 'medio'),
    ('Engenharia Informática', 'superior'),
    ('Medicina', 'superior'),
    ('Direito', 'superior'),
    ('Economia', 'superior');

-- Conta admin padrão (senha: admin123)
-- IMPORTANTE: Mudar a senha em produção!
INSERT INTO contas_adm (nome, senha_hash, role) VALUES
    ('Administrador', '$2a$10$rXqVqE8Z8Z8Z8Z8Z8Z8Z8O5.5.5.5.5.5.5.5.5.5.5.5.5.5.5', 'fpp');

COMMENT ON TABLE event_store IS 'Event Store: Fonte única da verdade. Todos os eventos são imutáveis.';
COMMENT ON TABLE registro_notas IS 'Read Model: Projeção otimizada para consulta de notas.';
COMMENT ON TABLE registro_faltas IS 'Read Model: Projeção otimizada para consulta de faltas.';
COMMENT ON COLUMN event_store.aggregate_id IS 'ID da entidade (estudante, escola, etc.)';
COMMENT ON COLUMN event_store.aggregate_type IS 'Tipo da entidade (Estudante, Academia, etc.)';
COMMENT ON COLUMN event_store.event_type IS 'Tipo do evento (NotasRegistradas, FaltasRegistradas, etc.)';
COMMENT ON COLUMN event_store.payload IS 'Dados completos do evento em JSON';
COMMENT ON COLUMN event_store.metadata IS 'Metadados: IP, user agent, etc.';

-- Visualização para auditoria rápida
CREATE VIEW auditoria_eventos AS
SELECT 
    e.event_id,
    e.aggregate_type,
    e.event_type,
    CASE 
        WHEN e.aggregate_type = 'Estudante' THEN est.nome
        WHEN e.aggregate_type = 'Academia' THEN esc.nome
        ELSE 'N/A'
    END as entidade_nome,
    e.occurred_at,
    e.payload
FROM event_store e
LEFT JOIN estudantes est ON e.aggregate_id = est.id AND e.aggregate_type = 'Estudante'
LEFT JOIN escolas_universidades esc ON e.aggregate_id = esc.id AND e.aggregate_type = 'Academia'
ORDER BY e.occurred_at DESC;