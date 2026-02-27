package handlers

import (
	"fmt"
	"log"
	"net/http"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// CriarCurso — POST /academia/cursos
func CriarCurso(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		Nome           string   `json:"nome"            binding:"required"`
		Type           string   `json:"type"            binding:"required"` // medio | superior
		AnosAcademicos []string `json:"anos_academicos" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("campos obrigatórios: nome, type, anos_academicos"))
		return
	}

	if req.Type != "medio" && req.Type != "superior" {
		utils.RespondWithValidationError(c, fmt.Errorf("type deve ser 'medio' ou 'superior'"))
		return
	}

	if len(req.AnosAcademicos) == 0 {
		utils.RespondWithValidationError(c, fmt.Errorf("anos_academicos é obrigatório e não pode ser vazio"))
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

	// Escolas puramente fundamentais não criam cursos
	if academiaDTO.NivelEscolar != nil && *academiaDTO.NivelEscolar == "fundamental" {
		utils.RespondWithForbiddenError(c, "Escolas de ensino fundamental não criam cursos (use anos_academicos na academia)")
		return
	}

	curso := aggregates.NewCurso()
	if err := curso.Criar(req.Nome, req.Type, req.AnosAcademicos, academiaDTO.CodigoAcademia); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	repository := getRepository(c)
	if err := repository.Save(curso); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Curso criado: %s (%s) — %d anos", curso.Nome, curso.Type, len(curso.AnosAcademicos))

	c.JSON(http.StatusCreated, gin.H{
		"message":         "curso criado com sucesso",
		"id":              curso.ID,
		"nome":            curso.Nome,
		"type":            curso.Type,
		"anos_academicos": curso.AnosAcademicos,
	})
}

// AtualizarDadosCurso — PUT /academia/cursos/:id
func AtualizarDadosCurso(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	cursoID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ID de curso inválido"))
		return
	}

	var req struct {
		Nome           *string  `json:"nome"`
		Type           *string  `json:"type"`
		AnosAcademicos []string `json:"anos_academicos"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("dados inválidos"))
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
		utils.RespondWithForbiddenError(c, "Curso não pertence a esta academia")
		return
	}

	repository := getRepository(c)
	cursoAgg, err := repository.Load(cursoID, "Curso")
	if err != nil {
		utils.RespondWithNotFoundError(c, "curso")
		return
	}

	curso := cursoAgg.(*aggregates.Curso)

	if err := curso.AtualizarDados(req.Nome, req.Type, req.AnosAcademicos); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(curso); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Curso atualizado: %s", curso.Nome)
	c.JSON(http.StatusOK, gin.H{
		"message":         "curso atualizado com sucesso",
		"id":              curso.ID,
		"nome":            curso.Nome,
		"anos_academicos": curso.AnosAcademicos,
	})
}

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

// AtivarCurso — PUT /academia/cursos/:id/ativar
func AtivarCurso(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	cursoID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ID de curso inválido"))
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
		utils.RespondWithForbiddenError(c, "Curso não pertence a esta academia")
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

// DesativarCurso — PUT /academia/cursos/:id/desativar
func DesativarCurso(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	cursoID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ID de curso inválido"))
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
		utils.RespondWithForbiddenError(c, "Curso não pertence a esta academia")
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

func CriarMateria(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		Nome    string     `json:"nome" binding:"required"`
		Type    string     `json:"type" binding:"required"`
		Nivel   []string   `json:"nivel"`
		CursoID *uuid.UUID `json:"curso_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("dados obrigatórios: nome e tipo"))
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	if academiaDTO.Status != "ativo" {
		utils.RespondWithForbiddenError(c, "Academia inativa não pode criar matérias")
		return
	}

	if (req.Type == "medio" || req.Type == "superior") && req.CursoID != nil {
		cursosProj := getCursosProjection(c)
		cursoDTO, _ := cursosProj.GetByID(*req.CursoID)
		
		if cursoDTO == nil {
			utils.RespondWithNotFoundError(c, "curso")
			return
		}
		
		if cursoDTO.Status != "ativo" {
			utils.RespondWithValidationError(c, fmt.Errorf("curso inativo não pode ter matérias"))
			return
		}
		
		if cursoDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
			utils.RespondWithForbiddenError(c, "Curso não pertence a esta academia")
			return
		}
	}

	repository := getRepository(c)
	materia := aggregates.NewMateriaDisciplinar()

	if err := materia.Criar(req.Nome, req.Type, req.Nivel, academiaDTO.CodigoAcademia, req.CursoID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(materia); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Matéria criada: %s - %s", req.Nome, materia.ID)

	c.JSON(http.StatusCreated, gin.H{
		"message": "matéria criada com sucesso",
		"data": gin.H{
			"id":   materia.ID,
			"nome": materia.Nome,
			"type": materia.Type,
		},
	})
}

func ListarMaterias(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	materiasProj := getMateriasProjection(c)
	materias, err := materiasProj.GetByAcademia(academiaDTO.CodigoAcademia)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"materias": materias,
		"total":    len(materias),
	})
}

func AtivarMateria(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	materiaID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ID de matéria inválido"))
		return
	}

	materiasProj := getMateriasProjection(c)
	materiaDTO, err := materiasProj.GetByID(materiaID)
	if err != nil || materiaDTO == nil {
		utils.RespondWithNotFoundError(c, "matéria")
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, _ := academiaProj.GetByID(userID)
	if academiaDTO == nil || academiaDTO.CodigoAcademia != materiaDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "Matéria não pertence a esta academia")
		return
	}

	repository := getRepository(c)
	materiaAgg, err := repository.Load(materiaID, "MateriaDisciplinar")
	if err != nil {
		utils.RespondWithNotFoundError(c, "matéria")
		return
	}

	materia := materiaAgg.(*aggregates.MateriaDisciplinar)

	if err := materia.Ativar(); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(materia); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Matéria ativada: %s", materia.Nome)

	c.JSON(http.StatusOK, gin.H{
		"message": "matéria ativada com sucesso",
		"nome":    materia.Nome,
	})
}

func DesativarMateria(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	materiaID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ID de matéria inválido"))
		return
	}

	materiasProj := getMateriasProjection(c)
	materiaDTO, err := materiasProj.GetByID(materiaID)
	if err != nil || materiaDTO == nil {
		utils.RespondWithNotFoundError(c, "matéria")
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, _ := academiaProj.GetByID(userID)
	if academiaDTO == nil || academiaDTO.CodigoAcademia != materiaDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "Matéria não pertence a esta academia")
		return
	}

	repository := getRepository(c)
	materiaAgg, err := repository.Load(materiaID, "MateriaDisciplinar")
	if err != nil {
		utils.RespondWithNotFoundError(c, "matéria")
		return
	}

	materia := materiaAgg.(*aggregates.MateriaDisciplinar)

	if err := materia.Desativar(); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(materia); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Matéria desativada: %s", materia.Nome)

	c.JSON(http.StatusOK, gin.H{
		"message": "matéria desativada com sucesso",
		"nome":    materia.Nome,
	})
}