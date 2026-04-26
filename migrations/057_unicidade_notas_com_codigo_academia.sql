-- MIGRATION 057 — Incluir codigo_academia na unicidade de notas
--
-- Motivação:
--   Um estudante pode mudar de academia ao longo do tempo e voltar a registrar
--   notas para a mesma disciplina/período/ano letivo. Sem codigo_academia na
--   chave de unicidade, registros legítimos de academias diferentes entram em
--   conflito.
--
-- Resultado:
--   A constraint uq_nota_unica passa a ser:
--   UNIQUE (codigo_estudante, codigo_academia, ano_lectivo, periodo, materia_disciplinar_id, tipo, categoria)

DO $$
BEGIN
    ALTER TABLE projection_notas
        DROP CONSTRAINT IF EXISTS uq_nota_unica;

    ALTER TABLE projection_notas
        ADD CONSTRAINT uq_nota_unica
            UNIQUE (
                codigo_estudante,
                codigo_academia,
                ano_lectivo,
                periodo,
                materia_disciplinar_id,
                tipo,
                categoria
            );

    RAISE NOTICE '✅ uq_nota_unica atualizada com codigo_academia em projection_notas';
END $$;

DROP INDEX IF EXISTS idx_notas_unica_lookup;
CREATE INDEX IF NOT EXISTS idx_notas_unica_lookup
    ON projection_notas (
        codigo_estudante,
        codigo_academia,
        ano_lectivo,
        periodo,
        materia_disciplinar_id,
        tipo,
        categoria
    )
    WHERE deleted_at IS NULL;

INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at)
VALUES ('notas', 0, CURRENT_TIMESTAMP)
ON CONFLICT (projection_name) DO NOTHING;

DO $$
BEGIN
    RAISE NOTICE '✅ MIGRATION 057 aplicada';
    RAISE NOTICE '   → uq_nota_unica inclui codigo_academia';
    RAISE NOTICE '   → idx_notas_unica_lookup realinhado';
END $$;
