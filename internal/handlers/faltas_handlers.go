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

func campoPresenteNoPayload(c *gin.Context, campo string) (bool, error) {
	body, err := c.GetRawData()
	if err != nil {
		return false, fmt.Errorf("payload inválido")
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(body))
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return false, nil
	}
	_, ok := raw[campo]
	return ok, nil
}
func rejeitarCamposImutaveisFalta(c *gin.Context, campos ...string) bool {
	body, err := c.GetRawData()
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("payload inválido"))
		return true
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(body))
	var raw map[string]json.RawMessage
	if json.Unmarshal(body, &raw) != nil {
		return false
	}
	for _, campo := range campos {
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
	userID, _ := middleware.GetUserID(c)

	var req struct {
		CodigoEstudante      string     `json:"codigo_estudante"       binding:"required"`
		Data                 utils.Date `json:"data"                binding:"required"`
		Periodo              string     `json:"periodo"             binding:"required"`
		MateriaDisciplinarID string     `json:"materia_disciplinar_id" binding:"required"`
		Quantidade           int        `json:"quantidade"             binding:"required,min=1"`
		Observacao           *string    `json:"observacao"`
		SumarioID            *uuid.UUID `json:"sumario_id"`
	}

	if err := decodeStrictJSON(c, &req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf(
			"dados obrigatórios: codigo_estudante, data, periodo, materia_disciplinar_id e quantidade",
		))
		return
	}
	if strings.TrimSpace(req.CodigoEstudante) == "" || strings.TrimSpace(req.Periodo) == "" || strings.TrimSpace(req.MateriaDisciplinarID) == "" || req.Quantidade <= 0 {
		utils.RespondWithValidationError(c, fmt.Errorf("dados obrigatorios: codigo_estudante, data, periodo, materia_disciplinar_id e quantidade"))
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

	periodosValidos, err := resolverPeriodosValidos(c, tipoLetivo, materiaDTO.CursoID)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if tipoLetivo == aggregates.TipoSuperior && materiaDTO.Periodo != nil && req.Periodo != *materiaDTO.Periodo {
		utils.RespondWithValidationError(c, fmt.Errorf(
			"periodo '%s' invalido para a materia '%s'. Periodo definido: '%s'",
			req.Periodo, materiaDTO.Nome, *materiaDTO.Periodo,
		))
		return
	}

	// Inferir anoAcademico com bloqueio de incompatibilidade estudante x matéria
	anoAcademico, err := inferirAnoAcademicoFaltas(estudanteDTO.AnoEscolar, materiaDTO.AnosAcademicos, materiaDTO.Nome, estudanteDTO.AnoEscolarMedio)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	var sumarioTitulo *string
	if req.SumarioID != nil {
		sumario, err := getSumariosProjection(c).GetByID(*req.SumarioID)
		if err != nil || sumario == nil {
			utils.RespondWithNotFoundError(c, "sumario")
			return
		}
		if sumario.CodigoAcademia != academiaDTO.CodigoAcademia {
			utils.RespondWithForbiddenError(c, "sumário não pertence a esta academia")
			return
		}
		if sumario.MateriaID != materiaID.String() || sumario.Periodo != req.Periodo || sumario.AnoAcademico != anoAcademico {
			utils.RespondWithValidationError(c, fmt.Errorf("sumário incompatível com a falta"))
			return
		}
		sumarioTitulo = &sumario.SumarioTitulo
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
		req.Periodo,
		req.Data.Time,
		materiaID,
		req.Quantidade,
		req.Observacao,
		userID,
		periodosValidos,
		aggregates.MaxQuantidadeFaltasPadrao,
		req.SumarioID, sumarioTitulo,
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
		"message":          "faltas registradas com sucesso",
		"estudante":        req.CodigoEstudante,
		"materia":          materiaDTO.Nome,
		"quantidade":       req.Quantidade,
		"periodo":          req.Periodo,
		"periodos_validos": periodosValidos,
		"ano_academico":    anoAcademico,
	})
}

