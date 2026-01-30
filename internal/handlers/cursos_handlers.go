package handlers

import (
	"log"
	"net/http"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// CURSOS

// CriarCurso cria um novo curso (apenas academia ativa)
func CriarCurso(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		Nome  string   `json:"nome" binding:"required"`
		Type  string   `json:"type" binding:"required"`
		Nivel []string `json:"nivel" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	// Buscar academia
	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar academia"})
		return
	}

	if academiaDTO.Status != "ativo" {
		c.JSON(http.StatusForbidden, gin.H{"error": "academia inativa não pode criar cursos"})
		return
	}

	// Criar curso
	repository := getRepository(c)
	curso := aggregates.NewCurso()

	if err := curso.Criar(req.Nome, req.Type, req.Nivel, academiaDTO.CodigoAcademia); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(curso); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao criar curso"})
		return
	}

	log.Printf("Curso criado: %s - %s", req.Nome, curso.ID)

	c.JSON(http.StatusCreated, gin.H{
		"message": "curso criado com sucesso",
		"data": gin.H{
			"id":   curso.ID,
			"nome": curso.Nome,
			"type": curso.Type,
		},
	})
}

// ListarCursos lista cursos da academia logada
func ListarCursos(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	// Buscar academia
	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar academia"})
		return
	}

	// Buscar cursos
	cursosProj := getCursosProjection(c)
	cursos, err := cursosProj.GetByAcademia(academiaDTO.CodigoAcademia)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar cursos"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"cursos": cursos,
		"total":  len(cursos),
	})
}

// AtivarCurso ativa um curso (academia)
func AtivarCurso(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	cursoID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID inválido"})
		return
	}

	// Verificar propriedade
	cursosProj := getCursosProjection(c)
	cursoDTO, err := cursosProj.GetByID(cursoID)
	if err != nil || cursoDTO == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "curso não encontrado"})
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, _ := academiaProj.GetByID(userID)
	if academiaDTO == nil || academiaDTO.CodigoAcademia != cursoDTO.CodigoAcademia {
		c.JSON(http.StatusForbidden, gin.H{"error": "curso não pertence a esta academia"})
		return
	}

	// Carregar e ativar
	repository := getRepository(c)
	cursoAgg, err := repository.Load(cursoID, "Curso")
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "curso não encontrado"})
		return
	}

	curso := cursoAgg.(*aggregates.Curso)

	if err := curso.Ativar(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(curso); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao ativar curso"})
		return
	}

	log.Printf("Curso ativado: %s", curso.Nome)

	c.JSON(http.StatusOK, gin.H{
		"message": "curso ativado com sucesso",
		"nome":    curso.Nome,
	})
}

// DesativarCurso desativa um curso (academia)
func DesativarCurso(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	cursoID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID inválido"})
		return
	}

	// Verificar propriedade
	cursosProj := getCursosProjection(c)
	cursoDTO, err := cursosProj.GetByID(cursoID)
	if err != nil || cursoDTO == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "curso não encontrado"})
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, _ := academiaProj.GetByID(userID)
	if academiaDTO == nil || academiaDTO.CodigoAcademia != cursoDTO.CodigoAcademia {
		c.JSON(http.StatusForbidden, gin.H{"error": "curso não pertence a esta academia"})
		return
	}

	// Carregar e desativar
	repository := getRepository(c)
	cursoAgg, err := repository.Load(cursoID, "Curso")
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "curso não encontrado"})
		return
	}

	curso := cursoAgg.(*aggregates.Curso)

	if err := curso.Desativar(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(curso); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao desativar curso"})
		return
	}

	log.Printf("Curso desativado: %s", curso.Nome)

	c.JSON(http.StatusOK, gin.H{
		"message": "curso desativado com sucesso",
		"nome":    curso.Nome,
	})
}

// MATÉRIAS

// CriarMateria cria uma nova matéria (apenas academia ativa)
func CriarMateria(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		Nome    string     `json:"nome" binding:"required"`
		Type    string     `json:"type" binding:"required"`
		Nivel   []string   `json:"nivel"`
		CursoID *uuid.UUID `json:"curso_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	// Buscar academia
	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar academia"})
		return
	}

	if academiaDTO.Status != "ativo" {
		c.JSON(http.StatusForbidden, gin.H{"error": "academia inativa não pode criar matérias"})
		return
	}

	// Verificar curso se necessário
	if (req.Type == "medio" || req.Type == "superior") && req.CursoID != nil {
		cursosProj := getCursosProjection(c)
		cursoDTO, _ := cursosProj.GetByID(*req.CursoID)
		
		if cursoDTO == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "curso não encontrado"})
			return
		}
		
		if cursoDTO.Status != "ativo" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "curso inativo não pode ter matérias"})
			return
		}
		
		if cursoDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
			c.JSON(http.StatusForbidden, gin.H{"error": "curso não pertence a esta academia"})
			return
		}
	}

	// Criar matéria
	repository := getRepository(c)
	materia := aggregates.NewMateriaDisciplinar()

	if err := materia.Criar(req.Nome, req.Type, req.Nivel, academiaDTO.CodigoAcademia, req.CursoID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(materia); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao criar matéria"})
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

// ListarMaterias lista matérias da academia
func ListarMaterias(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar academia"})
		return
	}

	materiasProj := getMateriasProjection(c)
	materias, err := materiasProj.GetByAcademia(academiaDTO.CodigoAcademia)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar matérias"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"materias": materias,
		"total":    len(materias),
	})
}

