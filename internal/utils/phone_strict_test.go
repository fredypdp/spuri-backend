package utils

import "testing"

func TestValidatePhoneStrictNationalAcceptsOnlyNineNationalDigits(t *testing.T) {
	t.Parallel()

	if err := ValidatePhoneStrictNational("923456789"); err != nil {
		t.Fatalf("telefone nacional válido foi rejeitado: %v", err)
	}

	invalid := []string{
		"+244923456789",
		"244923456789",
		"923 456 789",
		"923-456-789",
		"(923)456789",
		"923abc789",
		"",
		"   ",
	}
	for _, telefone := range invalid {
		telefone := telefone
		t.Run(telefone, func(t *testing.T) {
			t.Parallel()
			if err := ValidatePhoneStrictNational(telefone); err == nil {
				t.Fatalf("telefone inválido %q foi aceito", telefone)
			}
		})
	}
}
