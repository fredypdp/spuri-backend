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

func RegistrarNotas(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		CodigoEstudante      string  `json:"codigo_estudante" binding:"required"`
		AnoLectivo           string  `json:"ano_lectivo" binding:"required"`
		Periodo              string  `json:"periodo" binding:"required"`
		MateriaDisciplinarID string  `json:"materia_disciplinar_id" binding:"required"`
		Nota                 float64 `json:"nota" binding:"required,min=0,max=20"`
		Observacao           *string `json:"observacao"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("dados obrigatórios: codigo_estudante, ano_lectivo, periodo, materia_disciplinar_id e nota"))
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
		utils.RespondWithForbiddenError(c, "Estudante não pertence a esta academia")
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
		utils.RespondWithForbiddenError(c, "Matéria não pertence a esta academia")
		return
	}

	repository := getRepository(c)
	estudanteAgg, err := repository.Load(estudanteDTO.ID, "Estudante")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)
	
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

	log.Printf("Nota registrada: %s - %.2f em %s", req.CodigoEstudante, req.Nota, materiaDTO.Nome)

	c.JSON(http.StatusCreated, gin.H{
		"message":   "nota registrada com sucesso",
		"estudante": req.CodigoEstudante,
		"materia":   materiaDTO.Nome,
		"nota":      req.Nota,
	})
}

func RegistrarFaltas(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		CodigoEstudante      string  `json:"codigo_estudante" binding:"required"`
		AnoLectivo           string  `json:"ano_lectivo" binding:"required"`
		Data                 string  `json:"data" binding:"required"`
		MateriaDisciplinarID string  `json:"materia_disciplinar_id" binding:"required"`
		Quantidade           int     `json:"quantidade" binding:"required,min=1"`
		Observacao           *string `json:"observacao"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("dados obrigatórios: codigo_estudante, ano_lectivo, data, materia_disciplinar_id e quantidade"))
		return
	}

	data, err := time.Parse("2006-01-02", req.Data)
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("formato de data inválido. Use YYYY-MM-DD"))
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
		utils.RespondWithForbiddenError(c, "Estudante não pertence a esta academia")
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
		utils.RespondWithForbiddenError(c, "Matéria não pertence a esta academia")
		return
	}

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

	log.Printf("Faltas registradas: %s - %d em %s", req.CodigoEstudante, req.Quantidade, materiaDTO.Nome)

	c.JSON(http.StatusCreated, gin.H{
		"message":    "faltas registradas com sucesso",
		"estudante":  req.CodigoEstudante,
		"materia":    materiaDTO.Nome,
		"quantidade": req.Quantidade,
	})
}

func AprovarInscricao(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		CodigoEstudante string     `json:"codigo_estudante" binding:"required"`
		Tipo            string     `json:"tipo" binding:"required"`
		AnoInscricao    string     `json:"ano_inscricao" binding:"required"`
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
	safeCodEst := db.SafeString(req.CodigoEstudante)
	safeCodAcad := db.SafeString(academiaDTO.CodigoAcademia)
	safeTipo := db.SafeString(req.Tipo)

	queryInsc := fmt.Sprintf(`
		SELECT id FROM projection_inscricoes 
		WHERE codigo_estudante = '%s' 
		AND codigo_academia = '%s' 
		AND tipo = '%s' 
		AND status = 'espera'
		LIMIT 1
	`, safeCodEst, safeCodAcad, safeTipo)

	var inscricaoID uuid.UUID
	err = client.DB().QueryRow(queryInsc).Scan(&inscricaoID)
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
	
	err = academia.AprovarInscricao(
		estudanteDTO.ID,
		inscricaoID,
		req.Tipo,
		req.AnoInscricao,
		req.CursoID,
	)
	
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(academia); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Inscrição aprovada: %s - %s", req.CodigoEstudante, req.Tipo)

	c.JSON(http.StatusOK, gin.H{
		"message":   "inscrição aprovada com sucesso",
		"estudante": req.CodigoEstudante,
		"tipo":      req.Tipo,
	})
}

func ReprovarInscricao(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		CodigoEstudante string `json:"codigo_estudante" binding:"required"`
		Motivo          string `json:"motivo" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("codigo_estudante e motivo são obrigatórios"))
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
	safeCodEst := db.SafeString(req.CodigoEstudante)
	safeCodAcad := db.SafeString(academiaDTO.CodigoAcademia)

	queryInsc := fmt.Sprintf(`
		SELECT id FROM projection_inscricoes 
		WHERE codigo_estudante = '%s' 
		AND codigo_academia = '%s' 
		AND status = 'espera'
		LIMIT 1
	`, safeCodEst, safeCodAcad)

	var inscricaoID uuid.UUID
	err = client.DB().QueryRow(queryInsc).Scan(&inscricaoID)
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
	
	err = academia.ReprovarInscricao(
		estudanteDTO.ID,
		inscricaoID,
		req.Motivo,
	)
	
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(academia); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Inscrição reprovada: %s - Motivo: %s", req.CodigoEstudante, req.Motivo)

	c.JSON(http.StatusOK, gin.H{
		"message":   "inscrição reprovada",
		"estudante": req.CodigoEstudante,
		"motivo":    req.Motivo,
	})
}

func InscreverEstudante(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		CodigoAcademia string     `json:"codigo_academia" binding:"required"`
		Tipo           string     `json:"tipo" binding:"required"`
		AnoInscricao   string     `json:"ano_inscricao" binding:"required"`
		CursoID        *uuid.UUID `json:"curso_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("dados obrigatórios: codigo_academia, tipo e ano_inscricao"))
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByCodigo(req.CodigoAcademia)
	if err != nil || academiaDTO == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	if academiaDTO.Status != "ativo" {
		utils.RespondWithValidationError(c, fmt.Errorf("academia não está ativa"))
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

	repository := getRepository(c)
	estudanteAgg, err := repository.Load(userID, "Estudante")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)
	
	err = estudante.SolicitarInscricao(
		req.CodigoAcademia,
		req.Tipo,
		req.AnoInscricao,
		req.CursoID,
	)

	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(estudante); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Inscrição criada: %s em %s", estudante.CodigoEstudante, academiaDTO.Nome)

	c.JSON(http.StatusCreated, gin.H{
		"message":  "inscrição criada com sucesso",
		"status":   "espera",
		"academia": academiaDTO.Nome,
	})
}