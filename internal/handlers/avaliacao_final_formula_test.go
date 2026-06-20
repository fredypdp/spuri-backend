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
