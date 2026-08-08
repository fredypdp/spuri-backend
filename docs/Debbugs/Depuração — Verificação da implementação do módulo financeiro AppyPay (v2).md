---
criado: 2026-08-08
origem: Depuração solicitada pelo Fredy sobre a implementação da tarefa `docs/Lista de Tarefas/17 - Modulo base de gestao financeira com AppyPay.md`, feita pelo codex (parou por falta de tokens; foi seguida de dois commits manuais de correção na migration).
---

# Depuração — Verificação da implementação do módulo financeiro AppyPay (v2)

## 1. Objectivo

Verificar se a implementação actual em `fredypdp/spuri-backend` (branch `main`, commit `063ceb6`) cumpre `docs/Lista de Tarefas/17 - Modulo base de gestao financeira com AppyPay.md` e não repete as falhas que levaram ao rollback total anterior (migration 099, ver `docs/Debbugs/Depuração — Rollback total do módulo financeiro AppyPay.md` e `docs/Debbugs/Auditoria e Plano de Melhoria — Modulo Financeiro AppyPay.md`).

## 2. Método e uma limitação importante

Revisão manual linha-a-linha de: `migrations/097–101_*.sql`, `internal/finance/appypay.go`, `internal/finance/appypay_test.go`, `internal/domain/aggregates/financeiro.go` (+ diff a `aggregate.go`), `internal/handlers/financeiro_handlers.go`, `internal/projections/financeiro_projection.go`, diffs a `cmd/server/main.go`, `cmd/server/main_test.go`, `internal/db/safe_queries.go`, `.env.example`, e a secção 19 (`Financeiro / AppyPay`) de `Documentação da API.md`. Comparação directa contra a documentação oficial da AppyPay (`docs/Parceiros e integrações/AppyPay Documentação.md`) e contra o histórico de auditorias anteriores do mesmo módulo.

⚠️ **Não consegui correr `go build ./... && go vet ./... && go test ./...` neste ambiente**: `go.mod` exige o toolchain `go1.24.12` e este sandbox não tem acesso a `proxy.golang.org` para o descarregar (só há Go 1.22 disponível localmente). A revisão abaixo é baseada em leitura cuidadosa do código e verificação cruzada de assinaturas/tipos usados (`db.AuditContext`, `AggregateRepository.SaveWithAudit`, etc.), não em compilação real. **O primeiro passo do codex tem de ser correr esses três comandos** — se algo não compilar ou algum teste falhar, isso tem prioridade sobre tudo o resto deste documento.

## 3. Resultado geral

🟡 **Implementação substancialmente correcta e claramente melhor que as duas tentativas anteriores** (a primeira nunca chegou a chamar a AppyPay de verdade — usava só `FakeProvider` — e tinha uma falha crítica de RBAC; foi por isso totalmente removida). Esta versão já:

- Chama a API real da AppyPay (token + `/charges` + `/qr-codes` + `GET /charges/{id}`), não um mock.
- Corrige a falha crítica de RBAC da ronda anterior (admin `gerente`/`adm` **não** consegue mais tocar no módulo financeiro — confirmado por leitura de `verificarPermissaoAdmin(c, "fpp")`).
- Nunca grava nem devolve segredos em claro (`client_secret`, tokens, API keys de webhook) — confirmado nos payloads de evento, na projecção pública e nas respostas da API.
- Segue Event Sourcing/CQRS como o resto do sistema (aggregate `Financeiro`, projecção `financeiro`, whitelist de eventos actualizada em `safe_queries.go`).
- Só implementa GPO, GPO QR Code e REF — nenhuma rota de reembolso/reversão/reconciliação existe, e há teste automático (`TestLegacyPaymentRoutesAreRemoved`) confirmando 404 nessas rotas antigas.

Mas **não está pronta para produção sem as correcções da secção 5** — há um problema de idempotência que precisa de ficar mais robusto antes de haver dinheiro real em jogo, e a documentação nova (secção 19 do `Documentação da API.md`) tem exemplos que contradizem o próprio código e vão induzir quem integrar em erro.

## 4. Confirmado correcto (não repetir estas regressões)

