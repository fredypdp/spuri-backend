-- ============================================================================
-- MIGRATION 093 — Reservas de códigos gerados para cadastros assíncronos
-- ============================================================================
--
-- Garante unicidade antes da materialização das projeções e antes mesmo do
-- evento ser salvo no ledger. Isso evita que jobs assíncronos concorrentes de
-- cadastro de academias/estudantes recebam o mesmo código durante a janela entre
-- geração, upload de documentos e gravação do evento.
-- ============================================================================

BEGIN;

CREATE TABLE IF NOT EXISTS codigo_academia_reservas (
    codigo_academia VARCHAR(50) PRIMARY KEY,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS codigo_estudante_reservas (
    codigo_estudante VARCHAR(7) PRIMARY KEY,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE OR REPLACE FUNCTION spuri_generate_codigo_academia(p_provincia_code VARCHAR)
RETURNS VARCHAR AS $$
DECLARE
    v_ano    INTEGER;
    v_prefix VARCHAR;
    v_seq    INTEGER;
    v_codigo VARCHAR;
BEGIN
    v_ano    := EXTRACT(YEAR FROM CURRENT_TIMESTAMP)::INTEGER;
    v_prefix := p_provincia_code || v_ano::TEXT;

    SELECT COUNT(*) + 1
      INTO v_seq
      FROM spuri_ledger
     WHERE event_type = 'AcademiaCriada'
       AND payload->>'CodigoAcademia' LIKE v_prefix || '%';

    LOOP
        v_codigo := v_prefix || v_seq::TEXT;

        IF NOT EXISTS (
            SELECT 1
              FROM spuri_ledger
             WHERE event_type = 'AcademiaCriada'
               AND payload->>'CodigoAcademia' = v_codigo
        ) THEN
            INSERT INTO codigo_academia_reservas (codigo_academia)
            VALUES (v_codigo)
            ON CONFLICT DO NOTHING;

            IF FOUND THEN
                RETURN v_codigo;
            END IF;
        END IF;

        v_seq := v_seq + 1;
    END LOOP;
END;
$$ LANGUAGE plpgsql VOLATILE;

COMMENT ON TABLE codigo_academia_reservas IS
    'Reserva códigos de academia no momento da geração para impedir repetição em cadastros assíncronos concorrentes.';

COMMENT ON TABLE codigo_estudante_reservas IS
    'Reserva códigos de estudante no momento da geração para impedir repetição em cadastros assíncronos concorrentes.';

COMMENT ON FUNCTION spuri_generate_codigo_academia(VARCHAR) IS
    'Gera e reserva um código único para academia no formato {PROVINCIA}{ANO}{SEQUENCIAL}, consultando ledger e tabela de reservas.';

COMMIT;
