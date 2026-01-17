// ============================================================================
// ARQUIVO: internal/handlers/command_handlers.go
// ATUALIZADO: Usar codigo_academia em vez de id_academia
// ============================================================================

package handlers

import (
	"bytes"
	"io"
	"fmt"
	"log"
	"net/http"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RegistrarNotasRequest representa requisição de registro de notas
type RegistrarNotasRequest struct {
	CodigoEstudante string `json:"codigo_estudante" binding:"required"`
	AnoLectivo      string `json:"ano_lectivo" binding:"required"`
	Periodo         string `json:"periodo" binding:"required"`
	Materias        []struct {
		Nome string  `json:"nome" binding:"required"`
		Nota float64 `json:"nota" binding:"required"`
	} `json:"materias" binding:"required"`
}

// 🔥 ATUALIZADO: Usar codigo_academia
func RegistrarNotas(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req RegistrarNotasRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	if len(req.Materias) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "materias não pode estar vazio"})
		return
	}

	// Buscar estudante
	estudanteProj := getEstudanteProjection(c)
	estudanteDTO, err := estudanteProj.GetByCodigo(req.CodigoEstudante)
	if err != nil || estudanteDTO == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}

	// Buscar código da academia logada
	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar academia"})
		return
	}

	repository := getRepository(c)
	estudanteAgg, err := repository.Load(estudanteDTO.ID, "Estudante")
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)

	// 🔥 ATUALIZADO: Verificar codigo_academia em vez de ID
	if estudante.CodigoAcademia == nil || *estudante.CodigoAcademia != academiaDTO.CodigoAcademia {
		c.JSON(http.StatusForbidden, gin.H{"error": "estudante não pertence a esta academia"})
		return
	}

	materias := make([]aggregates.Materia, len(req.Materias))
	for i, m := range req.Materias {
		materias[i] = aggregates.Materia{
			Nome: m.Nome,
			Nota: m.Nota,
		}
	}

	// 🔥 ATUALIZADO: Passar codigo_academia
	err = estudante.RegistrarNotas(academiaDTO.CodigoAcademia, req.AnoLectivo, req.Periodo, materias)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(estudante); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao registrar notas"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":          "notas registradas com sucesso",
		"codigo_estudante": req.CodigoEstudante,
	})
}

// RegistrarFaltasRequest representa requisição de registro de faltas
type RegistrarFaltasRequest struct {
	CodigoEstudante string `json:"codigo_estudante" binding:"required"`
	AnoLectivo      string `json:"ano_lectivo" binding:"required"`
	Periodo         string `json:"periodo" binding:"required"`
	Materias        []struct {
		Nome   string `json:"nome" binding:"required"`
		Faltas int    `json:"faltas" binding:"required"`
	} `json:"materias" binding:"required"`
}

// 🔥 ATUALIZADO: Usar codigo_academia
func RegistrarFaltas(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req RegistrarFaltasRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	if len(req.Materias) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "materias não pode estar vazio"})
		return
	}

	// Buscar estudante
	estudanteProj := getEstudanteProjection(c)
	estudanteDTO, err := estudanteProj.GetByCodigo(req.CodigoEstudante)
	if err != nil || estudanteDTO == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}

	// Buscar código da academia logada
	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar academia"})
		return
	}

	repository := getRepository(c)
	estudanteAgg, err := repository.Load(estudanteDTO.ID, "Estudante")
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)

	// 🔥 ATUALIZADO: Verificar codigo_academia
	if estudante.CodigoAcademia == nil || *estudante.CodigoAcademia != academiaDTO.CodigoAcademia {
		c.JSON(http.StatusForbidden, gin.H{"error": "estudante não pertence a esta academia"})
		return
	}

	materias := make([]aggregates.MateriaFaltas, len(req.Materias))
	for i, m := range req.Materias {
		materias[i] = aggregates.MateriaFaltas{
			Nome:   m.Nome,
			Faltas: m.Faltas,
		}
	}

	// 🔥 ATUALIZADO: Passar codigo_academia
	err = estudante.RegistrarFaltas(academiaDTO.CodigoAcademia, req.AnoLectivo, req.Periodo, materias)
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
	})
}

// InscricaoEscolaRequest representa solicitação de inscrição em escola
type InscricaoEscolaRequest struct {
	CodigoAcademia      string  `json:"codigo_academia" binding:"required"`
	AnoEscolarInscricao string  `json:"ano_escolar_inscricao" binding:"required"`
	CursoMedio          *string `json:"curso_medio"`
}

