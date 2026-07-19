package storage

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func TestNewStorageProviderRequiresMegaCredentials(t *testing.T) {
	t.Setenv("STORAGE_PROVIDER", "mega")
	t.Setenv("MEGA_EMAIL", "")
	t.Setenv("MEGA_PASSWORD", "")
	t.Setenv("ENV", "")
	_, err := NewStorageProvider()
	if err == nil || !strings.Contains(err.Error(), "MEGA_EMAIL") || !strings.Contains(err.Error(), "MEGA_PASSWORD") {
		t.Fatalf("NewStorageProvider() error = %v, want missing Mega credentials", err)
	}
}

func TestIsTransientMegaErrorDetectsIncompleteJSONResponses(t *testing.T) {
	if !isTransientMegaError(errors.New("unexpected end of JSON input")) {
		t.Fatal("isTransientMegaError() = false, want true for incomplete Mega JSON response")
	}
	if isTransientMegaError(errors.New("falha de autenticação/permissão no Mega: login failed")) {
		t.Fatal("isTransientMegaError() = true, want false for authentication errors")
	}
}

func TestLocalProviderUploadReadListMoveRenameDelete(t *testing.T) {
	t.Setenv("STORAGE_PROVIDER", "local")
	t.Setenv("MEGA_LOCAL_ROOT", t.TempDir())
	provider, err := NewStorageProvider()
	if err != nil {
		t.Fatalf("NewStorageProvider() error = %v", err)
	}
	if provider.ProviderName() != "mega" {
		t.Fatalf("ProviderName() = %q, want mega", provider.ProviderName())
	}
	if err := provider.EnsureDir("ACA001/Documentação formal"); err != nil {
		t.Fatalf("EnsureDir() error = %v", err)
	}
	stored, err := provider.Upload("ACA001/Documentação formal/alvara.pdf", strings.NewReader("pdf"), 3)
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if stored.Path != "ACA001/Documentação formal/alvara.pdf" {
		t.Fatalf("Upload().Path = %q", stored.Path)
	}
	files, err := provider.List("ACA001/Documentação formal")
	if err != nil || len(files) != 1 {
		t.Fatalf("List() = %+v, %v; want one file", files, err)
	}
	r, err := provider.Read(stored.Path)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	b, _ := io.ReadAll(r)
	r.Close()
	if string(b) != "pdf" {
		t.Fatalf("Read() = %q", b)
	}
	if err := provider.Move(stored.Path, "ACA001/Estudantes/EST001/alvara.pdf"); err != nil {
		t.Fatalf("Move() error = %v", err)
	}
	renamed, err := provider.Rename("ACA001/Estudantes/EST001/alvara.pdf", "documento.pdf")
	if err != nil {
		t.Fatalf("Rename() error = %v", err)
	}
	if renamed.Path != "ACA001/Estudantes/EST001/documento.pdf" {
		t.Fatalf("Rename().Path = %q", renamed.Path)
	}
	if err := provider.Delete("ACA001"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if err := provider.Delete("ACA001"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete() error = %v, want ErrNotFound", err)
	}
}

func TestGetQuotaLocalEstimateCountsOnlyLocalRoot(t *testing.T) {
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
	provider := &MegaProvider{root: root, local: true}
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
	if len(quota.AccountFolders) != 1 || quota.AccountFolders[0].Path != "ACA001" || quota.AccountFolders[0].SizeBytes != 5 {
		t.Fatalf("GetQuota().AccountFolders = %+v, want ACA001 with 5 bytes", quota.AccountFolders)
	}
}

func TestAccountManagedUsageClassifiesAcademiaPaths(t *testing.T) {
	academias := map[string]uint64{}
	if outside := accountManagedUsage("ACA001/matriculas/doc.pdf", 10, academias); outside != 0 {
		t.Fatalf("accountManagedUsage() outside = %d, want 0", outside)
	}
	if academias["ACA001"] != 10 {
		t.Fatalf("accountManagedUsage() academias[ACA001] = %d, want 10", academias["ACA001"])
	}
	if outside := accountManagedUsage("avulso.pdf", 3, academias); outside != 3 {
		t.Fatalf("accountManagedUsage() outside = %d, want 3", outside)
	}
	if outside := accountManagedUsage("/sem-codigo.pdf", 5, academias); outside != 5 {
		t.Fatalf("accountManagedUsage() outside = %d, want 5", outside)
	}
}
