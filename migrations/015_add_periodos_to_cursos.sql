-- ============================================================================
-- MIGRATION 015 — Adicionar campo "periodos" em projection_cursos
--
-- REGRA DE NEGÓCIO:
--   • type = 'superior' → periodos é OBRIGATÓRIO (array JSON com 1+ itens)
--   • type = 'medio'    → periodos é NULL (escolas usam trimestres fixos)
--
-- Valores permitidos por item: 1_trimestre | 2_trimestre | 3_trimestre |
--                              1_semestre  | 2_semestre
-- ============================================================================

BEGIN;

-- 1. Adicionar coluna periodos (nullable — medio não usa)
ALTER TABLE projection_cursos
    ADD COLUMN IF NOT EXISTS periodos JSONB DEFAULT NULL;

COMMENT ON COLUMN projection_cursos.periodos IS
    'Apenas para type=superior: array JSON com os períodos letivos do curso. '
    'Ex: ["1_semestre","2_semestre"]. NULL para tipo medio.';

-- 2. Criar índice GIN para buscas por período
CREATE INDEX IF NOT EXISTS idx_cursos_periodos
    ON projection_cursos USING GIN (periodos)
    WHERE periodos IS NOT NULL;

-- 3. Verificar cursos superiores sem periodos (dados existentes — atenção!)
DO $$
DECLARE
    v_count INTEGER;
BEGIN
    SELECT COUNT(*)
    INTO v_count
    FROM projection_cursos
    WHERE type = 'superior'
      AND (periodos IS NULL OR periodos = 'null'::jsonb OR jsonb_array_length(COALESCE(periodos, '[]'::jsonb)) = 0);

    IF v_count > 0 THEN
        RAISE WARNING
            '⚠️  Existem % cursos superiores sem periodos definidos. '
            'Atualize-os via PUT /academia/cursos/:id antes de registrar novas notas.',
            v_count;
    ELSE
        RAISE NOTICE '✅ Nenhum curso superior sem periodos encontrado.';
    END IF;
END $$;

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 015 — periodos adicionado em projection_cursos.';
END $$;
