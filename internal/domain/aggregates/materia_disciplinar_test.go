package aggregates

import (
	"testing"

	"github.com/google/uuid"
)

func TestMateriaSuperiorCriadaComPendenciaPermitidaNasceAtiva(t *testing.T) {
	materia := NewMateriaDisciplinar()
	cursoID := uuid.New()

	if err := materia.Criar("Cálculo I", "superior", []string{"1_semestre"}, "ACA001", &cursoID, nil, nil, uuid.New()); err != nil {
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
	if err := materia.Criar("Matemática", "medio", []string{"10_classe"}, "ACA001", nil, nil, nil, uuid.New()); err != nil {
		t.Fatalf("erro inesperado ao criar matéria média: %v", err)
	}

	pendenciaPermitida := false
	if err := materia.AtualizarDados(nil, nil, nil, &pendenciaPermitida, nil, uuid.New()); err == nil {
		t.Fatal("esperava erro ao atualizar pendencia_permitida em matéria escolar")
	}
}

func TestMateriaSuperiorMantemPendenciaPermitidaFalseExplicita(t *testing.T) {
	materia := NewMateriaDisciplinar()
	cursoID := uuid.New()
	pendenciaPermitida := false

	if err := materia.Criar("Cálculo II", "superior", []string{"2_semestre"}, "ACA001", &cursoID, &pendenciaPermitida, nil, uuid.New()); err != nil {
		t.Fatalf("erro inesperado ao criar matéria superior: %v", err)
	}

	if materia.PendenciaPermitida {
		t.Fatal("esperava manter pendencia_permitida=false explícito para matéria superior")
	}
}

func TestMateriaEscolarNaoPermiteCriarPendenciaMesmoFalse(t *testing.T) {
	materia := NewMateriaDisciplinar()
	pendenciaPermitida := false

	if err := materia.Criar("Matemática", "medio", []string{"10_classe"}, "ACA001", nil, &pendenciaPermitida, nil, uuid.New()); err == nil {
		t.Fatal("esperava erro ao criar matéria escolar com pendencia_permitida explícita")
	}
}

func TestMateriaMedioPermiteMaisDeUmAnoAcademico(t *testing.T) {
	materia := NewMateriaDisciplinar()

	err := materia.Criar("Ciências", "medio", []string{"1_ano_medio", "2_ano_medio"}, "ACA001", nil, nil, nil, uuid.New())
	if err != nil {
		t.Fatalf("não esperava erro ao criar matéria média com múltiplos anos acadêmicos: %v", err)
	}

	if len(materia.AnosAcademicos) != 2 {
		t.Fatalf("esperava 2 anos acadêmicos, recebeu %d", len(materia.AnosAcademicos))
	}
}

func TestMateriaMedioBloqueiaQuartoAno(t *testing.T) {
	materia := NewMateriaDisciplinar()

	err := materia.Criar("PAP", "medio", []string{"4_ano_medio"}, "ACA001", nil, nil, nil, uuid.New())
	if err == nil {
		t.Fatal("esperava erro ao criar matéria média para o 4º ano")
	}
}
