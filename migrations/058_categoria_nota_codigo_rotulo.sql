-- MIGRATION 058 - Categoria de nota: separar codigo e nome (rótulo)

ALTER TABLE projection_categorias_nota
    RENAME COLUMN nome TO codigo;

ALTER TABLE projection_categorias_nota
    ADD COLUMN nome VARCHAR(255) NOT NULL DEFAULT '';

UPDATE projection_categorias_nota
SET nome = CASE codigo
    WHEN 'nota_escola' THEN 'Nota da escola'
    WHEN 'nota_professor' THEN 'Nota do professor'
    WHEN 'nota_pp1' THEN 'Prova Parcelar 1'
    WHEN 'nota_pp2' THEN 'Prova Parcelar 2'
    WHEN 'nota_exame' THEN 'Exame Final'
    ELSE REPLACE(INITCAP(REPLACE(codigo, '_', ' ')), 'Nota ', 'Nota ')
END
WHERE nome = '';

ALTER TABLE projection_categorias_nota
    DROP CONSTRAINT IF EXISTS projection_categorias_nota_codigo_academia_nome_key;

ALTER TABLE projection_categorias_nota
    ADD CONSTRAINT projection_categorias_nota_codigo_academia_codigo_key
    UNIQUE (codigo_academia, codigo);

ALTER TABLE projection_categorias_nota
    ADD CONSTRAINT chk_categoria_nota_codigo_sem_espacos
    CHECK (codigo !~ '\\s');
