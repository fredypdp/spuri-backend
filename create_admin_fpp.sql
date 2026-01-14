-- ============================================================================
-- Script para Criar Admin FPP Inicial (Bootstrap)
-- Spuri Event Sourcing v2.3.0
-- ============================================================================
-- IMPORTANTE: Execute este script UMA ÚNICA VEZ após criar o schema
-- Este script usa bcrypt IGUAL ao Go (bcrypt.DefaultCost = 10)
-- ============================================================================

-- Habilitar extensão pgcrypto (se ainda não estiver habilitada)
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

DO $$
DECLARE
    v_admin_id UUID;
    v_event_id UUID;
    v_senha_hash TEXT;
    v_payload JSONB;
    v_event_version INT := 1;
    v_ledger_event_id BIGINT;
BEGIN
    -- ============================================
    -- 1. VERIFICAR SE JÁ EXISTE ADMIN FPP
    -- ============================================
    IF EXISTS (SELECT 1 FROM projection_admins WHERE email = 'fredrodrigues795@gmail.com') THEN
        RAISE NOTICE '⚠️  Admin FPP já existe! Abortando script.';
        RAISE NOTICE '   Email: fredrodrigues795@gmail.com';
        RAISE NOTICE '   Use a senha existente ou delete manualmente antes de reexecutar.';
        RETURN;
    END IF;
    
    -- ============================================
    -- 2. GERAR UUID ÚNICO PARA O ADMIN
    -- ============================================
    v_admin_id := uuid_generate_v4();
    v_event_id := uuid_generate_v4();
    
    RAISE NOTICE '';
    RAISE NOTICE '╔═══════════════════════════════════════╗';
    RAISE NOTICE '║  🚀 Criando Admin FPP Inicial...     ║';
    RAISE NOTICE '╚═══════════════════════════════════════╝';
    RAISE NOTICE '';
    RAISE NOTICE '📋 Admin ID: %', v_admin_id;
    RAISE NOTICE '📋 Event ID: %', v_event_id;
    
    -- ============================================
    -- 3. GERAR HASH BCRYPT DA SENHA
    -- ============================================
    -- Senha: "123456789"
    -- Usando bcrypt com cost 10 (IGUAL ao Go: bcrypt.DefaultCost)
    -- O PostgreSQL pgcrypto usa 'bf' (Blowfish) que é bcrypt
    RAISE NOTICE '';
    RAISE NOTICE '🔐 Gerando hash bcrypt da senha...';
    RAISE NOTICE '   Algoritmo: bcrypt (Blowfish)';
    RAISE NOTICE '   Cost Factor: 10 (igual ao Go bcrypt.DefaultCost)';
    RAISE NOTICE '   Senha: 123456789';
    
    v_senha_hash := crypt('123456789', gen_salt('bf', 10));
    
    RAISE NOTICE '   ✅ Hash gerado: %', substring(v_senha_hash, 1, 30) || '...';
    RAISE NOTICE '   📏 Tamanho do hash: % caracteres', length(v_senha_hash);
    
    -- ============================================
    -- 4. VALIDAR HASH IMEDIATAMENTE
    -- ============================================
    RAISE NOTICE '';
    RAISE NOTICE '🧪 Validando hash gerado...';
    
    IF v_senha_hash = crypt('123456789', v_senha_hash) THEN
        RAISE NOTICE '   ✅ Hash validado: Senha correta pode fazer login';
    ELSE
        RAISE EXCEPTION '❌ ERRO CRÍTICO: Hash inválido! Login falhará.';
    END IF;
    
    -- ============================================
    -- 5. CRIAR PAYLOAD DO EVENTO AdminCriado
    -- ============================================
    RAISE NOTICE '';
    RAISE NOTICE '📦 Criando payload do evento...';
    
    v_payload := jsonb_build_object(
        'Nome', 'Fredy',
        'Email', 'fredrodrigues795@gmail.com',
        'SenhaHash', v_senha_hash,
        'Role', 'fpp',
        'CreatedBy', NULL,  -- Primeiro admin, ninguém criou
        'CreatedAt', to_char(CURRENT_TIMESTAMP, 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')
    );
    
    RAISE NOTICE '   ✅ Payload criado';
    
    -- ============================================
    -- 6. INSERIR EVENTO NO GENESIS LEDGER
    -- ============================================
    RAISE NOTICE '';
    RAISE NOTICE '📝 Inserindo evento AdminCriado no ledger...';
    
    INSERT INTO genesis_ledger (
        event_id,
        aggregate_id,
        aggregate_type,
        event_type,
        event_version,
        payload,
        metadata,
        occurred_at
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
        CURRENT_TIMESTAMP
    )
    RETURNING id INTO v_ledger_event_id;
    
    RAISE NOTICE '   ✅ Evento inserido (Ledger ID: %)', v_ledger_event_id;
    
    -- ============================================
    -- 7. CRIAR DIRETAMENTE NA PROJEÇÃO
    -- ============================================
    RAISE NOTICE '';
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
        NULL,
        CURRENT_TIMESTAMP,
        CURRENT_TIMESTAMP,
        v_event_version,
        0,
        v_event_id
    );
    
    RAISE NOTICE '   ✅ Admin criado na projeção';
    
    -- ============================================
    -- 8. ATUALIZAR CHECKPOINT DA PROJEÇÃO ADMINS
    -- ============================================
    RAISE NOTICE '';
    RAISE NOTICE '🔄 Atualizando checkpoint da projeção admins...';
    
    UPDATE projection_checkpoints
    SET 
        last_processed_event_id = v_ledger_event_id,
        last_processed_at = CURRENT_TIMESTAMP,
        events_processed = events_processed + 1
    WHERE projection_name = 'admins';
    
    IF NOT FOUND THEN
        RAISE NOTICE '   ⚠️  Checkpoint não existe, criando...';
        
        INSERT INTO projection_checkpoints (
            projection_name,
            last_processed_event_id,
            last_processed_at,
            events_processed,
            is_rebuilding,
            error_count
        ) VALUES (
            'admins',
            v_ledger_event_id,
            CURRENT_TIMESTAMP,
            1,
            FALSE,
            0
        );
    END IF;
    
    RAISE NOTICE '   ✅ Checkpoint atualizado';
    
    -- ============================================
    -- 9. REGISTRAR AÇÃO NO LOG (se existir)
    -- ============================================
    BEGIN
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
        RAISE NOTICE '';
        RAISE NOTICE '📋 Ação registrada no log';
    EXCEPTION
        WHEN undefined_table THEN
            RAISE NOTICE '';
            RAISE NOTICE '⚠️  Tabela admin_action_log não existe (OK para bootstrap)';
    END;
    
    -- ============================================
    -- 10. VERIFICAÇÃO FINAL
    -- ============================================
    RAISE NOTICE '';
    RAISE NOTICE '╔═══════════════════════════════════════╗';
    RAISE NOTICE '║  ✅ ADMIN FPP CRIADO COM SUCESSO!    ║';
    RAISE NOTICE '╚═══════════════════════════════════════╝';
    RAISE NOTICE '';
    RAISE NOTICE '👤 Nome: Fredy';
    RAISE NOTICE '📧 Email: fredrodrigues795@gmail.com';
    RAISE NOTICE '🔑 Senha: 123456789';
    RAISE NOTICE '👑 Role: fpp (máxima permissão)';
    RAISE NOTICE '🆔 ID: %', v_admin_id;
    RAISE NOTICE '🔐 Hash: %', substring(v_senha_hash, 1, 30) || '...';
    RAISE NOTICE '';
    RAISE NOTICE '⚠️  IMPORTANTE:';
    RAISE NOTICE '   1. Altere a senha após o primeiro login!';
    RAISE NOTICE '   2. Faça login em: POST /admin/login';
    RAISE NOTICE '   3. Use o token JWT retornado nas próximas requisições';
    RAISE NOTICE '';
    RAISE NOTICE '🧪 Teste de Login (curl):';
    RAISE NOTICE '   curl -X POST http://localhost:8080/admin/login \';
    RAISE NOTICE '        -H "Content-Type: application/json" \';
    RAISE NOTICE '        -d ''{"email":"fredrodrigues795@gmail.com","senha":"123456789"}''';
    RAISE NOTICE '';
    
    -- Verificação adicional do hash
    DECLARE
        v_hash_check TEXT;
        v_is_valid BOOLEAN;
    BEGIN
        SELECT senha_hash INTO v_hash_check 
        FROM projection_admins 
        WHERE id = v_admin_id;
        
        v_is_valid := (v_hash_check = crypt('123456789', v_hash_check));
        
        IF v_is_valid THEN
            RAISE NOTICE '✅ Verificação Final: Hash validado com sucesso!';
            RAISE NOTICE '   O login funcionará corretamente.';
        ELSE
            RAISE EXCEPTION '❌ ERRO: Hash não valida! LOGIN FALHARÁ!';
        END IF;
    END;
    
