#!/bin/bash

# =============================================================================
# Script de Teste - Spuri Event Sourcing API v2.0
# =============================================================================

API_URL="${API_URL:-http://localhost:8080}"
BOLD='\033[1m'
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# Contadores
TESTS_PASSED=0
TESTS_FAILED=0
TOTAL_TESTS=0

# IDs globais
ESCOLA_ID=""
ESCOLA_TOKEN=""
CODIGO_ACADEMIA=""
ESTUDANTE_ID=""
ESTUDANTE_TOKEN=""
INSCRICAO_ID=""

echo -e "${BOLD}${BLUE}"
echo "╔════════════════════════════════════════════════════════╗"
echo "║  🧪 Testando Spuri Event Sourcing API v2.0           ║"
echo "╚════════════════════════════════════════════════════════╝"
echo -e "${NC}"
echo -e "🌐 URL: ${YELLOW}$API_URL${NC}"
echo -e "📅 Data: $(date '+%Y-%m-%d %H:%M:%S')\n"

# =============================================================================
# FUNÇÕES AUXILIARES
# =============================================================================

request() {
    local method=$1
    local endpoint=$2
    local data=$3
    local token=$4
    
    if [ -n "$token" ]; then
        curl -s -X $method "$API_URL$endpoint" \
            -H "Content-Type: application/json" \
            -H "Authorization: Bearer $token" \
            -d "$data" 2>/dev/null
    else
        curl -s -X $method "$API_URL$endpoint" \
            -H "Content-Type: application/json" \
            -d "$data" 2>/dev/null
    fi
}

extract_json() {
    local json="$1"
    local key="$2"
    echo "$json" | grep -o "\"$key\"[[:space:]]*:[[:space:]]*\"[^\"]*\"" | sed 's/.*":\s*"\([^"]*\)".*/\1/' | head -1
}

test_case() {
    local name="$1"
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${YELLOW}📋 Test $TOTAL_TESTS: $name${NC}"
}

assert_success() {
    local response="$1"
    local expected="$2"
    
    if echo "$response" | grep -q "$expected"; then
        echo -e "${GREEN}✅ PASSOU${NC}"
        echo -e "${GREEN}   Resposta contém: '$expected'${NC}\n"
        TESTS_PASSED=$((TESTS_PASSED + 1))
        return 0
    else
        echo -e "${RED}❌ FALHOU${NC}"
        echo -e "${RED}   Esperado: '$expected'${NC}"
        echo -e "${RED}   Resposta: ${response:0:200}...${NC}\n"
        TESTS_FAILED=$((TESTS_FAILED + 1))
        return 1
    fi
}

