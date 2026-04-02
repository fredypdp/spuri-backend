-- ============================================================================
-- MIGRATION 045 — Corrigir spuri_generate_codigo_academia
--
-- PROBLEMA CORRIGIDO:
--   A função original consultava projection_academias (projeção assíncrona)
--   para determinar o próximo sequencial. Em cadastros rápidos em sequência,
--   a projeção ainda não materializou o evento anterior quando o próximo
--   chegou — resultado: duas academias recebiam o mesmo código (ex: BGO20261),
--   causando violação de unique constraint na projeção e travamento permanente
--   do pipeline de eventos.
--
-- SOLUÇÃO:
--   Consultar spuri_ledger (fonte de verdade imutável, síncrona com o INSERT)
--   em vez de projection_academias. O payload do evento AcademiaCriada contém
--   CodigoAcademia como campo JSON — extraímos via ->> para comparar o prefixo.
--
-- FORMATO DO CÓDIGO: {PROVINCIA}{ANO}{SEQUENCIAL}
--   Ex: LDA20261, LDA20262, BGO20261
--   O sequencial reinicia por (província, ano). Sem zero-padding.
--
-- Idempotente: CREATE OR REPLACE.
-- ============================================================================

BEGIN;

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

    -- Contar academias já gravadas no LEDGER com este prefixo.
    -- O ledger é síncrono: o INSERT do evento já ocorreu antes desta função
    -- ser chamada na mesma transação (ou imediatamente antes).
    -- Usar payload->>'CodigoAcademia' para extrair o código do JSON.
    SELECT COUNT(*) + 1
      INTO v_seq
      FROM spuri_ledger
     WHERE event_type = 'AcademiaCriada'
       AND payload->>'CodigoAcademia' LIKE v_prefix || '%';

    v_codigo := v_prefix || v_seq::TEXT;

    -- Loop de proteção: se por alguma race condition o código já existir
    -- no ledger, incrementar até encontrar um livre.
    WHILE EXISTS (
        SELECT 1
        FROM spuri_ledger
        WHERE event_type = 'AcademiaCriada'
          AND payload->>'CodigoAcademia' = v_codigo
    ) LOOP
        v_seq    := v_seq + 1;
        v_codigo := v_prefix || v_seq::TEXT;
    END LOOP;

    RETURN v_codigo;
END;
$$ LANGUAGE plpgsql VOLATILE;

COMMENT ON FUNCTION spuri_generate_codigo_academia(VARCHAR) IS
    'Gera um código único para academia no formato {PROVINCIA}{ANO}{SEQUENCIAL}. '
    'Exemplo: LDA20261 = 1ª academia de Luanda em 2026. '
    'Consulta o spuri_ledger (não a projeção) para garantir unicidade mesmo '
    'em cadastros simultâneos antes da projeção ser materializada. '
    'Corrigido na migration 045: substitui consulta a projection_academias '
    'por consulta ao ledger (fonte de verdade imutável).';

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 045 — spuri_generate_codigo_academia corrigida';
    RAISE NOTICE '   Agora consulta spuri_ledger em vez de projection_academias.';
    RAISE NOTICE '   Elimina race condition em cadastros rápidos em sequência.';
END $$;
