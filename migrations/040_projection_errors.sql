-- ============================================================================
-- MIGRATION 028 - Tabela projection_errors
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
-- A tabela é apenas de diagnóstico — não afeta o fluxo principal do event sourcing.
-- Permite auditoria de eventos que falharam permanentemente após todas as tentativas.
-- ============================================================================

BEGIN;

CREATE TABLE IF NOT EXISTS projection_errors (
    id                  BIGSERIAL PRIMARY KEY,
    projection_name     VARCHAR(100)  NOT NULL,
    event_id            BIGINT        NOT NULL,
    aggregate_id        UUID,
    aggregate_type      VARCHAR(100),
    event_type          VARCHAR(100),
    error_message       TEXT          NOT NULL,
    attempts            INTEGER       NOT NULL DEFAULT 1,
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

COMMENT ON COLUMN projection_errors.event_id         IS 'ID sequencial do evento no spuri_ledger';
COMMENT ON COLUMN projection_errors.projection_name  IS 'Nome da projeção que falhou (ex: academias, estudantes)';
COMMENT ON COLUMN projection_errors.attempts         IS 'Número de tentativas realizadas antes de registrar falha permanente';
COMMENT ON COLUMN projection_errors.resolved         IS 'TRUE quando a falha foi resolvida manualmente (ex: após rebuild)';
COMMENT ON COLUMN projection_errors.resolved_by      IS 'Identificador de quem/o que resolveu a falha (ex: admin UUID, rebuild)';

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 028 - projection_errors criada com sucesso';
    RAISE NOTICE '   Erros de projeção agora são registrados em vez de logar apenas no console.';
    RAISE NOTICE '   Consulte: SELECT * FROM projection_errors WHERE resolved = FALSE ORDER BY last_failed_at DESC;';
END $$;
