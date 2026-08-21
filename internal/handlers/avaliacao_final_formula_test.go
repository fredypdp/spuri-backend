package handlers

import (
	"strings"
	"testing"
)

func TestFormulaTextualValidaPrecedenciaParentesesPesos(t *testing.T) {
	formula := "([nota_escola,1_trimestre]+[nota_escola,2_trimestre]*2+[nota_exame_final,3_trimestre]*0.5)/2"
	if _, _, err := validarFormulaAvaliacao(formula, []string{"nota_escola", "nota_exame_final"}); err != nil {
		t.Fatalf("formula deveria ser válida: %v", err)
	}

	nota, err := calcularFormulaAvaliacao(formula, map[string]map[string][]float64{
		"nota_escola": {
			"1_trimestre": {10},
			"2_trimestre": {8},
		},
		"nota_exame_final": {
			"3_trimestre": {6},
		},
	})
	if err != nil {
		t.Fatalf("cálculo deveria funcionar: %v", err)
	}
	if nota != 14.5 {
		t.Fatalf("nota final inesperada: got=%v want=14.5", nota)
	}
}

func TestFormulaTextualRejeitaJSONAntigoECaracteresInvalidos(t *testing.T) {
	invalidas := []string{
		`{"op":"add"}`,
		"eval([nota_escola,1_trimestre])",
		"[nota_escola,1_trimestre];DROP TABLE notas",
		"[nota_escola]",
	}
	for _, formula := range invalidas {
		if _, _, err := validarFormulaAvaliacao(formula, []string{"nota_escola"}); err == nil {
			t.Fatalf("formula %q deveria ser inválida", formula)
		}
	}
}

func TestFormulaTextualValidaCategoriaPeriodoDivisaoZeroEAusencia(t *testing.T) {
	if _, _, err := validarFormulaAvaliacao("[nota_inexistente,1_trimestre]", []string{"nota_escola"}); err == nil || !strings.Contains(err.Error(), "categoria") {
		t.Fatalf("categoria fora da lista deveria ser rejeitada, err=%v", err)
	}
	if _, _, err := validarFormulaAvaliacao("[nota_escola,periodo_invalido]", []string{"nota_escola"}); err == nil || !strings.Contains(err.Error(), "periodo") {
		t.Fatalf("periodo inválido deveria ser rejeitado, err=%v", err)
	}
	if _, _, err := validarFormulaAvaliacao("[nota_escola,1_trimestre]/0", []string{"nota_escola"}); err == nil || !strings.Contains(err.Error(), "zero") {
		t.Fatalf("divisão por zero deveria ser rejeitada, err=%v", err)
	}
	if _, err := calcularFormulaAvaliacao("[nota_escola,1_trimestre]", map[string]map[string][]float64{}); err == nil || !strings.Contains(err.Error(), "nota ausente") {
		t.Fatalf("nota ausente deveria impedir cálculo, err=%v", err)
	}
}

func TestFormulaSuperiorDeveConterPeriodoAtual(t *testing.T) {
	formula := "([nota_pp1,1_semestre]+[nota_pp2,1_semestre])/2"
	if err := validarFormulaSuperiorContemPeriodo(formula, "1_semestre"); err != nil {
		t.Fatalf("formula contém período atual: %v", err)
	}
	if err := validarFormulaSuperiorContemPeriodo(formula, "2_semestre"); err == nil {
		t.Fatal("formula sem período atual deveria ser rejeitada")
	}
}

func TestFormulaPorNivelSuperiorInferePeriodoENaoAceitaPeriodoExplicito(t *testing.T) {
	formula := "([nota_pp1]+[nota_pp2])/2"
	if _, _, err := validarFormulaAvaliacaoPorNivel("superior", formula, []string{"nota_pp1", "nota_pp2"}); err != nil {
		t.Fatalf("formula superior sem período explícito deveria ser válida: %v", err)
	}
	if _, _, err := validarFormulaAvaliacaoPorNivel("superior", "[nota_pp1,1_semestre]", []string{"nota_pp1"}); err == nil || !strings.Contains(err.Error(), "periodo é inferido") {
		t.Fatalf("formula superior com período explícito deveria ser rejeitada, err=%v", err)
	}
}

