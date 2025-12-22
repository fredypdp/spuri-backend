# Spuri - Sistema de Gestão Acadêmica

Sistema de gestão acadêmica com Event Sourcing para Angola.

## 📁 Estrutura do Projeto

```
spuri/
├── cmd/
│   └── server/
│       └── main.go          # Ponto de entrada da aplicação
├── internal/
│   ├── domain/
│   │   ├── events.go        # Definição dos eventos
│   │   └── models.go        # Modelos de dados
│   ├── handlers/
│   │   ├── auth.go          # Autenticação
│   │   ├── registro.go      # Registro de notas/faltas
│   │   ├── inscricao.go     # Inscrições
│   │   └── consulta.go      # Consultas
│   ├── store/
│   │   ├── db.go            # Conexão com banco
│   │   ├── event_store.go   # Event Store
│   │   └── queries.go       # Queries de leitura
│   └── middleware/
│       └── auth.go          # Middleware de autenticação
├── migrations/
│   └── 001_init.sql         # Schema do banco de dados
├── .env.example             # Exemplo de variáveis de ambiente
├── go.mod                   # Dependências Go
└── README.md                # Este arquivo
```

## 🚀 Instalação

### 1. Pré-requisitos

- Go 1.21 ou superior
- PostgreSQL 14 ou superior
- Git

### 2. Clone e Configure

```bash
# Clone o repositório (ou crie a pasta manualmente)
mkdir spuri && cd spuri

# Copie os arquivos fornecidos para a estrutura acima

# Configure as variáveis de ambiente
cp .env.example .env
# Edite o .env com suas credenciais do PostgreSQL
```

### 3. Configure o Banco de Dados

```bash
# Entre no PostgreSQL
psql -U postgres

# Crie o banco
CREATE DATABASE spuri;

# Execute as migrations
psql -U postgres -d spuri -f migrations/001_init.sql
```

### 4. Instale Dependências

```bash
go mod init spuri
go mod tidy
```

### 5. Execute o Servidor

```bash
# Desenvolvimento
go run cmd/server/main.go

# Ou compile e execute
go build -o spuri cmd/server/main.go
./spuri
```

O servidor estará rodando em `http://localhost:8080`

## 📖 Como Usar

### 1. Cadastrar uma Academia (Escola/Universidade)

```bash
curl -X POST http://localhost:8080/academia/register \
  -H "Content-Type: application/json" \
  -d '{
    "type": "escola",
    "senha": "senha123",
    "nome": "Escola Primária de Luanda",
    "endereco": "Luanda, Angola",
    "nivel_escolar": "fundamental"
  }'
```

**Resposta:** Retorna o `codigo_academia` (ex: LUA20251) e o ID

### 2. Login da Academia

```bash
curl -X POST http://localhost:8080/login \
  -H "Content-Type: application/json" \
  -d '{
    "usuario": "LUA20251",
    "senha": "senha123",
    "type": "academia"
  }'
```

**Resposta:** Retorna um token JWT. Use este token nas próximas requisições.

### 3. Cadastrar um Estudante

```bash
curl -X POST http://localhost:8080/estudante/register \
  -H "Content-Type: application/json" \
  -d '{
    "senha": "senha456",
    "nome": "João Silva",
    "bilhete_identidade_responsavel": "123456789LA",
    "ano_escolar": "primeiro_fundamental",
    "status_escolar": "ativo"
  }'
```

### 4. Login do Estudante

```bash
curl -X POST http://localhost:8080/login \
  -H "Content-Type: application/json" \
  -d '{
    "usuario": "123456789LA",
    "senha": "senha456",
    "type": "estudante"
  }'
```

### 5. Estudante Solicita Inscrição

```bash
curl -X POST http://localhost:8080/inscricao-escola \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer SEU_TOKEN_ESTUDANTE" \
  -d '{
    "id_academia": "uuid-da-escola",
    "ano_escolar_inscricao": "segundo_fundamental"
  }'
```

### 6. Academia Registra Notas

```bash
curl -X POST http://localhost:8080/notas-aluno \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer SEU_TOKEN_ACADEMIA" \
  -d '{
    "estudante_id": "uuid-do-estudante",
    "ano_lectivo": "2025/2026",
    "periodo": "semestre_1",
    "materias": [
      {"nome": "Matemática", "nota": 15},
      {"nome": "Português", "nota": 14}
    ]
  }'
```

### 7. Consultar Histórico de Notas

```bash
curl http://localhost:8080/notas-estudante/UUID_ESTUDANTE \
  -H "Authorization: Bearer SEU_TOKEN"
```

## 🔑 Event Sourcing Explicado

### O que é?

Em vez de salvar apenas o estado atual (ex: "nota = 15"), salvamos **todos os eventos** que aconteceram:
- "Notas registradas em 01/06"
- "Faltas registradas em 15/06"
- "Estudante transferido em 20/07"

### Vantagens

1. **Auditoria completa**: Vemos tudo que aconteceu
2. **Rastreabilidade**: Quem fez, quando fez, o que fez
3. **Reconstrução**: Podemos reconstruir qualquer estado passado
4. **Imutabilidade**: Eventos nunca são apagados ou alterados

### Como funciona no Spuri

1. **Escrita**: Academia registra notas → Sistema cria evento → Evento salvo no `event_store`
2. **Leitura**: Sistema consulta tabelas otimizadas (`registro_notas`, `registro_faltas`)
3. **Verificação**: A qualquer momento, podemos reconstruir o histórico completo

## 🔐 Segurança

- Senhas são criptografadas com bcrypt
- Autenticação via JWT
- Tokens expiram em 24 horas
- Cada requisição valida permissões

## 🧪 Testando

```bash
# Teste simples
curl http://localhost:8080/health

# Deve retornar: {"status": "ok"}
```

## 📝 Próximos Passos

1. ✅ Sistema funcionando localmente
2. ⬜ Adicionar testes automatizados
3. ⬜ Implementar transferência de estudantes
4. ⬜ Dashboard web
5. ⬜ Deploy em produção

## ❓ Problemas Comuns

### Erro de conexão com PostgreSQL
- Verifique se o PostgreSQL está rodando: `sudo service postgresql status`
- Confirme as credenciais no arquivo `.env`

### "Port already in use"
- Outra aplicação está usando a porta 8080
- Mude a porta no arquivo `.env` ou `.env.example`

### Erro ao criar schema
- Certifique-se de que o banco `spuri` foi criado
- Execute as migrations manualmente: `psql -U postgres -d spuri -f migrations/001_init.sql`

## 📧 Suporte

Para dúvidas sobre Event Sourcing ou implementação, consulte os comentários no código.