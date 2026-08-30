package finance

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"spuri/internal/db"
)

func TestAmountContractRoundingComparisonAndJSONRoundTrip(t *testing.T) {
	const amount = 12345.67
	raw, err := json.Marshal(struct {
		Amount float64 `json:"amount"`
	}{Amount: amount})
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Amount float64 `json:"amount"`
	}
	if err = json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Amount != amount {
		t.Fatalf("round-trip JSON mudou amount: got %v want %v", decoded.Amount, amount)
	}
	if got := roundAmount(10.005); got != 10.01 {
		t.Fatalf("roundAmount(10.005) = %v, want 10.01", got)
	}
	if !amountsEqual(0.1+0.2, 0.3) {
		t.Fatal("erro típico de float64 deveria estar dentro da tolerância monetária")
	}
	if amountsEqual(10.00, 10.01) {
		t.Fatal("valores diferentes por um cêntimo não podem ser iguais")
	}
}

func TestChargeValidationRejectsInvalidMonetaryAmounts(t *testing.T) {
	base := ChargeRequest{
		ContextoTipo:   ContextoAcademia,
		CodigoAcademia: "ACA1",
		Amount:         10,
		Description:    "Teste",
		PaymentMethod:  "GPO",
		PaymentInfo:    map[string]any{"phoneNumber": "900000000"},
	}
	for _, amount := range []float64{15.999, 0, -0.01} {
		in := base
		in.Amount = amount
		if err := validateCharge(&in); err == nil {
			t.Fatalf("amount inválido %v foi aceite", amount)
		}
	}
	min := 1.999
	qr := QRCodeRequest{ContextoTipo: ContextoAcademia, CodigoAcademia: "ACA1", Amount: 10, Description: "Teste", QRCodeType: "MULTIPLE", MinAmount: &min}
	if err := validateQRCode(&qr); err == nil {
		t.Fatal("minAmount com mais de duas casas foi aceite")
	}
}

func TestCancelChargeAuthorizationAndTerminalStatuses(t *testing.T) {
	spuri := chargeRow{Contexto: ContextoSpuri}
	academy := chargeRow{Contexto: ContextoAcademia, Academia: "ACA1"}
	if !canCancelCharge(spuri, "", "admin") {
		t.Fatal("admin deveria poder cancelar cobrança do próprio contexto Spuri")
	}
	if canCancelCharge(academy, "", "admin") {
		t.Fatal("admin não pode cancelar cobrança de academia")
	}
	if !canCancelCharge(academy, "ACA1", "academia") || canCancelCharge(academy, "ACA2", "academia") {
		t.Fatal("isolamento de cancelamento por academia inválido")
	}
	// Antes desta tarefa só "cancelada"/"falhada"/"Success" eram
	// reconhecidos como terminal — "Failed", "Cancelled" e "Expired"
	// (documentados pela própria AppyPay) ficavam de fora, o que permitia
	// re-cancelar uma cobrança já resolvida e mantinha uma "mensalidade
	// aberta" presa para sempre (ver TestIntegrationMensalidadeComCobrancaFalhadaNaAppyPayPermiteNovaTentativa).
	for _, status := range []string{"cancelada", "FALHADA", "Success", "SUCCESS", "Failed", "FAILED", "Cancelled", "CANCELLED", "Expired", "EXPIRED"} {
		if !isTerminalChargeStatus(status) {
			t.Fatalf("estado terminal %q não foi reconhecido", status)
		}
	}
	// aguardando_pagamento (e os valores brutos/locais que normalizeChargeStatus
	// traduz para ele) é o único estado não-terminal possível para uma
	// cobrança real — nunca deve ser tratado como terminal.
	for _, status := range []string{EstadoCobrancaAguardandoPagamento, "Pending", "Requested", "solicitada", "criada", ""} {
		if isTerminalChargeStatus(status) {
			t.Fatalf("estado %q não deveria ser terminal", status)
		}
	}
}

