---
criado: 2026-08-30
origem: conversa com Fredy (Claude como orquestrador — segunda rodada de auditoria de conformidade com docs/Parceiros e integrações/AppyPay Documentação.md, desta vez focada em prontidão para produção do módulo de pagamentos como um todo; PostgreSQL 16 + Go 1.24 reais em sandbox; Codex como executor)
status: concluido
concluido: 2026-08-30
tipo: correcao_de_bug
depende_de: "78 - Reprecificar cobrancas pendentes de mensalidade e matricula (todos os meses) ou so a partir da atualizacao.md" (já concluída e mergeada — este documento foi escrito e validado sobre o main atual, que já inclui essa tarefa)
debug: docs/Debbugs/Auditoria de conformidade AppyPay (autenticação e geração de cobrança).md
---

# Tarefa 79 — Corrigir `expires_in` do token AppyPay, exigir confirmação ao vivo antes de efetivar um webhook, e reparar seeds de teste quebrados pela tarefa 78

## 0. Leia isto primeiro — sobre esta tarefa e sobre o seu ambiente (Codex)

Claude já implementou, testou e validou esta correção inteira com PostgreSQL 16 real e Go 1.24 real: `go build ./...`, `go vet ./...`, `gofmt -l .` (limpo) e `go test ./...` — **suíte inteira do repositório**, com banco de dados recriado do zero a cada execução — 387 testes passando, 0 falhas, contra o `main` atual (já incluindo a tarefa 78, mergeada em `#589` enquanto esta tarefa era escrita).

O seu ambiente (Codex) não tem `psql`/Docker/PostgreSQL e bloqueia `apt` — por isso toda a validação com banco de dados real já foi feita por Claude e está descrita na seção 5. A sua tarefa é mecânica: aplicar os 11 diffs da seção 3, na ordem em que aparecem, e depois rodar o que conseguir do checklist da seção 5.1 — não precisa reinstalar, reconfigurar nem re-decidir nada.

**Contexto de como isto surgiu.** Esta é a **segunda rodada** da mesma auditoria pró-ativa da tarefa 77 (comparação linha a linha da autenticação e geração de cobrança da AppyPay contra `docs/Parceiros e integrações/AppyPay Documentação.md`), desta vez pedida por Fredy especificamente para checar se **o módulo de pagamentos como um todo está pronto para produção** — não só "está de acordo com a documentação". A tarefa 77 corrigiu dois bugs (efetivação de matrícula via webhook e `paymentInfo` vazio em `POST /charges`) e já foi mergeada. Esta tarefa encontra e corrige **mais dois problemas reais**, mais **um achado colateral bloqueante**:

- **Bug 1 (crítico, produção)** — o campo `expires_in` da resposta do token OAuth2 vem como **string** no endpoint real da AppyPay, não como número — o código atual declara um campo `int`, o que quebra a autenticação inteira.
- **Bug 2 (produção, robustez)** — a documentação da AppyPay recomenda explicitamente confirmar um webhook de sucesso com um `GET /charges/{id}` antes de aplicar um efeito de negócio irreversível (confirmar mensalidade paga, efetivar matrícula). O código atual aplicava esses efeitos confiando cegamente no que o webhook reportava.
- **Achado colateral (bloqueante para validar qualquer coisa)** — a tarefa 78, já mergeada, adicionou a coluna `sequencia BIGINT NOT NULL` (sem default) em `financeiro_mensalidade_configuracoes` e tornou `modo_vigencia` obrigatório em `ConfigureMensalidade`/`ConfigureMatricula`, mas **dois helpers de seed usados por dezenas de testes não foram atualizados** — rodar `go test ./...` no `main` atual (antes desta tarefa) produz **37 falhas** que nada têm a ver com AppyPay. Sem corrigir isto primeiro não dá para confiar em nenhuma validação de suíte completa daqui para frente — nem desta tarefa, nem de nenhuma futura.

Detalhamento completo (causa raiz, prova empírica de cada item, por que os testes existentes não pegaram) está na seção 2 abaixo e, com mais profundidade técnica, deve ser adicionado por Claude a `docs/Debbugs/Auditoria de conformidade AppyPay (autenticação e geração de cobrança).md` na próxima revisão — não é pré-requisito para você executar esta tarefa.

## 1. Prompt recomendado para executar esta correção

> Execute exatamente as alterações descritas neste documento, nesta ordem, sobre o `main` atual do repositório `spuri-backend`. Todas as decisões de desenho já foram tomadas e validadas por Claude (implementação testada com `go build`, `go vet`, `gofmt` e a suíte inteira `go test ./...` contra PostgreSQL 16 real e Go 1.24 real, banco recriado do zero a cada execução, 387 testes passando). Sua tarefa é mecânica: (1) aplicar os 11 diffs da seção 3, na ordem em que aparecem, usando `git apply` quando possível ou replicando manualmente o antes/depois quando o diff não aplicar por causa de drift de linha; (2) rodar cada item do checklist da seção 5.1 e reportar o resultado; (3) seguir o "Procedimento de conclusão" (seção 8). Não toque em nenhum arquivo fora dos 11 listados na seção 6 ("Fora de escopo"). Se algo não bater exatamente com o que este documento descreve, pare e reporte antes de tentar consertar por conta própria.

---

## 2. Contexto — os problemas, resumidos

### Bug 1 (crítico) — `expires_in` do token vem como string, código exige número

`internal/finance/appypay.go`, função `token()`: a resposta do endpoint de token (`login.microsoftonline.com/{tenant}/oauth2/token` — o endpoint **v1** do Azure AD, que é o que a AppyPay usa) tem, no exemplo da própria documentação da AppyPay (secção "Get a token"), o campo `"expires_in": "3599"` — uma **string JSON**, não um número. Isto não é imprecisão de documentação: é o comportamento real e conhecido do endpoint v1 do Azure AD (diferente do endpoint v2.0, que usa número) — confirmado por relatos externos consistentes de outros integradores com o mesmo endpoint.

O código declarava `ExpiresIn int`. Provado com um programa Go isolado e depois reproduzido dentro do próprio pacote `finance` (com um transport HTTP simulando a resposta real, string incluída): `json.Unmarshal` de `{"expires_in":"3599","access_token":"..."}` para essa struct retorna um erro de tipo (`cannot unmarshal string into Go struct field ... of type int`) — **mesmo com `access_token` decodificado corretamente no mesmo payload**. A condição do código, `if err = json.Unmarshal(raw, &out); err != nil || out.AccessToken == ""`, trata qualquer erro de unmarshal como falha de autenticação total. Resultado: **toda tentativa real de obter um token falharia**, derrubando `CreateCharge`, `CreateGPOQRCode`, `ConsultCharge` e a validação de webhook — o módulo inteiro.

Todos os mocks de teste existentes (`appypay_integration_test.go`, `financeiro_handlers_integration_test.go`, etc.) simulam `"expires_in":3600` como **número**, nunca como string — o mesmo padrão de "mock testando a premissa errada sobre o contrato real da AppyPay" já identificado e corrigido nas tarefas 70 e 77 (para outros campos).

### Bug 2 (produção) — webhook de sucesso nunca é confirmado ao vivo antes de um efeito irreversível

A documentação da AppyPay é explícita, duas vezes, sobre isto: na secção "Merchant Webhooks" ("**Important**: As a security measure, double check the transaction by calling the `GET /charges` endpoint") e na secção interna "Escopo do Módulo Financeiro Base" ("Ao receber, é recomendado confirmar o estado com um `GET /charges/{id}` antes de aplicar efeitos de negócio irreversíveis"). Os dois efeitos irreversíveis do sistema — `confirmMensalidadeCharge` (marca uma mensalidade como paga) e `efetivarVinculoMatriculaPaga` (efetiva a matrícula do estudante) — eram disparados **direto a partir do payload do webhook recebido**, sem nenhuma confirmação ao vivo. Não existe hoje um fluxo automatizado de estorno/reversão (`Post a charge refund`/`Post a charge reverse` estão fora de escopo, per a secção "Escopo" da documentação interna), então uma confirmação indevida não tem correção automática — só reconciliação manual.

A correção adiciona uma consulta `GET /charges/{id}` (reaproveitando exatamente o mesmo caminho HTTP que `ConsultCharge` já usa) **antes** de tratar um webhook de sucesso "normal" (cobrança não cancelada localmente) como definitivo. Dois cuidados deliberados, validados com testes dedicados:

