---
criado: 2026-08-27
origem: conversa com Fredy (Claude como orquestrador — depuração e implementação com PostgreSQL 16 real em sandbox, Codex como executor)
status: feito
tipo: correcao_de_bug
concluido: 2026-08-27
depende_de: nenhuma
nota_de_numeracao: esta tarefa foi originalmente escrita como "68", mas esse número foi tomado enquanto o documento era escrito por "68 - Corrigir equivalencia do filtro estadoFailed para cobrancas com falha local falhada" (commit 598e142, PR #578) — sem relação com o conteúdo desta tarefa. Renumerada para 70. Os diffs abaixo já foram gerados e revalidados (build + vet + gofmt + suíte inteira com PostgreSQL real) contra o estado atual do repositório, já incluindo o commit 598e142.
---

# Corrigir a extração de status e motivo real da AppyPay (Cancelled do GPO, Expired do REF) e categorização preditiva de motivos (feito)

## 0. Leia isto primeiro — sobre esta tarefa e sobre o seu ambiente (Codex)

Claude já implementou, testou e validou esta correção inteira com PostgreSQL 16 real, **incluindo depois de rebasear em cima do commit 598e142 (`Corrigir equivalencia do filtro estado=Failed...`)**, que foi mesclado ao `main` enquanto esta tarefa estava sendo escrita: `go build ./...`, `go vet ./...`, `gofmt -l .` (limpo) e `go test ./...` — **suíte inteira do repositório**, com banco de dados limpo a cada execução — todos passando, sem nenhuma asserção pré-existente alterada.

O rebase foi automático e sem conflitos textuais: esta tarefa e a 598e142 tocam funções diferentes de `internal/finance/appypay.go` (598e142 mexeu no caminho de erro de `CreateCharge`/`CreateGPOQRCode` e em `estadosCobrancaEquivalentes`/`normalizeChargeStatus`; esta tarefa mexe na leitura do status de sucesso/consulta/webhook). Os diffs abaixo já refletem o arquivo **depois** de 598e142 — aplique-os sobre o `main` atual, não sobre uma cópia antiga.

O seu ambiente (Codex) não tem `psql`/Docker/Postgres e bloqueia `apt` — por isso, a validação com banco de dados real já foi feita por Claude e está descrita na seção 5. A sua tarefa é aplicar os diffs da seção 3, na ordem em que aparecem, e depois rodar o que conseguir do checklist da seção 5.1 — não precisa reinstalar nem reconfigurar nada.

## 1. Prompt recomendado para executar esta correção

> Execute exatamente as alterações descritas neste documento, nesta ordem, sobre o `main` atual do repositório (que já inclui o commit 598e142). Todas as decisões de desenho já foram tomadas e validadas por Claude (implementação testada com `go build`, `go vet`, `gofmt` e a suíte inteira `go test ./...` contra PostgreSQL 16 real, depois de rebasear em cima de 598e142 sem conflitos). Sua tarefa é mecânica: (1) aplicar os diffs da seção 3, na ordem em que aparecem, usando `git apply` quando possível ou replicando manualmente o antes/depois quando o diff não aplicar por causa de drift de linha; (2) rodar cada item do checklist da seção 5.1 e reportar o resultado; (3) seguir o "Procedimento de conclusão" (seção 7). Não toque em nenhum arquivo fora do escopo listado na seção 6 ("Fora de escopo").

---

## 2. Contexto — o bug e por que ele nunca foi percebido

A pergunta que originou esta investigação: "quando GPO fica `Cancelled` e REF fica `Expired`, existe mecanismo real que suporte todos os motivos documentados pela AppyPay?" A resposta encontrada: **não existia nenhum mecanismo funcional para isso**, apesar do código já ter a intenção certa (`isTerminalChargeStatus`/`normalizeChargeStatus` já reconheciam as palavras `Cancelled`/`Expired`/`Failed`/`Success`) — o problema é que **nada nunca alimentava essas funções com o dado real**. Isto é um bug diferente e anterior ao corrigido em 598e142: aquele tratava do caminho em que a própria chamada HTTP à AppyPay falha (nunca chega a existir uma cobrança do lado do provedor); este trata do caminho em que a AppyPay **responde normalmente**, mas o Spuri lê o campo errado da resposta.

### 2.1 A causa raiz

`internal/finance/appypay.go` tinha uma função:

```go
func responseStatus(v map[string]any) string {
	for _, k := range []string{"status", "state"} {
		if x, ok := v[k].(string); ok {
			return x
		}
	}
	return ""
}
```

Ela procura `v["status"]` ou `v["state"]` **no nível raiz** do JSON. Só que a AppyPay **nunca** manda o status de uma cobrança no nível raiz — confirmado lendo a documentação oficial da AppyPay linha a linha (exemplos de resposta reais, não só a tabela de schema):

- **`POST /charges`, `POST /qr-codes` e o webhook transacional**: o status vem aninhado em `responseStatus.status` (junto com `responseStatus.code`, `.message`, `.source`).
- **`GET /charges/{id}`** (e por `merchantTransactionId`): tudo vem dentro de um envelope `payment`, ou seja `payment.status`, com o motivo mais fino ainda mais dentro, em `payment.transactionEvents[i].responseStatus.{code,message,source}`.

Como `responseStatus(v)` só olhava o nível errado, ela **sempre devolvia `""`** contra tráfego real da AppyPay (respostas 2xx normais — o caso corrigido por 598e142 é diferente: lá a chamada HTTP falha antes de existir qualquer resposta para ler). Consequência prática:

- `CreateCharge`/`CreateGPOQRCode`: no caminho de sucesso (resposta 2xx da AppyPay), sempre caem no fallback `aguardando_pagamento`, mesmo que a AppyPay já tivesse devolvido `Failed`/`Cancelled` na hora da criação.
- `consultCharge` (usado por `GET /financeiro/appypay/cobrancas/:id` e pelo pré-check obrigatório de `CancelCharge`): como `status == ""`, sempre mantém o status anterior — **uma cobrança nunca sai de `aguardando_pagamento` sozinha**, não importa quantas vezes seja consultada.
- `AcceptWebhook`: o bloco que reflete o status do webhook no read model (`if raw := responseStatus(payload); raw != ""`) **nunca executa** para webhooks reais — só o evento bruto é logado, a cobrança em si nunca avança.

Ou seja: nenhum dos ~40+ motivos documentados de GPO virar `Cancelled`, nem o motivo de REF virar `Expired` (código 245), jamais chegava a aparecer no sistema — a cobrança ficava presa em `aguardando_pagamento` para sempre.

### 2.2 Por que ninguém percebeu

Os testes existentes (`appyPayMockTransport` em `appypay_integration_test.go`, reusado por `mensalidade_integration_test.go`, `qrcode_regression_integration_test.go` e `financeiro_ledger_integrity_test.go`) simulavam a resposta da AppyPay com a forma **plana e incorreta** (`{"id": "...", "status": "Success"}`) — exatamente a forma que `responseStatus()` sabia ler, mas que a AppyPay real nunca usa para cobranças. Os testes passavam porque testavam contra uma premissa errada sobre o próprio contrato da AppyPay, não contra o comportamento real.

O próprio documento de análise da integração (`docs/Parceiros e integrações/AppyPay - Análise de Integração para o Serviço de Gestão Financeira do Spuri.md`) já mostrava o envelope aninhado correto (`responseStatus: {successful, status, code, message, source, sourceDetails}`) e recomendava um "módulo de tradução de erros AppyPay" — que nunca chegou a ser implementado.

### 2.3 Uma ambiguidade real na documentação da AppyPay, e como ela foi resolvida

O schema genérico de `responseStatus.status` documenta só 4 valores possíveis (`Requested`, `Pending`, `Success`, `Failed`) — **sem** `Cancelled` nem `Expired`. Só que as tabelas de código (`HTTP 2xx/4xx/5xx Responses`) descrevem, para cada código numérico, uma tag entre colchetes na descrição (`[Cancelled]`, `[Expired]`, `[Failed]`...) — e a própria AppyPay instrui explicitamente, na seção "Error Handling Actions": *"For HTTP 2XX codes responses (...) update payment status to the one stated in the description"*.

Ou seja: não há confirmação se o literal que chega em `responseStatus.status` realmente é `"Cancelled"`/`"Expired"` para um GPO recusado/REF expirado, ou se é sempre `"Failed"` genérico (um dos 4 valores "oficiais") com o motivo verdadeiro só disponível no `code` numérico.

**Solução adotada (robusta para os dois cenários)**: o código numérico (`responseStatus.code`) é tratado como fonte de verdade **sempre que reconhecido**, sobrepondo o literal de `status` quando os dois divergem — exatamente o que a própria AppyPay instrui. Isto funciona corretamente nos dois cenários possíveis (se o literal já vier certo, o código concorda e nada muda; se o literal vier sempre `"Failed"`, o código é que resgata a informação real).

---

## 3. Diffs a aplicar, na ordem exata

### 3.1 `internal/finance/appypay.go`

Aplicar este diff (via `git apply`, colando o conteúdo abaixo em um arquivo `.patch` e rodando `git apply nome.patch` a partir da raiz do repositório; se o `git apply` falhar por drift de linha, replicar manualmente cada trecho `-`/`+`):

```diff
diff --git a/internal/finance/appypay.go b/internal/finance/appypay.go
index 2777985..49c3427 100644
--- a/internal/finance/appypay.go
+++ b/internal/finance/appypay.go
@@ -27,6 +27,7 @@ import (
 	"net/http"
 	"net/url"
 	"os"
+	"strconv"
 	"strings"
 	"sync"
 	"time"
@@ -160,11 +161,20 @@ type QRCodeRequest struct {
 	CodigoSolicitacao string                  `json:"codigo_solicitacao,omitempty"`
 }
 type ChargeResult struct {
-	ID                    uuid.UUID      `json:"id"`
-	ProviderChargeID      string         `json:"provider_charge_id,omitempty"`
-	MerchantTransactionID string         `json:"merchant_transaction_id"`
-	Status                string         `json:"status"`
-	Response              map[string]any `json:"response,omitempty"`
+	ID                    uuid.UUID `json:"id"`
+	ProviderChargeID      string    `json:"provider_charge_id,omitempty"`
+	MerchantTransactionID string    `json:"merchant_transaction_id"`
+	Status                string    `json:"status"`
+	// CodigoProvedor/MensagemProvedor/FonteProvedor/CategoriaMotivo expõem o
+	// motivo real devolvido pela AppyPay (responseStatus.code/message/source
+	// de POST/webhook, ou o do último transactionEvent de GET /charges) toda
+	// vez que a cobrança não está simplesmente aguardando pagamento — ver
+	// extractProviderOutcome/applyProviderOutcome nesta mesma package.
+	CodigoProvedor   *int           `json:"codigo_provedor,omitempty"`
+	MensagemProvedor string         `json:"mensagem_provedor,omitempty"`
+	FonteProvedor    string         `json:"fonte_provedor,omitempty"`
+	CategoriaMotivo  string         `json:"categoria_motivo,omitempty"`
+	Response         map[string]any `json:"response,omitempty"`
 }
 type QRCodeResult struct {
 	ChargeResult
@@ -189,11 +199,19 @@ type CobrancaResumo struct {
 	// codigo_solicitacao), "avulsa" nos demais casos (cobrança criada
 	// diretamente via POST /financeiro/appypay/cobrancas ou /appypay/qr-codes
 	// sem vínculo a mensalidade nem matrícula).
-	Origem    string  `json:"origem"`
-	Status    string  `json:"status"`
-	Valor     float64 `json:"valor"`
-	Moeda     string  `json:"moeda,omitempty"`
-	Descricao string  `json:"descricao,omitempty"`
+	Origem string `json:"origem"`
+	Status string `json:"status"`
+	// CodigoProvedor/MensagemProvedor/FonteProvedor/CategoriaMotivo: ver o
+	// comentário equivalente em ChargeResult, acima de CreateCharge. Mesma
+	// origem de dado (payload["codigo_provedor"] etc.), lida aqui para a
+	// listagem em vez de para uma única cobrança.
+	CodigoProvedor   *int    `json:"codigo_provedor,omitempty"`
+	MensagemProvedor string  `json:"mensagem_provedor,omitempty"`
+	FonteProvedor    string  `json:"fonte_provedor,omitempty"`
+	CategoriaMotivo  string  `json:"categoria_motivo,omitempty"`
+	Valor            float64 `json:"valor"`
+	Moeda            string  `json:"moeda,omitempty"`
+	Descricao        string  `json:"descricao,omitempty"`
 	// MetodoPagamento reflete "GPO_QR" (não apenas "GPO") quando a cobrança
 	// tem qr_code_type no payload — CreateGPOQRCode grava payment_method
 	// como "GPO" internamente, então sem este ajuste a origem QR ficaria
@@ -479,17 +497,26 @@ func (s *Service) CreateCharge(ctx context.Context, in ChargeRequest, actorID, a
 		return ChargeResult{ID: id, MerchantTransactionID: in.MerchantTransactionID, Status: "Failed"}, err
 	}
 	providerID := responseID(response)
-	status := normalizeChargeStatus(responseStatus(response))
+	outcome := extractProviderOutcome(response)
+	status := normalizeChargeStatus(outcome.Status)
 	if status == "" {
-		// A AppyPay respondeu 2xx sem nenhum campo de status no corpo — a
-		// cobrança foi aceita mas ainda não temos nenhuma informação sobre
-		// sua resolução, exatamente o significado de aguardando pagamento.
+		// A AppyPay respondeu 2xx sem nenhum campo de status reconhecível no
+		// corpo — a cobrança foi aceita mas ainda não temos nenhuma
+		// informação sobre sua resolução, exatamente o significado de
+		// aguardando pagamento.
 		status = EstadoCobrancaAguardandoPagamento
 	}
-	if err = s.record(ctx, id, "CobrancaAppyPayCriada", chargePayload(id, in, providerID, status, response), actorID, actorType, ip); err != nil {
+	created := chargePayload(id, in, providerID, status, response)
+	applyProviderOutcome(created, outcome)
+	if err = s.record(ctx, id, "CobrancaAppyPayCriada", created, actorID, actorType, ip); err != nil {
 		return ChargeResult{}, err
 	}
 	result := ChargeResult{ID: id, ProviderChargeID: providerID, MerchantTransactionID: in.MerchantTransactionID, Status: status, Response: response}
+	if outcome.HasCode {
+		code := outcome.Code
+		result.CodigoProvedor = &code
+	}
+	result.MensagemProvedor, result.FonteProvedor, result.CategoriaMotivo = outcome.Message, outcome.Source, outcome.Categoria
 	if isSuccessfulChargeStatus(status) {
 		_ = s.confirmMensalidadeCharge(ctx, id, actorID, actorType, ip)
 	}
@@ -556,18 +583,26 @@ func (s *Service) CreateGPOQRCode(ctx context.Context, in QRCodeRequest, actorID
 		return QRCodeResult{}, err
 	}
 	providerID := responseID(response)
-	status := normalizeChargeStatus(responseStatus(response))
+	outcome := extractProviderOutcome(response)
+	status := normalizeChargeStatus(outcome.Status)
 	if status == "" {
-		// Mesmo raciocínio de CreateCharge: 2xx sem status = aceito, ainda
-		// sem resolução conhecida.
+		// Mesmo raciocínio de CreateCharge: 2xx sem status reconhecível =
+		// aceito, ainda sem resolução conhecida.
 		status = EstadoCobrancaAguardandoPagamento
 	}
 	payload := qrCodePayload(id, in, typ, providerID, status, response)
+	applyProviderOutcome(payload, outcome)
 	if err = s.record(ctx, id, "QRCodeAppyPayGerado", payload, actorID, actorType, ip); err != nil {
 		return QRCodeResult{}, err
 	}
 	qr, _ := response["qrCodeArr"].(string)
-	result := QRCodeResult{ChargeResult: ChargeResult{ID: id, ProviderChargeID: providerID, MerchantTransactionID: in.MerchantTransactionID, Status: status, Response: response}, QRCodeArr: qr}
+	chargeResult := ChargeResult{ID: id, ProviderChargeID: providerID, MerchantTransactionID: in.MerchantTransactionID, Status: status, Response: response}
+	if outcome.HasCode {
+		code := outcome.Code
+		chargeResult.CodigoProvedor = &code
+	}
+	chargeResult.MensagemProvedor, chargeResult.FonteProvedor, chargeResult.CategoriaMotivo = outcome.Message, outcome.Source, outcome.Categoria
+	result := QRCodeResult{ChargeResult: chargeResult, QRCodeArr: qr}
 	if isSuccessfulChargeStatus(status) {
 		_ = s.confirmMensalidadeCharge(ctx, id, actorID, actorType, ip)
 	}
@@ -808,6 +843,13 @@ func scanCobrancaResumo(rows *sql.Rows) (CobrancaResumo, error) {
 	}
 	rawStatus, _ := payload["status"].(string)
 	dto.Status = normalizeChargeStatus(rawStatus)
+	if codigo, ok := payload["codigo_provedor"].(float64); ok {
+		c := int(codigo)
+		dto.CodigoProvedor = &c
+	}
+	dto.MensagemProvedor, _ = payload["mensagem_provedor"].(string)
+	dto.FonteProvedor, _ = payload["fonte_provedor"].(string)
+	dto.CategoriaMotivo, _ = payload["categoria_motivo"].(string)
 	dto.Valor, _ = payload["amount"].(float64)
 	dto.Moeda, _ = payload["currency"].(string)
 	dto.Descricao, _ = payload["description"].(string)
@@ -1028,22 +1070,30 @@ func (s *Service) consultCharge(ctx context.Context, row chargeRow, actorID, act
 	if err != nil {
 		return ChargeResult{}, err
 	}
-	status := normalizeChargeStatus(responseStatus(response))
+	outcome := extractProviderOutcome(response)
+	status := normalizeChargeStatus(outcome.Status)
 	if status == "" {
-		// AppyPay não devolveu nenhum campo de status desta vez — mantém o
-		// status anterior (row.Status já vem normalizado por loadCharge) em
-		// vez de assumir um novo estado.
+		// AppyPay não devolveu nenhum campo de status reconhecível desta vez
+		// — mantém o status anterior (row.Status já vem normalizado por
+		// loadCharge) em vez de assumir um novo estado.
 		status = row.Status
 	}
 	previousResponse := row.Payload["response"]
-	payload := make(map[string]any, len(row.Payload)+3)
+	payload := make(map[string]any, len(row.Payload)+7)
 	for key, value := range row.Payload {
 		payload[key] = value
 	}
 	payload["provider_charge_id"] = first(responseID(response), row.ProviderID)
 	payload["status"] = status
 	payload["response"] = sanitize(response)
+	applyProviderOutcome(payload, outcome)
 	providerID := first(responseID(response), row.ProviderID)
+	result := ChargeResult{ID: row.ID, ProviderChargeID: providerID, MerchantTransactionID: row.Merchant, Status: status, Response: response}
+	if outcome.HasCode {
+		code := outcome.Code
+		result.CodigoProvedor = &code
+	}
+	result.MensagemProvedor, result.FonteProvedor, result.CategoriaMotivo = outcome.Message, outcome.Source, outcome.Categoria
 	if strings.EqualFold(row.Status, "cancelada") && isSuccessfulChargeStatus(status) {
 		// Keep the cancellation definitive in the read model. The provider
 		// result is recorded for manual FPP reconciliation instead of silently
@@ -1053,14 +1103,15 @@ func (s *Service) consultCharge(ctx context.Context, row chargeRow, actorID, act
 		if err = s.record(ctx, row.ID, "CobrancaAppyPayConflitoPosCancelamento", payload, actorID, actorType, ip); err != nil {
 			return ChargeResult{}, err
 		}
-		return ChargeResult{ID: row.ID, ProviderChargeID: providerID, MerchantTransactionID: row.Merchant, Status: "cancelada", Response: response}, nil
+		result.Status = "cancelada"
+		return result, nil
 	}
 	if status != row.Status || providerID != row.ProviderID || !sameJSON(payload["response"], previousResponse) {
 		if err = s.record(ctx, row.ID, "CobrancaAppyPayConsultada", payload, actorID, actorType, ip); err != nil {
 			return ChargeResult{}, err
 		}
 	}
-	return ChargeResult{ID: row.ID, ProviderChargeID: providerID, MerchantTransactionID: row.Merchant, Status: status, Response: response}, nil
+	return result, nil
 }
 
 type credentialSecrets struct {
@@ -1371,8 +1422,9 @@ func (s *Service) AcceptWebhook(ctx context.Context, metodo, eventID string, own
 	// era gravado em WebhookAppyPayRecebido (acima) mas nunca refletia em
 	// financeiro_cobrancas, deixando a cobrança "presa" em
 	// aguardando_pagamento até alguém consultá-la manualmente.
-	if raw := responseStatus(payload); raw != "" {
-		normalized := normalizeChargeStatus(raw)
+	outcome := extractProviderOutcome(payload)
+	if outcome.Status != "" || outcome.HasCode {
+		normalized := normalizeChargeStatus(outcome.Status)
 		success := isSuccessfulChargeStatus(normalized)
 		if charge, loadErr := s.loadCharge(ctx, eventID); loadErr == nil && charge.Contexto == owner.ContextoTipo && charge.Academia == owner.CodigoAcademia &&
 			// Um webhook atrasado e não-bem-sucedido nunca sobrescreve uma
@@ -1380,7 +1432,7 @@ func (s *Service) AcceptWebhook(ctx context.Context, metodo, eventID string, own
 			// cancelada) — só um sucesso tem tratamento de conflito próprio
 			// (abaixo) que pode correr por cima de um estado terminal local.
 			(success || !isTerminalChargeStatus(charge.Status)) {
-			updated := make(map[string]any, len(charge.Payload)+3)
+			updated := make(map[string]any, len(charge.Payload)+7)
 			for k, v := range charge.Payload {
 				updated[k] = v
 			}
@@ -1390,6 +1442,7 @@ func (s *Service) AcceptWebhook(ctx context.Context, metodo, eventID string, own
 			}
 			updated["provider_charge_id"] = first(responseID(payload), charge.ProviderID)
 			updated["response"] = sanitize(payload)
+			applyProviderOutcome(updated, outcome)
 			eventType := "CobrancaAppyPayConsultada"
 			if success && strings.EqualFold(charge.Status, "cancelada") {
 				// A provider may still settle a REF/GPO/QR after Spuri's local
@@ -1442,6 +1495,22 @@ func (s *Service) releaseChargeReservation(ctx context.Context, merchant string,
 	return err
 }
 
+// applyPersistedProviderFields preenche os campos de motivo de um
+// ChargeResult a partir de um payload já persistido (financeiro_cobrancas),
+// para que uma resposta idempotente (mesmo merchantTransactionId reenviado)
+// devolva exatamente a mesma informação que a criação original devolveu —
+// ver applyProviderOutcome, gravado neste mesmo payload em CreateCharge/
+// CreateGPOQRCode/consultCharge/AcceptWebhook.
+func applyPersistedProviderFields(result *ChargeResult, payload map[string]any) {
+	if codigo, ok := payload["codigo_provedor"].(float64); ok {
+		c := int(codigo)
+		result.CodigoProvedor = &c
+	}
+	result.MensagemProvedor, _ = payload["mensagem_provedor"].(string)
+	result.FonteProvedor, _ = payload["fonte_provedor"].(string)
+	result.CategoriaMotivo, _ = payload["categoria_motivo"].(string)
+}
+
 func (s *Service) existingChargeResult(ctx context.Context, merchant, contexto, academia string) (ChargeResult, error) {
 	row, err := s.loadCharge(ctx, merchant)
 	if err != nil {
@@ -1453,7 +1522,9 @@ func (s *Service) existingChargeResult(ctx context.Context, merchant, contexto,
 		return ChargeResult{}, ErrConflict
 	}
 	response, _ := row.Payload["response"].(map[string]any)
-	return ChargeResult{ID: row.ID, ProviderChargeID: row.ProviderID, MerchantTransactionID: row.Merchant, Status: row.Status, Response: response}, nil
+	result := ChargeResult{ID: row.ID, ProviderChargeID: row.ProviderID, MerchantTransactionID: row.Merchant, Status: row.Status, Response: response}
+	applyPersistedProviderFields(&result, row.Payload)
+	return result, nil
 }
 
 func (s *Service) existingQRCodeResult(ctx context.Context, merchant, contexto, academia string) (QRCodeResult, error) {
@@ -1473,7 +1544,9 @@ func qrCodeResultFromRow(row chargeRow, contexto, academia string) (QRCodeResult
 	}
 	response, _ := row.Payload["response"].(map[string]any)
 	qr, _ := response["qrCodeArr"].(string)
-	return QRCodeResult{ChargeResult: ChargeResult{ID: row.ID, ProviderChargeID: row.ProviderID, MerchantTransactionID: row.Merchant, Status: row.Status, Response: response}, QRCodeArr: qr}, nil
+	chargeResult := ChargeResult{ID: row.ID, ProviderChargeID: row.ProviderID, MerchantTransactionID: row.Merchant, Status: row.Status, Response: response}
+	applyPersistedProviderFields(&chargeResult, row.Payload)
+	return QRCodeResult{ChargeResult: chargeResult, QRCodeArr: qr}, nil
 }
 
 func sameJSON(a, b any) bool {
@@ -1512,10 +1585,26 @@ func validateCharge(in *ChargeRequest) error {
 		return errors.New("GPO exige paymentInfo.phoneNumber")
 	}
 	if strings.HasPrefix(m, "REF") && len(in.PaymentInfo) > 0 {
-		for _, k := range []string{"referenceNumber", "dueDate", "nib"} {
-			value, ok := in.PaymentInfo[k].(string)
-			if !ok || strings.TrimSpace(value) == "" {
-				return fmt.Errorf("REF com paymentInfo exige %s", k)
+		// Duas formas válidas de paymentInfo para REF:
+		//  1. Só dueDate (o caso introduzido nesta tarefa): a referência
+		//     continua gerada pelo gateway (AppyPay escolhe
+		//     referenceNumber), só o prazo de expiração é customizado —
+		//     ver gerarCobranca em cobranca_geracao.go e o comentário sobre
+		//     a hipótese ainda não confirmada contra o ambiente real da
+		//     AppyPay.
+		//  2. Os três campos completos (referenceNumber+dueDate+nib): a
+		//     forma "referência gerada pelo comerciante" documentada pela
+		//     AppyPay. Nenhum chamador atual usa esta forma — mantida por
+		//     integridade caso um chamador futuro precise dela.
+		_, hasDueDateOnly := in.PaymentInfo["dueDate"].(string)
+		if hasDueDateOnly && len(in.PaymentInfo) == 1 {
+			// válido: só dueDate.
+		} else {
+			for _, k := range []string{"referenceNumber", "dueDate", "nib"} {
+				value, ok := in.PaymentInfo[k].(string)
+				if !ok || strings.TrimSpace(value) == "" {
+					return fmt.Errorf("REF com paymentInfo exige %s (ou apenas dueDate sozinho)", k)
+				}
 			}
 		}
 	}
@@ -1582,6 +1671,17 @@ func responseID(v map[string]any) string {
 			return x
 		}
 	}
+	// GET /charges/{id} (e GET /charges?merchantTransactionId=...) devolvem
+	// tudo dentro de um envelope "payment" — ver
+	// extractProviderOutcome, logo abaixo, para o mesmo problema aplicado ao
+	// status.
+	if payment, ok := v["payment"].(map[string]any); ok {
+		for _, k := range []string{"id", "chargeId", "charge_id"} {
+			if x, ok := payment[k].(string); ok {
+				return x
+			}
+		}
+	}
 	return ""
 }
 func responseStatus(v map[string]any) string {
@@ -1592,6 +1692,318 @@ func responseStatus(v map[string]any) string {
 	}
 	return ""
 }
+
+// providerOutcome carrega tudo o que a Spuri consegue aprender de uma única
+// resposta/webhook da AppyPay sobre o resultado real de uma cobrança: o
+// status usado para acionar a máquina de estados interna (Status, já
+// resolvido — ver resolveOutcomeStatus), mais tudo o que é preciso para
+// explicar "porquê" a um humano (Code/Message/Source, crus, exatamente como
+// a AppyPay os enviou) e uma categoria de melhor esforço para filtragem
+// programática (Categoria).
+type providerOutcome struct {
+	Status    string
+	Code      int
+	HasCode   bool
+	Message   string
+	Source    string
+	Categoria string
+}
+
+// extractProviderOutcome é o único ponto do módulo que sabe interpretar as
+// DUAS formas reais (e diferentes entre si) em que a AppyPay envia o estado
+// de uma cobrança — nenhuma delas é um campo "status"/"state" no nível raiz
+// do JSON, que é tudo que responseStatus (acima) sempre soube ler:
+//
+//  1. POST /charges, POST /qr-codes e o webhook transacional: aninhado em
+//     responseStatus.{status,code,message,source} — ver a secção "Common
+//     Response Body for Charges" e "Merchant Webhooks" da documentação da
+//     AppyPay.
+//  2. GET /charges/{id} (e por merchantTransactionId): aninhado em
+//     payment.status, com o motivo mais fino ainda mais dentro, em
+//     payment.transactionEvents[i].responseStatus.{code,message,source} —
+//     ver a secção "Get a charge".
+//
+// Antes desta função, appypay.go só sabia ler a forma (nunca documentada
+// para cobranças) de um "status" solto na raiz — por isso, contra tráfego
+// real da AppyPay, o status de uma cobrança nunca era atualizado depois da
+// criação: nem por consulta (ConsultCharge), nem por webhook
+// (AcceptWebhook). Ver
+// "docs/Debbugs/Depurar mecanismo ausente para status Cancelled do GPO e Expired do REF.md"
+// para o histórico completo desta investigação.
+//
+// Sobre transactionEvents (forma 2): a documentação só mostra um exemplo
+// com um único elemento, então não há confirmação de que o array esteja
+// ordenado cronologicamente. Assume-se aqui, de forma explícita, o ÚLTIMO
+// elemento como o mais recente (mesma ordem do único exemplo disponível).
+// Se uma cobrança real com múltiplas tentativas mostrar isso errado,
+// revisar para escolher por sourceDetails.attempt em vez da posição no
+// array.
+func extractProviderOutcome(v map[string]any) providerOutcome {
+	if payment, ok := v["payment"].(map[string]any); ok {
+		out := providerOutcome{}
+		if s, ok := payment["status"].(string); ok {
+			out.Status = s
+		}
+		if events, ok := payment["transactionEvents"].([]any); ok && len(events) > 0 {
+			if last, ok := events[len(events)-1].(map[string]any); ok {
+				if rs, ok := last["responseStatus"].(map[string]any); ok {
+					applyResponseStatus(&out, rs)
+				}
+			}
+		}
+		if out.Status != "" || out.HasCode {
+			resolveOutcomeStatus(&out)
+			return out
+		}
+	}
+	if rs, ok := v["responseStatus"].(map[string]any); ok {
+		out := providerOutcome{}
+		applyResponseStatus(&out, rs)
+		if out.Status != "" || out.HasCode {
+			resolveOutcomeStatus(&out)
+			return out
+		}
+	}
+	// Nível de segurança: um "status"/"state" solto na raiz nunca é
+	// documentado para cobranças em nenhum dos três fluxos (POST, GET,
+	// webhook), mas mantemos esta leitura para não regredir silenciosamente
+	// caso a AppyPay volte a enviar algo assim para um recurso não previsto.
+	out := providerOutcome{Status: responseStatus(v)}
+	resolveOutcomeStatus(&out)
+	return out
+}
+
+func applyResponseStatus(out *providerOutcome, rs map[string]any) {
+	if s, ok := rs["status"].(string); ok {
+		out.Status = s
+	}
+	if m, ok := rs["message"].(string); ok {
+		out.Message = m
+	}
+	if src, ok := rs["source"].(string); ok {
+		out.Source = src
+	}
+	switch c := rs["code"].(type) {
+	case float64:
+		out.Code = int(c)
+		out.HasCode = true
+	case string:
+		if n, err := strconv.Atoi(strings.TrimSpace(c)); err == nil {
+			out.Code = n
+			out.HasCode = true
+		}
+	}
+}
+
+// resolveOutcomeStatus usa appyPayCodeOutcomes como fonte de verdade quando
+// há um código reconhecido — a própria AppyPay instrui isto na secção
+// "Error Handling Actions" da documentação ("update payment status to the
+// one stated in the description"), e o schema genérico de
+// responseStatus.status documenta só 4 valores (Requested/Pending/Success/
+// Failed) mesmo descrevendo, nas tabelas de código, Cancelled e Expired
+// como desfechos de negócio válidos — ou seja, o texto literal de "status"
+// não é uma fonte confiável para distinguir estes dois casos do "Failed"
+// genérico, mas o código numérico é.
+//
+// Preditivo por design: um código que a Spuri ainda não conhece nunca fica
+// sem classificação alguma — Categoria vira "desconhecido" e, se nenhum
+// status literal tiver vindo junto, Status vira "Failed" (nunca "" nem
+// "Success"), para que uma cobrança jamais fique presa silenciosamente em
+// aguardando_pagamento só porque o código é novo.
+func resolveOutcomeStatus(out *providerOutcome) {
+	if out.HasCode {
+		if info, ok := appyPayCodeOutcomes[out.Code]; ok {
+			out.Status = info.Estado
+			out.Categoria = info.Categoria
+			return
+		}
+		out.Categoria = "desconhecido"
+		if out.Status == "" {
+			out.Status = "Failed"
+		}
+	}
+}
+
+// applyProviderOutcome grava em payload os campos adicionais de motivo de
+// um providerOutcome (quando presentes). Nunca sobrescreve com valor vazio
+// — chamado sempre depois de payload["status"] já ter sido definido a
+// partir de normalizeChargeStatus(outcome.Status).
+func applyProviderOutcome(payload map[string]any, outcome providerOutcome) {
+	if outcome.HasCode {
+		payload["codigo_provedor"] = outcome.Code
+	}
+	if outcome.Message != "" {
+		payload["mensagem_provedor"] = outcome.Message
+	}
+	if outcome.Source != "" {
+		payload["fonte_provedor"] = outcome.Source
+	}
+	if outcome.Categoria != "" {
+		payload["categoria_motivo"] = outcome.Categoria
+	}
+}
+
+type appyPayCodeInfo struct {
+	Estado    string // vocabulário literal da AppyPay: Success|Pending|Cancelled|Expired|Failed
+	Categoria string // agrupamento mais fino da Spuri, para filtragem/relatórios
+}
+
+// appyPayCodeOutcomes traduz o código numérico de responseStatus.code (ou do
+// último transactionEvent em GET /charges) para o par (Estado, Categoria)
+// documentado nas tabelas "HTTP 2xx/4xx/5xx Responses" e "Deprecated
+// Responses" da documentação da AppyPay. Cobre todos os métodos de
+// pagamento documentados (não só GPO/REF, que são o escopo atual da Spuri)
+// deliberadamente: um código UMM/FTBAI/SDD aqui é inofensivo (nunca deve
+// ocorrer, dado que validateCharge só aceita GPO/REF/GPO_QR) e mantém a
+// tabela pronta caso o escopo mude — ver "É essencial que o código seja
+// preditivo" na tarefa que originou esta correção.
+//
+// Códigos marcados "deprecated" na documentação são mantidos aqui: a
+// AppyPay pode continuar a enviá-los para integrações mais antigas, e um
+// código depreciado ausente desta tabela cairia no fallback "desconhecido"
+// sem necessidade.
+var appyPayCodeOutcomes = map[int]appyPayCodeInfo{
+	// ---- Sucesso / pendente ------------------------------------------------
+	100:  {"Success", ""},
+	101:  {"Pending", ""},
+	102:  {"Pending", ""},
+	103:  {"Success", ""},
+	319:  {"Pending", ""},
+	1100: {"Success", ""},
+
+	// ---- GPO: recusado pelo cliente ----------------------------------------
+	231: {"Cancelled", "recusado_pelo_cliente"},
+	219: {"Cancelled", "recusado_pelo_cliente"}, // deprecated, mesmo motivo que 231
+
+	// ---- GPO: saldo insuficiente --------------------------------------------
+	209: {"Cancelled", "saldo_insuficiente"},
+	203: {"Cancelled", "saldo_insuficiente"}, // deprecated
+	204: {"Cancelled", "saldo_insuficiente"}, // deprecated
+
+	// ---- GPO: tempo esgotado -------------------------------------------------
+	210: {"Cancelled", "tempo_esgotado"},
+	211: {"Cancelled", "tempo_esgotado"},
+
+	// ---- GPO: recusado pelo processador/terminal/estabelecimento ------------
+	200: {"Cancelled", "recusado_pelo_processador"},
+	206: {"Cancelled", "recusado_pelo_processador"},
+	208: {"Cancelled", "recusado_pelo_processador"},
+	217: {"Cancelled", "recusado_pelo_processador"},
+	227: {"Cancelled", "recusado_pelo_processador"},
+	230: {"Cancelled", "recusado_pelo_processador"},
+	205: {"Cancelled", "recusado_pelo_processador"}, // deprecated
+	207: {"Cancelled", "recusado_pelo_processador"}, // deprecated
+	218: {"Cancelled", "recusado_pelo_processador"}, // deprecated
+	226: {"Cancelled", "recusado_pelo_processador"}, // deprecated
+
+	// ---- GPO: recusado pelo banco emissor -----------------------------------
+	201: {"Cancelled", "recusado_pelo_emissor"},
+	212: {"Cancelled", "recusado_pelo_emissor"},
+	213: {"Cancelled", "recusado_pelo_emissor"},
+	214: {"Cancelled", "recusado_pelo_emissor"},
+	215: {"Cancelled", "recusado_pelo_emissor"},
+	216: {"Cancelled", "recusado_pelo_emissor"},
+	220: {"Cancelled", "recusado_pelo_emissor"},
+	221: {"Cancelled", "recusado_pelo_emissor"},
+	222: {"Cancelled", "recusado_pelo_emissor"},
+	223: {"Cancelled", "recusado_pelo_emissor"},
+	224: {"Cancelled", "recusado_pelo_emissor"},
+	225: {"Cancelled", "recusado_pelo_emissor"},
+	228: {"Cancelled", "recusado_pelo_emissor"},
+	229: {"Cancelled", "recusado_pelo_emissor"},
+	202: {"Cancelled", "recusado_pelo_emissor"}, // deprecated
+
+	// ---- REF: referência expirada (dueDate ultrapassada) --------------------
+	245: {"Expired", "referencia_expirada"},
+
+	// ---- REF: erro de validação na criação (não deveria chegar após a
+	// cobrança já existir; mantido por integridade) --------------------------
+	762: {"Failed", "referencia_invalida"},
+	763: {"Failed", "referencia_duplicada"},
+
+	// ---- UMM (fora do escopo atual da Spuri — ver comentário acima) --------
+	233: {"Cancelled", "conta_invalida"},
+	238: {"Cancelled", "recusado_pelo_processador"},
+	239: {"Cancelled", "recusado_pelo_cliente"},
+	240: {"Cancelled", "recusado_pelo_processador"},
+	242: {"Cancelled", "conta_invalida"},
+	243: {"Cancelled", "pin_invalido"},
+	244: {"Cancelled", "erro_interno_provedor"},
+	246: {"Failed", "erro_interno_provedor"},
+	247: {"Failed", "erro_interno_provedor"},
+	248: {"Failed", "erro_interno_provedor"},
+	309: {"Failed", "erro_comunicacao"},
+	310: {"Failed", "erro_interno_provedor"},
+	311: {"Failed", "erro_interno_provedor"},
+	312: {"Failed", "erro_interno_provedor"},
+	313: {"Failed", "erro_interno_provedor"},
+	314: {"Failed", "erro_comunicacao"},
+	315: {"Failed", "erro_interno_provedor"},
+	316: {"Failed", "erro_interno_provedor"},
+	317: {"Failed", "erro_interno_provedor"},
+	413: {"Failed", "erro_interno_provedor"},
+	414: {"Failed", "erro_interno_provedor"},
+	415: {"Failed", "erro_interno_provedor"},
+	416: {"Failed", "erro_interno_provedor"},
+	417: {"Failed", "erro_interno_provedor"},
+	418: {"Failed", "erro_interno_provedor"},
+	759: {"Failed", "valor_minimo"},
+
+	// ---- FTBAI (fora do escopo atual da Spuri) ------------------------------
+	249: {"Cancelled", "erro_interno_provedor"},
+	318: {"Failed", "erro_interno_provedor"},
+
+	// ---- Genéricos / erro interno do provedor (principalmente GPO) --------
+	301:  {"Failed", "erro_interno_provedor"},
+	302:  {"Failed", "erro_interno_provedor"},
+	306:  {"Failed", "erro_comunicacao"},
+	308:  {"Failed", "erro_interno_provedor"},
+	402:  {"Failed", "erro_interno_provedor"},
+	403:  {"Failed", "erro_interno_provedor"},
+	404:  {"Failed", "erro_interno_provedor"},
+	405:  {"Failed", "erro_interno_provedor"},
+	406:  {"Failed", "erro_comunicacao"},
+	407:  {"Failed", "erro_comunicacao"},
+	408:  {"Failed", "erro_interno_provedor"},
+	410:  {"Failed", "erro_interno_provedor"},
+	411:  {"Failed", "erro_interno_provedor"},
+	412:  {"Failed", "erro_interno_provedor"},
+	440:  {"Failed", "erro_interno_provedor"},
+	900:  {"Failed", "erro_desconhecido"},
+	901:  {"Failed", "erro_desconhecido"},
+	1101: {"Failed", "conta_inativa"},
+	1102: {"Failed", "conta_inativa"},
+	1103: {"Failed", "conta_inativa"},
+	1104: {"Failed", "conta_inativa"},
+	1105: {"Failed", "conta_inativa"},
+	-1:   {"Failed", "erro_desconhecido"},
+
+	// ---- Validação na criação (4xx) — não deveriam chegar após uma cobrança
+	// já existir, mas documentados por integridade da tabela ------------------
+	717: {"Failed", "dados_invalidos"},
+	718: {"Failed", "dados_invalidos"},
+	719: {"Failed", "dados_invalidos"},
+	720: {"Failed", "transacao_nao_encontrada"},
+	726: {"Failed", "referencia_duplicada"},
+	760: {"Failed", "dados_invalidos"},
+	761: {"Failed", "dados_invalidos"},
+	800: {"Failed", "dados_invalidos"},
+	803: {"Failed", "dados_invalidos"},
+
+	// ---- Deprecated (GPO) — erros internos de captura/cancelamento do lado
+	// da AppyPay; mantidos por integridade histórica -------------------------
+	500: {"Failed", "erro_interno_provedor"},
+	501: {"Failed", "erro_interno_provedor"},
+	502: {"Failed", "erro_interno_provedor"},
+	503: {"Failed", "erro_interno_provedor"},
+	504: {"Failed", "erro_interno_provedor"},
+	505: {"Failed", "erro_interno_provedor"},
+	507: {"Failed", "erro_interno_provedor"},
+	508: {"Failed", "erro_interno_provedor"},
+	801: {"Failed", "dados_invalidos"},
+	802: {"Failed", "dados_invalidos"},
+}
+
 func first(a, b string) string {
 	if a != "" {
 		return a
```

### 3.2 `internal/finance/appypay_integration_test.go`

```diff
diff --git a/internal/finance/appypay_integration_test.go b/internal/finance/appypay_integration_test.go
index dce5ea2..460ae1b 100644
--- a/internal/finance/appypay_integration_test.go
+++ b/internal/finance/appypay_integration_test.go
@@ -17,10 +17,40 @@ import (
 
 type appyPayMockTransport struct {
 	status string
+	// code/message/source, quando definidos, populam responseStatus.code/
+	// message/source (POST) ou o transactionEvent mais recente (GET),
+	// simulando o motivo detalhado que a AppyPay envia junto do status —
+	// ver extractProviderOutcome em appypay.go.
+	code    int
+	message string
+	source  string
 }
 
+func (t *appyPayMockTransport) responseStatusJSON() string {
+	code, message, source := t.code, t.message, t.source
+	if source == "" {
+		source = "GPO"
+	}
+	extra := ""
+	if code != 0 {
+		extra = fmt.Sprintf(`,"code":%d`, code)
+	}
+	if message != "" {
+		extra += fmt.Sprintf(`,"message":%q`, message)
+	}
+	return fmt.Sprintf(`{"successful":true,"status":%q,"source":%q%s}`, t.status, source, extra)
+}
+
+// RoundTrip simula as DUAS formas reais (e diferentes entre si) da AppyPay
+// devolver o estado de uma cobrança — nunca um "status" solto na raiz, que
+// é o que este mock simulava antes desta correção (e que nenhum endpoint
+// real da AppyPay documenta para cobranças; ver
+// docs/Debbugs/Depurar mecanismo ausente para status Cancelled do GPO e Expired do REF.md):
+//   - POST /charges e POST /qr-codes: aninhado em "responseStatus".
+//   - GET /charges/{id}: aninhado em "payment", com o motivo detalhado no
+//     transactionEvent mais recente.
 func (t *appyPayMockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
-	body := `{"id":"provider-charge","status":"Pending"}`
+	body := `{"id":"provider-charge","responseStatus":` + t.responseStatusJSON() + `}`
 	switch {
 	case strings.Contains(req.URL.Path, "/oauth2/token"):
 		body = `{"access_token":"test-token","expires_in":3600}`
@@ -29,11 +59,11 @@ func (t *appyPayMockTransport) RoundTrip(req *http.Request) (*http.Response, err
 		if providerID == req.URL.EscapedPath() || providerID == "" {
 			providerID = req.URL.Query().Get("merchantTransactionId")
 		}
-		body = `{"id":"` + providerID + `","status":"` + t.status + `"}`
+		body = `{"payment":{"id":"` + providerID + `","status":"` + t.status + `","transactionEvents":[{"responseStatus":` + t.responseStatusJSON() + `}]}}`
 	case strings.HasSuffix(req.URL.Path, "/qr-codes"):
-		body = `{"id":"` + t.providerID("qr") + `","status":"Pending","qrCodeArr":"base64-qr"}`
+		body = `{"id":"` + t.providerID("qr") + `","responseStatus":{"successful":true,"status":"Pending","source":"GPO"},"qrCodeArr":"base64-qr"}`
 	case req.Method == http.MethodPost:
-		body = `{"id":"` + t.providerID("charge") + `","status":"Pending"}`
+		body = `{"id":"` + t.providerID("charge") + `","responseStatus":{"successful":true,"status":"Pending","source":"GPO"}}`
 	}
 	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
 }
@@ -187,7 +217,7 @@ func TestIntegrationMatriculaWebhookTardioMantemCancelamentoERegistraConflito(t
 	if err = service.CancelarCobrancaMatriculaAberta(context.Background(), codigo, "solicitação cancelada", uuid.NewString(), "academia", "127.0.0.1"); err != nil {
 		t.Fatal(err)
 	}
-	accepted, err := service.AcceptWebhook(context.Background(), "REF", charge.Charge.ProviderChargeID, WebhookOwner{CredentialID: uuid.New(), ContextoTipo: ContextoAcademia, CodigoAcademia: academia}, map[string]any{"id": charge.Charge.ProviderChargeID, "status": "Success"})
+	accepted, err := service.AcceptWebhook(context.Background(), "REF", charge.Charge.ProviderChargeID, WebhookOwner{CredentialID: uuid.New(), ContextoTipo: ContextoAcademia, CodigoAcademia: academia}, map[string]any{"id": charge.Charge.ProviderChargeID, "responseStatus": map[string]any{"status": "Success", "code": float64(100)}})
 	if err != nil || !accepted {
 		t.Fatalf("webhook tardio = accepted %t, err %v", accepted, err)
 	}
@@ -234,7 +264,7 @@ func TestIntegrationAcceptWebhookReflecteEstadoNaoSucesso(t *testing.T) {
 	}
 
 	owner := WebhookOwner{CredentialID: uuid.New(), ContextoTipo: ContextoAcademia, CodigoAcademia: academia}
-	accepted, err := service.AcceptWebhook(context.Background(), "REF", charge.Charge.ProviderChargeID, owner, map[string]any{"id": charge.Charge.ProviderChargeID, "status": "Expired"})
+	accepted, err := service.AcceptWebhook(context.Background(), "REF", charge.Charge.ProviderChargeID, owner, map[string]any{"id": charge.Charge.ProviderChargeID, "responseStatus": map[string]any{"status": "Failed", "code": float64(245), "message": "The payment has expired", "source": "REF"}})
 	if err != nil || !accepted {
 		t.Fatalf("webhook Expired = accepted %t, err %v", accepted, err)
 	}
@@ -246,7 +276,15 @@ func TestIntegrationAcceptWebhookReflecteEstadoNaoSucesso(t *testing.T) {
 		return status
 	}
 	if got := statusAtual(); got != "Expired" {
-		t.Fatalf("esperava status=Expired refletido na cobrança após o webhook, obteve %q", got)
+		t.Fatalf("esperava status=Expired refletido na cobrança após o webhook (via código 245, mesmo com o literal da AppyPay vindo apenas como Failed), obteve %q", got)
+	}
+	var codigoProvedor int
+	var categoria string
+	if err := client.DB().QueryRow(`SELECT (payload->>'codigo_provedor')::int, payload->>'categoria_motivo' FROM financeiro_cobrancas WHERE id=$1`, charge.Charge.ID).Scan(&codigoProvedor, &categoria); err != nil {
+		t.Fatal(err)
+	}
+	if codigoProvedor != 245 || categoria != "referencia_expirada" {
+		t.Fatalf("motivo persistido = código %d categoria %q, queria 245/referencia_expirada", codigoProvedor, categoria)
 	}
 
 	// Um segundo webhook tardio (ex.: reentrega), com um estado terminal
@@ -254,7 +292,7 @@ func TestIntegrationAcceptWebhookReflecteEstadoNaoSucesso(t *testing.T) {
 	// eventID diferente (aqui o id interno da cobrança, que loadCharge
 	// também reconhece) passa pela deduplicação de
 	// financeiro_webhooks_recebidos para realmente exercer a guarda.
-	accepted2, err := service.AcceptWebhook(context.Background(), "REF", charge.Charge.ID.String(), owner, map[string]any{"id": charge.Charge.ProviderChargeID, "status": "Failed"})
+	accepted2, err := service.AcceptWebhook(context.Background(), "REF", charge.Charge.ID.String(), owner, map[string]any{"id": charge.Charge.ProviderChargeID, "responseStatus": map[string]any{"status": "Failed"}})
 	if err != nil || !accepted2 {
 		t.Fatalf("segundo webhook = accepted %t, err %v", accepted2, err)
 	}
@@ -561,3 +599,75 @@ func TestIntegrationCancelChargeAndLateSuccessConflict(t *testing.T) {
 		t.Fatal("cobrança falhada foi cancelada")
 	}
 }
+
+// TestIntegrationConsultChargeRefleteCanceladoGPOPorCodigo cobre o caso que
+// motivou toda esta correção: uma cobrança GPO criada como Pending, cujo
+// GET /charges/{id} posterior devolve o literal genérico "Failed" (um dos 4
+// valores do schema de responseStatus.status) mas com o código 209 (saldo
+// insuficiente) no transactionEvent mais recente. Antes desta correção,
+// ConsultCharge nunca lia nada disto (procurava "status" solto na raiz, que
+// a AppyPay nunca envia) e a cobrança ficava presa em aguardando_pagamento
+// para sempre. Ver
+// docs/Debbugs/Depurar mecanismo ausente para status Cancelled do GPO e Expired do REF.md
+func TestIntegrationConsultChargeRefleteCanceladoGPOPorCodigo(t *testing.T) {
+	client := integrationClient(t)
+	t.Setenv("ENV", "test")
+	t.Setenv("APPYPAY_RESOURCE", "integration-resource")
+	t.Setenv("FINANCE_ENCRYPTION_KEY", "test-only-secret-material-at-least-32")
+	service := NewService(client)
+	mock := &appyPayMockTransport{status: "Pending"}
+	service.httpClient = &http.Client{Transport: mock}
+	academia := "GPO" + uuid.NewString()[:8]
+	configureIntegrationCredential(t, service, ContextoAcademia, academia)
+
+	charge, err := service.CreateCharge(context.Background(), ChargeRequest{ContextoTipo: ContextoAcademia, CodigoAcademia: academia, Amount: 500, Currency: "AOA", Description: "Propina", MerchantTransactionID: integrationMerchant("G"), PaymentMethod: "GPO", PaymentInfo: map[string]any{"phoneNumber": "900000002"}}, "estudante-1", "estudante", "127.0.0.1")
+	if err != nil {
+		t.Fatal(err)
+	}
+	if charge.Status != EstadoCobrancaAguardandoPagamento {
+		t.Fatalf("status logo após criar = %q, queria %q", charge.Status, EstadoCobrancaAguardandoPagamento)
+	}
+
+	// Simula o que a AppyPay devolveria num GET /charges/{id} depois do
+	// cliente não ter saldo suficiente: literal genérico "Failed" + código
+	// 209 no transactionEvent mais recente.
+	mock.status, mock.code, mock.message, mock.source = "Failed", 209, "Insufficient funds", "GPO"
+	result, err := service.ConsultCharge(context.Background(), ContextoAcademia, academia, charge.MerchantTransactionID, "estudante-1", "estudante", "127.0.0.1")
+	if err != nil {
+		t.Fatal(err)
+	}
+	if result.Status != "Cancelled" {
+		t.Fatalf("status após consulta = %q, queria Cancelled", result.Status)
+	}
+	if result.CodigoProvedor == nil || *result.CodigoProvedor != 209 {
+		t.Fatalf("CodigoProvedor = %v, queria 209", result.CodigoProvedor)
+	}
+	if result.CategoriaMotivo != "saldo_insuficiente" {
+		t.Fatalf("CategoriaMotivo = %q, queria saldo_insuficiente", result.CategoriaMotivo)
+	}
+	if result.MensagemProvedor != "Insufficient funds" {
+		t.Fatalf("MensagemProvedor = %q, queria Insufficient funds", result.MensagemProvedor)
+	}
+
+	// O mesmo motivo persistido deve ser lido de volta por
+	// scanCobrancaResumo (a listagem), não só pelo ChargeResult direto.
+	var codigoNoBanco int
+	var categoriaNoBanco, mensagemNoBanco string
+	if err := client.DB().QueryRow(`SELECT (payload->>'codigo_provedor')::int, payload->>'categoria_motivo', payload->>'mensagem_provedor' FROM financeiro_cobrancas WHERE id=$1`, charge.ID).Scan(&codigoNoBanco, &categoriaNoBanco, &mensagemNoBanco); err != nil {
+		t.Fatal(err)
+	}
+	if codigoNoBanco != 209 || categoriaNoBanco != "saldo_insuficiente" || mensagemNoBanco != "Insufficient funds" {
+		t.Fatalf("motivo persistido = código %d categoria %q mensagem %q", codigoNoBanco, categoriaNoBanco, mensagemNoBanco)
+	}
+
+	// Uma segunda consulta idempotente (mesmo merchantTransactionId,
+	// cobrança já reservada) devolve os mesmos campos de motivo a partir do
+	// payload já persistido (existingChargeResult), não só na primeira vez.
+	again, err := service.CreateCharge(context.Background(), ChargeRequest{ContextoTipo: ContextoAcademia, CodigoAcademia: academia, Amount: 500, Currency: "AOA", Description: "Propina", MerchantTransactionID: charge.MerchantTransactionID, PaymentMethod: "GPO", PaymentInfo: map[string]any{"phoneNumber": "900000002"}}, "estudante-1", "estudante", "127.0.0.1")
+	if err != nil {
+		t.Fatal(err)
+	}
+	if again.CodigoProvedor == nil || *again.CodigoProvedor != 209 || again.CategoriaMotivo != "saldo_insuficiente" {
+		t.Fatalf("resultado idempotente não preservou o motivo: %+v", again)
+	}
+}
```

### 3.3 Novo arquivo `internal/finance/appypay_provider_outcome_test.go`

Criar este arquivo com exatamente este conteúdo:

```go
package finance

import "testing"

// Estes testes cobrem extractProviderOutcome/resolveOutcomeStatus/
// appyPayCodeOutcomes isoladamente (sem PostgreSQL): a extração do status e
// motivo reais de uma resposta/webhook da AppyPay, para as duas formas
// documentadas (POST/webhook via "responseStatus"; GET via "payment") mais
// os cenários de "código desconhecido" e "nenhuma informação disponível".
// Ver docs/Debbugs/Depurar mecanismo ausente para status Cancelled do GPO e Expired do REF.md
// para o histórico completo desta investigação.

func TestExtractProviderOutcomePostShapeGPOCancelledCodes(t *testing.T) {
	// Cobre um representante de cada categoria de motivo documentada para o
	// GPO ser Cancelled — não é uma lista exaustiva dos ~40 códigos
	// mapeados em appyPayCodeOutcomes (isso é responsabilidade do teste de
	// tabela abaixo), mas confirma que o CAMINHO de extração (POST/webhook,
	// responseStatus aninhado) resolve corretamente cada categoria.
	cases := []struct {
		nome              string
		code              float64
		wantCategoria     string
		wantStatusLiteral string // o que a AppyPay realmente manda em responseStatus.status
	}{
		{"saldo insuficiente", 209, "saldo_insuficiente", "Failed"},
		{"timeout do processador", 210, "tempo_esgotado", "Failed"},
		{"timeout da transação", 211, "tempo_esgotado", "Failed"},
		{"recusado pelo cliente", 231, "recusado_pelo_cliente", "Failed"},
		{"recusado pelo processador", 200, "recusado_pelo_processador", "Failed"},
		{"recusado pelo emissor", 201, "recusado_pelo_emissor", "Failed"},
	}
	for _, tc := range cases {
		t.Run(tc.nome, func(t *testing.T) {
			// Simula deliberadamente o cenário mais rigoroso: a AppyPay
			// manda o literal "Failed" (um dos 4 valores do schema genérico
			// de responseStatus.status), e é o CÓDIGO numérico que precisa
			// promover isto para "Cancelled" e para a categoria certa — ver
			// o comentário de resolveOutcomeStatus sobre por que o código é
			// a fonte de verdade, não o literal.
			response := map[string]any{
				"id": "provider-1",
				"responseStatus": map[string]any{
					"successful": false,
					"status":     tc.wantStatusLiteral,
					"code":       tc.code,
					"message":    "mensagem de teste",
					"source":     "GPO",
				},
			}
			outcome := extractProviderOutcome(response)
			if outcome.Status != "Cancelled" {
				t.Fatalf("Status = %q, queria Cancelled (código %v)", outcome.Status, tc.code)
			}
			if outcome.Categoria != tc.wantCategoria {
				t.Fatalf("Categoria = %q, queria %q", outcome.Categoria, tc.wantCategoria)
			}
			if !outcome.HasCode || outcome.Code != int(tc.code) {
				t.Fatalf("Code = %d (HasCode=%t), queria %v", outcome.Code, outcome.HasCode, tc.code)
			}
			if outcome.Message != "mensagem de teste" || outcome.Source != "GPO" {
				t.Fatalf("Message/Source não preservados: %q/%q", outcome.Message, outcome.Source)
			}
			if got := normalizeChargeStatus(outcome.Status); !isTerminalChargeStatus(got) {
				t.Fatalf("normalizeChargeStatus(%q) = %q não é reconhecido como terminal", outcome.Status, got)
			}
		})
	}
}

func TestExtractProviderOutcomeRefExpired(t *testing.T) {
	response := map[string]any{
		"id": "provider-ref-1",
		"responseStatus": map[string]any{
			"successful": false,
			"status":     "Failed", // literal da AppyPay; o código 245 é que manda
			"code":       float64(245),
			"message":    "The payment has expired",
			"source":     "REF",
		},
	}
	outcome := extractProviderOutcome(response)
	if outcome.Status != "Expired" {
		t.Fatalf("Status = %q, queria Expired", outcome.Status)
	}
	if outcome.Categoria != "referencia_expirada" {
		t.Fatalf("Categoria = %q, queria referencia_expirada", outcome.Categoria)
	}
}

func TestExtractProviderOutcomeGetChargeShape(t *testing.T) {
	// Forma de GET /charges/{id}: tudo dentro de "payment", com o motivo
	// fino no transactionEvent mais recente — nunca um "status" solto na
	// raiz nem um "responseStatus" no nível do payment.
	response := map[string]any{
		"payment": map[string]any{
			"id":     "provider-get-1",
			"status": "Failed",
			"transactionEvents": []any{
				map[string]any{
					"id": float64(1),
					"responseStatus": map[string]any{
						"successful": false,
						"status":     "Failed",
						"code":       float64(231),
						"message":    "Payment not authorized by the customer",
						"source":     "UMM",
					},
				},
			},
		},
	}
	outcome := extractProviderOutcome(response)
	if outcome.Status != "Cancelled" {
		t.Fatalf("Status = %q, queria Cancelled", outcome.Status)
	}
	if outcome.Categoria != "recusado_pelo_cliente" {
		t.Fatalf("Categoria = %q, queria recusado_pelo_cliente", outcome.Categoria)
	}
	if outcome.Code != 231 {
		t.Fatalf("Code = %d, queria 231", outcome.Code)
	}
	if id := responseID(response); id != "provider-get-1" {
		t.Fatalf("responseID(payment aninhado) = %q, queria provider-get-1", id)
	}
}

func TestExtractProviderOutcomeGetChargeMultipleEventsUsesLast(t *testing.T) {
	response := map[string]any{
		"payment": map[string]any{
			"id":     "provider-get-2",
			"status": "Failed",
			"transactionEvents": []any{
				map[string]any{"responseStatus": map[string]any{"status": "Failed", "code": float64(210)}},
				map[string]any{"responseStatus": map[string]any{"status": "Failed", "code": float64(231)}},
			},
		},
	}
	outcome := extractProviderOutcome(response)
	if outcome.Code != 231 {
		t.Fatalf("Code = %d, queria 231 (último elemento de transactionEvents)", outcome.Code)
	}
	if outcome.Categoria != "recusado_pelo_cliente" {
		t.Fatalf("Categoria = %q, queria recusado_pelo_cliente", outcome.Categoria)
	}
}

// TestExtractProviderOutcomeCodigoDesconhecidoNuncaFicaSemClassificacao é o
// teste que exercita diretamente o requisito "o código precisa ser
// preditivo e suportar todos os possíveis cenários": um código que a
// AppyPay ainda não documentou (ou que documentou depois desta correção)
// nunca deve deixar a cobrança presa silenciosamente em
// aguardando_pagamento nem virar Success por engano.
func TestExtractProviderOutcomeCodigoDesconhecidoNuncaFicaSemClassificacao(t *testing.T) {
	response := map[string]any{
		"responseStatus": map[string]any{
			"successful": false,
			"status":     "", // cenário mais adversarial: nem o literal ajuda
			"code":       float64(999999),
			"message":    "algo que a Spuri nunca viu antes",
			"source":     "GPO",
		},
	}
	outcome := extractProviderOutcome(response)
	if outcome.Status != "Failed" {
		t.Fatalf("código desconhecido sem status literal: Status = %q, queria Failed (nunca vazio nem Success)", outcome.Status)
	}
	if outcome.Categoria != "desconhecido" {
		t.Fatalf("Categoria = %q, queria desconhecido", outcome.Categoria)
	}
	if outcome.Message != "algo que a Spuri nunca viu antes" {
		t.Fatalf("mensagem crua não preservada para um código desconhecido: %q", outcome.Message)
	}
}

func TestExtractProviderOutcomeCodigoStringENaoFloat(t *testing.T) {
	// Defesa extra: mesmo que a AppyPay um dia mande "code" como string em
	// vez de number, a extração não deve simplesmente ignorar o campo.
	response := map[string]any{
		"responseStatus": map[string]any{"status": "Failed", "code": "245"},
	}
	outcome := extractProviderOutcome(response)
	if !outcome.HasCode || outcome.Code != 245 {
		t.Fatalf("code como string não foi convertido: HasCode=%t Code=%d", outcome.HasCode, outcome.Code)
	}
	if outcome.Status != "Expired" {
		t.Fatalf("Status = %q, queria Expired", outcome.Status)
	}
}

func TestExtractProviderOutcomeSemNenhumaInformacao(t *testing.T) {
	// Nem "payment", nem "responseStatus", nem "status"/"state" soltos —
	// deve devolver Status vazio (o chamador decide o fallback contextual:
	// aguardando_pagamento na criação, status anterior na consulta), nunca
	// inventar um valor.
	outcome := extractProviderOutcome(map[string]any{"id": "sem-informacao"})
	if outcome.Status != "" || outcome.HasCode {
		t.Fatalf("esperava outcome vazio, obteve %+v", outcome)
	}
}

// TestAppyPayCodeOutcomesConsistency garante que a tabela nunca tem uma
// entrada com Estado fora do vocabulário literal da AppyPay, e que todo
// código marcado Cancelled ou Expired tem uma Categoria não vazia (só
// Success/Pending/Failed genéricos podem ter Categoria vazia).
func TestAppyPayCodeOutcomesConsistency(t *testing.T) {
	estadosValidos := map[string]bool{"Success": true, "Pending": true, "Cancelled": true, "Expired": true, "Failed": true}
	for code, info := range appyPayCodeOutcomes {
		if !estadosValidos[info.Estado] {
			t.Errorf("código %d tem Estado %q fora do vocabulário da AppyPay", code, info.Estado)
		}
		if (info.Estado == "Cancelled" || info.Estado == "Expired") && info.Categoria == "" {
			t.Errorf("código %d (%s) deveria ter Categoria preenchida", code, info.Estado)
		}
	}
	// Amostra dos códigos mais importantes para o escopo atual (GPO/REF) —
	// uma regressão aqui indica edição acidental da tabela.
	mustBe := map[int]string{100: "Success", 101: "Pending", 209: "Cancelled", 210: "Cancelled", 211: "Cancelled", 231: "Cancelled", 245: "Expired"}
	for code, want := range mustBe {
		if got := appyPayCodeOutcomes[code].Estado; got != want {
			t.Errorf("appyPayCodeOutcomes[%d].Estado = %q, queria %q", code, got, want)
		}
	}
}
```

---

## 4. Resumo do que cada diff faz (para revisão, não para re-decidir nada)

1. **`ChargeResult`** e **`CobrancaResumo`** ganham 4 campos novos, aditivos (`json:"...,omitempty"`): `codigo_provedor` (`*int`), `mensagem_provedor`, `fonte_provedor`, `categoria_motivo`. Nenhum campo existente muda de nome, tipo ou posição.
2. **`responseID`** passa a também olhar `v["payment"]["id"]` (mesmo problema de aninhamento do GET, só que para o ID em vez do status).
3. Novo tipo `providerOutcome` e função `extractProviderOutcome(v map[string]any) providerOutcome`: o único ponto do módulo que sabe ler as duas formas reais da AppyPay (`payment.*` do GET; `responseStatus.*` do POST/webhook), com fallback defensivo para um `status`/`state` solto na raiz (nunca documentado para cobranças, mantido só por segurança).
4. Nova tabela `appyPayCodeOutcomes` (mapa `int → {Estado, Categoria}`), transcrita diretamente das tabelas "HTTP 2xx/4xx/5xx Responses" e "Deprecated Responses" da documentação da AppyPay — cobre GPO, REF e também UMM/FTBAI (fora do escopo atual da Spuri, mas mantidos por integridade da tabela e para não quebrar se o escopo mudar).
5. `resolveOutcomeStatus`: aplica a tabela quando há código reconhecido; se o código for desconhecido (futuro/não mapeado), marca `categoria="desconhecido"` e nunca deixa `Status` vazio nem `"Success"` por engano — vira `"Failed"` na ausência de qualquer outro sinal. **Esta é a parte "preditiva" pedida**: um código que a AppyPay documentar amanhã e que a Spuri ainda não conheça nunca deixa a cobrança presa silenciosamente nem é tratado como sucesso indevido.
6. Os 4 pontos de leitura de status (`CreateCharge`, `CreateGPOQRCode`, `consultCharge`, `AcceptWebhook`) passam a usar `extractProviderOutcome` em vez do `responseStatus` plano, e persistem os 4 campos novos via `applyProviderOutcome`. Isto é totalmente independente do caminho de erro HTTP que 598e142 corrigiu (esses continuam gravando `"Failed"` diretamente quando a chamada falha).
7. `existingChargeResult`/`qrCodeResultFromRow` (respostas idempotentes) também devolvem os campos de motivo, lidos de volta do payload já persistido.
8. `scanCobrancaResumo` (usado por `ListCobrancas`) também lê os 4 campos novos.
9. O mock de teste compartilhado (`appyPayMockTransport`) passa a simular as formas reais e aninhadas da AppyPay em vez da forma plana — isso re-exercita **toda** a suíte de testes que reusa esse mock (mensalidade, matrícula, QR code, ledger integrity) contra o comportamento real, sem precisar mudar nenhuma asserção deles.
10. Três testes inline de webhook (`Success` após cancelamento, `Expired`, `Failed`) passam a usar a forma aninhada real; o teste de `Expired` ganha asserções extras conferindo `codigo_provedor=245` e `categoria_motivo=referencia_expirada`.
11. Novo teste de integração `TestIntegrationConsultChargeRefleteCanceladoGPOPorCodigo`: cobre o cenário exato que motivou a investigação — GET devolve o literal genérico `"Failed"`, mas o código `209` (saldo insuficiente) no `transactionEvent` mais recente resolve corretamente para `Cancelled`/`saldo_insuficiente`, tanto na primeira consulta quanto numa resposta idempotente subsequente.
12. Novo arquivo `appypay_provider_outcome_test.go`: testes unitários puros (sem Postgres) para `extractProviderOutcome`/`resolveOutcomeStatus`/`appyPayCodeOutcomes`, incluindo o cenário de código desconhecido/futuro (o requisito "preditivo").

---

## 5. Validação já executada por Claude (com PostgreSQL 16 real, sobre o main atual incluindo 598e142)

```
$ go build ./...                    # sem erros
$ go vet ./...                      # sem avisos
$ gofmt -l .                        # sem arquivos com formatação incorreta
$ go test ./... -count=1            # com banco de dados recriado do zero antes de cada execução

ok  	spuri/cmd/server
ok  	spuri/internal/db
ok  	spuri/internal/domain/aggregates
ok  	spuri/internal/finance          (inclui os testes novos desta tarefa E os de 598e142)
ok  	spuri/internal/handlers
ok  	spuri/internal/middleware
ok  	spuri/internal/projections
ok  	spuri/internal/services
ok  	spuri/internal/storage
ok  	spuri/internal/utils
```

Nenhum teste pré-existente (nem os de 598e142, nem os anteriores a ele) precisou de asserção diferente — só a realismo do mock de resposta da AppyPay mudou, e o comportamento observável do sistema (o que cada teste checava) continuou correto.

### 5.1 O que o Codex deve rodar (sem Postgres)

```
go build ./...
go vet ./...
gofmt -l .
go test ./internal/finance/... -run "TestExtractProviderOutcome|TestAppyPayCodeOutcomesConsistency" -v
```

Os dois primeiros devem terminar sem erro algum; o terceiro (`gofmt -l .`) não deve listar nenhum arquivo; o quarto deve mostrar todos os `TestExtractProviderOutcome*` e `TestAppyPayCodeOutcomesConsistency` passando — estes não dependem de Postgres (`RUN_POSTGRES_INTEGRATION` não precisa estar definido).

Se o ambiente do Codex tiver acesso a `proxy.golang.org` (diferente do sandbox de Claude, que teve de usar `replace` temporários em `go.mod` só para compilar offline — **não aplicar nenhum `replace` no `go.mod` real, isso foi só uma limitação pontual do sandbox de Claude e não faz parte desta correção**), o build deve funcionar normalmente sem nenhum passo extra.

Se o ambiente do Codex tiver acesso a PostgreSQL (verificar antes de assumir que não tem), rodar também:

```
RUN_POSTGRES_INTEGRATION=1 DB_HOST=... DB_PORT=... DB_USER=... DB_PASSWORD=... DB_NAME=... DB_SSLMODE=disable \
FINANCE_ENCRYPTION_KEY=<qualquer string de 32+ bytes> APPYPAY_RESOURCE=<qualquer uuid> \
go test ./... -count=1
```

Se não houver Postgres disponível, pular esta etapa e reportar isso claramente no resultado final — não é motivo para bloquear a aplicação do diff, já que Claude já validou esta parte.

---

## 6. Fora de escopo

- Qualquer mudança em `estadosCobrancaEquivalentes`/`normalizeChargeStatus` relacionada a `"falhada"`/`"Failed"` do caminho de erro HTTP — isso é a tarefa 598e142, já mesclada, e não é tocado aqui (os diffs desta tarefa passam por cima dela sem conflito).
- Adicionar um endpoint de cancelamento ativo para GPO/REF na AppyPay — confirmado que não existe (só SDD tem a operação "Cancellation" documentada); `CancelCharge` já trata isso corretamente como cancelamento só do lado da Spuri.
- Qualquer mudança relacionada a dia-limite de pagamento de mensalidade ou `paymentInfo.dueDate` do REF — isso é o escopo da tarefa **71**, um documento separado (dependência conceitual, não técnica: os dois podem ser aplicados em qualquer ordem, mas fazem mais sentido juntos, já que 71 depende de 70 para que o `Expired` de uma referência realmente expirada seja refletido corretamente).
- Qualquer mudança no frontend (`spuripainel`) — não há nada a mudar lá para esta tarefa especificamente (os campos novos são aditivos/`omitempty`; um frontend que não os leia continua funcionando exatamente como antes).
- Backfill de cobranças já existentes: não há dados financeiros reais em produção a preservar (ver comentário original da migration 101), então não é necessário.

## 7. Critérios de aceite

1. Os diffs da seção 3 aplicados exatamente como descritos, sobre o `main` atual (incluindo 598e142).
2. `go build ./...`, `go vet ./...` e `gofmt -l .` limpos.
3. Todos os testes do checklist 5.1 passando.
4. Se Postgres estiver disponível no ambiente do Codex: `go test ./... -count=1` inteiro passando, com banco limpo.
5. Nenhum arquivo fora da lista da seção 3 alterado.

### Procedimento de conclusão

Ao finalizar:

1. Atualizar o título interno deste documento para `# Corrigir a extração de status e motivo real da AppyPay (Cancelled do GPO, Expired do REF) e categorização preditiva de motivos (feito)`;
2. Alterar o front matter para `status: feito` e adicionar `concluido: <data>`;
3. Mover este arquivo para `docs/Tarefas feitas/`.
