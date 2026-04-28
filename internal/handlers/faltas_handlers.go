package handlers

import (
	"fmt"
	"log"
	"net/http"
	"strings"
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
		CodigoEstudante      string     `json:"codigo_estudante"       binding:"required"`
		Data                 utils.Date `json:"data"                binding:"required"`
		MateriaDisciplinarID string     `json:"materia_disciplinar_id" binding:"required"`
		Quantidade           int        `json:"quantidade"             binding:"required,min=1"`
		Observacao           *string    `json:"observacao"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf(
			"dados obrigatórios: codigo_estudante, data, materia_disciplinar_id e quantidade",
		))
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	// Ano letivo obrigatório — bloqueia registro se a academia não tiver configurado
	anoLectivo, err := resolverAnoLetivoAcademia(academiaDTO.AnoLetivo, academiaDTO.CodigoAcademia)
	if err != nil {
		utils.RespondWithValidationError(c, err)
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

	// Inferir anoAcademico com bloqueio de incompatibilidade estudante x matéria
	anoAcademico, err := inferirAnoAcademicoFaltas(estudanteDTO.AnoEscolar, materiaDTO.AnosAcademicos, materiaDTO.Nome)
	if err != nil {
		utils.RespondWithValidationError(c, err)
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

	err = estudante.RegistrarFalta(
		academiaDTO.CodigoAcademia,
		anoLectivo,
		anoAcademico,
		req.Data.Time,
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
		ID                   string      `json:"id"                     binding:"required"` // ID da linha em projection_faltas
		Data                 *utils.Date `json:"data"`
		MateriaDisciplinarID *string     `json:"materia_disciplinar_id"`
		Quantidade           *int        `json:"quantidade"`
		Observacao           *string     `json:"observacao"`
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
	if req.Observacao == nil || strings.TrimSpace(*req.Observacao) == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("observacao é obrigatória para atualizar uma falta"))
		return
	}

	if req.Quantidade != nil && *req.Quantidade <= 0 {
		utils.RespondWithValidationError(c, fmt.Errorf("quantidade deve ser maior que zero"))
		return
	}

	// Converter data se fornecida
	var dataPtr *time.Time
	if req.Data != nil {
		parsed := req.Data.Time
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

	// Validar ano acadêmico do estudante na matéria alvo (nova ou atual).
	materiaIDFinal := faltaAtual.MateriaDisciplinarID
	if materiaIDPtr != nil {
		materiaIDFinal = materiaIDPtr.String()
	}
	materiaFinalUUID, err := uuid.Parse(materiaIDFinal)
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("materia_disciplinar_id inválido"))
		return
	}
	materiasProj := getMateriasProjection(c)
	materiaFinalDTO, _ := materiasProj.GetByID(materiaFinalUUID)
	if materiaFinalDTO == nil || materiaFinalDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "materia não pertence a esta academia")
		return
	}
	if _, err := inferirAnoAcademicoFaltas(estudanteDTO.AnoEscolar, materiaFinalDTO.AnosAcademicos, materiaFinalDTO.Nome); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	// Bloquear duplicata por (data + codigo_estudante + materia_disciplinar_id).
	dataFinal := faltaAtual.Data.Time
	if dataPtr != nil {
		dataFinal = dataPtr.UTC()
	}
	faltasEstudante, err := faltasProj.GetByEstudante(faltaAtual.CodigoEstudante)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	for _, f := range faltasEstudante {
		if f.ID == faltaAtual.ID {
			continue
		}
		if f.CodigoEstudante == faltaAtual.CodigoEstudante &&
			f.MateriaDisciplinarID == materiaIDFinal &&
			f.Data.Time.Format("2006-01-02") == dataFinal.Format("2006-01-02") {
			utils.RespondWithValidationError(c, fmt.Errorf(
				"falta já registrada para data '%s', materia '%s' e estudante '%s'",
				dataFinal.Format("2006-01-02"), materiaIDFinal, faltaAtual.CodigoEstudante,
			))
			return
		}
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
// DELETE /academia/falta/:id
// ============================================================================

// DeletarFalta faz soft delete de uma falta via event sourcing.
// Body: { "motivo": "string" } (obrigatório).
func DeletarFalta(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	faltaID := c.Param("id")
	if faltaID == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("id de falta inválido"))
		return
	}

	var req struct {
		Motivo string `json:"motivo" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("campo obrigatório: motivo"))
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	faltasProj := getFaltasProjection(c)
	faltaAtual, err := faltasProj.GetByID(faltaID)
	if err != nil || faltaAtual == nil {
		utils.RespondWithNotFoundError(c, "falta")
		return
	}
	if faltaAtual.CodigoAcademia != academiaDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "falta não pertence a esta academia")
		return
	}

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

	if err := estudante.DeletarFalta(academiaDTO.CodigoAcademia, faltaID, req.Motivo, userID); err != nil {
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

	log.Printf("Falta deletada: id=%s estudante=%s (por academia %s, motivo=%s)",
		faltaID, faltaAtual.CodigoEstudante, academiaDTO.CodigoAcademia, req.Motivo)

	c.JSON(http.StatusOK, gin.H{
		"message": "falta deletada com sucesso",
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

	filtros, err := parseFiltrosRegistrosEstudante(c, false)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	faltasFiltradas := make([]interface{}, 0, len(faltas))
	materiasProj := getMateriasProjection(c)
	materiaMetaCache := map[string]materiaMeta{}
	for _, falta := range faltas {
		if !matchesFiltroString(filtros.anoLectivos, falta.AnoLectivo) ||
			!matchesFiltroString(filtros.anoAcademicos, falta.AnoAcademico) ||
			!matchesFiltroString(filtros.materiasDisciplinares, falta.MateriaDisciplinarID) ||
			!matchesFiltroString(filtros.codigosAcademia, falta.CodigoAcademia) {
			continue
		}

		if len(filtros.cursoIDs) > 0 || len(filtros.periodos) > 0 {
			materiaMetaAtual, err := getMateriaMeta(materiasProj, materiaMetaCache, falta.MateriaDisciplinarID)
			if err != nil {
				utils.RespondWithInternalError(c, err)
				return
			}
			if !matchesFiltroString(filtros.cursoIDs, materiaMetaAtual.cursoID) ||
				!matchesFiltroString(filtros.periodos, materiaMetaAtual.periodo) {
				continue
			}
		}

		faltasFiltradas = append(faltasFiltradas, falta)
	}

	c.JSON(http.StatusOK, gin.H{
		"codigo_estudante": codigoEstudante,
		"nome":             estudante.Nome,
		"faltas":           faltasFiltradas,
		"total":            len(faltasFiltradas),
	})
}