| # | Item verificado | Onde |
|---|---|---|
| 1 | RBAC financeiro exige `admin.Role == "fpp"` (hierarquia `fpp:3 > adm:2 > gerente:1`), não apenas `user_type == "admin"` | `financeAdminAllowed` → `verificarPermissaoAdmin(c, "fpp")` em `financeiro_handlers.go` |
| 2 | Academia autenticada nunca escolhe o próprio `contexto_tipo`/`codigo_academia` — o handler força para o do próprio token, mesmo que venha outra coisa no body/query | `authorizeFinanceScope` |
| 3 | Segredos nunca aparecem na projecção pública nem na resposta da API — só `*_mask` | `CredentialView`, payload gravado em `ConfigureCredential` |
| 4 | `sanitize()` é recursivo (map aninhado e slices) e remove qualquer chave com `secret`/`token`/`apikey`/`api_key`/`authorization` | `sanitize()` em `appypay.go` |
| 5 | Provider HTTP real (token OAuth2 + chamadas REST), não `FakeProvider` | `token()`, `callJSON()` |
| 6 | Selecção TEST/PROD centralizada e alinhada com a convenção já usada no resto do repo (`ENV == "production"`) | `AmbienteAtual()`/`EndpointsAtuais()` |
| 7 | Whitelist de eventos/aggregate do ledger actualizada (`ValidateEventType`/`validAggregateTypes`) | `internal/db/safe_queries.go` |
| 8 | Webhook idempotente por `event_id` via constraint `UNIQUE`/`PRIMARY KEY` na tabela dedicada, reserva-antes-de-processar (e remove a reserva se o registo no ledger falhar) | `financeiro_webhooks_recebidos`, `AcceptWebhook` |
| 9 | Rotas antigas fora do escopo actual (reembolso/reversão/activar/desactivar/testar/reconciliar) devolvem 404, com teste automático | `TestLegacyPaymentRoutesAreRemoved` em `main_test.go` |
| 10 | Erros passam pelo envelope padrão do sistema (`utils.RespondWith...`), não `gin.H{"error": err.Error()}` cru | `financeiro_handlers.go` |

## 5. Problemas a corrigir

### 5.1 🟠 Alto — Retentativa com `merchantTransactionId` repetido não é verdadeiramente idempotente e suja o ledger

**Onde:** `Service.CreateCharge`, `internal/finance/appypay.go` (~linha 232).

**O que acontece:** se um chamador reenviar o mesmo `merchantTransactionId` (ex.: timeout de rede, duplo clique, retry de aplicação), a única protecção é a constraint `UNIQUE` na coluna `merchant_transaction_id` da tabela de projecção `financeiro_cobrancas`. Isso *impede* uma segunda chamada real à AppyPay (o erro acontece na escrita da projecção do evento "solicitada", **antes** de `s.callJSON` ser chamado) — o que é bom — mas:

1. Fica um evento `CobrancaAppyPaySolicitada` **órfão** no `spuri_ledger`, associado a um `aggregate_id` novo que nunca vai ser reflectido em nenhuma projecção (porque a escrita da projecção falhou e não há retry desse passo específico).
2. O chamador recebe um erro genérico ("Este valor já está cadastrado", HTTP 400) em vez do resultado da cobrança original — não é o comportamento clássico de uma idempotency key (que devolve a resposta original), obriga o chamador a saber que deve fazer `GET /financeiro/appypay/cobrancas/{merchantTransactionId}` à parte para recuperar o estado real.

**Correcção recomendada:** antes de gravar qualquer evento, verificar explicitamente se já existe uma cobrança com o mesmo `merchant_transaction_id` (`SELECT` simples em `financeiro_cobrancas`, ou uma reserva com `INSERT ... ON CONFLICT DO NOTHING` num índice dedicado, no mesmo espírito do `AcceptWebhook`). Se já existir, devolver directamente o `ChargeResult` já persistido em vez de tentar gravar um novo evento e deixar a constraint rebentar tarde. Isto elimina os eventos órfãos e torna a idempotência explícita e testável (adicionar um teste equivalente a `TestGerarCobrancaConcorrenteReservaIdempotenciaAntesDoProvider`, que já existiu na tentativa anterior segundo a auditoria de 2026-07-31).

### 5.2 🟡 Médio — Exemplos errados na documentação nova (secção 19 do `Documentação da API.md`)

**a) `resource` documentado como URL, mas é um UUID.** Em `19.1 POST /financeiro/appypay/credenciais`, o exemplo mostra `"resource": "https://gwy-api-tst.appypay.co.ao/v2.0"`. Isto está errado: segundo a documentação oficial da AppyPay (`docs/Parceiros e integrações/AppyPay Documentação.md`, secção "Get a token"), `resource` é um **UUID** fornecido pela AppyPay (ex.: `2aed7612-de64-46b5-9e59-1f48f8902d14`), usado no pedido de token — não uma URL. Uma academia que copie este exemplo literalmente vai falhar a autenticação.

