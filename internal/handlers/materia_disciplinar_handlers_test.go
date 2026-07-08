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

func TestValidarPendenciaNivelConclusaoRejeitaMateriasEscolares(t *testing.T) {
	nivel := "1_semestre"
	for _, tipo := range []string{"fundamental", "medio"} {
		tipo := tipo
		t.Run(tipo, func(t *testing.T) {
			if err := validarPendenciaNivelConclusao(tipo, &nivel, nil, nil); err == nil {
				t.Fatal("esperava erro para pendencia_nivel_conclusao em matéria escolar")
			}
		})
	}
}

func TestValidarPendenciaNivelConclusaoSuperior(t *testing.T) {
	nivel := "2_semestre"
	if err := validarPendenciaNivelConclusao("superior", &nivel, nil, []string{"1_semestre", "2_semestre"}); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	invalido := "10_classe"
	if err := validarPendenciaNivelConclusao("superior", &invalido, nil, []string{"1_semestre"}); err == nil {
		t.Fatal("esperava erro para nível de conclusão inválido")
	}
}