// TestNormalizeChargeStatus cobre a tradução central do novo modelo de
// estados: os valores locais intermediários ("solicitada", "criada") e os
// valores brutos que a própria AppyPay documenta para "cobrança gerada,
// ainda sem resolução" ("Requested", "Pending") devem virar o estado
// canônico único EstadoCobrancaAguardandoPagamento; e, desde a tarefa 69,
// "falhada" (o valor local que CreateCharge/CreateGPOQRCode gravavam antes
// dessa tarefa quando a própria chamada HTTP à AppyPay falhava) deve virar
// "Failed" — em qualquer combinação de maiúsculas/minúsculas, já que nem a
// AppyPay nem o código local garantem uma caixa fixa. Qualquer outro valor
// (terminal ou já canônico) deve passar inalterado — a função é
// idempotente. Entrada vazia continua vazia: quem decide o fallback é o
// chamador (CreateCharge/CreateGPOQRCode tratam "" como aguardando_pagamento;
// consultCharge tem preferido preservar o status anterior).
func TestNormalizeChargeStatus(t *testing.T) {
	awaiting := map[string]bool{
		"Pending": true, "pending": true, "PENDING": true,
		"Requested": true, "requested": true,
		"solicitada": true, "SOLICITADA": true,
		"criada": true, "CRIADA": true,
		EstadoCobrancaAguardandoPagamento: true,
	}
	for raw := range awaiting {
		if got := normalizeChargeStatus(raw); got != EstadoCobrancaAguardandoPagamento {
			t.Fatalf("normalizeChargeStatus(%q) = %q, esperava %q", raw, got, EstadoCobrancaAguardandoPagamento)
		}
	}
	// "falhada" (tarefa 69) — valor local histórico, nunca mais gravado por
	// CreateCharge/CreateGPOQRCode a partir desta tarefa, mas que ainda
	// pode aparecer no payload bruto de cobranças criadas antes do deploy
	// (ledger append-only, imutável) — deve normalizar para "Failed", o
	// mesmo valor que a AppyPay usa quando o processador recusa a
	// cobrança, para a API nunca expor os dois nomes distintos a nenhum
	// chamador.
	for _, raw := range []string{"falhada", "FALHADA", "Falhada"} {
		if got := normalizeChargeStatus(raw); got != "Failed" {
			t.Fatalf("normalizeChargeStatus(%q) = %q, esperava %q", raw, got, "Failed")
		}
	}
	passthrough := []string{"Success", "Failed", "Cancelled", "Expired", "cancelada", "algo-desconhecido"}
	for _, raw := range passthrough {
		if got := normalizeChargeStatus(raw); got != raw {
			t.Fatalf("normalizeChargeStatus(%q) deveria devolver o valor inalterado, obteve %q", raw, got)
		}
	}
	if got := normalizeChargeStatus(""); got != "" {
		t.Fatalf("normalizeChargeStatus(\"\") deveria devolver \"\", obteve %q", got)
	}
	// Idempotência: aplicar duas vezes sobre o próprio resultado não muda
	// nada — importante porque scanCobrancaResumo/loadCharge normalizam
	// tanto valores brutos históricos quanto valores já canônicos.
	for _, raw := range append(passthrough, EstadoCobrancaAguardandoPagamento, "falhada") {
		once := normalizeChargeStatus(raw)
		twice := normalizeChargeStatus(once)
		if once != twice {
			t.Fatalf("normalizeChargeStatus não é idempotente para %q: 1a chamada=%q, 2a chamada=%q", raw, once, twice)
		}
	}
}

