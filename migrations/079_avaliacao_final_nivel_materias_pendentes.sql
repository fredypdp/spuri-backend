-- ============================================================================
-- MIGRATION 079 — Avaliação final por nível, matérias e pendências
-- ============================================================================

BEGIN;

ALTER TABLE projection_regras_avaliacao_final
    RENAME COLUMN tipo_ensino TO nivel;

ALTER TABLE projection_regras_avaliacao_final
    DROP CONSTRAINT IF EXISTS projection_regras_avaliacao_final_tipo_ensino_check;

ALTER TABLE projection_regras_avaliacao_final
    ADD CONSTRAINT projection_regras_avaliacao_final_nivel_check
    CHECK (nivel IN ('fundamental','medio','superior'));

ALTER TABLE projection_regras_avaliacao_final
    ADD COLUMN IF NOT EXISTS materias_chave JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS materias_aplicaveis JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS limite_materias_pendentes INTEGER,
    ADD COLUMN IF NOT EXISTS resultados_materias_snapshot JSONB NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE projection_regras_avaliacao_final
    ADD CONSTRAINT chk_regras_avf_limite_pendencias
    CHECK (
        (nivel = 'fundamental' AND limite_materias_pendentes IS NULL)
        OR (nivel IN ('medio','superior') AND limite_materias_pendentes IS NOT NULL AND limite_materias_pendentes >= 0)
    );

ALTER TABLE projection_avaliacao_final
    ADD COLUMN IF NOT EXISTS resultados_materias JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS aprovado_com_pendencia BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS pendencias_geradas JSONB NOT NULL DEFAULT '[]'::jsonb;

CREATE TABLE IF NOT EXISTS projection_materias_pendentes (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    codigo_estudante VARCHAR(7) NOT NULL,
    materia_id UUID NOT NULL,
    codigo_academia VARCHAR(50) NOT NULL REFERENCES projection_academias(codigo_academia) ON DELETE CASCADE,
    curso_id UUID NOT NULL,
    nivel VARCHAR(20) NOT NULL CHECK (nivel IN ('medio','superior')),
    ano_escolar_medio VARCHAR(50),
    periodo_superior VARCHAR(50),
    ano_lectivo VARCHAR(20) NOT NULL,
    regra_avaliacao_final_id UUID,
    avaliacao_final_event_id UUID,
    pendente BOOLEAN NOT NULL DEFAULT TRUE,
    dados_origem JSONB NOT NULL DEFAULT '{}'::jsonb,
    criada_por_event_id UUID,
    baixada_por_event_id UUID,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (
        (nivel = 'medio' AND ano_escolar_medio IS NOT NULL AND periodo_superior IS NULL)
        OR (nivel = 'superior' AND periodo_superior IS NOT NULL AND ano_escolar_medio IS NULL)
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_materia_pendente_aberta_escopo
    ON projection_materias_pendentes (
        codigo_estudante,
        codigo_academia,
        materia_id,
        curso_id,
        nivel,
        ano_lectivo,
        COALESCE(ano_escolar_medio, ''),
        COALESCE(periodo_superior, '')
    )
    WHERE pendente = TRUE;

CREATE INDEX IF NOT EXISTS idx_materias_pendentes_consulta
    ON projection_materias_pendentes (codigo_academia, codigo_estudante, nivel, pendente, curso_id);

COMMIT;
