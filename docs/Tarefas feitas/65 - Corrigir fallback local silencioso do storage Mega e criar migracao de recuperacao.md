---
criado: 2026-08-25
origem: docs/Debbugs/Depurar arquivos de alvara nao chegando ao Mega real.md
status: concluido
concluido: 2026-08-26
tipo: correcao_bug_critico_mais_ferramenta_de_recuperacao
---

# Corrigir fallback local silencioso do storage Mega e criar migração de recuperação

## 0. Leia isto primeiro — sobre o seu ambiente (Codex)

Você não tem `apt`, Docker nem `psql` neste ambiente. Não precisa disso aqui. Claude já validou esta correção inteira com PostgreSQL 16 e Go 1.24 reais em sandbox — incluindo `go build`, `go vet`, `gofmt`, a suíte de testes completa (`go test ./...`, com e sem `RUN_POSTGRES_INTEGRATION=1`/`SPURI_RUN_DB_INTEGRITY_TESTS=1`, banco recriado do zero, 100% verde) — e, além disso, compilou o binário e rodou testes ao vivo end-to-end (registrar uma academia via `POST /academia/registo-publico` com upload de alvará; subir o servidor com as combinações exatas de variáveis que causavam os dois bugs) para confirmar tanto os bugs quanto as correções na prática, não só na teoria. Sua tarefa é mecânica: aplicar exatamente os blocos abaixo, rodar o checklist de validação (que não depende de PostgreSQL, Docker nem `psql`) e seguir o procedimento de conclusão.

Este documento tem duas partes:

- **Parte A** (seções 3 a 7): fecha a causa raiz do bug relatado — o storage caindo silenciosamente para o disco local em vez do Mega real.
- **Parte B** (seções 8 a 11): corrige um bug real e mais sério que a correção da Parte A expôs — vários handlers ignoravam silenciosamente o erro de `storage.NewStorageProvider()`, o que gerava `panic` (nil pointer) em vez de um erro HTTP limpo sempre que o Mega estivesse mal configurado ou indisponível. As duas partes precisam ser aplicadas juntas: aplicar só a Parte A sem a Parte B deixaria alguns fluxos de cadastro quebrando com erro 500/panic em vez de uma mensagem clara sempre que o Mega estiver fora do ar ou mal configurado.

---

## 1. Prompt recomendado para executar esta correção

> Execute exatamente as alterações descritas neste documento. Todas as decisões já foram tomadas e validadas (causa raiz confirmada com testes ao vivo do binário compilado, correção implementada e testada com PostgreSQL 16 e Go 1.24 reais, incluindo um segundo bug real exposto e corrigido). Sua tarefa é mecânica: aplique cada bloco das seções 3 a 11, na ordem, exatamente como descrito — a maioria é "localizar este bloco exato / substituir por", duas são "criar arquivo novo com este conteúdo exato", e uma (seção 9) precisa ser aplicada três vezes, em três ocorrências idênticas do mesmo arquivo (instrução detalhada na própria seção). Depois, rode cada item do "Checklist de validação" (seção 13) e reporte o resultado. Não toque em nenhum outro arquivo além dos listados. Não é necessário PostgreSQL, Docker nem `psql` — todos os itens do checklist usam apenas `go build`, `go vet`, `gofmt` e `go test ./...` (os testes de integração com Postgres pulam automaticamente sem `RUN_POSTGRES_INTEGRATION`/`SPURI_RUN_DB_INTEGRITY_TESTS`, isso é esperado e correto).

---

## 2. Contexto

Fredy fez alguns cadastros de academia pelo fluxo público/independente (`POST /academia/registo-publico`) e reparou que os arquivos de alvará não apareciam na conta Mega real, mas o sistema continuava retornando esses documentos normalmente para visualização/download.

### Parte A — causa raiz do bug relatado

Confirmado por leitura de código e por testes ao vivo do binário compilado (ver `docs/Debbugs/Depurar arquivos de alvara nao chegando ao Mega real.md` para a investigação completa e as provas ao vivo): `internal/storage/storage.go` tem um modo de "fallback local" (`MegaProvider{local: true}`) que faz upload/download/listagem trabalharem sobre um diretório no disco local em vez do Mega real — criado de propósito para os testes automatizados rodarem sem rede externa. A função que decidia ativar esse modo, `useLocalMegaFallback()`, tinha **duas** brechas que ativavam esse fallback por engano:

1. `ENV=test` ativava o fallback local mesmo com `STORAGE_PROVIDER=mega` definido explicitamente e credenciais Mega válidas configuradas.
2. `ENV=test` também ativava o fallback local quando `STORAGE_PROVIDER` estava completamente indefinido.

Como `ENV=test` é usado, para propósitos completamente diferentes, para colocar a AppyPay em modo sandbox (`internal/finance/appypay.go`) e para relaxar a exigência de `JWT_SECRET` em testes (`internal/middleware/auth.go`), qualquer ambiente com `ENV=test` — por qualquer motivo — desativava o Mega real para arquivos silenciosamente: sem erro, sem log, sem aviso. `ProviderName()` sempre retornava `"mega"` mesmo em modo local (inclusive no endpoint de diagnóstico `GET /dominis/storage/quota`), e não havia log de inicialização dizendo qual backend tinha sido escolhido.

