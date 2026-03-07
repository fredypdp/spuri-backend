package handlers

import (
	"fmt"
	"log"
	"net/http"
	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/projections"
	"spuri/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// UUID fixo e determinístico para o singleton de configuração do sistema.
var sistemaConfigID = uuid.NewSHA1(uuid.NameSpaceDNS, []byte("spuripainel.vercel.app"))

// DefinirAnoLetivo define o ano letivo atual do sistema.
// Rota: POST /admin/ano-letivo  (apenas FPP)
func DefinirAnoLetivo(c *gin.Context) {
	adminID, _ := middleware.GetUserID(c)

	var req struct {
		AnoLetivo string `json:"ano_letivo" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "campo 'ano_letivo' é obrigatório"})
		return
	}

	repository := getRepository(c)

	// Tenta carregar o agregado existente; se não existe, cria novo com ID fixo.
	var config *aggregates.SistemaConfig
	agg, err := repository.Load(sistemaConfigID, "SistemaConfig")
	if err != nil {
		// Primeira definição
		config = aggregates.NewSistemaConfigComID(sistemaConfigID)
	} else {
		config = agg.(*aggregates.SistemaConfig)
	}

	if err := config.DefinirAnoLetivo(req.AnoLetivo, adminID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// FIX: usar SaveWithAudit em vez de Save — garante user_id, user_type e IP
	// no metadata de cada linha do ledger, consistente com todas as outras rotas admin.
	audit := db.AuditContext{
		UserID:   adminID.String(),
		UserType: "admin",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(config, audit); err != nil {
		log.Printf("❌ [DefinirAnoLetivo] Erro ao salvar: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao definir ano letivo"})
		return
	}

	log.Printf("✅ [DefinirAnoLetivo] %s definido por admin %s", req.AnoLetivo, adminID)

	c.JSON(http.StatusOK, gin.H{
		"message":    "ano letivo definido com sucesso",
		"ano_letivo": req.AnoLetivo,
	})
}

// GetAnoLetivoAtual retorna o ano letivo atual configurado.
// Rota: GET /ano-letivo-atual  (autenticado)
func GetAnoLetivoAtual(c *gin.Context) {
	configProj := projections.NewSistemaConfigProjection(getDbClient(c))

	valor, err := configProj.GetValor("ano_letivo_atual")
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("ano letivo não definido: %s", err.Error())})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ano_letivo": valor})
}

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