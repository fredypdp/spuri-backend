---
modificado: 2026-08-07
criado: 2026-08-07
origem: |
  Unificação de dois documentos de depuração do módulo financeiro AppyPay:
  - "Depuração — Módulo base de gestão financeira com AppyPay 2.md"
    (2ª auditoria completa, pré-rollback, sobre a tarefa 17)
  - "Depuração — Verificação da reimplementação do módulo financeiro AppyPay (Fase 1).md"
    (verificação da reimplementação pós-rollback, origem: tarefa 15)
---

# Depuração — Módulo financeiro AppyPay (histórico unificado)

## Cronologia e como ler este documento

Este ficheiro junta, na ordem em que aconteceram, as duas rondas de depuração mais recentes do módulo financeiro AppyPay:

- **Parte I — Ronda 2 (pré-rollback), auditoria da tarefa 17.** Auditoria campo a campo do que o módulo envia/recebe da AppyPay, validado contra a documentação oficial. Encontrou 2 bugs críticos de leitura de resposta (envelope `payment` ignorado e `status` lido no nível errado) que, na prática, quebravam a consulta de cobrança e a sincronização de estado em quase todos os fluxos, mais um desvio arquitetural (escrita directa no `spuri_ledger`, contornando `EventStore`/`Repository`) e problemas de gravidade média/baixa. É esta auditoria — e mais 2 rondas anteriores não incluídas aqui — que motivou o **rollback total** referido na Parte II.
- **Parte II — Verificação da reimplementação (Fase 1), pós-rollback.** Confirma que a reimplementação (cuja origem documental é a tarefa 15, descrita como "reimplementação pós-rollback da tarefa 17") corrige de facto as falhas críticas que levaram ao rollback, mas introduz uma **regressão nova e crítica**: qualquer rebuild da projeção financeira apaga permanentemente o histórico de cobranças. Identifica ainda uma regressão arquitectural (escritor duplo) e problemas de gravidade média.

**Nota sobre a numeração das tarefas:** os dois documentos originais referem-se por vezes à tarefa "17" e por vezes à tarefa "15" ao descrever a mesma reimplementação pós-rollback (ex.: Parte II cita "a tarefa 15, secção 2" e também "a tarefa 17 (secção 2, 'Diagnóstico')" para o mesmo diagnóstico). Isto é uma inconsistência já presente nos documentos-fonte e não foi resolvida aqui — mantém-se tal como escrita em cada parte, para não alterar a lógica original de nenhum dos dois relatórios.

**Nota sobre o fim da Parte II:** o documento de origem da Parte II termina de forma abrupta, a meio da secção "6. Recomendação" (falta a lista que seguiria os dois pontos finais, e não há secção "5" — o próprio original salta de "4. Achados médios" para "6. Recomendação"). Mantive isso tal como estava no ficheiro fornecido, sem inventar conteúdo; vale a pena confirmar se o original tem mais conteúdo que não chegou a ser exportado.

**Síntese rápida do estado actual:** a Parte I documenta os bugs que motivaram o rollback (leitura de resposta da AppyPay). A Parte II confirma esses bugs como resolvidos na reimplementação, mas identifica que o mecanismo de rebuild da projeção (achado 3.1) é o novo bloqueador crítico antes de produção — e aponta, como causa estrutural, o mesmo tipo de problema de fundo da Parte I (achado 4): dois caminhos de escrita independentes para o mesmo read model, agora sem uma tabela `Historico` a expor o sintoma de duplicação, mas com a mesma raiz. Nenhuma das duas rondas encontrou testes de concorrência, de rebuild contra Postgres real, nem de isolamento por papel — em ambas é apontado como a lacuna que permitiu os respectivos achados críticos passarem despercebidos.

---

# Parte I — Ronda 2 (pré-rollback): auditoria da tarefa 17

## Objetivo da auditoria

