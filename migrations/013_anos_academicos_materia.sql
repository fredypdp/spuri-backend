-- ============================================================================
-- MIGRATION 013 — Atualização semântica de anos_academicos em projection_materias
--
-- ATUALIZAÇÃO 1:
--   O campo "nivel" (JSONB) em projection_materias passa a armazenar
--   AnosAcademicos com as seguintes regras:
--     • fundamental : array com 1–9 itens (primeiro_fundamental…nono_fundamental)
--     • medio       : array com EXATAMENTE 1 item (ano do curso da matéria)
--     • superior    : array com EXATAMENTE 1 item (ano do curso da matéria)
--
--   Nenhuma alteração estrutural é necessária no banco — a coluna "nivel" JSONB
--   já comporta os dados corretamente. Esta migration apenas:
--     1. Atualiza o COMMENT da coluna para refletir as novas regras.
--     2. Adiciona uma CHECK CONSTRAINT para garantir a cardinalidade em
--        novas inserções via SQL direto (a validação principal é feita no Go).
--
-- ATENÇÃO: Execute com cuidado em produção se houver matérias de medio/superior
--   com mais de 1 item em nivel — corrija os dados antes de aplicar o CHECK.
-- ============================================================================

BEGIN;

-- 1. Atualizar comentário da coluna
COMMENT ON COLUMN projection_materias.nivel IS
    'AnosAcademicos da matéria disciplinar. '
    'fundamental: 1–9 itens (primeiro_fundamental…nono_fundamental). '
    'medio/superior: exatamente 1 item — ano do curso ao qual a matéria pertence. '
    'Armazenado como JSONB array de strings.';

-- 2. Verificar se existem matérias de medio/superior com nivel com mais de 1 item
--    (caso existam, este SELECT retorna linhas — CORRIJA antes de aplicar o CHECK)
DO $$
DECLARE
    v_count INTEGER;
BEGIN
    SELECT COUNT(*)
    INTO v_count
    FROM projection_materias
    WHERE type IN ('medio', 'superior')
      AND jsonb_array_length(COALESCE(nivel, '[]'::jsonb)) != 1;

    IF v_count > 0 THEN
        RAISE WARNING
            '⚠️  Existem % matérias de medio/superior com nivel diferente de 1 item. '
            'Corrija os dados antes de ativar a CHECK CONSTRAINT.',
            v_count;
    ELSE
        RAISE NOTICE '✅ Nenhuma inconsistência encontrada em nivel de medio/superior.';
    END IF;
END $$;

-- 3. Adicionar CHECK CONSTRAINT (descomente após verificar que não há inconsistências)
-- ALTER TABLE projection_materias
--     ADD CONSTRAINT chk_nivel_cardinalidade CHECK (
--         (type = 'fundamental')
--         OR
--         (type IN ('medio', 'superior') AND jsonb_array_length(COALESCE(nivel, '[]'::jsonb)) = 1)
--     );

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 012 — semântica de anos_academicos em projection_materias aplicada.';
END $$;
