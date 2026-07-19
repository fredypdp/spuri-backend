-- MIGRATION 090 - Normalizar documentos acadêmicos de estudante por escopo
-- Preserva documentos legados e reindexa declarações/certificados com chave composta
-- nivel.ano_academico.tipo quando o escopo pode ser inferido. Quando não pode,
-- mantém os metadados em escopo_desconhecido.<campo> sem descartar arquivo.

CREATE OR REPLACE FUNCTION spuri_normalizar_documentos_estudante_090(docs jsonb)
RETURNS jsonb
LANGUAGE plpgsql
AS $$
DECLARE
    out jsonb := '{}'::jsonb;
    item record;
    campo text;
    doc jsonb;
    ano text;
    nivel text;
    tipo text;
    chave text;
BEGIN
    IF docs IS NULL OR jsonb_typeof(docs) <> 'object' THEN
        RETURN '{}'::jsonb;
    END IF;

    FOR item IN SELECT key, value FROM jsonb_each(docs) LOOP
        campo := item.key;
        doc := CASE WHEN jsonb_typeof(item.value) = 'string' THEN jsonb_build_object('path', item.value) ELSE item.value END;
        ano := COALESCE(NULLIF(doc->>'ano_academico', ''), '');
        tipo := campo;
        nivel := 'identificacao';

        IF campo = 'declaracao' THEN
            IF ano <> '' THEN
                tipo := 'declaracao_' || ano;
            END IF;
        END IF;

        IF tipo LIKE 'declaracao_%_ano_fundamental' THEN
            nivel := 'fundamental';
            ano := replace(tipo, 'declaracao_', '');
        ELSIF tipo LIKE 'declaracao_%_ano_medio' THEN
            nivel := 'medio';
            ano := replace(tipo, 'declaracao_', '');
        ELSIF tipo LIKE 'declaracao_%_ano_superior' THEN
            nivel := 'superior';
            ano := replace(tipo, 'declaracao_', '');
        ELSIF tipo = 'certificado_6_ano_fundamental' THEN
            nivel := 'fundamental'; ano := '6_ano_fundamental';
        ELSIF tipo = 'certificado_9_ano_fundamental' THEN
            nivel := 'fundamental'; ano := '9_ano_fundamental';
        ELSIF tipo = 'certificado_ensino_medio' THEN
            nivel := 'medio'; ano := '3_ano_medio';
        ELSIF campo NOT IN ('bi_estudante', 'bi_encarregado', 'cedula_estudante') THEN
            nivel := 'escopo_desconhecido';
        END IF;

        doc := doc || jsonb_build_object('tipo', tipo, 'nivel', nivel, 'ano_academico', NULLIF(ano, ''));
        IF NOT (doc ? 'documento_id') THEN
            doc := doc || jsonb_build_object('documento_id', md5(campo || ':' || COALESCE(doc->>'path', '') || ':' || COALESCE(doc->>'file_url', '')));
        END IF;
        IF NOT (doc ? 'versao') THEN
            doc := doc || jsonb_build_object('versao', 1);
        END IF;

        IF nivel IN ('fundamental', 'medio', 'superior') AND ano <> '' THEN
            chave := nivel || '.' || ano || '.' || tipo;
        ELSIF nivel = 'identificacao' THEN
            chave := tipo;
        ELSE
            chave := 'escopo_desconhecido.' || tipo;
        END IF;
        out := out || jsonb_build_object(chave, doc);
    END LOOP;
    RETURN out;
END;
$$;

UPDATE projection_estudantes
SET documentos = spuri_normalizar_documentos_estudante_090(documentos)
WHERE documentos IS NOT NULL AND documentos <> '{}'::jsonb;

COMMENT ON COLUMN projection_estudantes.documentos IS
    'Metadados normalizados dos documentos do estudante por escopo acadêmico: identificacao.<tipo> ou nivel.ano_academico.tipo, com documento_id, tipo, nivel, ano_academico, versao, path, file_url e download_url.';

DROP FUNCTION spuri_normalizar_documentos_estudante_090(jsonb);