// TestEstadosCobrancaEquivalentes cobre a expansão do filtro estado=
// aguardando_pagamento para o conjunto de valores brutos históricos
// equivalentes (ver ListCobrancas/ListCobrancasEstudante) — sem essa
// expansão, filtrar por esse novo estado canônico não encontraria nenhuma
// cobrança criada antes desta tarefa (ainda gravada como "Pending",
// "Requested", "solicitada" ou "criada" no payload do ledger, imutável). E,
// desde a tarefa 69, a expansão irmã de estado=Failed para também incluir
// "falhada" (o valor local que CreateCharge/CreateGPOQRCode gravavam antes
// desta tarefa quando a própria chamada HTTP à AppyPay falhava, nunca
// chegando a existir uma cobrança do lado do provedor) — sem ela, filtrar
// por Failed nunca encontrava cobranças criadas antes desta tarefa, mesmo
// elas sendo, do ponto de vista de quem filtra, tão "falhadas" quanto uma
// recusada pelo processador. Daqui pra frente as duas funções já gravam
// "Failed" diretamente (ver normalizeChargeStatus, que também traduz
// "falhada" para "Failed" na leitura) — esta expansão nunca deixa de ser
// necessária, porque o ledger é append-only e "falhada" continua existindo
// no payload bruto de cobranças antigas para sempre.
func TestEstadosCobrancaEquivalentes(t *testing.T) {
	got := estadosCobrancaEquivalentes([]string{"aguardando_pagamento"})
	esperado := map[string]bool{"aguardando_pagamento": true, "Pending": true, "Requested": true, "solicitada": true, "criada": true}
	if len(got) != len(esperado) {
		t.Fatalf("esperava %d valores equivalentes, obteve %d: %#v", len(esperado), len(got), got)
	}
	for _, v := range got {
		if !esperado[v] {
			t.Fatalf("valor inesperado na expansão: %q (lista completa: %#v)", v, got)
		}
	}
	// estado=Failed expande para também casar com "falhada" (tarefa 69).
	gotFailed := estadosCobrancaEquivalentes([]string{"Failed"})
	esperadoFailed := map[string]bool{"Failed": true, "falhada": true}
	if len(gotFailed) != len(esperadoFailed) {
		t.Fatalf("esperava %d valores equivalentes para Failed, obteve %d: %#v", len(esperadoFailed), len(gotFailed), gotFailed)
	}
	for _, v := range gotFailed {
		if !esperadoFailed[v] {
			t.Fatalf("valor inesperado na expansão de Failed: %q (lista completa: %#v)", v, gotFailed)
		}
	}
	// O inverso não é verdadeiro: filtrar por "falhada" diretamente
	// continua estrito, sem casar com "Failed" — só o valor canônico
	// exposto ao chamador (o que o frontend manda: ver
	// ESTADO_PAGAMENTO_OPCOES em financeiroShared.tsx, que só tem a opção
	// "Failed") expande para os valores brutos históricos equivalentes;
	// "falhada" sozinho só faria sentido numa consulta manual direto no
	// ledger. Qualquer outro estado também passa inalterado — não tem
	// equivalência com outros valores brutos.
	for _, outros := range [][]string{{"Success"}, {"falhada"}, {"Cancelled"}, {"Expired"}, {"pendente"}} {
		out := estadosCobrancaEquivalentes(outros)
		if len(out) != 1 || out[0] != outros[0] {
			t.Fatalf("esperava %v inalterado, obteve %v", outros, out)
		}
	}
	// Uma lista com múltiplos estados só expande o que casa com
	// aguardando_pagamento ou Failed, preservando os demais.
	misto := estadosCobrancaEquivalentes([]string{"Success", "aguardando_pagamento"})
	if len(misto) != 6 {
		t.Fatalf("esperava 6 valores (1 Success + 5 da expansão), obteve %d: %#v", len(misto), misto)
	}
	mistoFailed := estadosCobrancaEquivalentes([]string{"Cancelled", "Failed"})
	if len(mistoFailed) != 3 {
		t.Fatalf("esperava 3 valores (1 Cancelled + 2 da expansão de Failed), obteve %d: %#v", len(mistoFailed), mistoFailed)
	}
}

func TestEncryptionRoundTripAndNoFallbackKey(t *testing.T) {
	t.Setenv("FINANCE_ENCRYPTION_KEY", "test-only-secret-material-at-least-32")
	ciphertext, err := encrypt("segredo AppyPay")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(ciphertext, "segredo") {
		t.Fatal("ciphertext contém texto claro")
	}
	plain, err := decrypt(ciphertext)
	if err != nil || plain != "segredo AppyPay" {
		t.Fatalf("round trip inválido: %q %v", plain, err)
	}
	os.Unsetenv("FINANCE_ENCRYPTION_KEY")
	if _, err := encrypt("x"); err == nil {
		t.Fatal("esperava falha sem FINANCE_ENCRYPTION_KEY")
	}
}

func TestEncryptionKeyRequiresStrongMaterial(t *testing.T) {
	t.Setenv("FINANCE_ENCRYPTION_KEY", "123")
	if err := ValidateEncryptionConfig(); err == nil {
		t.Fatal("chave curta foi aceite")
	}
	t.Setenv("FINANCE_ENCRYPTION_KEY", "test-only-secret-material-at-least-32")
	if err := ValidateEncryptionConfig(); err != nil {
		t.Fatalf("chave válida foi rejeitada: %v", err)
	}
}

