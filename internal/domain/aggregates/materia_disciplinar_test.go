package aggregates

import (
	"testing"

	"github.com/google/uuid"
)

func TestMateriaSuperiorCriadaComPendenciaPermitidaNasceAtiva(t *testing.T) {
	materia := NewMateriaDisciplinar()
	cursoID := uuid.New()

	if err := materia.Criar("Cálculo I", "superior", []string{"1_semestre"}, "ACA001", &cursoID, true, nil, uuid.New()); err != nil {
		t.Fatalf("erro inesperado ao criar matéria superior: %v", err)
	}

	if !materia.PendenciaPermitida {
		t.Fatal("esperava pendencia_permitida=true para matéria superior")
	}
	if materia.Status != "ativo" {
		t.Fatalf("esperava status ativo, obtido %q", materia.Status)
	}
}

func TestMateriaEscolarNaoPermiteAtualizarPendenciaMesmoFalse(t *testing.T) {
	materia := NewMateriaDisciplinar()
	if err := materia.Criar("Matemática", "medio", []string{"10_classe"}, "ACA001", nil, false, nil, uuid.New()); err != nil {
		t.Fatalf("erro inesperado ao criar matéria média: %v", err)
	}

	pendenciaPermitida := false
	if err := materia.AtualizarDados(nil, nil, nil, &pendenciaPermitida, nil, uuid.New()); err == nil {
		t.Fatal("esperava erro ao atualizar pendencia_permitida em matéria escolar")
	}
}
