-- ============================================================================
-- MIGRATION 035 — Criar função spuri_generate_codigo_academia
-- ============================================================================
--
-- CONTEXTO:
--   O handler RegisterAcademia chama `SELECT spuri_generate_codigo_academia($1)`
--   desde a migration 001, mas a função nunca foi definida em nenhuma migration.
--   Qualquer cadastro de academia caia silenciosamente no fallback hash do Go,
--   gerando códigos no formato "{PROV}XXXXXXXX" (8 dígitos aleatórios) em vez
--   do formato correto.
--
-- FORMATO CORRETO: {PROVINCIA}{ANO}{SEQUENCIAL}
--   Exemplos:
--     LDA20261  →  1ª academia cadastrada em Luanda no ano de 2026
--     LDA20262  →  2ª academia cadastrada em Luanda no ano de 2026
--     BGU20261  →  1ª academia cadastrada em Benguela no ano de 2026
--     HUI20268  →  8ª academia cadastrada em Huila no ano de 2026
--
--   A sequência é reiniciada por província a cada ano-calendário.
--   O número sequencial não tem zero-padding — cresce naturalmente (1, 2, … N).
--
-- GARANTIA DE UNICIDADE:
--   A função conta academias existentes com o prefixo "{PROV}{ANO}" e incrementa.
--   Um loop WHILE protege contra race conditions em cadastros simultâneos na
--   mesma província/ano: se o código já existir, tenta o próximo.
--   O UNIQUE constraint em projection_academias.codigo_academia é a barreira
--   definitiva — a função minimiza colisões, o constraint as elimina.
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

    -- Ponto de partida: quantas academias já existem com este prefixo?
    -- +1 para obter a próxima posição disponível.
    SELECT COUNT(*) + 1
      INTO v_seq
      FROM projection_academias
     WHERE codigo_academia LIKE v_prefix || '%';

    v_codigo := v_prefix || v_seq::TEXT;

    -- Loop de proteção contra race condition:
    -- se dois cadastros simultâneos chegarem ao mesmo seq, um deles tenta o próximo.
    WHILE EXISTS (
        SELECT 1 FROM projection_academias WHERE codigo_academia = v_codigo
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
    'A sequência reinicia por província a cada ano-calendário. '
    'O loop WHILE protege contra race conditions em cadastros simultâneos.';

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 035 — spuri_generate_codigo_academia criada';
    RAISE NOTICE '   Formato: {PROVINCIA}{ANO}{SEQ} — ex: LDA20261, LDA20262, BGU20261';
    RAISE NOTICE '   Sequência por (província, ano), sem zero-padding no sequencial.';
END $$;
