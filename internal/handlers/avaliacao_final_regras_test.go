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

func TestMotivoProgressaoFundamentalSemOferta(t *testing.T) {
	proximo := "6_ano_fundamental"
	motivo := motivoProgressaoFundamentalSemOferta(true, &proximo, []string{"1_ano_fundamental", "5_ano_fundamental"})
	if motivo == nil || *motivo != motivoAcademiaSemOfertaProximoAnoFundamental {
		t.Fatalf("motivoProgressaoFundamentalSemOferta() = %v, want motivo sem oferta", motivo)
	}
}

func TestMotivoProgressaoFundamentalSemOfertaIgnoraQuandoOfertadoOuFinalizado(t *testing.T) {
	proximo := "6_ano_fundamental"
	if motivo := motivoProgressaoFundamentalSemOferta(true, &proximo, []string{"6_ano_fundamental"}); motivo != nil {
		t.Fatalf("motivo ofertado = %v, want nil", *motivo)
	}
	if motivo := motivoProgressaoFundamentalSemOferta(true, nil, []string{"9_ano_fundamental"}); motivo != nil {
		t.Fatalf("motivo ciclo finalizado = %v, want nil", *motivo)
	}
	if motivo := motivoProgressaoFundamentalSemOferta(false, &proximo, nil); motivo != nil {
		t.Fatalf("motivo reprovação = %v, want nil", *motivo)
	}
}

func TestCalcularProximoAnoFundamentalDiferenciaPromocaoFinalizacaoEReprovacao(t *testing.T) {
	proximo, err := calcularProximoAnoFundamental("8_ano_fundamental", true)
	if err != nil {
		t.Fatalf("calcularProximoAnoFundamental() unexpected error = %v", err)
	}
	if proximo == nil || *proximo != "9_ano_fundamental" {
		t.Fatalf("proximo 8º aprovado = %v, want 9_ano_fundamental", proximo)
	}

	finalizado, err := calcularProximoAnoFundamental("9_ano_fundamental", true)
	if err != nil {
		t.Fatalf("calcularProximoAnoFundamental() unexpected error = %v", err)
	}
	if finalizado != nil {
		t.Fatalf("proximo 9º aprovado = %v, want nil para finalização real", *finalizado)
	}

	reprovado, err := calcularProximoAnoFundamental("5_ano_fundamental", false)
	if err != nil {
		t.Fatalf("calcularProximoAnoFundamental() unexpected error = %v", err)
	}
	if reprovado != nil {
		t.Fatalf("proximo reprovado = %v, want nil", *reprovado)
	}
}

func TestMotivoProgressaoFundamentalSemOfertaAcademiaMistaNaoUsaAnoMedio(t *testing.T) {
	proximo := "6_ano_fundamental"
	motivo := motivoProgressaoFundamentalSemOferta(true, &proximo, []string{
		"1_ano_fundamental",
		"2_ano_fundamental",
		"3_ano_fundamental",
		"4_ano_fundamental",
		"5_ano_fundamental",
		"1_ano_medio",
	})
	if motivo == nil || *motivo != motivoAcademiaSemOfertaProximoAnoFundamental {
		t.Fatalf("motivo academia mista sem 6º fundamental = %v, want motivo sem oferta", motivo)
	}
}
