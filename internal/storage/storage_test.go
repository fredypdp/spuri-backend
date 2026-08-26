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

func TestNewStorageProviderIgnoresEnvTestWhenProviderExplicitlyMega(t *testing.T) {
	// Regressão do bug descrito em docs/Debbugs/Depurar arquivos de alvara
	// nao chegando ao Mega real.md: STORAGE_PROVIDER=mega explícito não
	// pode ser sobrescrito por ENV=test (usado também para o sandbox da
	// AppyPay). Mesmo com ENV=test, a ausência de credenciais Mega deve
	// continuar sendo um erro — nunca um fallback silencioso para local.
	t.Setenv("STORAGE_PROVIDER", "mega")
	t.Setenv("MEGA_EMAIL", "")
	t.Setenv("MEGA_PASSWORD", "")
	t.Setenv("ENV", "test")
	_, err := NewStorageProvider()
	if err == nil || !strings.Contains(err.Error(), "MEGA_EMAIL") || !strings.Contains(err.Error(), "MEGA_PASSWORD") {
		t.Fatalf("NewStorageProvider() error = %v, want missing Mega credentials mesmo com ENV=test", err)
	}
}

func TestNewStorageProviderIgnoresEnvTestWhenProviderUnset(t *testing.T) {
	// Segunda regressão do mesmo bug: mesmo com STORAGE_PROVIDER totalmente
	// indefinido (nem "mega" nem "local"), ENV=test sozinho não pode mais
	// ativar o fallback local — só STORAGE_PROVIDER=local, explícito, faz
	// isso. Sem isso, um deploy em produção que por engano deixasse
	// STORAGE_PROVIDER indefinido e ENV=test setado (por qualquer outro
	// motivo, ex. sandbox da AppyPay) ainda cairia silenciosamente para o
	// storage local.
	t.Setenv("STORAGE_PROVIDER", "")
	t.Setenv("MEGA_EMAIL", "")
	t.Setenv("MEGA_PASSWORD", "")
	t.Setenv("ENV", "test")
	_, err := NewStorageProvider()
	if err == nil || !strings.Contains(err.Error(), "MEGA_EMAIL") || !strings.Contains(err.Error(), "MEGA_PASSWORD") {
		t.Fatalf("NewStorageProvider() error = %v, want missing Mega credentials mesmo com STORAGE_PROVIDER indefinido e ENV=test", err)
	}
}

func TestUseLocalMegaFallback(t *testing.T) {
	// Regra única e definitiva: só STORAGE_PROVIDER=local ativa o fallback
	// local. ENV nunca é consultado — para nenhum valor, em nenhuma
	// combinação.
	cases := []struct {
		name            string
		storageProvider string
		env             string
		want            bool
	}{
		{"local explícito é sempre local, mesmo em produção", "local", "production", true},
		{"local explícito é local mesmo sem ENV", "local", "", true},
		{"mega explícito nunca é local, mesmo com ENV=test", "mega", "test", false},
		{"mega explícito em produção nunca é local", "mega", "production", false},
		{"provider não definido nunca é local, mesmo com ENV=test", "", "test", false},
		{"provider e ENV vazios não é local", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("STORAGE_PROVIDER", tc.storageProvider)
			t.Setenv("ENV", tc.env)
			if got := useLocalMegaFallback(); got != tc.want {
				t.Fatalf("useLocalMegaFallback() = %v, want %v (STORAGE_PROVIDER=%q ENV=%q)", got, tc.want, tc.storageProvider, tc.env)
			}
		})
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
	if provider.ProviderName() != "mega-local" {
		t.Fatalf("ProviderName() = %q, want mega-local (deve se identificar honestamente como fallback local, nunca como mega)", provider.ProviderName())
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

func TestMigrateLocalFallbackToProviderUploadsAndSkipsExisting(t *testing.T) {
	localRoot := t.TempDir()
	if err := os.MkdirAll(localRoot+"/ACA001/Documentação formal", 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(localRoot+"/ACA001/Documentação formal/alvara_ACA001.pdf", []byte("conteudo-alvara"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.MkdirAll(localRoot+"/ACA002", 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(localRoot+"/ACA002/alvara_ACA002.pdf", []byte("outro-alvara"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	dest := &MegaProvider{root: t.TempDir(), local: true}
	// Simula que ACA002/alvara_ACA002.pdf já tinha sido migrado numa
	// execução anterior, com um conteúdo diferente — não pode ser
	// sobrescrito.
	if _, err := dest.Upload("ACA002/alvara_ACA002.pdf", strings.NewReader("ja-estava-no-destino"), 21); err != nil {
		t.Fatalf("Upload() setup error = %v", err)
	}

	results, err := MigrateLocalFallbackToProvider(dest, localRoot)
	if err != nil {
		t.Fatalf("MigrateLocalFallbackToProvider() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("MigrateLocalFallbackToProvider() = %+v, want 2 resultados", results)
	}

	byPath := map[string]MigratedFile{}
	for _, r := range results {
		byPath[r.Path] = r
	}

	aca001, ok := byPath["ACA001/Documentação formal/alvara_ACA001.pdf"]
	if !ok || aca001.Status != MigrationStatusMigrated || aca001.Erro != "" {
		t.Fatalf("resultado ACA001 = %+v, want status=%q sem erro", aca001, MigrationStatusMigrated)
	}
	r, err := dest.Read("ACA001/Documentação formal/alvara_ACA001.pdf")
	if err != nil {
		t.Fatalf("Read() do arquivo migrado error = %v", err)
	}
	content, _ := io.ReadAll(r)
	r.Close()
	if string(content) != "conteudo-alvara" {
		t.Fatalf("conteúdo migrado = %q, want %q", content, "conteudo-alvara")
	}

	aca002, ok := byPath["ACA002/alvara_ACA002.pdf"]
	if !ok || aca002.Status != MigrationStatusExists {
		t.Fatalf("resultado ACA002 = %+v, want status=%q (não deve sobrescrever)", aca002, MigrationStatusExists)
	}
	r2, err := dest.Read("ACA002/alvara_ACA002.pdf")
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	content2, _ := io.ReadAll(r2)
	r2.Close()
	if string(content2) != "ja-estava-no-destino" {
		t.Fatalf("Delete/overwrite indevido: conteúdo no destino = %q, want preservado como %q", content2, "ja-estava-no-destino")
	}

	// Rodar de novo deve ser idempotente: agora os dois já existem no
	// destino, nada deve ser reenviado.
	results2, err := MigrateLocalFallbackToProvider(dest, localRoot)
	if err != nil {
		t.Fatalf("segunda chamada a MigrateLocalFallbackToProvider() error = %v", err)
	}
	for _, r := range results2 {
		if r.Status != MigrationStatusExists {
			t.Fatalf("segunda execução: resultado %+v, want status=%q para todos", r, MigrationStatusExists)
		}
	}
}

func TestMigrateLocalFallbackToProviderMissingLocalRootIsNoop(t *testing.T) {
	dest := &MegaProvider{root: t.TempDir(), local: true}
	results, err := MigrateLocalFallbackToProvider(dest, t.TempDir()+"/nao-existe")
	if err != nil {
		t.Fatalf("MigrateLocalFallbackToProvider() error = %v, want nil quando localRoot não existe", err)
	}
	if len(results) != 0 {
		t.Fatalf("MigrateLocalFallbackToProvider() = %+v, want vazio", results)
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
