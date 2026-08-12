package finance

import (
	"encoding/json"
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
	for _, status := range []string{"cancelada", "FALHADA", "Success", "SUCCESS"} {
		if !isTerminalChargeStatus(status) {
			t.Fatalf("estado terminal %q não foi reconhecido", status)
		}
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

func TestValidHTTPHeaderName(t *testing.T) {
	for _, name := range []string{"X-API-Key", "Authorization", "X-Spuri-Webhook-Secret"} {
		if !validHTTPHeaderName(name) {
			t.Fatalf("nome de cabeçalho válido foi rejeitado: %q", name)
		}
	}
	for _, name := range []string{"", "X API Key", "X-API-Key:"} {
		if validHTTPHeaderName(name) {
			t.Fatalf("nome de cabeçalho inválido foi aceite: %q", name)
		}
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
