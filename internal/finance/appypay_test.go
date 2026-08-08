package finance

import (
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"spuri/internal/db"
)

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
	for _, event := range []string{"QRCodeAppyPaySolicitado", "QRCodeAppyPayGerado", "QRCodeAppyPayFalhou"} {
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
