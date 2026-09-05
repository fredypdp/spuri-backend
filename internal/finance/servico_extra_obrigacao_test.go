package finance

import "testing"

func TestPrecedenciaEstadoServicoExtraObrigacoes(t *testing.T) {
	tests := []struct {
		name    string
		eventos []string
		want    string
	}{
		{"anulada, reativada, paga", []string{"anulada", "reativada", "paga"}, EstadoPago},
		{"paga prevalece sobre anulada", []string{"paga", "anulada"}, EstadoPago},
		{"sem eventos", nil, EstadoPendente},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := precedenciaEstado(tt.eventos); got != tt.want {
				t.Fatalf("precedenciaEstado(%v) = %q, want %q", tt.eventos, got, tt.want)
			}
		})
	}
}
