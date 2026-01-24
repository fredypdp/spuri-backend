// ============================================================================
// ARQUIVO: internal/handlers/admin_management_handlers.go
// Handlers para gerenciamento de admins e operações adicionais + logs debug
// ============================================================================

package handlers

import (
	"log"
	"net/http"
	"spuri/internal/domain/aggregates"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ============================================================================
// GERENCIAMENTO DE ADMINS
// ============================================================================

// ListarTodosAdmins lista todos os administradores (apenas ADM+)
func ListarTodosAdmins(c *gin.Context) {
	log.Printf("[DEBUG] ListarTodosAdmins: Início")
	
	adminProj := getAdminProjection(c)
	
	admins, err := adminProj.GetAll()
	if err != nil {
		log.Printf("[ERROR] ListarTodosAdmins: Erro ao buscar admins: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "erro ao buscar administradores",
		})
		return
	}

	log.Printf("[DEBUG] ListarTodosAdmins: %d administradores encontrados", len(admins))

	c.JSON(http.StatusOK, gin.H{
		"admins": admins,
		"total":  len(admins),
	})
}

// AtivarAdmin ativa um administrador (apenas FPP)
func AtivarAdmin(c *gin.Context) {
	userID, _ := c.Get("user_id")
	targetID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		log.Printf("[ERROR] AtivarAdmin: ID inválido: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID inválido"})
		return
	}

	log.Printf("[DEBUG] AtivarAdmin: UserID: %s, TargetID: %s", userID, targetID)

	// Verificar se é FPP
	if err := verificarPermissaoAdmin(c, "fpp"); err != nil {
		log.Printf("[ERROR] AtivarAdmin: Permissão negada: %v", err)
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[DEBUG] AtivarAdmin: Permissão FPP verificada")

	// Carregar admin alvo
	repository := getRepository(c)
	targetAdminAgg, err := repository.Load(targetID, "Admin")
	if err != nil {
		log.Printf("[ERROR] AtivarAdmin: Erro ao carregar agregado: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "administrador não encontrado"})
		return
	}

	log.Printf("[DEBUG] AtivarAdmin: Agregado carregado")

	targetAdmin := targetAdminAgg.(*aggregates.Admin)
	if err := targetAdmin.Ativar(userID.(uuid.UUID)); err != nil {
		log.Printf("[ERROR] AtivarAdmin: Erro ao ativar: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[DEBUG] AtivarAdmin: Comando Ativar executado")

	if err := repository.Save(targetAdmin); err != nil {
		log.Printf("[ERROR] AtivarAdmin: Erro ao salvar: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao ativar administrador"})
		return
	}

	log.Printf("[DEBUG] AtivarAdmin: Eventos salvos")

	// Registrar ação
	registrarAcaoAdmin(c, userID.(uuid.UUID), "admin_ativado", map[string]interface{}{
		"target_admin_id": targetID.String(),
	})

	log.Printf("[DEBUG] AtivarAdmin: Sucesso - Admin %s ativado", targetID)

	c.JSON(http.StatusOK, gin.H{
		"message": "administrador ativado com sucesso",
		"email":   targetAdmin.Email,
	})
}

// DesativarAdmin desativa um administrador (apenas FPP)
func DesativarAdmin(c *gin.Context) {
	userID, _ := c.Get("user_id")
	targetID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		log.Printf("[ERROR] DesativarAdmin: ID inválido: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID inválido"})
		return
	}

	log.Printf("[DEBUG] DesativarAdmin: UserID: %s, TargetID: %s", userID, targetID)

	var req struct {
		Motivo string `json:"motivo" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[ERROR] DesativarAdmin: Erro no bind JSON: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "motivo é obrigatório"})
		return
	}

	log.Printf("[DEBUG] DesativarAdmin: Motivo: %s", req.Motivo)

	// Verificar se é FPP
	if err := verificarPermissaoAdmin(c, "fpp"); err != nil {
		log.Printf("[ERROR] DesativarAdmin: Permissão negada: %v", err)
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[DEBUG] DesativarAdmin: Permissão FPP verificada")

	// Não pode desativar a si mesmo
	if targetID == userID.(uuid.UUID) {
		log.Printf("[ERROR] DesativarAdmin: Tentativa de auto-desativação")
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "você não pode desativar sua própria conta",
		})
		return
	}

	// Carregar admin alvo
	repository := getRepository(c)
	targetAdminAgg, err := repository.Load(targetID, "Admin")
	if err != nil {
		log.Printf("[ERROR] DesativarAdmin: Erro ao carregar agregado: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "administrador não encontrado"})
		return
	}

	log.Printf("[DEBUG] DesativarAdmin: Agregado carregado")

	targetAdmin := targetAdminAgg.(*aggregates.Admin)
	if err := targetAdmin.Desativar(userID.(uuid.UUID), req.Motivo); err != nil {
		log.Printf("[ERROR] DesativarAdmin: Erro ao desativar: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[DEBUG] DesativarAdmin: Comando Desativar executado")

	if err := repository.Save(targetAdmin); err != nil {
		log.Printf("[ERROR] DesativarAdmin: Erro ao salvar: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao desativar administrador"})
		return
	}

	log.Printf("[DEBUG] DesativarAdmin: Eventos salvos")

	// Registrar ação
	registrarAcaoAdmin(c, userID.(uuid.UUID), "admin_desativado", map[string]interface{}{
		"target_admin_id": targetID.String(),
		"motivo":          req.Motivo,
	})

	log.Printf("[DEBUG] DesativarAdmin: Sucesso - Admin %s desativado", targetID)

	c.JSON(http.StatusOK, gin.H{
		"message": "administrador desativado com sucesso",
		"email":   targetAdmin.Email,
	})
}