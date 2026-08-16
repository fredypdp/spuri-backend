---
criado: 2026-08-16 17:30
origem: Depuração profunda do módulo de pagamento (fluxo de matrícula/inscrição de estudante e mensalidade) conduzida por Claude (Anthropic) em ambiente real (Go 1.24, PostgreSQL 16, repositório clonado e compilado de fato), a pedido do Fredy, para auditar se as tarefas 41 e 44 foram corretamente implementadas. Resultado: **ambas as tarefas foram implementadas exatamente como especificado, sem nenhum bug de código encontrado.** O único trabalho pendente é administrativo (checklist de integração nunca confirmado por falta de PostgreSQL nas sessões anteriores do Codex, e arquivos de tarefa nunca movidos/atualizados). Esta tarefa existe só para fechar essa pendência de forma mecânica.
status: pendente
relacionado: "41 - Redesign da autenticação de webhook AppyPay (secret gerado pelo servidor).md, 42 - Correção de bugs críticos de integridade do ledger no módulo de mensalidades e matrícula (auditoria pós-tarefa 40).md, 44 - Correção de bug crítico de confirmação de matrícula via consulta de cobrança AppyPay (auditoria pós-tarefa 42).md, 45 - Pendências ambientais da tarefa 44 (checklist e publicação de PR).md"
---

# Fechamento administrativo pós-auditoria das tarefas 41, 42, 44 e 45 (nenhum bug encontrado)

## Prompt recomendado para executar esta tarefa

```
Leia por completo o arquivo "docs/Lista de Tarefas/46 - Fechamento administrativo pos-auditoria das
tarefas 41, 42, 44 e 45 (nenhum bug encontrado).md". Ele não contém nenhuma correção de código — uma
auditoria completa já confirmou que o código das tarefas 41 e 44 está correto. O documento contém apenas
comandos de validação (para você reconfirmar no seu próprio ambiente) e instruções mecânicas de arquivo
(mover, renomear, editar front-matter). Não há nenhuma decisão de design a tomar. Execute a seção "Passo
1" até o fim; se qualquer comando falhar com um resultado diferente do esperado (documentado ao lado de
cada comando), pare e reporte o erro exato — não prossiga para os passos seguintes nem tente investigar
ou corrigir nada por conta própria. Se todos os comandos do Passo 1 baterem com o resultado esperado,
prossiga mecanicamente pelos Passos 2 a 6, na ordem, e então confirme os "Critérios de aceite" ao final.
```

## Contexto

O Fredy pediu uma depuração profunda do módulo de pagamento para confirmar que as tarefas 41 (redesign da
autenticação de webhook AppyPay) e 44 (correção de confirmação de matrícula via consulta de cobrança) foram
corretamente implementadas, já que o Codex, em ambas as execuções, não teve acesso a PostgreSQL/`psql` e por
isso não conseguiu concluir a etapa de validação de integração de nenhuma das duas — ambos os documentos
originais ficaram com a "Nota de validação" incompleta, `status: pendente`, e sem serem movidos para
`docs/Tarefas feitas/`, exatamente como a tarefa 45 já registrava para o caso da tarefa 44.

