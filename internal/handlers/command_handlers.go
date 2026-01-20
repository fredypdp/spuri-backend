package handlers

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/utils"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type RegistrarNotasRequest struct {
	CodigoEstudante      string  `json:"codigo_estudante" binding:"required"`
	AnoLectivo           string  `json:"ano_lectivo" binding:"required"`
	Periodo              string  `json:"periodo" binding:"required"`
	MateriaDisciplinarID string  `json:"materia_disciplinar_id" binding:"required"`
	Nota                 float64 `json:"nota" binding:"required"`
	Observacao           *string `json:"observacao"`
}

func RegistrarNotas(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req RegistrarNotasRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	// ✅ VALIDAÇÕES
	if err := utils.ValidateNota(req.Nota); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := utils.ValidatePeriodo(req.Periodo); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := utils.ValidateString(req.AnoLectivo, "ano letivo", 4, 20, true); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := utils.ValidateObservacao(req.Observacao); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	materiaID, err := uuid.Parse(req.MateriaDisciplinarID)
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ID de matéria inválido"))
		return
	}

	estudanteProj := getEstudanteProjection(c)
	estudanteDTO, err := estudanteProj.GetByCodigo(req.CodigoEstudante)
	if err != nil || estudanteDTO == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	materiasProj := getMateriasProjection(c)
	materiaDTO, err := materiasProj.GetByID(materiaID)
	if err != nil || materiaDTO == nil {
		utils.RespondWithNotFoundError(c, "matéria")
		return
	}

	if materiaDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "matéria não pertence a esta academia")
		return
	}

	repository := getRepository(c)
	estudanteAgg, err := repository.Load(estudanteDTO.ID, "Estudante")
	if err != nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)

	if estudante.CodigoAcademia == nil || *estudante.CodigoAcademia != academiaDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "estudante não pertence a esta academia")
		return
	}

	err = estudante.RegistrarNota(
		academiaDTO.CodigoAcademia,
		req.AnoLectivo,
		req.Periodo,
		materiaID,
		req.Nota,
		req.Observacao,
	)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(estudante); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":          "nota registrada com sucesso",
		"codigo_estudante": req.CodigoEstudante,
		"materia":          materiaDTO.Nome,
		"nota":             req.Nota,
		"periodo":          req.Periodo,
	})
}

type RegistrarFaltasRequest struct {
	CodigoEstudante      string  `json:"codigo_estudante" binding:"required"`
	AnoLectivo           string  `json:"ano_lectivo" binding:"required"`
	Data                 string  `json:"data" binding:"required"`
	MateriaDisciplinarID string  `json:"materia_disciplinar_id" binding:"required"`
	Quantidade           int     `json:"quantidade" binding:"required"`
	Observacao           *string `json:"observacao"`
}

func RegistrarFaltas(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req RegistrarFaltasRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	// ✅ VALIDAÇÕES
	if err := utils.ValidateQuantidade(req.Quantidade, "quantidade"); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := utils.ValidateString(req.AnoLectivo, "ano letivo", 4, 20, true); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := utils.ValidateObservacao(req.Observacao); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	data, err := time.Parse("2006-01-02", req.Data)
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("formato de data inválido (use YYYY-MM-DD)"))
		return
	}

	materiaID, err := uuid.Parse(req.MateriaDisciplinarID)
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ID de matéria inválido"))
		return
	}

	estudanteProj := getEstudanteProjection(c)
	estudanteDTO, err := estudanteProj.GetByCodigo(req.CodigoEstudante)
	if err != nil || estudanteDTO == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	materiasProj := getMateriasProjection(c)
	materiaDTO, err := materiasProj.GetByID(materiaID)
	if err != nil || materiaDTO == nil {
		utils.RespondWithNotFoundError(c, "matéria")
		return
	}

	if materiaDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "matéria não pertence a esta academia")
		return
	}

	repository := getRepository(c)
	estudanteAgg, err := repository.Load(estudanteDTO.ID, "Estudante")
	if err != nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)

	if estudante.CodigoAcademia == nil || *estudante.CodigoAcademia != academiaDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "estudante não pertence a esta academia")
		return
	}

	err = estudante.RegistrarFalta(
		academiaDTO.CodigoAcademia,
		req.AnoLectivo,
		data,
		materiaID,
		req.Quantidade,
		req.Observacao,
	)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(estudante); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":          "faltas registradas com sucesso",
		"codigo_estudante": req.CodigoEstudante,
		"materia":          materiaDTO.Nome,
		"quantidade":       req.Quantidade,
		"data":             data.Format("2006-01-02"),
	})
}

type InscricaoEscolaRequest struct {
	CodigoAcademia      string  `json:"codigo_academia" binding:"required"`
	AnoEscolarInscricao string  `json:"ano_escolar_inscricao" binding:"required"`
	CursoMedio          *string `json:"curso_medio"`
}

