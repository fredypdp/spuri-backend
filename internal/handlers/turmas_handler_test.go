package handlers

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func strPtr(v string) *string { return &v }

func TestValidarCompatibilidadeTurmaComEstudantes_DeveFalharQuandoNivelIncompativel(t *testing.T) {
	estudantes := []estudanteCompatibilidadeDTO{
		{Codigo: "EST001", AnoEscolar: strPtr("2_ano_fundamental")},
	}

	err := validarCompatibilidadeTurmaComEstudantes(
		[]string{"1_ano_fundamental", "2_ano_fundamental", "3_ano_fundamental"},
		"3_ano_fundamental",
		nil,
		estudantes,
	)
	if err == nil {
		t.Fatal("esperava erro de incompatibilidade, mas retornou nil")
	}
	if !strings.Contains(err.Error(), "EST001") {
		t.Fatalf("esperava código do estudante no erro, obtido: %v", err)
	}
}

func TestValidarCompatibilidadeTurmaComEstudantes_DeveFalharQuandoCursoIncompativel(t *testing.T) {
	cursoTurma := uuid.New()
	outroCurso := uuid.New().String()
	estudantes := []estudanteCompatibilidadeDTO{
		{Codigo: "EST777", AnoEscolarMedio: strPtr("1_ano_medio"), CursoMedioID: &outroCurso},
	}

	err := validarCompatibilidadeTurmaComEstudantes(
		nil,
		"1_ano_medio",
		&cursoTurma,
		estudantes,
	)
	if err == nil {
		t.Fatal("esperava erro de curso incompatível, mas retornou nil")
	}
	if !strings.Contains(err.Error(), "EST777") {
		t.Fatalf("esperava código do estudante no erro, obtido: %v", err)
	}
}

func TestValidarCompatibilidadeTurmaComEstudantes_DevePermitirQuandoCompativel(t *testing.T) {
	cursoTurma := uuid.New()
	cursoTurmaStr := cursoTurma.String()
	estudantes := []estudanteCompatibilidadeDTO{
		{Codigo: "EST123", AnoEscolarMedio: strPtr("2_ano_medio"), CursoMedioID: &cursoTurmaStr},
	}

	err := validarCompatibilidadeTurmaComEstudantes(
		nil,
		"2_ano_medio",
		&cursoTurma,
		estudantes,
	)
	if err != nil {
		t.Fatalf("não esperava erro, mas obteve: %v", err)
	}
}