- Se a consulta ao vivo **falhar** (upstream indisponível, timeout), cai para confiar no webhook — exatamente o comportamento anterior a esta correção — em vez de bloquear a confirmação. Um `GET` indisponível não pode deixar uma cobrança presa para sempre sem confirmar um pagamento que a própria AppyPay já reportou como bem-sucedido no webhook (o mesmo raciocínio do Bug 1 da tarefa 77, em que o webhook nunca efetivava a matrícula).
- O caso já existente de "webhook de sucesso chegando depois de um cancelamento local" (que já tem tratamento próprio — grava um conflito para reconciliação manual, sem disparar nenhum efeito irreversível) **não** passa pelo double-check — a primeira versão desta correção reaproveitava ingenuamente a função interna `consultCharge` para todo o caminho de sucesso, e isso **quebrou exatamente esse teste já existente** (`TestIntegrationMatriculaWebhookTardioMantemCancelamentoERegistraConflito`): uma consulta ao vivo devolvendo um status não-terminal fazia `consultCharge` "ressuscitar" uma cobrança já cancelada de volta para `aguardando_pagamento`. A versão final (nos diffs abaixo) só ativa o double-check no caminho normal de sucesso, preservando 100% do comportamento já testado do caso de conflito pós-cancelamento.

`AcceptWebhook` passa a devolver um terceiro valor, `confirmedSuccess`, usado tanto internamente (gatilho de `confirmMensalidadeCharge`) quanto pelo handler HTTP (gatilho de `efetivarVinculoMatriculaPaga`, que antes decidia direto a partir de `finance.IsSuccessfulProviderPayload(payload)` — o payload cru do webhook, sem qualquer confirmação ao vivo).

### Achado colateral — seeds de teste quebrados pela tarefa 78 (bloqueante para validar qualquer coisa)

Ao rodar a suíte completa contra o `main` atual (já com a tarefa 78 mergeada) para validar os dois bugs acima, apareceram **37 falhas** sem relação nenhuma com AppyPay. Causa raiz: a migração `116_financeiro_mensalidade_modo_vigencia.sql` (tarefa 78) adicionou `sequencia BIGINT NOT NULL` sem valor default em `financeiro_mensalidade_configuracoes` — em produção, sempre populada a partir de `spuri_ledger.id` pela projeção — mas dois helpers de seed usados por dezenas de testes (`seedMensalidadeConfiguracao` em `internal/finance/mensalidade_integration_test.go` e `seedMensalidadeConfigParaHTTP` em `internal/handlers/financeiro_pendencias_handlers_test.go`) inserem direto por SQL sem popular essa coluna, violando a constraint `NOT NULL`. Mais 8 falhas adicionais vinham de chamadas diretas a `ConfigureMensalidade`/`ConfigureMatricula` em testes que não passavam o novo campo obrigatório `ModoVigencia` (a mesma tarefa 78 tornou esse campo obrigatório).

Nenhuma destas 8+2 correções é uma decisão de desenho nova — são mecânicas: os seeds passam a reservar um valor real de `spuri_ledger_id_seq` (a mesma sequence que a produção usa como fonte de ordem cronológica, sem precisar inserir uma linha de ledger só para o teste), e os 15 call sites de configuração passam a informar `ModoVigencia: ModoVigenciaAPartirDaAtualizacao` (o valor que preserva exatamente o comportamento que esses testes sempre validaram, per a própria documentação da tarefa 78: "é exatamente o comportamento que o sistema já tem hoje, sem nenhuma mudança"). Sem isto, `go test ./...` nunca sai limpo no `main` atual — nem para validar esta tarefa, nem nenhuma futura.

---

## 3. Diffs a aplicar, na ordem exata

Para cada arquivo: aplicar via `git apply` (colando o diff num arquivo `.patch` e rodando `git apply nome.patch` a partir da raiz do repositório) ou, se o `git apply` falhar por drift de linha, localizar o bloco `-` (removido) e substituir manualmente pelo bloco `+` (adicionado) — o contexto ao redor (linhas sem `+`/`-`) é suficiente para localizar o ponto exato mesmo se os números de linha tiverem mudado.

**Nenhum destes 11 diffs precisa de import novo** — `strconv` e `net/url` já estavam importados em `internal/finance/appypay.go` antes desta tarefa (usados em outras partes do mesmo arquivo); todos os outros arquivos só usam identificadores já importados.

### 3.1 `internal/finance/appypay.go` — Bug 1 + Bug 2 (núcleo da correção)

```diff
diff --git a/internal/finance/appypay.go b/internal/finance/appypay.go
index 19a71b6..9c4eb9c 100644
--- a/internal/finance/appypay.go
+++ b/internal/finance/appypay.go
@@ -1320,8 +1320,8 @@ func (s *Service) token(ctx context.Context, cred credentialSecrets) (string, er
 		return "", fmt.Errorf("%w: token AppyPay recusado: HTTP %d", ErrUpstream, res.StatusCode)
 	}
 	var out struct {
-		AccessToken string `json:"access_token"`
-		ExpiresIn   int    `json:"expires_in"`
+		AccessToken string          `json:"access_token"`
+		ExpiresIn   flexibleSeconds `json:"expires_in"`
 	}
 	if err = json.Unmarshal(raw, &out); err != nil || out.AccessToken == "" {
 		return "", fmt.Errorf("%w: resposta de token AppyPay inválida", ErrUpstream)
@@ -1335,6 +1335,40 @@ func (s *Service) token(ctx context.Context, cred credentialSecrets) (string, er
 	return out.AccessToken, nil
 }
 
+// flexibleSeconds decodifica um campo de segundos (usado aqui para
+// expires_in) que a AppyPay pode enviar como número JSON puro OU como
+// string JSON contendo um número. O endpoint real de token da AppyPay
+// (login.microsoftonline.com/{tenant}/oauth2/token, o endpoint "v1" do
+// Azure AD, não o "v2.0") devolve expires_in como STRING
+// (ex.: "expires_in": "3599") — documentado no exemplo de resposta da
+// secção "Get a token" e comportamento conhecido do próprio Azure AD v1
+// (distinto do endpoint v2.0, que usa número). Antes desta correção o
+// campo era declarado como int puro: json.Unmarshal de uma string JSON
+// para um campo int retorna erro, e token() tratava qualquer erro de
+// unmarshal como falha de autenticação total — mesmo com access_token
+// presente e válido no mesmo payload. Ver
+// docs/Debbugs/Auditoria de conformidade AppyPay — autenticação e geração
+// de cobrança (produção).md.
+type flexibleSeconds int
+
+func (f *flexibleSeconds) UnmarshalJSON(b []byte) error {
+	var asInt int
+	if err := json.Unmarshal(b, &asInt); err == nil {
+		*f = flexibleSeconds(asInt)
+		return nil
+	}
+	var asString string
+	if err := json.Unmarshal(b, &asString); err != nil {
+		return err
+	}
+	n, err := strconv.Atoi(strings.TrimSpace(asString))
+	if err != nil {
+		return err
+	}
+	*f = flexibleSeconds(n)
+	return nil
+}
+
 func (s *Service) record(ctx context.Context, id uuid.UUID, event string, payload map[string]any, userID, userType, ip string) error {
 	if strings.TrimSpace(userID) == "" {
 		return errors.New("autor do evento financeiro é obrigatório")
@@ -1435,26 +1469,62 @@ func (s *Service) AuthenticateWebhook(ctx context.Context, headers http.Header)
 	return WebhookOwner{}, errors.New("webhook não autenticado")
 }
 
+// liveChargeStatus faz uma consulta ao vivo (GET /charges/{id}) e devolve o
+// status autoritativo reportado pela AppyPay nesse exato momento. É a
+// medida de segurança que a própria documentação da AppyPay recomenda antes
+// de tratar um webhook de sucesso como definitivo — "Important: As a
+// security measure, double check the transaction by calling the GET
+// /charges endpoint" (secção "Merchant Webhooks"), reforçada na secção
+// interna "Escopo do Módulo Financeiro Base": "Ao receber, é recomendado
+// confirmar o estado com um GET /charges/{id} antes de aplicar efeitos de
+// negócio irreversíveis".
+//
+// Não persiste nada por si própria — quem chama decide o que gravar a
+// partir do status devolvido. O segundo valor devolvido é false quando a
+// consulta em si falhou (upstream indisponível, timeout, credencial
+// temporariamente inacessível) ou não devolveu nenhum status reconhecível
+// — nesses casos quem chama deve cair para confiar no que o webhook
+// reportou, em vez de bloquear a confirmação: um GET indisponível não pode
+// deixar uma cobrança presa sem nunca confirmar um pagamento que a própria
+// AppyPay já reportou como bem-sucedido no webhook (o mesmo raciocínio do
+// Bug 1 já corrigido, em que o webhook nunca efetivava a matrícula).
+func (s *Service) liveChargeStatus(ctx context.Context, charge chargeRow) (status string, ok bool) {
+	cred, err := s.loadCredential(ctx, charge.Contexto, charge.Academia)
+	if err != nil {
+		return "", false
+	}
+	path := "/charges/" + url.PathEscape(charge.ProviderID)
+	if charge.ProviderID == "" {
+		path = "/charges?merchantTransactionId=" + url.QueryEscape(charge.Merchant)
+	}
+	response, err := s.callJSON(ctx, cred, http.MethodGet, path, nil, false)
+	if err != nil {
+		return "", false
+	}
+	status = normalizeChargeStatus(extractProviderOutcome(response).Status)
+	return status, status != ""
+}
+
 // AcceptWebhook reserves its event id first in the dedicated idempotency index.
 // If ledger persistence fails the reservation is removed, so a delivery retry is
 // still processed. No charge side-effect is executed here.
-func (s *Service) AcceptWebhook(ctx context.Context, metodo, eventID string, owner WebhookOwner, payload map[string]any) (bool, error) {
+func (s *Service) AcceptWebhook(ctx context.Context, metodo, eventID string, owner WebhookOwner, payload map[string]any) (accepted bool, confirmedSuccess bool, err error) {
 	metodo = strings.ToUpper(metodo)
 	if (metodo != "GPO" && metodo != "REF") || strings.TrimSpace(eventID) == "" {
-		return false, errors.New("webhook inválido")
+		return false, false, errors.New("webhook inválido")
 	}
 	res, err := s.client.DB().ExecContext(ctx, `INSERT INTO financeiro_webhooks_recebidos(event_id,metodo) VALUES($1,$2) ON CONFLICT(event_id) DO NOTHING`, eventID, metodo)
 	if err != nil {
-		return false, err
+		return false, false, err
 	}
 	affected, _ := res.RowsAffected()
 	if affected == 0 {
-		return false, nil
+		return false, false, nil
 	}
 	data := map[string]any{"event_id": eventID, "metodo": metodo, "credential_id": owner.CredentialID.String(), "contexto_tipo": owner.ContextoTipo, "codigo_academia": owner.CodigoAcademia, "payload": sanitize(payload)}
 	if err = s.record(ctx, uuid.New(), "WebhookAppyPayRecebido", data, "appypay:webhook", "sistema", "webhook"); err != nil {
 		_, _ = s.client.DB().ExecContext(ctx, `DELETE FROM financeiro_webhooks_recebidos WHERE event_id=$1`, eventID)
-		return false, err
+		return false, false, err
 	}
 	// Reflete no read model qualquer estado que o webhook reporte — sucesso
 	// (Success) ou qualquer um dos outros três estados terminais que a
@@ -1474,6 +1544,19 @@ func (s *Service) AcceptWebhook(ctx context.Context, metodo, eventID string, own
 			// cancelada) — só um sucesso tem tratamento de conflito próprio
 			// (abaixo) que pode correr por cima de um estado terminal local.
 			(success || !isTerminalChargeStatus(charge.Status)) {
+			// Double-check: um webhook de sucesso "normal" (a cobrança não
+			// está cancelada localmente) só é tratado como definitivo depois
+			// de uma consulta ao vivo concordar. O caso de sucesso chegando
+			// depois de um cancelamento local (abaixo, eventType
+			// CobrancaAppyPayConflitoPosCancelamento) já não dispara nenhum
+			// efeito irreversível por si só — é só um registo de conflito
+			// para reconciliação manual — então não precisa do double-check.
+			if success && !strings.EqualFold(charge.Status, "cancelada") {
+				if live, ok := s.liveChargeStatus(ctx, charge); ok {
+					normalized = live
+					success = isSuccessfulChargeStatus(live)
+				}
+			}
 			updated := make(map[string]any, len(charge.Payload)+7)
 			for k, v := range charge.Payload {
 				updated[k] = v
@@ -1495,11 +1578,12 @@ func (s *Service) AcceptWebhook(ctx context.Context, metodo, eventID string, own
 				eventType = "CobrancaAppyPayConflitoPosCancelamento"
 			}
 			if s.record(ctx, charge.ID, eventType, updated, "appypay:webhook", "sistema", "webhook") == nil && success && eventType == "CobrancaAppyPayConsultada" {
+				confirmedSuccess = true
 				_ = s.confirmMensalidadeCharge(ctx, charge.ID, "appypay:webhook", "sistema", "webhook")
 			}
 		}
 	}
-	return true, nil
+	return true, confirmedSuccess, nil
 }
 
 type chargeRow struct {
```