**Correção**: a regra agora é única e definitiva — **`STORAGE_PROVIDER=local`, explícito, é o único jeito de ativar o fallback local**. Nenhum valor de `ENV`, em nenhuma combinação, ativa ou desativa esse comportamento. `ProviderName()` passa a retornar `"mega-local"` quando de fato está em modo local. `cmd/server/main.go` passa a logar, em toda inicialização, qual backend está ativo. Uma ferramenta de recuperação nova (`POST /dominis/storage/migrar-local-para-mega`, role `fpp`) reenvia para o Mega real qualquer arquivo que tenha ficado só no fallback local.

### Parte B — bug exposto pela correção da Parte A

Ao aplicar a regra "só `STORAGE_PROVIDER=local` explícito ativa o fallback local", um teste de integração (`cmd/server/turma_vinculo_estudante_integration_test.go`) passou a falhar com **panic de nil pointer** em vez do 201 esperado. Investigando, a causa é um bug real e pré-existente, apenas mascarado até agora: **7 pontos do código**, em 3 arquivos (`internal/handlers/estudante_handlers.go`, `internal/handlers/academia_handlers.go`, `internal/handlers/solicitacao_matricula_handlers.go`), chamam `storage.NewStorageProvider()` e **ignoram o erro retornado** (`p, _ := storage.NewStorageProvider()`), deixando o `provider` como `nil` e causando `panic` na primeira chamada de método sobre ele. Isso estava mascarado porque, antes da Parte A, `ENV=test` quase sempre fazia `NewStorageProvider()` "funcionar" (via fallback local), então esse caminho de erro nunca era exercitado nos testes. Sem essa correção, qualquer falha real do Mega em produção (credenciais erradas, API fora do ar, rede instável) faria esses mesmos endpoints — cadastro de academia, cadastro de estudante, criação de solicitação de matrícula, conclusão de documentos pendentes — quebrarem com `panic`/500 genérico em vez de um erro HTTP claro.

**Correção**: os 7 pontos passam a tratar o erro corretamente (mesmo padrão já usado em `internal/handlers/documento_download_handlers.go` e no `MigrarStorageLocalParaMega` novo): se `NewStorageProvider()` falhar, responde `503` com a mensagem de erro real, em vez de prosseguir com um provider nulo. O teste de integração que dependia do comportamento antigo (`ENV=test` "de graça" ativando storage local) passa a configurar `STORAGE_PROVIDER=local` explicitamente, do jeito que sempre deveria ter feito.

---

# Parte A — corrigir o fallback local silencioso

## 3. `internal/storage/storage.go` — localizar e substituir um bloco

### Localizar este bloco exato

```go
func useLocalMegaFallback() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("ENV")), "test") || strings.ToLower(strings.TrimSpace(os.Getenv("STORAGE_PROVIDER"))) == "local"
}

func (m *MegaProvider) ProviderName() string { return "mega" }
```

### Substituir por

```go
// useLocalMegaFallback decide se o provider deve operar sobre o filesystem
// local (fake) em vez do Mega remoto real.
//
// Regra (corrigida — ver docs/Debbugs/Depurar arquivos de alvara nao
// chegando ao Mega real.md): o único jeito de ativar o fallback local é
// STORAGE_PROVIDER=local, explícito. Nenhuma outra variável — em especial
// ENV — jamais ativa o fallback local, mesmo que ENV=test (usado, para um
// propósito completamente diferente, para selecionar o ambiente de sandbox
// da AppyPay em internal/finance/appypay.go). Antes desta correção,
// ENV=test sozinho já bastava para ativar o fallback local quando
// STORAGE_PROVIDER não estava definido — e, num bug ainda pior, mesmo
// quando STORAGE_PROVIDER=mega estava definido explicitamente com
// credenciais Mega válidas. As duas brechas foram fechadas: armazenamento
// de arquivos é sempre o Mega remoto real, a não ser que alguém peça
// explicitamente o contrário com STORAGE_PROVIDER=local.
func useLocalMegaFallback() bool {
	return strings.ToLower(strings.TrimSpace(os.Getenv("STORAGE_PROVIDER"))) == "local"
}

// ProviderName identifica o backend efetivamente ativo. Retorna "mega"
// apenas quando este provider está de fato falando com o Mega remoto real;
// no modo de fallback local (ver useLocalMegaFallback) retorna
// "mega-local", para que nada — nem o endpoint de diagnóstico
// GET /dominis/storage/quota, nem logs, nem qualquer outro consumidor —
// possa confundir os dois modos.
func (m *MegaProvider) ProviderName() string {
	if m.local {
		return "mega-local"
	}
	return "mega"
}
```

Nenhum import muda neste arquivo (`strings` e `os` já são usados em outras partes dele).

---

## 4. `internal/storage/storage_test.go` — substituir conteúdo inteiro

Apague todo o conteúdo atual do arquivo e substitua exatamente pelo conteúdo abaixo:

```go
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
```

---

## 5. `internal/storage/migration.go` — arquivo novo, conteúdo completo

```go
package storage

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
)

// MigratedFile descreve o resultado da tentativa de migrar um arquivo
// encontrado no diretório de fallback local (MEGA_LOCAL_ROOT) para o
// provider remoto ativo.
type MigratedFile struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
	Status    string `json:"status"` // "migrado", "ja_existia_no_destino" ou "falhou"
	Erro      string `json:"erro,omitempty"`
}

const (
	MigrationStatusMigrated = "migrado"
	MigrationStatusExists   = "ja_existia_no_destino"
	MigrationStatusFailed   = "falhou"
)

// MigrateLocalFallbackToProvider percorre localRoot (o diretório usado pelo
// provider de fallback local, tipicamente MEGA_LOCAL_ROOT) e reenvia cada
// arquivo regular encontrado para o provider informado, preservando o mesmo
// caminho relativo.
//
// É uma ferramenta de recuperação para os arquivos gravados enquanto
// STORAGE_PROVIDER resolvia, por engano, para o fallback local (ver
// docs/Debbugs/Depurar arquivos de alvara nao chegando ao Mega real.md).
// Não apaga nem modifica nada em localRoot — a limpeza dos arquivos locais,
// se desejada, é uma decisão separada e manual de quem operar o sistema,
// tomada depois de confirmar que os arquivos migrados estão corretos no
// destino.
//
// Um arquivo já existente no mesmo caminho no destino nunca é sobrescrito;
// é apenas reportado com status MigrationStatusExists. Por isso esta função
// é segura para rodar mais de uma vez (idempotente por caminho).
//
// provider deve ser o backend remoto real de destino (ProviderName() ==
// "mega"); o chamador é responsável por essa checagem antes de invocar esta
// função — ela não valida sozinha para permanecer utilizável em testes com
// dois providers locais distintos.
func MigrateLocalFallbackToProvider(provider StorageProvider, localRoot string) ([]MigratedFile, error) {
	results := []MigratedFile{}

	info, err := os.Stat(localRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return results, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return results, nil
	}

	err = filepath.WalkDir(localRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(localRoot, path)
		if relErr != nil {
			return relErr
		}
		remotePath := filepath.ToSlash(rel)
		results = append(results, migrateOneFile(provider, path, remotePath))
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(results, func(i, j int) bool { return results[i].Path < results[j].Path })
	return results, nil
}

func migrateOneFile(provider StorageProvider, localPath, remotePath string) MigratedFile {
	result := MigratedFile{Path: remotePath}

	fi, statErr := os.Stat(localPath)
	if statErr != nil {
		result.Status = MigrationStatusFailed
		result.Erro = statErr.Error()
		return result
	}
	result.SizeBytes = fi.Size()

	dir := filepath.ToSlash(filepath.Dir(remotePath))
	if dir == "." {
		dir = ""
	}
	name := filepath.Base(remotePath)

	existing, err := provider.List(dir)
	if err != nil && !errors.Is(err, ErrNotFound) {
		result.Status = MigrationStatusFailed
		result.Erro = err.Error()
		return result
	}
	for _, f := range existing {
		if filepath.Base(f.Path) == name {
			result.Status = MigrationStatusExists
			return result
		}
	}

	f, err := os.Open(localPath)
	if err != nil {
		result.Status = MigrationStatusFailed
		result.Erro = err.Error()
		return result
	}
	defer f.Close()

	if _, err := provider.Upload(remotePath, f, result.SizeBytes); err != nil {
		result.Status = MigrationStatusFailed
		result.Erro = err.Error()
		return result
	}

	result.Status = MigrationStatusMigrated
	return result
}
```

---

## 6. `cmd/server/main.go` — localizar e substituir dois blocos

Nenhum import novo é necessário (`log`, `os` e `strings` já são importados neste arquivo).

### 6.1 — Localizar este bloco exato

```go
func initStorage() error {
	provider, err := storage.NewStorageProvider()
	if err != nil {
		return err
	}
	storageProvider = provider
	return nil
}
```

### Substituir por

```go
func initStorage() error {
	provider, err := storage.NewStorageProvider()
	if err != nil {
		return err
	}
	storageProvider = provider
	logStorageProviderStartup(provider)
	return nil
}

// logStorageProviderStartup registra, de forma bem visível, qual backend de
// armazenamento de arquivos está realmente ativo nesta instância. Existe
// porque um provider em modo de fallback local historicamente não deixava
// nenhum rastro nos logs de inicialização — ver
// docs/Debbugs/Depurar arquivos de alvara nao chegando ao Mega real.md.
func logStorageProviderStartup(provider storage.StorageProvider) {
	name := provider.ProviderName()
	if name != "mega-local" {
		rootFolder := strings.Trim(strings.TrimSpace(os.Getenv("MEGA_ROOT_FOLDER")), "/")
		log.Printf("[INFO] armazenamento de arquivos: Mega remoto ativo (provider=%q, root_folder=%q)", name, rootFolder)
		return
	}

	localRoot := strings.TrimSpace(os.Getenv("MEGA_LOCAL_ROOT"))
	if localRoot == "" {
		localRoot = "data/mega_storage"
	}
	env := strings.ToLower(strings.TrimSpace(os.Getenv("ENV")))
	if env == "production" {
		log.Printf("[ALERTA] armazenamento de arquivos rodando em MODO LOCAL (provider=%q) com ENV=production — alvarás e outros documentos NÃO estão sendo enviados ao Mega real, ficam em %q e podem ser perdidos em redeploys. Verifique a variável STORAGE_PROVIDER (deve ser \"mega\", nunca \"local\", em produção).", name, localRoot)
		return
	}
	log.Printf("[INFO] armazenamento de arquivos: modo local (provider=%q), diretório=%q, ambiente=%q", name, localRoot, env)
}
```

