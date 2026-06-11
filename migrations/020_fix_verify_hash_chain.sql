-- ============================================
-- MIGRATION 020 - Corrigir verify_hash_chain
-- ============================================
-- 
-- Contexto:
-- 1. A função verify_hash_chain já existe na migration 001, mas o Go lê
--    broken_at_version como *int via Scan — a função retorna INTEGER ok.
--    Recriamos aqui para garantir a assinatura correta (idempotente).
-- 2. Mantém a validação da cadeia focada nos eventos persistidos no ledger.
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