**b) `qrCodeType: "dynamic"` é inválido.** Em `19.5 POST /financeiro/appypay/qr-codes`, o exemplo usa `"qrCodeType": "dynamic"`, mas o código (`CreateGPOQRCode`) só aceita `"SINGLE"` (default) ou `"MULTIPLE"` (maiúsculas), exactamente como a AppyPay real — qualquer outro valor é rejeitado com erro de validação. Uma chamada seguindo este exemplo falha sempre.

**c) Tabela de "Erros comuns" promete `404` e `409` que o código nunca devolve.** A tabela no fim da secção 19 lista `404` para "credencial/cobrança inexistente" e `409` para "conflito/idempotência", mas nenhum handler ou função do serviço financeiro chama `utils.RespondWithNotFoundError` ou `utils.RespondWithConflictError` — confirmado por busca no código (`grep` não encontra nenhuma ocorrência em `internal/handlers/financeiro_handlers.go` nem `internal/finance/appypay.go`). Hoje, **todo** erro do serviço (cobrança não encontrada, credenciais não configuradas, falha de comunicação com a AppyPay, resposta HTTP não-2xx da AppyPay) é devolvido como `400 VALIDATION_ERROR` via `utils.RespondWithValidationError`, mesmo quando a causa não é uma validação do pedido do cliente mas sim uma falha do provider externo.

**Correcção recomendada:**
- Corrigir os exemplos `resource` e `qrCodeType` na documentação.
- Decidir e implementar uma diferenciação real de erro no `Service` (ex.: erros sentinela `ErrNotFound`, `ErrUpstream`, ou um tipo de erro com campo `Kind`), e mapear cada handler para o código HTTP certo: `404` (recurso não encontrado), `502`/`503` ou `RespondWithServiceUnavailable` (falha de comunicação com a AppyPay), `400` (validação genuína de input). Só depois disso a tabela de erros da documentação passa a estar correcta — ajustá-la de novo se a decisão final for diferente da descrita aqui.

### 5.3 🟡 Médio — Comparação de `paymentMethod` sensível a maiúsculas/minúsculas

**Onde:** `credentialSecrets.method()`, `appypay.go` (~linha 364).

```go
func (c credentialSecrets) method(requested string) (string, error) {
    requested = strings.ToUpper(strings.TrimSpace(requested))
    if requested == "GPO" || requested == c.GPO { ... }
    if requested == "REF" || requested == c.REF { ... }
```

`requested` é colocado em maiúsculas antes de comparar com `c.GPO`/`c.REF`, mas estes vêm da AppyPay tal como configurados pela academia (normalmente com o UUID em minúsculas, ex.: `GPO_53c70da3-1c88-...`). Se um chamador passar o identificador completo em vez do atalho curto (`"GPO"`/`"REF"`), a comparação falha por causa do `ToUpper` e a cobrança é rejeitada com "paymentMethod não contratado para esta conta", mesmo sendo o valor certo.

**Correcção recomendada:** normalizar os dois lados da comparação (`strings.EqualFold`) **ou** decidir e documentar explicitamente que a função base só aceita os atalhos `GPO`/`REF` — e validar isso mais cedo com uma mensagem de erro clara, em vez de deixar o identificador completo passar pela validação (`validateCharge` aceita `strings.HasPrefix(m, "GPO_")`) e falhar só depois, no `method()`.

### 5.4 🟡 Médio — `ConsultCharge` grava um evento novo no ledger em toda consulta, mesmo sem mudança de estado

**Onde:** `Service.ConsultCharge`, `appypay.go` (~linha 322).

Cada chamada a `GET /financeiro/appypay/cobrancas/:id` grava um evento `CobrancaAppyPayConsultada` no `spuri_ledger`, mesmo que o `status` devolvido pela AppyPay seja idêntico ao já guardado. Um consumidor que faça *polling* do estado de pagamento (razoável se o webhook falhar ou atrasar) vai encher o ledger — que é append-only e imutável — com eventos que não representam nenhuma mudança real de estado.

**Correcção recomendada:** comparar o `status`/resposta obtidos com o `payload` actual antes de chamar `s.record`; só gravar um novo evento quando algo relevante realmente mudar.

### 5.5 🟢 Baixo — `FINANCE_ENCRYPTION_KEY` só é validada na primeira utilização, não no arranque