func TestFormulaPorNivelFundamentalMedioExigePeriodoExplicito(t *testing.T) {
	if _, _, err := validarFormulaAvaliacaoPorNivel("fundamental", "[nota_escola]", []string{"nota_escola"}); err == nil || !strings.Contains(err.Error(), "deve informar periodo") {
		t.Fatalf("formula fundamental sem período deveria ser rejeitada, err=%v", err)
	}
	if _, _, err := validarFormulaAvaliacaoPorNivel("medio", "[nota_escola]", []string{"nota_escola"}); err == nil || !strings.Contains(err.Error(), "deve informar periodo") {
		t.Fatalf("formula médio sem período deveria ser rejeitada, err=%v", err)
	}
}

func TestSubstituirNotasAusentesPorZero(t *testing.T) {
	notas := map[string]map[string][]float64{
		"nota_professor": {"1_trimestre": {12}},
	}
	faltantes, err := substituirNotasAusentesPorZero("([nota_professor,1_trimestre]+[prova_trimestral,1_trimestre]+[prova_trimestral,2_trimestre])/3", notas)
	if err != nil {
		t.Fatalf("substituição deveria funcionar: %v", err)
	}
	if len(faltantes) != 2 {
		t.Fatalf("faltantes = %v, want 2", faltantes)
	}
	nota, err := calcularFormulaAvaliacao("([nota_professor,1_trimestre]+[prova_trimestral,1_trimestre]+[prova_trimestral,2_trimestre])/3", notas)
	if err != nil {
		t.Fatalf("cálculo com zeros deveria funcionar: %v", err)
	}
	if nota != 4 {
		t.Fatalf("nota final com zeros = %v, want 4", nota)
	}
}

func TestSubstituirNotasAusentesPorZeroMantemComportamentoComNotasPresentes(t *testing.T) {
	notas := map[string]map[string][]float64{
		"nota_professor":   {"1_trimestre": {10}},
		"prova_trimestral": {"1_trimestre": {8}},
	}
	faltantes, err := substituirNotasAusentesPorZero("([nota_professor,1_trimestre]+[prova_trimestral,1_trimestre])/2", notas)
	if err != nil {
		t.Fatalf("substituição deveria funcionar: %v", err)
	}
	if len(faltantes) != 0 {
		t.Fatalf("não deveria substituir notas presentes: %v", faltantes)
	}
	nota, err := calcularFormulaAvaliacao("([nota_professor,1_trimestre]+[prova_trimestral,1_trimestre])/2", notas)
	if err != nil {
		t.Fatalf("cálculo deveria funcionar: %v", err)
	}
	if nota != 9 {
		t.Fatalf("nota final = %v, want 9", nota)
	}
}

func TestSubstituirNotasAusentesPorZeroSuperiorComPeriodoInferido(t *testing.T) {
	formula := preencherPeriodoFormulaSuperior("([prova_parcelar_1]+[prova_parcelar_2])/2", "1_semestre")
	notas := map[string]map[string][]float64{
		"prova_parcelar_2": {"1_semestre": {14}},
	}
	faltantes, err := substituirNotasAusentesPorZero(formula, notas)
	if err != nil {
		t.Fatalf("substituição superior deveria funcionar: %v", err)
	}
	if len(faltantes) != 1 || faltantes[0].Categoria != "prova_parcelar_1" || faltantes[0].Periodo != "1_semestre" {
		t.Fatalf("faltante superior inesperado: %v", faltantes)
	}
	nota, err := calcularFormulaAvaliacao(formula, notas)
	if err != nil {
		t.Fatalf("cálculo superior com zero deveria funcionar: %v", err)
	}
	if nota != 7 {
		t.Fatalf("nota final superior = %v, want 7", nota)
	}
}

