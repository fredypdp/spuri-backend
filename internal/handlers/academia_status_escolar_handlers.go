package handlers

import (
	"fmt"
	"log"
	"net/http"
	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type estudanteBusinessContext struct {
	AcademiaID     uuid.UUID
	CodigoAcademia string
	Estudante      *aggregates.Estudante
}

func carregarEstudanteDaAcademia(c *gin.Context) (*estudanteBusinessContext, bool) {
	codigoEstudante := c.Param("codigo")
	academiaID, _ := middleware.GetUserID(c)
	academiaProj := getAcademiaProjection(c)
	academia, err := academiaProj.GetByID(academiaID)
	if err != nil || academia == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return nil, false
	}

	estudanteProj := getEstudanteProjection(c)
	estudanteDTO, err := estudanteProj.GetByCodigo(codigoEstudante)
	if err != nil || estudanteDTO == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return nil, false
	}
	if estudanteDTO.CodigoAcademia == nil || *estudanteDTO.CodigoAcademia != academia.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "estudante não pertence a esta academia")
		return nil, false
	}

	repository := getRepository(c)
	estudanteAgg, err := repository.Load(estudanteDTO.ID, "Estudante")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return nil, false
	}
	estudante, ok := estudanteAgg.(*aggregates.Estudante)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("tipo de aggregate inesperado"))
		return nil, false
	}
	return &estudanteBusinessContext{AcademiaID: academiaID, CodigoAcademia: academia.CodigoAcademia, Estudante: estudante}, true
}

func salvarEventoEstudante(c *gin.Context, ctx *estudanteBusinessContext) bool {
	audit := db.AuditContext{UserID: ctx.AcademiaID.String(), UserType: "academia", IP: c.ClientIP()}
	if err := getRepository(c).SaveWithAudit(ctx.Estudante, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return false
	}
	return true
}

func parseCursoID(c *gin.Context, raw string, tipo string) (uuid.UUID, bool) {
	cursoID, err := uuid.Parse(raw)
	if err != nil || cursoID == uuid.Nil {
		utils.RespondWithValidationError(c, fmt.Errorf("curso_id inválido"))
		return uuid.Nil, false
	}
	curso, _ := getCursosProjection(c).GetByID(cursoID)
	if curso == nil {
		utils.RespondWithValidationError(c, fmt.Errorf("curso_id não encontrado"))
		return uuid.Nil, false
	}
	if curso.Type != tipo {
		utils.RespondWithValidationError(c, fmt.Errorf("curso_id deve ser do tipo '%s'", tipo))
		return uuid.Nil, false
	}
	return cursoID, true
}

func MatricularFundamentalHandler(c *gin.Context) {
	var req struct {
		AnoEscolar string `json:"ano_escolar_fundamental" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ano_escolar_fundamental é obrigatório"))
		return
	}
	ctx, ok := carregarEstudanteDaAcademia(c)
	if !ok {
		return
	}
	if err := ctx.Estudante.MatricularFundamental(req.AnoEscolar, ctx.AcademiaID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if !salvarEventoEstudante(c, ctx) {
		return
	}
	log.Printf("✅ [Academia %s] Matrícula fundamental efetivada: %s", ctx.CodigoAcademia, c.Param("codigo"))
	c.JSON(http.StatusOK, gin.H{"message": "matrícula fundamental efetivada", "status_escolar_fundamental": "em_andamento"})
}

func InterromperFundamentalHandler(c *gin.Context) {
	var req struct {
		Motivo string `json:"motivo" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("motivo é obrigatório"))
		return
	}
	ctx, ok := carregarEstudanteDaAcademia(c)
	if !ok {
		return
	}
	if err := ctx.Estudante.InterromperFundamental(req.Motivo, ctx.AcademiaID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if !salvarEventoEstudante(c, ctx) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "fundamental interrompido", "status_escolar_fundamental": "inativo"})
}

