package handlers

import (
	"testing"
	"time"
)

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

func TestPeriodoFixoPorTipoAnoLetivo(t *testing.T) {
	tests := []struct {
		tipo string
		want string
	}{
		{tipo: "escolar", want: "09_07"},
		{tipo: "superior", want: "10_07"},
	}
	for _, tt := range tests {
		got, err := periodoFixoPorTipoAnoLetivo(tt.tipo)
		if err != nil {
			t.Fatalf("periodoFixoPorTipoAnoLetivo(%q) unexpected error: %v", tt.tipo, err)
		}
		if got != tt.want {
			t.Fatalf("periodoFixoPorTipoAnoLetivo(%q)=%q, want %q", tt.tipo, got, tt.want)
		}
	}
	if _, err := periodoFixoPorTipoAnoLetivo("tecnico"); err == nil {
		t.Fatal("expected unknown type error")
	}
}

func TestValidarPeriodoLetivoFixoPayloadRejeitaDivergente(t *testing.T) {
	if got, err := validarPeriodoLetivoFixoPayload("superior", "10_07"); err != nil || got != "10_07" {
		t.Fatalf("valid fixed superior payload got=%q err=%v", got, err)
	}
	if _, err := validarPeriodoLetivoFixoPayload("superior", "09_07"); err == nil {
		t.Fatal("expected divergent superior periodo to be rejected")
	}
	if _, err := validarPeriodoLetivoFixoPayload("escolar", "10_07"); err == nil {
		t.Fatal("expected divergent escolar periodo to be rejected")
	}
}

func TestIntervaloAnoLetivoPorPeriodosFixos(t *testing.T) {
	tests := []struct {
		tipo       string
		wantInicio string
		wantFim    string
	}{
		{tipo: "escolar", wantInicio: "2025-09-01", wantFim: "2026-07-31"},
		{tipo: "superior", wantInicio: "2025-10-01", wantFim: "2026-07-31"},
	}
	for _, tt := range tests {
		periodo, err := periodoFixoPorTipoAnoLetivo(tt.tipo)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		inicio, fim, err := intervaloAnoLetivo("2025_2026", periodo)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := inicio.Format("2006-01-02"); got != tt.wantInicio {
			t.Fatalf("%s inicio=%s, want %s", tt.tipo, got, tt.wantInicio)
		}
		if got := fim.Format("2006-01-02"); got != tt.wantFim {
			t.Fatalf("%s fim=%s, want %s", tt.tipo, got, tt.wantFim)
		}
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

func TestMesPermiteFinalizacaoAnoLetivo(t *testing.T) {
	mesFim := 7
	mesInicio := 10
	permitidos := map[int]bool{7: true, 8: true, 9: true}
	for mes := 1; mes <= 12; mes++ {
		got := mesPermiteFinalizacaoAnoLetivo(mes, mesFim, mesInicio)
		if got != permitidos[mes] {
			t.Fatalf("mes=%02d got=%v, want=%v", mes, got, permitidos[mes])
		}
	}
}

func TestValidarDataNoPeriodoLetivoFaltasEscolarESuperior(t *testing.T) {
	cases := []struct {
		name    string
		tipo    string
		data    string
		wantErr bool
	}{
		{name: "escolar dentro", tipo: "escolar", data: "2025-09-01"},
		{name: "escolar antes", tipo: "escolar", data: "2025-08-31", wantErr: true},
		{name: "escolar depois", tipo: "escolar", data: "2026-08-01", wantErr: true},
		{name: "superior dentro", tipo: "superior", data: "2025-10-01"},
		{name: "superior antes", tipo: "superior", data: "2025-09-30", wantErr: true},
		{name: "superior depois", tipo: "superior", data: "2026-08-01", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := time.Parse("2006-01-02", tc.data)
			if err != nil {
				t.Fatal(err)
			}
			err = validarDataNoPeriodoLetivo(nil, tc.tipo, "2025_2026", data)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for %s %s", tc.tipo, tc.data)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error for %s %s: %v", tc.tipo, tc.data, err)
			}
		})
	}
}
