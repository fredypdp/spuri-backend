---
criado: 2026-08-30
origem: conversa com Fredy (Claude como orquestrador — auditoria de conformidade com docs/Parceiros e integrações/AppyPay Documentação.md, PostgreSQL 16 + Go 1.24 reais em sandbox, Codex como executor)
status: pronto_para_execucao
tipo: correcao_de_bug
depende_de: nenhuma
debug: docs/Debbugs/Auditoria de conformidade AppyPay (autenticação e geração de cobrança).md
---

# Tarefa 78 — Corrigir efetivação de matrícula via webhook AppyPay e `paymentInfo`/`options`/`notify` vazios em `POST /charges`

## 0. Leia isto primeiro — sobre esta tarefa e sobre o seu ambiente (Codex)

Claude já implementou, testou e validou esta correção inteira com PostgreSQL 16 real e Go 1.24 real: `go build ./...`, `go vet ./...`, `gofmt -l .` (limpo) e `go test ./...` — **suíte inteira do repositório**, com banco de dados recriado do zero a cada execução — 380 testes passando, 0 falhas, nenhuma asserção pré-existente alterada (só as duas explicitamente descritas nesta tarefa, que passam a testar contra o formato real da AppyPay em vez de um formato fictício).

O seu ambiente (Codex) não tem `psql`/Docker/PostgreSQL e bloqueia `apt` — por isso toda a validação com banco de dados real já foi feita por Claude e está descrita na seção 5. A sua tarefa é mecânica: aplicar os 5 diffs da seção 3, na ordem em que aparecem, e depois rodar o que conseguir do checklist da seção 5.1 — não precisa reinstalar, reconfigurar nem re-decidir nada.

Contexto de como isto surgiu: não é a correção de um bug relatado em produção — é o resultado de uma auditoria pró-ativa pedida por Fredy, comparando a autenticação e a geração de cobrança da AppyPay linha a linha contra `docs/Parceiros e integrações/AppyPay Documentação.md`. A análise completa, incluindo a prova empírica de cada bug (reproduzido e revertido com PostgreSQL real antes de ser corrigido), está em `docs/Debbugs/Auditoria de conformidade AppyPay (autenticação e geração de cobrança).md` — leia se quiser o "porquê" completo; esta tarefa já traz tudo que você precisa para executar sem consultar aquele documento.

## 1. Prompt recomendado para executar esta correção

> Execute exatamente as alterações descritas neste documento, nesta ordem, sobre o `main` atual do repositório `spuri-backend`. Todas as decisões de desenho já foram tomadas e validadas por Claude (implementação testada com `go build`, `go vet`, `gofmt` e a suíte inteira `go test ./...` contra PostgreSQL 16 real e Go 1.24 real, banco recriado do zero a cada execução, 380 testes passando). Sua tarefa é mecânica: (1) aplicar os 5 diffs da seção 3, na ordem em que aparecem, usando `git apply` quando possível ou replicando manualmente o antes/depois quando o diff não aplicar por causa de drift de linha; (2) rodar cada item do checklist da seção 5.1 e reportar o resultado; (3) seguir o "Procedimento de conclusão" (seção 8). Não toque em nenhum arquivo fora dos 5 listados na seção 6 ("Fora de escopo"). Se algo não bater exatamente com o que este documento descreve, pare e reporte antes de tentar consertar por conta própria.

---

## 2. Contexto — os dois bugs, resumidos

**Bug 1 (crítico) — confirmação de matrícula via webhook nunca dispara.** `ReceberWebhookAppyPay` (`internal/handlers/financeiro_handlers.go`, rotas reais `/webhooks/appypay/gpo` e `/webhooks/appypay/ref`) decidia se um webhook era de sucesso lendo `payload["status"]`/`payload["state"]` **na raiz** do JSON. A AppyPay nunca manda o status na raiz de um webhook — ele vem sempre dentro de `responseStatus.status` (documentação, seção "Merchant Webhooks"). Resultado: o bloco que efetiva a matrícula do estudante (`efetivarVinculoMatriculaPaga`) nunca executava para um webhook real — uma matrícula paga via REF/GPO com webhook configurado ficava presa em `aprovada_pendente_pagamento_matricula` para sempre, a menos que alguém consultasse manualmente a cobrança. O pacote `internal/finance` já tinha a extração correta (`extractProviderOutcome`, corrigida na tarefa 70) — só que `handlers` tinha sua própria implementação duplicada e nunca corrigida, guardando especificamente o gatilho de efetivação de matrícula.

