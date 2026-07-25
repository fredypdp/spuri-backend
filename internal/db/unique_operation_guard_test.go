package db

import (
	"strings"
	"testing"
)

func TestCanonicalGuardKeyNormalizesParts(t *testing.T) {
	t.Parallel()

	got := CanonicalGuardKey("  EST-001 ", " Nome ", " 2026 ")
	want := "est-001:nome:2026"
	if got != want {
		t.Fatalf("CanonicalGuardKey() = %q, want %q", got, want)
	}
}

func TestMaskGuardKeyDoesNotExposeRawSensitiveValue(t *testing.T) {
	t.Parallel()

	raw := "007123456LA026"
	got := MaskGuardKey(raw)
	if len(got) != 16 {
		t.Fatalf("MaskGuardKey() length = %d, want 16", len(got))
	}
	if strings.Contains(got, raw) || strings.Contains(raw, got) {
		t.Fatalf("MaskGuardKey() exposed raw sensitive value: %q", got)
	}
}
