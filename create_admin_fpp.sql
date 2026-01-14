-- ============================================================================
-- Script para Criar Admin FPP Inicial (Bootstrap)
-- Spuri Event Sourcing v2.2.0
-- ============================================================================
-- IMPORTANTE: Execute este script UMA ÚNICA VEZ após criar o schema
-- ============================================================================

DO $$
DECLARE
    v_admin_id UUID;
    v_event_id UUID;
    v_senha_hash TEXT;
    v_payload JSONB;
    v_event_version INT := 1;
BEGIN
    -- ============================================
    -- 1. GERAR UUID ÚNICO PARA O ADMIN
    -- ============================================
    v_admin_id := uuid_generate_v4();
    v_event_id := uuid_generate_v4();
    
    RAISE NOTICE '🔐 Criando Admin FPP Inicial...';
    RAISE NOTICE '   Admin ID: %', v_admin_id;
    
    -- ============================================
    -- 2. GERAR HASH BCRYPT DA SENHA
    -- ============================================
    -- Senha: "123456789"
    -- Usando bcrypt com cost 10 (padrão Go bcrypt.DefaultCost)
    v_senha_hash := crypt('123456789', gen_salt('bf', 10));
    
    RAISE NOTICE '   Senha hash gerado: %', substring(v_senha_hash, 1, 20) || '...';
    
    -- ============================================
    -- 3. CRIAR PAYLOAD DO EVENTO AdminCriado
    -- ============================================
    v_payload := jsonb_build_object(
        'Nome', 'Fredy',
        'Email', 'fredrodrigues795@gmail.com',
        'SenhaHash', v_senha_hash,
        'Role', 'fpp',
        'CreatedBy', NULL,  -- Primeiro admin, ninguém criou
        'CreatedAt', CURRENT_TIMESTAMP
    );
    
    -- ============================================
    -- 4. INSERIR EVENTO NO GENESIS LEDGER
    -- ============================================
    RAISE NOTICE '📝 Inserindo evento AdminCriado no ledger...';
    
    INSERT INTO genesis_ledger (
        event_id,
        aggregate_id,
        aggregate_type,
        event_type,
        event_version,
        payload,
        metadata,
        occurred_at,
        recorded_at
    ) VALUES (
        v_event_id,
        v_admin_id,
        'Admin',
        'AdminCriado',
        v_event_version,
        v_payload,
        jsonb_build_object(
            'bootstrap', true,
            'created_via', 'sql_script',
            'timestamp', extract(epoch from CURRENT_TIMESTAMP)
        ),
        CURRENT_TIMESTAMP,
        CURRENT_TIMESTAMP
    );
    
    RAISE NOTICE '   ✅ Evento inserido no ledger (Event ID: %)', v_event_id;
    
    -- ============================================
    -- 5. CRIAR DIRETAMENTE NA PROJEÇÃO
    -- ============================================
    -- Como é o bootstrap e as projeções ainda não estão rodando,
    -- criamos diretamente na projeção
    RAISE NOTICE '📊 Criando entrada na projection_admins...';
    
    INSERT INTO projection_admins (
        id,
        nome,
        email,
        senha_hash,
        role,
        status,
        created_by,
        created_at,
        updated_at,
        version,
        total_acoes_realizadas,
        last_event_id
    ) VALUES (
        v_admin_id,
        'Fredy',
        'fredrodrigues795@gmail.com',
        v_senha_hash,
        'fpp',
        'ativo',
        NULL,  -- Primeiro admin
        CURRENT_TIMESTAMP,
        CURRENT_TIMESTAMP,
        v_event_version,
        0,
        v_event_id
    );
    
    RAISE NOTICE '   ✅ Admin criado na projeção';
    
    -- ============================================
    -- 6. ATUALIZAR CHECKPOINT DA PROJEÇÃO ADMINS
    -- ============================================
    RAISE NOTICE '🔄 Atualizando checkpoint da projeção admins...';
    
    UPDATE projection_checkpoints
    SET 
        last_processed_event_id = (SELECT id FROM genesis_ledger WHERE event_id = v_event_id),
        last_processed_at = CURRENT_TIMESTAMP,
        events_processed = events_processed + 1
    WHERE projection_name = 'admins';
    
    RAISE NOTICE '   ✅ Checkpoint atualizado';
    
    -- ============================================
    -- 7. REGISTRAR AÇÃO NO LOG
    -- ============================================
    INSERT INTO admin_action_log (
        admin_id,
        acao,
        detalhes,
        target_type,
        target_id,
        performed_at
    ) VALUES (
        v_admin_id,
        'admin_bootstrap_criado',
        jsonb_build_object(
            'method', 'sql_script',
            'role', 'fpp',
            'is_first_admin', true
        ),
        'Admin',
        v_admin_id,
        CURRENT_TIMESTAMP
    );
    
    -- ============================================
    -- 8. VERIFICAÇÃO FINAL
    -- ============================================
    RAISE NOTICE '';
    RAISE NOTICE '╔════════════════════════════════════════╗';
    RAISE NOTICE '║  ✅ ADMIN FPP CRIADO COM SUCESSO!     ║';
    RAISE NOTICE '╚════════════════════════════════════════╝';
    RAISE NOTICE '';
    RAISE NOTICE '📧 Email: fredrodrigues795@gmail.com';
    RAISE NOTICE '🔑 Senha: 123456789';
    RAISE NOTICE '👤 Role: fpp (máxima permissão)';
    RAISE NOTICE '🆔 ID: %', v_admin_id;
    RAISE NOTICE '';
    RAISE NOTICE '⚠️  IMPORTANTE:';
    RAISE NOTICE '   1. Altere a senha após o primeiro login!';
    RAISE NOTICE '   2. Faça login em: POST /admin/login';
    RAISE NOTICE '   3. Use o token JWT retornado nas próximas requisições';
    RAISE NOTICE '';
    RAISE NOTICE '🧪 Teste de Login:';
    RAISE NOTICE '   curl -X POST http://localhost:8080/admin/login \';
    RAISE NOTICE '        -H "Content-Type: application/json" \';
    RAISE NOTICE '        -d ''{"email":"fredrodrigues795@gmail.com","senha":"123456789"}''';
    RAISE NOTICE '';
    
    -- Verificar se o hash foi criado corretamente
    DECLARE
        v_hash_check TEXT;
        v_is_valid BOOLEAN;
    BEGIN
        SELECT senha_hash INTO v_hash_check 
        FROM projection_admins 
        WHERE id = v_admin_id;
        
        -- Testar se o hash valida a senha correta
        v_is_valid := (v_hash_check = crypt('123456789', v_hash_check));
        
        IF v_is_valid THEN
            RAISE NOTICE '✅ Hash validado: Senha pode ser usada para login';
        ELSE
            RAISE WARNING '❌ Erro na validação do hash! Login pode falhar';
        END IF;
    END;
    
