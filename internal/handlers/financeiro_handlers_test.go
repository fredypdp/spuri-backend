package handlers

import "testing"

func TestFinanceiroMaskSecret(t *testing.T) {
	if got := maskSecret("secret-value-1234"); got != "****1234" {
		t.Fatalf("maskSecret() = %q", got)
	}
	if got := maskSecret("abc"); got != "****" {
		t.Fatalf("short secret must be fully masked, got %q", got)
	}
}

func TestFinanceiroMaskApplicationsHidesSensitiveKeys(t *testing.T) {
	apps := maskApplications([]map[string]interface{}{{"paymentMethod": "REF", "apiKey": "123456", "webhook_token": "token", "applicationId": "app"}})
	if apps[0]["apiKey"] != "****" || apps[0]["webhook_token"] != "****" {
		t.Fatalf("sensitive application values were not masked: %#v", apps[0])
	}
	if apps[0]["paymentMethod"] != "REF" || apps[0]["applicationId"] != "app" {
		t.Fatalf("non-sensitive application metadata changed: %#v", apps[0])
	}
}

func TestFinanceiroEncryptSecretDoesNotPersistPlaintext(t *testing.T) {
	ciphertext, err := encryptSecret("client-secret")
	if err != nil {
		t.Fatal(err)
	}
	if ciphertext == "" || ciphertext == "client-secret" {
		t.Fatalf("secret was not encrypted: %q", ciphertext)
	}
}