### 3.2 `internal/handlers/financeiro_handlers.go` — usa `confirmedSuccess` em vez do payload cru

```diff
diff --git a/internal/handlers/financeiro_handlers.go b/internal/handlers/financeiro_handlers.go
index c38b94e..2bb4cb4 100644
--- a/internal/handlers/financeiro_handlers.go
+++ b/internal/handlers/financeiro_handlers.go
@@ -600,11 +600,12 @@ func ReceberWebhookAppyPay(metodo string) gin.HandlerFunc {
 			c.Status(http.StatusBadRequest)
 			return
 		}
-		if _, err := FinanceiroService.AcceptWebhook(c.Request.Context(), metodo, eventID, owner, payload); err != nil {
+		_, confirmedSuccess, err := FinanceiroService.AcceptWebhook(c.Request.Context(), metodo, eventID, owner, payload)
+		if err != nil {
 			c.Status(http.StatusInternalServerError)
 			return
 		}
-		if finance.IsSuccessfulProviderPayload(payload) {
+		if confirmedSuccess {
 			if codigo, err := FinanceiroService.CodigoSolicitacaoDaCobranca(c.Request.Context(), eventID); err == nil && codigo != "" {
 				if err := efetivarVinculoMatriculaPaga(c, codigo); err != nil {
 					c.Status(http.StatusInternalServerError)
```

### 3.3 `internal/finance/appypay_test.go` — teste unitário novo para o Bug 1

```diff
diff --git a/internal/finance/appypay_test.go b/internal/finance/appypay_test.go
index 8ad8193..4df8bf0 100644
--- a/internal/finance/appypay_test.go
+++ b/internal/finance/appypay_test.go
@@ -1,7 +1,10 @@
 package finance
 
 import (
+	"context"
 	"encoding/json"
+	"io"
+	"net/http"
 	"os"
 	"strings"
 	"testing"
@@ -302,6 +305,52 @@ func TestAppyPayResourceConfig(t *testing.T) {
 	}
 }
 
+// appyPayTokenTransport simula a resposta do endpoint de token da AppyPay
+// com um corpo de expires_in configurável, para exercitar token() com os
+// dois formatos que o campo pode assumir na prática.
+type appyPayTokenTransport struct{ expiresInJSON string }
+
+func (t appyPayTokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
+	body := `{"access_token":"tok-` + uuid.NewString() + `","expires_in":` + t.expiresInJSON + `}`
+	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
+}
+
+// TestTokenAceitaExpiresInComoStringOuNumero cobre o formato real da
+// resposta do endpoint de token da AppyPay (secção "Get a token" de
+// docs/Parceiros e integrações/AppyPay Documentação.md): expires_in vem
+// como STRING JSON (ex.: "expires_in": "3599") — comportamento do endpoint
+// v1 do Azure AD (login.microsoftonline.com/{tenant}/oauth2/token, que é o
+// que a AppyPay usa), diferente do endpoint v2.0, que usa número. Antes
+// desta correção o campo era um int puro: json.Unmarshal de uma string
+// JSON para um campo int falha, e token() tratava qualquer erro de
+// unmarshal como falha de autenticação total — mesmo com access_token
+// presente e válido no mesmo payload. Cobre também o formato numérico puro,
+// usado por todos os mocks de teste deste pacote, para garantir que a
+// correção não regride o que já funcionava.
+func TestTokenAceitaExpiresInComoStringOuNumero(t *testing.T) {
+	t.Setenv("APPYPAY_RESOURCE", "integration-resource")
+	casos := []struct {
+		nome          string
+		expiresInJSON string
+	}{
+		{"string, formato real da AppyPay", `"3599"`},
+		{"número, formato usado pelos mocks de teste", `3600`},
+	}
+	for _, c := range casos {
+		t.Run(c.nome, func(t *testing.T) {
+			s := NewService(nil)
+			s.SetHTTPClient(&http.Client{Transport: appyPayTokenTransport{expiresInJSON: c.expiresInJSON}})
+			token, err := s.token(context.Background(), credentialSecrets{ID: uuid.New(), ClientID: "client-x", ClientSecret: "secret-y"})
+			if err != nil {
+				t.Fatalf("token() falhou com expires_in=%s: %v", c.expiresInJSON, err)
+			}
+			if token == "" {
+				t.Fatal("token vazio")
+			}
+		})
+	}
+}
+
 func TestCredentialMethodMatchesConfiguredIDWithoutCaseSensitivity(t *testing.T) {
 	credentials := credentialSecrets{GPO: "GPO_53c70da3-1c88", REF: "REF_42ef"}
 	if got, err := credentials.method("gpo_53C70DA3-1C88"); err != nil || got != credentials.GPO {
```

