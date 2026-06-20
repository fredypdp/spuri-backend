package handlers

import "testing"

func TestNormalizarTypeRegraAvaliacaoFinal(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "trim", in: " normal ", want: "normal"},
		{name: "leading and trailing spaces are trimmed before conversion", in: "  exame final  ", want: "exame_final"},
		{name: "space to underscore", in: "exame final", want: "exame_final"},
		{name: "multiple spaces collapse", in: "exame   final", want: "exame_final"},
		{name: "letters numbers underscore", in: "recurso_2", want: "recurso_2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizarTypeRegraAvaliacaoFinal(tt.in)
			if err != nil {
				t.Fatalf("normalizarTypeRegraAvaliacaoFinal() unexpected error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("normalizarTypeRegraAvaliacaoFinal() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizarTypeRegraAvaliacaoFinalRejeitaCaracteresInvalidos(t *testing.T) {
	tests := []string{
		"",
		"   ",
		"recurso-final",
		"recurso.final",
		"recurso/final",
	}

	for _, tt := range tests {
		t.Run(tt, func(t *testing.T) {
			if got, err := normalizarTypeRegraAvaliacaoFinal(tt); err == nil {
				t.Fatalf("normalizarTypeRegraAvaliacaoFinal() = %q, want error", got)
			}
		})
	}
}

func TestValidarFormulaAvaliacaoExtraiCategorias(t *testing.T) {
	_, categorias, err := validarFormulaAvaliacao("([nota_escola,1_trimestre]+[nota_exame,3_trimestre])/2", nil)
	if err != nil {
		t.Fatalf("validarFormulaAvaliacao() unexpected error = %v", err)
	}
	want := []string{"nota_escola", "nota_exame"}
	if !mesmasCategorias(categorias, want) {
		t.Fatalf("categorias = %v, want %v", categorias, want)
	}
}

func TestValidarFormulaAvaliacaoRejeitaCategoriasInformadasDiferentesDaFormula(t *testing.T) {
	_, _, err := validarFormulaAvaliacao("[nota_escola,1_trimestre]", []string{"nota_escola", "nota_exame"})
	if err == nil {
		t.Fatal("validarFormulaAvaliacao() should reject extra categorias_envolvidas")
	}
}

func TestMesmosAnosAcademicosIgnoraOrdem(t *testing.T) {
	if !mesmosAnosAcademicos([]string{"2_ano_fundamental", "1_ano_fundamental"}, []string{"1_ano_fundamental", "2_ano_fundamental"}) {
		t.Fatal("mesmosAnosAcademicos() should ignore order")
	}
}
