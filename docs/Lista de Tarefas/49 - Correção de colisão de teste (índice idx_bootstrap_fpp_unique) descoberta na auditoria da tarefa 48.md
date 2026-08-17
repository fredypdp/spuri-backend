---
criado: 2026-08-17
origem: auditoria da tarefa 48 (commit `bf9457b`, PR #545), feita por Claude (Anthropic). O código e a
  documentação da tarefa 48 foram conferidos byte a byte contra a especificação — 100% corretos, nada a
  corrigir ali. Esta tarefa é sobre um problema diferente, encontrado ao rodar a suíte de testes completa
  (não só os testes novos) contra um PostgreSQL 16 real, com as 115 migrations do projeto aplicadas.
status: pendente
depende_de: "47 - Correção de dois problemas de backend do módulo de pagamentos (...).md" e
  "48 - Auditoria da tarefa 47 + endpoint de consulta do estudante + atualização da documentação da API.md"
  (ambas já mescladas)
---

# Correção de colisão de teste entre `TestIntegrationListarCobrancasAppyPayRejeitaAdminSemPermissaoFPP` e `TestIntegrationFinanceRejectsNonFPPAdmins`

## Prompt recomendado para executar a tarefa

```
Leia por completo o arquivo "docs/Lista de Tarefas/49 - Correção de colisão de teste (índice
idx_bootstrap_fpp_unique) descoberta na auditoria da tarefa 48.md". É uma correção de uma linha, num arquivo
de teste, já validada de ponta a ponta (antes e depois da correção) contra um PostgreSQL 16 real pelo
orquestrador — não é uma suposição. Aplique exatamente o "Localizar" / "Substituir por" da seção "Correção",
rode a checklist ao final, e reporte o resultado. Não há nada para investigar nem decidir.
```

## O que foi encontrado

Isto **não é um problema na tarefa 48** — é um problema pré-existente da tarefa 47 que só aparece quando toda
a suíte de testes do pacote `internal/handlers` roda junta (não quando cada teste novo roda isolado, que foi
como a auditoria da tarefa 47 e a implementação da tarefa 48 foram validadas até agora).

Rodando `go test ./internal/handlers/... -run TestIntegration -v` contra um PostgreSQL 16 real (schema
completo, 115 migrations aplicadas), com todos os testes do pacote juntos:

```
--- FAIL: TestIntegrationFinanceRejectsNonFPPAdmins (0.04s)
    --- FAIL: TestIntegrationFinanceRejectsNonFPPAdmins/gerente (0.00s)
        financeiro_handlers_integration_test.go:195: pq: duplicate key value violates unique constraint "idx_bootstrap_fpp_unique"
```

**Causa raiz:** `idx_bootstrap_fpp_unique` (migration 025) é um índice único parcial —
`CREATE UNIQUE INDEX ... ON projection_admins (role) WHERE created_by IS NULL` — que garante no máximo **um**
admin por `role` com `created_by IS NULL` (o admin de "bootstrap", sem criador). Dois testes diferentes do
pacote `internal/handlers` inserem um admin com `role='gerente'` sem informar `created_by` (que por omissão
fica `NULL`):

1. `TestIntegrationFinanceRejectsNonFPPAdmins` (pré-existente, `internal/handlers/financeiro_handlers_integration_test.go`, linha 194) — já existia antes da tarefa 47 e está correto isoladamente.
2. `TestIntegrationListarCobrancasAppyPayRejeitaAdminSemPermissaoFPP` (introduzido pela tarefa 47, `internal/handlers/financeiro_cobrancas_handlers_test.go`, linha 103) — este é o que precisa mudar.

Como `go test` roda todas as funções de teste de um pacote sequencialmente, na mesma conexão/banco (sem
isolar `projection_admins` entre testes), o primeiro dos dois a rodar (por ordem alfabética de arquivo,
`financeiro_cobrancas_handlers_test.go` vem antes de `financeiro_handlers_integration_test.go`) insere o
admin `role='gerente', created_by=NULL` com sucesso; o segundo colide com o índice único e falha.

**Por que isso não foi pego antes:** a auditoria da tarefa 47 validou a SQL de `ListCobrancas` isoladamente
contra Postgres real, mas não tinha ainda, naquele momento, como compilar e rodar a suíte completa de testes
Go do projeto (bloqueio de rede para o proxy de módulos Go). Essa capacidade só foi destravada durante esta
auditoria da tarefa 48 (usando busca direta de dependências via VCS, contornando o proxy) — e foi rodando a
suíte completa, não só os testes novos isolados, que esta colisão apareceu. Os testes novos da tarefa 47, `go
build`, `go vet` e `gofmt` continuam corretos; é especificamente esta interação entre dois testes que estava
quebrada.

**Correção validada:** ao adicionar `created_by=<próprio id do admin>` (auto-referência — a coluna é uma FK
para `projection_admins(id)`, e uma linha pode referenciar o próprio `id` dentro do mesmo `INSERT`, já
confirmado funcionando no Postgres) no `INSERT` de
`TestIntegrationListarCobrancasAppyPayRejeitaAdminSemPermissaoFPP`, a linha deixa de contar como "bootstrap"
para o índice único, e a colisão desaparece. **Rodei a suíte completa de `internal/handlers` (banco recriado
do zero) duas vezes: antes da correção (falhou, exatamente como acima) e depois da correção (100% `PASS`, 16
testes incluindo subtestes, zero falhas).** Isso não muda nada do que o teste verifica — `created_by` não é
usado por nenhuma lógica de autorização (`verificarPermissaoAdmin` só olha `role`), então o teste continua
provando exatamente a mesma coisa: um admin `role='gerente'` recebe `403` na rota.

## Escopo desta tarefa

**Arquivo a modificar:** `internal/handlers/financeiro_cobrancas_handlers_test.go` — uma única linha.

**Não alterar:** `internal/handlers/financeiro_handlers_integration_test.go` (o teste pré-existente está
correto isoladamente; não precisa de nenhuma mudança) nem qualquer outro arquivo.

## Correção

**Localizar** (dentro de `TestIntegrationListarCobrancasAppyPayRejeitaAdminSemPermissaoFPP`):

```go
	if _, err := client.DB().Exec(`INSERT INTO projection_admins (id,nome,email,senha_hash,role,status) VALUES ($1,'gerente-lst',$2,'hash','gerente','ativo')`, adminID, "gerente-lst-"+uuid.NewString()+"@example.test"); err != nil {
```

**Substituir por:**

```go
	if _, err := client.DB().Exec(`INSERT INTO projection_admins (id,nome,email,senha_hash,role,status,created_by) VALUES ($1,'gerente-lst',$2,'hash','gerente','ativo',$1)`, adminID, "gerente-lst-"+uuid.NewString()+"@example.test"); err != nil {
```

(Note que `$1` é reutilizado duas vezes na mesma query — uma vez para `id`, outra para `created_by` — o que é
uma sintaxe válida e já usada em outras partes do projeto, ex.: `ListCobrancasEstudante`.)

Depois de aplicar, rode `gofmt -w internal/handlers/financeiro_cobrancas_handlers_test.go` (não deve mudar
nada além do já editado, já que é só um valor a mais dentro de uma linha já formatada).

## Checklist de aceitação

1. ```
   go build ./...
   go vet ./...
   gofmt -l .
   ```
   Todos devem terminar limpos.

2. Se você tiver PostgreSQL disponível no seu ambiente (o que não é garantido — ver a limitação já registrada
   na tarefa 48): rode a suíte completa do pacote, banco limpo, com `RUN_POSTGRES_INTEGRATION=1`:
   ```
   go test ./internal/handlers/... -run TestIntegration -v
   ```
   Confirme especificamente que tanto `TestIntegrationFinanceRejectsNonFPPAdmins/gerente` quanto
   `TestIntegrationListarCobrancasAppyPayRejeitaAdminSemPermissaoFPP` aparecem como `--- PASS` na mesma
   execução (rodando juntos — é exatamente essa combinação que falhava antes da correção). Se o seu ambiente
   não tiver Postgres disponível, reporte isso explicitamente em vez de marcar como `✅` — igual à orientação
   já dada na tarefa 48.

3. ```
   git diff --stat
   ```
   Deve mostrar exatamente um arquivo alterado: `internal/handlers/financeiro_cobrancas_handlers_test.go`,
   com uma única linha modificada.

Eu já validei esta correção de ponta a ponta contra PostgreSQL 16 real (suíte completa, banco recriado do
zero, antes e depois) — o passo 2 acima é uma confirmação adicional bem-vinda no seu ambiente, mas a
aceitação desta tarefa não depende dela.