END $$;

-- ============================================
-- VERIFICAÇÃO ADICIONAL (Automática)
-- ============================================

DO $$
DECLARE
    v_admin_record RECORD;
    v_event_record RECORD;
    v_hash_test BOOLEAN;
BEGIN
    RAISE NOTICE '';
    RAISE NOTICE '═══════════════════════════════════════';
    RAISE NOTICE '📊 VERIFICAÇÃO DOS DADOS CRIADOS';
    RAISE NOTICE '═══════════════════════════════════════';
    RAISE NOTICE '';
    
    -- Verificar projeção
    SELECT * INTO v_admin_record
    FROM projection_admins
    WHERE email = 'fredrodrigues795@gmail.com';
    
    IF FOUND THEN
        RAISE NOTICE '✅ Admin encontrado na projeção:';
        RAISE NOTICE '   ID: %', v_admin_record.id;
        RAISE NOTICE '   Nome: %', v_admin_record.nome;
        RAISE NOTICE '   Email: %', v_admin_record.email;
        RAISE NOTICE '   Role: %', v_admin_record.role;
        RAISE NOTICE '   Status: %', v_admin_record.status;
        RAISE NOTICE '   Hash: %', substring(v_admin_record.senha_hash, 1, 30) || '...';
        
        -- Testar hash
        v_hash_test := (v_admin_record.senha_hash = crypt('123456789', v_admin_record.senha_hash));
        
        IF v_hash_test THEN
            RAISE NOTICE '   ✅ Hash: VÁLIDO (login funcionará)';
        ELSE
            RAISE NOTICE '   ❌ Hash: INVÁLIDO (login falhará!)';
        END IF;
    ELSE
        RAISE NOTICE '❌ Admin NÃO encontrado na projeção!';
    END IF;
    
    RAISE NOTICE '';
    
    -- Verificar evento no ledger
    SELECT * INTO v_event_record
    FROM genesis_ledger
    WHERE aggregate_type = 'Admin'
      AND event_type = 'AdminCriado'
    ORDER BY occurred_at DESC
    LIMIT 1;
    
    IF FOUND THEN
        RAISE NOTICE '✅ Evento encontrado no ledger:';
        RAISE NOTICE '   Event ID: %', v_event_record.event_id;
        RAISE NOTICE '   Aggregate ID: %', v_event_record.aggregate_id;
        RAISE NOTICE '   Event Type: %', v_event_record.event_type;
        RAISE NOTICE '   Nome: %', v_event_record.payload->>'Nome';
        RAISE NOTICE '   Email: %', v_event_record.payload->>'Email';
        RAISE NOTICE '   Role: %', v_event_record.payload->>'Role';
    ELSE
        RAISE NOTICE '❌ Evento NÃO encontrado no ledger!';
    END IF;
    
    RAISE NOTICE '';
    RAISE NOTICE '═══════════════════════════════════════';
    RAISE NOTICE '';
END $$;

-- ============================================
-- SCRIPT BASH PARA TESTE DE LOGIN
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
    echo ""
    echo "🧪 Teste de requisição autenticada:"
    echo "   curl -X GET http://localhost:8080/admin/admins \\"
    echo "        -H 'Authorization: Bearer $TOKEN'"
else
    echo ""
    echo "❌ Falha no login. Verifique:"
    echo "   1. API está rodando?"
    echo "   2. Script SQL foi executado?"
    echo "   3. Hash da senha está correto?"
    echo ""
    echo "📋 Logs da API para debug"
fi

*/