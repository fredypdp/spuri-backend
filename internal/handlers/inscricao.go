package handlers

import (
	"log"
	"net/http"
	"spuri/internal/domain"
	"spuri/internal/middleware"
	"spuri/internal/store"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// InscricaoEscola cria uma solicitação de inscrição em escola
func InscricaoEscola(c *gin.Context) {
	// Apenas estudantes podem solicitar inscrição
	userID, _ := middleware.GetUserID(c)

	var req domain.InscricaoEscolaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, domain.ErrorResponse{Error: "dados inválidos"})
		return
	}

	// Verificar se a academia existe
	academia, err := store.GetAcademiaByID(req.IDAcademia)
	if err != nil {
		c.JSON(http.StatusInternalServerError, domain.ErrorResponse{Error: "erro ao buscar academia"})
		return
	}
	if academia == nil {
		c.JSON(http.StatusNotFound, domain.ErrorResponse{Error: "academia não encontrada"})
		return
	}

	// Verificar se é uma escola
	if academia.Type != "escola" {
		c.JSON(http.StatusBadRequest, domain.ErrorResponse{Error: "academia não é uma escola"})
		return
	}

	// Criar inscrição
	inscricao := &domain.Inscricao{
		EstudanteID:  userID,
		AcademiaID:   req.IDAcademia,
		Tipo:         "escola",
		AnoInscricao: req.AnoEscolarInscricao,
		Curso:        req.CursoMedio,
		Status:       "espera",
	}

	if err := store.CreateInscricao(inscricao); err != nil {
		c.JSON(http.StatusInternalServerError, domain.ErrorResponse{Error: "erro ao criar inscrição"})
		return
	}

	// Criar evento
	payload := domain.EstudanteInscritoPayload{
		AcademiaID:   req.IDAcademia,
		Tipo:         "escola",
		AnoInscricao: req.AnoEscolarInscricao,
		Curso:        req.CursoMedio,
	}

	metadata := domain.EventMetadata{
		ActorID:   userID,
		ActorType: "Estudante",
		IP:        c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	}

	event, err := domain.NewEvent(
		userID,
		domain.AggregateTypeEstudante,
		domain.EventTypeEstudanteInscrito,
		payload,
		metadata,
	)

	if err == nil {
		store.SaveEvent(event)
	}

	c.JSON(http.StatusCreated, domain.SuccessResponse{
		Message: "inscrição solicitada com sucesso",
		Data: map[string]interface{}{
			"inscricao_id": inscricao.ID,
			"status":       inscricao.Status,
		},
	})
}

// InscricaoUniversidade cria uma solicitação de inscrição em universidade
func InscricaoUniversidade(c *gin.Context) {
	// Apenas estudantes podem solicitar inscrição
	userID, _ := middleware.GetUserID(c)

	var req domain.InscricaoUniversidadeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, domain.ErrorResponse{Error: "dados inválidos"})
		return
	}

	// Verificar se a academia existe
	academia, err := store.GetAcademiaByID(req.IDAcademia)
	if err != nil {
		c.JSON(http.StatusInternalServerError, domain.ErrorResponse{Error: "erro ao buscar academia"})
		return
	}
	if academia == nil {
		c.JSON(http.StatusNotFound, domain.ErrorResponse{Error: "academia não encontrada"})
		return
	}

	// Verificar se é universidade
	if academia.Type != "superior" {
		c.JSON(http.StatusBadRequest, domain.ErrorResponse{Error: "academia não é uma universidade"})
		return
	}

	// Criar inscrição
	inscricao := &domain.Inscricao{
		EstudanteID:  userID,
		AcademiaID:   req.IDAcademia,
		Tipo:         "universidade",
		AnoInscricao: req.AnoSuperiorInscricao,
		Curso:        &req.CursoSuperior,
		Status:       "espera",
	}

	if err := store.CreateInscricao(inscricao); err != nil {
		c.JSON(http.StatusInternalServerError, domain.ErrorResponse{Error: "erro ao criar inscrição"})
		return
	}

	// Criar evento
	payload := domain.EstudanteInscritoPayload{
		AcademiaID:   req.IDAcademia,
		Tipo:         "universidade",
		AnoInscricao: req.AnoSuperiorInscricao,
		Curso:        &req.CursoSuperior,
	}

	metadata := domain.EventMetadata{
		ActorID:   userID,
		ActorType: "Estudante",
		IP:        c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	}

	event, err := domain.NewEvent(
		userID,
		domain.AggregateTypeEstudante,
		domain.EventTypeEstudanteInscrito,
		payload,
		metadata,
	)

	if err == nil {
		store.SaveEvent(event)
	}

	c.JSON(http.StatusCreated, domain.SuccessResponse{
		Message: "inscrição solicitada com sucesso",
		Data: map[string]interface{}{
			"inscricao_id": inscricao.ID,
			"status":       inscricao.Status,
		},
	})
}