**Bug 2 — corpo de `POST /charges` envia campos opcionais vazios em vez de omiti-los.** `CreateCharge` sempre incluía as chaves `paymentInfo`/`options`/`notify` no `map[string]any` do corpo, mesmo vazias. Como é um mapa bruto (não uma `struct` com `omitempty`), `json.Marshal` sempre serializa a chave presente — um `map[string]any{}` vazio vira `"paymentInfo": {}`, não é omitido. Isto importa sobretudo para REF com referência gerada pelo gateway, onde a documentação mostra o corpo **sem a chave `paymentInfo`** — um objeto vazio ali pode ser lido do lado da AppyPay como uma tentativa de referência própria incompleta, em vez de "gerar automaticamente".

Detalhamento completo (causa raiz, prova empírica de cada um, porque os testes existentes não pegaram) em `docs/Debbugs/Auditoria de conformidade AppyPay (autenticação e geração de cobrança).md`.

**Nota — algo que foi revisado e NÃO precisa de correção**: a documentação da AppyPay rotula o endpoint de token OAuth2 como `GET`. O código usa `POST`, corretamente — `POST` é exigido pela RFC 6749 e pela própria plataforma de identidade da Microsoft (`login.microsoftonline.com`), e o sistema já está em produção usando `POST` com sucesso. Não mexa nisto; é uma imprecisão da documentação da AppyPay, não um bug do Spuri. Detalhes em `docs/Debbugs/Auditoria de conformidade AppyPay (autenticação e geração de cobrança).md`.

---

## 3. Diffs a aplicar, na ordem exata

Para cada arquivo: aplicar via `git apply` (colando o diff num arquivo `.patch` e rodando `git apply nome.patch` a partir da raiz do repositório) ou, se o `git apply` falhar por drift de linha, localizar o bloco `-` (removido) e substituir manualmente pelo bloco `+` (adicionado) — o contexto ao redor (linhas sem `+`/`-`) é suficiente para localizar o ponto exato mesmo se os números de linha tiverem mudado.

### 3.1 `internal/finance/appypay.go`

