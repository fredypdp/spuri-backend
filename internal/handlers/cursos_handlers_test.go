package handlers

import (
	"bytes"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestValoresRemovidos(t *testing.T) {
	got := valoresRemovidos(
		[]string{"1_ano_medio", "2_ano_medio", "3_ano_medio"},
		[]string{"1_ano_medio", "3_ano_medio", "4_ano_medio"},
	)
	want := []string{"2_ano_medio"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("valores removidos = %v, want %v", got, want)
	}
}

func TestSemestresDosPeriodos(t *testing.T) {
	got := semestresDosPeriodos([]string{"1_semestre", "2_semestre", "periodo_invalido", "4_semestre"})
	want := []int{1, 2, 4}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("semestres = %v, want %v", got, want)
	}
}

func TestDerivarCursoSuperior(t *testing.T) {
	anos, periodos, err := derivarCursoSuperior(3)
	if err != nil {
		t.Fatalf("derivarCursoSuperior retornou erro: %v", err)
	}
	wantAnos := []string{"1_ano_superior", "2_ano_superior"}
	wantPeriodos := []string{"1_semestre", "2_semestre", "3_semestre"}
	if !reflect.DeepEqual(anos, wantAnos) {
		t.Fatalf("anos = %v, want %v", anos, wantAnos)
	}
	if !reflect.DeepEqual(periodos, wantPeriodos) {
		t.Fatalf("periodos = %v, want %v", periodos, wantPeriodos)
	}
}

func TestPrepararDadosCursoSuperiorRejeitaAnosAcademicos(t *testing.T) {
	_, _, err := prepararDadosCursoPorTipo("superior", cursoPayload{
		AnosInformado:      true,
		AnosAcademicos:     []string{"1_ano_superior"},
		PeriodosInformado:  true,
		PeriodosQuantidade: 2,
	}, true)
	if err == nil {
		t.Fatalf("esperava erro ao enviar anos_academicos em curso superior")
	}
}

func TestPrepararDadosCursoMedioRejeitaPeriodosNumerico(t *testing.T) {
	_, _, err := prepararDadosCursoPorTipo("medio", cursoPayload{
		AnosInformado:      true,
		AnosAcademicos:     []string{"1_ano_medio"},
		PeriodosInformado:  true,
		PeriodosQuantidade: 2,
	}, true)
	if err == nil {
		t.Fatalf("esperava erro ao enviar periodos numérico em curso médio")
	}
}

func TestPrepararDadosCursoMedioRejeitaAnosForaDeSequencia(t *testing.T) {
	for _, anos := range [][]string{
		{"2_ano_medio", "3_ano_medio"},
		{"1_ano_medio", "3_ano_medio"},
		{"2_ano_medio", "1_ano_medio"},
	} {
		_, _, err := prepararDadosCursoPorTipo("medio", cursoPayload{
			AnosInformado:  true,
			AnosAcademicos: anos,
		}, true)
		if err == nil {
			t.Fatalf("esperava erro para anos_academicos %v", anos)
		}
	}
}

func TestRejeitarCamposAcademicosEmAtualizacaoCurso(t *testing.T) {
	for _, payload := range []string{
		`{"anos_academicos":["1_ano_medio"]}`,
		`{"anosAcademicos":["1_ano_medio"]}`,
		`{"periodos":8}`,
		`{"semestres":["1_semestre"]}`,
		`{"quantidade_semestres":8}`,
		`{"anos":["1_ano_medio"]}`,
	} {
		req := httptest.NewRequest("PUT", "/academia/curso/id/dados", bytes.NewBufferString(payload))
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		if err := rejeitarCamposAcademicosEmAtualizacaoCurso(c); err == nil {
			t.Fatalf("esperava rejeição para payload %s", payload)
		}
	}
}

func TestValidarSequenciaAnosMedioExigePrefixoContinuo(t *testing.T) {
	if err := validarSequenciaAnosMedio([]string{"1_ano_medio", "2_ano_medio", "3_ano_medio"}); err != nil {
		t.Fatalf("sequência válida retornou erro: %v", err)
	}
	for _, anos := range [][]string{
		{"2_ano_medio", "3_ano_medio"},
		{"1_ano_medio", "3_ano_medio"},
		{"2_ano_medio", "1_ano_medio"},
		{},
	} {
		if err := validarSequenciaAnosMedio(anos); err == nil {
			t.Fatalf("esperava erro para sequência %v", anos)
		}
	}
}
