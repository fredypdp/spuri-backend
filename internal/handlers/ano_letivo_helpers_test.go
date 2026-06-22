package handlers

import "testing"

func TestNormalizarPeriodoLetivo(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "normaliza meses sem zero", input: "1_12", want: "01_12"},
		{name: "mantem dois digitos", input: "10_07", want: "10_07"},
		{name: "rejeita mes zero", input: "00_07", wantErr: true},
		{name: "rejeita mes treze", input: "09_13", wantErr: true},
		{name: "rejeita texto extra", input: "09_07_extra", wantErr: true},
		{name: "rejeita texto no mes", input: "09_07abc", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizarPeriodoLetivo(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIntervaloAnoLetivo(t *testing.T) {
	inicio, fim, err := intervaloAnoLetivo("2025_2026", "10_07")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := inicio.Format("2006-01-02"); got != "2025-10-01" {
		t.Fatalf("inicio=%s, want 2025-10-01", got)
	}
	if got := fim.Format("2006-01-02"); got != "2026-07-31" {
		t.Fatalf("fim=%s, want 2026-07-31", got)
	}
}

func TestAnoLetivoValidacaoComparacaoEProximo(t *testing.T) {
	if _, err := parseAnoLetivo("2025_2027"); err == nil {
		t.Fatal("expected invalid sequential year error")
	}
	prox, err := proximoAnoLetivoValidado("2025_2026")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prox != "2026_2027" {
		t.Fatalf("prox=%s, want 2026_2027", prox)
	}
	cmp, err := compareAnoLetivo("2026_2027", "2025_2026")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmp <= 0 {
		t.Fatalf("cmp=%d, want positive", cmp)
	}
}
