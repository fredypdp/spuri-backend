package services

import "testing"

func TestComunicacaoCryptoRoundTrip(t *testing.T) {
	t.Setenv("COMUNICACAO_ENCRYPTION_KEY", "uma-chave-de-comunicacao-com-pelo-menos-32-caracteres")
	cifrado, err := EncryptComunicacaoSegredo("token-secreto")
	if err != nil {
		t.Fatalf("EncryptComunicacaoSegredo() error = %v", err)
	}
	if cifrado == "token-secreto" {
		t.Fatal("EncryptComunicacaoSegredo() returned plaintext")
	}
	plano, err := DecryptComunicacaoSegredo(cifrado)
	if err != nil {
		t.Fatalf("DecryptComunicacaoSegredo() error = %v", err)
	}
	if plano != "token-secreto" {
		t.Fatalf("decrypted value = %q, want token-secreto", plano)
	}
}

func TestValidateComunicacaoEncryptionConfigRejectsMissingOrShortKey(t *testing.T) {
	t.Setenv("COMUNICACAO_ENCRYPTION_KEY", "")
	if err := ValidateComunicacaoEncryptionConfig(); err == nil {
		t.Fatal("ValidateComunicacaoEncryptionConfig() accepted missing key")
	}
	t.Setenv("COMUNICACAO_ENCRYPTION_KEY", "short")
	if err := ValidateComunicacaoEncryptionConfig(); err == nil {
		t.Fatal("ValidateComunicacaoEncryptionConfig() accepted short key")
	}
}