func TestWebhookHeaderNameIsFixedGlobalConstant(t *testing.T) {
	if WebhookHeaderName != "X-Spuri-Webhook-Secret" {
		t.Fatalf("WebhookHeaderName mudou de valor sem atualizar esta expectativa: %q", WebhookHeaderName)
	}
}

func TestGenerateWebhookSecretLengthAlphabetAndUniqueness(t *testing.T) {
	first, err := generateWebhookSecret()
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != webhookSecretLength {
		t.Fatalf("segredo de webhook com tamanho %d, queria %d", len(first), webhookSecretLength)
	}
	for _, r := range first {
		if !strings.ContainsRune(webhookSecretAlphabet, r) {
			t.Fatalf("segredo de webhook contém caractere fora do alfabeto esperado: %q", r)
		}
	}
	second, err := generateWebhookSecret()
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("duas chamadas a generateWebhookSecret produziram o mesmo valor")
	}
}

func TestAppyPayResourceConfig(t *testing.T) {
	t.Setenv("APPYPAY_RESOURCE", "")
	if _, err := appyPayResource(); err == nil {
		t.Fatal("esperava falha sem APPYPAY_RESOURCE")
	}
	if err := ValidateAppyPayResourceConfig(); err == nil {
		t.Fatal("validação aceitou APPYPAY_RESOURCE vazia")
	}
	if err := os.Unsetenv("APPYPAY_RESOURCE"); err != nil {
		t.Fatal(err)
	}
	if _, err := appyPayResource(); err == nil {
		t.Fatal("esperava falha com APPYPAY_RESOURCE ausente")
	}
	if err := ValidateAppyPayResourceConfig(); err == nil {
		t.Fatal("validação aceitou APPYPAY_RESOURCE ausente")
	}

	t.Setenv("APPYPAY_RESOURCE", "  resource-de-teste  ")
	if got, err := appyPayResource(); err != nil || got != "resource-de-teste" {
		t.Fatalf("resource inválido: %q, %v", got, err)
	}
	if err := ValidateAppyPayResourceConfig(); err != nil {
		t.Fatalf("validação rejeitou APPYPAY_RESOURCE definida: %v", err)
	}
}

// appyPayTokenTransport simula a resposta do endpoint de token da AppyPay
// com um corpo de expires_in configurável, para exercitar token() com os
// dois formatos que o campo pode assumir na prática.
type appyPayTokenTransport struct{ expiresInJSON string }

func (t appyPayTokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body := `{"access_token":"tok-` + uuid.NewString() + `","expires_in":` + t.expiresInJSON + `}`
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
}

// TestTokenAceitaExpiresInComoStringOuNumero cobre o formato real da
// resposta do endpoint de token da AppyPay (secção "Get a token" de
// docs/Parceiros e integrações/AppyPay Documentação.md): expires_in vem
// como STRING JSON (ex.: "expires_in": "3599") — comportamento do endpoint
// v1 do Azure AD (login.microsoftonline.com/{tenant}/oauth2/token, que é o
// que a AppyPay usa), diferente do endpoint v2.0, que usa número. Antes
// desta correção o campo era um int puro: json.Unmarshal de uma string
// JSON para um campo int falha, e token() tratava qualquer erro de
// unmarshal como falha de autenticação total — mesmo com access_token
// presente e válido no mesmo payload. Cobre também o formato numérico puro,
// usado por todos os mocks de teste deste pacote, para garantir que a
// correção não regride o que já funcionava.
func TestTokenAceitaExpiresInComoStringOuNumero(t *testing.T) {
	t.Setenv("APPYPAY_RESOURCE", "integration-resource")
	casos := []struct {
		nome          string
		expiresInJSON string
	}{
		{"string, formato real da AppyPay", `"3599"`},
		{"número, formato usado pelos mocks de teste", `3600`},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			s := NewService(nil)
			s.SetHTTPClient(&http.Client{Transport: appyPayTokenTransport{expiresInJSON: c.expiresInJSON}})
			token, err := s.token(context.Background(), credentialSecrets{ID: uuid.New(), ClientID: "client-x", ClientSecret: "secret-y"})
			if err != nil {
				t.Fatalf("token() falhou com expires_in=%s: %v", c.expiresInJSON, err)
			}
			if token == "" {
				t.Fatal("token vazio")
			}
		})
	}
}

