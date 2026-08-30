---
data: 2026-08-30
status: corrigido_via_75_corrigir_efetivacao_de_matricula_via_webhook_appypay_e_paymentinfo_vazio_em_post_charges
auditor: Claude (orquestrador) — auditoria linha a linha contra docs/Parceiros e integrações/AppyPay Documentação.md, com PostgreSQL 16 real e Go 1.24 real em sandbox
tarefa_correcao: docs/Lista de Tarefas/78 - Corrigir efetivação de matrícula via webhook AppyPay e paymentInfo vazio em POST charges.md
relacionado_a: docs/Tarefas feitas/70 - Corrigir extração de status e motivo real da AppyPay (Cancelled do GPO, Expired do REF) e categorização preditiva.md
---

# Auditoria de conformidade AppyPay — autenticação e geração de cobrança

## Como esta auditoria foi pedida

Fredy pediu uma depuração detalhada da autenticação e da geração de cobrança com a API da AppyPay, para garantir conformidade com `docs/Parceiros e integrações/AppyPay Documentação.md`, com a exigência explícita de que qualquer correção fosse validada com PostgreSQL/compilação real antes de virar tarefa para o Codex (que não tem `apt`/Docker/`psql`).

Esta não foi uma investigação motivada por um bug relatado em produção — foi uma auditoria pró-ativa, comparando cada trecho relevante de `internal/finance/appypay.go`, `internal/finance/cobranca_geracao.go`, `internal/finance/matricula.go`, `internal/finance/mensalidade.go` e os handlers HTTP em `internal/handlers/financeiro_handlers.go`/`solicitacao_matricula_handlers.go` contra a documentação (8387 linhas), incluindo a seção "Escopo do Módulo Financeiro Base — Spuri × AppyPay (Fase 1)" anexada ao fim do próprio documento (resumo já confirmado comercialmente com a AppyPay por e-mail).

## Escopo da auditoria

- Autenticação OAuth2 (client_credentials): URL, método HTTP, corpo, cache/expiração do token, seleção TEST/PROD.
- `POST /charges` (GPO e REF): construção do corpo, `paymentMethod`, `paymentInfo`, `merchantTransactionId`, `options`, `notify`.
- `POST /qr-codes` (GPO QR Code): endpoint próprio, corpo, tipos SINGLE/MULTIPLE.
- `GET /charges/{id}`: leitura do envelope `payment`/`transactionEvents`.
- Webhooks: autenticação (cabeçalho), formato do payload, idempotência, e o que o Spuri faz ao receber um webhook de sucesso.
- Armazenamento de credenciais (`client_id`/`client_secret`/segredo de webhook cifrados).

## O que está CORRETO (confirmado item a item)

Nenhuma correção foi necessária nestes pontos — todos conferidos linha a linha contra a documentação:

1. **URLs TEST/PROD** de token e de API — `EndpointsAtuais()` monta exatamente `https://login.microsoftonline.com/appypaydev.onmicrosoft.com/oauth2/token` (TEST) / `https://login.microsoftonline.com/auth.appypay.co.ao/oauth2/token` (PROD) e `https://gwy-api-tst.appypay.co.ao/v2.0` / `https://gwy-api.appypay.co.ao/v2.0`, selecionadas pela variável `ENV` já existente no Spuri — não fixas no código.
2. **`resource`** lido de uma única variável de ambiente global (`APPYPAY_RESOURCE`), não por academia — bate com a decisão já registrada na seção "Escopo" do documento.
3. **`client_id`/`client_secret`** cifrados por academia/Spuri em `financeiro_credenciais_appypay`, nunca em texto plano, nunca em resposta pública (`sanitize()` remove os campos sensíveis antes de qualquer serialização).
4. **Cache/renovação de token** — o token é cacheado em memória com sua expiração e só renovado quando necessário (não é pedido a cada chamada).
5. **`merchantTransactionId`** — gerado (`merchantID()`) ou validado (`validMerchantID()`) como alfanumérico, 1–15 caracteres, batendo exatamente com o padrão documentado (`^[a-zA-Z0-9]+$`, máx. 15).
6. **`paymentMethod`** no formato `{MÉTODO}_{uuid}` (ex.: `GPO_53c70da3-...`), lido das credenciais por academia — bate com o formato documentado.
7. **GPO QR Code usa endpoint próprio** (`POST /qr-codes`), não reaproveita `/charges` — confirmado no código (`CreateGPOQRCode`) e na documentação.
8. **Limite de 2 opções customizadas** (`options`) documentado em "Custom options" é respeitado por `validateCharge`.
9. **`GET /charges/{id}`** — leitura correta do envelope `payment.status` e do último `transactionEvents[i].responseStatus` (código, mensagem, fonte).
10. **Autenticação de webhook por um único cabeçalho fixo** (`X-Spuri-Webhook-Secret`) — já documentado internamente (tarefa 30/41) como a única forma que o painel da AppyPay realmente oferece (confirmado por e-mail com a AppyPay); não é uma simplificação indevida do "Basic Auth ou API Key" genérico da documentação pública.