`key()` (usada por `encrypt`/`decrypt`) falha correctamente se `FINANCE_ENCRYPTION_KEY` estiver vazia — mas só é chamada quando alguém tenta configurar/ler uma credencial pela primeira vez, não em `initDB`/`main`. Um deploy sem essa variável só é detectado nesse momento, não no arranque/health-check. A ronda de auditoria anterior tinha uma validação equivalente no arranque (`ValidateEncryptionConfig`) que se perdeu no rollback total.

**Correcção recomendada:** chamar `key()` (ou uma validação equivalente) em `initDB()`/`main()`, falhando o arranque cedo se a variável estiver ausente/vazia.

### 5.6 🟢 Baixo — Sem validação de robustez mínima da chave de cifra

`key()` aceita qualquer string não vazia como `FINANCE_ENCRYPTION_KEY` (mesmo algo curto como `"123"`), aplicando só SHA-256 sem verificar comprimento/entropia mínima.

**Correcção recomendada:** validar um comprimento mínimo razoável (ex.: ≥ 20–32 caracteres) antes de aceitar a chave, para reduzir o risco de um valor fraco em produção por engano.

### 5.7 🟡 Médio — Cobertura de testes ainda fina para um módulo financeiro

`internal/finance/appypay_test.go` tem só 3 testes (cifra/decifra, validação básica de `validateCharge`, `sanitize`). Faltam pelo menos:

- Teste de que o webhook processa o mesmo `event_id` duas vezes sem duplicar efeito (idempotência).
- Teste de isolamento entre academias (uma academia não consegue ler/consultar credencial ou cobrança de outra, nem do contexto `spuri`).
- Teste de RBAC confirmando que um admin `adm`/`gerente` é rejeitado nas rotas `/financeiro/*` (equivalente ao que a ronda de auditoria anterior tinha para o módulo antigo).
- Idealmente, um teste de integração da `FinanceiroProjection` contra Postgres real (não só funções puras) — é o único jeito de apanhar bugs de SQL/constraint como o item 5.1 antes de produção.

Isto era um critério de saída explícito da auditoria anterior (secção 6, "Auditoria e Plano de Melhoria — Módulo Financeiro (AppyPay)") e continua incompleto nesta implementação.

### 5.8 🟢 Baixo — `internal/finance/appypay.go` continua com várias responsabilidades num único ficheiro de 772 linhas

Credenciais, cobranças, QR code, webhooks e cifra estão todos no mesmo ficheiro. Não bloqueante, mas a auditoria anterior já recomendava dividir por sub-domínio (`credenciais.go`, `cobrancas.go`, `webhooks.go`, `crypto.go`), seguindo o padrão já usado no resto do repositório (um ficheiro por sub-domínio dentro do mesmo pacote).

## 6. Checklist de correcção para o codex

Ordem sugerida (do que mais protege dinheiro real para o que é só robustez/documentação):

1. [ ] Rodar `go build ./... && go vet ./... && go test ./...`; corrigir qualquer erro antes de tocar no resto.
2. [ ] 5.1 — Tornar `CreateCharge` verdadeiramente idempotente por `merchantTransactionId` (reserva antes de gravar qualquer evento; devolver o resultado existente em vez de erro genérico numa retentativa).
3. [ ] 5.3 — Corrigir a comparação de `paymentMethod` em `credentialSecrets.method()` (case-insensitive ou restringir/documentar só os atalhos curtos).
4. [ ] 5.4 — `ConsultCharge` só grava evento novo quando o estado muda.
5. [ ] 5.2 — Corrigir os exemplos (`resource`, `qrCodeType`) e a tabela de erros da secção 19 do `Documentação da API.md`; alinhar os códigos HTTP devolvidos pelo módulo financeiro com o que a documentação promete (ou ajustar a documentação à implementação, depois de decidir qual dos dois lados deve mudar).
6. [ ] 5.7 — Acrescentar os testes de idempotência de webhook, isolamento entre academias e RBAC (`gerente`/`adm` rejeitados).
7. [ ] 5.5 e 5.6 — Validar `FINANCE_ENCRYPTION_KEY` no arranque e impor um comprimento mínimo.
8. [ ] 5.8 (opcional, não bloqueante) — Dividir `appypay.go` por sub-domínio se sobrar orçamento/tokens.

Ao terminar, actualizar este documento (ou criar um "Correções executadas — ..." equivalente, no mesmo padrão já usado nas rondas anteriores em `docs/Debbugs/`) confirmando quais itens foram resolvidos, com os comandos de confirmação (`go build`/`go vet`/`go test`, e os `grep` relevantes) e os respectivos resultados.
