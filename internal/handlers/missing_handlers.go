package handlers

import (
	"log"
	"net/http"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// 🔥 ATUALIZADO
func InscricaoEscola(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		CodigoAcademia      string     `json:"codigo_academia" binding:"required"`
		AnoEscolarInscricao string     `json:"ano_escolar_inscricao" binding:"required"`
		CursoMedioID        *uuid.UUID `json:"curso_medio_id"` // 🔥 MUDOU
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	// Buscar academia
	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByCodigo(req.CodigoAcademia)
	if err != nil || academiaDTO == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "academia não encontrada"})
		return
	}

	if academiaDTO.Status != "ativo" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "academia não está ativa"})
		return
	}

	// 🔥 VALIDAR CURSO SE FORNECIDO
	if req.CursoMedioID != nil && *req.CursoMedioID != uuid.Nil {
		cursosProj := getCursosProjection(c)
		curso, _ := cursosProj.GetByID(*req.CursoMedioID)
		if curso == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "curso_medio_id não encontrado"})
			return
		}
		if curso.Type != "medio" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "curso deve ser do tipo 'medio'"})
			return
		}
		if curso.CodigoAcademia != academiaDTO.CodigoAcademia {
			c.JSON(http.StatusBadRequest, gin.H{"error": "curso não pertence a esta academia"})
			return
		}
		if curso.Status != "ativo" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "curso está inativo"})
			return
		}
	}

	// Carregar e inscrever
	repository := getRepository(c)
	estudanteAgg, err := repository.Load(userID, "Estudante")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao carregar estudante"})
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)

	// 🔥 MUDOU - passar UUID em vez de string
	err = estudante.SolicitarInscricao(req.CodigoAcademia, "escola", req.AnoEscolarInscricao, req.CursoMedioID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(estudante); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao salvar inscrição"})
		return
	}

	log.Printf("Inscrição escola criada: %s em %s", estudante.CodigoEstudante, academiaDTO.Nome)

	c.JSON(http.StatusCreated, gin.H{
		"message":  "inscrição criada com sucesso",
		"status":   "espera",
		"academia": academiaDTO.Nome,
	})
}

// 🔥 ATUALIZADO
func InscricaoUniversidade(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		CodigoAcademia  string    `json:"codigo_academia" binding:"required"`
		AnoInscricao    string    `json:"ano_inscricao" binding:"required"`
		CursoSuperiorID uuid.UUID `json:"curso_superior_id" binding:"required"` // 🔥 MUDOU
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByCodigo(req.CodigoAcademia)
	if err != nil || academiaDTO == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "academia não encontrada"})
		return
	}

	if academiaDTO.Status != "ativo" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "academia não está ativa"})
		return
	}

	// 🔥 VALIDAR CURSO OBRIGATÓRIO
	cursosProj := getCursosProjection(c)
	curso, _ := cursosProj.GetByID(req.CursoSuperiorID)
	if curso == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "curso_superior_id não encontrado"})
		return
	}
	if curso.Type != "superior" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "curso deve ser do tipo 'superior'"})
		return
	}
	if curso.CodigoAcademia != academiaDTO.CodigoAcademia {
		c.JSON(http.StatusBadRequest, gin.H{"error": "curso não pertence a esta academia"})
		return
	}
	if curso.Status != "ativo" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "curso está inativo"})
		return
	}

	repository := getRepository(c)
	estudanteAgg, err := repository.Load(userID, "Estudante")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao carregar estudante"})
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)

	// 🔥 MUDOU - passar UUID
	err = estudante.SolicitarInscricao(req.CodigoAcademia, "universidade", req.AnoInscricao, &req.CursoSuperiorID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(estudante); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao salvar inscrição"})
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

// BuscarUsuario - admin busca usuário por tipo e ID
func BuscarUsuario(c *gin.Context) {
	tipo := c.Query("tipo")
	id := c.Query("id")

	if tipo == "" || id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tipo e id são obrigatórios"})
		return
	}

	userID, err := uuid.Parse(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID inválido"})
		return
	}

	switch tipo {
	case "estudante":
		estudanteProj := getEstudanteProjection(c)
		estudante, err := estudanteProj.GetByID(userID)
		if err != nil || estudante == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"tipo": "estudante", "dados": estudante})

	case "academia":
		academiaProj := getAcademiaProjection(c)
		academia, err := academiaProj.GetByID(userID)
		if err != nil || academia == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "academia não encontrada"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"tipo": "academia", "dados": academia})

	case "admin":
		adminProj := getAdminProjection(c)
		admin, err := adminProj.GetByID(userID)
		if err != nil || admin == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "admin não encontrado"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"tipo": "admin", "dados": admin})

	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "tipo inválido (estudante, academia, admin)"})
	}
}

// GetAllProjectionStatuses - alias para GetAllProjectionsStatus
func GetAllProjectionStatuses(c *gin.Context) {
	GetAllProjectionsStatus(c)
}