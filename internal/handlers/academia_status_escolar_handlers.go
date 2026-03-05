package handlers

import (
	"fmt"
	"log"
	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/utils"

	"github.com/gin-gonic/gin"
)

// AtualizarStatusEscolarFundamentalHandler — academia atualiza status escolar fundamental de um estudante.
// Protegido por RequireAcademia() + ValidarStatusAcademia().
func AtualizarStatusEscolarFundamentalHandler(c *gin.Context) {
	codigoEstudante := c.Param("codigo")

	var req struct {
		NovoStatus string `json:"novo_status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("novo_status é obrigatório"))
		return
	}

	academiaID, _ := middleware.GetUserID(c)
	academiaProj := getAcademiaProjection(c)
	academia, err := academiaProj.GetByID(academiaID)
	if err != nil || academia == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	estudanteProj := getEstudanteProjection(c)
	estudanteDTO, err := estudanteProj.GetByCodigo(codigoEstudante)
	if err != nil || estudanteDTO == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	if estudanteDTO.CodigoAcademia == nil || *estudanteDTO.CodigoAcademia != academia.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "estudante não pertence a esta academia")
		return
	}

	repository := getRepository(c)
	estudanteAgg, err := repository.Load(estudanteDTO.ID, "Estudante")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)
	if err := estudante.AtualizarStatusEscolarFundamental(req.NovoStatus); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   academiaID.String(),
		UserType: "academia",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(estudante, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("✅ [Academia %s] Status escolar fundamental atualizado: %s → %s",
		academia.CodigoAcademia, codigoEstudante, req.NovoStatus)
	c.JSON(200, gin.H{
		"message":     "status_escolar_fundamental atualizado com sucesso",
		"novo_status": req.NovoStatus,
	})
}

// AtualizarStatusEscolarMedioHandler — academia atualiza status escolar médio de um estudante.
func AtualizarStatusEscolarMedioHandler(c *gin.Context) {
	codigoEstudante := c.Param("codigo")

	var req struct {
		NovoStatus string `json:"novo_status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("novo_status é obrigatório"))
		return
	}

	academiaID, _ := middleware.GetUserID(c)
	academiaProj := getAcademiaProjection(c)
	academia, err := academiaProj.GetByID(academiaID)
	if err != nil || academia == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	estudanteProj := getEstudanteProjection(c)
	estudanteDTO, err := estudanteProj.GetByCodigo(codigoEstudante)
	if err != nil || estudanteDTO == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	if estudanteDTO.CodigoAcademia == nil || *estudanteDTO.CodigoAcademia != academia.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "estudante não pertence a esta academia")
		return
	}

	repository := getRepository(c)
	estudanteAgg, err := repository.Load(estudanteDTO.ID, "Estudante")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)
	if err := estudante.AtualizarStatusEscolarMedio(req.NovoStatus); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   academiaID.String(),
		UserType: "academia",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(estudante, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("✅ [Academia %s] Status escolar médio atualizado: %s → %s",
		academia.CodigoAcademia, codigoEstudante, req.NovoStatus)
	c.JSON(200, gin.H{
		"message":     "status_escolar_medio atualizado com sucesso",
		"novo_status": req.NovoStatus,
	})
}

// AtualizarStatusSuperiorHandler — academia atualiza status superior de um estudante.
func AtualizarStatusSuperiorHandler(c *gin.Context) {
	codigoEstudante := c.Param("codigo")

	var req struct {
		NovoStatus string `json:"novo_status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("novo_status é obrigatório"))
		return
	}

	academiaID, _ := middleware.GetUserID(c)
	academiaProj := getAcademiaProjection(c)
	academia, err := academiaProj.GetByID(academiaID)
	if err != nil || academia == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	estudanteProj := getEstudanteProjection(c)
	estudanteDTO, err := estudanteProj.GetByCodigo(codigoEstudante)
	if err != nil || estudanteDTO == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	if estudanteDTO.CodigoAcademia == nil || *estudanteDTO.CodigoAcademia != academia.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "estudante não pertence a esta academia")
		return
	}

	repository := getRepository(c)
	estudanteAgg, err := repository.Load(estudanteDTO.ID, "Estudante")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)
	if err := estudante.AtualizarStatusSuperior(req.NovoStatus); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   academiaID.String(),
		UserType: "academia",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(estudante, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("✅ [Academia %s] Status superior atualizado: %s → %s",
		academia.CodigoAcademia, codigoEstudante, req.NovoStatus)
	c.JSON(200, gin.H{
		"message":     "status_superior atualizado com sucesso",
		"novo_status": req.NovoStatus,
	})
}