package aggregates

import (
	"testing"

	"github.com/google/uuid"
)

func TestEfetivarMatriculaMedioAtualizaStatusComoEfeitoDoEvento(t *testing.T) {
	estudante := NewEstudante()
	estudante.StatusEscolarFundamental = "finalizado"
	estudante.StatusEscolarMedio = "inativo"

	if err := estudante.EfetivarMatriculaMedio(uuid.New(), nil); err != nil {
		t.Fatalf("EfetivarMatriculaMedio retornou erro: %v", err)
	}

	if estudante.StatusEscolarMedio != "em_andamento" {
		t.Fatalf("StatusEscolarMedio = %q; esperado em_andamento", estudante.StatusEscolarMedio)
	}
	if got := estudante.UncommittedEvents[len(estudante.UncommittedEvents)-1].GetEventType(); got != "MatriculaMedioEfetivada" {
		t.Fatalf("evento gerado = %q; esperado MatriculaMedioEfetivada", got)
	}
}

func TestEfetivarMatriculaSuperiorExigeCiclosAnterioresFinalizadosOuInativos(t *testing.T) {
	estudante := NewEstudante()
	estudante.StatusEscolarFundamental = "em_andamento"
	estudante.StatusEscolarMedio = "finalizado"

	if err := estudante.EfetivarMatriculaSuperior(uuid.New(), nil); err == nil {
		t.Fatal("EfetivarMatriculaSuperior deveria rejeitar fundamental em andamento")
	}
}

func TestArquivarEReativarAtualizamStatusGeralPorAcontecimento(t *testing.T) {
	estudante := NewEstudante()
	estudante.Status = "ativo"

	if err := estudante.Arquivar(uuid.New(), nil); err != nil {
		t.Fatalf("Arquivar retornou erro: %v", err)
	}
	if estudante.Status != "inativo" {
		t.Fatalf("Status após arquivar = %q; esperado inativo", estudante.Status)
	}

	if err := estudante.Reativar(uuid.New(), nil); err != nil {
		t.Fatalf("Reativar retornou erro: %v", err)
	}
	if estudante.Status != "ativo" {
		t.Fatalf("Status após reativar = %q; esperado ativo", estudante.Status)
	}
}
