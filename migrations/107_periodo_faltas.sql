-- MIGRATION 107 — Adicionar periodo em projection_faltas
--
-- Estratégia de backfill para dados existentes:
-- 1. Faltas de matérias do tipo superior recebem backfill determinístico a partir
--    de projection_materias.periodo, pois esse período é fixo na matéria.
-- 2. O schema atual não possui janelas trimestrais por data para matérias
--    escolares/fundamental/médio; portanto a migração não inventa trimestre
--    para dados históricos.
-- 3. Registros legados que permanecerem sem período ficam explicitamente NULL,
--    com CHECK permitindo NULL apenas como dívida histórica. Registros novos
--    continuam obrigados pela aplicação/domínio a enviar periodo e ficam sujeitos
--    à constraint de valores quando não nulos. Essa escolha evita bloquear deploys
--    em bases produtivas com poucas faltas antigas sem período determinístico.

ALTER TABLE projection_faltas
    ADD COLUMN IF NOT EXISTS periodo VARCHAR(20);

UPDATE projection_faltas f
SET periodo = m.periodo
FROM projection_materias m
WHERE f.materia_disciplinar_id = m.id
  AND m.type = 'superior'
  AND m.periodo IS NOT NULL
  AND f.periodo IS NULL;

DO $$
DECLARE
    faltas_sem_periodo INTEGER;
BEGIN
    SELECT COUNT(*) INTO faltas_sem_periodo
    FROM projection_faltas
    WHERE periodo IS NULL;

    IF faltas_sem_periodo > 0 THEN
        RAISE NOTICE 'projection_faltas possui % registros historicos sem periodo deterministico; mantendo periodo NULL para saneamento operacional posterior', faltas_sem_periodo;
    END IF;
END $$;

ALTER TABLE projection_faltas
    DROP CONSTRAINT IF EXISTS chk_faltas_periodo_valores;

ALTER TABLE projection_faltas
    ADD CONSTRAINT chk_faltas_periodo_valores CHECK (
        periodo IS NULL OR periodo IN ('1_trimestre', '2_trimestre', '3_trimestre', '1_semestre', '2_semestre')
    );

ALTER TABLE projection_faltas
    DROP CONSTRAINT IF EXISTS uq_falta_unica;

ALTER TABLE projection_faltas
    ADD CONSTRAINT uq_falta_unica
        UNIQUE (codigo_estudante, codigo_academia, data, materia_disciplinar_id, periodo);

CREATE INDEX IF NOT EXISTS idx_faltas_periodo_lookup
    ON projection_faltas (codigo_estudante, codigo_academia, periodo);

COMMENT ON COLUMN projection_faltas.periodo IS
    'Período do próprio registro de falta: 1_trimestre, 2_trimestre, 3_trimestre, 1_semestre, 2_semestre. NULL é permitido somente para registros legados sem período determinístico.';

INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at)
VALUES ('faltas', 0, CURRENT_TIMESTAMP)
ON CONFLICT (projection_name) DO NOTHING;