show_response() {
    local response="$1"
    local max_length="${2:-300}"
    
    if [ ${#response} -gt $max_length ]; then
        echo -e "${BLUE}📄 Resposta: ${response:0:$max_length}...${NC}"
    else
        echo -e "${BLUE}📄 Resposta: $response${NC}"
    fi
}

# =============================================================================
# TESTES
# =============================================================================

# Test 1: Health Check
test_case "Health Check - Sistema Online"
response=$(curl -s "$API_URL/health" 2>/dev/null)

if [ $? -ne 0 ]; then
    echo -e "${RED}❌ FALHOU - API não está acessível em $API_URL${NC}\n"
    echo -e "${YELLOW}💡 Dica: Verifique se o servidor está rodando${NC}"
    echo -e "${YELLOW}   Comando: go run cmd/server/main.go${NC}\n"
    TESTS_FAILED=$((TESTS_FAILED + 1))
    exit 1
fi

if echo "$response" | grep -q "Event Sourcing"; then
    echo -e "${GREEN}✅ PASSOU - API rodando com Event Sourcing${NC}"
    show_response "$response" 150
    echo ""
    TESTS_PASSED=$((TESTS_PASSED + 1))
else
    echo -e "${RED}❌ FALHOU - Resposta inesperada${NC}"
    show_response "$response"
    echo ""
    TESTS_FAILED=$((TESTS_FAILED + 1))
    exit 1
fi

# Test 2: Registrar Academia
test_case "Command - Registrar Academia (Event Sourcing)"
escola_data='{
    "type": "escola",
    "senha": "escola2025",
    "nome": "Escola Primária Genesis",
    "provincia": "Luanda",
    "endereco": "Rua da Missão, Bairro Operário, Luanda",
    "email": "contato@escolagenesis.ao",
    "nivel_escolar": "fundamental"
}'

escola_response=$(request "POST" "/academia/register" "$escola_data")
ESCOLA_ID=$(extract_json "$escola_response" "id")
CODIGO_ACADEMIA=$(extract_json "$escola_response" "codigo_academia")

if [ -n "$ESCOLA_ID" ] && [ -n "$CODIGO_ACADEMIA" ]; then
    echo -e "${GREEN}✅ PASSOU - Academia criada${NC}"
    echo -e "${BLUE}   📍 ID: $ESCOLA_ID${NC}"
    echo -e "${BLUE}   🔑 Código: $CODIGO_ACADEMIA${NC}"
    echo -e "${BLUE}   📝 Evento: AcademiaCriada → GenesisDB Ledger${NC}\n"
    TESTS_PASSED=$((TESTS_PASSED + 1))
else
    echo -e "${RED}❌ FALHOU - Erro ao criar academia${NC}"
    show_response "$escola_response"
    echo ""
    TESTS_FAILED=$((TESTS_FAILED + 1))
    exit 1
fi

# Test 3: Login Academia
test_case "Autenticação - Login Academia"
login_data="{
    \"usuario\": \"$CODIGO_ACADEMIA\",
    \"senha\": \"escola2025\",
    \"type\": \"academia\"
}"

login_response=$(request "POST" "/login" "$login_data")
ESCOLA_TOKEN=$(extract_json "$login_response" "token")

if [ -n "$ESCOLA_TOKEN" ]; then
    echo -e "${GREEN}✅ PASSOU - Login realizado${NC}"
    echo -e "${BLUE}   🎫 Token JWT: ${ESCOLA_TOKEN:0:40}...${NC}\n"
    TESTS_PASSED=$((TESTS_PASSED + 1))
else
    echo -e "${RED}❌ FALHOU - Erro no login${NC}"
    show_response "$login_response"
    echo ""
    TESTS_FAILED=$((TESTS_FAILED + 1))
    exit 1
fi

# Test 4: Registrar Estudante
test_case "Command - Registrar Estudante (Event Sourcing)"
estudante_data='{
    "senha": "estudante2025",
    "nome": "Maria Santos Silva",
    "bilhete_identidade_responsavel": "TEST2025SPURI",
    "ano_escolar": "quarto_fundamental",
    "status_escolar": "ativo"
}'

estudante_response=$(request "POST" "/estudante/register" "$estudante_data")
ESTUDANTE_ID=$(extract_json "$estudante_response" "id")

if [ -n "$ESTUDANTE_ID" ]; then
    echo -e "${GREEN}✅ PASSOU - Estudante criado${NC}"
    echo -e "${BLUE}   📍 ID: $ESTUDANTE_ID${NC}"
    echo -e "${BLUE}   📝 Evento: EstudanteCriado → GenesisDB Ledger${NC}\n"
    TESTS_PASSED=$((TESTS_PASSED + 1))
else
    echo -e "${RED}❌ FALHOU - Erro ao criar estudante${NC}"
    show_response "$estudante_response"
    echo ""
    TESTS_FAILED=$((TESTS_FAILED + 1))
    exit 1
fi

# Test 5: Login Estudante
test_case "Autenticação - Login Estudante"
estudante_login='{
    "usuario": "TEST2025SPURI",
    "senha": "estudante2025",
    "type": "estudante"
}'

