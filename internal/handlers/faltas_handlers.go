package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/utils"
)

func rejeitarCamposLegadosSumarioFaltas(c *gin.Context) bool {
	body, err := c.GetRawData()
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("payload inválido"))
		return true
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(body))

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return false
	}
	for _, campo := range []string{"sumario_id", "sumario_titulo"} {
		if _, ok := raw[campo]; ok {
			utils.RespondWithValidationError(c, fmt.Errorf("campo não suportado em falta: %s", campo))
			return true
		}
	}
	return false
}

// ============================================================================
// POST /academia/faltas-aluno
// ============================================================================

func RegistrarFaltas(c *gin.Context) {
	if rejeitarCamposLegadosSumarioFaltas(c) {
		return
	}
	userID, _ := middleware.GetUserID(c)

	var req struct {
		CodigoEstudante      string     `json:"codigo_estudante"       binding:"required"`
		Data                 utils.Date `json:"data"                binding:"required"`
		MateriaDisciplinarID string     `json:"materia_disciplinar_id" binding:"required"`
		Quantidade           int        `json:"quantidade"             binding:"required,min=1"`
		Observacao           *string    `json:"observacao"`
	}

	if err := decodeStrictJSON(c, &req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf(
			"dados obrigatórios: codigo_estudante, data, materia_disciplinar_id e quantidade",
		))
		return
	}
	if strings.TrimSpace(req.CodigoEstudante) == "" || strings.TrimSpace(req.MateriaDisciplinarID) == "" || req.Quantidade <= 0 {
		utils.RespondWithValidationError(c, fmt.Errorf("dados obrigatorios: codigo_estudante, data, materia_disciplinar_id e quantidade"))
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
	if err := validarMatriculaEmAndamento(estudanteDTO); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if err := validarObservacao(req.Observacao); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if req.Quantidade > 100 {
		utils.RespondWithValidationError(c, fmt.Errorf("quantidade de faltas deve ser no máximo 100"))
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

	tipoLetivo, err := inferirTipoLetivoMateria(materiaDTO.Type)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if err := validarDataNoPeriodoLetivo(getDbClient(c), tipoLetivo, anoLectivo, req.Data.Time); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	// Inferir anoAcademico com bloqueio de incompatibilidade estudante x matéria
	anoAcademico, err := inferirAnoAcademicoFaltas(estudanteDTO.AnoEscolar, materiaDTO.AnosAcademicos, materiaDTO.Nome, estudanteDTO.AnoEscolarMedio)
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
		userID,
		aggregates.MaxQuantidadeFaltasPadrao,
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

func CorrigirFalta(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	faltaID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("id da falta inválido"))
		return
	}
	var req struct {
		Quantidade int     `json:"quantidade" binding:"required,min=1"`
		Observacao *string `json:"observacao"`
		Motivo     string  `json:"motivo" binding:"required"`
	}
	if err := decodeStrictJSON(c, &req); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if req.Quantidade > 100 || strings.TrimSpace(req.Motivo) == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("quantidade inválida ou motivo da correção ausente"))
		return
	}
	if err := validarObservacao(req.Observacao); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	academia, err := getAcademiaProjection(c).GetByID(userID)
	if err != nil || academia == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}
	falta, err := getFaltasProjection(c).GetByID(faltaID.String())
	if err != nil || falta == nil {
		utils.RespondWithNotFoundError(c, "falta")
		return
	}
	if falta.CodigoAcademia != academia.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "falta não pertence a esta academia")
		return
	}
	estudanteDTO, err := getEstudanteProjection(c).GetByCodigo(falta.CodigoEstudante)
	if err != nil || estudanteDTO == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}
	agg, err := getRepository(c).Load(estudanteDTO.ID, "Estudante")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	estudante, ok := agg.(*aggregates.Estudante)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("tipo de aggregate inesperado"))
		return
	}
	materiaID, err := uuid.Parse(falta.MateriaDisciplinarID)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	if err := estudante.CorrigirFalta(faltaID, academia.CodigoAcademia, falta.AnoLectivo, falta.Data.Time, materiaID, req.Quantidade, req.Observacao, req.Motivo, userID, aggregates.MaxQuantidadeFaltasPadrao); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if err := getRepository(c).SaveWithAudit(estudante, db.AuditContext{UserID: userID.String(), UserType: "academia", IP: c.ClientIP()}); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "falta corrigida com sucesso", "id": faltaID})
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
