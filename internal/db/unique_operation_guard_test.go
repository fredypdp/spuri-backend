package db

import "testing"

func TestCanonicalGuardKey(t *testing.T) {
	got := CanonicalGuardKey(" EST-001 ", " Nome ", " Pendente ")
	want := "est-001:nome:pendente"
	if got != want {
		t.Fatalf("CanonicalGuardKey() = %q, want %q", got, want)
	}
}

func TestMaskGuardKeyDoesNotExposeValue(t *testing.T) {
	key := "001LA000000"
	masked := MaskGuardKey(key)
	if masked == "" || masked == key {
		t.Fatalf("MaskGuardKey() = %q, should mask original value", masked)
	}
	if len(masked) != 16 {
		t.Fatalf("MaskGuardKey() length = %d, want 16", len(masked))
	}
}
