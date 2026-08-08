package finance

import (
	"os"
	"strings"
	"testing"
)

func TestEncryptionRoundTripAndNoFallbackKey(t *testing.T) {
	t.Setenv("FINANCE_ENCRYPTION_KEY", "test-only-secret-material")
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
