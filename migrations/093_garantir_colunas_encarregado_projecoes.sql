-- Garante as colunas públicas de encarregado em bases legadas onde migrations
-- antigas criaram a projeção sem os campos atuais e a migration de rename já foi aplicada.

ALTER TABLE projection_solicitacoes_matricula
    ADD COLUMN IF NOT EXISTS telefone_encarregado VARCHAR(20),
    ADD COLUMN IF NOT EXISTS bilhete_identidade_encarregado VARCHAR(50);

ALTER TABLE projection_estudantes
    ADD COLUMN IF NOT EXISTS telefone_encarregado VARCHAR(20),
    ADD COLUMN IF NOT EXISTS telefone_encarregado_verificado BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS bilhete_identidade_encarregado VARCHAR(50);
