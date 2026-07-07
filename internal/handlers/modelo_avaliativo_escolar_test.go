package handlers

import (
	"testing"

	"spuri/internal/domain/aggregates"
)

func TestCategoriasEscolaresFixasPorAno(t *testing.T) {
	tests := []struct {
		name   string
		ano    string
		modelo string
		want   []string
	}{
		{"fundamental regular", "5_ano_fundamental", "", []string{"nota_professor", "prova_trimestral"}},
		{"fundamental com exames", "6_ano_fundamental", "", []string{"nota_professor", "prova_trimestral", "exame_final", "exame_recurso"}},
		{"medio regular", "2_ano_medio", "", []string{"nota_professor", "prova_trimestral"}},
		{"medio com exames", "3_ano_medio", "", []string{"nota_professor", "prova_trimestral", "exame_final", "exame_recurso"}},
		{"pap tecnico", "4_ano_medio", aggregates.ModeloCursoMedioTecnico, []string{"nota_pap"}},
		{"4 medio nao tecnico", "4_ano_medio", "geral", []string{"nota_professor", "prova_trimestral"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := codigosCategoriasEscolaresFixasParaAno(tt.ano, tt.modelo)
			if len(got) != len(tt.want) {
				t.Fatalf("len=%d want %d: %#v", len(got), len(tt.want), got)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("got[%d]=%q want %q (all=%#v)", i, got[i], tt.want[i], got)
				}
			}
		})
	}
}

func TestRegraAvaliacaoFinalEscolarFixaPorAno(t *testing.T) {
	regular := regraAvaliacaoFinalEscolarFixa("ACA", "fundamental", "5_ano_fundamental", "normal", nil, "")
	if regular == nil || regular.NotaMinimaAprovacao != 5 || regular.NotaDespertadora == nil || *regular.NotaDespertadora != "prova_trimestral" {
		t.Fatalf("regra regular inesperada: %#v", regular)
	}
	if got := regraAvaliacaoFinalEscolarFixa("ACA", "fundamental", "5_ano_fundamental", "exame_recurso", nil, ""); got != nil {
		t.Fatalf("5_ano_fundamental nao deve ter exame_recurso: %#v", got)
	}

	comExame := regraAvaliacaoFinalEscolarFixa("ACA", "medio", "3_ano_medio", "normal", nil, "")
	if comExame == nil || comExame.NotaMinimaAprovacao != 10 || comExame.NotaDespertadora == nil || *comExame.NotaDespertadora != "exame_final" {
		t.Fatalf("regra com exame inesperada: %#v", comExame)
	}
	recurso := regraAvaliacaoFinalEscolarFixa("ACA", "medio", "3_ano_medio", "exame_recurso", nil, "")
	if recurso == nil || recurso.AplicaSeReprovadoEmType == nil || *recurso.AplicaSeReprovadoEmType != "normal" || recurso.NotaDespertadora == nil || *recurso.NotaDespertadora != "exame_recurso" {
		t.Fatalf("regra de recurso inesperada: %#v", recurso)
	}

	pap := regraAvaliacaoFinalEscolarFixa("ACA", "medio", "4_ano_medio", "normal", nil, aggregates.ModeloCursoMedioTecnico)
	if pap == nil || pap.Nome != "Prova de Aptidão Profissional" || pap.NotaDespertadora == nil || *pap.NotaDespertadora != "nota_pap" {
		t.Fatalf("regra pap inesperada: %#v", pap)
	}
	if got := regraAvaliacaoFinalEscolarFixa("ACA", "medio", "4_ano_medio", "normal", nil, "geral"); got != nil {
		t.Fatalf("4_ano_medio nao tecnico nao deve ter PAP: %#v", got)
	}
}

func TestRegrasAvaliacaoFinalEscolaresFixasFiltraPorCategoriaDescendente(t *testing.T) {
	categoria := "exame_recurso"
	regras := regrasAvaliacaoFinalEscolaresFixas(nil, "ACA", "medio", "3_ano_medio", &categoria, nil)
	if len(regras) != 1 {
		t.Fatalf("len=%d want 1: %#v", len(regras), regras)
	}
	if regras[0].Type != "exame_recurso" || regras[0].NotaDespertadora == nil || *regras[0].NotaDespertadora != "exame_recurso" {
		t.Fatalf("regra filtrada inesperada: %#v", regras[0])
	}
}

func TestListarRegrasAvaliacaoFinalAplicaveisEscolarNaoConsultaConfiguraveisQuandoCategoriaNaoDesperta(t *testing.T) {
	categoria := "nota_professor"
	regras, err := listarRegrasAvaliacaoFinalAplicaveis(nil, "ACA", "fundamental", "5_ano_fundamental", &categoria, nil)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(regras) != 0 {
		t.Fatalf("nota_professor nao deve despertar avaliação final nem cair para regras configuráveis: %#v", regras)
	}
}

func TestValidarEscalaNotaPorAnoAcademico(t *testing.T) {
	validas := []struct {
		ano  string
		nota float64
	}{
		{"1_ano_fundamental", 10},
		{"6_ano_fundamental", 8.5},
		{"7_ano_fundamental", 20},
		{"1_ano_medio", 15.5},
		{"1_ano_superior", 20},
	}
	for _, tt := range validas {
		if err := validarEscalaNotaPorAnoAcademico(tt.ano, tt.nota); err != nil {
			t.Fatalf("%s %.2f deveria ser valido: %v", tt.ano, tt.nota, err)
		}
	}
	invalidas := []struct {
		ano  string
		nota float64
	}{
		{"1_ano_fundamental", 10.01},
		{"6_ano_fundamental", 20},
		{"7_ano_fundamental", 20.01},
		{"1_ano_medio", -0.1},
	}
	for _, tt := range invalidas {
		if err := validarEscalaNotaPorAnoAcademico(tt.ano, tt.nota); err == nil {
			t.Fatalf("%s %.2f deveria ser invalido", tt.ano, tt.nota)
		}
	}
}
