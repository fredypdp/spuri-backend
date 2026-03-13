package handlers

import (
	"fmt"
	"log"
	"net/http"
	"spuri/internal/middleware"
	"spuri/internal/utils"

	"github.com/gin-gonic/gin"
)

// ============================================================================
// POST /admin/projections/rebuild/:name
// ============================================================================
func RebuildProjection(c *gin.Context) {
	adminID, _ := middleware.GetUserID(c)
	name := c.Param("name")
	if name == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("nome da projeção é obrigatório"))
		return
	}

	// FIX RB-01: usar o Manager — garante markRebuildStart/Complete/Failed
	// e atualização correta de is_rebuilding e checkpoint.
	manager := getProjManager(c)
	if manager == nil {
		utils.RespondWithInternalError(c, fmt.Errorf("projection manager não disponível"))
		return
	}

	if err := manager.RebuildProjection(name); err != nil {
		log.Printf("❌ [RebuildProjection] Falha ao reconstruir '%s' por admin %s: %v", name, adminID, err)

		// FIX RB-02: registrar falha no ledger do admin executor.
		registrarAcaoAdmin(c, adminID, "rebuild_projection", map[string]interface{}{
			"projection": name,
			"resultado":  "falha",
			"erro":       err.Error(),
		})

		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("✅ [RebuildProjection] Projeção '%s' reconstruída com sucesso por admin %s", name, adminID)

	// FIX RB-02: registrar sucesso no ledger do admin executor — rastreabilidade
	// de quem disparou o rebuild, quando e com qual resultado.
	registrarAcaoAdmin(c, adminID, "rebuild_projection", map[string]interface{}{
		"projection": name,
		"resultado":  "sucesso",
	})

	c.JSON(http.StatusOK, gin.H{
		"message":    "projeção reconstruída com sucesso",
		"projection": name,
	})
}