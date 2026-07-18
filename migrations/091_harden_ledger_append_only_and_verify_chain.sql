-- ============================================================================
-- MIGRATION 091 — Reforçar ledger append-only e verificação completa da cadeia
-- ============================================================================
--
-- Auditoria de integridade do event sourcing:
--   * A aplicação usa a role de runtime configurada em DATABASE_URL ou nas vars
--     DB_USER/DB_PASSWORD/DB_HOST/DB_NAME (ver internal/db/client.go).
--   * Como o projeto não separa, hoje, uma role administrativa para migrations e
--     uma role de runtime para a API, o controle preventivo escolhido é trigger.
--   * Triggers BEFORE UPDATE/DELETE/TRUNCATE tornam spuri_ledger append-only para
--     qualquer role que não seja superuser com poderes de desabilitar triggers.
--   * INSERT e SELECT continuam permitidos para o fluxo normal de gravação e
--     leitura de eventos.
--
-- A função verify_hash_chain também passa a validar invariantes estruturais:
--   * primeira versão deve ser 1 e previous_hash deve ser NULL;
--   * versões devem ser contíguas (sem lacunas após DELETE físico);
--   * previous_hash deve apontar para o ledger_hash imediatamente anterior;
--   * ledger_hash deve bater com event_id, aggregate_id, event_type, payload e
--     previous_hash. Metadata NÃO participa do hash por compatibilidade com o
--     algoritmo histórico e permanece uma limitação documentada.
-- ============================================================================

BEGIN;

CREATE OR REPLACE FUNCTION prevent_ledger_modification()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION
        'Spuri Ledger é append-only. Operação % não permitida em spuri_ledger.',
        TG_OP
        USING ERRCODE = '45000';
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION prevent_ledger_modification() IS
    'Bloqueia UPDATE e DELETE em spuri_ledger para impor append-only no banco. '
    'Como a aplicação e migrations podem compartilhar a mesma role, trigger é '
    'o controle preventivo escolhido em vez de depender apenas de GRANT/REVOKE.';

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

CREATE OR REPLACE FUNCTION prevent_ledger_truncate()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION
        'Spuri Ledger é append-only. TRUNCATE não permitido em spuri_ledger.'
        USING ERRCODE = '45000';
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS prevent_truncate_ledger ON spuri_ledger;
CREATE TRIGGER prevent_truncate_ledger
    BEFORE TRUNCATE ON spuri_ledger
    FOR EACH STATEMENT
    EXECUTE FUNCTION prevent_ledger_truncate();

CREATE OR REPLACE FUNCTION verify_hash_chain(p_aggregate_id UUID)
RETURNS TABLE(
    is_valid          BOOLEAN,
    broken_at_version INTEGER,
    message           TEXT
) AS $$
DECLARE
    v_current_hash      VARCHAR(64);
    v_event             RECORD;
    v_expected_hash     VARCHAR(64);
    v_expected_version  INTEGER := 1;
BEGIN
    FOR v_event IN
        SELECT *
        FROM spuri_ledger
        WHERE aggregate_id = p_aggregate_id
        ORDER BY event_version ASC, recorded_at ASC, id ASC
    LOOP
        IF v_event.event_version <> v_expected_version THEN
            is_valid          := FALSE;
            broken_at_version := v_event.event_version;
            message           := format(
                'Sequência de versões inválida: esperado event_version=%s, encontrado event_version=%s (event_id=%s)',
                v_expected_version, v_event.event_version, v_event.event_id
            );
            RETURN NEXT;
            RETURN;
        END IF;

        IF v_expected_version = 1 AND v_event.previous_hash IS NOT NULL THEN
            is_valid          := FALSE;
            broken_at_version := v_event.event_version;
            message           := format(
                'Primeiro evento deve ter previous_hash NULL, encontrado %s (event_id=%s)',
                v_event.previous_hash, v_event.event_id
            );
            RETURN NEXT;
            RETURN;
        END IF;

        IF v_expected_version > 1 AND v_event.previous_hash IS DISTINCT FROM v_current_hash THEN
            is_valid          := FALSE;
            broken_at_version := v_event.event_version;
            message           := format(
                'Cadeia de hashes quebrada no event_version=%s: previous_hash=%s esperado=%s',
                v_event.event_version, v_event.previous_hash, v_current_hash
            );
            RETURN NEXT;
            RETURN;
        END IF;

        v_expected_hash := generate_ledger_hash(
            v_event.event_id,
            v_event.aggregate_id,
            v_event.event_type,
            v_event.payload,
            v_event.previous_hash
        );

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

        v_current_hash := v_event.ledger_hash;
        v_expected_version := v_expected_version + 1;
    END LOOP;

    is_valid          := TRUE;
    broken_at_version := NULL;
    message           := 'Cadeia de hashes íntegra';
    RETURN NEXT;
END;
$$ LANGUAGE plpgsql STABLE;

COMMENT ON FUNCTION verify_hash_chain(UUID) IS
    'Valida hashes e sequência contígua por aggregate. O hash protege event_id, '
    'aggregate_id, event_type, payload e previous_hash; metadata não participa '
    'do cálculo por compatibilidade histórica.';

COMMIT;