Esta auditoria foi conduzida em ambiente real: repositório `spuri-backend` clonado do zero a partir de
`https://github.com/fredypdp/spuri-backend` (branch `main`, HEAD em `e29ea77`, que já inclui os merges das
PRs #540 — tarefa 44 — e #541 — tarefa 41), com Go 1.24 e PostgreSQL 16 instalados e configurados
localmente para rodar as suítes de integração de verdade.

**Resultado da auditoria, resumido:**

1. **Tarefa 41** — todas as 9 seções do documento (`internal/finance/appypay.go`,
   `internal/handlers/financeiro_handlers.go`, `cmd/server/main.go`, `internal/db/safe_queries.go`,
   `internal/projections/financeiro_projection.go`, os 3 arquivos de teste, e as 2 documentações) foram
   conferidas uma a uma contra o código real do repositório. Todos os 18 critérios de aceite do documento
   batem exatamente. `gofmt -l .`, `go build ./...` e `go vet ./...` passam sem nenhuma saída. `go test
   ./...` (sem integração) passa em todos os pacotes.
2. **Passo obrigatório de investigação da tarefa 41** (nunca antes executado, por falta de
   PostgreSQL nas sessões do Codex) — executado nesta auditoria: com `RUN_POSTGRES_INTEGRATION=1` e banco
   recriado do zero, a suíte completa de `internal/finance` e de `internal/handlers` foi executada tanto no
   estado atual do `main` (com a tarefa 41 aplicada) quanto num checkout do commit imediatamente anterior à
   tarefa 41 (`e8cfffe`, ou seja, `main` com a tarefa 44 já aplicada mas sem a 41) — **as duas execuções
   terminaram 100% verdes, sem nenhum `FAIL`, incluindo `TestIntegrationReceberWebhookAppyPayEfetivaVinculoMatricula`
   e todos os cenários de mensalidade.** Não há nenhuma falha pré-existente no `main` para reportar ao Fredy
   separadamente — o item 2 do "Como concluir esta pendência" da tarefa 41 fica satisfeito com "nenhuma
   falha encontrada".
   - Nota honesta sobre um falso positivo desta própria auditoria: numa primeira tentativa, a suíte de
     `internal/finance` foi executada sem a variável `APPYPAY_RESOURCE` definida no ambiente do processo
     (só via `t.Setenv` dentro de alguns testes), e isso fez `TestIntegrationPagamentoMensalidadeConfirmadoPelaAppyPayMarcaComoPago`
     falhar isoladamente com `APPYPAY_RESOURCE não configurada`. Ao repetir a execução com as variáveis de
     ambiente exatamente como especificado no checklist da tarefa 44 (`APPYPAY_RESOURCE=integration-resource`
     definida no processo do `go test`, não só em testes individuais), o teste passou normalmente — em ambos
     os lados da comparação (antes e depois da tarefa 41). Não é um bug do repositório; foi um erro de
     invocação desta auditoria, documentado aqui só para transparência.
3. **Tarefa 44** — o diff aplicado em `internal/handlers/financeiro_handlers.go` bate exatamente com o
   especificado (`ConsultarCobrancaAppyPay` agora chama `efetivarVinculoMatriculaPaga` quando a consulta
   revela `status: "success"`, usando `CodigoSolicitacaoDaCobranca` para resolver o código da solicitação).
   O arquivo de teste novo `internal/handlers/financeiro_matricula_consulta_test.go` existe com o conteúdo
   especificado. `git show --stat` do commit da tarefa 44 confirma que **apenas** esses 2 arquivos foram
   tocados, nenhum outro — inclusive `go.mod`/`go.sum` continuam intactos.
4. **Checklist de aceitação completo da tarefa 44** (nunca antes executado, mesmo motivo de ambiente) —
   executado nesta auditoria: `internal/handlers` e `internal/finance`, cada um rodado 5 vezes seguidas com
   banco recriado do zero a cada execução, usando exatamente os comandos do próprio checklist da tarefa 44
   — **as 10 execuções (5+5) terminaram `ok`, sem nenhum `FAIL`**, incluindo
   `TestIntegrationConsultarCobrancaAppyPayNaoEfetivaMatriculaAposSuccess` e
   `TestIntegrationReceberWebhookAppyPayEfetivaVinculoMatricula` em todas as 5 execuções de `internal/handlers`.
5. **Tarefa 42** — o próprio front-matter já está `status: feito` (confirmado corretamente pela tarefa 44),
   mas o arquivo nunca foi fisicamente movido para `docs/Tarefas feitas/`.
6. **Tarefa 45** — documentava 3 pendências ambientais da tarefa 44: (a) concluir o checklist de aceitação —
   **resolvido nesta auditoria** (item 4 acima); (b) mover as tarefas 44 e 42 para `docs/Tarefas feitas/` —
   será feito nesta tarefa (Passos 4 e 5 abaixo); (c) criar o pull request — **já resolvido**, o Fredy
   mesclou manualmente a PR #540 (tarefa 44) e a PR #541 (tarefa 41) diretamente no `main`, confirmado por
   `git log --graph` no repositório (merges em `e8cfffe` e `e29ea77`). As 3 pendências da tarefa 45 estão,
   portanto, encerradas.

**Conclusão da auditoria: não existe nenhum bug de código a corrigir no módulo de pagamento relacionado às
tarefas 41, 42, 44 ou 45.** Esta tarefa 46 não altera nenhum arquivo `.go`, `.sql` ou de configuração — é
puramente a execução mecânica da checklist (para você reconfirmar no seu próprio ambiente, já que a
auditoria acima foi feita num ambiente diferente do seu) seguida da limpeza documental que as próprias
tarefas 41, 44 e 45 já pediam e que nunca pôde ser concluída.

---

## Resumo executivo

| # | Ação | Arquivos afetados | Tipo |
|---|------|--------------------|------|
| 1 | Reconfirmar checklist de integração (tarefas 41 e 44) no seu ambiente | nenhum (só leitura/execução) | validação |
| 2 | Preencher a Nota de validação da tarefa 41 com o resultado real | `docs/Lista de Tarefas/41 - Redesign da autenticação de webhook AppyPay (secret gerado pelo servidor).md` | edição de documentação |
| 3 | Concluir a tarefa 41: status, título, mover para Tarefas feitas, remover handoff | idem + `docs/Lista de Tarefas/Handoff — Redesign da autenticação de webhook AppyPay (secret gerado pelo servidor).md` (remover) | mover/remover arquivo |
| 4 | Concluir a tarefa 44: status e mover para Tarefas feitas | `docs/Lista de Tarefas/44 - Correção de bug crítico de confirmação de matrícula via consulta de cobrança AppyPay (auditoria pós-tarefa 42).md` | mover arquivo |
| 5 | Mover a tarefa 42 (já `status: feito`) para Tarefas feitas | `docs/Lista de Tarefas/42 - Correção de bugs críticos de integridade do ledger no módulo de mensalidades e matrícula (auditoria pós-tarefa 40).md` | mover arquivo |
| 6 | Concluir a tarefa 45: status e mover para Tarefas feitas | `docs/Lista de Tarefas/45 - Pendências ambientais da tarefa 44 (checklist e publicação de PR).md` | mover arquivo |

**Nenhum arquivo `.go`, `.sql`, `go.mod` ou `go.sum` é criado, editado ou removido por esta tarefa.**

---

## Passo 1 — Reconfirmar o checklist de integração no seu ambiente

Estes comandos já foram executados com sucesso na auditoria (resultado documentado no Passo 2). Rode-os de
novo no seu próprio ambiente só para confirmar que o mesmo resultado se reproduz aqui — é o passo que as
duas sessões anteriores do Codex não conseguiram completar por falta de PostgreSQL.

### 1.1. Se `psql`/PostgreSQL não estiver disponível no seu ambiente

Se `which psql` não encontrar nada, instale e suba um PostgreSQL local (testado e funcional em ambiente
Ubuntu/Debian; ajuste conforme a distribuição do seu ambiente se for diferente):

```bash
apt-get update && apt-get install -y postgresql postgresql-contrib
service postgresql start
su postgres -c "psql -c \"ALTER USER postgres PASSWORD 'postgres';\""
su postgres -c "createdb spuri_test"
```

Confirme que `pg_hba.conf` permite conexão TCP em `127.0.0.1`/`localhost` com senha (`scram-sha-256` ou
`md5`) — a instalação padrão do pacote `postgresql` do Ubuntu já vem assim configurada por padrão; não é
necessário editar nada manualmente se você seguiu os comandos acima.

### 1.2. Confirmar build/vet/gofmt limpos

```bash
gofmt -l .
go build ./...
go vet ./...
go test ./...
```

**Resultado esperado (confirmado na auditoria):** as 4 saídas terminam sem nenhum erro; `gofmt -l .` não
lista nenhum arquivo; `go test ./...` termina com `ok` em todos os pacotes.

### 1.3. Suíte `internal/finance`, 5 execuções seguidas, banco recriado a cada vez

```bash
for i in 1 2 3 4 5; do
  psql -c "DROP DATABASE IF EXISTS spuri_test;" -U postgres -h localhost
  psql -c "CREATE DATABASE spuri_test;" -U postgres -h localhost
  RUN_POSTGRES_INTEGRATION=1 DB_HOST=localhost DB_PORT=5432 DB_USER=postgres DB_PASSWORD=postgres \
    DB_NAME=spuri_test DB_SSLMODE=disable APPYPAY_RESOURCE=integration-resource \
    FINANCE_ENCRYPTION_KEY=01234567890123456789012345678901 ENV=test \
    go test -count=1 ./internal/finance/... -run TestIntegration -v
done
```

**Resultado esperado (confirmado na auditoria):** as 5 execuções terminam `PASS`/`ok`, sem nenhum `FAIL`.
Confirme especificamente que `TestIntegrationWebhookSecretGeneratedOnceGlobalHeaderAndRotation` e
`TestIntegrationPagamentoMensalidadeConfirmadoPelaAppyPayMarcaComoPago` aparecem como `--- PASS` em cada
execução.

### 1.4. Suíte `internal/handlers`, 5 execuções seguidas, banco recriado a cada vez

```bash
for i in 1 2 3 4 5; do
  psql -c "DROP DATABASE IF EXISTS spuri_test;" -U postgres -h localhost
  psql -c "CREATE DATABASE spuri_test;" -U postgres -h localhost
  RUN_POSTGRES_INTEGRATION=1 DB_HOST=localhost DB_PORT=5432 DB_USER=postgres DB_PASSWORD=postgres \
    DB_NAME=spuri_test DB_SSLMODE=disable APPYPAY_RESOURCE=integration-resource \
    FINANCE_ENCRYPTION_KEY=01234567890123456789012345678901 ENV=test \
    go test -count=1 ./internal/handlers/... -run TestIntegration -v
done
```

**Resultado esperado (confirmado na auditoria):** as 5 execuções terminam `PASS`/`ok`, sem nenhum `FAIL`.
Confirme especificamente que `TestIntegrationConsultarCobrancaAppyPayNaoEfetivaMatriculaAposSuccess` e
`TestIntegrationReceberWebhookAppyPayEfetivaVinculoMatricula` aparecem como `--- PASS` em cada execução.

### 1.5. Critérios de limpeza de código (rápidos, sem banco)

```bash
grep -rn "validHTTPHeaderName\|defaultWebhookHeaderName" --include="*.go" .
grep -rn "webhook_auth_type\|WebhookAuthType\|webhook_username\|WebhookUsername" --include="*.go" .
grep -n "X-API-Key" "Documentação da API.md" "docs/Parceiros e integrações/AppyPay Documentação.md"
```

**Resultado esperado (confirmado na auditoria):** as 3 buscas não retornam nenhuma linha.

**Se qualquer um dos comandos acima (1.2 a 1.5) produzir um resultado diferente do documentado — qualquer
`FAIL`, qualquer saída de erro, ou qualquer linha retornada pelas buscas do item 1.5 — pare aqui e reporte
o erro exato, com o output completo do comando que falhou. Não prossiga para o Passo 2 nem tente corrigir
nada por conta própria: isso significaria que algo mudou no repositório desde esta auditoria (16 de agosto
de 2026, commit `e29ea77`), e precisa ser investigado antes de fechar as tarefas.**

Se todos os itens do Passo 1 baterem com o resultado esperado, prossiga para o Passo 2.

---

## Passo 2 — Preencher a Nota de validação da tarefa 41

**Arquivo:** `docs/Lista de Tarefas/41 - Redesign da autenticação de webhook AppyPay (secret gerado pelo servidor).md`

**Localizar** (o bloco completo da Nota de validação atual, incluindo o cabeçalho da seção):

```
## Nota de validação

Execução Codex em 2026-08-16:

- `gofmt -l .`: passou, sem listar arquivos.
- `go build ./...`: passou.
- `go vet ./...`: passou.
- `go test ./...`: passou sem `RUN_POSTGRES_INTEGRATION=1`.
- Critério de limpeza `rg -n "validHTTPHeaderName|defaultWebhookHeaderName" --glob '*.go' .`: passou, sem resultados.
- Critério de limpeza `rg -n "webhook_auth_type|WebhookAuthType|webhook_username|WebhookUsername" --glob '*.go' .`: passou, sem resultados.
- Inspeção de `X-API-Key` em `Documentação da API.md` e `docs/Parceiros e integrações/AppyPay Documentação.md`: passou, sem resultados nas duas documentações.

Validação de integração e passo obrigatório de investigação: **não concluídos neste ambiente**. O cliente `psql` não está disponível (`psql: command not found`) e o PostgreSQL esperado em `localhost:5432` recusou conexão (`dial tcp [::1]:5432: connect: connection refused`). A tentativa de executar o teste novo isolado com `RUN_POSTGRES_INTEGRATION=1 ... go test ./internal/finance/... -run TestIntegrationWebhookSecretGeneratedOnceGlobalHeaderAndRotation -v` falhou antes de validar o cenário pelo mesmo motivo de ambiente. Portanto, a comparação exigida entre `main` puro e branch alterada, bem como as suítes completas de integração `internal/finance` e `internal/handlers`, ainda precisam ser executadas em ambiente com PostgreSQL/`psql` disponíveis antes de mover esta tarefa para `docs/Tarefas feitas/`.

Diferente das tarefas anteriores deste módulo (24 e 30), o código desta tarefa **não foi revalidado com `go build`/`go test` reais** durante a sessão de orquestração que escreveu este documento — o ambiente disponível não tinha Go nem PostgreSQL, apenas leitura do repositório via `codeload.github.com`/`api.github.com`. O desenho foi revisado cuidadosamente, linha a linha, contra o estado real e atual (agosto de 2026) de todos os arquivos citados, mas a validação mecânica de compilação e testes — incluindo a investigação de falhas pré-existentes acima — fica inteiramente a cargo de quem executar esta tarefa.
```

**Substituir por:**

```
## Nota de validação

Execução Codex em 2026-08-16 (implementação do código):

- `gofmt -l .`: passou, sem listar arquivos.
- `go build ./...`: passou.
- `go vet ./...`: passou.
- `go test ./...`: passou sem `RUN_POSTGRES_INTEGRATION=1`.
- Critério de limpeza `rg -n "validHTTPHeaderName|defaultWebhookHeaderName" --glob '*.go' .`: passou, sem resultados.
- Critério de limpeza `rg -n "webhook_auth_type|WebhookAuthType|webhook_username|WebhookUsername" --glob '*.go' .`: passou, sem resultados.
- Inspeção de `X-API-Key` em `Documentação da API.md` e `docs/Parceiros e integrações/AppyPay Documentação.md`: passou, sem resultados nas duas documentações.

Auditoria de fechamento (Claude, 16 de agosto de 2026, tarefa 46) — passo obrigatório de investigação e
validação de integração, ambiente real (Go 1.24, PostgreSQL 16, repositório clonado do `main`, commit
`e29ea77`):

- Todas as 9 seções do documento conferidas linha a linha contra o código real do repositório: batem
  exatamente. Os 18 critérios de aceite passam.
- Com `RUN_POSTGRES_INTEGRATION=1` e banco recriado do zero, a suíte completa de `internal/finance` e de
  `internal/handlers` foi executada tanto no `main` com esta tarefa aplicada quanto num checkout do commit
  imediatamente anterior a ela (`e8cfffe`) — **nas duas execuções, 100% dos testes passaram, sem nenhum
  `FAIL`**, incluindo `TestIntegrationReceberWebhookAppyPayEfetivaVinculoMatricula` e todos os cenários de
  mensalidade. Não há nenhuma falha pré-existente no `main` para reportar separadamente ao Fredy.
- `TestIntegrationWebhookSecretGeneratedOnceGlobalHeaderAndRotation` e as demais suítes de webhook passam
  isoladamente e dentro da suíte completa, 5 execuções seguidas contra banco recriado a cada vez.
- Reconfirmado neste ambiente (Codex) na execução do Passo 1 da tarefa 46: [PREENCHER — "confirmado, mesmo
  resultado" ou detalhar qualquer divergência encontrada].
```

Depois de colar o texto acima, substitua o trecho `[PREENCHER — ...]` pelo resultado real da sua própria
execução do Passo 1 desta tarefa 46 (ex.: `"confirmado, mesmo resultado da auditoria — todas as 10
execuções de integração (5 finance + 5 handlers) terminaram ok, sem nenhum FAIL"`).

---

## Passo 3 — Concluir a tarefa 41

**Arquivo:** `docs/Lista de Tarefas/41 - Redesign da autenticação de webhook AppyPay (secret gerado pelo servidor).md`

1. **Localizar** no front-matter:
   ```
   status: pendente
   ```
   **Substituir por:**
   ```
   status: feito
   ```

2. **Atenção — ambiguidade de texto:** a string `# Redesign da autenticação de webhook AppyPay (secret
   gerado pelo servidor)` aparece **duas vezes** no arquivo: a primeira é o título real (linha 7, logo
   após o front-matter); a segunda é só uma citação dentro da instrução do item 3 do "Procedimento de
   conclusão" (perto do fim do arquivo, dentro de um trecho entre crases que já tem `(feito)` no final —
   não mexa nessa segunda ocorrência, ela já está correta e é só texto instrucional). Para evitar editar a
   ocorrência errada, **localize especificamente** o bloco de 3 linhas abaixo (título + linha em branco +
   início da seção seguinte), que só existe uma vez no arquivo — logo no topo, depois do front-matter:
   ```
   # Redesign da autenticação de webhook AppyPay (secret gerado pelo servidor)

   ## Prompt recomendado para executar esta tarefa
   ```
   **Substituir por:**
   ```
   # Redesign da autenticação de webhook AppyPay (secret gerado pelo servidor) (feito)

   ## Prompt recomendado para executar esta tarefa
   ```