func InscricaoEscola(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	bodyBytes, _ := c.GetRawData()
	log.Printf("📥 [INSCRICAO] Raw body: %s", string(bodyBytes))
	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	var req InscricaoEscolaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByCodigo(req.CodigoAcademia)
	if err != nil || academiaDTO == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	inscProj := getInscricoesProjection(c)
	inscricoes, err := inscProj.GetByEstudante(userID)
	if err == nil {
		for _, insc := range inscricoes {
			if insc.CodigoAcademia == req.CodigoAcademia && insc.Status == "espera" {
				utils.RespondWithValidationError(c, fmt.Errorf("você já possui uma inscrição pendente nesta academia"))
				return
			}
		}
	}

	repository := getRepository(c)
	estudanteAgg, err := repository.Load(userID, "Estudante")
	if err != nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)
	
	err = estudante.SolicitarInscricao(
		req.CodigoAcademia,
		"escola",
		req.AnoEscolarInscricao,
		req.CursoMedio,
	)

	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(estudante); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":         "inscrição solicitada com sucesso",
		"codigo_academia": req.CodigoAcademia,
	})
}

type InscricaoUniversidadeRequest struct {
	CodigoAcademia       string `json:"codigo_academia" binding:"required"`
	AnoSuperiorInscricao string `json:"ano_superior_inscricao" binding:"required"`
	CursoSuperior        string `json:"curso_superior" binding:"required"`
}

func InscricaoUniversidade(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req InscricaoUniversidadeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByCodigo(req.CodigoAcademia)
	if err != nil || academiaDTO == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	repository := getRepository(c)
	estudanteAgg, err := repository.Load(userID, "Estudante")
	if err != nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)

	err = estudante.SolicitarInscricao(
		req.CodigoAcademia,
		"universidade",
		req.AnoSuperiorInscricao,
		&req.CursoSuperior,
	)

	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(estudante); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":         "inscrição solicitada com sucesso",
		"codigo_academia": req.CodigoAcademia,
	})
}

func AprovarInscricao(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	inscricaoID, err := parseUUID(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ID de inscrição inválido"))
		return
	}

	inscProj := getInscricoesProjection(c)
	inscricao, err := inscProj.GetByID(inscricaoID)
	if err != nil || inscricao == nil {
		utils.RespondWithNotFoundError(c, "inscrição")
		return
	}

	switch inscricao.Status {
	case "aprovado":
		utils.RespondWithValidationError(c, fmt.Errorf("esta inscrição já foi aprovada anteriormente"))
		return
	case "reprovado":
		utils.RespondWithValidationError(c, fmt.Errorf("esta inscrição foi reprovada e não pode ser aprovada"))
		return
	}

	if inscricao.AcademiaID != userID {
		utils.RespondWithForbiddenError(c, "inscrição não pertence a esta academia")
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	repository := getRepository(c)

	estudanteAgg, err := repository.Load(inscricao.EstudanteID, "Estudante")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)
	
	err = estudante.AprovarInscricao(academiaDTO.CodigoAcademia, inscricaoID)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(estudante); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	academiaAgg, err := repository.Load(userID, "Academia")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	academia := academiaAgg.(*aggregates.Academia)
	err = academia.AprovarInscricao(
		inscricao.EstudanteID,
		inscricaoID,
		inscricao.Tipo,
		inscricao.AnoInscricao,
		inscricao.Curso,
	)

	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(academia); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":          "inscrição aprovada com sucesso",
		"codigo_estudante": inscricao.CodigoEstudante,
		"status_anterior":  "espera",
		"status_atual":     "aprovado",
	})
}

func ReprovarInscricao(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	inscricaoID, err := parseUUID(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ID de inscrição inválido"))
		return
	}

	inscProj := getInscricoesProjection(c)
	inscricao, err := inscProj.GetByID(inscricaoID)
	if err != nil || inscricao == nil {
		utils.RespondWithNotFoundError(c, "inscrição")
		return
	}

	if inscricao.Status != "espera" {
		utils.RespondWithValidationError(c, fmt.Errorf("inscrição já foi processada com status '%s'", inscricao.Status))
		return
	}

	if inscricao.AcademiaID != userID {
		utils.RespondWithForbiddenError(c, "inscrição não pertence a esta academia")
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	repository := getRepository(c)

	estudanteAgg, err := repository.Load(inscricao.EstudanteID, "Estudante")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)

	reprovadoEvent := &aggregates.InscricaoReprovadaEvent{
		BaseEvent: aggregates.BaseEvent{
			EventType:   "InscricaoReprovada",
			AggregateID: estudante.ID,
		},
		InscricaoID:    inscricaoID,
		CodigoAcademia: academiaDTO.CodigoAcademia,
	}

	if err := estudante.Apply(reprovadoEvent); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	estudante.RaiseEvent(reprovadoEvent)

	if err := repository.Save(estudante); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	academiaAgg, err := repository.Load(userID, "Academia")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	academia := academiaAgg.(*aggregates.Academia)

	err = academia.ReprovarInscricao(inscricao.EstudanteID, inscricaoID, "")
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(academia); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":          "inscrição reprovada com sucesso",
		"codigo_estudante": inscricao.CodigoEstudante,
		"status":           "reprovado",
	})
}

func parseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}