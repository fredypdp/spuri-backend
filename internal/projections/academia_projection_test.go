package projections

import "testing"

func TestNormalizeAcademiaNivelForProjection(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "escolar ledger para coluna escola", input: " escolar ", want: "escola"},
		{name: "superior permanece superior", input: "SUPERIOR", want: "superior"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeAcademiaNivelForProjection(tt.input); got != tt.want {
				t.Fatalf("normalizeAcademiaNivelForProjection(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
