-- MIGRATION 117 - NIF de academia deixa de ser único; cria fluxo de
-- solicitação de alteração de NIF (academia solicita -> admin adm/fpp
-- aprova ou reprova).
--
-- Antes desta migration, projection_academias.nif tinha unicidade
-- operacional (migration 111: único apenas entre academias não deletadas).
-- Regra de negócio nova (Tarefa 81): o mesmo NIF pode estar associado a mais
-- de uma academia na plataforma — ex.: grupos educacionais com a mesma
-- entidade fiscal operando várias unidades/academias. A validação de
-- formato (exatamente 10 dígitos, check_academia_nif_10_digits) continua
-- valendo; apenas a unicidade é removida.

-- 1) Remove a unicidade operacional do NIF.
DROP INDEX IF EXISTS idx_projection_academias_nif_active_unique;

-- Mantém um índice não-único para acelerar buscas administrativas por NIF
-- (ex.: localizar todas as academias associadas a um mesmo NIF).
CREATE INDEX IF NOT EXISTS idx_projection_academias_nif
    ON projection_academias (nif);

COMMENT ON COLUMN projection_academias.nif IS
    'NIF da academia: string obrigatória com exatamente 10 dígitos. NÃO é único — a mesma entidade fiscal pode estar associada a mais de uma academia (Tarefa 81). Alteração de NIF exige aprovação via fluxo de solicitação (ver projection_solicitacoes_alteracao_nif_academia); não pode mais ser alterado diretamente por PUT /academia/dados.';

-- 2) Tabela de solicitações de alteração de NIF — event-sourced, mesmo
--    padrão estrutural de projection_solicitacoes_edicao_dados_estudante
--    (migration 095), mas sem documento comprobatório (não exigido pela
--    regra de negócio desta tarefa) e decidida por um Admin (role adm ou
--    fpp), não pela academia.
CREATE TABLE IF NOT EXISTS projection_solicitacoes_alteracao_nif_academia (
    id UUID PRIMARY KEY,
    codigo_solicitacao VARCHAR(32) UNIQUE NOT NULL,
    codigo_academia VARCHAR(20) NOT NULL,
    nif_atual VARCHAR(10) NOT NULL,
    nif_solicitado VARCHAR(10) NOT NULL,
    status VARCHAR(20) NOT NULL CHECK (status IN ('pendente','aprovada','reprovada')),
    motivo_reprovacao TEXT,
    solicitado_por VARCHAR(20) NOT NULL,
    decidido_por TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    version INTEGER NOT NULL DEFAULT 1,
    last_event_id BIGINT,
    CONSTRAINT check_solicitacao_nif_10_digits CHECK (nif_solicitado ~ '^[0-9]{10}$')
);

-- Apenas uma solicitação pendente por academia por vez (mesma filosofia da
-- migration 095: idx_solicitacoes_edicao_dados_estudante_pendente).
CREATE UNIQUE INDEX IF NOT EXISTS idx_solicitacoes_alteracao_nif_academia_pendente
    ON projection_solicitacoes_alteracao_nif_academia (codigo_academia)
    WHERE status = 'pendente';

CREATE INDEX IF NOT EXISTS idx_solicitacoes_alteracao_nif_academia_academia_status
    ON projection_solicitacoes_alteracao_nif_academia (codigo_academia, status);
CREATE INDEX IF NOT EXISTS idx_solicitacoes_alteracao_nif_academia_status
    ON projection_solicitacoes_alteracao_nif_academia (status);

COMMENT ON TABLE projection_solicitacoes_alteracao_nif_academia IS
    'Projeção das solicitações de alteração de NIF de academia. A alteração real de projection_academias.nif só acontece quando um Admin (role adm ou fpp) aprova a solicitação — ver evento AcademiaNIFAlteradoPorSolicitacao.';

DO $$ BEGIN RAISE NOTICE '✅ MIGRATION 117 - NIF de academia não é mais único; fluxo de solicitação de alteração criado'; END $$;