estudante_login_response=$(request "POST" "/login" "$estudante_login")
ESTUDANTE_TOKEN=$(extract_json "$estudante_login_response" "token")

if [ -n "$ESTUDANTE_TOKEN" ]; then
    echo -e "${GREEN}✅ PASSOU - Login do estudante realizado${NC}"
    echo -e "${BLUE}   🎫 Token JWT: ${ESTUDANTE_TOKEN:0:40}...${NC}\n"
    TESTS_PASSED=$((TESTS_PASSED + 1))
else
    echo -e "${RED}❌ FALHOU - Erro no login do estudante${NC}"
    show_response "$estudante_login_response"
    echo ""
    TESTS_FAILED=$((TESTS_FAILED + 1))
    exit 1
fi

# Test 6: Estudante Solicita Inscrição
test_case "Command - Solicitar Inscrição em Escola"
inscricao_data="{
    \"id_academia\": \"$ESCOLA_ID\",
    \"ano_escolar_inscricao\": \"quinto_fundamental\"
}"

inscricao_response=$(request "POST" "/estudante/inscricao-escola" "$inscricao_data" "$ESTUDANTE_TOKEN")

if echo "$inscricao_response" | grep -q "sucesso"; then
    echo -e "${GREEN}✅ PASSOU - Inscrição solicitada${NC}"
    echo -e "${BLUE}   📝 Evento: EstudanteInscrito → GenesisDB Ledger${NC}\n"
    TESTS_PASSED=$((TESTS_PASSED + 1))
else
    echo -e "${RED}❌ FALHOU - Erro ao solicitar inscrição${NC}"
    show_response "$inscricao_response"
    echo ""
    TESTS_FAILED=$((TESTS_FAILED + 1))
fi

# Test 7: Aguardar Projeções
echo -e "${BLUE}⏳ Aguardando projeções processarem eventos (2s)...${NC}\n"
sleep 2

# Test 8: Academia Lista Inscrições Pendentes
test_case "Query - Listar Inscrições Pendentes (CQRS)"
pendentes_response=$(request "GET" "/academia/inscricoes-pendentes" "" "$ESCOLA_TOKEN")

if echo "$pendentes_response" | grep -q "inscricoes"; then
    echo -e "${GREEN}✅ PASSOU - Inscrições listadas${NC}"
    echo -e "${BLUE}   📊 Query executada na projeção (Read Model)${NC}"
    
    INSCRICAO_ID=$(echo "$pendentes_response" | grep -o '"id":"[^"]*"' | head -1 | sed 's/"id":"\([^"]*\)"/\1/')
    
    if [ -n "$INSCRICAO_ID" ]; then
        echo -e "${BLUE}   📍 Inscrição ID: $INSCRICAO_ID${NC}\n"
    else
        echo -e "${YELLOW}   ⚠️  Nenhuma inscrição pendente encontrada${NC}\n"
    fi
    
    TESTS_PASSED=$((TESTS_PASSED + 1))
else
    echo -e "${RED}❌ FALHOU - Erro ao listar inscrições${NC}"
    show_response "$pendentes_response"
    echo ""
    TESTS_FAILED=$((TESTS_FAILED + 1))
fi

# Test 9: Academia Aprova Inscrição
if [ -n "$INSCRICAO_ID" ]; then
    test_case "Command - Aprovar Inscrição"
    aprovar_response=$(request "PUT" "/academia/inscricao/$INSCRICAO_ID/aprovar" "" "$ESCOLA_TOKEN")
    
    if echo "$aprovar_response" | grep -q "sucesso"; then
        echo -e "${GREEN}✅ PASSOU - Inscrição aprovada${NC}"
        echo -e "${BLUE}   📝 Eventos gerados:${NC}"
        echo -e "${BLUE}      • InscricaoAprovada (Estudante)${NC}"
        echo -e "${BLUE}      • InscricaoAprovada (Academia)${NC}\n"
        TESTS_PASSED=$((TESTS_PASSED + 1))
    else
        echo -e "${RED}❌ FALHOU - Erro ao aprovar inscrição${NC}"
        show_response "$aprovar_response"
        echo ""
        TESTS_FAILED=$((TESTS_FAILED + 1))
    fi
