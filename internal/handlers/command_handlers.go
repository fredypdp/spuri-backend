package handlers

import (
	"bytes"
	"io"
	"fmt"
	"log"
	"net/http"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ============================================
// 🔥 NOTAS - ESTRUTURA v3.0
// ============================================

type RegistrarNotasRequest struct {
	CodigoEstudante      string   `json:"codigo_estudante" binding:"required"`
	AnoLectivo           string   `json:"ano_lectivo" binding:"required"`
	Periodo              string   `json:"periodo" binding:"required"`
	MateriaDisciplinarID string   `json:"materia_disciplinar_id" binding:"required"`
	Nota                 float64  `json:"nota" binding:"required"`
	Observacao           *string  `json:"observacao"`
}

func RegistrarNotas(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req RegistrarNotasRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	// Validar nota
	if req.Nota < 0 || req.Nota > 20 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "nota deve estar entre 0 e 20"})
		return
	}

	// Validar UUID da matéria
	materiaID, err := uuid.Parse(req.MateriaDisciplinarID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID de matéria inválido"})
		return
	}

	// Buscar estudante
	estudanteProj := getEstudanteProjection(c)
	estudanteDTO, err := estudanteProj.GetByCodigo(req.CodigoEstudante)
	if err != nil || estudanteDTO == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}

	// Buscar academia logada
	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar academia"})
		return
	}

	// Verificar se matéria pertence à academia
	materiasProj := getMateriasProjection(c)
	materiaDTO, err := materiasProj.GetByID(materiaID)
	if err != nil || materiaDTO == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "matéria não encontrada"})
		return
	}

	if materiaDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
		c.JSON(http.StatusForbidden, gin.H{"error": "matéria não pertence a esta academia"})
		return
	}

	// Carregar agregado
	repository := getRepository(c)
	estudanteAgg, err := repository.Load(estudanteDTO.ID, "Estudante")
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)

	// Verificar se pertence à academia
	if estudante.CodigoAcademia == nil || *estudante.CodigoAcademia != academiaDTO.CodigoAcademia {
		c.JSON(http.StatusForbidden, gin.H{"error": "estudante não pertence a esta academia"})
		return
	}

	// Registrar nota
	err = estudante.RegistrarNota(
		academiaDTO.CodigoAcademia,
		req.AnoLectivo,
		req.Periodo,
		materiaID,
		req.Nota,
		req.Observacao,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(estudante); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao registrar nota"})
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

// ============================================
// 🔥 FALTAS - ESTRUTURA v3.0
// ============================================

type RegistrarFaltasRequest struct {
	CodigoEstudante      string  `json:"codigo_estudante" binding:"required"`
	AnoLectivo           string  `json:"ano_lectivo" binding:"required"`
	Data                 string  `json:"data" binding:"required"` // formato: "2024-01-15"
	MateriaDisciplinarID string  `json:"materia_disciplinar_id" binding:"required"`
	Quantidade           int     `json:"quantidade" binding:"required"`
	Observacao           *string `json:"observacao"`
}

func RegistrarFaltas(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req RegistrarFaltasRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	// Validar quantidade
	if req.Quantidade <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "quantidade deve ser maior que zero"})
		return
	}

	// Parsear data
	data, err := time.Parse("2006-01-02", req.Data)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "formato de data inválido (use YYYY-MM-DD)"})
		return
	}

	// Validar UUID da matéria
	materiaID, err := uuid.Parse(req.MateriaDisciplinarID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID de matéria inválido"})
		return
	}

	// Buscar estudante
	estudanteProj := getEstudanteProjection(c)
	estudanteDTO, err := estudanteProj.GetByCodigo(req.CodigoEstudante)
	if err != nil || estudanteDTO == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}

	// Buscar academia logada
	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar academia"})
		return
	}

	// Verificar se matéria pertence à academia
	materiasProj := getMateriasProjection(c)
	materiaDTO, err := materiasProj.GetByID(materiaID)
	if err != nil || materiaDTO == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "matéria não encontrada"})
		return
	}

	if materiaDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
		c.JSON(http.StatusForbidden, gin.H{"error": "matéria não pertence a esta academia"})
		return
	}

	// Carregar agregado
	repository := getRepository(c)
	estudanteAgg, err := repository.Load(estudanteDTO.ID, "Estudante")
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)

	// Verificar se pertence à academia
	if estudante.CodigoAcademia == nil || *estudante.CodigoAcademia != academiaDTO.CodigoAcademia {
		c.JSON(http.StatusForbidden, gin.H{"error": "estudante não pertence a esta academia"})
		return
	}

	// Registrar falta
	err = estudante.RegistrarFalta(
		academiaDTO.CodigoAcademia,
		req.AnoLectivo,
		data,
		materiaID,
		req.Quantidade,
		req.Observacao,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(estudante); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao registrar faltas"})
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

// ============================================
// INSCRIÇÕES
// ============================================

type InscricaoEscolaRequest struct {
	CodigoAcademia      string  `json:"codigo_academia" binding:"required"`
	AnoEscolarInscricao string  `json:"ano_escolar_inscricao" binding:"required"`
	CursoMedio          *string `json:"curso_medio"`
}

func InscricaoEscola(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	bodyBytes, _ := c.GetRawData()
	log.Printf("🔥 [INSCRICAO] Raw body: %s", string(bodyBytes))
	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	var req InscricaoEscolaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("❌ [INSCRICAO] Erro no bind: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("dados inválidos: %v", err)})
		return
	}

	// Verificar se academia existe
	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByCodigo(req.CodigoAcademia)
	if err != nil || academiaDTO == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "academia não encontrada"})
		return
	}

	// Verificar inscrição pendente
	inscProj := getInscricoesProjection(c)
	inscricoes, err := inscProj.GetByEstudante(userID)
	if err == nil {
		for _, insc := range inscricoes {
			if insc.CodigoAcademia == req.CodigoAcademia && insc.Status == "espera" {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": "você já possui uma inscrição pendente nesta academia",
				})
				return
			}
		}
	}

	repository := getRepository(c)
	estudanteAgg, err := repository.Load(userID, "Estudante")
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(estudante); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao solicitar inscrição"})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByCodigo(req.CodigoAcademia)
	if err != nil || academiaDTO == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "academia não encontrada"})
		return
	}

	repository := getRepository(c)
	estudanteAgg, err := repository.Load(userID, "Estudante")
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(estudante); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao solicitar inscrição"})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID de inscrição inválido"})
		return
	}

	inscProj := getInscricoesProjection(c)
	inscricao, err := inscProj.GetByID(inscricaoID)
	if err != nil || inscricao == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "inscrição não encontrada"})
		return
	}

	switch inscricao.Status {
	case "aprovado":
		c.JSON(http.StatusBadRequest, gin.H{
			"error":        "esta inscrição já foi aprovada anteriormente",
			"status_atual": "aprovado",
		})
		return
	case "reprovado":
		c.JSON(http.StatusBadRequest, gin.H{
			"error":        "esta inscrição foi reprovada e não pode ser aprovada",
			"status_atual": "reprovado",
		})
		return
	}

	if inscricao.AcademiaID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "inscrição não pertence a esta academia"})
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar academia"})
		return
	}

	repository := getRepository(c)

	estudanteAgg, err := repository.Load(inscricao.EstudanteID, "Estudante")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao carregar estudante"})
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)
	
	err = estudante.AprovarInscricao(academiaDTO.CodigoAcademia, inscricaoID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(estudante); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao aprovar inscrição"})
		return
	}

	academiaAgg, err := repository.Load(userID, "Academia")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao carregar academia"})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(academia); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao registrar aprovação"})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID de inscrição inválido"})
		return
	}

	inscProj := getInscricoesProjection(c)
	inscricao, err := inscProj.GetByID(inscricaoID)
	if err != nil || inscricao == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "inscrição não encontrada"})
		return
	}

	if inscricao.Status != "espera" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":        fmt.Sprintf("inscrição já foi processada com status '%s'", inscricao.Status),
			"status_atual": inscricao.Status,
		})
		return
	}

	if inscricao.AcademiaID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "inscrição não pertence a esta academia"})
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar academia"})
		return
	}

	repository := getRepository(c)

	estudanteAgg, err := repository.Load(inscricao.EstudanteID, "Estudante")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao carregar estudante"})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao processar reprovação"})
		return
	}

	estudante.RaiseEvent(reprovadoEvent)

	if err := repository.Save(estudante); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao salvar reprovação"})
		return
	}

	academiaAgg, err := repository.Load(userID, "Academia")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao carregar academia"})
		return
	}

	academia := academiaAgg.(*aggregates.Academia)

	err = academia.ReprovarInscricao(inscricao.EstudanteID, inscricaoID, "")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(academia); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao registrar reprovação"})
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