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

func VincularAcademia(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		InscricaoID string `json:"inscricao_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("inscricao_id é obrigatório"))
		return
	}

	inscricaoID, err := uuid.Parse(req.InscricaoID)
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("inscricao_id inválido"))
		return
	}

	repository := getRepository(c)
	estudanteAgg, err := repository.Load(userID, "Estudante")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)
	if err := estudante.VincularAcademia(inscricaoID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(estudante); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Estudante vinculado à academia: %s", estudante.CodigoEstudante)
	c.JSON(http.StatusOK, gin.H{
		"message": "vinculado à academia com sucesso",
		"status":  "ativo",
	})
}

func ListarInscricoesAprovadas(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

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
		utils.RespondWithInternalError(c, err)
		return
	}
	defer rows.Close()

	var inscricoes []InscricaoAprovada
	for rows.Next() {
		var insc InscricaoAprovada
		if err := rows.Scan(&insc.ID, &insc.EstudanteID, &insc.CodigoEstudante, &insc.AcademiaID,
			&insc.CodigoAcademia, &insc.Tipo, &insc.AnoInscricao, &insc.Curso, &insc.Status,
			&insc.StatusUsado, &insc.CreatedAt); err != nil {
			continue
		}
		inscricoes = append(inscricoes, insc)
	}

	c.JSON(http.StatusOK, gin.H{
		"inscricoes": inscricoes,
		"total":      len(inscricoes),
		"mensagem":   "Use POST /estudante/vincular-academia com inscricao_id para se vincular",
	})
}

func AtualizarStatusEscolar(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		NovoStatus string `json:"novo_status" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("novo_status é obrigatório"))
		return
	}

	repository := getRepository(c)
	estudanteAgg, err := repository.Load(userID, "Estudante")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)
	if err := estudante.AtualizarStatusEscolar(req.NovoStatus); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(estudante); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Status escolar atualizado: %s - %s", estudante.CodigoEstudante, req.NovoStatus)
	c.JSON(http.StatusOK, gin.H{
		"message":     "status escolar atualizado com sucesso",
		"novo_status": req.NovoStatus,
	})
}

func AtualizarStatusSuperior(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		NovoStatus string `json:"novo_status" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("novo_status é obrigatório"))
		return
	}

	repository := getRepository(c)
	estudanteAgg, err := repository.Load(userID, "Estudante")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)
	if err := estudante.AtualizarStatusSuperior(req.NovoStatus); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(estudante); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Status superior atualizado: %s - %s", estudante.CodigoEstudante, req.NovoStatus)
	c.JSON(http.StatusOK, gin.H{
		"message":     "status superior atualizado com sucesso",
		"novo_status": req.NovoStatus,
	})
}