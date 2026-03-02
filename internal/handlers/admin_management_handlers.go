package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func ListarTodosAdmins(c *gin.Context) {
	adminProj := getAdminProjection(c)
	admins, err := adminProj.GetAll()
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	var adminsResponse []map[string]interface{}
	for _, admin := range admins {
		adminBytes, _ := json.Marshal(admin)
		var adminMap map[string]interface{}
		json.Unmarshal(adminBytes, &adminMap) //nolint:errcheck
		delete(adminMap, "senha_hash")
		adminsResponse = append(adminsResponse, adminMap)
	}

	c.JSON(http.StatusOK, gin.H{
		"admins": adminsResponse,
		"total":  len(adminsResponse),
	})
}

// AtivarAdmin — rota já protegida por RequireFPP no middleware.
// CORRIGIDO: removida chamada redundante a verificarPermissaoAdmin(c, "fpp")
// que gerava dupla consulta ao banco para a mesma verificação.
func AtivarAdmin(c *gin.Context) {
	userID, _ := c.Get("user_id")
	targetID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ID de administrador inválido"))
		return
	}

	repository := getRepository(c)
	targetAdminAgg, err := repository.Load(targetID, "Admin")
	if err != nil {
		utils.RespondWithNotFoundError(c, "administrador")
		return
	}

	targetAdmin := targetAdminAgg.(*aggregates.Admin)
	if err := targetAdmin.Ativar(userID.(uuid.UUID)); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   userID.(uuid.UUID).String(),
		UserType: "admin",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(targetAdmin, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	registrarAcaoAdmin(c, userID.(uuid.UUID), "admin_ativado", map[string]interface{}{
		"target_admin_id": targetID.String(),
		"target_email":    targetAdmin.Email,
	})

	log.Printf("Admin ativado: %s (por: %s)", targetAdmin.Email, userID)

	c.JSON(http.StatusOK, gin.H{
		"message": "administrador ativado com sucesso",
		"email":   targetAdmin.Email,
	})
}

// DesativarAdmin — rota já protegida por RequireFPP no middleware.
// CORRIGIDO: removida chamada redundante a verificarPermissaoAdmin(c, "fpp").
func DesativarAdmin(c *gin.Context) {
	userID, _ := c.Get("user_id")
	targetID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ID de administrador inválido"))
		return
	}

	var req struct {
		Motivo string `json:"motivo" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("motivo é obrigatório"))
		return
	}

	// Proteção contra auto-desativação
	if targetID == userID.(uuid.UUID) {
		utils.RespondWithValidationError(c, fmt.Errorf("você não pode desativar sua própria conta"))
		return
	}

	repository := getRepository(c)
	targetAdminAgg, err := repository.Load(targetID, "Admin")
	if err != nil {
		utils.RespondWithNotFoundError(c, "administrador")
		return
	}

	targetAdmin := targetAdminAgg.(*aggregates.Admin)
	if err := targetAdmin.Desativar(userID.(uuid.UUID), req.Motivo); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   userID.(uuid.UUID).String(),
		UserType: "admin",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(targetAdmin, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	registrarAcaoAdmin(c, userID.(uuid.UUID), "admin_desativado", map[string]interface{}{
		"target_admin_id": targetID.String(),
		"target_email":    targetAdmin.Email,
		"motivo":          req.Motivo,
	})

	log.Printf("Admin desativado: %s - Motivo: %s (por: %s)", targetAdmin.Email, req.Motivo, userID)

	c.JSON(http.StatusOK, gin.H{
		"message": "administrador desativado com sucesso",
		"email":   targetAdmin.Email,
	})
}