3. **Mover** o arquivo de
   `docs/Lista de Tarefas/41 - Redesign da autenticação de webhook AppyPay (secret gerado pelo servidor).md`
   para
   `docs/Tarefas feitas/41 - Redesign da autenticação de webhook AppyPay (secret gerado pelo servidor).md`.

4. **Remover** o arquivo
   `docs/Lista de Tarefas/Handoff — Redesign da autenticação de webhook AppyPay (secret gerado pelo servidor).md`
   — seu conteúdo já está totalmente incorporado à tarefa 41 (esta é a própria instrução original do
   "Procedimento de conclusão" da tarefa 41, item 6; não é uma decisão nova desta tarefa 46).

---

## Passo 4 — Concluir a tarefa 44

**Arquivo:** `docs/Lista de Tarefas/44 - Correção de bug crítico de confirmação de matrícula via consulta de cobrança AppyPay (auditoria pós-tarefa 42).md`

1. **Localizar** no front-matter:
   ```
   status: pendente
   ```
   **Substituir por:**
   ```
   status: feito
   ```

2. **Mover** o arquivo de
   `docs/Lista de Tarefas/44 - Correção de bug crítico de confirmação de matrícula via consulta de cobrança AppyPay (auditoria pós-tarefa 42).md`
   para
   `docs/Tarefas feitas/44 - Correção de bug crítico de confirmação de matrícula via consulta de cobrança AppyPay (auditoria pós-tarefa 42).md`.

   (O documento original desta tarefa não pede alteração do título com sufixo `(feito)`, só do
   front-matter — mantenha como está para não divergir do padrão que a própria tarefa definiu para si.)

