package handlers

import (
	"fmt"
	"net/http"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ============================================
// GET /inscricoes/estudante/:codigo
// Buscar todas as inscrições de um estudante pelo código
// Acesso: estudante (próprio), academia, admin
// ============================================

func GetInscricoesPorCodigoEstudante(c *gin.Context) {
	codigoEstudante := c.Param("codigo")

	// Buscar estudante
	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByCodigo(codigoEstudante)
	if err != nil || estudante == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	// Verificar permissões
	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	// Estudante só pode ver suas próprias inscrições
	if userType == "estudante" && userID != estudante.ID {
		utils.RespondWithForbiddenError(c, "acesso negado")
		return
	}

	// Academia só pode ver inscrições de estudantes da sua academia
	if userType == "academia" {
		academiaProj := getAcademiaProjection(c)
		academiaDTO, _ := academiaProj.GetByID(userID)
		if estudante.CodigoAcademia == nil || academiaDTO == nil || *estudante.CodigoAcademia != academiaDTO.CodigoAcademia {
			utils.RespondWithForbiddenError(c, "estudante não pertence a esta academia")
			return
		}
	}

	// Buscar todas as inscrições do estudante
	inscProj := getInscricoesProjection(c)
	inscricoes, err := inscProj.GetByEstudante(estudante.ID)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"codigo_estudante": codigoEstudante,
		"nome":             estudante.Nome,
		"inscricoes":       inscricoes,
		"total":            len(inscricoes),
	})
}

// ============================================
// PUT /academia/estudante/:codigo/curso
// Academia altera o curso do estudante
// Acesso: academia
// ============================================

type AlterarCursoRequest struct {
	TipoEnsino string    `json:"tipo_ensino" binding:"required"` // "medio" ou "superior"
	CursoID    uuid.UUID `json:"curso_id" binding:"required"`
}

func AlterarCursoEstudante(c *gin.Context) {
	codigoEstudante := c.Param("codigo")

	var req AlterarCursoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	// Validar tipo de ensino
	if req.TipoEnsino != "medio" && req.TipoEnsino != "superior" {
		utils.RespondWithValidationError(c, fmt.Errorf("tipo_ensino deve ser 'medio' ou 'superior'"))
		return
	}

	// Buscar estudante
	estudanteProj := getEstudanteProjection(c)
	estudanteDTO, err := estudanteProj.GetByCodigo(codigoEstudante)
	if err != nil || estudanteDTO == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	// Verificar se estudante pertence à academia
	userID, _ := middleware.GetUserID(c)
	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	if estudanteDTO.CodigoAcademia == nil || *estudanteDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "estudante não pertence a esta academia")
		return
	}

	// Verificar se curso existe e pertence à academia
	cursosProj := getCursosProjection(c)
	cursoDTO, err := cursosProj.GetByID(req.CursoID)
	if err != nil || cursoDTO == nil {
		utils.RespondWithNotFoundError(c, "curso")
		return
	}

	if cursoDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "curso não pertence a esta academia")
		return
	}

	// Validar tipo de curso
	if req.TipoEnsino == "medio" && cursoDTO.Type != "medio" {
		utils.RespondWithValidationError(c, fmt.Errorf("curso não é do tipo médio"))
		return
	}
	if req.TipoEnsino == "superior" && cursoDTO.Type != "superior" {
		utils.RespondWithValidationError(c, fmt.Errorf("curso não é do tipo superior"))
		return
	}

	// Carregar agregado
	repository := getRepository(c)
	aggregate, err := repository.Load(estudanteDTO.ID, "Estudante")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	estudante, ok := aggregate.(*aggregates.Estudante)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("erro ao converter agregado"))
		return
	}

	// Executar comando
	if err := estudante.AlterarCurso(req.TipoEnsino, req.CursoID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	// Salvar
	if err := repository.Save(estudante); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":          "curso alterado com sucesso",
		"codigo_estudante": codigoEstudante,
		"tipo_ensino":      req.TipoEnsino,
		"curso_id":         req.CursoID,
		"curso_nome":       cursoDTO.Nome,
	})
}