// ListarInscricoesPendentes lista inscrições pendentes de uma academia
func ListarInscricoesPendentes(c *gin.Context) {
	// Apenas academias podem listar suas inscrições
	userID, _ := middleware.GetUserID(c)

	inscricoes, err := store.GetInscricoesByAcademia(userID, "espera")
	if err != nil {
		c.JSON(http.StatusInternalServerError, domain.ErrorResponse{Error: "erro ao buscar inscrições"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"inscricoes": inscricoes,
		"total":      len(inscricoes),
	})
}

// AprovarInscricao aprova uma inscrição
func AprovarInscricao(c *gin.Context) {
	// Apenas academias podem aprovar inscrições
	userID, _ := middleware.GetUserID(c)

	log.Printf("🔍 [AprovarInscricao] Academia ID: %s", userID)

	inscricaoID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, domain.ErrorResponse{Error: "ID de inscrição inválido"})
		return
	}

	log.Printf("🔍 [AprovarInscricao] Inscrição ID: %s", inscricaoID)

	// Buscar inscrição (verificar se pertence a esta academia)
	inscricoes, err := store.GetInscricoesByAcademia(userID, "espera")
	if err != nil {
		log.Printf("❌ Erro ao buscar inscrições: %v", err)
		c.JSON(http.StatusInternalServerError, domain.ErrorResponse{Error: "erro ao buscar inscrição"})
		return
	}

	log.Printf("🔍 [AprovarInscricao] Inscrições encontradas: %d", len(inscricoes))

	var inscricao *domain.Inscricao
	for i := range inscricoes {
		if inscricoes[i].ID == inscricaoID {
			inscricao = &inscricoes[i]
			break
		}
	}

	if inscricao == nil {
		log.Printf("❌ Inscrição não encontrada na lista")
		c.JSON(http.StatusNotFound, domain.ErrorResponse{Error: "inscrição não encontrada"})
		return
	}

	log.Printf("✅ Inscrição encontrada! Estudante ID: %s", inscricao.EstudanteID)

	// Atualizar status da inscrição
	log.Printf("🔄 Atualizando status para 'aprovado'...")
	if err := store.UpdateInscricaoStatus(inscricaoID, "aprovado"); err != nil {
		log.Printf("❌ Erro ao atualizar status: %v", err)
		c.JSON(http.StatusInternalServerError, domain.ErrorResponse{Error: "erro ao aprovar inscrição"})
		return
	}
	log.Printf("✅ Status atualizado!")

	// Vincular estudante à academia
	log.Printf("🔗 Vinculando estudante %s à academia %s...", inscricao.EstudanteID, userID)
	if err := store.VincularEstudanteAcademia(inscricao.EstudanteID, userID); err != nil {
		log.Printf("❌ Erro ao vincular estudante: %v", err)
		c.JSON(http.StatusInternalServerError, domain.ErrorResponse{Error: "erro ao vincular estudante"})
		return
	}
	log.Printf("✅ Estudante vinculado!")

	// Criar evento
	payload := domain.InscricaoAprovadaPayload{
		InscricaoID:  inscricaoID,
		AcademiaID:   userID,
		Tipo:         inscricao.Tipo,
		AnoInscricao: inscricao.AnoInscricao,
		Curso:        inscricao.Curso,
	}

	metadata := domain.EventMetadata{
		ActorID:   userID,
		ActorType: "Academia",
		IP:        c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	}

	event, err := domain.NewEvent(
		inscricao.EstudanteID,
		domain.AggregateTypeEstudante,
		domain.EventTypeInscricaoAprovada,
		payload,
		metadata,
	)

	if err == nil {
		store.SaveEvent(event)
		log.Printf("✅ Evento criado!")
	}

	c.JSON(http.StatusOK, domain.SuccessResponse{
		Message: "inscrição aprovada com sucesso",
	})
}

// ReprovarInscricao reprova uma inscrição
func ReprovarInscricao(c *gin.Context) {
	// Apenas academias podem reprovar inscrições
	userID, _ := middleware.GetUserID(c)

	inscricaoID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, domain.ErrorResponse{Error: "ID de inscrição inválido"})
		return
	}

	// Buscar inscrição (verificar se pertence a esta academia)
	inscricoes, err := store.GetInscricoesByAcademia(userID, "espera")
	if err != nil {
		c.JSON(http.StatusInternalServerError, domain.ErrorResponse{Error: "erro ao buscar inscrição"})
		return
	}

	var inscricao *domain.Inscricao
	for i := range inscricoes {
		if inscricoes[i].ID == inscricaoID {
			inscricao = &inscricoes[i]
			break
		}
	}

	if inscricao == nil {
		c.JSON(http.StatusNotFound, domain.ErrorResponse{Error: "inscrição não encontrada"})
		return
	}

	// Atualizar status
	if err := store.UpdateInscricaoStatus(inscricaoID, "reprovado"); err != nil {
		c.JSON(http.StatusInternalServerError, domain.ErrorResponse{Error: "erro ao reprovar inscrição"})
		return
	}

	// Criar evento
	metadata := domain.EventMetadata{
		ActorID:   userID,
		ActorType: "Academia",
		IP:        c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	}

	event, err := domain.NewEvent(
		inscricao.EstudanteID,
		domain.AggregateTypeEstudante,
		domain.EventTypeInscricaoReprovada,
		map[string]interface{}{
			"inscricao_id": inscricaoID,
			"academia_id":  userID,
		},
		metadata,
	)

	if err == nil {
		store.SaveEvent(event)
	}

	c.JSON(http.StatusOK, domain.SuccessResponse{
		Message: "inscrição reprovada",
	})
}