---

## Passo 5 — Mover a tarefa 42 para Tarefas feitas

**Arquivo:** `docs/Lista de Tarefas/42 - Correção de bugs críticos de integridade do ledger no módulo de mensalidades e matrícula (auditoria pós-tarefa 40).md`

Esta tarefa já está com `status: feito` no front-matter (confirmado corretamente pela auditoria que originou
a tarefa 44) — falta só mover o arquivo, que ficou para trás porque a sessão anterior do Codex não tinha
PostgreSQL disponível para confirmar o checklist antes de mover.

1. **Mover** o arquivo de
   `docs/Lista de Tarefas/42 - Correção de bugs críticos de integridade do ledger no módulo de mensalidades e matrícula (auditoria pós-tarefa 40).md`
   para
   `docs/Tarefas feitas/42 - Correção de bugs críticos de integridade do ledger no módulo de mensalidades e matrícula (auditoria pós-tarefa 40).md`.

   Não altere o conteúdo deste arquivo — só o local. (Se notar, dentro do próprio texto da tarefa 42, uma
   referência a mover "a tarefa 41" para `Tarefas feitas` — isso é um erro de digitação pré-existente do
   próprio documento, sobra de uma numeração antiga antes de existir a atual tarefa 41 de webhook; ignore
   essa frase interna e mova o arquivo 42 como instruído aqui.)

