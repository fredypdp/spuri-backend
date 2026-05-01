package handlers

import "testing"

func TestResolverTipoMateriaMistoAceitaFundamentalEMedio(t *testing.T) {
	for _, tipo := range []string{"fundamental", "medio", "  MEDIO  "} {
		tipo := tipo
		t.Run(tipo, func(t *testing.T) {
			nivel := "misto"
			got, err := resolverTipoMateria("escola", &nivel, &tipo)
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if got != "fundamental" && got != "medio" {
				t.Fatalf("tipo retornado inválido: %s", got)
			}
		})
	}
}

func TestResolverTipoMateriaMistoRejeitaTipoInvalido(t *testing.T) {
	nivel := "misto"
	tipo := "superior"
	_, err := resolverTipoMateria("escola", &nivel, &tipo)
	if err == nil {
		t.Fatal("esperava erro para tipo inválido")
	}
}
