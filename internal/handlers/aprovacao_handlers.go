package handlers

import (
	"log"
	"net/http"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"

	"github.com/gin-gonic/gin"
)

// RegistrarAprovacaoAno - Academia registra aprovação/reprovação
func RegistrarAprovacaoAno(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		CodigoEstudante string  `json:"codigo_estudante" binding:"required"`
		AnoLectivo      string  `json:"ano_lectivo" binding:"required"`
		NivelAtual      string  `json:"nivel_atual" binding:"required"`
		AvancarAno      bool    `json:"avancar_ano"`
		Observacao      *string `json:"observacao"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	// Buscar academia
	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "academia não encontrada"})
		return
	}

	// Buscar estudante
	estudanteProj := getEstudanteProjection(c)
	estudanteDTO, err := estudanteProj.GetByCodigo(req.CodigoEstudante)
	if err != nil || estudanteDTO == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}

	// Verificar vínculo
	if estudanteDTO.CodigoAcademia == nil || *estudanteDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
		c.JSON(http.StatusForbidden, gin.H{"error": "estudante não pertence a esta academia"})
		return
	}

	// Carregar e registrar
	repository := getRepository(c)
	estudanteAgg, err := repository.Load(estudanteDTO.ID, "Estudante")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao carregar estudante"})
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)
	
	err = estudante.RegistrarAprovacaoAno(
		academiaDTO.CodigoAcademia,
		req.AnoLectivo,
		req.NivelAtual,
		req.AvancarAno,
		req.Observacao,
	)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(estudante); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao salvar aprovação"})
		return
	}

	resultado := "REPROVADO"
	if req.AvancarAno {
		resultado = "APROVADO"
	}

	log.Printf("Aprovação registrada: %s - %s - %s", req.CodigoEstudante, req.AnoLectivo, resultado)

	c.JSON(http.StatusCreated, gin.H{
		"message":   "aprovação registrada com sucesso",
		"estudante": req.CodigoEstudante,
		"resultado": resultado,
	})
}

// GetAprovacoesEstudante - Listar aprovações de um estudante
func GetAprovacoesEstudante(c *gin.Context) {
	codigoEstudante := c.Param("codigo")

	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByCodigo(codigoEstudante)
	if err != nil || estudante == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}

	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	// Verificar permissão
	if userType == "estudante" && userID != estudante.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "acesso negado"})
		return
	}

	if userType == "academia" {
		academiaProj := getAcademiaProjection(c)
		academiaDTO, _ := academiaProj.GetByID(userID)
		if estudante.CodigoAcademia == nil || academiaDTO == nil || *estudante.CodigoAcademia != academiaDTO.CodigoAcademia {
			c.JSON(http.StatusForbidden, gin.H{"error": "estudante não pertence a esta academia"})
			return
		}
	}

	aprovacaoProj := getAprovacaoAnoProjection(c)
	aprovacoes, err := aprovacaoProj.GetByEstudante(codigoEstudante)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar aprovações"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"codigo_estudante": codigoEstudante,
		"nome":             estudante.Nome,
		"aprovacoes":       aprovacoes,
		"total":            len(aprovacoes),
	})
}

// GetMinhasAprovacoes - Estudante consulta suas próprias aprovações
func GetMinhasAprovacoes(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByID(userID)
	if err != nil || estudante == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}

	aprovacaoProj := getAprovacaoAnoProjection(c)
	aprovacoes, err := aprovacaoProj.GetByEstudante(estudante.CodigoEstudante)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar aprovações"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"aprovacoes": aprovacoes,
		"total":      len(aprovacoes),
	})
}

// Helper
func getAprovacaoAnoProjection(c *gin.Context) *projections.AprovacaoAnoProjection {
	client := getDbClient(c)
	return projections.NewAprovacaoAnoProjection(client)
}