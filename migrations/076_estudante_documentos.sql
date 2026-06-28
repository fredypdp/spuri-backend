ALTER TABLE projection_estudantes
    ADD COLUMN IF NOT EXISTS documentos JSONB NOT NULL DEFAULT '{}'::jsonb;

COMMENT ON COLUMN projection_estudantes.documentos IS
    'Metadados dos documentos PDF enviados no cadastro direto ou na matrícula aprovada do estudante (path, file_url, download_url por campo).';