Confirmar se a tarefa `docs/Tarefas feitas/17 - Modulo base de gestao financeira com AppyPay.md` está corretamente implementada no código atual (`internal/financeiro/appypay.go`, `internal/handlers/financeiro_handlers.go`, `internal/projections/financeiro_projection.go`, `internal/db/safe_queries.go`, `internal/db/event_store.go`, `internal/db/repository.go`, migrações `097`–`100`, `cmd/server/main.go`), validando cada endpoint e cada leitura de resposta contra a documentação oficial da AppyPay (`docs/Parceiros e integrações/AppyPay Documentação.md`), conforme pedido na seção 8 da própria tarefa.

Esta é a segunda auditoria completa deste módulo (a primeira, `Depuração — Módulo base de gestão financeira com AppyPay 1.md`, era sobre a implementação da tarefa 15, removida por rollback). Desta vez o foco foi deliberadamente diferente: em vez de RBAC/isolamento (já bem coberto na v1), esta ronda validou campo a campo se o formato dos pedidos/respostas trocados com a AppyPay bate com o que a API realmente devolve — e é aí que apareceram os problemas mais graves.

### Resultado geral

A base de autenticação, cifragem, mascaramento e whitelist de eventos está **correta e sólida**. Porém há **dois bugs críticos na leitura das respostas da AppyPay** que, na prática, quebram a consulta de cobrança e a captura de estado em quase todos os fluxos — o módulo cria pedidos corretamente, mas não sabe interpretar o que a AppyPay responde. Há ainda **um desvio arquitetural** relevante (escrita direta no `spuri_ledger`, contornando o `EventStore`/`Repository` padrão) e alguns problemas de gravidade menor.

---

## I.1 🔴 CRÍTICO — `GetCharge` nunca lê dados reais (envelope `payment` ignorado)

A documentação oficial (`Get a charge`, resposta 200) mostra que o corpo real vem **todo** dentro de um objeto `payment`:

```json
{
  "payment": {
    "id": "57af18f2-64b7-4464-a503-3baf741c9f0d",
    "merchantTransactionId": "131fd130701",
    "status": "Failed",
    ...
  }
}
```

Mas `GetCharge` (em `internal/financeiro/appypay.go`) passa o corpo bruto directamente para `resultFrom`, que lê tudo no nível raiz:

```go
func resultFrom(data map[string]any, status int, merchant string) *ChargeResult {
	r := &ChargeResult{ID: stringValue(data, "id"), MerchantTransactionID: stringValue(data, "merchantTransactionId"), Status: stringValue(data, "status"), HTTPStatus: status, Data: data}
	...
}
```

Como `data["id"]`, `data["status"]`, etc. nunca existem no nível raiz de uma resposta de `GET /charges/{id}`, o resultado devolvido por `ConsultarCobranca` está **sempre vazio** (ID vazio, status a cair no fallback `"aceita"`), independentemente do que a AppyPay realmente reportou. Isto invalida:

- O critério de aceitação nº 5 da tarefa 17 ("função base de consulta de cobrança").
- A recomendação da própria tarefa/documentação de fazer dupla verificação via `GET /charges/{id}` antes de aplicar efeitos de negócio irreversíveis (seção "Webhooks (transacional)" do resumo AppyPay, linha ~8382).

**Correção sugerida:** desembrulhar o envelope antes de interpretar a resposta:

```go
func unwrapPayment(data map[string]any) map[string]any {
	if payment, ok := data["payment"].(map[string]any); ok {
		return payment
	}
	return data
}
```

e chamar `resultFrom(unwrapPayment(data), status, idOrMerchantID)` em `GetCharge`.

---

## I.2 🔴 CRÍTICO — Estado (`status`) nunca é capturado — campo lido no nível errado em todos os fluxos

Tanto a resposta síncrona de `POST /charges` e `POST /qr-codes` como o payload recebido no webhook trazem o estado dentro de `responseStatus.status`, nunca como `status` de topo:

```json
"responseStatus": {
  "successful": true,
  "status": "Success",
  ...
}
```

`resultFrom()` e `ReceiveWebhook()` leem `stringValue(data, "status")` / `stringValue(body, "status")` diretamente no nível raiz — sempre vazio na prática. Consequências concretas:

- **`CreateCharge`**: toda cobrança criada é gravada com status fixo `"aceita"` (fallback de `resultFrom`), mesmo quando a AppyPay já devolve `Failed` de forma síncrona (ex.: simular `phoneNumber: "900000003"` — rejeitado pelo cliente — continuaria a ser persistido como `"aceita"`).
- **`CreateGPOQRCode`**: mesmo problema.
- **`ReceiveWebhook`**: a condição `if status != "" { UPDATE financeiro_cobrancas SET status=... }` nunca é satisfeita, então **o webhook nunca atualiza o estado real da cobrança** — o único efeito prático do webhook hoje é gravar o payload bruto (sanitizado) no ledger/`financeiro_webhooks_recebidos`, não sincronizar o estado.

Isto compromete diretamente o propósito central da tarefa: acompanhar o resultado real de uma cobrança GPO/REF.

**Correção sugerida:** extrair de `responseStatus.status` com fallback para o nível raiz (cobre variação entre endpoints):

```go
func statusFrom(data map[string]any) string {
	if rs, ok := data["responseStatus"].(map[string]any); ok {
		if s := stringValue(rs, "status"); s != "" {
			return s
		}
	}
	return stringValue(data, "status")
}
```

e usar `statusFrom(data)` em vez de `stringValue(data, "status")` em `resultFrom` e em `ReceiveWebhook`.

---

## I.3 🟠 ALTO — Idempotência do webhook por `id` isolado pode descartar transições de estado legítimas

A documentação avisa explicitamente:

> **Important:** The webhook can be triggered **more than 1 time** for the same transaction [...] For REF payments that reflects the number of communications with the provider (for a single payment and the compensation file as summary).

Ou seja: o mesmo `id` pode chegar mais de uma vez **com conteúdo diferente** (ex.: uma tentativa falhada seguida de uma bem-sucedida), não apenas como reenvio idêntico por falha de comunicação. A chave de deduplicação usada hoje é só `método:id`:

```go
eventKey := method + ":" + key
...
res, err := tx.ExecContext(ctx, `INSERT INTO financeiro_webhooks_recebidos(event_key,...) VALUES(...) ON CONFLICT DO NOTHING`, eventKey, ...)
if n, _ := res.RowsAffected(); n == 0 {
	// tratado sempre como duplicado — nunca reprocessado
}
```

Isto significa que, mesmo depois de corrigir o achado I.2, uma segunda notificação **legítima e diferente** para a mesma cobrança (ex.: `Pending` → `Success`) seria descartada como duplicado puro, porque a chave não incorpora o estado/conteúdo da notificação.

**Correção sugerida:** basear a deduplicação num hash do conteúdo relevante (`id + status + code`) em vez de só `id`, ou manter `id` como chave mas fazer `ON CONFLICT DO UPDATE` comparando se o `status` mudou antes de decidir se é duplicado.

---

## I.4 🟠 ALTO — Módulo contorna o `EventStore`/`Repository` padrão do resto do sistema

`internal/financeiro/appypay.go` escreve directamente no `spuri_ledger` com SQL cru:

```go
func (s *Service) writeEventTx(ctx context.Context, tx *sqlx.Tx, id uuid.UUID, typ string, payload map[string]any, actor db.AuditContext) (uuid.UUID, error) {
	...
	_, err = tx.ExecContext(ctx, `INSERT INTO spuri_ledger(event_id,aggregate_id,aggregate_type,event_type,event_version,payload,metadata,occurred_at) VALUES($1,$2,'Financeiro',$3,$4,$5,$6,CURRENT_TIMESTAMP)`, ...)
	return eventID, err
}
```

Isto contorna por completo `db.EventStore.AppendTx` / `db.AggregateRepository.Save`, que são o único caminho usado por todos os outros agregados (`Estudante`, `Academia`, `Admin`, etc.). Duas consequências:

