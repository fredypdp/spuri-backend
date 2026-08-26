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
