package storage

import (
	"os"
	"strings"
	"testing"
)

func TestGetQuotaRequiresExplicitLocalEstimate(t *testing.T) {
	t.Setenv("MEGA_SESSION_ID", "")
	t.Setenv("MEGA_QUOTA_LOCAL_ESTIMATE", "")

	provider := &MegaProvider{root: t.TempDir()}
	_, err := provider.GetQuota()
	if err == nil {
		t.Fatal("GetQuota() error = nil, want quota unavailable error")
	}
	if !strings.Contains(err.Error(), "quota do Mega indisponível") || !strings.Contains(err.Error(), "MEGA_QUOTA_LOCAL_ESTIMATE=true") {
		t.Fatalf("GetQuota() error = %q, want message explaining Mega quota configuration", err.Error())
	}
}

func TestGetQuotaLocalEstimateCountsOnlyLocalRoot(t *testing.T) {
	t.Setenv("MEGA_SESSION_ID", "")
	t.Setenv("MEGA_QUOTA_LOCAL_ESTIMATE", "true")
	t.Setenv("MEGA_QUOTA_TOTAL_BYTES", "100")
	t.Setenv("MEGA_QUOTA_TOTAL_GB", "")

	root := t.TempDir()
	if err := os.MkdirAll(root+"/ACA001", 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(root+"/ACA001/video.bin", []byte("12345"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	provider := &MegaProvider{root: root}
	quota, err := provider.GetQuota()
	if err != nil {
		t.Fatalf("GetQuota() error = %v", err)
	}
	if quota.TotalBytes != 100 || quota.UsedBytes != 5 || quota.AvailableBytes != 95 {
		t.Fatalf("GetQuota() = %+v, want total=100 used=5 available=95", quota)
	}
	if len(quota.Academias) != 1 || quota.Academias[0].CodigoAcademia != "ACA001" || quota.Academias[0].UsedBytes != 5 {
		t.Fatalf("GetQuota().Academias = %+v, want ACA001 with 5 bytes", quota.Academias)
	}
}

func TestConfiguredQuotaTotalBytesUsesGBOverride(t *testing.T) {
	t.Setenv("MEGA_QUOTA_TOTAL_BYTES", "")
	t.Setenv("MEGA_QUOTA_TOTAL_GB", "50")

	total, err := configuredQuotaTotalBytes()
	if err != nil {
		t.Fatalf("configuredQuotaTotalBytes() error = %v", err)
	}

	const want uint64 = 50 * 1024 * 1024 * 1024
	if total != want {
		t.Fatalf("configuredQuotaTotalBytes() = %d, want %d", total, want)
	}
}

func TestMegaAccountUsageSeparatesRootFilesFromManagedFolders(t *testing.T) {
	nodes := map[string]megaNode{
		"root":  {Hash: "root", Type: 2},
		"aca":   {Hash: "aca", Parent: "root", Type: 1},
		"file":  {Hash: "file", Parent: "aca", Type: 0, Size: 10},
		"loose": {Hash: "loose", Parent: "root", Type: 0, Size: 20},
	}
	names := map[string]string{
		"aca":   "ACA001",
		"file":  "video.mp4",
		"loose": "avulso.mp4",
	}

	if got := topLevelMegaFolder(nodes["file"], nodes, names); got != "ACA001" {
		t.Fatalf("topLevelMegaFolder(managed) = %q, want ACA001", got)
	}
	if got := topLevelMegaFolder(nodes["loose"], nodes, names); got != "" {
		t.Fatalf("topLevelMegaFolder(root file) = %q, want empty", got)
	}
	if got := megaNodePath(nodes["file"], nodes, names); got != "ACA001/video.mp4" {
		t.Fatalf("megaNodePath(managed) = %q, want ACA001/video.mp4", got)
	}
	if got := megaNodePath(nodes["loose"], nodes, names); got != "avulso.mp4" {
		t.Fatalf("megaNodePath(root file) = %q, want avulso.mp4", got)
	}
}
