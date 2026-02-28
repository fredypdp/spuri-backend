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
// POST /academia/materias
// ============================================================================

func CriarMateria(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		Nome           string     `json:"nome"            binding:"required"`
		Type           string     `json:"type"            binding:"required"`
		AnosAcademicos []string   `json:"anos_academicos"`
		CursoID        *uuid.UUID `json:"curso_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("dados obrigatorios: nome e tipo"))
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	if academiaDTO.Status != "ativo" {
		utils.RespondWithForbiddenError(c, "Academia inativa nao pode criar materias")
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
			utils.RespondWithValidationError(c, fmt.Errorf("curso inativo nao pode ter materias"))
			return
		}

		if cursoDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
			utils.RespondWithForbiddenError(c, "Curso nao pertence a esta academia")
			return
		}

		// Para superior: garantir que o curso tem periodos definidos
		if req.Type == "superior" && len(cursoDTO.Periodos) == 0 {
			utils.RespondWithValidationError(c, fmt.Errorf(
				"o curso '%s' nao possui periodos definidos. Atualize o curso antes de criar materias superiores",
				cursoDTO.Nome,
			))
			return
		}
	}

	repository := getRepository(c)
	materia := aggregates.NewMateriaDisciplinar()

	// Assinatura original: Criar(nome, tipo, anosAcademicos, codigoAcademia, cursoID *uuid.UUID)
	if err := materia.Criar(req.Nome, req.Type, req.AnosAcademicos, academiaDTO.CodigoAcademia, req.CursoID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(materia); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Materia criada: %s - %s", req.Nome, materia.ID)

	c.JSON(http.StatusCreated, gin.H{
		"message": "materia criada com sucesso",
		"data": gin.H{
			"id":   materia.ID,
			"nome": materia.Nome,
			"type": materia.Type,
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
		utils.RespondWithValidationError(c, fmt.Errorf("ID de materia invalido"))
		return
	}

	materiasProj := getMateriasProjection(c)
	materiaDTO, err := materiasProj.GetByID(materiaID)
	if err != nil || materiaDTO == nil {
		utils.RespondWithNotFoundError(c, "materia")
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, _ := academiaProj.GetByID(userID)
	if academiaDTO == nil || academiaDTO.CodigoAcademia != materiaDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "Materia nao pertence a esta academia")
		return
	}

	repository := getRepository(c)
	materiaAgg, err := repository.Load(materiaID, "MateriaDisciplinar")
	if err != nil {
		utils.RespondWithNotFoundError(c, "materia")
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

	log.Printf("Materia ativada: %s", materia.Nome)
	c.JSON(http.StatusOK, gin.H{"message": "materia ativada com sucesso", "nome": materia.Nome})
}

// ============================================================================
// PUT /academia/materias/:id/desativar
// ============================================================================

func DesativarMateria(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	materiaID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ID de materia invalido"))
		return
	}

	materiasProj := getMateriasProjection(c)
	materiaDTO, err := materiasProj.GetByID(materiaID)
	if err != nil || materiaDTO == nil {
		utils.RespondWithNotFoundError(c, "materia")
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, _ := academiaProj.GetByID(userID)
	if academiaDTO == nil || academiaDTO.CodigoAcademia != materiaDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "Materia nao pertence a esta academia")
		return
	}

	repository := getRepository(c)
	materiaAgg, err := repository.Load(materiaID, "MateriaDisciplinar")
	if err != nil {
		utils.RespondWithNotFoundError(c, "materia")
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

	log.Printf("Materia desativada: %s", materia.Nome)

	c.JSON(http.StatusOK, gin.H{
		"message": "materia desativada com sucesso",
		"nome":    materia.Nome,
	})
}

// ============================================================================
// PUT /academia/materias/:id
// ============================================================================

// AtualizarDadosMateria atualiza nome e/ou tipo de uma matéria.
//
// Nota: a atualização de anos_academicos não é suportada por este comando —
// alterar os anos implicaria em revalidar todas as notas associadas, o que
// exige uma operação de maior impacto. Para mudar os anos, desative a matéria
// e crie uma nova com os anos corretos.
func AtualizarDadosMateria(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	materiaID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ID de matéria inválido"))
		return
	}

	var req struct {
		Nome *string `json:"nome"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("dados inválidos"))
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
	if err := materia.AtualizarDados(req.Nome); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(materia); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Matéria atualizada: %s", materia.Nome)
	c.JSON(http.StatusOK, gin.H{
		"message": "matéria atualizada com sucesso",
		"nome":    materia.Nome,
	})
}