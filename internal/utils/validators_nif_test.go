package utils

import "testing"

func TestValidateNIFAcceptsTenDigitsAndLeadingZeros(t *testing.T) {
	if err := ValidateNIF("0012345678"); err != nil {
		t.Fatalf("nif válido rejeitado: %v", err)
	}
}

func TestValidateNIFRejectsInvalidValues(t *testing.T) {
	for _, nif := range []string{"123456789", "12345678901", "12345A7890", "12345 7890", "123.456789"} {
		if err := ValidateNIF(nif); err == nil {
			t.Fatalf("nif inválido aceito: %q", nif)
		}
	}
}
