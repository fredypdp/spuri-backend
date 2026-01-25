package handlers

import (
	"fmt"
	"log"
	"net/http"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func VincularAcademia(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	log.Printf("[DEBUG] VincularAcademia - userID: %s", userID)

	var req struct {
		InscricaoID string `json:"inscricao_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "inscricao_id é obrigatório"})
		return
	}

	inscricaoID, err := uuid.Parse(req.InscricaoID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "inscricao_id inválido"})
		return
	}

	repository := getRepository(c)
	estudanteAgg, err := repository.Load(userID, "Estudante")
	if err != nil {
		log.Printf("[DEBUG] Erro: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao carregar estudante"})
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)
	if err := estudante.VincularAcademia(inscricaoID); err != nil {
		log.Printf("[DEBUG] Erro: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(estudante); err != nil {
		log.Printf("[DEBUG] Erro ao salvar: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao vincular"})
		return
	}

	log.Printf("[DEBUG] Sucesso")
	c.JSON(http.StatusOK, gin.H{"message": "vinculado à academia", "status": "ativo"})
}

func ListarInscricoesAprovadas(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	log.Printf("[DEBUG] ListarInscricoesAprovadas - userID: %s", userID)

	client := getDbClient(c)
	query := fmt.Sprintf(`
		SELECT id, estudante_id, codigo_estudante, academia_id, codigo_academia,
		       tipo, ano_inscricao, curso, status, status_usado, created_at
		FROM projection_inscricoes
		WHERE estudante_id = '%s' AND status = 'aprovado' AND status_usado = FALSE
		ORDER BY created_at DESC
	`, userID)

	type InscricaoAprovada struct {
		ID              uuid.UUID `json:"id"`
		EstudanteID     uuid.UUID `json:"estudante_id"`
		CodigoEstudante string    `json:"codigo_estudante"`
		AcademiaID      uuid.UUID `json:"academia_id"`
		CodigoAcademia  string    `json:"codigo_academia"`
		Tipo            string    `json:"tipo"`
		AnoInscricao    string    `json:"ano_inscricao"`
		Curso           *string   `json:"curso,omitempty"`
		Status          string    `json:"status"`
		StatusUsado     bool      `json:"status_usado"`
		CreatedAt       string    `json:"created_at"`
	}

	rows, err := client.DB().Query(query)
	if err != nil {
		log.Printf("[DEBUG] Erro: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar inscrições"})
		return
	}
	defer rows.Close()

	var inscricoes []InscricaoAprovada
	for rows.Next() {
		var insc InscricaoAprovada
		if err := rows.Scan(&insc.ID, &insc.EstudanteID, &insc.CodigoEstudante, &insc.AcademiaID,
			&insc.CodigoAcademia, &insc.Tipo, &insc.AnoInscricao, &insc.Curso, &insc.Status,
			&insc.StatusUsado, &insc.CreatedAt); err != nil {
			log.Printf("[DEBUG] Erro scan: %v", err)
			continue
		}
		inscricoes = append(inscricoes, insc)
	}

	log.Printf("[DEBUG] Total: %d", len(inscricoes))
	c.JSON(http.StatusOK, gin.H{
		"inscricoes": inscricoes,
		"total":      len(inscricoes),
		"mensagem":   "Use POST /estudante/vincular-academia com inscricao_id",
	})
}

func AtualizarStatusEscolar(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	log.Printf("[DEBUG] AtualizarStatusEscolar - userID: %s", userID)

	var req struct {
		NovoStatus string `json:"novo_status" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "novo_status é obrigatório"})
		return
	}

	repository := getRepository(c)
	estudanteAgg, err := repository.Load(userID, "Estudante")
	if err != nil {
		log.Printf("[DEBUG] Erro: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao carregar estudante"})
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)
	if err := estudante.AtualizarStatusEscolar(req.NovoStatus); err != nil {
		log.Printf("[DEBUG] Erro: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(estudante); err != nil {
		log.Printf("[DEBUG] Erro ao salvar: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao atualizar"})
		return
	}

	log.Printf("[DEBUG] Sucesso: %s", req.NovoStatus)
	c.JSON(http.StatusOK, gin.H{"message": "status escolar atualizado", "novo_status": req.NovoStatus})
}

func AtualizarStatusSuperior(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	log.Printf("[DEBUG] AtualizarStatusSuperior - userID: %s", userID)

	var req struct {
		NovoStatus string `json:"novo_status" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "novo_status é obrigatório"})
		return
	}

	repository := getRepository(c)
	estudanteAgg, err := repository.Load(userID, "Estudante")
	if err != nil {
		log.Printf("[DEBUG] Erro: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao carregar estudante"})
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)
	if err := estudante.AtualizarStatusSuperior(req.NovoStatus); err != nil {
		log.Printf("[DEBUG] Erro: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(estudante); err != nil {
		log.Printf("[DEBUG] Erro ao salvar: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao atualizar"})
		return
	}

	log.Printf("[DEBUG] Sucesso: %s", req.NovoStatus)
	c.JSON(http.StatusOK, gin.H{"message": "status superior atualizado", "novo_status": req.NovoStatus})
}