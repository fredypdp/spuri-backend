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
        'primeiro_fundamental', 'segundo_fundamental', 'terceiro_fundamental',
        'quarto_fundamental', 'quinto_fundamental', 'sexto_fundamental',
        'setimo_fundamental', 'oitavo_fundamental', 'nono_fundamental',
        'primeiro_medio', 'segundo_medio', 'terceiro_medio'
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

RAISE NOTICE '✅ MIGRATION 003 CONCLUÍDA - Sistema de Aprovação de Ano';