- **Não existe agregado `Financeiro`** em `internal/domain/aggregates/` nem registo em `DefaultAggregateFactory` — ao contrário do que a própria tarefa 17 pede explicitamente na seção 8: *"Seguir os padrões e nomes já usados no resto do repositório (agregados em `internal/domain/aggregates/`...)"* e na seção 4: *"tal como qualquer outro agregado do sistema"*.
- **Contorna `ValidateAggregateType`/`ValidateEventType`**, que só são chamadas dentro de `EventStore.appendDirect`/`AppendTx`. O comentário no próprio código explica por que isto importa:

  ```go
  // Método interno (unexported) — uso externo ao pacote db deve passar por
  // AggregateRepository.Save / SaveWithAudit que usam AppendTx com tx Serializable.
  //
  // FIX DB-07: renomeado de Append (público) para appendDirect (interno) para
  // evitar que código externo ao pacote grave eventos diretamente no ledger
  // contornando a serialização de versão do repositório.
  ```

  O módulo financeiro é hoje o único ponto do sistema que volta a fazer exatamente o que a FIX DB-07 tentou impedir. Não é explorável agora (os `event_type`/`aggregate_type` usados são strings fixas dentro do próprio pacote), mas é uma inconsistência real com uma proteção da qual o resto do sistema depende, e torna a whitelist em `safe_queries.go` decorativa para este módulo.

**Correção sugerida:** criar `internal/domain/aggregates/financeiro.go` seguindo o mesmo padrão de `Estudante`/`Academia` (implementando `Aggregate`, com `Apply`/eventos próprios), registá-lo em `DefaultAggregateFactory.Create`, e passar a escrever via `db.AggregateRepository.SaveWithAudit`. Alternativa mais rápida e menos invasiva: expor um método público equivalente a `AppendTx` que mantenha as validações, e usá-lo em vez de montar o `INSERT` à mão.

---

## I.5 🟡 MÉDIO — Formato de data errado no QR Code `MULTIPLE`

A documentação define `startDate`/`endDate` como `string<date>` (exemplo: `"2024-11-19"`). O código envia data-hora completa:

```go
if req.StartDate != nil {
	body["startDate"] = req.StartDate.UTC().Format(time.RFC3339) // "2024-11-19T00:00:00Z"
}
```

Risco de a AppyPay rejeitar o pedido por formato inesperado. **Correção sugerida:** `req.StartDate.UTC().Format("2006-01-02")`.

## I.6 🟡 MÉDIO — Erros de validação devolvidos como `502 Bad Gateway`

`criarCobrancaAppyPay`, `criarQRCodeAppyPay` e `consultarCobrancaAppyPay` devolvem sempre `http.StatusBadGateway` para qualquer erro vindo do `Service`:

```go
out, err := financeiroService(c).CreateCharge(c.Request.Context(), scope, req, auditFinanceiro(c))
if err != nil {
	utils.RespondWithError(c, http.StatusBadGateway, "APPYPAY_ERROR", err)
	return
}
```

Isto inclui erros de validação de input puramente locais (`"amount deve ser maior que zero"`, `"GPO exige paymentInfo.phoneNumber"`), que deviam ser `400`, não `502` (que semanticamente diz "a AppyPay falhou", quando na verdade o pedido nem chegou a sair do Spuri). **Correção sugerida:** distinguir erros de validação (retornados antes de qualquer chamada HTTP) dos erros vindos de `appypayJSON`, devolvendo 400 para os primeiros.

---

## Achados de gravidade baixa / observações (Parte I)

- **Cobertura de testes rasa.** `appypay_test.go` só cobre seleção de ambiente, round-trip de cifragem e `sanitize()`. Não há nenhum teste que simule uma resposta real da AppyPay (com `payment`/`responseStatus` aninhados) contra `CreateCharge`, `GetCharge` ou `ReceiveWebhook` — é exatamente por isso que os achados I.1 e I.2 passaram despercebidos.
- **Comentário órfão em `internal/db/repository.go`** (linha final): `// placeholder - wrong file, ignore`. Não afeta o comportamento, mas parece resíduo de edição e vale a pena remover por limpeza.
- **Numeração de migração duplicada**: `100_financeiro_appypay_base.sql` coexiste com `100_remove_projection_estudantes_ano_escolar_legado.sql`. Inofensivo — `loadMigrations` rastreia por nome de ficheiro completo, não pelo prefixo numérico — mas quebra a convenção sequencial estrita que outras migrações da tarefa seguem.

## O que está correto (validado) — Parte I

