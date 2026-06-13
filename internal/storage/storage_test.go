package storage

import (
	"os"
	"strings"
	"testing"
)

func TestGetQuotaRequiresExplicitLocalEstimate(t *testing.T) {
	t.Setenv("GOOGLE_DRIVE_QUOTA_LOCAL_ESTIMATE", "")

	provider := &DriveProvider{root: t.TempDir()}
	_, err := provider.GetQuota()
	if err == nil {
		t.Fatal("GetQuota() error = nil, want quota unavailable error")
	}
	if !strings.Contains(err.Error(), "quota do Google Drive indisponível") || !strings.Contains(err.Error(), "GOOGLE_DRIVE_QUOTA_LOCAL_ESTIMATE=true") {
		t.Fatalf("GetQuota() error = %q, want message explaining Google Drive quota configuration", err.Error())
	}
}

func TestGetQuotaLocalEstimateCountsOnlyLocalRoot(t *testing.T) {
	t.Setenv("GOOGLE_DRIVE_QUOTA_LOCAL_ESTIMATE", "true")

	root := t.TempDir()
	if err := os.MkdirAll(root+"/ACA001", 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(root+"/ACA001/video.bin", []byte("12345"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.WriteFile(root+"/avulso.pdf", []byte("123"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	provider := &DriveProvider{root: root}
	quota, err := provider.GetQuota()
	if err != nil {
		t.Fatalf("GetQuota() error = %v", err)
	}
	if quota.TotalBytes != 8 || quota.UsedBytes != 8 || quota.AvailableBytes != 0 {
		t.Fatalf("GetQuota() = %+v, want total=8 used=8 available=0", quota)
	}
	if quota.ManagedBytes != 5 || quota.OutsideAcademiasBytes != 3 || quota.UnmanagedBytes != 0 {
		t.Fatalf("GetQuota() = %+v, want managed=5 outside_academias=3 unmanaged=0", quota)
	}
	if len(quota.Academias) != 1 || quota.Academias[0].CodigoAcademia != "ACA001" || quota.Academias[0].UsedBytes != 5 {
		t.Fatalf("GetQuota().Academias = %+v, want ACA001 with 5 bytes", quota.Academias)
	}
}