---

## Passo 6 — Concluir a tarefa 45

**Arquivo:** `docs/Lista de Tarefas/45 - Pendências ambientais da tarefa 44 (checklist e publicação de PR).md`

As 3 pendências que este documento registrava estão resolvidas: (1) o checklist de aceitação da tarefa 44
foi concluído no Passo 1 desta tarefa 46; (2) as tarefas 44 e 42 foram movidas para `Tarefas feitas` nos
Passos 4 e 5; (3) as PRs #540 (tarefa 44) e #541 (tarefa 41) já foram mescladas manualmente pelo Fredy
diretamente no `main` — não é mais necessário criar nenhum PR.

1. **Localizar** no front-matter:
   ```
   status: pendente
   ```
   **Substituir por:**
   ```
   status: feito
   ```

2. **Localizar** a seção final do documento, a partir de:
   ```
   ## Como concluir esta pendência
   ```
   até o fim do arquivo (incluindo os 3 itens numerados que vêm depois).

   **Substituir por:**
   ```
   ## Como concluir esta pendência

   Resolvido pela tarefa 46 (fechamento administrativo pós-auditoria) em 16 de agosto de 2026:

   1. O checklist completo da tarefa 44 foi executado com sucesso (5 execuções de `internal/handlers` e 5
      de `internal/finance`, banco recriado do zero a cada vez, todas verdes).
   2. As tarefas 44 e 42 foram movidas para `docs/Tarefas feitas/` com `status: feito`.
   3. O PR não precisou ser criado por ferramenta: o Fredy mesclou manualmente a PR #540 (tarefa 44) e a
      PR #541 (tarefa 41) diretamente no `main` (confirmável em `git log --graph`, merges nos commits
      `e8cfffe` e `e29ea77`).
   ```