```diff
diff --git a/internal/finance/appypay.go b/internal/finance/appypay.go
index dc13801..19a71b6 100644
--- a/internal/finance/appypay.go
+++ b/internal/finance/appypay.go
@@ -507,7 +507,26 @@ func (s *Service) CreateCharge(ctx context.Context, in ChargeRequest, actorID, a
 		_ = s.releaseChargeReservation(ctx, in.MerchantTransactionID, id)
 		return ChargeResult{}, err
 	}
-	providerBody := map[string]any{"amount": in.Amount, "currency": in.Currency, "description": in.Description, "merchantTransactionId": in.MerchantTransactionID, "paymentMethod": method, "paymentInfo": in.PaymentInfo, "options": in.Options, "notify": in.Notify}
+	providerBody := map[string]any{"amount": in.Amount, "currency": in.Currency, "description": in.Description, "merchantTransactionId": in.MerchantTransactionID, "paymentMethod": method}
+	// paymentInfo/options/notify só entram no corpo quando têm conteúdo real.
+	// providerBody é um map[string]any bruto (não uma struct com `omitempty`),
+	// então json.Marshal sempre serializa toda chave presente no map — mesmo
+	// que o valor seja um map[string]any{} vazio (vira "{}", não é omitido).
+	// Isto importa sobretudo para REF com referência gerada pelo gateway, onde
+	// a documentação da AppyPay mostra o corpo sem a chave "paymentInfo": um
+	// objeto vazio ali pode ser lido pela AppyPay como "o merchant tentou
+	// enviar uma referência própria, mas sem os campos exigidos", em vez de
+	// "nenhuma referência própria, gerar automaticamente". Ver
+	// docs/Debbugs/Auditoria de conformidade AppyPay (autenticação e geração de cobrança).md.
+	if len(in.PaymentInfo) > 0 {
+		providerBody["paymentInfo"] = in.PaymentInfo
+	}
+	if len(in.Options) > 0 {
+		providerBody["options"] = in.Options
+	}
+	if len(in.Notify) > 0 {
+		providerBody["notify"] = in.Notify
+	}
 	response, err := s.callJSON(ctx, credential, http.MethodPost, "/charges", providerBody, in.Async)
 	if err != nil {
 		_ = s.record(ctx, id, "CobrancaAppyPayFalhou", chargePayload(id, in, "", "Failed", map[string]any{"error": "provider_request_failed"}), actorID, actorType, ip)
@@ -1763,6 +1782,25 @@ func extractProviderOutcome(v map[string]any) providerOutcome {
 	return out
 }
 
+// IsSuccessfulProviderPayload informa se um payload cru vindo da AppyPay —
+// seja a resposta síncrona de POST /charges ou /qr-codes, seja o corpo de um
+// webhook — representa um pagamento concluído com sucesso.
+//
+// É a MESMA extração/normalização usada por CreateCharge, CreateGPOQRCode e
+// ConsultCharge (extractProviderOutcome + normalizeChargeStatus), incluindo
+// a leitura correta de "responseStatus.status"/"responseStatus.successful",
+// que é onde a AppyPay realmente coloca o status (nunca em campos soltos
+// "status"/"state" na raiz do payload — ver seção "Merchant Webhooks" de
+// docs/Parceiros e integrações/AppyPay Documentação.md). Qualquer código
+// fora deste pacote que precise reagir a um sucesso da AppyPay (por exemplo,
+// um handler HTTP de webhook decidindo se efetiva uma matrícula) deve
+// chamar esta função em vez de reimplementar sua própria leitura do
+// payload — ver docs/Debbugs/Auditoria de conformidade AppyPay (autenticação
+// e geração de cobrança).md para o bug que isto substitui.
+func IsSuccessfulProviderPayload(payload map[string]any) bool {
+	return isSuccessfulChargeStatus(normalizeChargeStatus(extractProviderOutcome(payload).Status))
+}
+
 func applyResponseStatus(out *providerOutcome, rs map[string]any) {
 	if s, ok := rs["status"].(string); ok {
 		out.Status = s
```

### 3.2 `internal/handlers/financeiro_handlers.go`

```diff
diff --git a/internal/handlers/financeiro_handlers.go b/internal/handlers/financeiro_handlers.go
index 279abc5..c38b94e 100644
--- a/internal/handlers/financeiro_handlers.go
+++ b/internal/handlers/financeiro_handlers.go
@@ -604,7 +604,7 @@ func ReceberWebhookAppyPay(metodo string) gin.HandlerFunc {
 			c.Status(http.StatusInternalServerError)
 			return
 		}
-		if isSuccessfulWebhook(payload) {
+		if finance.IsSuccessfulProviderPayload(payload) {
 			if codigo, err := FinanceiroService.CodigoSolicitacaoDaCobranca(c.Request.Context(), eventID); err == nil && codigo != "" {
 				if err := efetivarVinculoMatriculaPaga(c, codigo); err != nil {
 					c.Status(http.StatusInternalServerError)
@@ -615,17 +615,6 @@ func ReceberWebhookAppyPay(metodo string) gin.HandlerFunc {
 		c.Status(http.StatusOK)
 	}
 }
-func isSuccessfulWebhook(payload map[string]any) bool {
-	return strings.EqualFold(strings.TrimSpace(webhookStatus(payload)), "success")
-}
-func webhookStatus(payload map[string]any) string {
-	for _, k := range []string{"status", "state"} {
-		if v, ok := payload[k].(string); ok {
-			return v
-		}
-	}
-	return ""
-}
 func webhookID(payload map[string]any) string {
 	for _, k := range []string{"id", "merchantTransactionId", "merchant_transaction_id"} {
 		if v, ok := payload[k].(string); ok && strings.TrimSpace(v) != "" {
```

