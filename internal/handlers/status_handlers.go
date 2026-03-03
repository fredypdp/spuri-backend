// ============================================================================
// ARQUIVO: internal/handlers/status_handlers.go
// ============================================================================
// Handlers para atualização de status escolar do estudante.
// ATENÇÃO: Estes handlers NÃO devem ser redeclarados em nenhum outro arquivo.
//          O arquivo vincular_handler.go foi REMOVIDO — estes handlers estavam
//          duplicados lá e causavam erro de compilação.
// ============================================================================

package handlers

import (
	"fmt"
	"log"
	"net/http"

	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/utils"

	"github.com/gin-gonic/gin"
)

// AtualizarStatusEscolarFundamentalHandler — PUT /estudante/status-escolar-fundamental
func AtualizarStatusEscolarFundamentalHandler(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		NovoStatus string `json:"novo_status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("novo_status é obrigatório"))
		return
	}

	repository := getRepository(c)
	estudanteAgg, err := repository.Load(userID, "Estudante")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)
	if err := estudante.AtualizarStatusEscolarFundamental(req.NovoStatus); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(estudante); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("✅ Status escolar fundamental atualizado: %s → %s", estudante.CodigoEstudante, req.NovoStatus)
	c.JSON(http.StatusOK, gin.H{
		"message":     "status_escolar_fundamental atualizado com sucesso",
		"novo_status": req.NovoStatus,
	})
}

// AtualizarStatusEscolarMedioHandler — PUT /estudante/status-escolar-medio
func AtualizarStatusEscolarMedioHandler(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		NovoStatus string `json:"novo_status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("novo_status é obrigatório"))
		return
	}

	repository := getRepository(c)
	estudanteAgg, err := repository.Load(userID, "Estudante")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)
	if err := estudante.AtualizarStatusEscolarMedio(req.NovoStatus); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(estudante); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("✅ Status escolar médio atualizado: %s → %s", estudante.CodigoEstudante, req.NovoStatus)
	c.JSON(http.StatusOK, gin.H{
		"message":     "status_escolar_medio atualizado com sucesso",
		"novo_status": req.NovoStatus,
	})
}

// AtualizarStatusSuperior — PUT /estudante/status-superior
func AtualizarStatusSuperior(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		NovoStatus string `json:"novo_status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("novo_status é obrigatório"))
		return
	}

	repository := getRepository(c)
	estudanteAgg, err := repository.Load(userID, "Estudante")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)
	if err := estudante.AtualizarStatusSuperior(req.NovoStatus); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(estudante); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("✅ Status superior atualizado: %s → %s", estudante.CodigoEstudante, req.NovoStatus)
	c.JSON(http.StatusOK, gin.H{
		"message":     "status superior atualizado com sucesso",
		"novo_status": req.NovoStatus,
	})
}