#!/bin/bash

# Script de Teste da API Spuri
# Execute: chmod +x test_api.sh && ./test_api.sh

API_URL="${API_URL:-http://localhost:8080}"
BOLD='\033[1m'
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${BOLD}🧪 Testando API Spuri${NC}"
echo -e "URL: $API_URL\n"

# Função para fazer requests
request() {
    local method=$1
    local endpoint=$2
    local data=$3
    local token=$4
    
    if [ -n "$token" ]; then
        curl -s -X $method "$API_URL$endpoint" \
            -H "Content-Type: application/json" \
            -H "Authorization: Bearer $token" \
            -d "$data"
    else
        curl -s -X $method "$API_URL$endpoint" \
            -H "Content-Type: application/json" \
            -d "$data"
    fi
}

# Função para extrair valor do JSON
extract_json() {
    local json="$1"
    local key="$2"
    
    # Tenta extrair valor (funciona para strings e números)
    echo "$json" | grep -o "\"$key\"[[:space:]]*:[[:space:]]*[^,}]*" | sed 's/.*:[[:space:]]*//;s/"//g'
}

# Teste 1: Health Check
echo -e "${YELLOW}1. Testando Health Check...${NC}"
response=$(curl -s "$API_URL/health")
status=$(extract_json "$response" "status")

if [ "$status" = "ok" ]; then
    echo -e "${GREEN}✓ Health check passou!${NC}\n"
else
    echo -e "${RED}✗ Health check falhou!${NC}"
    echo "Resposta: $response"
    exit 1
fi

# Teste 2: Registrar Academia
echo -e "${YELLOW}2. Registrando escola...${NC}"
escola_data='{
    "type": "escola",
    "senha": "teste123",
    "nome": "Escola Teste Automatizado",
    "provincia": "Luanda",
    "endereco": "Rua 123, Bairro Maianga, Luanda",
    "nivel_escolar": "fundamental"
}'

escola_response=$(request "POST" "/academia/register" "$escola_data")
escola_id=$(extract_json "$escola_response" "id")
codigo_academia=$(extract_json "$escola_response" "codigo_academia")

if [ -n "$escola_id" ]; then
    echo -e "${GREEN}✓ Escola criada!${NC}"
    echo "  ID: $escola_id"
    echo "  Código: $codigo_academia"
    echo ""
else
    echo -e "${RED}✗ Falha ao criar escola!${NC}"
    echo "Resposta: $escola_response"
    exit 1
fi

# Teste 3: Login da Academia
echo -e "${YELLOW}3. Fazendo login da escola...${NC}"
login_data="{
    \"usuario\": \"$codigo_academia\",
    \"senha\": \"teste123\",
    \"type\": \"academia\"
}"

login_response=$(request "POST" "/login" "$login_data")
escola_token=$(extract_json "$login_response" "token")

if [ -n "$escola_token" ]; then
    echo -e "${GREEN}✓ Login realizado!${NC}"
    echo "  Token: ${escola_token:0:20}..."
    echo ""
else
    echo -e "${RED}✗ Falha no login!${NC}"
    echo "Resposta: $login_response"
    exit 1
fi

# Teste 4: Registrar Estudante
echo -e "${YELLOW}4. Registrando estudante...${NC}"
estudante_data='{
    "senha": "estudante123",
    "nome": "João Teste Automatizado",
    "bilhete_identidade_responsavel": "TEST123456LA",
    "ano_escolar": "terceiro_fundamental",
    "status_escolar": "ativo"
}'

estudante_response=$(request "POST" "/estudante/register" "$estudante_data")
estudante_id=$(extract_json "$estudante_response" "id")

if [ -n "$estudante_id" ]; then
    echo -e "${GREEN}✓ Estudante criado!${NC}"
    echo "  ID: $estudante_id"
    echo ""
else
    echo -e "${RED}✗ Falha ao criar estudante!${NC}"
    echo "Resposta: $estudante_response"
    exit 1
fi