func TestCredentialMethodMatchesConfiguredIDWithoutCaseSensitivity(t *testing.T) {
	credentials := credentialSecrets{GPO: "GPO_53c70da3-1c88", REF: "REF_42ef"}
	if got, err := credentials.method("gpo_53C70DA3-1C88"); err != nil || got != credentials.GPO {
		t.Fatalf("método GPO configurado não foi reconhecido: %q, %v", got, err)
	}
	if got, err := credentials.method("ref"); err != nil || got != credentials.REF {
		t.Fatalf("atalho REF não foi reconhecido: %q, %v", got, err)
	}
}

func TestQRCodeIdempotencyPayloadAndPersistedResult(t *testing.T) {
	in := QRCodeRequest{ContextoTipo: ContextoAcademia, CodigoAcademia: "ACA1", Amount: 10, Currency: "AOA", Description: "Teste", MerchantTransactionID: "QRTEST00000001"}
	payload := qrCodePayload(uuid.New(), in, "SINGLE", "provider-1", "criada", map[string]any{"qrCodeArr": "base64-qr"})
	if payload["merchant_transaction_id"] != in.MerchantTransactionID || payload["status"] != "criada" {
		t.Fatalf("payload idempotente inválido: %#v", payload)
	}
	row := chargeRow{ID: uuid.New(), ProviderID: "provider-1", Merchant: in.MerchantTransactionID, Contexto: ContextoAcademia, Academia: "ACA1", Status: "criada", Payload: payload}
	result, err := qrCodeResultFromRow(row, ContextoAcademia, "ACA1")
	if err != nil || result.QRCodeArr != "base64-qr" || result.MerchantTransactionID != in.MerchantTransactionID {
		t.Fatalf("resultado persistido de QR inválido: %#v, %v", result, err)
	}
	if _, err := qrCodeResultFromRow(row, ContextoAcademia, "ACA2"); err != ErrConflict {
		t.Fatalf("QR de outra academia foi exposto: %v", err)
	}
	for _, event := range []string{"QRCodeAppyPaySolicitado", "QRCodeAppyPayGerado", "QRCodeAppyPayFalhou", "CobrancaAppyPayCancelada", "CobrancaAppyPayConflitoPosCancelamento"} {
		if err := db.ValidateEventType(event); err != nil {
			t.Fatalf("evento QR não foi autorizado no ledger: %s: %v", event, err)
		}
	}
}

func TestChargeValidationRestrictsScopeAndRequiredMethodData(t *testing.T) {
	base := ChargeRequest{ContextoTipo: ContextoAcademia, CodigoAcademia: "ACA1", Amount: 1, Description: "Teste", PaymentMethod: "GPO"}
	if err := validateCharge(&base); err == nil {
		t.Fatal("GPO sem phoneNumber foi aceite")
	}
	base.PaymentInfo = map[string]any{"phoneNumber": "900000000"}
	if err := validateCharge(&base); err != nil {
		t.Fatal(err)
	}
	base.PaymentMethod = "REF"
	base.PaymentInfo = map[string]any{"referenceNumber": "1"}
	if err := validateCharge(&base); err == nil {
		t.Fatal("REF parcial foi aceite")
	}
	base.PaymentMethod = "UMM"
	base.PaymentInfo = nil
	if err := validateCharge(&base); err == nil {
		t.Fatal("método fora do escopo foi aceite")
	}
}

func TestSanitizeNeverKeepsSecrets(t *testing.T) {
	v := sanitize(map[string]any{"accessToken": "x", "nested": map[string]any{"client_secret": "y", "ok": "z"}}).(map[string]any)
	if _, ok := v["accessToken"]; ok {
		t.Fatal("token não removido")
	}
	nested := v["nested"].(map[string]any)
	if _, ok := nested["client_secret"]; ok || nested["ok"] != "z" {
		t.Fatal("sanitização inválida")
	}
}
