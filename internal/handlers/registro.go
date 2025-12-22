package handlers

import (
	"log"
	"net/http"
	"spuri/internal/domain"
	"spuri/internal/middleware"
	"spuri/internal/store"

	"github.com/gin-gonic/gin"
)

// RegistrarNotas registra as notas de um estudante
// Este é um exemplo clássico de CQRS: o comando não retorna o estado, apenas confirmação
func RegistrarNotas(c *gin.Context) {
	// Apenas academias podem registrar notas
	userID, _ := middleware.GetUserID(c)
	
	log.Printf("🔍 [RegistrarNotas] Academia ID: %s", userID)

	var req domain.RegistrarNotasRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("❌ Erro ao parsear JSON: %v", err)
		c.JSON(http.StatusBadRequest, domain.ErrorResponse{Error: "dados inválidos"})
		return
	}
	
	log.Printf("🔍 [RegistrarNotas] Estudante ID: %s", req.EstudanteID)

	// Validações
	if len(req.Materias) == 0 {
		c.JSON(http.StatusBadRequest, domain.ErrorResponse{Error: "materias não pode estar vazio"})
		return
	}

	// Verificar se o estudante existe
	estudante, err := store.GetEstudanteByID(req.EstudanteID)
	if err != nil {
		log.Printf("❌ Erro ao buscar estudante: %v", err)
		c.JSON(http.StatusInternalServerError, domain.ErrorResponse{Error: "erro ao buscar estudante"})
		return
	}
	if estudante == nil {
		log.Printf("❌ Estudante não encontrado")
		c.JSON(http.StatusNotFound, domain.ErrorResponse{Error: "estudante não encontrado"})
		return
	}
	
	log.Printf("🔍 [RegistrarNotas] Estudante encontrado! id_academia: %v", estudante.IDAcademia)

	// Verificar se o estudante pertence a esta academia
	if estudante.IDAcademia == nil || *estudante.IDAcademia != userID {
		log.Printf("❌ Estudante não pertence à academia. id_academia=%v, userID=%s", estudante.IDAcademia, userID)
		c.JSON(http.StatusForbidden, domain.ErrorResponse{Error: "estudante não pertence a esta academia"})
		return
	}
	
	log.Printf("✅ Estudante pertence à academia!")

	// Criar o registro de notas (read model)
	registro := &domain.RegistroNotas{
		EstudanteID: req.EstudanteID,
		IDAcademia:  userID,
		AnoLectivo:  req.AnoLectivo,
		Periodo:     req.Periodo,
		Materias:    req.Materias,
	}

	// Criar o evento (Event Sourcing)
	payload := domain.NotasRegistradasPayload{
		IDAcademia:  userID,
		AnoLectivo:  req.AnoLectivo,
		Periodo:     req.Periodo,
		Materias:    req.Materias,
	}

	metadata := domain.EventMetadata{
		ActorID:   userID,
		ActorType: "Academia",
		IP:        c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	}

	event, err := domain.NewEvent(
		req.EstudanteID,
		domain.AggregateTypeEstudante,
		domain.EventTypeNotasRegistradas,
		payload,
		metadata,
	)

	if err != nil {
		log.Printf("❌ Erro ao criar evento: %v", err)
		c.JSON(http.StatusInternalServerError, domain.ErrorResponse{Error: "erro ao criar evento"})
		return
	}

	// Salvar evento + projeção em transação atômica
	if err := store.SaveNotasWithEvent(registro, event); err != nil {
		log.Printf("❌ Erro ao registrar notas: %v", err)
		c.JSON(http.StatusInternalServerError, domain.ErrorResponse{Error: "erro ao registrar notas"})
		return
	}
	
	log.Printf("✅ Notas registradas com sucesso!")

	// CQRS: Não retornamos o estado, apenas confirmação
	c.JSON(http.StatusOK, domain.SuccessResponse{
		Message: "notas registradas com sucesso",
		Data: map[string]interface{}{
			"event_id": event.EventID,
		},
	})
}

// RegistrarFaltas registra as faltas de um estudante
func RegistrarFaltas(c *gin.Context) {
	// Apenas academias podem registrar faltas
	userID, _ := middleware.GetUserID(c)

	var req domain.RegistrarFaltasRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, domain.ErrorResponse{Error: "dados inválidos"})
		return
	}

	// Validações
	if len(req.Materias) == 0 {
		c.JSON(http.StatusBadRequest, domain.ErrorResponse{Error: "materias não pode estar vazio"})
		return
	}

	// Verificar se o estudante existe
	estudante, err := store.GetEstudanteByID(req.EstudanteID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, domain.ErrorResponse{Error: "erro ao buscar estudante"})
		return
	}
	if estudante == nil {
		c.JSON(http.StatusNotFound, domain.ErrorResponse{Error: "estudante não encontrado"})
		return
	}

	// Verificar se o estudante pertence a esta academia
	if estudante.IDAcademia == nil || *estudante.IDAcademia != userID {
		c.JSON(http.StatusForbidden, domain.ErrorResponse{Error: "estudante não pertence a esta academia"})
		return
	}

	// Criar o registro de faltas (read model)
	registro := &domain.RegistroFaltas{
		EstudanteID: req.EstudanteID,
		IDAcademia:  userID,
		AnoLectivo:  req.AnoLectivo,
		Periodo:     req.Periodo,
		Materias:    req.Materias,
	}

	// Criar o evento (Event Sourcing)
	payload := domain.FaltasRegistradasPayload{
		IDAcademia:  userID,
		AnoLectivo:  req.AnoLectivo,
		Periodo:     req.Periodo,
		Materias:    req.Materias,
	}

	metadata := domain.EventMetadata{
		ActorID:   userID,
		ActorType: "Academia",
		IP:        c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	}

	event, err := domain.NewEvent(
		req.EstudanteID,
		domain.AggregateTypeEstudante,
		domain.EventTypeFaltasRegistradas,
		payload,
		metadata,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, domain.ErrorResponse{Error: "erro ao criar evento"})
		return
	}

	// Salvar evento + projeção em transação atômica
	if err := store.SaveFaltasWithEvent(registro, event); err != nil {
		c.JSON(http.StatusInternalServerError, domain.ErrorResponse{Error: "erro ao registrar faltas"})
		return
	}

	// CQRS: Não retornamos o estado, apenas confirmação
	c.JSON(http.StatusOK, domain.SuccessResponse{
		Message: "faltas registradas com sucesso",
		Data: map[string]interface{}{
			"event_id": event.EventID,
		},
	})
}