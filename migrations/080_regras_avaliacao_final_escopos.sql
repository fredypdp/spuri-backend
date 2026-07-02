-- ============================================================
-- MIGRATION 080 - Escopos normalizados de regras de avaliação final
-- Garante unicidade concorrente por ano fundamental e por curso/ano médio.
-- ============================================================

BEGIN;

CREATE TABLE IF NOT EXISTS projection_regras_avaliacao_final_escopos (
    regra_id UUID NOT NULL REFERENCES projection_regras_avaliacao_final(id) ON DELETE CASCADE,
    codigo_academia VARCHAR(50) NOT NULL,
    nivel VARCHAR(20) NOT NULL CHECK (nivel IN ('fundamental', 'medio', 'superior')),
    type VARCHAR(100) NOT NULL,
    curso_id UUID NULL,
    ano_academico VARCHAR(50) NOT NULL,
    status VARCHAR(20) NOT NULL,
    PRIMARY KEY (regra_id, curso_id, ano_academico),
    CHECK (
        (nivel = 'fundamental' AND curso_id IS NULL) OR
        (nivel = 'medio' AND curso_id IS NOT NULL)
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_regra_avf_escopo_ativo
    ON projection_regras_avaliacao_final_escopos (
        codigo_academia,
        nivel,
        type,
        COALESCE(curso_id, '00000000-0000-0000-0000-000000000000'::uuid),
        ano_academico
    )
    WHERE status = 'ativo';

CREATE OR REPLACE FUNCTION sync_projection_regras_avaliacao_final_escopos()
RETURNS trigger AS $$
DECLARE
    item TEXT;
    curso_txt TEXT;
    ano_txt TEXT;
BEGIN
    DELETE FROM projection_regras_avaliacao_final_escopos WHERE regra_id = NEW.id;

    IF NEW.nivel = 'fundamental' THEN
        FOR item IN SELECT jsonb_array_elements_text(COALESCE(NEW.anos_academicos, '[]'::jsonb)) LOOP
            INSERT INTO projection_regras_avaliacao_final_escopos (regra_id, codigo_academia, nivel, type, curso_id, ano_academico, status)
            VALUES (NEW.id, NEW.codigo_academia, NEW.nivel, NEW.type, NULL, item, NEW.status);
        END LOOP;
    ELSIF NEW.nivel = 'medio' THEN
        FOR item IN SELECT jsonb_array_elements_text(COALESCE(NEW.anos_academicos, '[]'::jsonb)) LOOP
            curso_txt := split_part(item, '|', 1);
            ano_txt := split_part(item, '|', 2);
            IF curso_txt <> '' AND ano_txt <> '' THEN
                INSERT INTO projection_regras_avaliacao_final_escopos (regra_id, codigo_academia, nivel, type, curso_id, ano_academico, status)
                VALUES (NEW.id, NEW.codigo_academia, NEW.nivel, NEW.type, curso_txt::uuid, ano_txt, NEW.status);
            END IF;
        END LOOP;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_sync_projection_regras_avaliacao_final_escopos ON projection_regras_avaliacao_final;
CREATE TRIGGER trg_sync_projection_regras_avaliacao_final_escopos
AFTER INSERT OR UPDATE OF codigo_academia, nivel, type, anos_academicos, status
ON projection_regras_avaliacao_final
FOR EACH ROW
EXECUTE FUNCTION sync_projection_regras_avaliacao_final_escopos();

INSERT INTO projection_regras_avaliacao_final_escopos (regra_id, codigo_academia, nivel, type, curso_id, ano_academico, status)
SELECT r.id, r.codigo_academia, r.nivel, r.type, NULL, item, r.status
FROM projection_regras_avaliacao_final r
CROSS JOIN LATERAL jsonb_array_elements_text(COALESCE(r.anos_academicos, '[]'::jsonb)) item
WHERE r.nivel = 'fundamental'
ON CONFLICT DO NOTHING;

INSERT INTO projection_regras_avaliacao_final_escopos (regra_id, codigo_academia, nivel, type, curso_id, ano_academico, status)
SELECT r.id, r.codigo_academia, r.nivel, r.type, split_part(item, '|', 1)::uuid, split_part(item, '|', 2), r.status
FROM projection_regras_avaliacao_final r
CROSS JOIN LATERAL jsonb_array_elements_text(COALESCE(r.anos_academicos, '[]'::jsonb)) item
WHERE r.nivel = 'medio'
  AND split_part(item, '|', 1) <> ''
  AND split_part(item, '|', 2) <> ''
ON CONFLICT DO NOTHING;

COMMIT;