### 6.2 — Localizar este bloco exato

```go
		admin.GET("/storage/quota", handlers.GetStorageQuota)
```

### Substituir por

```go
		admin.GET("/storage/quota", handlers.GetStorageQuota)
		admin.POST("/storage/migrar-local-para-mega", middleware.RequireFPP(), handlers.MigrarStorageLocalParaMega)
```

Esse bloco aparece uma única vez em `cmd/server/main.go`, dentro de `setupRouter()`, no grupo de rotas `admin` (`/dominis`).

---

## 7. `internal/handlers/storage_migration_handlers.go` — arquivo novo, conteúdo completo

```go
package handlers

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"

	"spuri/internal/storage"
	"spuri/internal/utils"
)

// MigrarStorageLocalParaMega é uma ferramenta administrativa de recuperação
// para o cenário descrito em
// docs/Debbugs/Depurar arquivos de alvara nao chegando ao Mega real.md:
// enquanto STORAGE_PROVIDER resolvia (por engano) para o fallback local,
// documentos como o alvará das academias foram gravados apenas em disco
// local, nunca no Mega real, embora continuassem visíveis/baixáveis pelo
// próprio sistema (upload e download usam o mesmo provider).
//
// Este endpoint reenvia para o Mega remoto real qualquer arquivo ainda
// presente em MEGA_LOCAL_ROOT, preservando o caminho relativo. É seguro
// rodar mais de uma vez: nunca sobrescreve um arquivo já existente no
// destino, e nunca apaga nem modifica os arquivos locais — a limpeza do
// diretório local, se desejada, é manual e feita depois de confirmar que os
// arquivos migrados estão corretos no Mega.
//
// Só executa quando o provider ativo é de fato o Mega remoto (não faz
// sentido "migrar" para um destino que também é local).
func MigrarStorageLocalParaMega(c *gin.Context) {
	provider := getStorageProvider(c)
	if provider == nil {
		var err error
		provider, err = storage.NewStorageProvider()
		if err != nil {
			utils.RespondWithError(c, http.StatusServiceUnavailable, err.Error(), err)
			return
		}
	}

	if provider.ProviderName() != "mega" {
		err := fmt.Errorf(
			"esta migração só roda quando o provider ativo é o Mega remoto real; provider ativo agora é %q (STORAGE_PROVIDER=mega, com MEGA_EMAIL/MEGA_PASSWORD configurados, é obrigatório antes de migrar)",
			provider.ProviderName(),
		)
		utils.RespondWithValidationError(c, err)
		return
	}

	localRoot := strings.TrimSpace(os.Getenv("MEGA_LOCAL_ROOT"))
	if localRoot == "" {
		localRoot = "data/mega_storage"
	}

	resultados, err := storage.MigrateLocalFallbackToProvider(provider, localRoot)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	migrados, existentes, falhas := 0, 0, 0
	for _, r := range resultados {
		switch r.Status {
		case storage.MigrationStatusMigrated:
			migrados++
		case storage.MigrationStatusExists:
			existentes++
		case storage.MigrationStatusFailed:
			falhas++
		}
	}

	status := http.StatusOK
	if falhas > 0 {
		status = http.StatusMultiStatus
	}

	c.JSON(status, gin.H{
		"local_root":        localRoot,
		"total_encontrados": len(resultados),
		"migrados":          migrados,
		"ja_existiam":       existentes,
		"falharam":          falhas,
		"arquivos":          resultados,
	})
}
```

`getStorageProvider` já existe em `internal/handlers/helpers.go` e é usado por outros handlers deste pacote (ex.: `GetStorageQuota`) — não precisa ser criado.

---

# Parte B — corrigir o erro de storage ignorado silenciosamente (7 ocorrências)

## 8. `internal/handlers/estudante_handlers.go` — localizar e substituir dois blocos

### 8.1 — Localizar este bloco exato

```go
	provider := getStorageProvider(c)
	if provider == nil {
		p, _ := storage.NewStorageProvider()
		provider = p
	}
	dir := fmt.Sprintf("%s/estudantes/%s/documentos", academia.CodigoAcademia, codigoEstudante)
```

### Substituir por

