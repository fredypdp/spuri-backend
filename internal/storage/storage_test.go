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

func TestNewStorageProviderMegaDoesNotSilentlyUseLocalRoot(t *testing.T) {
	t.Setenv("STORAGE_PROVIDER", "mega")
	t.Setenv("MEGA_EMAIL", "conta@example.com")
	t.Setenv("MEGA_PASSWORD", "segredo")
	t.Setenv("MEGA_LOCAL_ROOT", t.TempDir())
	t.Setenv("ENV", "")

	_, err := NewStorageProvider()
	if err == nil || !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("NewStorageProvider() error = %v, want invalid configuration without MEGAcmd fallback", err)
	}
	if !strings.Contains(err.Error(), "MEGAcmd") {
		t.Fatalf("NewStorageProvider() error = %v, want MEGAcmd validation", err)
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
}

func writeMegaCmdStubs(t *testing.T, bin string, scripts map[string]string) {
	t.Helper()
	for _, name := range megaCmds {
		script := scripts[name]
		if script == "" {
			script = "#!/bin/sh\nexit 0\n"
		}
		if err := os.WriteFile(bin+"/"+name, []byte(script), 0o700); err != nil {
			t.Fatalf("WriteFile %s error = %v", name, err)
		}
	}
}

func TestMegaProviderLogsOutBeforeLoginToAvoidStaleSession(t *testing.T) {
	bin := t.TempDir()
	logFile := bin + "/calls.log"
	writeMegaCmdStubs(t, bin, map[string]string{
		"mega-login": "#!/bin/sh\necho login:$1:$2 >> \"$MEGA_TEST_LOG\"\nexit 0\n",
	})
	if err := os.WriteFile(bin+"/mega-logout", []byte("#!/bin/sh\necho logout >> \"$MEGA_TEST_LOG\"\nexit 0\n"), 0o700); err != nil {
		t.Fatalf("WriteFile mega-logout error = %v", err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("MEGA_TEST_LOG", logFile)
	t.Setenv("STORAGE_PROVIDER", "mega")
	t.Setenv("MEGA_EMAIL", "conta@example.com")
	t.Setenv("MEGA_PASSWORD", "senha-nova")
	t.Setenv("MEGA_ROOT_FOLDER", "")
	t.Setenv("ENV", "")

	provider, err := NewStorageProvider()
	if err != nil {
		t.Fatalf("NewStorageProvider() error = %v", err)
	}
	if err := provider.EnsureDir("documentos"); err != nil {
		t.Fatalf("EnsureDir() error = %v", err)
	}
	calls, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("ReadFile calls log error = %v", err)
	}
	if string(calls) != "logout\nlogin:conta@example.com:senha-nova\n" {
		t.Fatalf("calls = %q, want logout before login with configured credentials", calls)
	}
}

func TestMegaProviderRejectsInvalidPasswordAfterLogout(t *testing.T) {
	bin := t.TempDir()
	writeMegaCmdStubs(t, bin, map[string]string{
		"mega-login": "#!/bin/sh\necho login failed for $1\nexit 1\n",
	})
	if err := os.WriteFile(bin+"/mega-logout", []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatalf("WriteFile mega-logout error = %v", err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("STORAGE_PROVIDER", "mega")
	t.Setenv("MEGA_EMAIL", "conta@example.com")
	t.Setenv("MEGA_PASSWORD", "senha-antiga")
	t.Setenv("MEGA_ROOT_FOLDER", "")
	t.Setenv("ENV", "")

	provider, err := NewStorageProvider()
	if err != nil {
		t.Fatalf("NewStorageProvider() error = %v", err)
	}
	err = provider.EnsureDir("documentos")
	if err == nil || !strings.Contains(err.Error(), "falha ao autenticar no Mega") {
		t.Fatalf("EnsureDir() error = %v, want Mega authentication failure", err)
	}
}