## Não conformidade da documentação da AppyPay revisada e mantida de propósito (não é bug)

A documentação rotula o endpoint de token como `GET` (`get` + exemplo `curl --request GET '{{server}}/token' --data-urlencode ...`), e dois documentos internos de análise (`AppyPay - Analise para o Spuri.md`, `AppyPay - Análise de Integração...md`) leem a mesma coisa. O código atual usa `POST` (`http.NewRequestWithContext(ctx, http.MethodPost, EndpointsAtuais().TokenURL, ...)`).

**Decisão: manter `POST`, não é um bug.** Motivos:
- RFC 6749 §4.4.2 (client_credentials grant) exige `POST` para o endpoint de token — não é opcional.
- O endpoint é do Azure AD/Microsoft identity platform (`login.microsoftonline.com`), que não aceita `GET` para emissão de token — isto é um comportamento estável da plataforma da Microsoft, não algo que varia por integração.
- O sistema está em uso — dezenas de tarefas anteriores (26 a 74) construíram cobrança real em cima desta autenticação; se `POST` estivesse errado, nada mais teria funcionado.

Ler isto como `GET` na documentação da AppyPay é quase certamente um artefato da ferramenta de geração automática de documentação (Stoplight/ReadMe) usada pela própria AppyPay, não uma instrução real da API. Mudar para `GET` teria altíssima chance de quebrar a autenticação inteira — por isso não virou tarefa de correção. Fica registrado aqui para o caso de a AppyPay confirmar o contrário no futuro.

## Bug 1 (crítico) — confirmação de matrícula via webhook nunca dispara

### Causa raiz

`internal/handlers/financeiro_handlers.go`, dentro de `ReceberWebhookAppyPay` (a rota HTTP real, registrada em `cmd/server/main.go` como `/webhooks/appypay/gpo` e `/webhooks/appypay/ref`), decidia se o webhook representava sucesso chamando duas funções locais:

```go
func isSuccessfulWebhook(payload map[string]any) bool {
	return strings.EqualFold(strings.TrimSpace(webhookStatus(payload)), "success")
}
func webhookStatus(payload map[string]any) string {
	for _, k := range []string{"status", "state"} {
		if v, ok := payload[k].(string); ok {
			return v
		}
	}
	return ""
}
```

Isto lê `payload["status"]`/`payload["state"]` **na raiz** do JSON. Só que a AppyPay **nunca** manda o status de um webhook na raiz — confirmado na seção "Merchant Webhooks" da documentação: o status vem sempre dentro de `responseStatus.status` (junto de `responseStatus.successful`, `.code`, `.message`, `.source`). Contra um webhook real, `webhookStatus` sempre devolve `""`, `isSuccessfulWebhook` sempre devolve `false`, e o bloco que efetiva a matrícula do estudante nunca executa:

```go
if isSuccessfulWebhook(payload) {
    if codigo, err := FinanceiroService.CodigoSolicitacaoDaCobranca(...); err == nil && codigo != "" {
        if err := efetivarVinculoMatriculaPaga(c, codigo); err != nil { ... }
    }
}
```

Ou seja: uma matrícula de estudante paga via REF ou GPO com webhook configurado ficaria **presa para sempre** em `aprovada_pendente_pagamento_matricula`, mesmo com o pagamento confirmado do lado da AppyPay — a menos que alguém consultasse manualmente `GET /financeiro/appypay/cobrancas/:id` (que usa um caminho diferente e correto, ver abaixo).

Este é o **mesmo padrão de bug já corrigido na tarefa 70** (`responseStatus(v)` em `internal/finance/appypay.go` lendo `status`/`state` soltos em vez de `responseStatus.status`) — mas a tarefa 70 corrigiu apenas a função interna do pacote `finance` (`extractProviderOutcome`, usada por `CreateCharge`/`CreateGPOQRCode`/`ConsultCharge`/`AcceptWebhook` para classificar a cobrança em si). Ela não tocou nesta segunda implementação, independente e duplicada, que vive no pacote `handlers` e existe só para decidir se aciona `efetivarVinculoMatriculaPaga`. As duas nunca foram a mesma função — por isso a correção da tarefa 70 não alcançou este bug.

