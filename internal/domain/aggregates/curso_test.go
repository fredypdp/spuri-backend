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
		"",
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
	if err := curso.AtualizarDados(nil, novosAnos, &novosPeriodos, nil, nil, uuid.New()); err != nil {
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
		"",
		[]string{"1_ano_superior", "2_ano_superior"},
		[]string{"1_semestre", "2_semestre", "3_semestre", "4_semestre"},
		nil,
		"ACA_01",
	); err != nil {
		t.Fatalf("Criar() erro inesperado: %v", err)
	}

	novosAnos := []string{"1_ano_superior", "2_ano_superior", "3_ano_superior"}
	novosPeriodos := []string{"1_semestre", "2_semestre", "3_semestre", "4_semestre"}
	if err := curso.AtualizarDados(nil, novosAnos, &novosPeriodos, nil, nil, uuid.New()); err == nil {
		t.Fatalf("AtualizarDados() deve rejeitar anos superiores não derivados dos períodos")
	}
}

func TestCursoMedioMateriasChaveObrigatoriaPorAno(t *testing.T) {
	curso := NewCurso()
	materiaID := uuid.New()
	if err := curso.Criar(
		"Curso Médio",
		"medio",
		"liceu",
		[]string{"1_ano_medio", "2_ano_medio"},
		nil,
		[]MateriasChaveCursoAno{{AnoAcademico: "1_ano_medio", MateriasChave: []uuid.UUID{materiaID}}},
		"ACA_01",
	); err == nil {
		t.Fatalf("esperava rejeição quando algum ano_academico não tem materias_chave")
	}
}

func TestCursoMedioMateriasChaveValidaAnoEDuplicados(t *testing.T) {
	curso := NewCurso()
	materiaID := uuid.New()
	if err := curso.Criar(
		"Curso Médio",
		"medio",
		"liceu",
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
		"liceu",
		[]string{"1_ano_medio"},
		nil,
		[]MateriasChaveCursoAno{{AnoAcademico: "1_ano_medio", MateriasChave: []uuid.UUID{materiaID, materiaID}}},
		"ACA_01",
	); err == nil {
		t.Fatalf("esperava rejeição de matéria duplicada no mesmo ano")
	}
}

func TestCursoMedioPodeSerCriadoSemMateriasChave(t *testing.T) {
	curso := NewCurso()
	err := curso.Criar("Ciências", "medio", "liceu", nil, nil, nil, "ACA001")
	if err != nil {
		t.Fatalf("curso médio sem materias_chave deveria ser criado: %v", err)
	}
	if len(curso.MateriasChave) != 0 {
		t.Fatalf("materias_chave = %v, want empty", curso.MateriasChave)
	}
}

func TestCursoMedioModeloObrigatorioEValido(t *testing.T) {
	for _, modelo := range []string{"liceu", "tecnico"} {
		curso := NewCurso()
		if err := curso.Criar("Curso Médio", "medio", modelo, nil, nil, nil, "ACA_01"); err != nil {
			t.Fatalf("modelo %s deveria ser aceito: %v", modelo, err)
		}
		if curso.Modelo != modelo {
			t.Fatalf("Modelo = %q, want %q", curso.Modelo, modelo)
		}
		esperado, _ := DerivarAnosAcademicosCursoMedio(modelo)
		if !reflect.DeepEqual(curso.AnosAcademicos, esperado) {
			t.Fatalf("modelo %s derivou anos %v, want %v", modelo, curso.AnosAcademicos, esperado)
		}
	}

	for _, modelo := range []string{"", "profissional", "LICEU"} {
		curso := NewCurso()
		if err := curso.Criar("Curso Médio", "medio", modelo, nil, nil, nil, "ACA_01"); err == nil {
			t.Fatalf("modelo %q deveria ser rejeitado", modelo)
		}
	}
}

func TestCursoSuperiorRejeitaModelo(t *testing.T) {
	curso := NewCurso()
	if err := curso.Criar("Engenharia", "superior", "tecnico", []string{"1_ano_superior"}, []string{"1_semestre"}, nil, "ACA_01"); err == nil {
		t.Fatalf("curso superior com modelo deveria ser rejeitado")
	}
}

func TestCursoMedioRejeitaAtualizacaoDeModelo(t *testing.T) {
	curso := NewCurso()
	if err := curso.Criar("Curso Médio", "medio", "liceu", nil, nil, nil, "ACA_01"); err != nil {
		t.Fatalf("Criar() erro inesperado: %v", err)
	}
	modelo := "tecnico"
	if err := curso.AtualizarDados(nil, nil, nil, nil, &modelo, uuid.New()); err == nil {
		t.Fatalf("AtualizarDados() deveria rejeitar troca de modelo de curso médio")
	}
}

func TestCursoMedioRejeitaAnosAcademicosManuaisNoAggregate(t *testing.T) {
	curso := NewCurso()
	if err := curso.Criar("Curso Médio", "medio", "tecnico", []string{"1_ano_medio", "2_ano_medio", "3_ano_medio", "4_ano_medio"}, nil, nil, "ACA_01"); err == nil {
		t.Fatalf("Criar() deveria rejeitar anos_academicos manuais em curso médio")
	}
}
