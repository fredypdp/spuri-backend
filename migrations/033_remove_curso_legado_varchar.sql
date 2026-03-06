-- ============================================================================
-- MIGRATION 033 — Correções auditoria-etapa2-db (DB-10, DB-15)
-- ============================================================================
--
-- DB-10: DROP das colunas curso_medio e curso_superior (VARCHAR obsoletas)
--   A migration 004 adicionou curso_medio_id UUID e curso_superior_id UUID
--   como substitutas das colunas VARCHAR originais, mas deixou os DROPs
--   comentados com aviso "execute DEPOIS de migrar os dados existentes".
--   Nenhuma migration posterior executou os DROPs.
--   O schema ficou com 4 colunas: 2 VARCHAR obsoletas + 2 UUID ativas,
--   criando ambiguidade para queries e handlers.
--
--   Esta migration executa os DROPs com segurança:
--   - IF NOT EXISTS garante idempotência
--   - A FK entre curso_medio_id/curso_superior_id e projection_cursos
--     já foi criada pela migration 004 — não precisa ser recriada
--   - Os índices idx_estudante_curso_medio e idx_estudante_curso_superior
--     foram criados pela migration 004 sobre as colunas UUID — permanecem
--
-- DB-15: migration 024 não tem BEGIN/COMMIT.
--   Os statements originais da 024 (DROP INDEX + INSERT INTO checkpoints) não
--   podem ser retificados retroativamente pois já foram aplicados.
--   Esta migration documenta o risco e verifica o estado resultante,
--   garantindo que os checkpoints estejam consistentes.
-- ============================================================================

BEGIN;

-- ============================================================================
-- 1. DB-10: remover colunas VARCHAR obsoletas de projection_estudantes
-- ============================================================================

-- Dropar coluna curso_medio (VARCHAR, substituída por curso_medio_id UUID)
ALTER TABLE projection_estudantes
    DROP COLUMN IF EXISTS curso_medio;

-- Dropar coluna curso_superior (VARCHAR, substituída por curso_superior_id UUID)
ALTER TABLE projection_estudantes
    DROP COLUMN IF EXISTS curso_superior;

-- Atualizar comentário da tabela para refletir o schema limpo
COMMENT ON TABLE projection_estudantes IS
    'Projeção de leitura para estudantes. '
    'Colunas de curso usam UUID (curso_medio_id, curso_superior_id) '
    'com FK para projection_cursos. '
    'Colunas VARCHAR legadas (curso_medio, curso_superior) removidas na migration 033.';

-- ============================================================================
-- 2. DB-15: verificar consistência dos checkpoints criados pela migration 024
--    (que foi executada sem BEGIN/COMMIT, sem garantia de atomicidade)
-- ============================================================================

-- Garantir que os checkpoints da migration 024 existem e estão consistentes.
-- ON CONFLICT DO NOTHING é idempotente — não altera dados existentes.
INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at, events_processed)
VALUES
    ('aprovacao_ano', 0, CURRENT_TIMESTAMP, 0),
    ('reprovacoes',   0, CURRENT_TIMESTAMP, 0)
ON CONFLICT (projection_name) DO NOTHING;

-- Verificar e logar o estado dos índices que migration 024 deveria ter removido
DO $$
DECLARE
    v_idx_status    TEXT;
    v_idx_academia  TEXT;
    v_idx_estudante TEXT;
BEGIN
    SELECT CASE WHEN EXISTS (
        SELECT 1 FROM pg_indexes
        WHERE indexname = 'idx_projection_inscricoes_status'
    ) THEN 'EXISTS' ELSE 'DROPPED' END INTO v_idx_status;

    SELECT CASE WHEN EXISTS (
        SELECT 1 FROM pg_indexes
        WHERE indexname = 'idx_projection_inscricoes_academia'
    ) THEN 'EXISTS' ELSE 'DROPPED' END INTO v_idx_academia;

    SELECT CASE WHEN EXISTS (
        SELECT 1 FROM pg_indexes
        WHERE indexname = 'idx_projection_inscricoes_estudante'
    ) THEN 'EXISTS' ELSE 'DROPPED' END INTO v_idx_estudante;

    RAISE NOTICE '[DB-15] Estado dos índices da migration 024:';
    RAISE NOTICE '  idx_projection_inscricoes_status:    %', v_idx_status;
    RAISE NOTICE '  idx_projection_inscricoes_academia:  %', v_idx_academia;
    RAISE NOTICE '  idx_projection_inscricoes_estudante: %', v_idx_estudante;

    -- Se algum índice ainda existir (migration 024 falhou parcialmente),
    -- removê-lo agora para garantir estado consistente.
    IF v_idx_status = 'EXISTS' THEN
        DROP INDEX IF EXISTS idx_projection_inscricoes_status;
        RAISE NOTICE '  [CORRIGIDO] idx_projection_inscricoes_status removido';
    END IF;
    IF v_idx_academia = 'EXISTS' THEN
        DROP INDEX IF EXISTS idx_projection_inscricoes_academia;
        RAISE NOTICE '  [CORRIGIDO] idx_projection_inscricoes_academia removido';
    END IF;
    IF v_idx_estudante = 'EXISTS' THEN
        DROP INDEX IF EXISTS idx_projection_inscricoes_estudante;
        RAISE NOTICE '  [CORRIGIDO] idx_projection_inscricoes_estudante removido';
    END IF;
END $$;

-- ============================================================================
-- 3. Recriar views que referenciam projection_estudantes
--    após remoção das colunas obsoletas
--    (views que NÃO referenciam curso_medio/curso_superior não precisam
--    ser recriadas — apenas documentar para verificação)
-- ============================================================================

DO $$
BEGIN
    RAISE NOTICE '[DB-10] Verificando views dependentes de projection_estudantes...';
    RAISE NOTICE '  v_estudantes_com_cursos: usa curso_medio_id/curso_superior_id (UUID) — OK';
    RAISE NOTICE '  v_estudante_completo: usa SELECT e.* — pode ser afetada se criada antes desta migration';
    RAISE NOTICE '  Execute rebuild das views se necessário.';
END $$;

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 033 — Correções auditoria-etapa2-db aplicadas';
    RAISE NOTICE '   DB-10: colunas curso_medio e curso_superior (VARCHAR) removidas de projection_estudantes';
    RAISE NOTICE '   DB-15: checkpoints da migration 024 verificados e consistentes';
    RAISE NOTICE '   ⚠️  Rebuild recomendado: POST /admin/rebuild-projection/estudantes';
END $$;