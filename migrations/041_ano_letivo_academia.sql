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