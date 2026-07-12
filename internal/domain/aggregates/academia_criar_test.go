package aggregates

import "testing"

func TestAcademiaCriarExigeNivelEscolarParaEscola(t *testing.T) {
	a := NewAcademia()

	err := a.Criar(
		"escola",
		"public",
		"Academia Teste",
		"1234567890",
		"LDA260001",
		"hash",
		"LDA",
		"Rua de Teste 123",
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	if err == nil {
		t.Fatal("Criar() deveria rejeitar escola sem nivel_escolar")
	}
}

func TestAcademiaCriarNormalizaNivelEscolar(t *testing.T) {
	a := NewAcademia()
	nivelEscolar := " Fundamental "

	err := a.Criar(
		"escola",
		"public",
		"Academia Teste",
		"1234567890",
		"LDA260001",
		"hash",
		"LDA",
		"Rua de Teste 123",
		nil,
		nil,
		nil,
		&nivelEscolar,
		nil,
		[]string{"1_ano_fundamental"},
		nil,
	)
	if err != nil {
		t.Fatalf("Criar() erro inesperado: %v", err)
	}
	if a.NivelEscolar == nil || *a.NivelEscolar != "fundamental" {
		t.Fatalf("NivelEscolar = %v, queria fundamental", a.NivelEscolar)
	}
}