- Autenticação client credentials grant, cache de token em memória com renovação antes da expiração (`expiry - time.Minute`).
- Seleção TEST/PROD centralizada em `CurrentEnvironment()`, lida de `ENV`, sem valores fixos espalhados pelo código.
- Credenciais cifradas com AES-GCM (nonce aleatório por valor), nunca devolvidas em texto puro — `GetCredential`/`CredentialView` só expõem máscaras.
- Isolamento por escopo: uma academia só configura/consulta as próprias credenciais (`academiaFinanceiroScope` usa o código da academia autenticada, não input do cliente); credenciais do Spuri restritas a `middleware.RequireFPP()`.
- Whitelist de eventos e aggregate types em `safe_queries.go` cobre corretamente os novos tipos financeiros (ainda que, pelo achado I.4, não seja de facto aplicada no caminho de escrita deste módulo).
- `sanitize()` remove chaves candidatas a segredo (`secret`, `token`, `apikey`, `authorization`) do payload do webhook antes de persistir — testado.
- `merchantTransactionId` gerado e validado corretamente (alfanumérico, ≤15 caracteres).
- Validação mínima por método (GPO exige `phoneNumber`; REF aceita `paymentInfo` opcional) implementada corretamente.
- Reembolso, reversão e reconciliação/observabilidade **não** foram implementados — confirmado por busca no código (nenhuma referência a refund/reverse/reconciliação).
- Endpoints documentados em `Documentação da API.md` (seção "Financeiro — AppyPay (Fase 1)").
- Rotas registadas corretamente em `cmd/server/main.go`, incluindo os dois webhooks públicos e os grupos `academia`/`adminSistema` com os middlewares esperados.

## Checklist da tarefa 17 revisado

- [x] Credenciais cifradas, nunca devolvidas em texto puro, para academia e Spuri
- [x] Token reutilizado com renovação automática; seleção TEST/PROD automática
- [ ] ⚠️ Cobrança GPO/REF é criada, mas o estado devolvido/gravado não reflete o resultado real da AppyPay (achado I.2)
- [ ] ⚠️ QR Code GPO funciona, mas com formato de data incorreto para `MULTIPLE` (achado I.5) e mesmo problema de status (achado I.2)
- [ ] ❌ Consulta de cobrança por id/merchantTransactionId está quebrada (achado I.1)
- [ ] ⚠️ Webhooks GPO/REF existem, protegidos e persistem payload, mas não sincronizam o estado real da cobrança (achados I.2+I.3)
- [x] Nenhum segredo aparece em payload de evento, log ou resposta de API
- [x] Reembolso, reversão e reconciliação/observabilidade não foram implementados

## Recomendação de ordem de correção (Parte I)

1. **Corrigir a extração de `status`/envelope `payment`** (achados I.1+I.2) — é a mesma causa raiz (assumir que a resposta da AppyPay é sempre "plana"), e é o que bloqueia todo o resto de funcionar como esperado.
2. **Revisar a chave de idempotência do webhook** (achado I.3) para não descartar transições de estado legítimas, aproveitando a mesma correção para adicionar teste de replay com dois payloads diferentes para o mesmo `id`.
3. **Migrar a escrita de eventos para o `EventStore`/`Repository` padrão** (achado I.4) — maior esforço, mas remove a única exceção arquitetural do sistema.
4. Corrigir o formato de data do QR Code `MULTIPLE` e a semântica HTTP dos erros de validação (achados I.5 e I.6).
5. Adicionar testes com respostas simuladas no formato real da AppyPay (`payment`/`responseStatus` aninhados) para `CreateCharge`, `GetCharge` e `ReceiveWebhook`, para impedir regressão dos achados I.1 e I.2.

## Comandos de validação sugeridos (Parte I)

- `go build ./...` e `go vet ./...` (checagem mecânica mínima antes de qualquer correção).
- `go test ./internal/financeiro/... -v` (hoje passa, mas não cobre os achados I.1/I.2 — útil como baseline antes/depois da correção).
- Teste manual em ambiente TEST: criar uma cobrança GPO com `phoneNumber: "900000003"` (rejeitado pelo cliente) e confirmar que `financeiro_cobrancas.status` fica diferente de `"aceita"` depois da correção.
- Teste manual: `GET /academia/financeiro/appypay/cobrancas/{merchantTransactionId}` depois da correção do achado I.1, confirmando que `id`/`status` deixam de vir vazios.