### 3.4 `internal/finance/appypay_integration_test.go` — 2 call sites atualizados para o novo retorno de 3 valores + 3 testes novos para o Bug 2

```diff
diff --git a/internal/finance/appypay_integration_test.go b/internal/finance/appypay_integration_test.go
index 721ba3c..4c8005f 100644
--- a/internal/finance/appypay_integration_test.go
+++ b/internal/finance/appypay_integration_test.go
@@ -123,11 +123,11 @@ func TestIntegrationAcceptWebhookIsIdempotent(t *testing.T) {
 	owner := WebhookOwner{CredentialID: uuid.New(), ContextoTipo: ContextoAcademia, CodigoAcademia: "INTWEBHOOK"}
 	payload := map[string]any{"id": eventID, "status": "Paid"}
 
-	accepted, err := service.AcceptWebhook(context.Background(), "GPO", eventID, owner, payload)
+	accepted, _, err := service.AcceptWebhook(context.Background(), "GPO", eventID, owner, payload)
 	if err != nil || !accepted {
 		t.Fatalf("primeiro webhook = accepted %t, err %v", accepted, err)
 	}
-	accepted, err = service.AcceptWebhook(context.Background(), "GPO", eventID, owner, payload)
+	accepted, _, err = service.AcceptWebhook(context.Background(), "GPO", eventID, owner, payload)
 	if err != nil || accepted {
 		t.Fatalf("webhook repetido = accepted %t, err %v", accepted, err)
 	}
@@ -205,7 +205,7 @@ func TestIntegrationMatriculaWebhookTardioMantemCancelamentoERegistraConflito(t
 	if err = service.CancelarCobrancaMatriculaAberta(context.Background(), codigo, "solicitação cancelada", uuid.NewString(), "academia", "127.0.0.1"); err != nil {
 		t.Fatal(err)
 	}
-	accepted, err := service.AcceptWebhook(context.Background(), "REF", charge.Charge.ProviderChargeID, WebhookOwner{CredentialID: uuid.New(), ContextoTipo: ContextoAcademia, CodigoAcademia: academia}, map[string]any{"id": charge.Charge.ProviderChargeID, "responseStatus": map[string]any{"status": "Success", "code": float64(100)}})
+	accepted, _, err := service.AcceptWebhook(context.Background(), "REF", charge.Charge.ProviderChargeID, WebhookOwner{CredentialID: uuid.New(), ContextoTipo: ContextoAcademia, CodigoAcademia: academia}, map[string]any{"id": charge.Charge.ProviderChargeID, "responseStatus": map[string]any{"status": "Success", "code": float64(100)}})
 	if err != nil || !accepted {
 		t.Fatalf("webhook tardio = accepted %t, err %v", accepted, err)
 	}
@@ -225,6 +225,134 @@ func TestIntegrationMatriculaWebhookTardioMantemCancelamentoERegistraConflito(t
 	}
 }
 
+// methodAwareMockTransport permite controlar, de forma independente, o
+// resultado de um GET (a consulta ao vivo usada pelo double-check de
+// segurança de AcceptWebhook — ver liveChargeStatus) separadamente do POST
+// de criação da cobrança e do endpoint de token. getStatus define o status
+// devolvido pelo GET; se getErr não for nil, o GET falha com esse erro em
+// vez de responder (simula a AppyPay upstream indisponível).
+type methodAwareMockTransport struct {
+	getStatus string
+	getErr    error
+}
+
+func (t *methodAwareMockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
+	switch {
+	case strings.Contains(req.URL.Path, "/oauth2/token"):
+		return methodAwareJSON(req, `{"access_token":"test-token","expires_in":3600}`), nil
+	case req.Method == http.MethodGet:
+		if t.getErr != nil {
+			return nil, t.getErr
+		}
+		providerID := strings.TrimPrefix(req.URL.EscapedPath(), "/v2.0/charges/")
+		if providerID == req.URL.EscapedPath() || providerID == "" {
+			providerID = req.URL.Query().Get("merchantTransactionId")
+		}
+		return methodAwareJSON(req, `{"payment":{"id":"`+providerID+`","status":"`+t.getStatus+`","transactionEvents":[{"responseStatus":{"successful":true,"status":"`+t.getStatus+`","source":"REF"}}]}}`), nil
+	default:
+		return methodAwareJSON(req, `{"id":"provider-`+uuid.NewString()+`","responseStatus":{"successful":true,"status":"Pending","source":"REF"}}`), nil
+	}
+}
+
+func methodAwareJSON(req *http.Request, body string) *http.Response {
+	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}
+}
+
+// TestIntegrationAcceptWebhookConfirmaSucessoQuandoConsultaAoVivoConcorda
+// cobre o double-check de segurança recomendado pela documentação da
+// AppyPay ("Important: As a security measure, double check the transaction
+// by calling the GET /charges endpoint" — secção "Merchant Webhooks"):
+// quando o webhook alega sucesso e a consulta ao vivo concorda,
+// confirmedSuccess deve ser true.
+func TestIntegrationAcceptWebhookConfirmaSucessoQuandoConsultaAoVivoConcorda(t *testing.T) {
+	client := integrationClient(t)
+	t.Setenv("ENV", "test")
+	t.Setenv("APPYPAY_RESOURCE", "integration-resource")
+	t.Setenv("FINANCE_ENCRYPTION_KEY", "test-only-secret-material-at-least-32")
+	service := NewService(client)
+	service.SetHTTPClient(&http.Client{Transport: &methodAwareMockTransport{getStatus: "Success"}})
+	academia := "MAT" + uuid.NewString()[:8]
+	configureIntegrationCredential(t, service, ContextoAcademia, academia)
+	codigo := seedMatriculaPendente(t, client, academia, 900)
+	charge, err := service.IniciarPagamentoMatricula(context.Background(), MatriculaPagamentoInput{CodigoSolicitacao: codigo, MetodoPagamento: "REF"}, "127.0.0.1")
+	if err != nil {
+		t.Fatal(err)
+	}
+	owner := WebhookOwner{CredentialID: uuid.New(), ContextoTipo: ContextoAcademia, CodigoAcademia: academia}
+	accepted, confirmed, err := service.AcceptWebhook(context.Background(), "REF", charge.Charge.ProviderChargeID, owner, map[string]any{"id": charge.Charge.ProviderChargeID, "responseStatus": map[string]any{"status": "Success", "code": float64(100)}})
+	if err != nil || !accepted || !confirmed {
+		t.Fatalf("accepted=%t confirmed=%t err=%v, queria accepted=true confirmed=true", accepted, confirmed, err)
+	}
+	var status string
+	if err := client.DB().QueryRow(`SELECT payload->>'status' FROM financeiro_cobrancas WHERE id=$1`, charge.Charge.ID).Scan(&status); err != nil {
+		t.Fatal(err)
+	}
+	if status != "Success" {
+		t.Fatalf("status da cobrança = %q, queria Success", status)
+	}
+}
+
+// TestIntegrationAcceptWebhookNaoConfirmaSucessoQuandoConsultaAoVivoDiscorda
+// cobre o mesmo double-check no caso em que ele realmente pega algo: o
+// webhook alega sucesso mas a consulta ao vivo diz Pending. confirmedSuccess
+// deve ser false (nenhum efeito irreversível é disparado) e o estado
+// persistido deve refletir a consulta ao vivo — autoritativa — em vez da
+// alegação do webhook.
+func TestIntegrationAcceptWebhookNaoConfirmaSucessoQuandoConsultaAoVivoDiscorda(t *testing.T) {
+	client := integrationClient(t)
+	t.Setenv("ENV", "test")
+	t.Setenv("APPYPAY_RESOURCE", "integration-resource")
+	t.Setenv("FINANCE_ENCRYPTION_KEY", "test-only-secret-material-at-least-32")
+	service := NewService(client)
+	service.SetHTTPClient(&http.Client{Transport: &methodAwareMockTransport{getStatus: "Pending"}})
+	academia := "MAT" + uuid.NewString()[:8]
+	configureIntegrationCredential(t, service, ContextoAcademia, academia)
+	codigo := seedMatriculaPendente(t, client, academia, 900)
+	charge, err := service.IniciarPagamentoMatricula(context.Background(), MatriculaPagamentoInput{CodigoSolicitacao: codigo, MetodoPagamento: "REF"}, "127.0.0.1")
+	if err != nil {
+		t.Fatal(err)
+	}
+	owner := WebhookOwner{CredentialID: uuid.New(), ContextoTipo: ContextoAcademia, CodigoAcademia: academia}
+	accepted, confirmed, err := service.AcceptWebhook(context.Background(), "REF", charge.Charge.ProviderChargeID, owner, map[string]any{"id": charge.Charge.ProviderChargeID, "responseStatus": map[string]any{"status": "Success", "code": float64(100)}})
+	if err != nil || !accepted || confirmed {
+		t.Fatalf("accepted=%t confirmed=%t err=%v, queria accepted=true confirmed=false", accepted, confirmed, err)
+	}
+	var status string
+	if err := client.DB().QueryRow(`SELECT payload->>'status' FROM financeiro_cobrancas WHERE id=$1`, charge.Charge.ID).Scan(&status); err != nil {
+		t.Fatal(err)
+	}
+	if status != EstadoCobrancaAguardandoPagamento {
+		t.Fatalf("status da cobrança = %q, queria %q (autoritativo, da consulta ao vivo)", status, EstadoCobrancaAguardandoPagamento)
+	}
+}
+
+// TestIntegrationAcceptWebhookConfirmaSucessoQuandoConsultaAoVivoFalha cobre
+// o fallback do double-check: se a consulta ao vivo falhar (upstream
+// indisponível, timeout), AcceptWebhook cai para confiar no webhook em vez
+// de bloquear a confirmação — nunca deixa uma cobrança presa por causa de
+// uma falha temporária do próprio double-check (mesmo raciocínio do Bug 1
+// já corrigido, em que o webhook nunca efetivava a matrícula).
+func TestIntegrationAcceptWebhookConfirmaSucessoQuandoConsultaAoVivoFalha(t *testing.T) {
+	client := integrationClient(t)
+	t.Setenv("ENV", "test")
+	t.Setenv("APPYPAY_RESOURCE", "integration-resource")
+	t.Setenv("FINANCE_ENCRYPTION_KEY", "test-only-secret-material-at-least-32")
+	service := NewService(client)
+	service.SetHTTPClient(&http.Client{Transport: &methodAwareMockTransport{getErr: errors.New("upstream indisponível (simulado)")}})
+	academia := "MAT" + uuid.NewString()[:8]
+	configureIntegrationCredential(t, service, ContextoAcademia, academia)
+	codigo := seedMatriculaPendente(t, client, academia, 900)
+	charge, err := service.IniciarPagamentoMatricula(context.Background(), MatriculaPagamentoInput{CodigoSolicitacao: codigo, MetodoPagamento: "REF"}, "127.0.0.1")
+	if err != nil {
+		t.Fatal(err)
+	}
+	owner := WebhookOwner{CredentialID: uuid.New(), ContextoTipo: ContextoAcademia, CodigoAcademia: academia}
+	accepted, confirmed, err := service.AcceptWebhook(context.Background(), "REF", charge.Charge.ProviderChargeID, owner, map[string]any{"id": charge.Charge.ProviderChargeID, "responseStatus": map[string]any{"status": "Success", "code": float64(100)}})
+	if err != nil || !accepted || !confirmed {
+		t.Fatalf("accepted=%t confirmed=%t err=%v, queria accepted=true confirmed=true (fallback)", accepted, confirmed, err)
+	}
+}
+
 // TestIntegrationAcceptWebhookReflecteEstadoNaoSucesso cobre a
 // generalização de AcceptWebhook feita nesta tarefa: antes, só um webhook
 // de sucesso atualizava financeiro_cobrancas — um webhook avisando que uma
@@ -252,7 +380,7 @@ func TestIntegrationAcceptWebhookReflecteEstadoNaoSucesso(t *testing.T) {
 	}
 
 	owner := WebhookOwner{CredentialID: uuid.New(), ContextoTipo: ContextoAcademia, CodigoAcademia: academia}
-	accepted, err := service.AcceptWebhook(context.Background(), "REF", charge.Charge.ProviderChargeID, owner, map[string]any{"id": charge.Charge.ProviderChargeID, "responseStatus": map[string]any{"status": "Failed", "code": float64(245), "message": "The payment has expired", "source": "REF"}})
+	accepted, _, err := service.AcceptWebhook(context.Background(), "REF", charge.Charge.ProviderChargeID, owner, map[string]any{"id": charge.Charge.ProviderChargeID, "responseStatus": map[string]any{"status": "Failed", "code": float64(245), "message": "The payment has expired", "source": "REF"}})
 	if err != nil || !accepted {
 		t.Fatalf("webhook Expired = accepted %t, err %v", accepted, err)
 	}
@@ -280,7 +408,7 @@ func TestIntegrationAcceptWebhookReflecteEstadoNaoSucesso(t *testing.T) {
 	// eventID diferente (aqui o id interno da cobrança, que loadCharge
 	// também reconhece) passa pela deduplicação de
 	// financeiro_webhooks_recebidos para realmente exercer a guarda.
-	accepted2, err := service.AcceptWebhook(context.Background(), "REF", charge.Charge.ID.String(), owner, map[string]any{"id": charge.Charge.ProviderChargeID, "responseStatus": map[string]any{"status": "Failed"}})
+	accepted2, _, err := service.AcceptWebhook(context.Background(), "REF", charge.Charge.ID.String(), owner, map[string]any{"id": charge.Charge.ProviderChargeID, "responseStatus": map[string]any{"status": "Failed"}})
 	if err != nil || !accepted2 {
 		t.Fatalf("segundo webhook = accepted %t, err %v", accepted2, err)
 	}
```

