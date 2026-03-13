-- ============================================
-- MIGRATION 004 - Alterar curso_medio/curso_superior para UUID
-- ============================================

-- 1. Adicionar novas colunas UUID
ALTER TABLE projection_estudantes 
  ADD COLUMN IF NOT EXISTS curso_medio_id UUID,
  ADD COLUMN IF NOT EXISTS curso_superior_id UUID;

-- 2. Adicionar FKs para cursos
ALTER TABLE projection_estudantes
  ADD CONSTRAINT fk_estudante_curso_medio 
    FOREIGN KEY (curso_medio_id) 
    REFERENCES projection_cursos(id) 
    ON DELETE SET NULL;

ALTER TABLE projection_estudantes
  ADD CONSTRAINT fk_estudante_curso_superior 
    FOREIGN KEY (curso_superior_id) 
    REFERENCES projection_cursos(id) 
    ON DELETE SET NULL;

-- 3. Atualizar tabela de inscrições
ALTER TABLE projection_inscricoes 
  DROP COLUMN IF EXISTS curso CASCADE;

ALTER TABLE projection_inscricoes 
  ADD COLUMN IF NOT EXISTS curso_id UUID;

ALTER TABLE projection_inscricoes
  ADD CONSTRAINT fk_inscricao_curso 
    FOREIGN KEY (curso_id) 
    REFERENCES projection_cursos(id) 
    ON DELETE SET NULL;

-- 4. Remover colunas antigas (SOMENTE após migração de dados)
-- CUIDADO: Execute isso DEPOIS de migrar os dados existentes!
-- ALTER TABLE projection_estudantes DROP COLUMN IF EXISTS curso_medio CASCADE;
-- ALTER TABLE projection_estudantes DROP COLUMN IF EXISTS curso_superior CASCADE;

-- 5. Comentários
COMMENT ON COLUMN projection_estudantes.curso_medio_id IS 'FK para curso de ensino médio (UUID)';
COMMENT ON COLUMN projection_estudantes.curso_superior_id IS 'FK para curso de ensino superior (UUID)';
COMMENT ON COLUMN projection_inscricoes.curso_id IS 'FK para curso da inscrição (UUID)';

-- 6. Índices
CREATE INDEX IF NOT EXISTS idx_estudante_curso_medio ON projection_estudantes(curso_medio_id);
CREATE INDEX IF NOT EXISTS idx_estudante_curso_superior ON projection_estudantes(curso_superior_id);
CREATE INDEX IF NOT EXISTS idx_inscricao_curso ON projection_inscricoes(curso_id);

-- 7. View auxiliar com nomes dos cursos
CREATE OR REPLACE VIEW v_estudantes_com_cursos AS
SELECT 
    e.id,
    e.nome,
    e.codigo_estudante,
    e.email,
    e.codigo_academia,
    e.status,
    e.status_escolar,
    e.status_superior,
    e.ano_escolar,
    e.ano_superior,
    cm.id as curso_medio_id,
    cm.nome as curso_medio_nome,
    cs.id as curso_superior_id,
    cs.nome as curso_superior_nome,
    e.created_at,
    e.updated_at
FROM projection_estudantes e
LEFT JOIN projection_cursos cm ON e.curso_medio_id = cm.id
LEFT JOIN projection_cursos cs ON e.curso_superior_id = cs.id;

-- 8. View de inscrições com cursos
CREATE OR REPLACE VIEW v_inscricoes_com_cursos AS
SELECT 
    i.id,
    i.estudante_id,
    i.codigo_estudante,
    e.nome as estudante_nome,
    i.academia_id,
    i.codigo_academia,
    a.nome as academia_nome,
    i.tipo,
    i.ano_inscricao,
    i.curso_id,
    c.nome as curso_nome,
    c.type as curso_type,
    i.status,
    i.status_usado,
    i.created_at,
    i.updated_at
FROM projection_inscricoes i
LEFT JOIN projection_estudantes e ON i.estudante_id = e.id
LEFT JOIN projection_academias a ON i.academia_id = a.id
LEFT JOIN projection_cursos c ON i.curso_id = c.id;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 004 CONCLUÍDA - Cursos agora são UUID com FKs';
    RAISE NOTICE '⚠️  LEMBRE-SE: Migre os dados existentes antes de dropar as colunas antigas';
END $$;