Este arquivo já importa `spuri/internal/finance` (usado em vários outros pontos do mesmo arquivo, ex.: `finance.ContextoAcademia`) e já usa `strings` em outras funções (ex.: `webhookID` logo abaixo) — nenhum import precisa ser adicionado ou removido.

### 3.3 `internal/finance/appypay_provider_outcome_test.go`

```diff
diff --git a/internal/finance/appypay_provider_outcome_test.go b/internal/finance/appypay_provider_outcome_test.go
index 0920499..42add79 100644
--- a/internal/finance/appypay_provider_outcome_test.go
+++ b/internal/finance/appypay_provider_outcome_test.go
@@ -109,6 +109,67 @@ func TestExtractProviderOutcomeSemNenhumaInformacao(t *testing.T) {
 	}
 }
 
+// TestIsSuccessfulProviderPayloadRespondeAoFormatoRealDeWebhookDaAppyPay
+// prova que IsSuccessfulProviderPayload lê corretamente o formato real de
+// webhook da AppyPay — status dentro de "responseStatus", nunca solto na
+// raiz do payload (ver seção "Merchant Webhooks" de docs/Parceiros e
+// integrações/AppyPay Documentação.md) — e não regride para payloads sem
+// "responseStatus" nenhum.
+func TestIsSuccessfulProviderPayloadRespondeAoFormatoRealDeWebhookDaAppyPay(t *testing.T) {
+	casos := []struct {
+		nome    string
+		payload map[string]any
+		quer    bool
+	}{
+		{
+			nome: "webhook real de sucesso (responseStatus aninhado)",
+			payload: map[string]any{
+				"id":                    "56985af8-7256-408c-8e71-99d63dd2074b",
+				"merchantTransactionId": "030000000301201",
+				"amount":                float64(100),
+				"responseStatus": map[string]any{
+					"successful": true,
+					"status":     "Success",
+					"code":       float64(100),
+					"message":    "Transaction Approved",
+					"source":     "GPO",
+				},
+			},
+			quer: true,
+		},
+		{
+			nome: "webhook real de falha (responseStatus aninhado)",
+			payload: map[string]any{
+				"id": "outro-id",
+				"responseStatus": map[string]any{
+					"successful": false,
+					"status":     "Failed",
+					"code":       float64(231),
+					"source":     "GPO",
+				},
+			},
+			quer: false,
+		},
+		{
+			nome:    "payload sem responseStatus e sem status solto: nunca é sucesso",
+			payload: map[string]any{"id": "sem-informacao"},
+			quer:    false,
+		},
+		{
+			nome:    "compatibilidade com status solto na raiz (formato usado por mocks de teste)",
+			payload: map[string]any{"id": "id-legado", "status": "Success"},
+			quer:    true,
+		},
+	}
+	for _, c := range casos {
+		t.Run(c.nome, func(t *testing.T) {
+			if got := IsSuccessfulProviderPayload(c.payload); got != c.quer {
+				t.Fatalf("IsSuccessfulProviderPayload(%#v) = %t, queria %t", c.payload, got, c.quer)
+			}
+		})
+	}
+}
+
 func TestAppyPayCodeOutcomesConsistency(t *testing.T) {
 	estadosValidos := map[string]bool{"Success": true, "Pending": true, "Cancelled": true, "Expired": true, "Failed": true}
 	for code, info := range appyPayCodeOutcomes {
```

### 3.4 `internal/finance/cobranca_geracao_integration_test.go`

