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

func InscricaoEscola(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		CodigoAcademia      string     `json:"codigo_academia" binding:"required"`
		AnoEscolarInscricao string     `json:"ano_escolar_inscricao" binding:"required"`
		CursoMedioID        *uuid.UUID `json:"curso_medio_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("codigo_academia e ano_escolar_inscricao são obrigatórios"))
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByCodigo(req.CodigoAcademia)
	if err != nil || academiaDTO == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	if academiaDTO.Status != "ativo" {
		utils.RespondWithValidationError(c, fmt.Errorf("academia não está ativa"))
		return
	}

	if req.CursoMedioID != nil && *req.CursoMedioID != uuid.Nil {
		cursosProj := getCursosProjection(c)
		curso, _ := cursosProj.GetByID(*req.CursoMedioID)
		if curso == nil {
			utils.RespondWithNotFoundError(c, "curso")
			return
		}
		if curso.Type != "medio" {
			utils.RespondWithValidationError(c, fmt.Errorf("curso deve ser do tipo 'medio'"))
			return
		}
		if curso.CodigoAcademia != academiaDTO.CodigoAcademia {
			utils.RespondWithForbiddenError(c, "Curso não pertence a esta academia")
			return
		}
		if curso.Status != "ativo" {
			utils.RespondWithValidationError(c, fmt.Errorf("curso está inativo"))
			return
		}
	}

	repository := getRepository(c)
	estudanteAgg, err := repository.Load(userID, "Estudante")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)

	err = estudante.SolicitarInscricao(req.CodigoAcademia, "escola", req.AnoEscolarInscricao, req.CursoMedioID)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(estudante); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Inscrição escola criada: %s em %s", estudante.CodigoEstudante, academiaDTO.Nome)

	c.JSON(http.StatusCreated, gin.H{
		"message":  "inscrição criada com sucesso",
		"status":   "espera",
		"academia": academiaDTO.Nome,
	})
}

func InscricaoUniversidade(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		CodigoAcademia  string    `json:"codigo_academia" binding:"required"`
		AnoInscricao    string    `json:"ano_inscricao" binding:"required"`
		CursoSuperiorID uuid.UUID `json:"curso_superior_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("codigo_academia, ano_inscricao e curso_superior_id são obrigatórios"))
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByCodigo(req.CodigoAcademia)
	if err != nil || academiaDTO == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	if academiaDTO.Status != "ativo" {
		utils.RespondWithValidationError(c, fmt.Errorf("academia não está ativa"))
		return
	}

	cursosProj := getCursosProjection(c)
	curso, _ := cursosProj.GetByID(req.CursoSuperiorID)
	if curso == nil {
		utils.RespondWithNotFoundError(c, "curso")
		return
	}
	if curso.Type != "superior" {
		utils.RespondWithValidationError(c, fmt.Errorf("curso deve ser do tipo 'superior'"))
		return
	}
	if curso.CodigoAcademia != academiaDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "Curso não pertence a esta academia")
		return
	}
	if curso.Status != "ativo" {
		utils.RespondWithValidationError(c, fmt.Errorf("curso está inativo"))
		return
	}

	repository := getRepository(c)
	estudanteAgg, err := repository.Load(userID, "Estudante")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)

	err = estudante.SolicitarInscricao(req.CodigoAcademia, "universidade", req.AnoInscricao, &req.CursoSuperiorID)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(estudante); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Inscrição universidade criada: %s em %s - Curso: %s", estudante.CodigoEstudante, academiaDTO.Nome, curso.Nome)

	c.JSON(http.StatusCreated, gin.H{
		"message":  "inscrição criada com sucesso",
		"status":   "espera",
		"academia": academiaDTO.Nome,
		"curso":    curso.Nome,
	})
}

func BuscarUsuario(c *gin.Context) {
	tipo := c.Query("tipo")
	id := c.Query("id")

	if tipo == "" || id == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("parâmetros 'tipo' e 'id' são obrigatórios"))
		return
	}

	userID, err := uuid.Parse(id)
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ID inválido"))
		return
	}

	switch tipo {
	case "estudante":
		estudanteProj := getEstudanteProjection(c)
		estudante, err := estudanteProj.GetByID(userID)
		if err != nil || estudante == nil {
			utils.RespondWithNotFoundError(c, "estudante")
			return
		}
		c.JSON(http.StatusOK, gin.H{"tipo": "estudante", "dados": estudante})

	case "academia":
		academiaProj := getAcademiaProjection(c)
		academia, err := academiaProj.GetByID(userID)
		if err != nil || academia == nil {
			utils.RespondWithNotFoundError(c, "academia")
			return
		}
		c.JSON(http.StatusOK, gin.H{"tipo": "academia", "dados": academia})

	case "admin":
		adminProj := getAdminProjection(c)
		admin, err := adminProj.GetByID(userID)
		if err != nil || admin == nil {
			utils.RespondWithNotFoundError(c, "administrador")
			return
		}
		c.JSON(http.StatusOK, gin.H{"tipo": "admin", "dados": admin})

	default:
		utils.RespondWithValidationError(c, fmt.Errorf("tipo inválido. Use: estudante, academia ou admin"))
	}
}

func GetAllProjectionStatuses(c *gin.Context) {
	GetAllProjectionsStatus(c)
}