func CorrigirFalta(c *gin.Context) {
	if rejeitarCamposImutaveisFalta(c, "periodo") {
		return
	}
	userID, _ := middleware.GetUserID(c)
	faltaID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("id da falta inválido"))
		return
	}
	var req struct {
		Quantidade int        `json:"quantidade" binding:"required,min=1"`
		Observacao *string    `json:"observacao"`
		Motivo     string     `json:"motivo" binding:"required"`
		SumarioID  *uuid.UUID `json:"sumario_id"`
	}
	if err := decodeStrictJSON(c, &req); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	sumarioIDPresente, err := campoPresenteNoPayload(c, "sumario_id")
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if sumarioIDPresente && req.SumarioID == nil {
		utils.RespondWithValidationError(c, fmt.Errorf("sumario_id não pode ser definido como null; use o endpoint de desvínculo"))
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
	var novoSumarioTitulo *string
	if sumarioIDPresente {
		sumario, err := getSumariosProjection(c).GetByID(*req.SumarioID)
		if err != nil || sumario == nil {
			utils.RespondWithNotFoundError(c, "sumario")
			return
		}
		if sumario.CodigoAcademia != academia.CodigoAcademia || sumario.MateriaID != falta.MateriaDisciplinarID || sumario.Periodo != falta.Periodo || sumario.AnoAcademico != falta.AnoAcademico {
			utils.RespondWithValidationError(c, fmt.Errorf("sumário incompatível com a falta"))
			return
		}
		novoSumarioTitulo = &sumario.SumarioTitulo
	}
	if err := estudante.CorrigirFalta(faltaID, academia.CodigoAcademia, falta.AnoLectivo, falta.Periodo, falta.Data.Time, materiaID, req.Quantidade, req.Observacao, req.Motivo, userID, aggregates.MaxQuantidadeFaltasPadrao, sumarioIDPresente, req.SumarioID, novoSumarioTitulo); err != nil {
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
			!matchesFiltroString(filtros.periodos, falta.Periodo) ||
			!matchesFiltroString(filtros.materiasDisciplinares, falta.MateriaDisciplinarID) ||
			!matchesFiltroString(filtros.codigosAcademia, falta.CodigoAcademia) {
			continue
		}

		if len(filtros.cursoIDs) > 0 {
			materiaMetaAtual, err := getMateriaMeta(materiasProj, materiaMetaCache, falta.MateriaDisciplinarID)
			if err != nil {
				utils.RespondWithInternalError(c, err)
				return
			}
			if !matchesFiltroString(filtros.cursoIDs, materiaMetaAtual.cursoID) {
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

func DesvincularSumarioFalta(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		utils.RespondWithUnauthorizedError(c)
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("id inválido"))
		return
	}
	f, err := getFaltasProjection(c).GetByID(id.String())
	if err != nil || f == nil {
		utils.RespondWithNotFoundError(c, "falta")
		return
	}
	a, err := getAcademiaProjection(c).GetByID(userID)
	if err != nil || a == nil || f.CodigoAcademia != a.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "falta não pertence a esta academia")
		return
	}
	if f.SumarioID == nil {
		c.JSON(http.StatusOK, gin.H{"message": "falta já não possui sumário vinculado", "id": id})
		return
	}
	student, err := getEstudanteProjection(c).GetByCodigo(f.CodigoEstudante)
	if err != nil || student == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}
	agg, err := getRepository(c).Load(student.ID, "Estudante")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	e, ok := agg.(*aggregates.Estudante)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("tipo de aggregate inesperado"))
		return
	}
	mid, err := uuid.Parse(f.MateriaDisciplinarID)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	if err := e.CorrigirFalta(id, a.CodigoAcademia, f.AnoLectivo, f.Periodo, f.Data.Time, mid, f.Quantidade, f.Observacao, "Sumário desvinculado via endpoint dedicado", userID, aggregates.MaxQuantidadeFaltasPadrao, true, nil, nil); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if err := getRepository(c).SaveWithAudit(e, db.AuditContext{UserID: userID.String(), UserType: "academia", IP: c.ClientIP()}); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "sumário desvinculado com sucesso", "id": id})
}
