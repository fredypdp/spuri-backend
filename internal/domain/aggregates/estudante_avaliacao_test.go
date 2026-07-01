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
		nil,
		nil,
		nil,
		nil,
		false,
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
		nil,
		nil,
		nil,
		nil,
		false,
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
		nil,
		nil,
		nil,
		nil,
		false,
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
		nil,
		nil,
		nil,
		nil,
		false,
		nil,
	)
	if err == nil {
		t.Fatal("segunda avaliação final no mesmo nível deveria ser rejeitada")
	}
	if !strings.Contains(err.Error(), "nível") {
		t.Fatalf("erro deveria mencionar duplicidade no nível, recebido: %v", err)
	}
}

func TestAvaliacaoFinalFundamentalComProximoAnoMantemStatusEmAndamento(t *testing.T) {
	codigoAcademia := "ACA_01"
	proximoAno := "6_ano_fundamental"

	estudante := NewEstudante()
	estudante.CodigoEstudante = "EST1234"
	estudante.CodigoAcademia = &codigoAcademia
	estudante.StatusEscolarFundamental = "finalizado"

	if err := estudante.RegistrarAvaliacaoFinal(
		codigoAcademia,
		"2025_2026",
		"fundamental",
		"5_ano_fundamental",
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
		nil,
		nil,
		nil,
		nil,
		false,
		nil,
	); err != nil {
		t.Fatalf("avaliação final retornou erro: %v", err)
	}

	if estudante.AnoEscolar == nil || *estudante.AnoEscolar != proximoAno {
		t.Fatalf("AnoEscolar = %v, want %s", estudante.AnoEscolar, proximoAno)
	}
	if estudante.StatusEscolarFundamental != "em_andamento" {
		t.Fatalf("StatusEscolarFundamental = %q, want em_andamento", estudante.StatusEscolarFundamental)
	}
}
