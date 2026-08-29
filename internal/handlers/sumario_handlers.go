package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/utils"
)

// ============================================================================
// POST /academia/sumario
// ============================================================================

func CriarSumario(c *gin.Context) {
	userID, authenticated := middleware.GetUserID(c)
	if !authenticated {
		utils.RespondWithUnauthorizedError(c)
		return
	}

	var req struct {
		SumarioTitulo string    `json:"sumario_titulo"`
		Descricao     *string   `json:"descricao"`
		MateriaID     uuid.UUID `json:"materia_id"`
		Periodo       string    `json:"periodo"`
		AnoAcademico  string    `json:"ano_academico"`
	}
	if err := decodeStrictJSON(c, &req); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if strings.TrimSpace(req.SumarioTitulo) == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("sumario_titulo é obrigatório"))
		return
	}
	if req.MateriaID == uuid.Nil {
		utils.RespondWithValidationError(c, fmt.Errorf("materia_id é obrigatório"))
		return
	}
	if strings.TrimSpace(req.Periodo) == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("periodo é obrigatório"))
		return
	}
	if strings.TrimSpace(req.AnoAcademico) == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("ano_academico é obrigatório"))
		return
	}

	academiaDTO, err := getAcademiaProjection(c).GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	materiaDTO, err := getMateriasProjection(c).GetByID(req.MateriaID)
	if err != nil || materiaDTO == nil {
		utils.RespondWithNotFoundError(c, "materia")
		return
	}
	if materiaDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "matéria não pertence a esta academia")
		return
	}

	// nivel/type inferidos da matéria — nunca aceitos do cliente.
	nivel := materiaDTO.Type
	tipo, err := inferirTipoLetivoMateria(nivel)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	cursoID := materiaDTO.CursoID // curso_id inferido da matéria — nunca aceito do cliente (decisão de design nº 1)

	// periodo: mesma regra que faltas/notas já aplicam para matéria superior.
	if tipo == aggregates.TipoSuperior {
		if materiaDTO.Periodo == nil || strings.TrimSpace(*materiaDTO.Periodo) == "" {
			utils.RespondWithValidationError(c, fmt.Errorf("matéria superior sem período definido"))
			return
		}
		if req.Periodo != *materiaDTO.Periodo {
			utils.RespondWithValidationError(c, fmt.Errorf("periodo (%s) não corresponde ao período da matéria (%s)", req.Periodo, *materiaDTO.Periodo))
			return
		}
	} else if !containsString(aggregates.PeriodosEscolar, req.Periodo) {
		utils.RespondWithValidationError(c, fmt.Errorf("periodo inválido para matéria %s; use um de %v", nivel, aggregates.PeriodosEscolar))
		return
	}

	// ano_academico: deve pertencer aos anos em que a matéria é lecionada
	// (mesma regra usada por faltas/notas via inferirAnoAcademicoParaNota).
	if !containsString(materiaDTO.AnosAcademicos, req.AnoAcademico) {
		utils.RespondWithValidationError(c, fmt.Errorf("ano_academico (%s) não é um dos anos em que a matéria é lecionada", req.AnoAcademico))
		return
	}

	var cursoUUID *uuid.UUID
	if cursoID != nil {
		cursoUUID = cursoID
	}

	sumario := aggregates.NewSumario()
	if err := sumario.Criar(req.SumarioTitulo, req.Descricao, academiaDTO.CodigoAcademia, tipo, nivel, req.Periodo, req.AnoAcademico, cursoUUID, req.MateriaID, userID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	repository := getRepository(c)
	audit := db.AuditContext{UserID: userID.String(), UserType: "academia", IP: c.ClientIP()}
	if err := repository.SaveWithAudit(sumario, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "sumário criado com sucesso",
		"id":      sumario.ID,
	})
}

// ============================================================================
// GET /academia/sumarios
// ============================================================================

func ListarSumarios(c *gin.Context) {
	userID, authenticated := middleware.GetUserID(c)
	if !authenticated {
		utils.RespondWithUnauthorizedError(c)
		return
	}
	codigoAcademia, err := resolverCodigoAcademiaSumarios(c, userID) // reaproveite o helper já usado por ListarMaterias para resolver codigo_academia (admin pode passar ?codigo_academia=, academia usa o próprio)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	var materiaID, periodo, anoAcademico *string
	if v := c.Query("materia_id"); v != "" {
		materiaID = &v
	}
	if v := c.Query("periodo"); v != "" {
		periodo = &v
	}
	if v := c.Query("ano_academico"); v != "" {
		anoAcademico = &v
	}

	sumarios, err := getSumariosProjection(c).GetByAcademia(codigoAcademia, materiaID, periodo, anoAcademico)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"sumarios": sumarios})
}

// ============================================================================
// GET /academia/sumario/:id
// ============================================================================

