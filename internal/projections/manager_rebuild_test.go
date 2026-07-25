package projections

import (
	"strings"
	"testing"
)

func TestRebuildLockRejectsConcurrentRebuildAndReleasesAfterFailure(t *testing.T) {
	manager := NewManager(nil)

	if err := manager.beginRebuild("projection:academias"); err != nil {
		t.Fatalf("begin first rebuild: %v", err)
	}

	if err := manager.beginRebuild("projection:estudantes"); err == nil || !strings.Contains(err.Error(), "rebuild já em andamento: projection:academias") {
		t.Fatalf("second begin error = %v, want rebuild-in-progress error", err)
	}

	manager.endRebuild()

	if err := manager.beginRebuild("projection:estudantes"); err != nil {
		t.Fatalf("begin after release: %v", err)
	}
	manager.endRebuild()
}

func TestRebuildProjectionUnknownNameReturnsControlledErrorAndReleasesLock(t *testing.T) {
	manager := NewManager(nil)
	maliciousName := "academias; DROP TABLE spuri_ledger"

	err := manager.RebuildProjection(maliciousName)
	if err == nil || !strings.Contains(err.Error(), "projeção não encontrada: "+maliciousName) {
		t.Fatalf("RebuildProjection(%q) error = %v, want controlled not-found error", maliciousName, err)
	}

	if err := manager.beginRebuild("projection:academias"); err != nil {
		t.Fatalf("lock was not released after unknown projection error: %v", err)
	}
	manager.endRebuild()
}

func TestDefaultRebuildOrderCoversAllRegisteredProjections(t *testing.T) {
	registeredInServer := []string{
		"admins",
		"academias",
		"cursos",
		"materias",
		"categorias_nota",
		"estudantes",
		"turmas",
		"notas",
		"faltas",
		"avaliacao_final",
		"solicitacoes_matricula",
		"solicitacoes_edicao_dados_estudante",
	}

	seen := make(map[string]bool, len(defaultRebuildOrder))
	for _, name := range defaultRebuildOrder {
		if seen[name] {
			t.Fatalf("defaultRebuildOrder contains duplicate projection %q", name)
		}
		seen[name] = true
	}

	for _, name := range registeredInServer {
		if !seen[name] {
			t.Fatalf("registered projection %q is missing from defaultRebuildOrder", name)
		}
	}
}

func TestOrderedRebuildProjectionNamesUsesDependencyOrderBeforeFallback(t *testing.T) {
	projections := map[string]Projection{
		"z_custom":                            nil,
		"notas":                               nil,
		"solicitacoes_edicao_dados_estudante": nil,
		"academias":                           nil,
		"a_custom":                            nil,
		"solicitacoes_matricula":              nil,
	}

	got := orderedRebuildProjectionNames(projections)
	want := []string{
		"academias",
		"solicitacoes_matricula",
		"solicitacoes_edicao_dados_estudante",
		"notas",
		"a_custom",
		"z_custom",
	}

	if len(got) != len(want) {
		t.Fatalf("orderedRebuildProjectionNames len = %d (%v), want %d (%v)", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("orderedRebuildProjectionNames[%d] = %q (%v), want %q (%v)", i, got[i], got, want[i], want)
		}
	}
}
