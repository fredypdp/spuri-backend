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
		utils.RespondWithValidationError(c, fmt.Errorf("formato de data inválido. Use AAAA-MM-DD"))
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
	if estudanteDTO.CodigoAcademia == nil || *estudanteDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "estudante não pertence a esta academia")
		return
	}

	materiaID, err := uuid.Parse(req.MateriaDisciplinarID)
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("materia_disciplinar_id inválido"))
		return
	}
	materiasProj := getMateriasProjection(c)
	materiaDTO, _ := materiasProj.GetByID(materiaID)
	if materiaDTO == nil || materiaDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "materia não pertence a esta academia")
		return
	}

	// Inferir anoAcademico
	anoAcademico := ""
	if estudanteDTO.AnoEscolar != nil && *estudanteDTO.AnoEscolar != "" {
		anoAcademico = *estudanteDTO.AnoEscolar
	} else if len(materiaDTO.AnosAcademicos) > 0 {
		anoAcademico = materiaDTO.AnosAcademicos[0]
	}

	repository := getRepository(c)
	estudanteAgg, err := repository.Load(estudanteDTO.ID, "Estudante")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	estudante, ok := estudanteAgg.(*aggregates.Estudante)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("tipo de aggregate inesperado"))
		return
	}

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
// PUT /academia/atualizar-falta
// ============================================================================

func AtualizarFalta(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		ID                   string  `json:"id"                     binding:"required"` // ID da linha em projection_faltas
		Data                 *string `json:"data"`
		MateriaDisciplinarID *string `json:"materia_disciplinar_id"`
		Quantidade           *int    `json:"quantidade"`
		Observacao           *string `json:"observacao"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("campo obrigatório: id"))
		return
	}

	if req.Data == nil && req.MateriaDisciplinarID == nil && req.Quantidade == nil && req.Observacao == nil {
		utils.RespondWithValidationError(c, fmt.Errorf(
			"ao menos um campo deve ser fornecido: data, materia_disciplinar_id, quantidade ou observacao",
		))
		return
	}

	if req.Quantidade != nil && *req.Quantidade <= 0 {
		utils.RespondWithValidationError(c, fmt.Errorf("quantidade deve ser maior que zero"))
		return
	}

	// Converter data se fornecida
	var dataPtr *time.Time
	if req.Data != nil {
		parsed, err := time.Parse("2006-01-02", *req.Data)
		if err != nil {
			utils.RespondWithValidationError(c, fmt.Errorf("formato de data inválido. Use AAAA-MM-DD"))
			return
		}
		dataPtr = &parsed
	}

	// Academia autenticada
	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	// Buscar falta existente para validar posse e obter estudante
	faltasProj := getFaltasProjection(c)
	faltaAtual, err := faltasProj.GetByID(req.ID)
	if err != nil || faltaAtual == nil {
		utils.RespondWithNotFoundError(c, "falta")
		return
	}
	if faltaAtual.CodigoAcademia != academiaDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "falta não pertence a esta academia")
		return
	}

	// Validar nova matéria se fornecida
	var materiaIDPtr *uuid.UUID
	if req.MateriaDisciplinarID != nil {
		parsed, err := uuid.Parse(*req.MateriaDisciplinarID)
		if err != nil {
			utils.RespondWithValidationError(c, fmt.Errorf("materia_disciplinar_id inválido"))
			return
		}
		materiasProj := getMateriasProjection(c)
		materiaDTO, _ := materiasProj.GetByID(parsed)
		if materiaDTO == nil || materiaDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
			utils.RespondWithForbiddenError(c, "materia não pertence a esta academia")
			return
		}
		materiaIDPtr = &parsed
	}

	// Carregar estudante pelo codigo_estudante da falta
	estudanteProj := getEstudanteProjection(c)
	estudanteDTO, err := estudanteProj.GetByCodigo(faltaAtual.CodigoEstudante)
	if err != nil || estudanteDTO == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	repository := getRepository(c)
	estudanteAgg, err := repository.Load(estudanteDTO.ID, "Estudante")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	estudante, ok := estudanteAgg.(*aggregates.Estudante)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("tipo de aggregate inesperado"))
		return
	}

	if err := estudante.AtualizarFalta(
		academiaDTO.CodigoAcademia,
		req.ID,
		dataPtr,
		materiaIDPtr,
		req.Quantidade,
		req.Observacao,
		userID,
	); err != nil {
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

	log.Printf("Falta atualizada: id=%s estudante=%s (por academia %s)",
		req.ID, faltaAtual.CodigoEstudante, academiaDTO.CodigoAcademia)

	c.JSON(http.StatusOK, gin.H{
		"message":          "falta atualizada com sucesso",
		"id":               req.ID,
		"codigo_estudante": faltaAtual.CodigoEstudante,
	})
}

// ============================================================================
// GET /faltas-estudante/:codigo
// ============================================================================

func GetFaltasEstudante(c *gin.Context) {
	codigoEstudante := c.Param("codigo")

	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByCodigo(codigoEstudante)
	if err != nil || estudante == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	if userType == "estudante" && userID != estudante.ID {
		utils.RespondWithForbiddenError(c, "Você só pode visualizar suas próprias faltas")
		return
	}

	if userType == "academia" {
		academiaProj := getAcademiaProjection(c)
		academiaDTO, _ := academiaProj.GetByID(userID)
		if estudante.CodigoAcademia == nil || academiaDTO == nil || *estudante.CodigoAcademia != academiaDTO.CodigoAcademia {
			utils.RespondWithForbiddenError(c, "Estudante não pertence a esta academia")
			return
		}
	}

	faltasProj := getFaltasProjection(c)
	faltas, err := faltasProj.GetByEstudante(codigoEstudante)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"codigo_estudante": codigoEstudante,
		"nome":             estudante.Nome,
		"faltas":           faltas,
		"total":            len(faltas),
	})
}