---

# Parte II — Verificação da reimplementação (Fase 1), pós-rollback

## Veredito imediato

**A reimplementação corrige, de facto, todas as 5 falhas críticas que motivaram o rollback total (tarefa 17), mas introduz uma regressão nova e crítica: qualquer rebuild da projeção financeira apaga permanentemente todo o histórico de cobranças.** Existe ainda um teste de regressão deixado pelo rollback que hoje **falha** contra a whitelist de eventos actual, o que por si só implica que `go test ./...` não passa neste momento. Não há testes de concorrência, de rebuild contra Postgres real, nem de isolamento por papel — exactamente o que a tarefa 17 (secção 2, "Diagnóstico") recomendava existir desde o primeiro commit desta reimplementação.

**Não considero este módulo pronto para produção** enquanto os itens da secção "Achados críticos" (II.3.1–II.3.2) não forem corrigidos. Os itens da secção "Achados médios" (II.4.1–II.4.3) devem ser avaliados antes de ligar a um provedor com dinheiro real.

## Âmbito e método desta verificação

Clonei `https://github.com/fredypdp/spuri-backend` (branch `main`) e li integralmente:

- `internal/financeiro/appypay.go` (789 linhas) e `internal/financeiro/appypay_test.go`
- `internal/handlers/financeiro_handlers.go`
- `internal/projections/financeiro_projection.go`
- `migrations/100_financeiro_appypay_base.sql`
- `internal/db/safe_queries.go` e `internal/db/safe_queries_test.go` (whitelist de eventos/aggregates)
- `internal/domain/aggregates/aggregate.go` (factory de aggregates)
- `cmd/server/main.go` (registo de rotas, grupos de middleware, projeção)
- `Documentação da API.md` e `docs/Parceiros e integrações/AppyPay Documentação.md`
- Todo o histórico em `docs/Debbugs/` relativo ao módulo financeiro (as 3 rondas de auditoria pré-rollback e a tarefa 17)

**Limitação a registar:** o `go.mod` deste repositório exige `go 1.24.0` / toolchain `go1.24.12`. O ambiente onde corri esta verificação só permite instalar `golang-go 1.22` via `apt` (rede restrita a um conjunto fixo de domínios, sem acesso a `proxy.golang.org`), e uma dependência transitiva (`github.com/t3rm1n4l/go-mega`) exige Go ≥ 1.24 no seu próprio `go.mod`. **Não consegui executar `go build ./...` / `go test ./...` neste ambiente.** Todos os achados abaixo vêm de leitura directa do código e, quando indicado, de dedução lógica verificável (ex.: secção II.3.2, onde a contradição é puramente textual entre dois ficheiros e não depende de o binário compilar). Recomendo confirmar a secção II.3.2 correndo `go test ./internal/db/...` num ambiente com Go 1.24.

## Achados críticos (Parte II)

### II.3.1 🔴 Rebuild da projeção financeira apaga permanentemente as cobranças

`internal/projections/financeiro_projection.go`:

```go
func (p *FinanceiroProjection) Rebuild() error {
	if _, err := p.client.DB().Exec(`DELETE FROM financeiro_webhooks_recebidos; DELETE FROM financeiro_cobrancas`); err != nil {
		return err
	}
	// ... replay de todos os eventos "Financeiro" via p.Handle(e)
}

func (p *FinanceiroProjection) charge(e db.Event, v map[string]any) error {
	// ...
	_, err = p.client.DB().Exec(`UPDATE financeiro_cobrancas SET ... WHERE id=$5`, ...)
	return err
}
```

