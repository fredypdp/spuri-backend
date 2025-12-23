#!/bin/sh

# =============================================================================
# Script de Inicialização do Banco de Dados
# Compatível com: Railway, Heroku, Local
# =============================================================================

set -e

echo "🚀 Spuri Event Sourcing - Inicialização do Banco"
echo "=================================================="

# =============================================================================
# DETECTAR AMBIENTE
# =============================================================================

if [ -n "$DATABASE_URL" ]; then
    echo "📊 Ambiente: Railway/Heroku (DATABASE_URL detectada)"
    DB_CONNECTION="$DATABASE_URL"
else
    echo "📊 Ambiente: Local"
    
    # Construir connection string
    DB_HOST="${GENESISDB_HOST:-localhost}"
    DB_PORT="${GENESISDB_PORT:-5432}"
    DB_USER="${GENESISDB_USER:-genesis}"
    DB_PASS="${GENESISDB_PASSWORD:-genesis123}"
    DB_NAME="${GENESISDB_NAME:-spuri_genesis}"
    
    DB_CONNECTION="postgresql://${DB_USER}:${DB_PASS}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=disable"
    
    echo "   Host: $DB_HOST:$DB_PORT"
    echo "   Database: $DB_NAME"
    echo "   User: $DB_USER"
fi

# =============================================================================
# AGUARDAR BANCO ESTAR PRONTO
# =============================================================================

echo ""
echo "⏳ Aguardando banco de dados ficar disponível..."

MAX_RETRIES=30
RETRY_COUNT=0

while [ $RETRY_COUNT -lt $MAX_RETRIES ]; do
    if psql "$DB_CONNECTION" -c "SELECT 1;" > /dev/null 2>&1; then
        echo "✅ Banco de dados está disponível!"
        break
    fi
    
    RETRY_COUNT=$((RETRY_COUNT + 1))
    echo "   Tentativa $RETRY_COUNT/$MAX_RETRIES..."
    sleep 2
done

if [ $RETRY_COUNT -eq $MAX_RETRIES ]; then
    echo "❌ Timeout: Banco de dados não respondeu após $MAX_RETRIES tentativas"
    exit 1
fi

# =============================================================================
# VERIFICAR SE MIGRATIONS JÁ FORAM APLICADAS
# =============================================================================

echo ""
echo "🔍 Verificando estado das migrations..."

# Verificar se tabela genesis_ledger existe
TABLE_EXISTS=$(psql "$DB_CONNECTION" -t -c "
    SELECT COUNT(*) 
    FROM information_schema.tables 
    WHERE table_schema = 'public' 
    AND table_name = 'genesis_ledger';
" 2>/dev/null | tr -d ' ')

if [ "$TABLE_EXISTS" = "1" ]; then
    echo "✅ Migrations já aplicadas - Tabelas existem"
    
    # Mostrar estatísticas
    echo ""
    echo "📊 Estatísticas do Banco:"
    
    # Contar eventos
    EVENT_COUNT=$(psql "$DB_CONNECTION" -t -c "SELECT COUNT(*) FROM genesis_ledger;" 2>/dev/null | tr -d ' ')
    echo "   Eventos no ledger: $EVENT_COUNT"
    
    # Contar estudantes
    ESTUDANTE_COUNT=$(psql "$DB_CONNECTION" -t -c "SELECT COUNT(*) FROM projection_estudantes;" 2>/dev/null | tr -d ' ')
    echo "   Estudantes: $ESTUDANTE_COUNT"
    
    # Contar academias
    ACADEMIA_COUNT=$(psql "$DB_CONNECTION" -t -c "SELECT COUNT(*) FROM projection_academias;" 2>/dev/null | tr -d ' ')
    echo "   Academias: $ACADEMIA_COUNT"
    
    echo ""
    echo "⏭️  Pulando migrations (já aplicadas)"
else
    echo "🆕 Primeira execução - Aplicando migrations..."
    
    # =============================================================================
    # APLICAR MIGRATIONS
    # =============================================================================
    
    echo ""
    echo "📝 Aplicando Migration 1: Genesis Ledger..."
    
    if [ -f "migrations/genesisdb/001_create_ledger.sql" ]; then
        psql "$DB_CONNECTION" -f migrations/genesisdb/001_create_ledger.sql
        
        if [ $? -eq 0 ]; then
            echo "✅ Migration 1 aplicada com sucesso"
        else
            echo "❌ Erro ao aplicar Migration 1"
            exit 1
        fi
    else
        echo "⚠️  Arquivo migrations/genesisdb/001_create_ledger.sql não encontrado"
        exit 1
    fi
    
    echo ""
    echo "📝 Aplicando Migration 2: Projeções..."
    
    if [ -f "migrations/genesisdb/002_create_projections.sql" ]; then
        psql "$DB_CONNECTION" -f migrations/genesisdb/002_create_projections.sql
        
        if [ $? -eq 0 ]; then
            echo "✅ Migration 2 aplicada com sucesso"
        else
            echo "❌ Erro ao aplicar Migration 2"
            exit 1
        fi
    else
        echo "⚠️  Arquivo migrations/genesisdb/002_create_projections.sql não encontrado"
        exit 1
    fi
    
    echo ""
    echo "🎉 Todas as migrations aplicadas com sucesso!"
fi

# =============================================================================
# VERIFICAÇÃO FINAL
# =============================================================================

echo ""
echo "🔎 Verificação final das tabelas..."

TABLES=$(psql "$DB_CONNECTION" -t -c "
    SELECT table_name 
    FROM information_schema.tables 
    WHERE table_schema = 'public' 
    ORDER BY table_name;
" 2>/dev/null)

echo "📋 Tabelas encontradas:"
echo "$TABLES" | while read -r table; do
    if [ -n "$table" ]; then
        echo "   ✓ $table"
    fi
done

# =============================================================================
# CONCLUÍDO
# =============================================================================

echo ""
echo "=================================================="
echo "✅ Inicialização concluída com sucesso!"
echo "🚀 Sistema pronto para iniciar"
echo "=================================================="
echo ""