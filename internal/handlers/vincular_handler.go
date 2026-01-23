package handlers

import (
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

	// Carregar agregado estudante
	repository := getRepository(c)
	estudanteAgg, err := repository.Load(userID, "Estudante")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao carregar estudante"})
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)

	// Executar comando VincularAcademia
	if err := estudante.VincularAcademia(inscricaoID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Salvar eventos
	if err := repository.Save(estudante); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao vincular academia"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "vinculado à academia com sucesso",
		"status":  "ativo",
	})
}

// ✅ CORRIGIDO: Query() + loop manual
func ListarInscricoesAprovadas(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	client := getDbClient(c)

	query := `
		SELECT 
			id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso, status, status_usado, created_at
		FROM projection_inscricoes
		WHERE estudante_id = $1 AND status = 'aprovado' AND status_usado = FALSE
		ORDER BY created_at DESC
	`

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

	rows, err := client.DB().Query(query, userID)
	if err != nil {
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
			continue
		}
		inscricoes = append(inscricoes, insc)
	}

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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao carregar estudante"})
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)

	if err := estudante.AtualizarStatusEscolar(req.NovoStatus); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(estudante); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao atualizar status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "status escolar atualizado",
		"novo_status": req.NovoStatus,
	})
}

// AtualizarStatusSuperior atualiza status superior do estudante
// PUT /estudante/status-superior
func AtualizarStatusSuperior(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao carregar estudante"})
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)

	if err := estudante.AtualizarStatusSuperior(req.NovoStatus); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(estudante); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao atualizar status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "status superior atualizado",
		"novo_status": req.NovoStatus,
	})
}