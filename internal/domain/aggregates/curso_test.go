package aggregates

import (
	"reflect"
	"testing"

	"github.com/google/uuid"
)

func TestCursoSuperiorAtualizarPeriodosRecalculaAnosAntesDeValidar(t *testing.T) {
	curso := NewCurso()
	err := curso.Criar(
		"Engenharia Informática",
		"superior",
		[]string{"1_ano_superior", "2_ano_superior", "3_ano_superior"},
		[]string{"1_semestre", "2_semestre", "3_semestre", "4_semestre", "5_semestre", "6_semestre"},
		nil,
		"ACA_01",
	)
	if err != nil {
		t.Fatalf("Criar() erro inesperado: %v", err)
	}

	novosAnos := []string{"1_ano_superior", "2_ano_superior", "3_ano_superior", "4_ano_superior"}
	novosPeriodos := []string{"1_semestre", "2_semestre", "3_semestre", "4_semestre", "5_semestre", "6_semestre", "7_semestre", "8_semestre"}
	if err := curso.AtualizarDados(nil, novosAnos, &novosPeriodos, nil, uuid.New()); err != nil {
		t.Fatalf("AtualizarDados() deve aceitar anos derivados dos novos períodos, erro: %v", err)
	}

	if !reflect.DeepEqual(curso.AnosAcademicos, novosAnos) {
		t.Fatalf("AnosAcademicos = %v, want %v", curso.AnosAcademicos, novosAnos)
	}
	if !reflect.DeepEqual(curso.Periodos, novosPeriodos) {
		t.Fatalf("Periodos = %v, want %v", curso.Periodos, novosPeriodos)
	}
}

func TestCursoSuperiorAtualizarPeriodosRejeitaAnosNaoDerivados(t *testing.T) {
	curso := NewCurso()
	if err := curso.Criar(
		"Engenharia Informática",
		"superior",
		[]string{"1_ano_superior", "2_ano_superior"},
		[]string{"1_semestre", "2_semestre", "3_semestre", "4_semestre"},
		nil,
		"ACA_01",
	); err != nil {
		t.Fatalf("Criar() erro inesperado: %v", err)
	}

	novosAnos := []string{"1_ano_superior", "2_ano_superior", "3_ano_superior"}
	novosPeriodos := []string{"1_semestre", "2_semestre", "3_semestre", "4_semestre"}
	if err := curso.AtualizarDados(nil, novosAnos, &novosPeriodos, nil, uuid.New()); err == nil {
		t.Fatalf("AtualizarDados() deve rejeitar anos superiores não derivados dos períodos")
	}
}

func TestCursoMedioMateriasChaveValidaAnoEDuplicados(t *testing.T) {
	curso := NewCurso()
	materiaID := uuid.New()
	if err := curso.Criar(
		"Curso Médio",
		"medio",
		[]string{"1_ano_medio"},
		nil,
		[]MateriasChaveCursoAno{{AnoAcademico: "2_ano_medio", MateriasChave: []uuid.UUID{materiaID}}},
		"ACA_01",
	); err == nil {
		t.Fatalf("esperava rejeição de ano fora de anos_academicos")
	}

	if err := curso.Criar(
		"Curso Médio",
		"medio",
		[]string{"1_ano_medio"},
		nil,
		[]MateriasChaveCursoAno{{AnoAcademico: "1_ano_medio", MateriasChave: []uuid.UUID{materiaID, materiaID}}},
		"ACA_01",
	); err == nil {
		t.Fatalf("esperava rejeição de matéria duplicada no mesmo ano")
	}
}
