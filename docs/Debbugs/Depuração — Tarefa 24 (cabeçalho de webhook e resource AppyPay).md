---
criado: 2026-08-11 00:00
origem: Depuração da tarefa `docs/Tarefas feitas/24 - Cabeçalho de webhook configurável e resource AppyPay via variável de ambiente.md` já implementada, por Claude (orquestrador)
status: pendente
---

# Depuração — Tarefa 24 (cabeçalho de webhook configurável e resource AppyPay via variável de ambiente)

## Prompt recomendado para executar a correção

Corrija exclusivamente os três pontos descritos neste documento em `internal/finance/appypay_integration_test.go` e em `docs/Parceiros e integrações/AppyPay Documentação.md`. Não altere `internal/finance/appypay.go`, `internal/handlers/financeiro_handlers.go`, `cmd/server/main.go`, `.env.example` nem `Documentação da API.md` — esses arquivos foram auditados e estão corretos, e mudanças neles não fazem parte desta correção. Não toque em nenhum outro teste do repositório.

## Resultado da depuração

Cloneei o repositório, apliquei `go build ./...`, `go vet ./...` e rodei a suíte de testes inteira (`go test ./...`) com Go 1.24 (o exigido pelo `go.mod`), incluindo os testes de integração com `RUN_POSTGRES_INTEGRATION=1` contra um PostgreSQL real.

**O código de produção está correto.** `internal/finance/appypay.go`, `internal/handlers/financeiro_handlers.go` e `cmd/server/main.go` batem exatamente com a Seção 3 da tarefa 24: `CredentialInput`/`CredentialView` sem `Resource`/`ResourceMask` e com `WebhookHeaderName`; `AuthenticateWebhook` recebendo `http.Header` completo com fallback para `X-API-Key`; `token()` usando `appyPayResource()`; `ValidateAppyPayResourceConfig()` chamada em `initDB()`. `.env.example` e `Documentação da API.md` (seções 19.1, 19.2, 19.3, 19.7, 19.8) também estão corretos. `go build ./...` e `go vet ./...` passam sem nenhum erro no repositório inteiro.

**Os problemas encontrados estão isolados em `internal/finance/appypay_integration_test.go` (um teste com defeito, não o código que ele testa) e numa referência quebrada em `docs/Parceiros e integrações/AppyPay Documentação.md`.**

---

# 1. Bug confirmado — `TestIntegrationWebhookAuthConfigurableHeaderAndResourceFreeCredentials` falha por construção incorreta de `http.Header`

## O que está a acontecer

O teste constrói `http.Header` diretamente como map literal, por exemplo:

```go
owner, err = service.AuthenticateWebhook(ctx, "", "", http.Header{"X-API-Key": []string{"legacy-webhook-secret"}})
```

Isto **não** é equivalente a como o Gin/`net/http` populam `c.Request.Header` numa requisição real. `http.Header.Get(chave)` canonicaliza a **chave da consulta** internamente (`textproto.CanonicalMIMEHeaderKey`) antes de procurar no mapa — mas um map literal escrito à mão não canonicaliza as suas próprias chaves. Comprovei isto isoladamente:

```go
raw := http.Header{"X-API-Key": []string{"segredo"}}
raw.Get("X-API-Key") // retorna "" — a chave real no mapa é "X-API-Key", mas Get() procura por "X-Api-Key"

proper := http.Header{}
proper.Set("X-API-Key", "segredo")
proper.Get("X-API-Key") // retorna "segredo" — Set() já grava sob a chave canônica "X-Api-Key"
```

**Isto não é um bug no código de produção.** Requisições HTTP reais são sempre parseadas pela biblioteca padrão do Go, que canonicaliza os nomes de cabeçalho automaticamente — por isso `AuthenticateWebhook`/`ReceberWebhookAppyPay` funcionam corretamente com webhooks reais da AppyPay, seja qual for a capitalização que a AppyPay use no cabeçalho. O problema é exclusivamente a forma como o teste simula esse `http.Header` à mão.

Confirmei rodando o teste antes e depois da correção: falha exatamente no cenário de fallback para `X-API-Key` (linha ~120 do arquivo atual) com o erro `webhook não autenticado`, porque `headers.Get("X-API-Key")` (chamado dentro de `AuthenticateWebhook`) não encontra a chave gravada como literal `"X-API-Key"` no mapa do teste.

## Correção (já validada — aplique exatamente assim)

Localizar as três construções de `http.Header` por map literal no arquivo e substituir cada uma por `http.Header{}` + `.Set(...)`:

**Linha com `"X-Spuri-Webhook-Secret"` (cabeçalho customizado):**

```go
owner, err := service.AuthenticateWebhook(ctx, "", "", http.Header{"X-Spuri-Webhook-Secret": []string{"custom-webhook-secret"}})
if err != nil || owner.CredentialID != custom.ID {
	t.Fatalf("cabeçalho customizado não autenticou: owner=%#v err=%v", owner, err)
}
if _, err = service.AuthenticateWebhook(ctx, "", "", http.Header{"X-API-Key": []string{"custom-webhook-secret"}}); err == nil {
	t.Fatal("X-API-Key autenticou credencial configurada para cabeçalho customizado")
}
```

Substituir por:

```go
customHeaders := http.Header{}
customHeaders.Set("X-Spuri-Webhook-Secret", "custom-webhook-secret")
owner, err := service.AuthenticateWebhook(ctx, "", "", customHeaders)
if err != nil || owner.CredentialID != custom.ID {
	t.Fatalf("cabeçalho customizado não autenticou: owner=%#v err=%v", owner, err)
}
wrongHeaders := http.Header{}
wrongHeaders.Set("X-API-Key", "custom-webhook-secret")
if _, err = service.AuthenticateWebhook(ctx, "", "", wrongHeaders); err == nil {
	t.Fatal("X-API-Key autenticou credencial configurada para cabeçalho customizado")
}
```

**Linha com o fallback legado `"X-API-Key"`:**

```go
owner, err = service.AuthenticateWebhook(ctx, "", "", http.Header{"X-API-Key": []string{"legacy-webhook-secret"}})
```

Substituir por:

```go
legacyHeaders := http.Header{}
legacyHeaders.Set("X-API-Key", "legacy-webhook-secret")
owner, err = service.AuthenticateWebhook(ctx, "", "", legacyHeaders)
```

A quarta ocorrência, `http.Header{}` vazio no cenário de Basic Auth (linha ~140), já está correta — não precisa de nenhuma mudança, pois nenhum cabeçalho é necessário ali (a autenticação passa por `basicUser`/`basicPassword`, não por `headers`).

---

# 2. Robustez — o teste deixa linhas reais na base de dados com segredos fixos, o que pode causar falso negativo se a suíte for rodada mais de uma vez contra o mesmo Postgres

## O que está a acontecer

`TestIntegrationWebhookAuthConfigurableHeaderAndResourceFreeCredentials` cria linhas reais em `financeiro_credenciais_appypay`/`financeiro_segredos_appypay` (via `ConfigureCredential` e via `INSERT` direto para o cenário legado) e nunca as remove no fim do teste. Os códigos de academia já são únicos por execução (`"INT" + uuid.NewString()[:8]`), mas os **valores de segredo são fixos**: `"custom-webhook-secret"`, `"legacy-webhook-secret"`, `"basic-secret"`.

Reproduzi o problema: rodei a suíte de integração duas vezes seguidas contra o mesmo PostgreSQL (sem recriar a base entre as execuções, exatamente como pode acontecer num ambiente de desenvolvimento local). Na segunda execução, `TestIntegrationWebhookAuthConfigurableHeaderAndResourceFreeCredentials` falhou porque `AuthenticateWebhook` — cuja consulta SQL não tem `ORDER BY` e retorna todas as credenciais `api_key`/`basic` cadastradas — encontrou a linha da **primeira** execução (com o mesmo segredo `"custom-webhook-secret"`) antes da linha recém-criada nesta segunda execução, e o teste comparou `owner.CredentialID` com o `ID` da credencial errada.

Contra uma base de dados nova a cada execução (como normalmente acontece em CI), isto não se manifesta. Mas é uma fragilidade real: qualquer reexecução local da suíte contra o mesmo Postgres sem recriar a base pode gerar uma falha que não tem relação nenhuma com o código, e sim com sujeira deixada por execuções anteriores.

## Correção

Tornar os valores de segredo únicos por execução, do mesmo jeito que os códigos de academia já são. Adicionar, no início da função de teste (logo após `t.Setenv("FINANCE_ENCRYPTION_KEY", ...)`), uma variável de sufixo compartilhada:

```go
suffix := uuid.NewString()[:8]
```

E usar esse `suffix` para tornar únicos:

- `WebhookSecret: "custom-webhook-secret"` → `WebhookSecret: "custom-webhook-secret-" + suffix`, e a mesma string em `customHeaders.Set("X-Spuri-Webhook-Secret", ...)` e em `wrongHeaders.Set("X-API-Key", ...)` (Seção 1).
- `"webhook_secret": "legacy-webhook-secret"` (dentro do `map[string]string` passado a `saveSecrets`) → `"legacy-webhook-secret-" + suffix`, e a mesma string em `legacyHeaders.Set("X-API-Key", ...)` (Seção 1).
- `WebhookUsername: "basic-user"` → `"basic-user-" + suffix`; `WebhookSecret: "basic-secret"` → `"basic-secret-" + suffix`; e os mesmos valores nos dois argumentos correspondentes da chamada final `service.AuthenticateWebhook(ctx, "basic-user", "basic-secret", http.Header{})`.

Não é necessário limpar as linhas inseridas no fim do teste (isso exigiria apagar de duas tabelas e não traria benefício adicional depois desta mudança) — a unicidade do segredo já é suficiente para eliminar a colisão entre execuções, incluindo qualquer linha antiga já deixada no banco de desenvolvimento por execuções anteriores a esta correção.

---

# 3. Referência quebrada em `docs/Parceiros e integrações/AppyPay Documentação.md`

## O que está a acontecer

A linha 8249 (nota corrigida pela própria tarefa 24) termina com:

> Se uma integração real revelar um `resource` diferente por academia, esta suposição precisa ser revista — ver `docs/Tarefas feitas/21 - ...md`.

Esse caminho não existe. A tarefa foi concluída e salva como `docs/Tarefas feitas/24 - Cabeçalho de webhook configurável e resource AppyPay via variável de ambiente.md` (o número 21 já estava ocupado por outra tarefa quando esta foi executada).

## Correção

Substituir:

```
— ver `docs/Tarefas feitas/21 - ...md`.
```

por:

```
— ver `docs/Tarefas feitas/24 - Cabeçalho de webhook configurável e resource AppyPay via variável de ambiente.md`.
```

---

# Critérios de aceite

1. As três construções de `http.Header` corrigidas na Seção 1 usam `.Set()`, não map literal.
2. Todos os valores de segredo usados em `TestIntegrationWebhookAuthConfigurableHeaderAndResourceFreeCredentials` (cabeçalho customizado, fallback legado, Basic Auth) incluem o sufixo único `suffix := uuid.NewString()[:8]` gerado no início da função.
3. A referência a `docs/Tarefas feitas/21 - ...md` em `docs/Parceiros e integrações/AppyPay Documentação.md` (linha 8249) aponta para o arquivo correto, `24 - Cabeçalho de webhook configurável e resource AppyPay via variável de ambiente.md`.
4. `go build ./...` e `go vet ./...` continuam a passar sem erro no repositório inteiro.
5. `go test ./...` passa sem `RUN_POSTGRES_INTEGRATION=1`.
6. Com `RUN_POSTGRES_INTEGRATION=1` e PostgreSQL disponível, `go test ./internal/finance/... -v -run TestIntegration` passa — rode a suíte de integração **duas vezes seguidas contra o mesmo banco, sem recriar a base entre as execuções**, para confirmar que a correção da Seção 2 realmente elimina a colisão entre execuções.
7. Nenhum arquivo além de `internal/finance/appypay_integration_test.go` e `docs/Parceiros e integrações/AppyPay Documentação.md` é alterado.

## Nota de validação (Claude, antes de entregar esta correção)

Apliquei a correção da Seção 1 sobre o arquivo real do repositório e confirmei: com banco limpo, o teste passa; provei isoladamente com um programa Go mínimo que `http.Header{"X-API-Key": ...}` como map literal realmente não é encontrado por `.Get("X-API-Key")`, enquanto `.Set()` resolve. Também reproduzi o problema da Seção 2 de propósito, rodando a suíte de integração duas vezes seguidas contra o mesmo Postgres sem recriar a base — a segunda execução falhou exatamente pelo motivo descrito, confirmando que não é um falso alarme. Depois de recriar o banco do zero e validar só a correção da Seção 1, toda a suíte (`go build ./...`, `go vet ./...`, `go test ./...` completo, incluindo os testes de integração) passou sem nenhuma falha. Não apliquei a correção da Seção 2 no meu ambiente de validação — descrevi-a mas não a testei fisicamente; o Codex deve validar o critério de aceite 6 (duas execuções seguidas sem recriar o banco) para confirmar.

## Procedimento de conclusão

Este é um documento de depuração avulso, não uma tarefa numerada da `Lista de Tarefas`. Depois de aplicar e validar a correção, mova este arquivo para `docs/Debbugs/` (se ainda não estiver lá) e apague-o do lugar onde foi colocado inicialmente, mantendo apenas a cópia final em `docs/Debbugs/`.