`Rebuild()` apaga todas as linhas de `financeiro_cobrancas` e depois tenta repor o estado reaplicando os eventos `CobrancaFinanceiraSolicitada` / `CobrancaFinanceiraCriada` / `CobrancaFinanceiraStatusAtualizado` — mas `charge()` só faz `UPDATE ... WHERE id=$5`. Como a linha acabou de ser apagada, o `UPDATE` afecta 0 linhas, não retorna erro, e a cobrança **nunca é recriada**. Ao contrário de `webhook()` (linha 102, `INSERT ... ON CONFLICT DO NOTHING` — correcto) e ao contrário de `persistCharge` no `Service` (`appypay.go`, `INSERT ... ON CONFLICT(id) DO UPDATE` — também correcto), `charge()` é o único dos três handlers que não sabe recriar a linha do zero.

**Impacto:** qualquer rebuild de projeções (rota administrativa de rebuild, recuperação de desastre, ou simplesmente correr `manager.RebuildAll()`) esvazia `financeiro_cobrancas` de forma silenciosa e permanente para qualquer cobrança que não seja tocada de novo por uma chamada síncrona do `Service` depois do rebuild (ex.: uma cobrança já paga e nunca mais consultada). Isto viola directamente a invariante central do desenho de Event Sourcing do próprio Spuri — "projeções são read models reconstruíveis a partir do ledger" — e contradiz literalmente a secção 6 da tarefa 15 ("Rebuild de projeções financeiras deve seguir o mesmo mecanismo dos outros módulos... reaplicando os eventos financeiros em ordem determinística").

**Correcção sugerida:** `charge()` deve fazer `INSERT ... ON CONFLICT(id) DO UPDATE`, à semelhança de `persistCharge`, tratando `CobrancaFinanceiraSolicitada` como o evento que cria a linha (tem `credential_id`, `contexto_tipo`, `codigo_academia`, `merchant_transaction_id`, `metodo` no payload) e os dois eventos seguintes como actualizações parciais sobre uma linha que, num replay em ordem, já foi inserida pelo evento anterior.

### II.3.2 🟠 Regressão de arquitectura: escritor duplo nas tabelas `financeiro_*`

A 4ª ronda de auditoria pré-rollback (`docs/Debbugs/Auditoria e Plano de Melhoria — Modulo Financeiro AppyPay.md`, item 6 da secção 3) já tinha identificado e corrigido este problema: *"Escritor duplo (Service + FinanceiroProjection) duplicava Historico — Corrigido: Service só escreve no cofre de segredos; as tabelas públicas financeiro_* só são escritas pela FinanceiroProjection assíncrona"*.

Na reimplementação actual, o `Service` (`appypay.go`) voltou a escrever directamente nas tabelas públicas — `ConfigureCredentials` faz `INSERT ... ON CONFLICT ... DO UPDATE` em `financeiro_credenciais_appypay`, `persistCharge` faz o mesmo em `financeiro_cobrancas`, `ReceiveWebhook` insere em `financeiro_webhooks_recebidos` — **e** a `FinanceiroProjection` assíncrona volta a processar os mesmos eventos e a escrever nas mesmas tabelas. Não há hoje uma tabela `Historico` separada (por isso o sintoma exacto da ronda anterior — histórico duplicado — não se repete), mas a causa raiz é a mesma reintroduzida: duas escritas independentes, uma síncrona e outra assíncrona, apontando ao mesmo read model a partir da mesma fonte de eventos.

É precisamente esta duplicidade que torna o achado II.3.1 possível: como o caminho síncrono já mantém as tabelas actualizadas durante a operação normal, ninguém reparou que o caminho assíncrono (o único que corre durante um `Rebuild()`) está incompleto — os dois caminhos nunca foram testados a operar sozinhos um sem o outro.

**Correcção sugerida:** escolher um único escritor para as tabelas de leitura pública `financeiro_credenciais_appypay` / `financeiro_cobrancas` / `financeiro_webhooks_recebidos`. Ou (a) o `Service` deixa de escrever nelas directamente e delega inteiramente à `FinanceiroProjection` (como a ronda anterior tinha decidido), aceitando a latência de propagação normal do resto do sistema; ou (b), se a escrita síncrona for mantida por necessidade de resposta imediata ao chamador, a `FinanceiroProjection` deixa de reprocessar estes três tipos de evento fora do `Rebuild()` explícito (ou seja, o registo em `RegisterProjection` continua a existir só para permitir rebuild sob pedido, não para consumo contínuo do ledger). De qualquer forma, resolver primeiro o achado II.3.1, porque sem ele qualquer uma das duas opções deixa de reconstruir o estado correctamente.