### 3.5 `internal/handlers/financeiro_handlers_integration_test.go` — mock do `GET` com formato real + retorno de 3 valores

```diff
diff --git a/internal/handlers/financeiro_handlers_integration_test.go b/internal/handlers/financeiro_handlers_integration_test.go
index 8c53a0b..b802714 100644
--- a/internal/handlers/financeiro_handlers_integration_test.go
+++ b/internal/handlers/financeiro_handlers_integration_test.go
@@ -24,8 +24,21 @@ type handlerAppyPayMockTransport struct{}
 
 func (handlerAppyPayMockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
 	body := `{"id":"provider-charge-handler","status":"Pending"}`
-	if strings.Contains(req.URL.Path, "/oauth2/token") {
+	switch {
+	case strings.Contains(req.URL.Path, "/oauth2/token"):
 		body = `{"access_token":"test-token","expires_in":3600}`
+	case req.Method == http.MethodGet:
+		// Formato real de GET /charges/{id} (seção "Get a charge" da
+		// documentação AppyPay): o status vem dentro de "payment.status",
+		// nunca num campo solto "status" na raiz — diferente do corpo
+		// simplificado usado acima para a criação da cobrança (que este
+		// teste não usa para decidir nada). AcceptWebhook confirma um
+		// webhook de sucesso com exatamente este GET antes de aplicar um
+		// efeito irreversível (ver liveChargeStatus em
+		// internal/finance/appypay.go); devolver aqui o mesmo resultado que
+		// o webhook do teste relata (Success) reflete o cenário sendo
+		// testado — a AppyPay já confirmou o pagamento.
+		body = `{"payment":{"id":"provider-charge-handler","status":"Success","transactionEvents":[{"responseStatus":{"successful":true,"status":"Success","code":100,"source":"REF"}}]}}`
 	}
 	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
 }
```

### 3.6 `internal/finance/mensalidade_integration_test.go` — achado colateral: seed sem `sequencia` + `ModoVigencia` ausente em 6 call sites

```diff
diff --git a/internal/finance/mensalidade_integration_test.go b/internal/finance/mensalidade_integration_test.go
index 89e4d73..5117792 100644
--- a/internal/finance/mensalidade_integration_test.go
+++ b/internal/finance/mensalidade_integration_test.go
@@ -88,9 +88,16 @@ func seedMensalidadeConfiguracao(t *testing.T, client *db.Client, academia, nive
 	if curso != nil {
 		cursoID = *curso
 	}
+	// sequencia é NOT NULL sem default (migração 116, tarefa 78) — em
+	// produção vem sempre de spuri_ledger.id (a única fonte real de ordem
+	// cronológica do sistema); aqui, para não inserir uma linha de ledger
+	// só para popular este seed direto de teste, reserva-se um valor da
+	// mesma sequence real (spuri_ledger_id_seq), que continua
+	// monotonicamente crescente e nunca colide com sequencia já usada por
+	// eventos reais no mesmo banco de teste.
 	_, err := client.DB().Exec(`INSERT INTO financeiro_mensalidade_configuracoes