# Teste 5: Login do Estudante
echo -e "${YELLOW}5. Fazendo login do estudante...${NC}"
estudante_login='{
    "usuario": "TEST123456LA",
    "senha": "estudante123",
    "type": "estudante"
}'

estudante_login_response=$(request "POST" "/login" "$estudante_login")
estudante_token=$(extract_json "$estudante_login_response" "token")

if [ -n "$estudante_token" ]; then
    echo -e "${GREEN}✓ Login do estudante realizado!${NC}"
    echo "  Token: ${estudante_token:0:20}..."
    echo ""
else
    echo -e "${RED}✗ Falha no login do estudante!${NC}"
    echo "Resposta: $estudante_login_response"
    exit 1
fi

# Teste 6: Estudante Solicita Inscrição
echo -e "${YELLOW}6. Estudante solicitando inscrição...${NC}"
inscricao_data="{
    \"id_academia\": \"$escola_id\",
    \"ano_escolar_inscricao\": \"quarto_fundamental\"
}"

inscricao_response=$(request "POST" "/estudante/inscricao-escola" "$inscricao_data" "$estudante_token")
inscricao_id=$(extract_json "$inscricao_response" "inscricao_id")

if [ -n "$inscricao_id" ]; then
    echo -e "${GREEN}✓ Inscrição solicitada!${NC}"
    echo "  ID: $inscricao_id"
    echo ""
else
    echo -e "${RED}✗ Falha ao solicitar inscrição!${NC}"
    echo "Resposta: $inscricao_response"
    exit 1
fi

# Teste 7: Academia Lista Inscrições Pendentes
echo -e "${YELLOW}7. Listando inscrições pendentes...${NC}"
pendentes_response=$(request "GET" "/academia/inscricoes-pendentes" "" "$escola_token")
total=$(extract_json "$pendentes_response" "total")

if [ "$total" -ge "1" ]; then
    echo -e "${GREEN}✓ Inscrições listadas!${NC}"
    echo "  Total: $total"
    echo ""
else
    echo -e "${RED}✗ Nenhuma inscrição encontrada!${NC}"
    echo "Resposta: $pendentes_response"
    exit 1
fi

# Teste 8: Academia Aprova Inscrição
echo -e "${YELLOW}8. Aprovando inscrição...${NC}"
aprovar_response=$(request "PUT" "/academia/inscricao/$inscricao_id/aprovar" "" "$escola_token")

if echo "$aprovar_response" | grep -q "aprovada com sucesso"; then
    echo -e "${GREEN}✓ Inscrição aprovada!${NC}\n"
else
    echo -e "${RED}✗ Falha ao aprovar inscrição!${NC}"
    echo "Resposta: $aprovar_response"
    exit 1
fi

