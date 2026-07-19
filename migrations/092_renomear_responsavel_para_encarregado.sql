-- Renomeia campos públicos de responsável para encarregado nas projeções reconstruíveis.

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='projection_estudantes' AND column_name='telefone_responsavel')
       AND NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='projection_estudantes' AND column_name='telefone_encarregado') THEN
        ALTER TABLE projection_estudantes RENAME COLUMN telefone_responsavel TO telefone_encarregado;
    END IF;
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='projection_estudantes' AND column_name='telefone_responsavel_verificado')
       AND NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='projection_estudantes' AND column_name='telefone_encarregado_verificado') THEN
        ALTER TABLE projection_estudantes RENAME COLUMN telefone_responsavel_verificado TO telefone_encarregado_verificado;
    END IF;
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='projection_estudantes' AND column_name='bilhete_identidade_responsavel')
       AND NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='projection_estudantes' AND column_name='bilhete_identidade_encarregado') THEN
        ALTER TABLE projection_estudantes RENAME COLUMN bilhete_identidade_responsavel TO bilhete_identidade_encarregado;
    END IF;
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='projection_solicitacoes_matricula' AND column_name='telefone_responsavel')
       AND NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='projection_solicitacoes_matricula' AND column_name='telefone_encarregado') THEN
        ALTER TABLE projection_solicitacoes_matricula RENAME COLUMN telefone_responsavel TO telefone_encarregado;
    END IF;
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='projection_solicitacoes_matricula' AND column_name='bilhete_identidade_responsavel')
       AND NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='projection_solicitacoes_matricula' AND column_name='bilhete_identidade_encarregado') THEN
        ALTER TABLE projection_solicitacoes_matricula RENAME COLUMN bilhete_identidade_responsavel TO bilhete_identidade_encarregado;
    END IF;
END $$;

UPDATE projection_estudantes
SET documentos = (documentos - 'bi_responsavel') || jsonb_build_object('bi_encarregado', documentos->'bi_responsavel')
WHERE documentos ? 'bi_responsavel' AND NOT (documentos ? 'bi_encarregado');

UPDATE projection_estudantes
SET documentos = documentos - 'bi_responsavel'
WHERE documentos ? 'bi_responsavel';

UPDATE projection_solicitacoes_matricula
SET documentos = (documentos - 'bi_responsavel') || jsonb_build_object('bi_encarregado', documentos->'bi_responsavel')
WHERE documentos ? 'bi_responsavel' AND NOT (documentos ? 'bi_encarregado');

UPDATE projection_solicitacoes_matricula
SET documentos = documentos - 'bi_responsavel'
WHERE documentos ? 'bi_responsavel';
