package aggregates

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func novaTurmaParaHistorico(t *testing.T) *Turma {
	t.Helper()
	turma := NewTurma()
	if err := turma.Criar("TURMA_HIST", "ACA_HIST", "6_ano_fundamental", nil, "manha", uuid.New()); err != nil {
		t.Fatal(err)
	}
	return turma
}

func TestTurmaAtualizarDadosPermiteIdentidadeSemHistorico(t *testing.T) {
	turma := novaTurmaParaHistorico(t)
	nivel := "7_ano_fundamental"
	if err := turma.AtualizarDados(&nivel, nil, nil, uuid.New()); err != nil {
		t.Fatalf("turma sem histórico deve aceitar correção de nível: %v", err)
	}
}

func TestTurmaAtualizarDadosBloqueiaIdentidadeComHistorico(t *testing.T) {
	turma := novaTurmaParaHistorico(t)
	if err := turma.AdicionarEstudanteNoAnoLectivo("EST-HIST", "2025_2026", uuid.New()); err != nil {
		t.Fatal(err)
	}

	// Antes da proteção da tarefa 31 este comando era aceite e permitia que a
	// projeção resolvesse meses antigos com a identidade atual da turma.
	nivel := "7_ano_fundamental"
	err := turma.AtualizarDados(&nivel, nil, nil, uuid.New())
	if err == nil || !strings.Contains(err.Error(), "histórico") {
		t.Fatalf("alteração de nível com histórico = %v, queria erro claro", err)
	}

	cursoID := uuid.New()
	err = turma.AtualizarDados(nil, &cursoID, nil, uuid.New())
	if err == nil || !strings.Contains(err.Error(), "histórico") {
		t.Fatalf("alteração de curso com histórico = %v, queria erro claro", err)
	}
}

func TestTurmaAtualizarDadosPermiteTurnoComHistorico(t *testing.T) {
	turma := novaTurmaParaHistorico(t)
	if err := turma.AdicionarEstudanteNoAnoLectivo("EST-HIST", "2025_2026", uuid.New()); err != nil {
		t.Fatal(err)
	}
	turno := "tarde"
	if err := turma.AtualizarDados(nil, nil, &turno, uuid.New()); err != nil {
		t.Fatalf("turno com histórico deve continuar editável: %v", err)
	}
}
