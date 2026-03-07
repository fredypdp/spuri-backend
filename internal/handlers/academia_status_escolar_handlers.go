package handlers

import (
	"fmt"
	"log"
	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

var statusEscolarValidos = map[string]bool{
	"inativo":      true,
	"em_andamento": true,
	"finalizado":   true,
}

func atualizarStatusEscolar(
	c *gin.Context,
	tipoEnsino string,
	executarComando func(*aggregates.Estudante, string, uuid.UUID) error,
	campoResposta string,
) {
	codigoEstudante := c.Param("codigo")

	var req struct {
		NovoStatus string `json:"novo_status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("novo_status é obrigatório"))
		return
	}
	
	if !statusEscolarValidos[req.NovoStatus] {
		utils.RespondWithValidationError(c, fmt.Errorf(
			"novo_status inválido: %q. Valores aceitos: inativo, em_andamento, finalizado",
			req.NovoStatus,
		))
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
	
	estudante, ok := estudanteAgg.(*aggregates.Estudante)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("tipo de aggregate inesperado"))
		return
	}
	
	if err := executarComando(estudante, req.NovoStatus, academiaID); err != nil {
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

	log.Printf("✅ [Academia %s] Status escolar (%s) atualizado: %s → %s",
		academia.CodigoAcademia, tipoEnsino, codigoEstudante, req.NovoStatus)

	c.JSON(200, gin.H{
		"message":     campoResposta + " atualizado com sucesso",
		"novo_status": req.NovoStatus,
	})
}

func AtualizarStatusEscolarFundamentalHandler(c *gin.Context) {
	atualizarStatusEscolar(
		c,
		"fundamental",
		func(e *aggregates.Estudante, status string, atualizadoPor uuid.UUID) error {
			return e.AtualizarStatusEscolarFundamental(status, atualizadoPor)
		},
		"status_escolar_fundamental",
	)
}

func AtualizarStatusEscolarMedioHandler(c *gin.Context) {
	atualizarStatusEscolar(
		c,
		"medio",
		func(e *aggregates.Estudante, status string, atualizadoPor uuid.UUID) error {
			return e.AtualizarStatusEscolarMedio(status, atualizadoPor)
		},
		"status_escolar_medio",
	)
}

func AtualizarStatusSuperiorHandler(c *gin.Context) {
	atualizarStatusEscolar(
		c,
		"superior",
		func(e *aggregates.Estudante, status string, atualizadoPor uuid.UUID) error {
			return e.AtualizarStatusSuperior(status, atualizadoPor)
		},
		"status_superior",
	)
}