func TestTiposAvaliacaoFinalDespertadosPorCategoriaRaizNaoDisparaDescendente(t *testing.T) {
	notaRaiz := "exame_final"
	notaRecurso := "exame_recurso"
	dep := "normal"
	regras := []regraAvaliacaoFinalDTO{
		{Type: "normal", NotaDespertadora: &notaRaiz},
		{Type: "exame_recurso", AplicaSeReprovadoEmType: &dep, NotaDespertadora: &notaRecurso},
	}

	disparados := tiposAvaliacaoFinalDespertadosPorCategoria(regras, &regras[0], "exame_final", "3_trimestre")
	if !disparados["normal"] {
		t.Fatalf("gatilho da raiz deveria disparar a raiz: %v", disparados)
	}
	if disparados["exame_recurso"] {
		t.Fatalf("gatilho da raiz não deve disparar descendente antes da própria nota de recurso: %v", disparados)
	}
}

func TestTiposAvaliacaoFinalDespertadosPorCategoriaDescendenteDisparaApenasDescendente(t *testing.T) {
	notaRaiz := "exame_final"
	notaRecurso := "exame_recurso"
	dep := "normal"
	regras := []regraAvaliacaoFinalDTO{
		{Type: "normal", NotaDespertadora: &notaRaiz},
		{Type: "exame_recurso", AplicaSeReprovadoEmType: &dep, NotaDespertadora: &notaRecurso},
	}

	disparados := tiposAvaliacaoFinalDespertadosPorCategoria(regras, &regras[0], "exame_recurso", "3_trimestre")
	if disparados["normal"] {
		t.Fatalf("gatilho de descendente não deve recalcular raiz: %v", disparados)
	}
	if !disparados["exame_recurso"] {
		t.Fatalf("gatilho de descendente deveria disparar descendente: %v", disparados)
	}
}

func TestTiposAvaliacaoFinalDespertadosRespeitaPeriodoDaFormula(t *testing.T) {
	nota := "prova_trimestral"
	regras := []regraAvaliacaoFinalDTO{{Type: "normal", Formula: "([nota_professor,1_trimestre]+[prova_trimestral,1_trimestre]+[nota_professor,2_trimestre]+[prova_trimestral,2_trimestre]+[nota_professor,3_trimestre]+[prova_trimestral,3_trimestre])/6", NotaDespertadora: &nota}}

	if disparados := tiposAvaliacaoFinalDespertadosPorCategoria(regras, &regras[0], "prova_trimestral", "1_trimestre"); len(disparados) != 0 {
		t.Fatalf("gatilho não deve disparar no período errado: %#v", disparados)
	}
	if disparados := tiposAvaliacaoFinalDespertadosPorCategoria(regras, &regras[0], "prova_trimestral", "3_trimestre"); !disparados["normal"] {
		t.Fatalf("gatilho deve disparar no período de fechamento: %#v", disparados)
	}
}

func TestSubstituirNotasAusentesPorZeroContinuaAtivaAposFechamentoPorPeriodo(t *testing.T) {
	formula := "([nota_professor,1_trimestre]+[prova_trimestral,1_trimestre]+[nota_professor,2_trimestre]+[prova_trimestral,2_trimestre]+[nota_professor,3_trimestre]+[prova_trimestral,3_trimestre])/6"
	notas := map[string]map[string][]float64{
		"prova_trimestral": {
			"1_trimestre": {7},
			"2_trimestre": {8},
			"3_trimestre": {9},
		},
		// nota_professor nunca foi lançada em nenhum trimestre (cenário real: professor esqueceu).
	}

	faltantes, err := substituirNotasAusentesPorZero(formula, notas)
	if err != nil {
		t.Fatalf("substituirNotasAusentesPorZero retornou erro: %v", err)
	}
	if len(faltantes) != 3 {
		t.Fatalf("esperava 3 referências substituídas por zero (nota_professor x3), obteve: %#v", faltantes)
	}

	nota, err := calcularFormulaAvaliacao(formula, notas)
	if err != nil {
		t.Fatalf("cálculo da fórmula não deveria falhar mesmo com nota_professor ausente: %v", err)
	}
	esperado := (0.0 + 7 + 0.0 + 8 + 0.0 + 9) / 6
	if nota != esperado {
		t.Fatalf("nota final inesperada: got=%v want=%v", nota, esperado)
	}
}
