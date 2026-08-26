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
