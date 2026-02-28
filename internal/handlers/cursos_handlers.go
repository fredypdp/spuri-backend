package handlers

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/utils"
)

// ============================================================================
// POST /academia/cursos
// ============================================================================

func CriarCurso(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		Nome           string   `json:"nome"            binding:"required"`
		Type           string   `json:"type"            binding:"required"`
		AnosAcademicos []string `json:"anos_academicos" binding:"required"`
		Periodos       []string `json:"periodos"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("campos obrigatórios: nome, type, anos_academicos"))
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	if academiaDTO.Status != "ativo" {
		utils.RespondWithForbiddenError(c, "Academia inativa não pode criar cursos")
		return
	}

	if err := validarTipoCursoVsAcademia(req.Type, academiaDTO.Type); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	repository := getRepository(c)
	curso := aggregates.NewCurso()

	if err := curso.Criar(req.Nome, req.Type, req.AnosAcademicos, req.Periodos, academiaDTO.CodigoAcademia); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(curso); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Curso criado: %s - %s (periodos=%v)", req.Nome, curso.ID, curso.Periodos)

	c.JSON(http.StatusCreated, gin.H{
		"message": "curso criado com sucesso",
		"data": gin.H{
			"id":       curso.ID,
			"nome":     curso.Nome,
			"type":     curso.Type,
			"periodos": curso.Periodos,
		},
	})
}

// ============================================================================
// GET /academia/cursos
// ============================================================================

func ListarCursos(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	cursosProj := getCursosProjection(c)
	cursos, err := cursosProj.GetByAcademia(academiaDTO.CodigoAcademia)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"cursos": cursos,
		"total":  len(cursos),
	})
}

// ============================================================================
// PUT /academia/cursos/:id
// ============================================================================

func AtualizarDadosCurso(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	cursoID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ID de curso inválido"))
		return
	}

	// Type não é aceito: o tipo do curso é imutável após a criação.
	var req struct {
		Nome           *string   `json:"nome"`
		AnosAcademicos []string  `json:"anos_academicos"`
		Periodos       *[]string `json:"periodos"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("dados invalidos"))
		return
	}

	cursosProj := getCursosProjection(c)
	cursoDTO, err := cursosProj.GetByID(cursoID)
	if err != nil || cursoDTO == nil {
		utils.RespondWithNotFoundError(c, "curso")
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, _ := academiaProj.GetByID(userID)
	if academiaDTO == nil || academiaDTO.CodigoAcademia != cursoDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "curso nao pertence a esta academia")
		return
	}

	repository := getRepository(c)
	cursoAgg, err := repository.Load(cursoID, "Curso")
	if err != nil {
		utils.RespondWithNotFoundError(c, "curso")
		return
	}

	curso := cursoAgg.(*aggregates.Curso)

	if err := curso.AtualizarDados(req.Nome, req.AnosAcademicos, req.Periodos); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(curso); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Curso atualizado: %s (periodos=%v)", curso.Nome, curso.Periodos)
	c.JSON(http.StatusOK, gin.H{
		"message":         "curso atualizado com sucesso",
		"nome":            curso.Nome,
		"type":            curso.Type,
		"anos_academicos": curso.AnosAcademicos,
		"periodos":        curso.Periodos,
	})
}

// ============================================================================
// PUT /academia/cursos/:id/ativar
// ============================================================================

func AtivarCurso(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	cursoID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ID de curso invalido"))
		return
	}

	cursosProj := getCursosProjection(c)
	cursoDTO, err := cursosProj.GetByID(cursoID)
	if err != nil || cursoDTO == nil {
		utils.RespondWithNotFoundError(c, "curso")
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, _ := academiaProj.GetByID(userID)
	if academiaDTO == nil || academiaDTO.CodigoAcademia != cursoDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "Curso nao pertence a esta academia")
		return
	}

	repository := getRepository(c)
	cursoAgg, err := repository.Load(cursoID, "Curso")
	if err != nil {
		utils.RespondWithNotFoundError(c, "curso")
		return
	}

	curso := cursoAgg.(*aggregates.Curso)

	if err := curso.Ativar(); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(curso); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Curso ativado: %s", curso.Nome)
	c.JSON(http.StatusOK, gin.H{"message": "curso ativado com sucesso", "nome": curso.Nome})
}

// ============================================================================
// PUT /academia/cursos/:id/desativar
// ============================================================================

func DesativarCurso(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	cursoID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ID de curso invalido"))
		return
	}

	cursosProj := getCursosProjection(c)
	cursoDTO, err := cursosProj.GetByID(cursoID)
	if err != nil || cursoDTO == nil {
		utils.RespondWithNotFoundError(c, "curso")
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, _ := academiaProj.GetByID(userID)
	if academiaDTO == nil || academiaDTO.CodigoAcademia != cursoDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "Curso nao pertence a esta academia")
		return
	}

	repository := getRepository(c)
	cursoAgg, err := repository.Load(cursoID, "Curso")
	if err != nil {
		utils.RespondWithNotFoundError(c, "curso")
		return
	}

	curso := cursoAgg.(*aggregates.Curso)

	if err := curso.Desativar(); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(curso); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Curso desativado: %s", curso.Nome)
	c.JSON(http.StatusOK, gin.H{"message": "curso desativado com sucesso", "nome": curso.Nome})
}

// ============================================================================
// Helpers internos
// ============================================================================

func validarTipoCursoVsAcademia(tipoCurso, tipoAcademia string) error {
	switch tipoAcademia {
	case "escola":
		if tipoCurso != "medio" {
			return fmt.Errorf("academias do tipo 'escola' so podem criar cursos do tipo 'medio'")
		}
	case "superior":
		if tipoCurso != "superior" {
			return fmt.Errorf("academias do tipo 'superior' so podem criar cursos do tipo 'superior'")
		}
	}
	return nil
}