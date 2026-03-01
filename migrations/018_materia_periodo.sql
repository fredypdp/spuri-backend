-- ============================================================================
-- MIGRATION 018 — Adicionar campo 'periodo' em projection_materias
--
-- REGRAS:
--   • Apenas matérias do type='superior' usam este campo.
--   • Valores aceitos: 1_semestre, 2_semestre (subconjunto dos periodos do curso).
--   • NULL para type='fundamental' e type='medio'.
--   • Matérias superior são criadas como 'inativo' e só podem ser ativadas
--     quando periodo estiver preenchido (regra aplicada no domínio Go).
-- ============================================================================

BEGIN;

-- 1. Adicionar coluna
ALTER TABLE projection_materias
    ADD COLUMN IF NOT EXISTS periodo VARCHAR(20) DEFAULT NULL;

-- 2. Constraint: período só pode ser um dos valores válidos (ou NULL)
ALTER TABLE projection_materias
    DROP CONSTRAINT IF EXISTS chk_materia_periodo_valores;

ALTER TABLE projection_materias
    ADD CONSTRAINT chk_materia_periodo_valores CHECK (
        periodo IS NULL
        OR periodo IN ('1_trimestre', '2_trimestre', '3_trimestre', '1_semestre', '2_semestre')
    );

-- 3. Constraint: apenas matérias superior podem ter período preenchido
ALTER TABLE projection_materias
    DROP CONSTRAINT IF EXISTS chk_materia_periodo_tipo;

ALTER TABLE projection_materias
    ADD CONSTRAINT chk_materia_periodo_tipo CHECK (
        (type = 'superior')
        OR
        (type IN ('fundamental', 'medio') AND periodo IS NULL)
    );

-- 4. Comentário
COMMENT ON COLUMN projection_materias.periodo IS
    'Período letivo da matéria. Obrigatório para ativar matérias do type=superior. '
    'Deve ser um dos períodos definidos no curso vinculado. '
    'NULL para type=fundamental e type=medio.';

-- 5. Índice
CREATE INDEX IF NOT EXISTS idx_materias_periodo
    ON projection_materias(periodo)
    WHERE periodo IS NOT NULL;

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 018 — campo periodo adicionado em projection_materias';
END $$;
