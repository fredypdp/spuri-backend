package handlers

import (
	"fmt"
	"log"
	"net/http"
	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RegistrarNotas registra notas para um estudante
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	// Buscar academia
	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "academia não encontrada"})
		return
	}

	// Buscar estudante
	estudanteProj := getEstudanteProjection(c)
	estudanteDTO, err := estudanteProj.GetByCodigo(req.CodigoEstudante)
	if err != nil || estudanteDTO == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}

	// Verificar vínculo
	if estudanteDTO.CodigoAcademia == nil || *estudanteDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
		c.JSON(http.StatusForbidden, gin.H{"error": "estudante não pertence a esta academia"})
		return
	}

	// Validar matéria
	materiaID, err := uuid.Parse(req.MateriaDisciplinarID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "materia_disciplinar_id inválido"})
		return
	}

	materiasProj := getMateriasProjection(c)
	materiaDTO, _ := materiasProj.GetByID(materiaID)
	if materiaDTO == nil || materiaDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
		c.JSON(http.StatusForbidden, gin.H{"error": "matéria não pertence a esta academia"})
		return
	}

	// Carregar e registrar nota
	repository := getRepository(c)
	estudanteAgg, err := repository.Load(estudanteDTO.ID, "Estudante")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao carregar estudante"})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(estudante); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao salvar notas"})
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

// RegistrarFaltas registra faltas para um estudante
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	// Parse data
	data, err := time.Parse("2006-01-02", req.Data)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "formato de data inválido (use YYYY-MM-DD)"})
		return
	}

	// Buscar academia
	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "academia não encontrada"})
		return
	}

	// Buscar estudante
	estudanteProj := getEstudanteProjection(c)
	estudanteDTO, err := estudanteProj.GetByCodigo(req.CodigoEstudante)
	if err != nil || estudanteDTO == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}

	// Verificar vínculo
	if estudanteDTO.CodigoAcademia == nil || *estudanteDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
		c.JSON(http.StatusForbidden, gin.H{"error": "estudante não pertence a esta academia"})
		return
	}

	// Validar matéria
	materiaID, err := uuid.Parse(req.MateriaDisciplinarID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "materia_disciplinar_id inválido"})
		return
	}

	materiasProj := getMateriasProjection(c)
	materiaDTO, _ := materiasProj.GetByID(materiaID)
	if materiaDTO == nil || materiaDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
		c.JSON(http.StatusForbidden, gin.H{"error": "matéria não pertence a esta academia"})
		return
	}

	// Carregar e registrar falta
	repository := getRepository(c)
	estudanteAgg, err := repository.Load(estudanteDTO.ID, "Estudante")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao carregar estudante"})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(estudante); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao salvar faltas"})
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

// 🔥 ATUALIZADO
func AprovarInscricao(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		CodigoEstudante string     `json:"codigo_estudante" binding:"required"`
		Tipo            string     `json:"tipo" binding:"required"`
		AnoInscricao    string     `json:"ano_inscricao" binding:"required"`
		CursoID         *uuid.UUID `json:"curso_id"` // 🔥 MUDOU
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	// Buscar academia
	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "academia não encontrada"})
		return
	}

	// Buscar estudante
	estudanteProj := getEstudanteProjection(c)
	estudanteDTO, err := estudanteProj.GetByCodigo(req.CodigoEstudante)
	if err != nil || estudanteDTO == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}

	// 🔥 VALIDAR CURSO SE FORNECIDO
	if req.CursoID != nil && *req.CursoID != uuid.Nil {
		cursosProj := getCursosProjection(c)
		curso, _ := cursosProj.GetByID(*req.CursoID)
		if curso == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "curso_id não encontrado"})
			return
		}
		if curso.CodigoAcademia != academiaDTO.CodigoAcademia {
			c.JSON(http.StatusForbidden, gin.H{"error": "curso não pertence a esta academia"})
			return
		}
		if curso.Status != "ativo" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "curso está inativo"})
			return
		}
	}

	// Buscar inscrição pendente
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
		c.JSON(http.StatusNotFound, gin.H{"error": "inscrição não encontrada ou já processada"})
		return
	}

	// Carregar e aprovar
	repository := getRepository(c)
	academiaAgg, err := repository.Load(academiaDTO.ID, "Academia")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao carregar academia"})
		return
	}

	academia := academiaAgg.(*aggregates.Academia)
	
	// 🔥 MUDOU - passar UUID em vez de string
	err = academia.AprovarInscricao(
		estudanteDTO.ID,
		inscricaoID,
		req.Tipo,
		req.AnoInscricao,
		req.CursoID, // 🔥 MUDOU
	)
	
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(academia); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao aprovar inscrição"})
		return
	}

	log.Printf("Inscrição aprovada: %s - %s", req.CodigoEstudante, req.Tipo)

	c.JSON(http.StatusOK, gin.H{
		"message":   "inscrição aprovada com sucesso",
		"estudante": req.CodigoEstudante,
		"tipo":      req.Tipo,
	})
}

