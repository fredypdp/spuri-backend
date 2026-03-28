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
