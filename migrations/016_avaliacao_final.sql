-- ============================================================
-- MIGRATION 016 - projection_avaliacao_final
-- Substitui projection_aprovacao_ano + projection_reprovacoes
-- ============================================================

BEGIN;

CREATE TABLE IF NOT EXISTS projection_avaliacao_final (
    id                    UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    event_id              UUID        NOT NULL,

    codigo_estudante      VARCHAR(7)  NOT NULL,
    codigo_academia       VARCHAR(50) NOT NULL,

    ano_lectivo           VARCHAR(20) NOT NULL,
    tipo_ensino           VARCHAR(20) NOT NULL
                              CHECK (tipo_ensino IN ('fundamental', 'medio', 'superior')),

    ano_academico_atual   VARCHAR(50) NOT NULL,
    proximo_ano_academico VARCHAR(50),          -- NULL = último ano ou reprovado

    aprovado              BOOLEAN     NOT NULL DEFAULT FALSE,
    observacao            TEXT,

    registered_at         TIMESTAMP   NOT NULL DEFAULT CURRENT_TIMESTAMP,
    version               INTEGER     NOT NULL,

    -- Uma avaliação por estudante/academia/ano_lectivo/tipo_ensino
    UNIQUE (codigo_estudante, codigo_academia, ano_lectivo, tipo_ensino),

    FOREIGN KEY (codigo_academia)
        REFERENCES projection_academias(codigo_academia)
        ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_avf_estudante  ON projection_avaliacao_final(codigo_estudante);
CREATE INDEX IF NOT EXISTS idx_avf_academia   ON projection_avaliacao_final(codigo_academia);
CREATE INDEX IF NOT EXISTS idx_avf_ano        ON projection_avaliacao_final(ano_lectivo);
CREATE INDEX IF NOT EXISTS idx_avf_tipo       ON projection_avaliacao_final(tipo_ensino);
CREATE INDEX IF NOT EXISTS idx_avf_aprovado   ON projection_avaliacao_final(aprovado);

INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at)
VALUES ('avaliacao_final', 0, CURRENT_TIMESTAMP)
ON CONFLICT (projection_name) DO NOTHING;

COMMENT ON TABLE  projection_avaliacao_final IS
    'Avaliações finais de ano acadêmico — substitui projection_aprovacao_ano e projection_reprovacoes';
COMMENT ON COLUMN projection_avaliacao_final.aprovado IS
    'TRUE = aprovado (avança ou finaliza ciclo); FALSE = reprovado (só registra)';
COMMENT ON COLUMN projection_avaliacao_final.proximo_ano_academico IS
    'Próximo nível. NULL se último ano do ciclo ou se reprovado.';
COMMENT ON COLUMN projection_avaliacao_final.observacao IS
    'Justificativa obrigatória para forçar aprovação com notas ausentes.';

COMMIT;