// ReprovarInscricao reprova uma inscrição de estudante
func ReprovarInscricao(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		CodigoEstudante string `json:"codigo_estudante" binding:"required"`
		Motivo          string `json:"motivo" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	// Buscar academia
	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "academia não encontrada"})
		return
	}

	// Buscar estudante
	estudanteProj := getEstudanteProjection(c)
	estudanteDTO, err := estudanteProj.GetByCodigo(req.CodigoEstudante)
	if err != nil || estudanteDTO == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}

	// Buscar inscrição pendente
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
		c.JSON(http.StatusNotFound, gin.H{"error": "inscrição não encontrada ou já processada"})
		return
	}

	// Carregar e reprovar
	repository := getRepository(c)
	academiaAgg, err := repository.Load(academiaDTO.ID, "Academia")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao carregar academia"})
		return
	}

	academia := academiaAgg.(*aggregates.Academia)
	
	err = academia.ReprovarInscricao(
		estudanteDTO.ID,
		inscricaoID,
		req.Motivo,
	)
	
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(academia); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao reprovar inscrição"})
		return
	}

	log.Printf("Inscrição reprovada: %s - Motivo: %s", req.CodigoEstudante, req.Motivo)

	c.JSON(http.StatusOK, gin.H{
		"message":   "inscrição reprovada",
		"estudante": req.CodigoEstudante,
		"motivo":    req.Motivo,
	})
}

// 🔥 ATUALIZADO
func InscreverEstudante(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		CodigoAcademia string     `json:"codigo_academia" binding:"required"`
		Tipo           string     `json:"tipo" binding:"required"`
		AnoInscricao   string     `json:"ano_inscricao" binding:"required"`
		CursoID        *uuid.UUID `json:"curso_id"` // 🔥 MUDOU
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	// Buscar academia
	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByCodigo(req.CodigoAcademia)
	if err != nil || academiaDTO == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "academia não encontrada"})
		return
	}

	if academiaDTO.Status != "ativo" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "academia não está ativa"})
		return
	}

	// 🔥 VALIDAR CURSO SE FORNECIDO
	if req.CursoID != nil && *req.CursoID != uuid.Nil {
		cursosProj := getCursosProjection(c)
		curso, _ := cursosProj.GetByID(*req.CursoID)
		if curso == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "curso_id não encontrado"})
			return
		}
		if curso.CodigoAcademia != academiaDTO.CodigoAcademia {
			c.JSON(http.StatusForbidden, gin.H{"error": "curso não pertence a esta academia"})
			return
		}
		if curso.Status != "ativo" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "curso está inativo"})
			return
		}
	}

	// Carregar e inscrever
	repository := getRepository(c)
	estudanteAgg, err := repository.Load(userID, "Estudante")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao carregar estudante"})
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)
	
	// 🔥 MUDOU - passar UUID
	err = estudante.SolicitarInscricao(
		req.CodigoAcademia,
		req.Tipo,
		req.AnoInscricao,
		req.CursoID, // 🔥 MUDOU
	)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(estudante); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao salvar inscrição"})
		return
	}

	log.Printf("Inscrição criada: %s em %s", estudante.CodigoEstudante, academiaDTO.Nome)

	c.JSON(http.StatusCreated, gin.H{
		"message":  "inscrição criada com sucesso",
		"status":   "espera",
		"academia": academiaDTO.Nome,
	})
}