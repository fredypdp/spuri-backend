package handlers

import (
	"strings"
	"testing"
)

func TestInferirAnoAcademicoParaNota_DeveBloquearQuandoAnoDoEstudanteNaoPertenceAMateria(t *testing.T) {
	anoEstudante := "2_ano_fundamental"
	_, err := inferirAnoAcademicoParaNota(&anoEstudante, []string{"1_ano_fundamental"}, "Matemática")
	if err == nil {
		t.Fatalf("esperava erro de incompatibilidade entre ano do estudante e da matéria")
	}
	if !strings.Contains(err.Error(), "não faz parte da matéria") {
		t.Fatalf("mensagem inesperada: %v", err)
	}
}

func TestInferirAnoAcademicoParaNota_DevePermitirQuandoAnoDoEstudantePertenceAMateria(t *testing.T) {
	anoEstudante := "2_ano_fundamental"
	ano, err := inferirAnoAcademicoParaNota(
		&anoEstudante,
		[]string{"1_ano_fundamental", "2_ano_fundamental"},
		"Matemática",
	)
	if err != nil {
		t.Fatalf("não esperava erro, mas recebeu: %v", err)
	}
	if ano != "2_ano_fundamental" {
		t.Fatalf("ano esperado '2_ano_fundamental', recebido '%s'", ano)
	}
}

func TestInferirAnoAcademicoParaNota_DeveInferirPeloAnoDaMateriaQuandoEstudanteNaoTemAnoEscolar(t *testing.T) {
	ano, err := inferirAnoAcademicoParaNota(nil, []string{"1_ano_medio"}, "Física")
	if err != nil {
		t.Fatalf("não esperava erro, mas recebeu: %v", err)
	}
	if ano != "1_ano_medio" {
		t.Fatalf("ano esperado '1_ano_medio', recebido '%s'", ano)
	}
}

func TestInferirAnoAcademicoParaNota_DeveBloquearMedioQuandoAnoDoEstudanteNaoPertenceAMateria(t *testing.T) {
	anoMedio := "2_ano_medio"
	_, err := inferirAnoAcademicoParaNota(nil, []string{"1_ano_medio"}, "Física", &anoMedio)
	if err == nil {
		t.Fatalf("esperava erro de incompatibilidade entre ano médio do estudante e da matéria")
	}
	if !strings.Contains(err.Error(), "ano acadêmico médio") || !strings.Contains(err.Error(), "não faz parte da matéria") {
		t.Fatalf("mensagem inesperada: %v", err)
	}
}

func TestInferirAnoAcademicoParaNota_DevePermitirMedioQuandoAnoDoEstudantePertenceAMateria(t *testing.T) {
	anoMedio := "2_ano_medio"
	ano, err := inferirAnoAcademicoParaNota(nil, []string{"1_ano_medio", "2_ano_medio"}, "Física", &anoMedio)
	if err != nil {
		t.Fatalf("não esperava erro, mas recebeu: %v", err)
	}
	if ano != "2_ano_medio" {
		t.Fatalf("ano esperado '2_ano_medio', recebido '%s'", ano)
	}
}

func TestInferirAnoAcademicoParaNota_DevePermitirMateriaMedioMultiAnoSemAnoEstudante(t *testing.T) {
	ano, err := inferirAnoAcademicoParaNota(nil, []string{"1_ano_medio", "2_ano_medio"}, "Física")
	if err != nil {
		t.Fatalf("não esperava erro para matéria média multi-ano: %v", err)
	}
	if ano != "1_ano_medio" {
		t.Fatalf("ano esperado '1_ano_medio', recebido '%s'", ano)
	}
}
