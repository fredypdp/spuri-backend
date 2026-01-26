package handlers

import (
	"log"
	"net/http"
	"spuri/internal/domain/aggregates"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ListarTodosAdmins lista todos os administradores (apenas ADM+)
func ListarTodosAdmins(c *gin.Context) {
	adminProj := getAdminProjection(c)
	
	admins, err := adminProj.GetAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar administradores"})
		return
	}

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
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID inválido"})
		return
	}

	if err := verificarPermissaoAdmin(c, "fpp"); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	repository := getRepository(c)
	targetAdminAgg, err := repository.Load(targetID, "Admin")
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "administrador não encontrado"})
		return
	}

	targetAdmin := targetAdminAgg.(*aggregates.Admin)
	if err := targetAdmin.Ativar(userID.(uuid.UUID)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(targetAdmin); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao ativar administrador"})
		return
	}

	registrarAcaoAdmin(c, userID.(uuid.UUID), "admin_ativado", map[string]interface{}{
		"target_admin_id": targetID.String(),
	})

	log.Printf("Admin ativado: %s", targetAdmin.Email)

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
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID inválido"})
		return
	}

	var req struct {
		Motivo string `json:"motivo" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "motivo é obrigatório"})
		return
	}

	if err := verificarPermissaoAdmin(c, "fpp"); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	if targetID == userID.(uuid.UUID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "você não pode desativar sua própria conta"})
		return
	}

	repository := getRepository(c)
	targetAdminAgg, err := repository.Load(targetID, "Admin")
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "administrador não encontrado"})
		return
	}

	targetAdmin := targetAdminAgg.(*aggregates.Admin)
	if err := targetAdmin.Desativar(userID.(uuid.UUID), req.Motivo); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(targetAdmin); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao desativar administrador"})
		return
	}

	registrarAcaoAdmin(c, userID.(uuid.UUID), "admin_desativado", map[string]interface{}{
		"target_admin_id": targetID.String(),
		"motivo":          req.Motivo,
	})

	log.Printf("Admin desativado: %s - Motivo: %s", targetAdmin.Email, req.Motivo)

	c.JSON(http.StatusOK, gin.H{
		"message": "administrador desativado com sucesso",
		"email":   targetAdmin.Email,
	})
}