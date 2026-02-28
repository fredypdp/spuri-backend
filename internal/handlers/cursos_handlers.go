// ============================================================================
// ARQUIVO: internal/handlers/cursos_handlers.go
//
// ATUALIZAÇÃO 1:
//   - CriarMateria: campo de request renomeado de "nivel" para "anos_academicos"
//     para refletir a semântica correta.
//   - Validação via utils.ValidateAnosMateria (em vez de só ValidateAnosFundamental):
//       • fundamental → 1–9 itens, todos primeiro_fundamental…nono_fundamental
//       • medio/superior → exatamente 1 item (livre, deve ser ano do curso)
//   - Para medio/superior, valida que anos_academicos[0] pertence ao curso vinculado.
// ============================================================================

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
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("dados obrigatórios: nome, type, anos_academicos"))
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	if academiaDTO.Status != "ativo" {
		utils.RespondWithForbiddenError(c, "Academia inativa não pode criar cursos")
		return
	}

	repository := getRepository(c)
	curso := aggregates.NewCurso()

	if err := curso.Criar(req.Nome, req.Type, req.AnosAcademicos, academiaDTO.CodigoAcademia); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(curso); err != nil {
		utils.RespondWithInternalError(c, err)
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
// PUT /academia/cursos/:id/ativar
// ============================================================================

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

// ============================================================================
// PUT /academia/cursos/:id/desativar
// ============================================================================

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

// ============================================================================
// POST /academia/materias
// ============================================================================

// CriarMateria cria uma nova matéria disciplinar.
//
// ATUALIZAÇÃO 1 — campo "anos_academicos" (antes "nivel"):
//   - fundamental : array com 1–9 anos (primeiro_fundamental…nono_fundamental)
//   - medio/superior: array com EXATAMENTE 1 item — o ano do curso ao qual a
//     matéria pertence (deve existir em curso.AnosAcademicos)
func CriarMateria(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		Nome           string     `json:"nome"            binding:"required"`
		Type           string     `json:"type"            binding:"required"`
		AnosAcademicos []string   `json:"anos_academicos"`
		CursoID        *uuid.UUID `json:"curso_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("dados obrigatórios: nome e type"))
		return
	}

	// ── Academia ───────────────────────────────────────────────────────────
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

	// ── Pré-validar anos_academicos antes de chamar o aggregate ───────────
	if err := utils.ValidateAnosMateria(req.Type, req.AnosAcademicos); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	// ── Para medio/superior: validar curso e que o ano pertence ao curso ──
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

		// O único ano da matéria deve pertencer aos anos do curso
		anoMateria := req.AnosAcademicos[0]
		anoValido := false
		for _, a := range cursoDTO.AnosAcademicos {
			if a == anoMateria {
				anoValido = true
				break
			}
		}
		if !anoValido {
			utils.RespondWithValidationError(c, fmt.Errorf(
				"o ano_academico '%s' não pertence aos anos do curso '%s'. "+
					"Anos disponíveis: %v",
				anoMateria, cursoDTO.Nome, cursoDTO.AnosAcademicos,
			))
			return
		}
	}

	// ── Criar aggregate ────────────────────────────────────────────────────
	repository := getRepository(c)
	materia := aggregates.NewMateriaDisciplinar()

	// O aggregate recebe AnosAcademicos no parâmetro "nivel" (campo interno)
	if err := materia.Criar(req.Nome, req.Type, req.AnosAcademicos, academiaDTO.CodigoAcademia, req.CursoID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(materia); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Matéria criada: %s - %s (anos_academicos=%v)", req.Nome, materia.ID, req.AnosAcademicos)

	c.JSON(http.StatusCreated, gin.H{
		"message": "matéria criada com sucesso",
		"data": gin.H{
			"id":              materia.ID,
			"nome":            materia.Nome,
			"type":            materia.Type,
			"anos_academicos": materia.Nivel,
		},
	})
}

// ============================================================================
// GET /academia/materias
// ============================================================================

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

// ============================================================================
// PUT /academia/materias/:id/ativar
// ============================================================================

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

// ============================================================================
// PUT /academia/materias/:id/desativar
// ============================================================================

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
		utils.RespondWithForbiddenError(c, "curso não pertence a esta academia")
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
		"nome":            curso.Nome,
		"type":            curso.Type,
		"anos_academicos": curso.AnosAcademicos,
	})
}