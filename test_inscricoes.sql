-- ============================================================================
-- Script de Diagnóstico - Inscrições
-- Execute este script para verificar a estrutura e dados
-- ============================================================================

\echo '=========================================='
\echo '🔍 DIAGNÓSTICO DE INSCRIÇÕES'
\echo '=========================================='

-- 1. Verificar se a tabela existe
\echo ''
\echo '1️⃣ Verificando se a tabela projection_inscricoes existe...'
SELECT 
    CASE 
        WHEN EXISTS (
            SELECT 1 FROM information_schema.tables 
            WHERE table_name = 'projection_inscricoes'
        ) 
        THEN '✅ Tabela projection_inscricoes EXISTE'
        ELSE '❌ Tabela projection_inscricoes NÃO EXISTE'
    END as status;

-- 2. Ver estrutura da tabela
\echo ''
\echo '2️⃣ Estrutura da tabela:'
SELECT 
    column_name,
    data_type,
    character_maximum_length,
    is_nullable,
    column_default
FROM information_schema.columns
WHERE table_name = 'projection_inscricoes'
ORDER BY ordinal_position;

-- 3. Verificar índices
\echo ''
\echo '3️⃣ Índices da tabela:'
SELECT
    indexname,
    indexdef
FROM pg_indexes
WHERE tablename = 'projection_inscricoes';

-- 4. Contar registros por status
\echo ''
\echo '4️⃣ Contagem de inscrições por status:'
SELECT 
    status,
    COUNT(*) as quantidade
FROM projection_inscricoes
GROUP BY status
ORDER BY quantidade DESC;

-- 5. Total geral
\echo ''
\echo '5️⃣ Total geral de inscrições:'
SELECT COUNT(*) as total_inscricoes FROM projection_inscricoes;

-- 6. Exemplos de registros
\echo ''
\echo '6️⃣ Primeiras 5 inscrições (se existirem):'
SELECT 
    id::text as inscricao_id,
    codigo_estudante,
    codigo_academia,
    tipo,
    status,
    TO_CHAR(created_at, 'YYYY-MM-DD HH24:MI:SS') as criado_em
FROM projection_inscricoes
ORDER BY created_at DESC
LIMIT 5;

-- 7. Verificar relacionamentos
\echo ''
\echo '7️⃣ Verificando relacionamentos (inscrições com estudantes e academias):'
SELECT 
    i.codigo_estudante,
    CASE 
        WHEN e.id IS NOT NULL THEN '✅ Estudante existe'
        ELSE '❌ Estudante NÃO existe'
    END as status_estudante,
    i.codigo_academia,
    CASE 
        WHEN a.id IS NOT NULL THEN '✅ Academia existe'
        ELSE '❌ Academia NÃO existe'
    END as status_academia,
    i.status as status_inscricao
FROM projection_inscricoes i
LEFT JOIN projection_estudantes e ON i.estudante_id = e.id
LEFT JOIN projection_academias a ON i.academia_id = a.id
LIMIT 5;

-- 8. Verificar inscrições órfãs (sem estudante ou academia)
\echo ''
\echo '8️⃣ Inscrições órfãs (sem estudante ou academia vinculado):'
SELECT 
    COUNT(*) as total_orfas,
    COUNT(CASE WHEN e.id IS NULL THEN 1 END) as sem_estudante,
    COUNT(CASE WHEN a.id IS NULL THEN 1 END) as sem_academia
FROM projection_inscricoes i
LEFT JOIN projection_estudantes e ON i.estudante_id = e.id
LEFT JOIN projection_academias a ON i.academia_id = a.id
WHERE e.id IS NULL OR a.id IS NULL;

-- 9. Verificar checkpoint da projeção
\echo ''
\echo '9️⃣ Status do checkpoint da projeção de inscrições:'
SELECT 
    projection_name,
    last_processed_event_id,
    TO_CHAR(last_processed_at, 'YYYY-MM-DD HH24:MI:SS') as ultimo_processamento,
    events_processed,
    is_rebuilding,
    error_count,
    last_error
FROM projection_checkpoints
WHERE projection_name = 'inscricoes';

-- 10. Verificar eventos no ledger
\echo ''
\echo '🔟 Eventos de inscrição no ledger:'
SELECT 
    event_type,
    COUNT(*) as quantidade
FROM genesis_ledger
WHERE event_type IN ('EstudanteInscrito', 'InscricaoAprovada', 'InscricaoReprovada')
GROUP BY event_type
ORDER BY quantidade DESC;

-- 11. Distribuição por academia
\echo ''
\echo '1️⃣1️⃣ Top 5 academias com mais inscrições:'
SELECT 
    i.codigo_academia,
    a.nome as nome_academia,
    COUNT(*) as total_inscricoes,
    COUNT(CASE WHEN i.status = 'espera' THEN 1 END) as pendentes,
    COUNT(CASE WHEN i.status = 'aprovado' THEN 1 END) as aprovadas,
    COUNT(CASE WHEN i.status = 'reprovado' THEN 1 END) as reprovadas
FROM projection_inscricoes i
LEFT JOIN projection_academias a ON i.academia_id = a.id
GROUP BY i.codigo_academia, a.nome
ORDER BY total_inscricoes DESC
LIMIT 5;

-- 12. Distribuição temporal
\echo ''
\echo '1️⃣2️⃣ Inscrições nos últimos 7 dias:'
SELECT 
    DATE(created_at) as data,
    COUNT(*) as quantidade
FROM projection_inscricoes
WHERE created_at >= CURRENT_DATE - INTERVAL '7 days'
GROUP BY DATE(created_at)
ORDER BY data DESC;

\echo ''
\echo '=========================================='
\echo '✅ DIAGNÓSTICO CONCLUÍDO'
\echo '=========================================='