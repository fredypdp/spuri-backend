package db

import "testing"

func TestValidateEventTypeAcceptsCategoriaNotaRemovida(t *testing.T) {
	if err := ValidateEventType("CategoriaNotaRemovida"); err != nil {
		t.Fatalf("ValidateEventType(CategoriaNotaRemovida) unexpected error: %v", err)
	}
}