```go
	provider := getStorageProvider(c)
	if provider == nil {
		p, err := storage.NewStorageProvider()
		if err != nil {
			utils.RespondWithError(c, http.StatusServiceUnavailable, err.Error(), err)
			return
		}
		provider = p
	}
	dir := fmt.Sprintf("%s/estudantes/%s/documentos", academia.CodigoAcademia, codigoEstudante)
```

### 8.2 — Localizar este bloco exato

```go
	provider := getStorageProvider(c)
	if provider == nil {
		provider, _ = storage.NewStorageProvider()
	}
	dir := fmt.Sprintf("%s/estudantes/%s/documentos", academia.CodigoAcademia, codigo)
```

### Substituir por

```go
	provider := getStorageProvider(c)
	if provider == nil {
		var err error
		provider, err = storage.NewStorageProvider()
		if err != nil {
			utils.RespondWithError(c, http.StatusServiceUnavailable, err.Error(), err)
			return
		}
	}
	dir := fmt.Sprintf("%s/estudantes/%s/documentos", academia.CodigoAcademia, codigo)
```

`http`, `storage` e `utils` já são importados neste arquivo (usados por outras funções dele) — nenhum import novo é necessário.

---

## 9. `internal/handlers/academia_handlers.go` — localizar e substituir três ocorrências idênticas

Este bloco aparece **três vezes**, byte a byte idêntico, nas funções `RegisterAcademia`, `RegisterAcademiaPublica` e `DeletarAcademia`. Aplique a mesma substituição nas três ocorrências — não pule nenhuma, e não aplique em nenhum outro lugar do arquivo.

### Localizar este bloco exato (três ocorrências)

```go
	provider := getStorageProvider(c)
	if provider == nil {
		p, _ := storage.NewStorageProvider()
		provider = p
	}
```

### Substituir por (nas três ocorrências)

```go
	provider := getStorageProvider(c)
	if provider == nil {
		p, err := storage.NewStorageProvider()
		if err != nil {
			utils.RespondWithError(c, http.StatusServiceUnavailable, err.Error(), err)
			return
		}
		provider = p
	}
```

Depois de aplicar, confirme com `grep -c "p, _ := storage.NewStorageProvider()" internal/handlers/academia_handlers.go` — deve retornar `0`. Na terceira ocorrência (dentro de `DeletarAcademia`), o bloco seguinte já existente (`if provider == nil { utils.RespondWithInternalError(...); return }`) fica como está — é um segundo guard, agora redundante mas inofensivo; não precisa remover.

`http`, `storage` e `utils` já são importados neste arquivo — nenhum import novo é necessário.

---

## 10. `internal/handlers/solicitacao_matricula_handlers.go` — localizar e substituir um bloco

### Localizar este bloco exato

```go
	provider := getStorageProvider(c)
	if provider == nil {
		p, _ := storage.NewStorageProvider()
		provider = p
	}
	dir := fmt.Sprintf("%s/matriculas/matricula_%s", codigoAcademia, codigo)
```

### Substituir por

```go
	provider := getStorageProvider(c)
	if provider == nil {
		p, err := storage.NewStorageProvider()
		if err != nil {
			utils.RespondWithError(c, http.StatusServiceUnavailable, err.Error(), err)
			return
		}
		provider = p
	}
	dir := fmt.Sprintf("%s/matriculas/matricula_%s", codigoAcademia, codigo)
```

`http`, `storage` e `utils` já são importados neste arquivo — nenhum import novo é necessário.

---

## 11. `cmd/server/turma_vinculo_estudante_integration_test.go` — localizar e substituir um bloco

Este teste envia documentos (BI e cédula) no cadastro de estudante e por isso precisa mesmo de um storage local funcional — antes ele conseguia isso "de graça" através da brecha do `ENV=test` corrigida na Parte A; agora precisa pedir explicitamente.

### Localizar este bloco exato

```go
	t.Setenv("ENV", "test")
	prev, _ := os.Getwd()
```

### Substituir por

```go
	t.Setenv("ENV", "test")
	t.Setenv("STORAGE_PROVIDER", "local")
	t.Setenv("MEGA_LOCAL_ROOT", t.TempDir())
	prev, _ := os.Getwd()
```

---

# Documentação e configuração

## 12. `.env.example` — localizar e substituir um bloco

### Localizar este bloco exato

```
# =============================================================================
# Armazenamento de Arquivos (Mega)
# =============================================================================
STORAGE_PROVIDER=mega
MEGA_EMAIL=conta@exemplo.com
MEGA_PASSWORD=senha-mega
MEGA_ROOT_FOLDER=spuri

# Desenvolvimento/testes locais sem Mega real:
# STORAGE_PROVIDER=local
MEGA_LOCAL_ROOT=data/mega_storage
```

### Substituir por

```
# =============================================================================
# Armazenamento de Arquivos (Mega)
# =============================================================================
# IMPORTANTE: o storage de arquivos é sempre o Mega remoto real, com
# MEGA_EMAIL/MEGA_PASSWORD obrigatórios, EXCETO se STORAGE_PROVIDER=local
# for definido explicitamente (uso local/testes, ver abaixo). Nenhum outro
# valor de nenhuma outra variável (incluindo ENV) jamais ativa storage
# local — não deixe STORAGE_PROVIDER=local configurado em produção.
STORAGE_PROVIDER=mega
MEGA_EMAIL=conta@exemplo.com
MEGA_PASSWORD=senha-mega
MEGA_ROOT_FOLDER=spuri

# Desenvolvimento/testes locais sem Mega real:
# STORAGE_PROVIDER=local
MEGA_LOCAL_ROOT=data/mega_storage
```