### Por que os testes existentes não pegaram isto

`TestIntegrationReceberWebhookAppyPayEfetivaVinculoMatricula` (o teste dedicado exatamente a este comportamento) simulava o webhook com `json.Marshal(map[string]any{"id": eventID, "status": "Success"})` — um formato plano que **não existe** num webhook real da AppyPay, mas que coincidentemente é o único formato que `webhookStatus` sabia ler. O teste passava testando contra a premissa errada sobre o próprio contrato da AppyPay, não contra o comportamento real — o mesmo padrão de causa (mock com formato plano incorreto) já documentado na tarefa 70 para outros arquivos de teste.

### Prova empírica (não é só leitura de código)

Com PostgreSQL 16 real:
1. Troquei o payload do teste para o formato real documentado (`responseStatus: {successful, status, code, message, source}` aninhado).
2. Rodei o teste **com o código antigo** (`isSuccessfulWebhook`/`webhookStatus`): falhou exatamente como previsto — `status da solicitação = "aprovada_pendente_pagamento_matricula", queria aprovada`.
3. Apliquei a correção (reaproveitar a extração já correta do pacote `finance` via uma nova função exportada) e rodei de novo: passou, incluindo o segundo POST do mesmo webhook (idempotência) e a checagem de que nenhum estudante duplicado foi criado.

### Correção

Adicionada `finance.IsSuccessfulProviderPayload(payload map[string]any) bool` em `internal/finance/appypay.go`, reaproveitando a mesma cadeia já testada e correta (`extractProviderOutcome` → `normalizeChargeStatus` → `isSuccessfulChargeStatus`) usada por `CreateCharge`/`CreateGPOQRCode`/`ConsultCharge`/`AcceptWebhook`. `ReceberWebhookAppyPay` passa a chamar essa função; `isSuccessfulWebhook`/`webhookStatus` (que não tinham nenhum outro chamador, confirmado por busca no repositório inteiro) foram removidas.

## Bug 2 — `paymentInfo`/`options`/`notify` vazios enviados como `{}`/`null` em vez de omitidos

### Causa raiz

`CreateCharge` (`internal/finance/appypay.go`) montava o corpo de `POST /charges` assim:

```go
providerBody := map[string]any{"amount": ..., "currency": ..., "description": ..., "merchantTransactionId": ..., "paymentMethod": method, "paymentInfo": in.PaymentInfo, "options": in.Options, "notify": in.Notify}
```

`providerBody` é um `map[string]any` bruto — não uma `struct` com `json:"...,omitempty"`. O pacote `encoding/json` do Go só omite uma chave com `omitempty` quando ela vem de uma tag de struct; **numa chave de mapa, o valor é sempre serializado**, mesmo vazio ou nulo. Confirmei isto empiricamente (programa Go isolado, fora do repositório) antes de mexer em qualquer coisa:

```go
map[string]any{"paymentInfo": map[string]any{}, "options": map[string]any(nil), "notify": map[string]any(nil)}
// json.Marshal produz:
// {"notify":null,"options":null,"paymentInfo":{}, ...}
```

Ou seja: para REF com **referência gerada pelo gateway** — o caso em que `internal/finance/cobranca_geracao.go` deliberadamente passa `paymentInfo` vazio, exatamente como a documentação instrui ("REF — referência gerada pelo gateway: Spuri omite `paymentInfo`; a AppyPay devolve a referência gerada na resposta") — o Spuri na verdade envia `"paymentInfo": {}` (um objeto presente, vazio), não a ausência do campo. Os exemplos de corpo da documentação, tanto na seção "Post a Charge" quanto na seção "Escopo" (Fase 1), mostram esse caso **sem a chave `paymentInfo` nenhuma** — não com um objeto vazio.

Um objeto vazio presente pode ser lido do lado da AppyPay como "o merchant tentou enviar uma referência própria, mas sem os campos exigidos" em vez de "nenhuma referência própria, gerar automaticamente" — dependendo de como o backend deles distingue "campo ausente" de "objeto vazio" no model binding. Não há como confirmar isto sem uma chamada real à AppyPay (fora do alcance deste sandbox), mas o comportamento correto e mais seguro é, de qualquer forma, espelhar exatamente o que a documentação mostra: omitir a chave. O mesmo valia para `options`/`notify`: nenhum chamador atual de `CreateCharge` (via `gerarCobranca`) os preenche, então eles sempre viravam `"options": null, "notify": null` no corpo — também fora do formato dos exemplos documentados (que simplesmente não incluem a chave quando não há valor).