3. **Mover** o arquivo de
   `docs/Lista de Tarefas/45 - Pendências ambientais da tarefa 44 (checklist e publicação de PR).md`
   para
   `docs/Tarefas feitas/45 - Pendências ambientais da tarefa 44 (checklist e publicação de PR).md`.

---

## Fora de escopo

- Nenhum arquivo `.go`, `.sql`, `go.mod` ou `go.sum` deve ser criado, editado ou removido por esta tarefa.
- Não mexer na tarefa 43 (`docs/Lista de Tarefas/43 - Implementar os testes de regressao ainda ausentes das Tarefas 38 e 39...md`) — não relacionada ao módulo de pagamento/matrícula, fora do escopo desta auditoria.
- Não mexer no documento `docs/Lista de Tarefas/Problemas de Backend - Modulo de Pagamentos.md` — é um levantamento de gaps para uma futura tarefa de frontend, ainda `status: pendente de virar tarefa`; não faz parte do fechamento das tarefas 41/42/44/45.
- Não criar nenhum PR novo — as alterações desta tarefa são só documentação; pode ser commitado diretamente ou incluído no próximo PR que o Fredy já for abrir, a critério dele.

---

## Critérios de aceite

1. Todos os comandos do Passo 1 terminam exatamente com o resultado documentado ao lado de cada um — sem
   nenhum `FAIL`, sem nenhuma saída de erro, sem nenhuma linha retornada pelas 3 buscas do item 1.5.