# Teste 9: Registrar Notas
echo -e "${YELLOW}9. Registrando notas...${NC}"
notas_data="{
    \"estudante_id\": \"$estudante_id\",
    \"ano_lectivo\": \"2025/2026\",
    \"periodo\": \"trimestre_1\",
    \"materias\": [
        {\"nome\": \"Matemática\", \"nota\": 16},
        {\"nome\": \"Português\", \"nota\": 15},
        {\"nome\": \"Ciências\", \"nota\": 17}
    ]
}"

notas_response=$(request "POST" "/academia/notas-aluno" "$notas_data" "$escola_token")

if echo "$notas_response" | grep -q "registradas com sucesso"; then
    echo -e "${GREEN}✓ Notas registradas!${NC}\n"
else
    echo -e "${RED}✗ Falha ao registrar notas!${NC}"
    echo "Resposta: $notas_response"
    exit 1
fi

# Teste 10: Registrar Faltas
echo -e "${YELLOW}10. Registrando faltas...${NC}"
faltas_data="{
    \"estudante_id\": \"$estudante_id\",
    \"ano_lectivo\": \"2025/2026\",
    \"periodo\": \"trimestre_1\",
    \"materias\": [
        {\"nome\": \"Matemática\", \"faltas\": 2},
        {\"nome\": \"Português\", \"faltas\": 0},
        {\"nome\": \"Ciências\", \"faltas\": 1}
    ]
}"

faltas_response=$(request "POST" "/academia/faltas-aluno" "$faltas_data" "$escola_token")

if echo "$faltas_response" | grep -q "registradas com sucesso"; then
    echo -e "${GREEN}✓ Faltas registradas!${NC}\n"
else
    echo -e "${RED}✗ Falha ao registrar faltas!${NC}"
    echo "Resposta: $faltas_response"
    exit 1
fi

# Teste 11: Consultar Notas
echo -e "${YELLOW}11. Consultando notas do estudante...${NC}"
consulta_notas=$(request "GET" "/notas-estudante/$estudante_id" "" "$estudante_token")

if echo "$consulta_notas" | grep -q "Matemática"; then
    echo -e "${GREEN}✓ Notas consultadas com sucesso!${NC}\n"
else
    echo -e "${RED}✗ Falha ao consultar notas!${NC}"
    echo "Resposta: $consulta_notas"
    exit 1
fi

# Teste 12: Consultar Histórico Completo
echo -e "${YELLOW}12. Consultando histórico completo...${NC}"
historico=$(request "GET" "/historico-estudante/$estudante_id" "" "$estudante_token")

if echo "$historico" | grep -q "estudante" && echo "$historico" | grep -q "notas"; then
    echo -e "${GREEN}✓ Histórico consultado com sucesso!${NC}\n"
else
    echo -e "${RED}✗ Falha ao consultar histórico!${NC}"
    echo "Resposta: $historico"
    exit 1
fi

# Teste 13: Consultar Eventos (Event Sourcing)
echo -e "${YELLOW}13. Consultando eventos do estudante (Event Sourcing)...${NC}"
eventos=$(request "GET" "/eventos-estudante/$estudante_id" "" "$estudante_token")

if echo "$eventos" | grep -q "eventos" && echo "$eventos" | grep -q "total"; then
    total_eventos=$(extract_json "$eventos" "total")
    echo -e "${GREEN}✓ Eventos consultados!${NC}"
    echo "  Total de eventos: $total_eventos"
    echo ""
else
    echo -e "${RED}✗ Falha ao consultar eventos!${NC}"
    echo "Resposta: $eventos"
    exit 1
fi

# Teste 14: Validar Província Inválida
echo -e "${YELLOW}14. Testando validação de província inválida...${NC}"
escola_invalida='{
    "type": "escola",
    "senha": "teste123",
    "nome": "Escola Teste Província Inválida",
    "provincia": "ProvinciaInexistente",
    "endereco": "Rua 123",
    "nivel_escolar": "fundamental"
}'

escola_invalida_response=$(request "POST" "/academia/register" "$escola_invalida")

if echo "$escola_invalida_response" | grep -q "província inválida"; then
    echo -e "${GREEN}✓ Validação de província funcionando!${NC}\n"
else
    echo -e "${RED}✗ Validação não funcionou!${NC}"
    echo "Resposta: $escola_invalida_response"
    exit 1
fi

# Resumo Final
echo -e "${BOLD}${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BOLD}${GREEN}✅ TODOS OS TESTES PASSARAM!${NC}"
echo -e "${BOLD}${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
echo -e "${BOLD}📊 Resumo:${NC}"
echo "  • Escola criada: $codigo_academia (Província: LUA)"
echo "  • Estudante criado: $estudante_id"
echo "  • Inscrição aprovada: $inscricao_id"
echo "  • Notas e faltas registradas"
echo "  • Consultas funcionando"
echo "  • Event Sourcing operacional"
echo "  • Validação de província funcionando"
echo ""
echo -e "${BOLD}🎉 Sistema Spuri está funcionando perfeitamente!${NC}"
echo ""
echo -e "${YELLOW}Nota: Os dados de teste foram criados no banco.${NC}"
echo -e "${YELLOW}Para limpar, delete manualmente ou recrie o banco.${NC}"