-		(event_id,aggregate_id,codigo_academia,nivel,ano_academico,curso_id,valor,mes_fim_cobranca,vigente_em)
-		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, uuid.New(), uuid.New(), academia, nivel, ano, cursoID, valor, fim, vigente)
+		(event_id,aggregate_id,codigo_academia,nivel,ano_academico,curso_id,valor,mes_fim_cobranca,vigente_em,sequencia)
+		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,nextval('spuri_ledger_id_seq'))`, uuid.New(), uuid.New(), academia, nivel, ano, cursoID, valor, fim, vigente)
 	if err != nil {
 		t.Fatal(err)
 	}
@@ -221,17 +228,17 @@ func TestIntegrationMensalidadeValidaGranularidade(t *testing.T) {
 	seedMensalidadeAcademia(t, client, publica, "public", "fundamental", "2026_2027")
 	seedMensalidadeAcademia(t, client, privada, "private", "medio", "2026_2027")
 	curso := seedMensalidadeCurso(t, client, privada)
-	if err := service.validateConfiguracaoMensalidade(context.Background(), &MensalidadeConfiguracaoInput{CodigoAcademia: publica, Nivel: NivelFundamental, AnoAcademico: "6_ano_fundamental", Valor: 10, MesFimCobranca: 6}); err == nil {
+	if err := service.validateConfiguracaoMensalidade(context.Background(), &MensalidadeConfiguracaoInput{CodigoAcademia: publica, Nivel: NivelFundamental, AnoAcademico: "6_ano_fundamental", Valor: 10, MesFimCobranca: 6, ModoVigencia: ModoVigenciaAPartirDaAtualizacao}); err == nil {
 		t.Fatal("academia publica aceitou configuracao")
 	}
-	if err := service.validateConfiguracaoMensalidade(context.Background(), &MensalidadeConfiguracaoInput{CodigoAcademia: privada, Nivel: NivelMedio, AnoAcademico: "1_ano_medio", Valor: 10, MesFimCobranca: 6}); err == nil {
+	if err := service.validateConfiguracaoMensalidade(context.Background(), &MensalidadeConfiguracaoInput{CodigoAcademia: privada, Nivel: NivelMedio, AnoAcademico: "1_ano_medio", Valor: 10, MesFimCobranca: 6, ModoVigencia: ModoVigenciaAPartirDaAtualizacao}); err == nil {
 		t.Fatal("curso obrigatorio no medio foi aceite sem curso_id")
 	}
 	cursoTexto := curso.String()
-	if err := service.validateConfiguracaoMensalidade(context.Background(), &MensalidadeConfiguracaoInput{CodigoAcademia: privada, Nivel: NivelMedio, AnoAcademico: "1_ano_medio", CursoID: &cursoTexto, Valor: 10, MesFimCobranca: 6}); err != nil {
+	if err := service.validateConfiguracaoMensalidade(context.Background(), &MensalidadeConfiguracaoInput{CodigoAcademia: privada, Nivel: NivelMedio, AnoAcademico: "1_ano_medio", CursoID: &cursoTexto, Valor: 10, MesFimCobranca: 6, ModoVigencia: ModoVigenciaAPartirDaAtualizacao}); err != nil {
 		t.Fatalf("curso medio oferecido foi rejeitado: %v", err)
 	}
-	if err := service.validateConfiguracaoMensalidade(context.Background(), &MensalidadeConfiguracaoInput{CodigoAcademia: privada, Nivel: NivelMedio, AnoAcademico: "9_ano_medio", CursoID: &cursoTexto, Valor: 10, MesFimCobranca: 6}); err == nil {
+	if err := service.validateConfiguracaoMensalidade(context.Background(), &MensalidadeConfiguracaoInput{CodigoAcademia: privada, Nivel: NivelMedio, AnoAcademico: "9_ano_medio", CursoID: &cursoTexto, Valor: 10, MesFimCobranca: 6, ModoVigencia: ModoVigenciaAPartirDaAtualizacao}); err == nil {
 		t.Fatal("ano nao oferecido foi aceite")
 	}
 }
@@ -242,12 +249,12 @@ func TestIntegrationMensalidadeValidaMesFim(t *testing.T) {
 	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2026_2027")
 	service := NewService(client)
 	for _, fim := range []int{6, 7} {
-		if err := service.validateConfiguracaoMensalidade(context.Background(), &MensalidadeConfiguracaoInput{CodigoAcademia: academia, Nivel: NivelFundamental, AnoAcademico: "6_ano_fundamental", Valor: 10, MesFimCobranca: fim}); err != nil {
+		if err := service.validateConfiguracaoMensalidade(context.Background(), &MensalidadeConfiguracaoInput{CodigoAcademia: academia, Nivel: NivelFundamental, AnoAcademico: "6_ano_fundamental", Valor: 10, MesFimCobranca: fim, ModoVigencia: ModoVigenciaAPartirDaAtualizacao}); err != nil {
 			t.Fatalf("mes_fim %d foi rejeitado: %v", fim, err)
 		}
 	}
 	for _, fim := range []int{5, 8} {
-		if err := service.validateConfiguracaoMensalidade(context.Background(), &MensalidadeConfiguracaoInput{CodigoAcademia: academia, Nivel: NivelFundamental, AnoAcademico: "6_ano_fundamental", Valor: 10, MesFimCobranca: fim}); err == nil {
+		if err := service.validateConfiguracaoMensalidade(context.Background(), &MensalidadeConfiguracaoInput{CodigoAcademia: academia, Nivel: NivelFundamental, AnoAcademico: "6_ano_fundamental", Valor: 10, MesFimCobranca: fim, ModoVigencia: ModoVigenciaAPartirDaAtualizacao}); err == nil {
 			t.Fatalf("mes_fim %d foi aceite", fim)
 		}
 	}
```

### 3.7 `internal/finance/financeiro_ledger_integrity_test.go` — achado colateral: `ModoVigencia` ausente em 3 call sites

```diff
diff --git a/internal/finance/financeiro_ledger_integrity_test.go b/internal/finance/financeiro_ledger_integrity_test.go
index aee8952..f079ecc 100644
--- a/internal/finance/financeiro_ledger_integrity_test.go
+++ b/internal/finance/financeiro_ledger_integrity_test.go
@@ -25,7 +25,7 @@ func TestIntegrationConfigureMensalidadeGravaNoLedgerEProjectaCorretamente(t *te
 
 	view, err := service.ConfigureMensalidade(ctx, MensalidadeConfiguracaoInput{
 		CodigoAcademia: academia, Nivel: NivelFundamental, AnoAcademico: "6_ano_fundamental",
-		Valor: 1000, MesFimCobranca: 7, MetodosPagamento: []string{"GPO", "REF"},
+		Valor: 1000, MesFimCobranca: 7, MetodosPagamento: []string{"GPO", "REF"}, ModoVigencia: ModoVigenciaAPartirDaAtualizacao,
 	}, "admin-teste", "academia", "127.0.0.1")
 	if err != nil {
 		t.Fatalf("ConfigureMensalidade retornou erro: %v", err)
@@ -68,7 +68,7 @@ func TestIntegrationConfigureMatriculaGravaNoLedgerEProjectaCorretamente(t *test
 
 	view, err := service.ConfigureMatricula(ctx, MatriculaConfiguracaoInput{
 		CodigoAcademia: academia, Nivel: NivelFundamental, AnoAcademico: "6_ano_fundamental",
-		Valor: 5000, MetodosPagamento: []string{"GPO"},
+		Valor: 5000, MetodosPagamento: []string{"GPO"}, ModoVigencia: ModoVigenciaAPartirDaAtualizacao,
 	}, "admin-teste", "academia", "127.0.0.1")
 	if err != nil {
 		t.Fatalf("ConfigureMatricula retornou erro: %v", err)
@@ -233,7 +233,7 @@ func TestIntegrationRebuildFinanceiroReconstroiConfiguracoesEcobrancasMensalidad
 
 	if _, err := service.ConfigureMatricula(ctx, MatriculaConfiguracaoInput{
 		CodigoAcademia: academia, Nivel: NivelFundamental, AnoAcademico: "6_ano_fundamental",
-		Valor: 5000, MetodosPagamento: []string{"GPO"},
+		Valor: 5000, MetodosPagamento: []string{"GPO"}, ModoVigencia: ModoVigenciaAPartirDaAtualizacao,
 	}, "admin-teste", "academia", "127.0.0.1"); err != nil {
 		t.Fatalf("ConfigureMatricula falhou: %v", err)
 	}
```

### 3.8 `internal/finance/mensalidade_remocao_integration_test.go` — achado colateral: `ModoVigencia` ausente em 2 call sites