// AtivarMateria ativa uma matéria (academia)
func AtivarMateria(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	materiaID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID inválido"})
		return
	}

	// Verificar propriedade
	materiasProj := getMateriasProjection(c)
	materiaDTO, err := materiasProj.GetByID(materiaID)
	if err != nil || materiaDTO == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "matéria não encontrada"})
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, _ := academiaProj.GetByID(userID)
	if academiaDTO == nil || academiaDTO.CodigoAcademia != materiaDTO.CodigoAcademia {
		c.JSON(http.StatusForbidden, gin.H{"error": "matéria não pertence a esta academia"})
		return
	}

	// Carregar e ativar
	repository := getRepository(c)
	materiaAgg, err := repository.Load(materiaID, "MateriaDisciplinar")
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "matéria não encontrada"})
		return
	}

	materia := materiaAgg.(*aggregates.MateriaDisciplinar)

	if err := materia.Ativar(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(materia); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao ativar matéria"})
		return
	}

	log.Printf("Matéria ativada: %s", materia.Nome)

	c.JSON(http.StatusOK, gin.H{
		"message": "matéria ativada com sucesso",
		"nome":    materia.Nome,
	})
}

// DesativarMateria desativa uma matéria (academia)
func DesativarMateria(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	materiaID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID inválido"})
		return
	}

	// Verificar propriedade
	materiasProj := getMateriasProjection(c)
	materiaDTO, err := materiasProj.GetByID(materiaID)
	if err != nil || materiaDTO == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "matéria não encontrada"})
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, _ := academiaProj.GetByID(userID)
	if academiaDTO == nil || academiaDTO.CodigoAcademia != materiaDTO.CodigoAcademia {
		c.JSON(http.StatusForbidden, gin.H{"error": "matéria não pertence a esta academia"})
		return
	}

	// Carregar e desativar
	repository := getRepository(c)
	materiaAgg, err := repository.Load(materiaID, "MateriaDisciplinar")
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "matéria não encontrada"})
		return
	}

	materia := materiaAgg.(*aggregates.MateriaDisciplinar)

	if err := materia.Desativar(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(materia); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao desativar matéria"})
		return
	}

	log.Printf("Matéria desativada: %s", materia.Nome)

	c.JSON(http.StatusOK, gin.H{
		"message": "matéria desativada com sucesso",
		"nome":    materia.Nome,
	})
}