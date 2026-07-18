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
