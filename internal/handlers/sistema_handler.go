package handlers

import (
	"fmt"
	"log"
	"net/http"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/projections"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// UUID fixo e determinístico para o singleton de configuração do sistema.
var sistemaConfigID = uuid.NewSHA1(uuid.NameSpaceDNS, []byte("sistema_config.spuri.ao"))

// DefinirAnoLetivo define o ano letivo atual do sistema.
// Rota: POST /admin/definir-ano-letivo  (apenas FPP)
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

	if err := repository.Save(config); err != nil {
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
	projManager := getProjManager(c)

	proj, err := projManager.GetProjection("sistema_config")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "projeção não disponível"})
		return
	}

	configProj, ok := proj.(*projections.SistemaConfigProjection)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tipo de projeção inválido"})
		return
	}

	valor, err := configProj.GetValor("ano_letivo_atual")
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("ano letivo não definido: %s", err.Error())})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ano_letivo": valor})
}