package financeiro

import "testing"

func TestCurrentEnvironmentUsesExecutionEnvironment(t *testing.T) {
	t.Setenv("ENV", "development")
	if got := CurrentEnvironment(); got.Name != "TEST" || got.APIBaseURL != "https://gwy-api-tst.appypay.co.ao/v2.0" {
		t.Fatalf("development environment = %#v, want AppyPay TEST", got)
	}
	t.Setenv("ENV", "production")
	if got := CurrentEnvironment(); got.Name != "PROD" || got.APIBaseURL != "https://gwy-api.appypay.co.ao/v2.0" {
		t.Fatalf("production environment = %#v, want AppyPay PROD", got)
	}
}

func TestCredentialCiphertextRoundTrip(t *testing.T) {
	t.Setenv("FINANCE_ENCRYPTION_KEY", "a-chave-de-teste-nao-vai-para-o-banco")
	ciphertext, err := encrypt("segredo-appypay")
	if err != nil {
		t.Fatal(err)
	}
	if ciphertext == "segredo-appypay" {
		t.Fatal("ciphertext must not expose plaintext")
	}
	plain, err := decrypt(ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	if plain != "segredo-appypay" {
		t.Fatalf("decrypt=%q", plain)
	}
}

func TestSanitizeWebhookPayloadRemovesSecrets(t *testing.T) {
	clean := sanitize(map[string]any{"id": "charge-1", "accessToken": "no", "nested": map[string]any{"apiKey": "no", "status": "paid"}}).(map[string]any)
	if _, ok := clean["accessToken"]; ok {
		t.Fatal("access token was not removed")
	}
	nested := clean["nested"].(map[string]any)
	if _, ok := nested["apiKey"]; ok {
		t.Fatal("API key was not removed")
	}
	if nested["status"] != "paid" {
		t.Fatal("non-secret payload was changed")
	}
}
