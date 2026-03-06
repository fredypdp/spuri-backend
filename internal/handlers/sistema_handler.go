package handlers

import (
	"fmt"
	"log"
	"net/http"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/projections"
	"spuri/internal/utils"

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

// RebuildProjection força o rebuild de uma projeção específica pelo nome.
// Requer role admin. Útil para recovery após falha de processamento.
func RebuildProjection(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("nome da projeção é obrigatório"))
		return
	}

	// Cada projeção tem o seu próprio Rebuild() — instanciar pelo nome.
	type rebuilder interface {
		Rebuild() error
	}

	dbClient := getDbClient(c)
	if dbClient == nil {
		return // getDbClient já abortou com 500
	}

	var proj rebuilder
	switch name {
	case "admins":
		proj = projections.NewAdminProjection(dbClient)
	case "academias":
		proj = projections.NewAcademiaProjection(dbClient)
	case "estudantes":
		proj = projections.NewEstudanteProjection(dbClient)
	case "cursos":
		proj = projections.NewCursosProjection(dbClient)
	case "materias":
		proj = projections.NewMateriasProjection(dbClient)
	case "notas":
		proj = projections.NewNotasProjection(dbClient)
	case "faltas":
		proj = projections.NewFaltasProjection(dbClient)
	case "turmas":
		proj = projections.NewTurmasProjection(dbClient)
	case "avaliacao_final":
		proj = projections.NewAvaliacaoFinalProjection(dbClient)
	case "aprovacoes":
		proj = projections.NewAprovacaoAnoProjection(dbClient)
	case "reprovacoes":
		proj = projections.NewReprovacoesProjection(dbClient)
	case "categorias_nota":
		proj = projections.NewCategoriasNotaProjection(dbClient)
	case "sistema_config":
		proj = projections.NewSistemaConfigProjection(dbClient)
	default:
		utils.RespondWithValidationError(c, fmt.Errorf("projeção '%s' desconhecida", name))
		return
	}

	if err := proj.Rebuild(); err != nil {
		log.Printf("❌ [RebuildProjection] Falha ao reconstruir '%s': %v", name, err)
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("✅ [RebuildProjection] Projeção '%s' reconstruída com sucesso", name)
	c.JSON(http.StatusOK, gin.H{
		"message":    "projeção reconstruída com sucesso",
		"projection": name,
	})
}

// ============================================================================
// GET /health
// ============================================================================

// HealthCheck retorna o estado de saúde da aplicação.
// Rota pública — sem autenticação.
func HealthCheck(c *gin.Context) {
	dbClient := getDbClient(c)
	if dbClient == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "unhealthy",
			"error":  "database unavailable",
		})
		return
	}

	if err := dbClient.Health(); err != nil {
		log.Printf("[WARN] [HealthCheck] DB health check falhou: %v", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "unhealthy",
			"error":  "database check failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "healthy",
	})
}

// ============================================================================
// GET /docs
// ============================================================================

// GetDocs retorna informação básica sobre a API.
// Rota pública — sem autenticação.
func GetDocs(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"api":     "Spuri Backend API",
		"version": "1.0.0",
		"docs":    "Consulte a documentação interna para detalhes dos endpoints.",
	})
}