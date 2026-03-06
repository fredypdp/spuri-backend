-- ============================================================================
-- MIGRATION 033 — Remover colunas legadas curso_medio / curso_superior
--
-- DB-10 FIX (auditoria-etapa2-db.md):
--   A migration 004 adicionou curso_medio_id (UUID) e curso_superior_id (UUID)
--   mas deixou as colunas antigas curso_medio (VARCHAR) e curso_superior (VARCHAR)
--   comentadas com aviso de "migrar dados antes". Nenhuma migration posterior
--   executou o DROP. As colunas co-existem, criando ambiguidade de schema.
--
--   Esta migration:
--     1. Verifica se ainda existem dados nas colunas legadas e emite WARNING.
--     2. Dropa as colunas VARCHAR agora que as colunas UUID estão em produção
--        e as projeções já usam exclusivamente curso_medio_id / curso_superior_id.
--     3. Remove as constraints de FK antigas que possam ter sobrado.
-- ============================================================================

BEGIN;

-- ============================================================================
-- 1. Alertar se ainda há dados nas colunas legadas
-- ============================================================================

DO $$
DECLARE
    v_count_medio    INTEGER;
    v_count_superior INTEGER;
BEGIN
    -- Verifica existência das colunas antes de contar (idempotência)
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'projection_estudantes' AND column_name = 'curso_medio'
    ) THEN
        SELECT COUNT(*) INTO v_count_medio
        FROM projection_estudantes
        WHERE curso_medio IS NOT NULL AND curso_medio != '';

        IF v_count_medio > 0 THEN
            RAISE WARNING
                '⚠️  % registros em projection_estudantes ainda têm curso_medio (VARCHAR) preenchido. '
                'Os dados de curso agora são armazenados em curso_medio_id (UUID). '
                'Verifique se o rebuild da projeção estudantes foi executado antes desta migration.',
                v_count_medio;
        ELSE
            RAISE NOTICE '✅ curso_medio (VARCHAR) está vazio — seguro dropar.';
        END IF;
    END IF;

    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'projection_estudantes' AND column_name = 'curso_superior'
    ) THEN
        SELECT COUNT(*) INTO v_count_superior
        FROM projection_estudantes
        WHERE curso_superior IS NOT NULL AND curso_superior != '';

        IF v_count_superior > 0 THEN
            RAISE WARNING
                '⚠️  % registros em projection_estudantes ainda têm curso_superior (VARCHAR) preenchido. '
                'Verifique se o rebuild da projeção estudantes foi executado antes desta migration.',
                v_count_superior;
        ELSE
            RAISE NOTICE '✅ curso_superior (VARCHAR) está vazio — seguro dropar.';
        END IF;
    END IF;
END $$;

-- ============================================================================
-- 2. Dropar colunas legadas (idempotente via IF EXISTS)
-- ============================================================================

ALTER TABLE projection_estudantes
    DROP COLUMN IF EXISTS curso_medio CASCADE;

ALTER TABLE projection_estudantes
    DROP COLUMN IF EXISTS curso_superior CASCADE;

-- ============================================================================
-- 3. Garantir que os índices das colunas UUID existem
--    (já criados na migration 004, mas reforçados aqui por segurança)
-- ============================================================================

CREATE INDEX IF NOT EXISTS idx_estudante_curso_medio
    ON projection_estudantes(curso_medio_id);

CREATE INDEX IF NOT EXISTS idx_estudante_curso_superior
    ON projection_estudantes(curso_superior_id);

-- ============================================================================
-- 4. Atualizar comentário da tabela
-- ============================================================================

COMMENT ON TABLE projection_estudantes IS
    'Projeção de leitura para estudantes. '
    'Colunas curso_medio (VARCHAR) e curso_superior (VARCHAR) removidas em migration 033. '
    'Usar curso_medio_id (UUID FK) e curso_superior_id (UUID FK).';

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 033 — colunas legadas curso_medio/curso_superior removidas de projection_estudantes.';
    RAISE NOTICE '   Se houve WARNING sobre dados existentes, execute:';
    RAISE NOTICE '   POST /admin/rebuild-projection/estudantes';
END $$;