func MatricularMedioHandler(c *gin.Context) {
	var req struct {
		AnoEscolar string `json:"ano_escolar_medio" binding:"required"`
		CursoID    string `json:"curso_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ano_escolar_medio e curso_id são obrigatórios"))
		return
	}
	cursoID, ok := parseCursoID(c, req.CursoID, "medio")
	if !ok {
		return
	}
	ctx, ok := carregarEstudanteDaAcademia(c)
	if !ok {
		return
	}
	if err := ctx.Estudante.MatricularMedio(req.AnoEscolar, cursoID, ctx.AcademiaID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if !salvarEventoEstudante(c, ctx) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "matrícula no médio efetivada", "status_escolar_medio": "em_andamento"})
}

func InterromperMedioHandler(c *gin.Context) {
	var req struct {
		Motivo string `json:"motivo" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("motivo é obrigatório"))
		return
	}
	ctx, ok := carregarEstudanteDaAcademia(c)
	if !ok {
		return
	}
	if err := ctx.Estudante.InterromperMedio(req.Motivo, ctx.AcademiaID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if !salvarEventoEstudante(c, ctx) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "médio interrompido", "status_escolar_medio": "inativo"})
}

func MatricularSuperiorHandler(c *gin.Context) {
	var req struct {
		CursoID string `json:"curso_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("curso_id é obrigatório"))
		return
	}
	cursoID, ok := parseCursoID(c, req.CursoID, "superior")
	if !ok {
		return
	}
	ctx, ok := carregarEstudanteDaAcademia(c)
	if !ok {
		return
	}
	if err := ctx.Estudante.MatricularSuperior(cursoID, ctx.AcademiaID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if !salvarEventoEstudante(c, ctx) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "matrícula superior efetivada", "status_superior": "em_andamento", "ano_superior": "1_ano_superior", "semestre_atual": 1})
}

func TrancarSuperiorHandler(c *gin.Context) {
	var req struct {
		Motivo string `json:"motivo" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("motivo é obrigatório"))
		return
	}
	ctx, ok := carregarEstudanteDaAcademia(c)
	if !ok {
		return
	}
	if err := ctx.Estudante.TrancarSuperior(req.Motivo, ctx.AcademiaID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if !salvarEventoEstudante(c, ctx) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "superior trancado", "status_superior": "inativo"})
}

func DesvincularEstudanteHandler(c *gin.Context) {
	var req struct {
		Motivo string `json:"motivo" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("motivo é obrigatório"))
		return
	}
	ctx, ok := carregarEstudanteDaAcademia(c)
	if !ok {
		return
	}
	if err := ctx.Estudante.DesvincularDaAcademia(ctx.CodigoAcademia, req.Motivo, ctx.AcademiaID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if !salvarEventoEstudante(c, ctx) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "estudante desvinculado da academia", "status": "arquivado"})
}

func ReintegrarEstudanteHandler(c *gin.Context) {
	var req struct {
		TipoEnsino      string `json:"tipo_ensino" binding:"required"`
		AnoEscolar      string `json:"ano_escolar_fundamental"`
		AnoEscolarMedio string `json:"ano_escolar_medio"`
		CursoMedioID    string `json:"curso_medio_id"`
		CursoSuperiorID string `json:"curso_superior_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("tipo_ensino é obrigatório"))
		return
	}
	ctx, ok := carregarEstudanteDaAcademia(c)
	if !ok {
		return
	}
	var anoFund, anoMed *string
	var cursoMed, cursoSup *uuid.UUID
	if req.AnoEscolar != "" {
		anoFund = &req.AnoEscolar
	}
	if req.AnoEscolarMedio != "" {
		anoMed = &req.AnoEscolarMedio
	}
	if req.CursoMedioID != "" {
		id, ok := parseCursoID(c, req.CursoMedioID, "medio")
		if !ok {
			return
		}
		cursoMed = &id
	}
	if req.CursoSuperiorID != "" {
		id, ok := parseCursoID(c, req.CursoSuperiorID, "superior")
		if !ok {
			return
		}
		cursoSup = &id
	}
	if err := ctx.Estudante.Reintegrar(ctx.CodigoAcademia, req.TipoEnsino, anoFund, anoMed, cursoMed, cursoSup, ctx.AcademiaID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if !salvarEventoEstudante(c, ctx) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "estudante reintegrado", "status": "ativo", "tipo_ensino": req.TipoEnsino})
}
