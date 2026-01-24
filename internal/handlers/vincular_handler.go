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

// VincularAcademia permite estudante usar inscrição aprovada para entrar na academia
// POST /estudante/vincular-academia
func VincularAcademia(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	log.Printf("[DEBUG] VincularAcademia - userID: %s", userID)

	var req struct {
		InscricaoID string `json:"inscricao_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[DEBUG] Erro ao fazer bind do JSON: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "inscricao_id é obrigatório"})
		return
	}

	inscricaoID, err := uuid.Parse(req.InscricaoID)
	if err != nil {
		log.Printf("[DEBUG] InscricaoID inválido: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "inscricao_id inválido"})
		return
	}

	log.Printf("[DEBUG] InscricaoID: %s", inscricaoID)

	// Carregar agregado estudante
	repository := getRepository(c)
	estudanteAgg, err := repository.Load(userID, "Estudante")
	if err != nil {
		log.Printf("[DEBUG] Erro ao carregar estudante: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao carregar estudante"})
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)
	log.Printf("[DEBUG] Estudante carregado: %s", estudante.ID)

	// Executar comando VincularAcademia
	if err := estudante.VincularAcademia(inscricaoID); err != nil {
		log.Printf("[DEBUG] Erro ao vincular academia: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Salvar eventos
	if err := repository.Save(estudante); err != nil {
		log.Printf("[DEBUG] Erro ao salvar eventos: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao vincular academia"})
		return
	}

	log.Printf("[DEBUG] Academia vinculada com sucesso")
	c.JSON(http.StatusOK, gin.H{
		"message": "vinculado à academia com sucesso",
		"status":  "ativo",
	})
}

// ListarInscricoesAprovadas lista inscrições aprovadas do estudante
func ListarInscricoesAprovadas(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	log.Printf("[DEBUG] ListarInscricoesAprovadas - userID: %s", userID)

	client := getDbClient(c)

	query := fmt.Sprintf(`
		SELECT 
			id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso, status, status_usado, created_at
		FROM projection_inscricoes
		WHERE estudante_id = '%s' AND status = 'aprovado' AND status_usado = FALSE
		ORDER BY created_at DESC
	`, userID)

	log.Printf("[DEBUG] Query: %s", query)

	type InscricaoAprovada struct {
		ID              uuid.UUID `db:"id" json:"id"`
		EstudanteID     uuid.UUID `db:"estudante_id" json:"estudante_id"`
		CodigoEstudante string    `db:"codigo_estudante" json:"codigo_estudante"`
		AcademiaID      uuid.UUID `db:"academia_id" json:"academia_id"`
		CodigoAcademia  string    `db:"codigo_academia" json:"codigo_academia"`
		Tipo            string    `db:"tipo" json:"tipo"`
		AnoInscricao    string    `db:"ano_inscricao" json:"ano_inscricao"`
		Curso           *string   `db:"curso" json:"curso,omitempty"`
		Status          string    `db:"status" json:"status"`
		StatusUsado     bool      `db:"status_usado" json:"status_usado"`
		CreatedAt       string    `db:"created_at" json:"created_at"`
	}

	rows, err := client.DB().Query(query)
	if err != nil {
		log.Printf("[DEBUG] Erro ao executar query: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar inscrições"})
		return
	}
	defer rows.Close()

	var inscricoes []InscricaoAprovada
	for rows.Next() {
		var insc InscricaoAprovada
		err := rows.Scan(&insc.ID, &insc.EstudanteID, &insc.CodigoEstudante, &insc.AcademiaID,
			&insc.CodigoAcademia, &insc.Tipo, &insc.AnoInscricao, &insc.Curso, &insc.Status,
			&insc.StatusUsado, &insc.CreatedAt)
		if err != nil {
			log.Printf("[DEBUG] Erro ao fazer scan da linha: %v", err)
			continue
		}
		inscricoes = append(inscricoes, insc)
	}

	log.Printf("[DEBUG] Total de inscrições encontradas: %d", len(inscricoes))

	c.JSON(http.StatusOK, gin.H{
		"inscricoes": inscricoes,
		"total":      len(inscricoes),
		"mensagem":   "Use POST /estudante/vincular-academia com o inscricao_id para entrar na academia",
	})
}

// AtualizarStatusEscolar atualiza status escolar do estudante
// PUT /estudante/status-escolar
func AtualizarStatusEscolar(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	log.Printf("[DEBUG] AtualizarStatusEscolar - userID: %s", userID)

	var req struct {
		NovoStatus string `json:"novo_status" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[DEBUG] Erro ao fazer bind do JSON: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "novo_status é obrigatório"})
		return
	}

	log.Printf("[DEBUG] Novo status: %s", req.NovoStatus)

	repository := getRepository(c)
	estudanteAgg, err := repository.Load(userID, "Estudante")
	if err != nil {
		log.Printf("[DEBUG] Erro ao carregar estudante: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao carregar estudante"})
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)

	if err := estudante.AtualizarStatusEscolar(req.NovoStatus); err != nil {
		log.Printf("[DEBUG] Erro ao atualizar status escolar: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(estudante); err != nil {
		log.Printf("[DEBUG] Erro ao salvar eventos: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao atualizar status"})
		return
	}

	log.Printf("[DEBUG] Status escolar atualizado com sucesso: %s", req.NovoStatus)
	c.JSON(http.StatusOK, gin.H{
		"message":     "status escolar atualizado",
		"novo_status": req.NovoStatus,
	})
}

// AtualizarStatusSuperior atualiza status superior do estudante
// PUT /estudante/status-superior
func AtualizarStatusSuperior(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	log.Printf("[DEBUG] AtualizarStatusSuperior - userID: %s", userID)

	var req struct {
		NovoStatus string `json:"novo_status" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[DEBUG] Erro ao fazer bind do JSON: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "novo_status é obrigatório"})
		return
	}

	log.Printf("[DEBUG] Novo status: %s", req.NovoStatus)

	repository := getRepository(c)
	estudanteAgg, err := repository.Load(userID, "Estudante")
	if err != nil {
		log.Printf("[DEBUG] Erro ao carregar estudante: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao carregar estudante"})
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)

	if err := estudante.AtualizarStatusSuperior(req.NovoStatus); err != nil {
		log.Printf("[DEBUG] Erro ao atualizar status superior: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(estudante); err != nil {
		log.Printf("[DEBUG] Erro ao salvar eventos: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao atualizar status"})
		return
	}

	log.Printf("[DEBUG] Status superior atualizado com sucesso: %s", req.NovoStatus)
	c.JSON(http.StatusOK, gin.H{
		"message":     "status superior atualizado",
		"novo_status": req.NovoStatus,
	})
}