## Achados médios (Parte II)

### II.4.1 🟡 `merchant_transaction_id` é único globalmente, não por conta AppyPay

`migrations/100_financeiro_appypay_base.sql`: `merchant_transaction_id VARCHAR(15) NOT NULL UNIQUE`. É uma restrição de coluna simples, sem `credential_id` na chave. Duas academias diferentes que (independentemente da geração automática por UUID em `newMerchantID()`) fornecerem o mesmo `merchantTransactionId` — plausível se uma tarefa futura de negócio (propinas, matrículas) adoptar um esquema legível por humanos em vez do gerador automático — vão colidir entre si mesmo não sendo a mesma conta AppyPay, apesar de a AppyPay documentar unicidade por conta de comerciante, não global. Sugiro `UNIQUE (credential_id, merchant_transaction_id)`.

### II.4.2 🟡 `GetCharge` pode gerar uma chamada sem `{id}` no path, fora do que a AppyPay documenta

Em `appypay.go`, `GetCharge` cai, quando não há `appypay_charge_id` guardado localmente, em `path = "/charges?merchantTransactionId=" + ...` — **sem nenhum segmento `{id}` no path**. A documentação oficial (`Get a charge`) marca `id` como parâmetro de path **obrigatório** e `merchantTransactionId` como parâmetro de query opcional/complementar, não como substituto de `id`. Não tenho como confirmar contra o ambiente de teste real da AppyPay se este padrão de chamada é aceite; recomendo um teste de integração dedicado contra o TEST antes de confiar neste caminho (ele só é exercido quando ainda não recebemos nenhuma resposta anterior da AppyPay para essa cobrança, isto é, exactamente o cenário em que mais precisamos que a consulta funcione).

*Nota de ligação com a Parte I:* este é o mesmo endpoint do achado I.1 (envelope `payment` ignorado). A Parte I confirma que o corpo da resposta de `GET /charges/{id}` vem dentro de `payment`; este achado (II.4.2) é sobre a forma do pedido em si (falta de `{id}` no path num dos dois caminhos de `GetCharge`), não sobre a leitura da resposta — são bugs distintos no mesmo método.

### II.4.3 🟡 Ausência total de testes de concorrência, de rebuild real e de isolamento por papel

`internal/financeiro/appypay_test.go` tem 3 testes, todos unitários e sem dependência de Postgres: selecção TEST/PROD, round-trip de cifra, e remoção de segredos do payload de webhook. Não existe nenhum teste que:

- crie duas cobranças concorrentes com o mesmo `merchantTransactionId` (que teria apanhado, por dedução, ou pelo menos exercitado, o comportamento descrito na secção II.3.1 se combinado com um rebuild);
- corra `FinanceiroProjection.Rebuild()` contra um Postgres real e confirme que os dados batem certo com o estado pré-rebuild (isto teria apanhado II.3.1 directamente);
- confirme que um `estudante` autenticado recebe 403/404 em todas as rotas `/academia/financeiro/*` e `/admin/financeiro/*` (a vulnerabilidade de RBAC da ronda 2 pré-rollback não tem nenhum teste de regressão equivalente nesta reimplementação).

Isto é exactamente o que a tarefa 15, secção 2 ("Diagnóstico"), pede para a reimplementação ter *desde o primeiro commit*: *"testes de concorrência (-race), testes de replay contra Postgres real, e testes de isolamento por papel (estudante, academia de outro tenant) desde o primeiro commit — não adicionados depois, ronda a ronda."* Isso não aconteceu; o achado II.3.1 é a prova directa da consequência.

## Recomendação (Parte II)

Não marcar esta verificação como concluída/aprovada. Antes de considerar o módulo pronto para ligar a credenciais reais:

> *(O documento de origem termina aqui — a lista de próximos passos que seguiria este parágrafo não estava presente no ficheiro fornecido. Ver nota no topo deste documento.)*