---

## 13. `Documentação da API.md` — localizar e substituir quatro blocos

Este arquivo tem duas descrições distintas de `GET /dominis/storage/quota` (uma na seção 16, outra — mais antiga, redundante — na seção 20). Os blocos 13.1 e 13.3 tratam de cada uma; não são a mesma edição repetida.

Os blocos 13.1, 13.2 e 13.3 usam cerca de 4 crases (` ```` `) para o bloco "localizar/substituir" porque o conteúdo já contém, dentro dele, um bloco ` ```json ` — as 4 crases são só delimitação deste documento, não fazem parte do texto a inserir; copie apenas o conteúdo interno (que começa com 3 crases).

### 13.1 — Localizar este bloco exato (seção 16, `total_bytes: 53687091200`)

````
**Response 200:**

```json
{
  "provider": "mega",
  "total_bytes": 53687091200,
````

### Substituir por

````
**Response 200:**

`provider` identifica o backend efetivamente ativo nesta instância: `"mega"` quando é de fato o Mega remoto real, ou `"mega-local"` quando a instância está rodando em modo de fallback local (ver `STORAGE_PROVIDER` e `ENV` na seção "20. Armazenamento" mais abaixo) — nunca `"mega"` nesse segundo caso, para não mascarar o modo ativo.

```json
{
  "provider": "mega",
  "total_bytes": 53687091200,
````

### 13.2 — Localizar este bloco exato (logo depois da tabela de erros do mesmo endpoint, seção 16)

```
| `503` | provider de armazenamento indisponível ou falha ao obter quota |

## 17. Jobs Assíncronos
```

### Substituir por

````
| `503` | provider de armazenamento indisponível ou falha ao obter quota |

#### POST /dominis/storage/migrar-local-para-mega

Ferramenta administrativa de recuperação: reenvia para o Mega remoto real qualquer arquivo que tenha ficado apenas no fallback local (`MEGA_LOCAL_ROOT`), preservando o caminho relativo. Não apaga nem sobrescreve nada — um arquivo já existente no mesmo caminho no destino é apenas reportado, nunca substituído, então é seguro chamar mais de uma vez. Existe para o cenário descrito em `docs/Debbugs/Depurar arquivos de alvara nao chegando ao Mega real.md`.

**Proteção real**: autenticado + admin com role `fpp`.

**Pré-condição**: só executa quando o provider ativo é de fato o Mega remoto (`ProviderName() == "mega"`); se a instância estiver rodando em `mega-local`, retorna `400` explicando que não há para onde migrar.

**Request:** sem payload

**Response 200 ou 207** (207 quando `falharam > 0`):

```json
{
  "local_root": "data/mega_storage",
  "total_encontrados": 2,
  "migrados": 1,
  "ja_existiam": 1,
  "falharam": 0,
  "arquivos": [
    {
      "path": "ACA001/Documentação formal/alvara_ACA001.pdf",
      "size_bytes": 48213,
      "status": "migrado"
    },
    {
      "path": "ACA002/alvara_ACA002.pdf",
      "size_bytes": 51022,
      "status": "ja_existia_no_destino"
    }
  ]
}
```

`status` de cada arquivo é `"migrado"`, `"ja_existia_no_destino"` ou `"falhou"` (com campo `erro` adicional nesse último caso).

**Erros principais:**

| Status | Quando ocorre |
| --- | --- |
| `400` | provider ativo não é o Mega remoto real (nada a migrar) |
| `503` | provider de armazenamento indisponível |

## 17. Jobs Assíncronos
````

### 13.3 — Localizar este bloco exato (seção 20, legado, `total_bytes: 108003328`)

````
**Response 200:**

```json
{
  "provider": "mega",
  "total_bytes": 108003328,
````

### Substituir por

````
**Response 200:**

`provider` identifica o backend efetivamente ativo nesta instância: `"mega"` quando é de fato o Mega remoto real, ou `"mega-local"` quando a instância está rodando em modo de fallback local (ver `STORAGE_PROVIDER` e `ENV` logo acima) — nunca `"mega"` nesse segundo caso, para não mascarar o modo ativo.

```json
{
  "provider": "mega",
  "total_bytes": 108003328,
````

### 13.4 — Localizar este bloco exato (seção 20, bullet `ENV=test`)

```
- `STORAGE_PROVIDER=local`: seleciona o provider local compatível com a mesma interface, sem conexão externa.
- `MEGA_LOCAL_ROOT`: diretório local usado pelo provider local (padrão `data/mega_storage`).
- `ENV=test`: permite usar o provider local nos testes automatizados.
```

### Substituir por

```
- `STORAGE_PROVIDER=local`: seleciona o provider local compatível com a mesma interface, sem conexão externa.
- `MEGA_LOCAL_ROOT`: diretório local usado pelo provider local (padrão `data/mega_storage`).
- `ENV`: **não tem nenhum efeito** sobre qual storage é usado (mesmo `ENV=test`, usado para o sandbox da AppyPay). O único jeito de ativar o storage local é `STORAGE_PROVIDER=local`, explícito — sem ele, um deploy sem `MEGA_EMAIL`/`MEGA_PASSWORD` válidos falha alto na inicialização em vez de cair, em silêncio, para o storage local. Ver `docs/Debbugs/Depurar arquivos de alvara nao chegando ao Mega real.md` para o incidente que motivou essa regra.

A cada inicialização, o backend registra em log qual backend está de fato ativo (`[INFO] armazenamento de arquivos: Mega remoto ativo ...` ou, em modo local, `[INFO]`/`[ALERTA] armazenamento de arquivos: modo local ...`) — não há mais inicialização silenciosa em nenhum dos dois modos.

Caso arquivos tenham ficado gravados apenas no fallback local por engano (`STORAGE_PROVIDER` mal configurado em algum deploy anterior), `POST /dominis/storage/migrar-local-para-mega` (role `fpp`, ver seção 16) reenvia para o Mega real tudo que ainda estiver em `MEGA_LOCAL_ROOT`, sem apagar nem sobrescrever nada.
```

---

## 14. Fora de escopo (não altere)

- Qualquer outro método de `MegaProvider` além de `useLocalMegaFallback` e `ProviderName` (`Upload`, `Read`, `List`, `Delete`, `Move`, `Rename`, `EnsureDir`, `GetQuota`, `clean`, `lookupMegaNodeLocked`, `login`, etc.) — nenhum muda.
- As três ocorrências corrigidas em `academia_handlers.go` e as duas em `estudante_handlers.go` são as únicas nesse padrão nesses arquivos — não crie um guard genérico novo nem refatore para uma função compartilhada; a duplicação já existia antes desta correção e não é o escopo desta tarefa resolver.
- `internal/handlers/documento_download_handlers.go` e o novo `internal/handlers/storage_migration_handlers.go` — já tratam o erro corretamente, servem de referência, não precisam de nenhuma mudança.
- `internal/finance/appypay.go` e `internal/middleware/auth.go` — o uso de `ENV=test` para o sandbox da AppyPay e para relaxar `JWT_SECRET` em testes não muda; a correção só isola o storage desse acoplamento indevido, sem alterar o comportamento da AppyPay ou do JWT.
- A descrição legada "implementado por `internal/storage` via MEGAcmd" na seção 20 de `Documentação da API.md` (o provider real hoje usa a lib `go-mega`, não o binário MEGAcmd) — é uma inconsistência de documentação pré-existente e não relacionada a este bug; não faz parte desta tarefa.
- Não crie nenhuma opção para apagar automaticamente os arquivos locais depois de migrados — a limpeza é deliberadamente manual, decisão de quem operar o sistema depois de conferir o resultado.
- Nenhum outro teste além de `cmd/server/turma_vinculo_estudante_integration_test.go` precisa de `STORAGE_PROVIDER=local` adicionado — já foi conferido que nenhum outro teste do repositório depende do caminho antigo (`ENV=test` sozinho ativando o fallback local); não adicione essa variável em nenhum outro arquivo de teste por precaução.
- Qualquer arquivo do repositório `spuripainel` (frontend) — nenhuma alteração de frontend é necessária.

---

## 15. Checklist de validação (Codex deve executar e reportar o resultado de cada item)

Nenhum destes comandos requer PostgreSQL, Docker ou `psql`:

1. `grep -n "func useLocalMegaFallback" -A 5 internal/storage/storage.go` — deve mostrar só `strings.ToLower(strings.TrimSpace(os.Getenv("STORAGE_PROVIDER"))) == "local"`, sem nenhuma menção a `ENV` dentro da função.
2. `grep -rn ", _\s*:*=\s*storage\.NewStorageProvider()\|, _ = storage\.NewStorageProvider()" --include="*.go" . | grep -v _test.go` — deve retornar vazio (nenhum erro de `NewStorageProvider()` ignorado em código de produção).
3. `go build ./...` — sem erros.
4. `go vet ./...` — sem erros.
5. `gofmt -l internal/storage/storage.go internal/storage/storage_test.go internal/storage/migration.go cmd/server/main.go internal/handlers/storage_migration_handlers.go internal/handlers/estudante_handlers.go internal/handlers/academia_handlers.go internal/handlers/solicitacao_matricula_handlers.go cmd/server/turma_vinculo_estudante_integration_test.go` — vazio (nenhum arquivo listado).
6. `go test ./internal/storage/... -v` — todos os testes passam, incluindo `TestUseLocalMegaFallback` (6 subtestes), `TestNewStorageProviderIgnoresEnvTestWhenProviderExplicitlyMega`, `TestNewStorageProviderIgnoresEnvTestWhenProviderUnset`, `TestMigrateLocalFallbackToProviderUploadsAndSkipsExisting` e `TestMigrateLocalFallbackToProviderMissingLocalRootIsNoop`.
7. `go test ./...` — sem falhas (testes de integração com Postgres aparecem como `SKIP`, não `FAIL`, sem `RUN_POSTGRES_INTEGRATION`/`SPURI_RUN_DB_INTEGRITY_TESTS` — esperado).
8. `git diff --stat` — alterações apenas em `internal/storage/storage.go`, `internal/storage/storage_test.go`, `cmd/server/main.go`, `cmd/server/turma_vinculo_estudante_integration_test.go`, `internal/handlers/estudante_handlers.go`, `internal/handlers/academia_handlers.go`, `internal/handlers/solicitacao_matricula_handlers.go`, `.env.example`, `Documentação da API.md`, mais os dois arquivos novos (`internal/storage/migration.go`, `internal/handlers/storage_migration_handlers.go`) e os documentos de conclusão.

Se qualquer item falhar, não prossiga — reporte o erro exato.

---

## 16. Critérios de aceite

- [ ] `internal/storage/storage.go` com o bloco da seção 3 aplicado.
- [ ] `internal/storage/storage_test.go` substituído exatamente pelo conteúdo da seção 4.
- [ ] `internal/storage/migration.go` criado exatamente com o conteúdo da seção 5.
- [ ] `cmd/server/main.go` com os dois blocos da seção 6 aplicados.
- [ ] `internal/handlers/storage_migration_handlers.go` criado exatamente com o conteúdo da seção 7.
- [ ] `internal/handlers/estudante_handlers.go` com os dois blocos da seção 8 aplicados.
- [ ] `internal/handlers/academia_handlers.go` com o bloco da seção 9 aplicado nas três ocorrências.
- [ ] `internal/handlers/solicitacao_matricula_handlers.go` com o bloco da seção 10 aplicado.
- [ ] `cmd/server/turma_vinculo_estudante_integration_test.go` com o bloco da seção 11 aplicado.
- [ ] `.env.example` com o bloco da seção 12 aplicado.
- [ ] `Documentação da API.md` com os 4 blocos da seção 13 aplicados.
- [ ] Todos os 8 itens do checklist (seção 15) executados e reportados com sucesso.
- [ ] Nenhum arquivo fora do escopo desta tarefa foi alterado (seção 14).

---

## 17. Procedimento de conclusão

1. Mover este arquivo para `docs/Tarefas feitas/`, com `status: concluido` e `concluido: <data de hoje>` no frontmatter (mantendo o número 65 — era o próximo disponível no momento em que este documento foi escrito).
2. Atualizar `docs/Debbugs/Depurar arquivos de alvara nao chegando ao Mega real.md`, campo `status`, para confirmar o nome final do arquivo desta tarefa após movido (já está como `corrigido_via_65_...`; só confirmar que bate com o nome real).
3. Um commit único, mensagem: `storage: STORAGE_PROVIDER=local explicito e so ele ativa fallback local; corrige 7 pontos que ignoravam erro de NewStorageProvider; adiciona migracao de recuperacao`.
4. Reportar a Fredy: resultado de cada item do checklist e `git diff --stat` do commit. Nenhuma validação adicional com PostgreSQL real é necessária — já foi feita (ver `docs/Debbugs/Depurar arquivos de alvara nao chegando ao Mega real.md`, incluindo os testes ao vivo do binário compilado nas duas partes do bug).
5. Lembrar Fredy, no relatório final: se algum ambiente de deploy (Render) estiver com `STORAGE_PROVIDER` indefinido ou `STORAGE_PROVIDER=local`, depois deste deploy ele vai parar de subir (falha alta e explícita, por design, a não ser que `STORAGE_PROVIDER=local` tenha sido realmente a intenção) até `STORAGE_PROVIDER=mega` com `MEGA_EMAIL`/`MEGA_PASSWORD` reais serem configurados — o que é a correção funcionando, não uma regressão nova. Se esse ambiente tiver arquivos gravados no fallback local antes deste deploy, chamar `POST /dominis/storage/migrar-local-para-mega` assim que o Mega real estiver configurado e a instância subir, antes de qualquer novo redeploy dessa mesma instância.
6. Lembrar Fredy também: a partir de agora, se o Mega ficar fora do ar ou mal configurado em produção, cadastro de academia, cadastro de estudante, criação de solicitação de matrícula e conclusão de documentos pendentes vão responder `503` com uma mensagem clara, em vez de quebrar com erro 500 genérico — esse é o efeito pretendido da Parte B, não uma regressão.

**Nenhuma etapa desta tarefa altera fluxos de matrícula, notas, faltas, financeiro ou qualquer domínio fora do módulo de armazenamento de arquivos** — as alterações estão contidas a `internal/storage/`, a três handlers que resolvem o storage provider (`estudante_handlers.go`, `academia_handlers.go`, `solicitacao_matricula_handlers.go`), ao registro de uma rota e uma função de log em `cmd/server/main.go`, a um handler novo em `internal/handlers/`, a um teste de integração existente, e à documentação correspondente.
