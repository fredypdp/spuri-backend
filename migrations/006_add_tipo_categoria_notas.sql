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
