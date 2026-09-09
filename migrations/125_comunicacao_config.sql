BEGIN;

CREATE TABLE IF NOT EXISTS projection_comunicacao_config (
    id INTEGER PRIMARY KEY DEFAULT 1,
    provedor_padrao VARCHAR(10),
    atualizado_por UUID,
    atualizado_em TIMESTAMPTZ,
    CONSTRAINT chk_comunicacao_config_singleton CHECK (id = 1),
    CONSTRAINT chk_comunicacao_config_provedor_padrao CHECK (provedor_padrao IS NULL OR provedor_padrao IN ('GOSMS', 'ZIETT'))
);

INSERT INTO projection_comunicacao_config (id, provedor_padrao, atualizado_por, atualizado_em)
VALUES (1, NULL, NULL, NULL)
ON CONFLICT (id) DO NOTHING;

COMMIT;