func GetSumario(c *gin.Context) {
	userID, authenticated := middleware.GetUserID(c)
	if !authenticated {
		utils.RespondWithUnauthorizedError(c)
		return
	}
	sumarioID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("id inválido"))
		return
	}
	sumarioDTO, err := getSumariosProjection(c).GetByID(sumarioID)
	if err != nil || sumarioDTO == nil {
		utils.RespondWithNotFoundError(c, "sumario")
		return
	}
	codigoAcademia, err := resolverCodigoAcademiaSumarios(c, userID)
	if err != nil || sumarioDTO.CodigoAcademia != codigoAcademia {
		utils.RespondWithForbiddenError(c, "sumário não pertence a esta academia")
		return
	}
	c.JSON(http.StatusOK, sumarioDTO)
}

// ============================================================================
// PUT /academia/sumario/:id/dados
// ============================================================================

func AtualizarDadosSumario(c *gin.Context) {
	userID, authenticated := middleware.GetUserID(c)
	if !authenticated {
		utils.RespondWithUnauthorizedError(c)
		return
	}
	sumarioUUID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("id inválido"))
		return
	}

	// materia_id/periodo/ano_academico são imutáveis (decisão de design nº 3):
	// mesma técnica de detecção usada em AtualizarDadosMateria para "periodo".
	var raw map[string]json.RawMessage
	rawBody, _ := c.GetRawData()
	c.Request.Body = io.NopCloser(bytes.NewBuffer(rawBody))
	_ = json.Unmarshal(rawBody, &raw)
	for _, campoImutavel := range []string{"materia_id", "periodo", "ano_academico", "curso_id", "nivel", "type"} {
		if _, ok := raw[campoImutavel]; ok {
			utils.RespondWithValidationError(c, fmt.Errorf("campo imutável após a criação: %s (delete este sumário e crie outro)", campoImutavel))
			return
		}
	}

	var req struct {
		SumarioTitulo *string `json:"sumario_titulo"`
		Descricao     *string `json:"descricao"`
	}
	if err := decodeStrictJSON(c, &req); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	sumarioDTO, err := getSumariosProjection(c).GetByID(sumarioUUID)
	if err != nil || sumarioDTO == nil {
		utils.RespondWithNotFoundError(c, "sumario")
		return
	}
	academiaDTO, err := getAcademiaProjection(c).GetByID(userID)
	if err != nil || academiaDTO == nil || sumarioDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "sumário não pertence a esta academia")
		return
	}

	repository := getRepository(c)
	sumarioAgg, err := repository.Load(sumarioUUID, "Sumario")
	if err != nil {
		utils.RespondWithNotFoundError(c, "sumario")
		return
	}
	sumario, ok := sumarioAgg.(*aggregates.Sumario)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("tipo de aggregate inesperado"))
		return
	}

	if err := sumario.AtualizarDados(req.SumarioTitulo, req.Descricao, userID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{UserID: userID.String(), UserType: "academia", IP: c.ClientIP()}
	if err := repository.SaveWithAudit(sumario, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "sumário atualizado com sucesso"})
}

// ============================================================================
// DELETE /academia/sumario/:id
// ============================================================================

func DeletarSumario(c *gin.Context) {
	userID, authenticated := middleware.GetUserID(c)
	if !authenticated {
		utils.RespondWithUnauthorizedError(c)
		return
	}
	sumarioUUID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("id inválido"))
		return
	}

	sumarioDTO, err := getSumariosProjection(c).GetByID(sumarioUUID)
	if err != nil || sumarioDTO == nil {
		utils.RespondWithNotFoundError(c, "sumario")
		return
	}
	academiaDTO, err := getAcademiaProjection(c).GetByID(userID)
	if err != nil || academiaDTO == nil || sumarioDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "sumário não pertence a esta academia")
		return
	}

	repository := getRepository(c)
	sumarioAgg, err := repository.Load(sumarioUUID, "Sumario")
	if err != nil {
		utils.RespondWithNotFoundError(c, "sumario")
		return
	}
	sumario, ok := sumarioAgg.(*aggregates.Sumario)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("tipo de aggregate inesperado"))
		return
	}

	// Nota de design: deletar NUNCA é bloqueado por já existirem faltas
	// vinculadas — é o oposto do que MateriaDisciplinar faz (que exige status
	// "inativo" antes). O soft delete + snapshot de sumario_titulo em
	// projection_faltas existe exatamente para permitir isso com segurança.
	if err := sumario.Deletar(userID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{UserID: userID.String(), UserType: "academia", IP: c.ClientIP()}
	if err := repository.SaveWithAudit(sumario, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "sumário deletado com sucesso"})
}

func resolverCodigoAcademiaSumarios(c *gin.Context, userID uuid.UUID) (string, error) {
	userType, _ := middleware.GetUserType(c)
	if userType == "admin" {
		codigo := c.Query("codigo_academia")
		if codigo == "" {
			return "", fmt.Errorf("codigo_academia é obrigatório para admin")
		}
		a, err := getAcademiaProjection(c).GetByCodigo(codigo)
		if err != nil {
			return "", err
		}
		if a == nil {
			return "", fmt.Errorf("academia não encontrada")
		}
		return codigo, nil
	}
	a, err := getAcademiaProjection(c).GetByID(userID)
	if err != nil {
		return "", err
	}
	if a == nil {
		return "", fmt.Errorf("academia não encontrada")
	}
	return a.CodigoAcademia, nil
}
