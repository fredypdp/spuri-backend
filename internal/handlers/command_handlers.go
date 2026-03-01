package handlers

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/utils"
)

// ============================================================================
// POST /academia/faltas-aluno
// ============================================================================

func RegistrarFaltas(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		CodigoEstudante      string  `json:"codigo_estudante"       binding:"required"`
		AnoLectivo           string  `json:"ano_lectivo"            binding:"required"`
		// ano_academico REMOVIDO — inferido pelo back end (mesma lógica das notas)
		Data                 string  `json:"data"                   binding:"required"`
		MateriaDisciplinarID string  `json:"materia_disciplinar_id" binding:"required"`
		Quantidade           int     `json:"quantidade"             binding:"required,min=1"`
		Observacao           *string `json:"observacao"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf(
			"dados obrigatórios: codigo_estudante, ano_lectivo, data, materia_disciplinar_id e quantidade",
		))
		return
	}

	data, err := time.Parse("2006-01-02", req.Data)
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("formato de data inválido. Use YYYY-MM-DD"))
		return
	}

	// ── Academia ───────────────────────────────────────────────────────────
	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	// ── Estudante ──────────────────────────────────────────────────────────
	estudanteProj := getEstudanteProjection(c)
	estudanteDTO, err := estudanteProj.GetByCodigo(req.CodigoEstudante)
	if err != nil || estudanteDTO == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	if estudanteDTO.CodigoAcademia == nil || *estudanteDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "Estudante não pertence a esta academia")
		return
	}

	// ── Matéria ────────────────────────────────────────────────────────────
	materiaID, err := uuid.Parse(req.MateriaDisciplinarID)
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("materia_disciplinar_id inválido"))
		return
	}

	materiasProj := getMateriasProjection(c)
	materiaDTO, _ := materiasProj.GetByID(materiaID)
	if materiaDTO == nil || materiaDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "Matéria não pertence a esta academia")
		return
	}

	// ── Inferir AnoAcademico (mesma regra das notas — Atualização 2) ──────
	anoAcademico, err := inferirAnoAcademicoParaNota(estudanteDTO.AnoEscolar, materiaDTO.AnosAcademicos, materiaDTO.Nome)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	// ── Aggregate e comando ────────────────────────────────────────────────
	repository := getRepository(c)
	estudanteAgg, err := repository.Load(estudanteDTO.ID, "Estudante")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)

	err = estudante.RegistrarFalta(
		academiaDTO.CodigoAcademia,
		req.AnoLectivo,
		anoAcademico,
		data,
		materiaID,
		req.Quantidade,
		req.Observacao,
	)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   userID.String(),
		UserType: "academia",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(estudante, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Faltas registradas: %s - %d em %s (ano_academico=%s)",
		req.CodigoEstudante, req.Quantidade, materiaDTO.Nome, anoAcademico)

	c.JSON(http.StatusCreated, gin.H{
		"message":       "faltas registradas com sucesso",
		"estudante":     req.CodigoEstudante,
		"materia":       materiaDTO.Nome,
		"quantidade":    req.Quantidade,
		"ano_academico": anoAcademico,
	})
}

// ============================================================================
// PUT /academia/inscricao/:id/aprovar
// ============================================================================

// AprovarInscricao aprova uma inscrição pendente de um estudante.
//
// CORREÇÃO: a busca da inscrição pendente agora usa prepared statement ($1, $2, $3)
// em vez de fmt.Sprintf com SafeString. Isso elimina o risco de SQL injection
// e alinha com o padrão do restante do projeto.
func AprovarInscricao(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		CodigoEstudante string     `json:"codigo_estudante" binding:"required"`
		Tipo            string     `json:"tipo"             binding:"required"`
		AnoInscricao    string     `json:"ano_inscricao"    binding:"required"`
		CursoID         *uuid.UUID `json:"curso_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("dados obrigatórios: codigo_estudante, tipo e ano_inscricao"))
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	estudanteProj := getEstudanteProjection(c)
	estudanteDTO, err := estudanteProj.GetByCodigo(req.CodigoEstudante)
	if err != nil || estudanteDTO == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	if req.CursoID != nil && *req.CursoID != uuid.Nil {
		cursosProj := getCursosProjection(c)
		curso, _ := cursosProj.GetByID(*req.CursoID)
		if curso == nil {
			utils.RespondWithNotFoundError(c, "curso")
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

	client := getDbClient(c)

	// ✅ Prepared statement — $1, $2, $3 em vez de fmt.Sprintf + SafeString
	var inscricaoID uuid.UUID
	err = client.DB().QueryRow(`
		SELECT id FROM projection_inscricoes
		WHERE codigo_estudante = $1
		  AND codigo_academia = $2
		  AND tipo = $3
		  AND status = 'espera'
		LIMIT 1
	`, req.CodigoEstudante, academiaDTO.CodigoAcademia, req.Tipo).Scan(&inscricaoID)
	if err != nil {
		utils.RespondWithNotFoundError(c, "inscrição pendente")
		return
	}

	repository := getRepository(c)
	academiaAgg, err := repository.Load(academiaDTO.ID, "Academia")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	academia := academiaAgg.(*aggregates.Academia)
	if err := academia.AprovarInscricao(estudanteDTO.ID, inscricaoID, req.Tipo, req.AnoInscricao, req.CursoID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   userID.String(),
		UserType: "academia",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(academia, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Inscrição aprovada: %s na academia %s", req.CodigoEstudante, academiaDTO.CodigoAcademia)

	c.JSON(http.StatusOK, gin.H{
		"message":   "inscrição aprovada com sucesso",
		"estudante": req.CodigoEstudante,
	})
}

// ============================================================================
// PUT /academia/inscricao/:id/reprovar
// ============================================================================

// ReprovarInscricao reprova uma inscrição pendente de um estudante.
//
// CORREÇÃO: a busca da inscrição pendente agora usa prepared statement ($1, $2, $3)
// em vez de fmt.Sprintf com SafeString. Isso elimina o risco de SQL injection
// e alinha com o padrão do restante do projeto.
func ReprovarInscricao(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		CodigoEstudante string `json:"codigo_estudante" binding:"required"`
		Tipo            string `json:"tipo"             binding:"required"`
		Motivo          string `json:"motivo"           binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("dados obrigatórios: codigo_estudante, tipo e motivo"))
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	estudanteProj := getEstudanteProjection(c)
	estudanteDTO, err := estudanteProj.GetByCodigo(req.CodigoEstudante)
	if err != nil || estudanteDTO == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	client := getDbClient(c)

	// ✅ Prepared statement — $1, $2, $3 em vez de fmt.Sprintf + SafeString
	var inscricaoID uuid.UUID
	err = client.DB().QueryRow(`
		SELECT id FROM projection_inscricoes
		WHERE codigo_estudante = $1
		  AND codigo_academia = $2
		  AND tipo = $3
		  AND status = 'espera'
		LIMIT 1
	`, req.CodigoEstudante, academiaDTO.CodigoAcademia, req.Tipo).Scan(&inscricaoID)
	if err != nil {
		utils.RespondWithNotFoundError(c, "inscrição pendente")
		return
	}

	repository := getRepository(c)
	academiaAgg, err := repository.Load(academiaDTO.ID, "Academia")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	academia := academiaAgg.(*aggregates.Academia)
	if err := academia.ReprovarInscricao(estudanteDTO.ID, inscricaoID, req.Motivo); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   userID.String(),
		UserType: "academia",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(academia, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Inscrição reprovada: %s na academia %s", req.CodigoEstudante, academiaDTO.CodigoAcademia)

	c.JSON(http.StatusOK, gin.H{
		"message":   "inscrição reprovada com sucesso",
		"estudante": req.CodigoEstudante,
	})
}