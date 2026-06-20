package handlers

import (
	"reflect"
	"testing"
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
