package handlers

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

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

func TestRegraAvaliacaoFinalDTOAceitaEscopoMedioPorCurso(t *testing.T) {
	cursoID := uuid.New()
	body := []byte(`{
		"type":"avaliacao_final",
		"nome":"Avaliação Final",
		"nivel":"medio",
		"nota_minima_aprovacao":10,
		"formula":"[nota_exame]",
		"limite_materias_pendentes":2,
		"anos_academicos":[{"curso_id":"` + cursoID.String() + `","anos_academicos":["1_ano_medio","2_ano_medio"]}]
	}`)
	var req regraAvaliacaoFinalDTO
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("json.Unmarshal() unexpected error = %v", err)
	}
	want := []string{cursoID.String() + "|1_ano_medio", cursoID.String() + "|2_ano_medio"}
	if !mesmosAnosAcademicos(req.AnosAcademicos, want) {
		t.Fatalf("AnosAcademicos = %v, want %v", req.AnosAcademicos, want)
	}
	if err := validarCamposPorNivelRegraAvaliacaoFinal(req); err != nil {
		t.Fatalf("validarCamposPorNivelRegraAvaliacaoFinal() unexpected error = %v", err)
	}
	if err := validarEscopoRegraAvaliacaoFinal(req.Nivel, req.AnosAcademicos); err != nil {
		t.Fatalf("validarEscopoRegraAvaliacaoFinal() unexpected error = %v", err)
	}
}

func TestRegraAvaliacaoFinalRejeitaFormatosIncompativeisPorNivel(t *testing.T) {
	cursoID := uuid.New()
	fundamentalComCurso := regraAvaliacaoFinalDTO{Nivel: "fundamental", AnosAcademicos: []string{cursoID.String() + "|1_ano_fundamental"}}
	if err := validarCamposPorNivelRegraAvaliacaoFinal(fundamentalComCurso); err == nil || !strings.Contains(err.Error(), "array simples") {
		t.Fatalf("fundamental com curso deve falhar com erro de array simples, err=%v", err)
	}

	medioLegado := regraAvaliacaoFinalDTO{Nivel: "medio", AnosAcademicos: []string{"1_ano_medio"}, LimiteMateriasPendentes: ptrInt(1)}
	if err := validarCamposPorNivelRegraAvaliacaoFinal(medioLegado); err == nil || !strings.Contains(err.Error(), "array simples legado") {
		t.Fatalf("medio legado deve falhar, err=%v", err)
	}

	superiorComAnos := regraAvaliacaoFinalDTO{Nivel: "superior", AnosAcademicos: []string{"1_ano_superior"}, LimiteMateriasPendentes: ptrInt(1)}
	if err := validarCamposPorNivelRegraAvaliacaoFinal(superiorComAnos); err == nil || !strings.Contains(err.Error(), "superior") {
		t.Fatalf("superior com anos_academicos deve falhar, err=%v", err)
	}
}

func TestValidarEscopoRegraAvaliacaoFinalRejeitaDuplicidadeMedio(t *testing.T) {
	cursoID := uuid.New().String()
	err := validarEscopoRegraAvaliacaoFinal("medio", []string{cursoID + "|1_ano_medio", cursoID + "|1_ano_medio"})
	if err == nil || !strings.Contains(err.Error(), "duplicado") {
		t.Fatalf("duplicidade de curso/ano deve falhar, err=%v", err)
	}
}

func ptrInt(v int) *int { return &v }

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

func TestParseFiltrosAvaliacaoFinalUsaNivelERejeitaTipoEnsino(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("GET", "/avaliacoes?nivel=medio", nil)
	filtros, err := parseFiltrosAvaliacaoFinal(ctx)
	if err != nil {
		t.Fatalf("parseFiltrosAvaliacaoFinal() unexpected error = %v", err)
	}
	if filtros.Nivel == nil || *filtros.Nivel != "medio" {
		t.Fatalf("nivel filter = %#v, want medio", filtros.Nivel)
	}

	ctx, _ = gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("GET", "/avaliacoes?tipo_ensino=medio", nil)
	if _, err := parseFiltrosAvaliacaoFinal(ctx); err == nil || !strings.Contains(err.Error(), "tipo_ensino") {
		t.Fatalf("legacy tipo_ensino filter should be rejected, err=%v", err)
	}
}

func TestRegraAvaliacaoFinalDTONaoExpoeMateriasChave(t *testing.T) {
	body, err := json.Marshal(regraAvaliacaoFinalDTO{Nivel: "medio", Type: "exame", Nome: "Exame", Formula: "[nota_exame]", NotaMinimaAprovacao: 10})
	if err != nil {
		t.Fatalf("json.Marshal() unexpected error = %v", err)
	}
	if strings.Contains(string(body), "materias_chave") {
		t.Fatalf("regraAvaliacaoFinalDTO expôs materias_chave: %s", string(body))
	}
}