2. `git status --short` mostra apenas movimentações de arquivo (`R` ou `D`+`A` conforme o cliente git) e
   edições de conteúdo dentro dos arquivos `.md` listados no Resumo executivo — nenhum arquivo `.go`,
   `.sql`, `go.mod` ou `go.sum` aparece na saída.
3. `docs/Lista de Tarefas/` não contém mais nenhum dos arquivos: `41 - Redesign da autenticação...md`,
   `42 - Correção de bugs críticos...md`, `44 - Correção de bug crítico...md`,
   `45 - Pendências ambientais...md`, `Handoff — Redesign da autenticação...md`.
4. `docs/Tarefas feitas/` contém os 4 arquivos movidos (41, 42, 44, 45), cada um com `status: feito` no
   front-matter.
5. A Nota de validação da tarefa 41 (agora em `docs/Tarefas feitas/`) não contém mais nenhuma menção a
   "não concluído" ou a erro de ambiente — reflete o resultado real e completo da validação.

Se todos os itens acima forem confirmados, esta tarefa 46 está concluída; não é necessário movê-la para
`docs/Tarefas feitas/` nem alterar seu próprio front-matter — ela é uma tarefa de fechamento, não uma
funcionalidade, mas fique à vontade para movê-la também, se preferir manter o padrão do repositório.