func InscricaoEscola(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	// 🔥 DEBUG: Ver o que está chegando
	bodyBytes, _ := c.GetRawData()
	log.Printf("📥 [INSCRICAO] Raw body: %s", string(bodyBytes))
	
	// 🔥 Restaurar o body para o ShouldBindJSON funcionar
	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	var req InscricaoEscolaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("❌ [INSCRICAO] Erro no bind: %v", err)
		log.Printf("❌ [INSCRICAO] Request recebido: %+v", req)
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("dados inválidos: %v", err)})
		return
	}

	log.Printf("✅ [INSCRICAO] Request válido: %+v", req)
	log.Printf("📋 [INSCRICAO] UserID: %s", userID)

	// Verificar se academia existe
	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByCodigo(req.CodigoAcademia)
	if err != nil || academiaDTO == nil {
		log.Printf("❌ [INSCRICAO] Academia não encontrada: %s", req.CodigoAcademia)
		c.JSON(http.StatusNotFound, gin.H{"error": "academia não encontrada"})
		return
	}

	log.Printf("✅ [INSCRICAO] Academia encontrada: %s", academiaDTO.Nome)

	repository := getRepository(c)
	estudanteAgg, err := repository.Load(userID, "Estudante")
	if err != nil {
		log.Printf("❌ [INSCRICAO] Erro ao carregar estudante: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)

	// 🔥 ATUALIZADO: Passar codigo_academia
	log.Printf("⚡ [INSCRICAO] Executando SolicitarInscricao...")
	err = estudante.SolicitarInscricao(
		req.CodigoAcademia,
		"escola",
		req.AnoEscolarInscricao,
		req.CursoMedio,
	)

	if err != nil {
		log.Printf("❌ [INSCRICAO] Erro no comando: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("💾 [INSCRICAO] Salvando eventos...")
	if err := repository.Save(estudante); err != nil {
		log.Printf("❌ [INSCRICAO] Erro ao salvar: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao solicitar inscrição"})
		return
	}

	log.Printf("✅ [INSCRICAO] Inscrição criada com sucesso!")
	c.JSON(http.StatusCreated, gin.H{
		"message":         "inscrição solicitada com sucesso",
		"codigo_academia": req.CodigoAcademia,
	})
}

// InscricaoUniversidadeRequest representa solicitação de inscrição em universidade
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

	// Verificar se academia existe
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

	// 🔥 ATUALIZADO: Passar codigo_academia
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

// AprovarInscricao comando para aprovar inscrição
func AprovarInscricao(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	inscricaoID, err := parseUUID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID de inscrição inválido"})
		return
	}

	// Buscar inscrição
	inscProj := getInscricoesProjection(c)
	inscricao, err := inscProj.GetByID(inscricaoID)
	if err != nil || inscricao == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "inscrição não encontrada"})
		return
	}

	// Validações de status
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

	// Verificar se pertence a esta academia
	if inscricao.AcademiaID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "inscrição não pertence a esta academia"})
		return
	}

	// Buscar código da academia
	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar academia"})
		return
	}

	repository := getRepository(c)

	// Carregar estudante e aprovar
	estudanteAgg, err := repository.Load(inscricao.EstudanteID, "Estudante")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao carregar estudante"})
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)
	
	// 🔥 ATUALIZADO: Passar codigo_academia
	err = estudante.AprovarInscricao(academiaDTO.CodigoAcademia, inscricaoID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(estudante); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao aprovar inscrição"})
		return
	}

	// Carregar academia e registrar aprovação
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

// ReprovarInscricao comando para reprovar inscrição
func ReprovarInscricao(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	inscricaoID, err := parseUUID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID de inscrição inválido"})
		return
	}

	// Buscar inscrição
	inscProj := getInscricoesProjection(c)
	inscricao, err := inscProj.GetByID(inscricaoID)
	if err != nil || inscricao == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "inscrição não encontrada"})
		return
	}

	// Verificar se já foi processada
	if inscricao.Status != "espera" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":        fmt.Sprintf("inscrição já foi processada com status '%s'", inscricao.Status),
			"status_atual": inscricao.Status,
		})
		return
	}

	// Verificar permissão
	if inscricao.AcademiaID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "inscrição não pertence a esta academia"})
		return
	}

	// Buscar código da academia
	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar academia"})
		return
	}

	repository := getRepository(c)

	// Carregar estudante
	log.Printf("📝 [REPROVAR] Carregando agregado Estudante: %s", inscricao.EstudanteID)
	estudanteAgg, err := repository.Load(inscricao.EstudanteID, "Estudante")
	if err != nil {
		log.Printf("❌ [REPROVAR] Erro ao carregar estudante: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao carregar estudante"})
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)

	// Aplicar evento de reprovação
	log.Printf("⚡ [REPROVAR] Aplicando reprovação ao agregado Estudante...")
	reprovadoEvent := &aggregates.InscricaoReprovadaEvent{
		BaseEvent: aggregates.BaseEvent{
			EventType:   "InscricaoReprovada",
			AggregateID: estudante.ID,
		},
		InscricaoID:    inscricaoID,
		CodigoAcademia: academiaDTO.CodigoAcademia, // 🔥 ATUALIZADO
	}

	if err := estudante.Apply(reprovadoEvent); err != nil {
		log.Printf("❌ [REPROVAR] Erro ao aplicar evento: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao processar reprovação"})
		return
	}

	estudante.RaiseEvent(reprovadoEvent)

	// Salvar eventos do estudante
	log.Printf("💾 [REPROVAR] Salvando eventos do estudante...")
	if err := repository.Save(estudante); err != nil {
		log.Printf("❌ [REPROVAR] Erro ao salvar estudante: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao salvar reprovação"})
		return
	}

	// Carregar academia
	log.Printf("📝 [REPROVAR] Carregando agregado Academia: %s", userID)
	academiaAgg, err := repository.Load(userID, "Academia")
	if err != nil {
		log.Printf("❌ [REPROVAR] Erro ao carregar academia: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao carregar academia"})
		return
	}

	academia := academiaAgg.(*aggregates.Academia)

	log.Printf("⚡ [REPROVAR] Executando comando ReprovarInscricao na academia...")
	err = academia.ReprovarInscricao(inscricao.EstudanteID, inscricaoID, "")
	if err != nil {
		log.Printf("❌ [REPROVAR] Erro no comando: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Salvar eventos da academia
	log.Printf("💾 [REPROVAR] Salvando eventos da academia...")
	if err := repository.Save(academia); err != nil {
		log.Printf("❌ [REPROVAR] Erro ao salvar academia: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao registrar reprovação"})
		return
	}

	log.Printf("✅ [REPROVAR] Inscrição reprovada com sucesso: %s", inscricaoID)
	c.JSON(http.StatusOK, gin.H{
		"message":          "inscrição reprovada com sucesso",
		"codigo_estudante": inscricao.CodigoEstudante,
		"status":           "reprovado",
	})
}

// Helper para parse UUID
func parseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}