```diff
diff --git a/internal/finance/cobranca_geracao_integration_test.go b/internal/finance/cobranca_geracao_integration_test.go
index c408ebd..9d4af28 100644
--- a/internal/finance/cobranca_geracao_integration_test.go
+++ b/internal/finance/cobranca_geracao_integration_test.go
@@ -57,6 +57,19 @@ func (t *capturingAppyPayTransport) paymentInfo() map[string]any {
 	return pi
 }
 
+// lastBodyHasKey reporta se a última requisição POST /charges enviada à
+// AppyPay incluiu literalmente a chave informada no corpo JSON — não apenas
+// se o valor era vazio. Usado para provar que campos opcionais sem conteúdo
+// (paymentInfo/options/notify) são omitidos do corpo, e não enviados como
+// "{}" ou "null" (ver docs/Debbugs/Auditoria de conformidade AppyPay
+// (autenticação e geração de cobrança).md).
+func (t *capturingAppyPayTransport) lastBodyHasKey(key string) bool {
+	t.mu.Lock()
+	defer t.mu.Unlock()
+	_, ok := t.lastBody[key]
+	return ok
+}
+
 // TestIntegrationGerarCobrancaMensalidadeGPOEnviaPhoneNumberNormalizado
 // prova que, após a extração de gerarCobranca (internal/finance/cobranca_geracao.go),
 // o fluxo de mensalidade continua enviando paymentInfo.phoneNumber já sem
@@ -184,6 +197,15 @@ func TestIntegrationGerarCobrancaREFNaoEnviaPhoneNumber(t *testing.T) {
 		if _, ok := pi["phoneNumber"]; ok {
 			t.Fatalf("REF não deveria enviar phoneNumber, obteve paymentInfo=%#v", pi)
 		}
+		// REF com referência gerada pelo gateway: a documentação da AppyPay
+		// mostra o corpo de POST /charges sem a chave "paymentInfo" (nem
+		// vazia). options/notify também não são usados por gerarCobranca e
+		// devem ficar totalmente ausentes, não "null".
+		for _, chave := range []string{"paymentInfo", "options", "notify"} {
+			if transport.lastBodyHasKey(chave) {
+				t.Fatalf("REF com referência gerada pelo gateway não deveria enviar a chave %q, corpo=%#v", chave, transport.lastBody)
+			}
+		}
 	})
 
 	t.Run("matricula", func(t *testing.T) {
@@ -203,5 +225,10 @@ func TestIntegrationGerarCobrancaREFNaoEnviaPhoneNumber(t *testing.T) {
 		if _, ok := pi["phoneNumber"]; ok {
 			t.Fatalf("REF não deveria enviar phoneNumber, obteve paymentInfo=%#v", pi)
 		}
+		for _, chave := range []string{"paymentInfo", "options", "notify"} {
+			if transport.lastBodyHasKey(chave) {
+				t.Fatalf("REF com referência gerada pelo gateway não deveria enviar a chave %q, corpo=%#v", chave, transport.lastBody)
+			}
+		}
 	})
 }
```

### 3.5 `internal/handlers/financeiro_handlers_integration_test.go`

```diff
diff --git a/internal/handlers/financeiro_handlers_integration_test.go b/internal/handlers/financeiro_handlers_integration_test.go
index 87dfcaa..8c53a0b 100644
--- a/internal/handlers/financeiro_handlers_integration_test.go
+++ b/internal/handlers/financeiro_handlers_integration_test.go
@@ -282,7 +282,22 @@ func TestIntegrationReceberWebhookAppyPayEfetivaVinculoMatricula(t *testing.T) {
 	router.POST("/financeiro/appypay/webhooks/ref", ReceberWebhookAppyPay("REF"))
 
 	eventID := charge.Charge.ProviderChargeID
-	payload, _ := json.Marshal(map[string]any{"id": eventID, "status": "Success"})
+	// Formato real de um webhook da AppyPay (ver seção "Merchant Webhooks" de
+	// docs/Parceiros e integrações/AppyPay Documentação.md): o status vem
+	// dentro de "responseStatus", nunca em um campo solto "status"/"state"
+	// na raiz do payload.
+	payload, _ := json.Marshal(map[string]any{
+		"id":                    eventID,
+		"merchantTransactionId": charge.Charge.MerchantTransactionID,
+		"amount":                750,
+		"responseStatus": map[string]any{
+			"successful": true,
+			"status":     "Success",
+			"code":       100,
+			"message":    "Transaction Approved",
+			"source":     "REF",
+		},
+	})
 	postWebhook := func() *httptest.ResponseRecorder {
 		rec := httptest.NewRecorder()
 		req := httptest.NewRequest(http.MethodPost, "/financeiro/appypay/webhooks/ref", bytes.NewReader(payload))
```

