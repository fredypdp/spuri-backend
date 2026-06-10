package handlers

import (
	"errors"
	"fmt"
	"io"
	"log"
	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type comandoAcontecimentoEstudante func(*aggregates.Estudante, uuid.UUID, *string) error

func registrarAcontecimentoEstudante(
	c *gin.Context,
	nomeAcontecimento string,
	executarComando comandoAcontecimentoEstudante,
	mensagemSucesso string,
) {
	codigoEstudante := c.Param("codigo")

	var req struct {
		Motivo *string `json:"motivo"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		// Corpo é opcional; quando ausente, Gin retorna EOF e seguimos sem motivo.
		if !errors.Is(err, io.EOF) {
			utils.RespondWithValidationError(c, fmt.Errorf("corpo inválido: %w", err))
			return
		}
		req.Motivo = nil
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

	if err := executarComando(estudante, academiaID, req.Motivo); err != nil {
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

	log.Printf("✅ [Academia %s] Acontecimento '%s' registrado para estudante %s",
		academia.CodigoAcademia, nomeAcontecimento, codigoEstudante)

	c.JSON(200, gin.H{
		"message":       mensagemSucesso,
		"acontecimento": nomeAcontecimento,
	})
}

func EfetivarMatriculaFundamentalHandler(c *gin.Context) {
	registrarAcontecimentoEstudante(c, "MatriculaFundamentalEfetivada", (*aggregates.Estudante).EfetivarMatriculaFundamental, "matrícula no fundamental efetivada com sucesso")
}

func EfetivarMatriculaMedioHandler(c *gin.Context) {
	registrarAcontecimentoEstudante(c, "MatriculaMedioEfetivada", (*aggregates.Estudante).EfetivarMatriculaMedio, "matrícula no médio efetivada com sucesso")
}

func EfetivarMatriculaSuperiorHandler(c *gin.Context) {
	registrarAcontecimentoEstudante(c, "MatriculaSuperiorEfetivada", (*aggregates.Estudante).EfetivarMatriculaSuperior, "matrícula no superior efetivada com sucesso")
}

func InterromperFundamentalHandler(c *gin.Context) {
	registrarAcontecimentoEstudante(c, "FundamentalInterrompido", (*aggregates.Estudante).InterromperFundamental, "interrupção do fundamental registrada com sucesso")
}

func InterromperMedioHandler(c *gin.Context) {
	registrarAcontecimentoEstudante(c, "MedioInterrompido", (*aggregates.Estudante).InterromperMedio, "interrupção do médio registrada com sucesso")
}

func TrancarSuperiorHandler(c *gin.Context) {
	registrarAcontecimentoEstudante(c, "SuperiorTrancado", (*aggregates.Estudante).TrancarSuperior, "trancamento do superior registrado com sucesso")
}

func ArquivarEstudanteHandler(c *gin.Context) {
	registrarAcontecimentoEstudante(c, "EstudanteArquivado", (*aggregates.Estudante).Arquivar, "estudante arquivado com sucesso")
}

func ReativarEstudanteHandler(c *gin.Context) {
	registrarAcontecimentoEstudante(c, "EstudanteReativado", (*aggregates.Estudante).Reativar, "estudante reativado com sucesso")
}