END $$;

-- ============================================
-- VERIFICAÇÃO ADICIONAL (OPCIONAL)
-- ============================================

-- Mostrar dados do admin criado
SELECT 
    '🔍 VERIFICAÇÃO DO ADMIN CRIADO' as info;

SELECT 
    id,
    nome,
    email,
    role,
    status,
    created_at,
    substring(senha_hash, 1, 20) || '...' as senha_hash_preview
FROM projection_admins
WHERE email = 'fredrodrigues795@gmail.com';

-- Mostrar evento no ledger
SELECT 
    '📜 EVENTO NO LEDGER' as info;

SELECT 
    event_id,
    aggregate_id,
    event_type,
    event_version,
    occurred_at,
    payload->>'Nome' as nome,
    payload->>'Email' as email,
    payload->>'Role' as role
FROM genesis_ledger
WHERE aggregate_type = 'Admin'
  AND event_type = 'AdminCriado'
ORDER BY occurred_at DESC
LIMIT 1;

-- ============================================
-- SCRIPT DE TESTE DE LOGIN (Bash)
-- ============================================
/*

Copie e cole no terminal para testar o login:

#!/bin/bash

echo "🧪 Testando login do Admin FPP..."
echo ""

RESPONSE=$(curl -s -X POST http://localhost:8080/admin/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "fredrodrigues795@gmail.com",
    "senha": "123456789"
  }')

echo "📨 Resposta da API:"
echo "$RESPONSE" | jq .

TOKEN=$(echo "$RESPONSE" | jq -r '.token')

if [ "$TOKEN" != "null" ] && [ ! -z "$TOKEN" ]; then
    echo ""
    echo "✅ Login bem-sucedido!"
    echo "🎫 Token JWT: ${TOKEN:0:50}..."
    echo ""
    echo "📝 Use este token nas próximas requisições:"
    echo "   Authorization: Bearer $TOKEN"
else
    echo ""
    echo "❌ Falha no login. Verifique os logs da API."
fi

*/