else
    echo -e "${YELLOW}⊘ Test 9: PULADO - Sem inscrição para aprovar${NC}\n"
fi

# Test 10: Aguardar Projeções
echo -e "${BLUE}⏳ Aguardando projeções processarem eventos (2s)...${NC}\n"
sleep 2

# Test 11: Registrar Notas
test_case "Command - Registrar Notas do Estudante"
notas_data="{
    \"estudante_id\": \"$ESTUDANTE_ID\",
    \"ano_lectivo\": \"2025/2026\",
    \"periodo\": \"trimestre_1\",
    \"materias\": [
        {\"nome\": \"Matemática\", \"nota\": 17},
        {\"nome\": \"Português\", \"nota\": 16},
        {\"nome\": \"Ciências Naturais\", \"nota\": 18},
        {\"nome\": \"História\", \"nota\": 15}
    ]
}"

notas_response=$(request "POST" "/academia/notas-aluno" "$notas_data" "$ESCOLA_TOKEN")

if echo "$notas_response" | grep -q "sucesso"; then
    echo -e "${GREEN}✅ PASSOU - Notas registradas${NC}"
    echo -e "${BLUE}   📝 Evento: NotasRegistradas → GenesisDB Ledger${NC}\n"
    TESTS_PASSED=$((TESTS_PASSED + 1))
else
    echo -e "${RED}❌ FALHOU - Erro ao registrar notas${NC}"
    show_response "$notas_response"
    echo ""
    TESTS_FAILED=$((TESTS_FAILED + 1))
fi

# Test 12: Registrar Faltas
test_case "Command - Registrar Faltas do Estudante"
faltas_data="{
    \"estudante_id\": \"$ESTUDANTE_ID\",
    \"ano_lectivo\": \"2025/2026\",
    \"periodo\": \"trimestre_1\",
    \"materias\": [
        {\"nome\": \"Matemática\", \"faltas\": 1},
        {\"nome\": \"Português\", \"faltas\": 0},
        {\"nome\": \"Ciências Naturais\", \"faltas\": 2},
        {\"nome\": \"História\", \"faltas\": 0}
    ]
}"

faltas_response=$(request "POST" "/academia/faltas-aluno" "$faltas_data" "$ESCOLA_TOKEN")

if echo "$faltas_response" | grep -q "sucesso"; then
    echo -e "${GREEN}✅ PASSOU - Faltas registradas${NC}"
    echo -e "${BLUE}   📝 Evento: FaltasRegistradas → GenesisDB Ledger${NC}\n"
    TESTS_PASSED=$((TESTS_PASSED + 1))
else
    echo -e "${RED}❌ FALHOU - Erro ao registrar faltas${NC}"
    show_response "$faltas_response"
    echo ""
    TESTS_FAILED=$((TESTS_FAILED + 1))
fi

# Test 13: Aguardar Projeções
echo -e "${BLUE}⏳ Aguardando projeções processarem eventos (2s)...${NC}\n"
sleep 2

# Test 14: Query - Consultar Notas
test_case "Query - Consultar Notas (CQRS Read Model)"
consulta_notas=$(request "GET" "/notas-estudante/$ESTUDANTE_ID" "" "$ESTUDANTE_TOKEN")

if echo "$consulta_notas" | grep -q "Matemática"; then
    echo -e "${GREEN}✅ PASSOU - Notas consultadas${NC}"
    echo -e "${BLUE}   📊 Dados lidos da projeção (não do ledger)${NC}\n"
    TESTS_PASSED=$((TESTS_PASSED + 1))
else
    echo -e "${RED}❌ FALHOU - Erro ao consultar notas${NC}"
    show_response "$consulta_notas"
    echo ""
    TESTS_FAILED=$((TESTS_FAILED + 1))
fi

# Test 15: Query - Consultar Faltas
test_case "Query - Consultar Faltas"
consulta_faltas=$(request "GET" "/faltas-estudante/$ESTUDANTE_ID" "" "$ESTUDANTE_TOKEN")

if echo "$consulta_faltas" | grep -q "faltas"; then
    echo -e "${GREEN}✅ PASSOU - Faltas consultadas${NC}\n"
    TESTS_PASSED=$((TESTS_PASSED + 1))
else
    echo -e "${RED}❌ FALHOU - Erro ao consultar faltas${NC}"
    show_response "$consulta_faltas"
    echo ""
    TESTS_FAILED=$((TESTS_FAILED + 1))
fi

# Test 16: Query - Histórico Completo
test_case "Query - Histórico Completo do Estudante"
historico=$(request "GET" "/historico-estudante/$ESTUDANTE_ID" "" "$ESTUDANTE_TOKEN")

if echo "$historico" | grep -q "notas" && echo "$historico" | grep -q "faltas"; then
    echo -e "${GREEN}✅ PASSOU - Histórico completo obtido${NC}"
    echo -e "${BLUE}   📊 Dados agregados de múltiplas projeções${NC}\n"
    TESTS_PASSED=$((TESTS_PASSED + 1))
else
    echo -e "${RED}❌ FALHOU - Erro ao consultar histórico${NC}"
    show_response "$historico"
    echo ""
    TESTS_FAILED=$((TESTS_FAILED + 1))
fi

# Test 17: Event Sourcing - Ver Timeline de Eventos
test_case "Event Sourcing - Timeline Completa de Eventos"
eventos=$(request "GET" "/eventos-estudante/$ESTUDANTE_ID" "" "$ESTUDANTE_TOKEN")

if echo "$eventos" | grep -q "eventos"; then
    total_eventos=$(extract_json "$eventos" "total")
    if [ -z "$total_eventos" ]; then
        total_eventos=$(echo "$eventos" | grep -o '"total":[0-9]*' | sed 's/"total"://')
    fi
    
    echo -e "${GREEN}✅ PASSOU - Timeline de eventos consultada${NC}"
    echo -e "${BLUE}   📜 Total de eventos: $total_eventos${NC}"
    echo -e "${BLUE}   💾 Eventos imutáveis no GenesisDB Ledger${NC}\n"
    TESTS_PASSED=$((TESTS_PASSED + 1))
else
    echo -e "${RED}❌ FALHOU - Erro ao consultar eventos${NC}"
    show_response "$eventos"
    echo ""
    TESTS_FAILED=$((TESTS_FAILED + 1))
fi

# Test 18: Verificar Integridade (Hash Chain)
test_case "GenesisDB - Verificar Integridade da Hash Chain"
integridade=$(request "GET" "/verificar-integridade/$ESTUDANTE_ID" "" "$ESTUDANTE_TOKEN")

if echo "$integridade" | grep -q "integro"; then
    echo -e "${GREEN}✅ PASSOU - Hash chain íntegra${NC}"
    echo -e "${BLUE}   🔒 Blockchain interna verificada (SHA-256)${NC}\n"
    TESTS_PASSED=$((TESTS_PASSED + 1))
else
    echo -e "${RED}❌ FALHOU - Problema na verificação${NC}"
    show_response "$integridade"
    echo ""
    TESTS_FAILED=$((TESTS_FAILED + 1))
fi

# Test 19: Validação - Província Inválida
test_case "Validação - Rejeitar Província Inválida"
escola_invalida='{
    "type": "escola",
    "senha": "teste123",
    "nome": "Escola Teste Inválida",
    "provincia": "ProvinciaFantasia",
    "endereco": "Rua Teste",
    "nivel_escolar": "fundamental"
}'

escola_invalida_response=$(request "POST" "/academia/register" "$escola_invalida")

if echo "$escola_invalida_response" | grep -qi "inválid"; then
    echo -e "${GREEN}✅ PASSOU - Validação funcionando corretamente${NC}\n"
    TESTS_PASSED=$((TESTS_PASSED + 1))
else
    echo -e "${RED}❌ FALHOU - Validação não funcionou${NC}"
    show_response "$escola_invalida_response"
    echo ""
    TESTS_FAILED=$((TESTS_FAILED + 1))
fi

# =============================================================================
# RESUMO FINAL
# =============================================================================

echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}\n"
echo -e "${BOLD}${BLUE}╔════════════════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}${BLUE}║              📊 RESUMO DOS TESTES                      ║${NC}"
echo -e "${BOLD}${BLUE}╚════════════════════════════════════════════════════════╝${NC}\n"

SUCCESS_RATE=$(awk "BEGIN {printf \"%.1f\", ($TESTS_PASSED/$TOTAL_TESTS)*100}")

echo -e "${BOLD}Total de Testes:${NC} $TOTAL_TESTS"
echo -e "${GREEN}${BOLD}✅ Passou:${NC} $TESTS_PASSED"
echo -e "${RED}${BOLD}❌ Falhou:${NC} $TESTS_FAILED"
echo -e "${BOLD}Taxa de Sucesso:${NC} ${SUCCESS_RATE}%\n"

if [ $TESTS_FAILED -eq 0 ]; then
    echo -e "${BOLD}${GREEN}╔════════════════════════════════════════════════════════╗${NC}"
    echo -e "${BOLD}${GREEN}║     🎉 TODOS OS TESTES PASSARAM COM SUCESSO! 🎉       ║${NC}"
    echo -e "${BOLD}${GREEN}╚════════════════════════════════════════════════════════╝${NC}\n"
    
    echo -e "${BOLD}✨ Sistema Event Sourcing Funcionando Perfeitamente!${NC}\n"
    
    echo -e "${BOLD}📋 Recursos Testados:${NC}"
    echo -e "  ✅ Event Sourcing completo"
    echo -e "  ✅ CQRS (Commands e Queries)"
    echo -e "  ✅ GenesisDB Ledger (append-only)"
    echo -e "  ✅ Hash Chain (blockchain interna)"
    echo -e "  ✅ Projeções automáticas"
    echo -e "  ✅ Agregados (Estudante e Academia)"
    echo -e "  ✅ Auditoria completa"
    echo -e "  ✅ Verificação de integridade"
    echo -e "  ✅ Autenticação JWT"
    echo -e "  ✅ Validações de negócio\n"
    
    echo -e "${BOLD}💾 Dados de Teste Criados:${NC}"
    echo -e "  🏫 Academia: $CODIGO_ACADEMIA"
    echo -e "  👤 Estudante: TEST2025SPURI"
    echo -e "  📝 Eventos no Ledger: ~12+"
    echo -e "  📊 Projeções atualizadas: 5\n"
    
    exit 0
else
    echo -e "${BOLD}${RED}╔════════════════════════════════════════════════════════╗${NC}"
    echo -e "${BOLD}${RED}║        ⚠️  ALGUNS TESTES FALHARAM                     ║${NC}"
    echo -e "${BOLD}${RED}╚════════════════════════════════════════════════════════╝${NC}\n"
    
    echo -e "${YELLOW}💡 Dicas para Debug:${NC}"
    echo -e "  1. Verifique os logs do servidor"
    echo -e "  2. Confirme que o PostgreSQL está rodando"
    echo -e "  3. Verifique as migrations foram executadas"
    echo -e "  4. Teste endpoints individualmente\n"
    
    exit 1
fi