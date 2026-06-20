package aggregates

import "testing"

func TestNormalizarCodigoCategoriaNota(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "trim", in: "  nota teste  ", want: "nota_teste"},
		{name: "multiple spaces collapse", in: "nota   teste", want: "nota_teste"},
		{name: "uppercase to lowercase", in: "Nota Teste 2", want: "nota_teste_2"},
		{name: "underscore", in: "nota_teste", want: "nota_teste"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizarCodigoCategoriaNota(tt.in)
			if err != nil {
				t.Fatalf("normalizarCodigoCategoriaNota() unexpected error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("normalizarCodigoCategoriaNota() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizarCodigoCategoriaNotaRejeitaCaracteresInvalidos(t *testing.T) {
	for _, in := range []string{"", "   ", "nota-teste", "nota.teste", "nota/teste", "nota@teste"} {
		t.Run(in, func(t *testing.T) {
			if got, err := normalizarCodigoCategoriaNota(in); err == nil {
				t.Fatalf("normalizarCodigoCategoriaNota() = %q, want error", got)
			}
		})
	}
}

func TestNormalizarCodigoTurma(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "trim", in: "  Turma 10 A  ", want: "Turma_10_A"},
		{name: "multiple spaces collapse", in: "Turma   10", want: "Turma_10"},
		{name: "underscore", in: "Turma_10", want: "Turma_10"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizarCodigoTurma(tt.in)
			if err != nil {
				t.Fatalf("NormalizarCodigoTurma() unexpected error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("NormalizarCodigoTurma() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizarCodigoTurmaRejeitaCaracteresInvalidos(t *testing.T) {
	for _, in := range []string{"", "   ", "Turma-10", "Turma.10", "Turma/10", "Turma@10"} {
		t.Run(in, func(t *testing.T) {
			if got, err := NormalizarCodigoTurma(in); err == nil {
				t.Fatalf("NormalizarCodigoTurma() = %q, want error", got)
			}
		})
	}
}
