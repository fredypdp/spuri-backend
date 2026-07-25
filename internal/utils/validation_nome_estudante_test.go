package utils

import "testing"

func TestValidateNomeEstudanteAceitaLetrasAcentosEspacosEApostrofos(t *testing.T) {
	validos := []string{
		"Ana Maria",
		"João Luís",
		"Ângela D'Ávila",
		"Màrta Noël",
		"Conceição d’Água",
		"Zo\u0065\u0308 Silva",
	}

	for _, nome := range validos {
		t.Run(nome, func(t *testing.T) {
			if err := ValidateNomeEstudante(nome); err != nil {
				t.Fatalf("ValidateNomeEstudante(%q) retornou erro: %v", nome, err)
			}
		})
	}
}

func TestValidateNomeEstudanteRejeitaNumerosSinaisEspeciaisEPontuacao(t *testing.T) {
	invalidos := []string{
		"Ana 2",
		"Ana@Maria",
		"Ana#Maria",
		"Ana$Maria",
		"Ana?Maria",
		"Ana!Maria",
		"Ana_Maria",
		"Ana-Maria",
		"Ana, Maria",
		"Ana. Maria",
	}

	for _, nome := range invalidos {
		t.Run(nome, func(t *testing.T) {
			if err := ValidateNomeEstudante(nome); err == nil {
				t.Fatalf("ValidateNomeEstudante(%q) deveria rejeitar o nome", nome)
			}
		})
	}
}
