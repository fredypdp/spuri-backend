package security

import "testing"

func TestDeriveKeyRequiresSecret(t *testing.T) {
	t.Setenv("JWT_SECRET", "")
	if _, err := DeriveKey("x"); err == nil {
		t.Fatal("esperava erro sem JWT_SECRET")
	}
}

func TestDeriveKeyAcceptsAnyNonEmptySecretLength(t *testing.T) {
	// JWT_SECRET não ganha nenhum requisito novo de tamanho — a mesma
	// configuração usada hoje para assinar tokens JWT também serve para
	// derivar chaves de criptografia, curta ou longa.
	t.Setenv("JWT_SECRET", "123")
	if _, err := DeriveKey("x"); err != nil {
		t.Fatalf("JWT_SECRET curto deveria ser aceito: %v", err)
	}
}

func TestDeriveKeyIsDeterministicAndLabelSeparated(t *testing.T) {
	t.Setenv("JWT_SECRET", "um-segredo-bem-longo-com-mais-de-32-caracteres")
	k1a, err := DeriveKey("finance")
	if err != nil {
		t.Fatal(err)
	}
	k1b, err := DeriveKey("finance")
	if err != nil {
		t.Fatal(err)
	}
	if string(k1a) != string(k1b) {
		t.Fatal("DeriveKey não é determinística para o mesmo label")
	}
	k2, err := DeriveKey("comunicacao")
	if err != nil {
		t.Fatal(err)
	}
	if string(k1a) == string(k2) {
		t.Fatal("labels diferentes produziram a MESMA chave — falha de separação de domínio")
	}
	if len(k1a) != 32 {
		t.Fatalf("chave derivada deveria ter 32 bytes (AES-256), tem %d", len(k1a))
	}
}