---

## 4. Resumo do que cada diff faz (para revisão, não para re-decidir nada)

1. **3.1** — `CreateCharge` só inclui `paymentInfo`/`options`/`notify` no corpo enviado à AppyPay quando têm conteúdo real; nova função exportada `IsSuccessfulProviderPayload`, que reaproveita a extração já testada (`extractProviderOutcome` + `normalizeChargeStatus` + `isSuccessfulChargeStatus`) para decidir se um payload cru da AppyPay representa sucesso.
2. **3.2** — `ReceberWebhookAppyPay` passa a chamar `finance.IsSuccessfulProviderPayload` em vez da função local quebrada; `isSuccessfulWebhook`/`webhookStatus` são removidas (confirmadas sem nenhum outro chamador no repositório).
3. **3.3** — teste unitário novo, cobrindo os 4 formatos de payload relevantes para `IsSuccessfulProviderPayload`.
4. **3.4** — helper de teste `lastBodyHasKey` + asserções novas provando que `paymentInfo`/`options`/`notify` ficam totalmente ausentes do corpo de `POST /charges` quando vazios, nos dois fluxos (mensalidade e matrícula) que já passam por REF com referência gerada pelo gateway.
5. **3.5** — o teste que já existia especificamente para "webhook efetiva matrícula" passa a simular o payload real da AppyPay (`responseStatus` aninhado) em vez de um formato plano fictício que nunca ocorre em produção.

---

## 5. Validação já executada por Claude (com PostgreSQL 16 real e Go 1.24 real)

- Go 1.24.4, PostgreSQL 16, banco `spuri_test` recriado do zero (`DROP DATABASE` + `CREATE DATABASE`) antes de cada rodada de suíte completa, para eliminar qualquer resíduo entre execuções.
- **Baseline, antes de qualquer alteração**: `go build ./...` limpo, `go vet ./...` limpo, `go test ./internal/finance/... ./internal/handlers/...` → 230 testes, 0 falhas.
- **Prova de que os dois bugs são reais** (não suposição): reverti temporariamente só a correção do diff 3.2 (mantendo o payload de teste do diff 3.5 já no formato real) e rodei `TestIntegrationReceberWebhookAppyPayEfetivaVinculoMatricula` — falhou exatamente como previsto: `status da solicitação = "aprovada_pendente_pagamento_matricula", queria aprovada`. Reapliquei a correção e o mesmo teste passou, incluindo o segundo POST do mesmo webhook (idempotência) e a checagem de que nenhum estudante duplicado foi criado. Para o corpo vazio (diff 3.1), um programa Go isolado (fora do repositório) confirmou o comportamento de serialização do `encoding/json` antes de qualquer mudança.
- **Depois de aplicar os 5 diffs**: `go build ./...` limpo, `go vet ./...` limpo, `gofmt -l .` sem nenhuma linha (formatação já correta), `go test -count=1 ./...` (repositório inteiro, sem cache) → **380 testes passando, 0 falhas**, banco recriado do zero.
- `go.mod`/`go.sum` ficam **byte-idênticos** ao estado atual do `main` — nenhuma dependência foi adicionada, removida ou trocada de versão.

### 5.1 Checklist para você (Codex) rodar depois de aplicar os 5 diffs