### Correção

`paymentInfo`/`options`/`notify` só entram no mapa do corpo quando `len(...) > 0`. Adicionei uma asserção de teste de integração (`lastBodyHasKey`, em `cobranca_geracao_integration_test.go`) que verifica, para os dois fluxos que já existiam (mensalidade e matrícula com REF/gateway), que as três chaves ficam **totalmente ausentes** do corpo — não `null`, não `{}` — quando não há conteúdo.

## Reprodução e validação com PostgreSQL/Go reais (não apenas leitura de código)

- Go 1.24.4 (via `apt-get install golang-1.24-go`) e PostgreSQL 16 reais instalados no sandbox; `proxy.golang.org` está bloqueado na rede do sandbox, então usei `replace` temporários em `go.mod` (apontando `golang.org/x/*`, `google.golang.org/protobuf` e `gopkg.in/yaml.v3` para seus espelhos em `github.com/golang/*` / `github.com/protocolbuffers/*` / `github.com/go-yaml/yaml`) só para conseguir compilar aqui — revertidos antes de gerar os arquivos finais; `go.mod`/`go.sum` estão **byte-idênticos** ao original.
- Baseline (antes de qualquer alteração, banco recriado do zero a cada rodada): `go build ./...`, `go vet ./...`, `gofmt -l .` limpos; `go test ./internal/finance/... ./internal/handlers/...` com 230 testes passando, 0 falhas.
- Depois das duas correções: mesma bateria, **380 testes passando em `go test ./...` (repositório inteiro)**, 0 falhas, banco recriado do zero.
- Provei os dois bugs de forma empírica, não só por leitura: reverti temporariamente a correção do webhook (mantendo o payload de teste no formato real) e confirmei a falha exata prevista; reverti de volta e confirmei sucesso. Para o corpo vazio, um programa Go isolado confirmou o comportamento de serialização antes de qualquer mudança no repositório.

## Correção — arquivos alterados

1. `internal/finance/appypay.go` — `CreateCharge` só inclui `paymentInfo`/`options`/`notify` quando não vazios; nova função exportada `IsSuccessfulProviderPayload`.
2. `internal/handlers/financeiro_handlers.go` — `ReceberWebhookAppyPay` passa a usar `finance.IsSuccessfulProviderPayload`; `isSuccessfulWebhook`/`webhookStatus` removidas (sem outros chamadores).
3. `internal/finance/appypay_provider_outcome_test.go` — teste unitário novo para `IsSuccessfulProviderPayload` (formato real de webhook, formato de falha, payload sem informação, compatibilidade com o formato plano usado por mocks de teste existentes).
4. `internal/finance/cobranca_geracao_integration_test.go` — helper `lastBodyHasKey` e asserções novas em `TestIntegrationGerarCobrancaREFNaoEnviaPhoneNumber` (mensalidade e matrícula) provando ausência total de `paymentInfo`/`options`/`notify`.
5. `internal/handlers/financeiro_handlers_integration_test.go` — payload de `TestIntegrationReceberWebhookAppyPayEfetivaVinculoMatricula` trocado do formato plano fictício para o formato real documentado (`responseStatus` aninhado).

Nenhum arquivo precisa ser removido do repositório.

## Fora de escopo (observado, não corrigido — risco/benefício não justifica nesta tarefa)

- **Validação local do tamanho do `phoneNumber` (GPO)** — a documentação exige 9–15 dígitos; o código atual só verifica presença/não-vazio, deixando a AppyPay rejeitar números inválidos. Não é uma não-conformidade (o corpo enviado é sempre o que o usuário forneceu, sem inventar formatação), só uma validação antecipada a menos.
- **Valor mínimo de 1 AOA documentado para GPO** — `validAmount` só exige `> 0`. Mesma lógica: a AppyPay já rejeitaria um valor abaixo do mínimo.
- **REF com `paymentInfo` contendo só `dueDate`** (sem `referenceNumber` nem `nib`) — já vinha marcado no próprio código como hipótese não confirmada contra o ambiente real da AppyPay, e nenhum chamador atual usa essa forma. Mantido como está.

Nenhum destes três itens foi incluído na tarefa de correção — nenhum é uma divergência em relação ao que a documentação descreve, e adicionar validação local extra sem necessidade concreta aumentaria o risco de a Codex mexer em código fora do que foi realmente auditado como quebrado.
