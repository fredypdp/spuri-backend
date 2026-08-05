package handlers

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

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

func TestDecodeStrictJSONRejeitaCamposDesconhecidos(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	c.Request = &http.Request{
		Body: ioNopCloser{Reader: strings.NewReader(`{"nome":"Matemática","pendenciaPermitida":true}`)},
	}
	var req struct {
		Nome string `json:"nome"`
	}

	if err := decodeStrictJSON(c, &req); err == nil {
		t.Fatal("esperava erro para campo desconhecido/alias fora do contrato")
	}
}

type ioNopCloser struct {
	*strings.Reader
}

func (ioNopCloser) Close() error { return nil }

func TestValidarAnosAcademicosMateriaMedioPermiteMultiplosAnos(t *testing.T) {
	err := validarAnosAcademicosMateria("medio", []string{"1_ano_medio", "2_ano_medio", "3_ano_medio"})
	if err != nil {
		t.Fatalf("não esperava erro para matéria média multi-ano: %v", err)
	}
}

func TestValidarAnosAcademicosMateriaMedioBloqueiaQuartoAno(t *testing.T) {
	err := validarAnosAcademicosMateria("medio", []string{"3_ano_medio", "4_ano_medio"})
	if err == nil {
		t.Fatal("esperava erro para matéria média com 4_ano_medio")
	}
}
