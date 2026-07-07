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

func TestPrepararDadosCursoMedioRejeitaAnosAcademicosManuais(t *testing.T) {
	_, _, err := prepararDadosCursoPorTipo("medio", cursoPayload{
		ModeloInformado: true,
		Modelo:          "tecnico",
		AnosInformado:   true,
		AnosAcademicos:  []string{"1_ano_medio", "2_ano_medio", "3_ano_medio", "4_ano_medio"},
	}, true)
	if err == nil {
		t.Fatalf("esperava erro ao enviar anos_academicos em curso médio")
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
		{"1_ano_medio", "1_ano_medio"},
		{},
	} {
		if err := validarSequenciaAnosMedio(anos); err == nil {
			t.Fatalf("esperava erro para sequência %v", anos)
		}
	}
}

func TestRejeitarCamposAcademicosEmAtualizacaoCursoRejeitaMateriasChave(t *testing.T) {
	casos := []string{"materias_chave", "materiasChave", "MateriasChave"}
	for _, campo := range casos {
		t.Run(campo, func(t *testing.T) {
			req := httptest.NewRequest("PUT", "/academia/curso/id/dados", bytes.NewBufferString(`{"`+campo+`":[]}`))
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			if err := rejeitarCamposAcademicosEmAtualizacaoCurso(c); err == nil {
				t.Fatalf("esperava rejeição para %s na edição cadastral do curso", campo)
			}
		})
	}
}

func TestPrepararDadosCursoMedioExigeModelo(t *testing.T) {
	_, _, err := prepararDadosCursoPorTipo("medio", cursoPayload{
		AnosInformado:  true,
		AnosAcademicos: []string{"1_ano_medio"},
	}, true)
	if err == nil {
		t.Fatalf("esperava erro ao criar curso médio sem modelo")
	}
}

func TestPrepararDadosCursoMedioDerivaAnosPorModelo(t *testing.T) {
	casos := map[string][]string{
		"liceu":   {"1_ano_medio", "2_ano_medio", "3_ano_medio"},
		"tecnico": {"1_ano_medio", "2_ano_medio", "3_ano_medio", "4_ano_medio"},
	}
	for modelo, esperado := range casos {
		anos, periodos, err := prepararDadosCursoPorTipo("medio", cursoPayload{
			ModeloInformado: true,
			Modelo:          modelo,
		}, true)
		if err != nil {
			t.Fatalf("modelo %s deveria ser aceito: %v", modelo, err)
		}
		if !reflect.DeepEqual(anos, esperado) {
			t.Fatalf("modelo %s derivou anos %v, want %v", modelo, anos, esperado)
		}
		if periodos != nil {
			t.Fatalf("curso médio não deve derivar periodos, recebeu %v", periodos)
		}
	}
}

func TestPrepararDadosCursoMedioRejeitaModeloInvalido(t *testing.T) {
	_, _, err := prepararDadosCursoPorTipo("medio", cursoPayload{
		ModeloInformado: true,
		Modelo:          "LICEU",
	}, true)
	if err == nil {
		t.Fatalf("esperava erro ao enviar modelo inválido")
	}
}

func TestPrepararDadosCursoSuperiorRejeitaModelo(t *testing.T) {
	_, _, err := prepararDadosCursoPorTipo("superior", cursoPayload{
		ModeloInformado:    true,
		Modelo:             "tecnico",
		PeriodosInformado:  true,
		PeriodosQuantidade: 2,
	}, true)
	if err == nil {
		t.Fatalf("esperava erro ao enviar modelo em curso superior")
	}
}
