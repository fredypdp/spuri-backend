package services

import "testing"

func TestComunicacaoCryptoRoundTrip(t *testing.T) {
	t.Setenv("JWT_SECRET", "uma-chave-unificada-com-pelo-menos-32-caracteres")
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

func TestValidateComunicacaoEncryptionConfigRequiresNonEmptyKey(t *testing.T) {
	t.Setenv("JWT_SECRET", "")
	if err := ValidateComunicacaoEncryptionConfig(); err == nil {
		t.Fatal("ValidateComunicacaoEncryptionConfig() accepted missing key")
	}
	// JWT_SECRET não ganha nenhum requisito novo de tamanho — mesmo um
	// valor curto deve ser aceito, exatamente como já é hoje para a
	// assinatura de tokens JWT.
	t.Setenv("JWT_SECRET", "short")
	if err := ValidateComunicacaoEncryptionConfig(); err != nil {
		t.Fatalf("ValidateComunicacaoEncryptionConfig() rejected a short (but non-empty) key: %v", err)
	}
}
