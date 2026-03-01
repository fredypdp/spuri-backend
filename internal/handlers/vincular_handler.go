package handlers

import (
	"fmt"
	"log"
	"net/http"
	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/utils"
	"time"

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

	audit := db.AuditContext{
		UserID:   userID.String(),
		UserType: "estudante",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(estudante, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Estudante vinculado à academia: %s", estudante.CodigoEstudante)
	c.JSON(http.StatusOK, gin.H{
		"message": "vinculado à academia com sucesso",
		"status":  "ativo",
	})
}

// ListarInscricoesAprovadas lista as inscrições aprovadas e ainda não usadas
// do estudante autenticado.
//
// CORREÇÕES:
//   - Coluna 'curso' renomeada para 'curso_id' (migration 004). A query antiga
//     usava SELECT curso que não existe mais e causava erro SQL.
//   - userID era interpolado diretamente no fmt.Sprintf (potencial SQL injection).
//     Agora usa prepared statement com $1.
//   - Struct InscricaoAprovada.CursoID tipado como *uuid.UUID (era *string).
func ListarInscricoesAprovadas(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	client := getDbClient(c)

	type InscricaoAprovada struct {
		ID              uuid.UUID  `json:"id"`
		EstudanteID     uuid.UUID  `json:"estudante_id"`
		CodigoEstudante string     `json:"codigo_estudante"`
		AcademiaID      uuid.UUID  `json:"academia_id"`
		CodigoAcademia  string     `json:"codigo_academia"`
		Tipo            string     `json:"tipo"`
		AnoInscricao    string     `json:"ano_inscricao"`
		CursoID         *uuid.UUID `json:"curso_id,omitempty"` // ✅ era Curso *string com coluna inexistente 'curso'
		Status          string     `json:"status"`
		StatusUsado     bool       `json:"status_usado"`
		CreatedAt       time.Time  `json:"created_at"`
	}

	// ✅ Prepared statement — userID é parâmetro $1, sem interpolação de string
	// ✅ curso_id no lugar de 'curso' (migration 004 renomeou a coluna)
	rows, err := client.DB().Query(`
		SELECT id, estudante_id, codigo_estudante, academia_id, codigo_academia,
		       tipo, ano_inscricao, curso_id, status, status_usado, created_at
		FROM projection_inscricoes
		WHERE estudante_id = $1
		  AND status = 'aprovado'
		  AND status_usado = FALSE
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	defer rows.Close()

	var inscricoes []InscricaoAprovada
	for rows.Next() {
		var insc InscricaoAprovada
		var cursoID uuid.UUID
		var cursoIDNull *uuid.UUID

		// curso_id pode ser NULL na tabela
		var rawCursoID *string
		if err := rows.Scan(
			&insc.ID, &insc.EstudanteID, &insc.CodigoEstudante, &insc.AcademiaID,
			&insc.CodigoAcademia, &insc.Tipo, &insc.AnoInscricao, &rawCursoID,
			&insc.Status, &insc.StatusUsado, &insc.CreatedAt,
		); err != nil {
			log.Printf("[WARN] ListarInscricoesAprovadas scan error: %v", err)
			continue
		}

		if rawCursoID != nil {
			if parsed, err := uuid.Parse(*rawCursoID); err == nil {
				cursoIDNull = &parsed
			}
		}
		_ = cursoID
		insc.CursoID = cursoIDNull
		inscricoes = append(inscricoes, insc)
	}

	if err := rows.Err(); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"inscricoes": inscricoes,
		"total":      len(inscricoes),
		"mensagem":   "Use POST /estudante/vincular-academia com inscricao_id para se vincular",
	})
}

// AtualizarStatusEscolarFundamentalHandler — PUT /estudante/status-escolar-fundamental
func AtualizarStatusEscolarFundamentalHandler(c *gin.Context) {
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
	if err := estudante.AtualizarStatusEscolarFundamental(req.NovoStatus); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(estudante); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Status escolar fundamental atualizado: %s - %s", estudante.CodigoEstudante, req.NovoStatus)
	c.JSON(http.StatusOK, gin.H{
		"message":     "status_escolar_fundamental atualizado com sucesso",
		"novo_status": req.NovoStatus,
	})
}

// AtualizarStatusEscolarMedioHandler — PUT /estudante/status-escolar-medio
func AtualizarStatusEscolarMedioHandler(c *gin.Context) {
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
	if err := estudante.AtualizarStatusEscolarMedio(req.NovoStatus); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(estudante); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Status escolar médio atualizado: %s - %s", estudante.CodigoEstudante, req.NovoStatus)
	c.JSON(http.StatusOK, gin.H{
		"message":     "status_escolar_medio atualizado com sucesso",
		"novo_status": req.NovoStatus,
	})
}

// AtualizarStatusSuperior — PUT /estudante/status-superior
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