```diff
diff --git a/internal/finance/mensalidade_remocao_integration_test.go b/internal/finance/mensalidade_remocao_integration_test.go
index fe8e9cb..5013a28 100644
--- a/internal/finance/mensalidade_remocao_integration_test.go
+++ b/internal/finance/mensalidade_remocao_integration_test.go
@@ -103,7 +103,7 @@ func TestIntegrationRemoveMensalidadeConfiguracaoFluxoDeComando(t *testing.T) {
 	}
 	if _, err := service.ConfigureMensalidade(context.Background(), MensalidadeConfiguracaoInput{
 		CodigoAcademia: academia, Nivel: NivelFundamental, AnoAcademico: "7_ano_fundamental",
-		Valor: 5000, MesFimCobranca: 7, MetodosPagamento: []string{"GPO_QR"},
+		Valor: 5000, MesFimCobranca: 7, MetodosPagamento: []string{"GPO_QR"}, ModoVigencia: ModoVigenciaAPartirDaAtualizacao,
 	}, uuid.NewString(), "academia", "127.0.0.1"); err != nil {
 		t.Fatalf("ConfigureMensalidade falhou: %v", err)
 	}
@@ -153,7 +153,7 @@ func TestIntegrationRemoveMensalidadeConfiguracaoFluxoDeComando(t *testing.T) {
 
 	if _, err := service.ConfigureMensalidade(context.Background(), MensalidadeConfiguracaoInput{
 		CodigoAcademia: academia, Nivel: NivelFundamental, AnoAcademico: "7_ano_fundamental",
-		Valor: 6000, MesFimCobranca: 7, MetodosPagamento: []string{"GPO_QR"},
+		Valor: 6000, MesFimCobranca: 7, MetodosPagamento: []string{"GPO_QR"}, ModoVigencia: ModoVigenciaAPartirDaAtualizacao,
 	}, uuid.NewString(), "academia", "127.0.0.1"); err != nil {
 		t.Fatalf("reconfiguração após remoção não deveria falhar: %v", err)
 	}
```

### 3.9 `internal/finance/matricula_remocao_integration_test.go` — achado colateral: `ModoVigencia` ausente em 2 call sites

```diff
diff --git a/internal/finance/matricula_remocao_integration_test.go b/internal/finance/matricula_remocao_integration_test.go
index c7b8d6d..9f62c23 100644
--- a/internal/finance/matricula_remocao_integration_test.go
+++ b/internal/finance/matricula_remocao_integration_test.go
@@ -23,7 +23,7 @@ func TestIntegrationRemoveMatriculaConfiguracaoFluxoDeComando(t *testing.T) {
 	}
 	if _, err := service.ConfigureMatricula(context.Background(), MatriculaConfiguracaoInput{
 		CodigoAcademia: academia, Nivel: NivelFundamental, AnoAcademico: "7_ano_fundamental",
-		Valor: 15000, MetodosPagamento: []string{"GPO_QR"},
+		Valor: 15000, MetodosPagamento: []string{"GPO_QR"}, ModoVigencia: ModoVigenciaAPartirDaAtualizacao,
 	}, uuid.NewString(), "academia", "127.0.0.1"); err != nil {
 		t.Fatalf("ConfigureMatricula falhou: %v", err)
 	}
@@ -77,7 +77,7 @@ func TestIntegrationRemoveMatriculaConfiguracaoFluxoDeComando(t *testing.T) {
 	// Reconfigurar depois de remover funciona.
 	if _, err := service.ConfigureMatricula(context.Background(), MatriculaConfiguracaoInput{
 		CodigoAcademia: academia, Nivel: NivelFundamental, AnoAcademico: "7_ano_fundamental",
-		Valor: 20000, MetodosPagamento: []string{"GPO_QR"},
+		Valor: 20000, MetodosPagamento: []string{"GPO_QR"}, ModoVigencia: ModoVigenciaAPartirDaAtualizacao,
 	}, uuid.NewString(), "academia", "127.0.0.1"); err != nil {
 		t.Fatalf("reconfiguração após remoção não deveria falhar: %v", err)
 	}
```

### 3.10 `internal/handlers/financeiro_pendencias_handlers_test.go` — achado colateral: segundo seed sem `sequencia`

```diff
diff --git a/internal/handlers/financeiro_pendencias_handlers_test.go b/internal/handlers/financeiro_pendencias_handlers_test.go
index b449adb..0ae8c8c 100644
--- a/internal/handlers/financeiro_pendencias_handlers_test.go
+++ b/internal/handlers/financeiro_pendencias_handlers_test.go
@@ -46,9 +46,12 @@ func seedAcademiaEscolarPrivadaComTurma(t *testing.T, client *db.Client, academi
 
 func seedMensalidadeConfigParaHTTP(t *testing.T, client *db.Client, academia, anoAcademico string, valor float64) {
 	t.Helper()
+	// Ver o comentário equivalente em seedMensalidadeConfiguracao
+	// (internal/finance/mensalidade_integration_test.go) sobre por que
+	// sequencia usa nextval('spuri_ledger_id_seq') num seed direto de teste.
 	if _, err := client.DB().Exec(`INSERT INTO financeiro_mensalidade_configuracoes
-		(event_id,aggregate_id,codigo_academia,nivel,ano_academico,curso_id,valor,mes_fim_cobranca,vigente_em)
-		VALUES ($1,$2,$3,'fundamental',$4,NULL,$5,7,'2026-01-01')`,
+		(event_id,aggregate_id,codigo_academia,nivel,ano_academico,curso_id,valor,mes_fim_cobranca,vigente_em,sequencia)
+		VALUES ($1,$2,$3,'fundamental',$4,NULL,$5,7,'2026-01-01',nextval('spuri_ledger_id_seq'))`,
 		uuid.New(), uuid.New(), academia, anoAcademico, valor); err != nil {
 		t.Fatal(err)
 	}
```

### 3.11 `internal/handlers/financeiro_remocao_handlers_integration_test.go` — achado colateral: `ModoVigencia` ausente em 2 call sites

```diff
diff --git a/internal/handlers/financeiro_remocao_handlers_integration_test.go b/internal/handlers/financeiro_remocao_handlers_integration_test.go
index 7892bf1..d22ad2b 100644
--- a/internal/handlers/financeiro_remocao_handlers_integration_test.go
+++ b/internal/handlers/financeiro_remocao_handlers_integration_test.go
@@ -118,7 +118,7 @@ func TestIntegrationHandlersRemocaoFinanceiraRespeitamEscopoDaAcademia(t *testin
 	}
 	if _, err := FinanceiroService.ConfigureMensalidade(ctx.Request.Context(), finance.MensalidadeConfiguracaoInput{
 		CodigoAcademia: academiaDona, Nivel: finance.NivelFundamental, AnoAcademico: "7_ano_fundamental",
-		Valor: 5000, MesFimCobranca: 7, MetodosPagamento: []string{"GPO_QR"},
+		Valor: 5000, MesFimCobranca: 7, MetodosPagamento: []string{"GPO_QR"}, ModoVigencia: finance.ModoVigenciaAPartirDaAtualizacao,
 	}, uuid.NewString(), "academia", "127.0.0.1"); err != nil {
 		t.Fatalf("ConfigureMensalidade falhou: %v", err)
 	}
