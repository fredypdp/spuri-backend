package aggregates

import "testing"

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
	)
	if err == nil {
		t.Fatal("segunda avaliação final no mesmo ano letivo deveria ser rejeitada")
	}
}
