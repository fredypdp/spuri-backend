package aggregates

import (
	"strings"
	"testing"
)

func TestRegistrarAvaliacaoFinalBloqueiaDuplicidadeNoMesmoAnoLetivo(t *testing.T) {
	codigoAcademia := "ACA_01"
	proximoAno := "3_ano_fundamental"

	estudante := NewEstudante()
	estudante.CodigoEstudante = "EST1234"
	estudante.CodigoAcademia = &codigoAcademia

	if err := estudante.RegistrarAvaliacaoFinal(
		codigoAcademia,
		"2025_2026",
		"fundamental",
		"2_ano_fundamental",
		&proximoAno,
		nil,
		nil,
		true,
		nil,
		"normal",
		10,
		10,
		nil,
		"",
		nil,
	); err != nil {
		t.Fatalf("primeira avaliação final retornou erro: %v", err)
	}

	err := estudante.RegistrarAvaliacaoFinal(
		codigoAcademia,
		"2025_2026",
		"fundamental",
		"3_ano_fundamental",
		nil,
		nil,
		nil,
		false,
		nil,
		"normal",
		5,
		10,
		nil,
		"",
		nil,
	)
	if err == nil {
		t.Fatal("segunda avaliação final no mesmo ano letivo deveria ser rejeitada")
	}
	if !strings.Contains(err.Error(), "ano letivo") {
		t.Fatalf("erro deveria mencionar duplicidade no ano letivo, recebido: %v", err)
	}
}

func TestRegistrarAvaliacaoFinalBloqueiaDuplicidadeNoMesmoNivel(t *testing.T) {
	codigoAcademia := "ACA_01"
	proximoAno := "3_ano_fundamental"

	estudante := NewEstudante()
	estudante.CodigoEstudante = "EST1234"
	estudante.CodigoAcademia = &codigoAcademia

	if err := estudante.RegistrarAvaliacaoFinal(
		codigoAcademia,
		"2025_2026",
		"fundamental",
		"2_ano_fundamental",
		&proximoAno,
		nil,
		nil,
		true,
		nil,
		"normal",
		10,
		10,
		nil,
		"",
		nil,
	); err != nil {
		t.Fatalf("primeira avaliação final retornou erro: %v", err)
	}

	err := estudante.RegistrarAvaliacaoFinal(
		codigoAcademia,
		"2025_2026",
		"fundamental",
		"2_ano_fundamental",
		nil,
		nil,
		nil,
		false,
		nil,
		"normal",
		5,
		10,
		nil,
		"",
		nil,
	)
	if err == nil {
		t.Fatal("segunda avaliação final no mesmo nível deveria ser rejeitada")
	}
	if !strings.Contains(err.Error(), "nível") {
		t.Fatalf("erro deveria mencionar duplicidade no nível, recebido: %v", err)
	}
}