@@ -149,7 +149,7 @@ func TestIntegrationHandlersRemocaoFinanceiraRespeitamEscopoDaAcademia(t *testin
 	// 4) Configuração de matrícula.
 	if _, err := FinanceiroService.ConfigureMatricula(ctx.Request.Context(), finance.MatriculaConfiguracaoInput{
 		CodigoAcademia: academiaDona, Nivel: finance.NivelFundamental, AnoAcademico: "7_ano_fundamental",
-		Valor: 15000, MetodosPagamento: []string{"GPO_QR"},
+		Valor: 15000, MetodosPagamento: []string{"GPO_QR"}, ModoVigencia: finance.ModoVigenciaAPartirDaAtualizacao,
 	}, uuid.NewString(), "academia", "127.0.0.1"); err != nil {
 		t.Fatalf("ConfigureMatricula falhou: %v", err)
 	}
```

---

## 4. Resumo do que cada diff faz (para revisão, não para re-decidir nada)

1. **3.1** — `token()` passa a aceitar `expires_in` como string OU número (tipo novo `flexibleSeconds`, com `UnmarshalJSON` próprio); `AcceptWebhook` ganha um double-check via `GET /charges/{id}` (`liveChargeStatus`, nova função) antes de tratar um webhook de sucesso "normal" como definitivo, e passa a devolver um terceiro valor `confirmedSuccess`.
2. **3.2** — o handler do webhook usa `confirmedSuccess` (devolvido por `AcceptWebhook`) para decidir se efetiva a matrícula, em vez de reler o payload cru do webhook via `finance.IsSuccessfulProviderPayload`.
3. **3.3** — teste unitário novo (`TestTokenAceitaExpiresInComoStringOuNumero`), sem necessidade de Postgres, cobrindo os dois formatos de `expires_in`.
4. **3.4** — dois call sites existentes de `AcceptWebhook` atualizados para o novo retorno de 3 valores (comportamento inalterado, só a assinatura); três testes de integração novos provando o double-check nos três cenários (consulta concorda, consulta discorda, consulta falha).
5. **3.5** — o mock HTTP usado pelo teste `TestIntegrationReceberWebhookAppyPayEfetivaVinculoMatricula` passa a responder a um `GET` com o formato real documentado (`payment.status`, não um campo `status` solto na raiz — que só existia porque nada no código antes desta tarefa jamais fazia um `GET` neste teste) e o handler é atualizado para o novo retorno de 3 valores de `AcceptWebhook`.
6. **3.6 a 3.11** — achado colateral da tarefa 78 (seção 2): dois helpers de seed passam a popular `sequencia` via `nextval('spuri_ledger_id_seq')`; 15 call sites de `ConfigureMensalidade`/`ConfigureMatricula` em testes passam a informar `ModoVigencia: ModoVigenciaAPartirDaAtualizacao` (preserva o comportamento que cada um desses testes sempre validou). Nenhuma asserção de nenhum destes testes muda — só o que é necessário para eles voltarem a compilar/rodar contra o schema atual.

---

## 5. Validação já executada por Claude (com PostgreSQL 16 real e Go 1.24 real)

- Go 1.24.4 (via `apt-get install golang-1.24-go`, com `GOTOOLCHAIN=local` e `GOPROXY=direct` para contornar o proxy do Go bloqueado no sandbox — só afeta o ambiente de validação, não o código), PostgreSQL 16, banco `spuri_test` recriado do zero (`DROP DATABASE` + `CREATE DATABASE`) antes de cada rodada de suíte completa.
- **Baseline, direto do `main` atual (já com a tarefa 78 mergeada), antes de qualquer alteração desta tarefa**: `go build ./...` e `go vet ./...` limpos; `go test ./...` → **37 falhas**, todas rastreadas até o achado colateral da seção 2 (nenhuma relacionada a AppyPay).
- **Prova de que o Bug 1 é real**: reproduzido dentro do próprio pacote `finance`, com um `http.RoundTripper` simulando a resposta real do token (`expires_in` como string) contra o código sem a correção — `token()` falhou com `resposta de token AppyPay inválida`, exatamente como previsto. Reaplicada a correção, o mesmo teste passou.
- **Prova de que o Bug 2 é real e de que a primeira tentativa de correção tinha uma regressão real**: a primeira versão da correção (reaproveitando `consultCharge` para todo o caminho de sucesso) foi implementada, testada contra a suíte completa, e **quebrou** `TestIntegrationMatriculaWebhookTardioMantemCancelamentoERegistraConflito` — uma cobrança já cancelada localmente voltava a `aguardando_pagamento` quando a consulta ao vivo (no mock desse teste) devolvia `Pending`. Revertida e corrigida para a versão final (só ativa o double-check no caminho normal de sucesso, nunca no caso já-cancelado) — a suíte completa voltou a passar, incluindo esse teste específico.
- **Depois de aplicar os 11 diffs (achado colateral incluído)**: `go build ./...` limpo, `go vet ./...` limpo, `gofmt -l .` sem nenhuma linha, `go test -count=1 ./...` (repositório inteiro, sem cache) → **387 testes passando, 0 falhas**, banco recriado do zero — repetido duas vezes seguidas para confirmar determinismo (mesmo resultado nas duas rodadas).
- `go.mod`/`go.sum` ficam **byte-idênticos** ao estado atual do `main` — nenhuma dependência foi adicionada, removida ou trocada de versão (os `replace` temporários usados só para contornar o proxy bloqueado no sandbox de validação foram revertidos antes de gerar os diffs acima).

### 5.1 Checklist para você (Codex) rodar depois de aplicar os 11 diffs

1. `gofmt -l internal/finance/appypay.go internal/finance/appypay_test.go internal/finance/appypay_integration_test.go internal/handlers/financeiro_handlers.go internal/handlers/financeiro_handlers_integration_test.go internal/finance/mensalidade_integration_test.go internal/finance/financeiro_ledger_integrity_test.go internal/finance/mensalidade_remocao_integration_test.go internal/finance/matricula_remocao_integration_test.go internal/handlers/financeiro_pendencias_handlers_test.go internal/handlers/financeiro_remocao_handlers_integration_test.go` — deve devolver **vazio**. Se devolver algo, rode `gofmt -w` nesses arquivos e confira o diff resultante contra a seção 3 antes de prosseguir.
2. `go build ./...` — deve compilar sem erro.
3. `go vet ./...` — deve rodar sem apontar nada.
4. Sem PostgreSQL disponível no seu ambiente, `go test ./...` vai pular (`SKIP`) todos os testes de integração (os que chamam `integrationClient`/`integrationFinanceClient`, que exigem `RUN_POSTGRES_INTEGRATION=1`) — isso é esperado e não é falha. O que você **consegue** validar sem banco: `go test ./internal/finance/... -run TestTokenAceitaExpiresInComoStringOuNumero -v` (teste novo do diff 3.3, unitário, sem banco) deve passar com as 2 subasserções em verde.
5. Se, por qualquer motivo, você tiver acesso a um PostgreSQL neste ambiente (não esperado, mas caso mude): repita exatamente a validação da seção 5 (`go test -count=1 ./...` com o banco recriado do zero antes) e confirme 387 testes passando, 0 falhas.

---

## 6. Fora de escopo — não toque em nenhum destes

- Qualquer arquivo além dos 11 listados na seção 3.
- Validação local de tamanho do `phoneNumber` do GPO, valor mínimo de 1 AOA, ou o caminho hipotético de REF com só `dueDate` em `paymentInfo` — os mesmos três itens já observados e deliberadamente deixados de fora na tarefa 77 (detalhes em `docs/Debbugs/Auditoria de conformidade AppyPay (autenticação e geração de cobrança).md`, secção "Fora de escopo"); revisados de novo nesta rodada, continuam sem ser uma divergência real em relação à documentação.
- O endpoint de token continuar usando `POST` (não mudar para `GET`) — já resolvido e confirmado como correto na tarefa 77.
- Qualquer redesenho do mecanismo de `modo_vigencia`/`sequencia` da tarefa 78 — os diffs 3.6 a 3.11 desta tarefa só reparam seeds de teste para o schema que a tarefa 78 já define; não mudam nenhuma regra de negócio dela.
- Adicionar um fluxo automatizado de estorno/reversão (`Post a charge refund`/`Post a charge reverse`) — confirmado como fora de escopo desta fase pela própria documentação interna ("Escopo do Módulo Financeiro Base").
- `go.mod`/`go.sum` — não devem mudar.

---

## 7. Critérios de aceite

- [ ] Os 11 diffs da seção 3 aplicados, byte a byte, sem nenhuma alteração fora deles.
- [ ] `gofmt -l` vazio para os 11 arquivos.
- [ ] `go build ./...` e `go vet ./...` limpos.
- [ ] `go test ./internal/finance/... -run TestTokenAceitaExpiresInComoStringOuNumero -v` passando (2/2 subtestes).
- [ ] Se houver PostgreSQL disponível: `go test -count=1 ./...` com banco recriado do zero → 387 testes, 0 falhas.
- [ ] `go.mod`/`go.sum` sem nenhuma alteração.

---

## 8. Procedimento de conclusão

1. Rode o checklist da seção 5.1 e reporte o resultado de cada item para Fredy, exatamente como saiu (sem resumir/reformular).
2. Se tudo estiver verde, mova este arquivo de `docs/Lista de Tarefas/` para `docs/Tarefas feitas/`, atualizando o frontmatter: `status: concluido` e adicione `concluido: <data de hoje>`.
3. Não altere o frontmatter de `docs/Debbugs/Auditoria de conformidade AppyPay (autenticação e geração de cobrança).md` — isso é feito por Claude na próxima revisão.
4. Se qualquer item do checklist não bater exatamente com o esperado (ex.: `go build` aponta um erro não previsto nesta tarefa), **pare e reporte o erro exato** para Fredy antes de tentar corrigir por conta própria — a menos que seja um erro mecânico trivial e óbvio (ex.: um `gofmt` não aplicado), caso em que pode corrigir e seguir, relatando o que foi feito.
