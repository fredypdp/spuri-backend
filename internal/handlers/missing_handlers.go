package handlers

import (
	"log"
	"net/http"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// InscricaoEscola - estudante solicita inscrição em escola
func InscricaoEscola(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		CodigoAcademia      string  `json:"codigo_academia" binding:"required"`
		AnoEscolarInscricao string  `json:"ano_escolar_inscricao" binding:"required"`
		CursoMedio          *string `json:"curso_medio"`
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

	// Carregar e inscrever
	repository := getRepository(c)
	estudanteAgg, err := repository.Load(userID, "Estudante")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao carregar estudante"})
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)

	err = estudante.SolicitarInscricao(req.CodigoAcademia, "escola", req.AnoEscolarInscricao, req.CursoMedio)
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

// InscricaoUniversidade - estudante solicita inscrição em universidade
func InscricaoUniversidade(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		CodigoAcademia string  `json:"codigo_academia" binding:"required"`
		AnoInscricao   string  `json:"ano_inscricao" binding:"required"`
		Curso          *string `json:"curso" binding:"required"`
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

	repository := getRepository(c)
	estudanteAgg, err := repository.Load(userID, "Estudante")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao carregar estudante"})
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)

	err = estudante.SolicitarInscricao(req.CodigoAcademia, "superior", req.AnoInscricao, req.Curso)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(estudante); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao salvar inscrição"})
		return
	}

	log.Printf("Inscrição universidade criada: %s em %s", estudante.CodigoEstudante, academiaDTO.Nome)

	c.JSON(http.StatusCreated, gin.H{
		"message":  "inscrição criada com sucesso",
		"status":   "espera",
		"academia": academiaDTO.Nome,
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