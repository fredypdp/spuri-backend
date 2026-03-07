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

// statusEscolarValidos define os valores permitidos para cada tipo de ensino.
// FIX H4-STS-02: validação de novo_status contra conjunto de valores permitidos
// aplicada no handler, antes de qualquer chamada ao aggregate.
var statusEscolarValidos = map[string]bool{
	"cursando":    true,
	"concluido":   true,
	"reprovado":   true,
	"transferido": true,
	"evadido":     true,
	"matriculado": true,
	"trancado":    true,
}

// atualizarStatusEscolar é o helper centralizado que executa a lógica comum
// dos três handlers de status escolar.
//
// FIX H4-STS-01: três handlers com corpo 100% duplicado substituídos por
// um único helper tipado. Correções de segurança ou lógica aplicadas aqui
// propagam-se automaticamente para todos os tipos de ensino.
//
// FIX H4-STS-02: validação de novo_status contra conjunto de valores permitidos.
// FIX H4-TRX-03: type assertion protegida.
func atualizarStatusEscolar(
	c *gin.Context,
	tipoEnsino string,
	executarComando func(*aggregates.Estudante, string) error,
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

	// FIX H4-STS-02: validar novo_status contra conjunto de valores permitidos.
	if !statusEscolarValidos[req.NovoStatus] {
		utils.RespondWithValidationError(c, fmt.Errorf(
			"novo_status inválido: %q. Valores aceitos: cursando, concluido, reprovado, transferido, evadido, matriculado, trancado",
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

	// FIX H4-TRX-03: type assertion protegida.
	estudante, ok := estudanteAgg.(*aggregates.Estudante)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("tipo de aggregate inesperado"))
		return
	}

	if err := executarComando(estudante, req.NovoStatus); err != nil {
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

// AtualizarStatusEscolarFundamentalHandler — academia atualiza status escolar
// fundamental de um estudante.
// Protegido por RequireAcademia() + ValidarStatusAcademia().
func AtualizarStatusEscolarFundamentalHandler(c *gin.Context) {
	atualizarStatusEscolar(
		c,
		"fundamental",
		func(e *aggregates.Estudante, status string) error {
			return e.AtualizarStatusEscolarFundamental(status)
		},
		"status_escolar_fundamental",
	)
}

// AtualizarStatusEscolarMedioHandler — academia atualiza status escolar médio
// de um estudante.
func AtualizarStatusEscolarMedioHandler(c *gin.Context) {
	atualizarStatusEscolar(
		c,
		"medio",
		func(e *aggregates.Estudante, status string) error {
			return e.AtualizarStatusEscolarMedio(status)
		},
		"status_escolar_medio",
	)
}

// AtualizarStatusSuperiorHandler — academia atualiza status superior de um
// estudante.
func AtualizarStatusSuperiorHandler(c *gin.Context) {
	atualizarStatusEscolar(
		c,
		"superior",
		func(e *aggregates.Estudante, status string) error {
			return e.AtualizarStatusSuperior(status)
		},
		"status_superior",
	)
}