1. `gofmt -l internal/finance/appypay.go internal/finance/appypay_provider_outcome_test.go internal/finance/cobranca_geracao_integration_test.go internal/handlers/financeiro_handlers.go internal/handlers/financeiro_handlers_integration_test.go` — deve devolver **vazio** (nenhuma linha). Se devolver algo, rode `gofmt -w` nesses arquivos e confira o diff resultante contra a seção 3 antes de prosseguir.
2. `go build ./...` — deve compilar sem erro. Se der erro de `undefined: isSuccessfulWebhook`/`webhookStatus` em algum lugar não listado nesta tarefa, pare e reporte — significa que existe um chamador que Claude não encontrou (busca feita: `grep -rn "isSuccessfulWebhook\|webhookStatus\b" --include="*.go" .` no repositório inteiro, zero resultados fora dos dois pontos já tratados nos diffs 3.1/3.2).
3. `go vet ./...` — deve rodar sem apontar nada.
4. Sem PostgreSQL disponível no seu ambiente, `go test ./...` vai pular (`SKIP`) todos os testes de integração (os que chamam `integrationClient`/`integrationFinanceClient`, que exigem `RUN_POSTGRES_INTEGRATION=1`) — isso é esperado e não é falha. O que você **consegue** validar sem banco: `go test ./internal/finance/... -run TestIsSuccessfulProviderPayloadRespondeAoFormatoRealDeWebhookDaAppyPay -v` (teste novo do diff 3.3, unitário, sem banco) deve passar com as 4 subasserções em verde.
5. Se, por qualquer motivo, você tiver acesso a um PostgreSQL neste ambiente (não esperado, mas caso mude): repita exatamente a validação da seção 5 (`go test -count=1 ./...` com o banco recriado do zero antes) e confirme 380 testes passando, 0 falhas.

---

## 6. Fora de escopo — não toque em nenhum destes

- Qualquer arquivo além dos 5 listados na seção 3.
- O endpoint de token continuar usando `POST` (não mudar para `GET` — ver seção 2, nota final).
- Validação local de tamanho do `phoneNumber` do GPO, valor mínimo de 1 AOA, ou o caminho hipotético de REF com só `dueDate` em `paymentInfo` — observados na auditoria, deliberadamente deixados de fora por não serem uma divergência real em relação à documentação (detalhes em `docs/Debbugs/Auditoria de conformidade AppyPay (autenticação e geração de cobrança).md`, seção "Fora de escopo").
- Qualquer outro teste de integração que use `appyPayMockTransport`/`handlerAppyPayMockTransport` com payload em formato plano (fora dos dois testes explicitamente tratados nos diffs 3.4/3.5) — não fazem parte desta auditoria e mexer neles sem necessidade aumenta o risco de regressão fora do escopo revisado.
- `go.mod`/`go.sum` — não devem mudar.

---

## 7. Critérios de aceite

- [ ] Os 5 diffs da seção 3 aplicados, byte a byte, sem nenhuma alteração fora deles.
- [ ] `gofmt -l` vazio para os 5 arquivos.
- [ ] `go build ./...` e `go vet ./...` limpos.
- [ ] `go test ./internal/finance/... -run TestIsSuccessfulProviderPayloadRespondeAoFormatoRealDeWebhookDaAppyPay -v` passando (4/4 subtestes).
- [ ] Se houver PostgreSQL disponível: `go test -count=1 ./...` com banco recriado do zero → 380 testes, 0 falhas.
- [ ] `go.mod`/`go.sum` sem nenhuma alteração.

---

## 8. Procedimento de conclusão

1. Rode o checklist da seção 5.1 e reporte o resultado de cada item para Fredy, exatamente como saiu (sem resumir/reformular).
2. Se tudo estiver verde, mova este arquivo de `docs/Lista de Tarefas/` para `docs/Tarefas feitas/`, atualizando o frontmatter: `status: concluido` e adicione `concluido: <data de hoje>`.
3. Não altere o frontmatter de `docs/Debbugs/Auditoria de conformidade AppyPay (autenticação e geração de cobrança).md` — isso é feito por Claude na próxima revisão.
4. Se qualquer item do checklist não bater exatamente com o esperado (ex.: `go build` aponta um erro não previsto nesta tarefa), **pare e reporte o erro exato** para Fredy antes de tentar corrigir por conta própria — a menos que seja um erro mecânico trivial e óbvio (ex.: um `gofmt` não aplicado), caso em que pode corrigir e seguir, relatando o que foi feito.
