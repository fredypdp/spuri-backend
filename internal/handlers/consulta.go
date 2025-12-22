package handlers

import (
	"net/http"
	"spuri/internal/domain"
	"spuri/internal/middleware"
	"spuri/internal/store"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// GetNotasEstudante obtém o histórico de notas de um estudante
// Esta é a parte de Query do CQRS - apenas leitura das projeções
func GetNotasEstudante(c *gin.Context) {
	estudanteID, err := uuid.Parse(c.Param("estudanteId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, domain.ErrorResponse{Error: "ID de estudante inválido"})
		return
	}

	// Verificar permissões
	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	// Estudante pode ver suas próprias notas
	// Academia pode ver notas dos seus estudantes
	if userType == "estudante" && userID != estudanteID {
		c.JSON(http.StatusForbidden, domain.ErrorResponse{Error: "acesso negado"})
		return
	}

	if userType == "academia" {
		// Verificar se o estudante pertence a esta academia
		estudante, err := store.GetEstudanteByID(estudanteID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, domain.ErrorResponse{Error: "erro ao buscar estudante"})
			return
		}
		if estudante == nil {
			c.JSON(http.StatusNotFound, domain.ErrorResponse{Error: "estudante não encontrado"})
			return
		}
		if estudante.IDAcademia == nil || *estudante.IDAcademia != userID {
			c.JSON(http.StatusForbidden, domain.ErrorResponse{Error: "estudante não pertence a esta academia"})
			return
		}
	}

	// Buscar notas (read model otimizado)
	notas, err := store.GetNotasByEstudante(estudanteID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, domain.ErrorResponse{Error: "erro ao buscar notas"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"estudante_id": estudanteID,
		"notas":        notas,
		"total":        len(notas),
	})
}

// GetFaltasEstudante obtém o histórico de faltas de um estudante
func GetFaltasEstudante(c *gin.Context) {
	estudanteID, err := uuid.Parse(c.Param("estudanteId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, domain.ErrorResponse{Error: "ID de estudante inválido"})
		return
	}

	// Verificar permissões
	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	// Estudante pode ver suas próprias faltas
	// Academia pode ver faltas dos seus estudantes
	if userType == "estudante" && userID != estudanteID {
		c.JSON(http.StatusForbidden, domain.ErrorResponse{Error: "acesso negado"})
		return
	}

	if userType == "academia" {
		// Verificar se o estudante pertence a esta academia
		estudante, err := store.GetEstudanteByID(estudanteID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, domain.ErrorResponse{Error: "erro ao buscar estudante"})
			return
		}
		if estudante == nil {
			c.JSON(http.StatusNotFound, domain.ErrorResponse{Error: "estudante não encontrado"})
			return
		}
		if estudante.IDAcademia == nil || *estudante.IDAcademia != userID {
			c.JSON(http.StatusForbidden, domain.ErrorResponse{Error: "estudante não pertence a esta academia"})
			return
		}
	}

	// Buscar faltas (read model otimizado)
	faltas, err := store.GetFaltasByEstudante(estudanteID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, domain.ErrorResponse{Error: "erro ao buscar faltas"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"estudante_id": estudanteID,
		"faltas":       faltas,
		"total":        len(faltas),
	})
}

// GetHistoricoCompleto obtém histórico completo (notas + faltas)
func GetHistoricoCompleto(c *gin.Context) {
	estudanteID, err := uuid.Parse(c.Param("estudanteId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, domain.ErrorResponse{Error: "ID de estudante inválido"})
		return
	}

	// Verificar permissões
	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	if userType == "estudante" && userID != estudanteID {
		c.JSON(http.StatusForbidden, domain.ErrorResponse{Error: "acesso negado"})
		return
	}

	if userType == "academia" {
		estudante, err := store.GetEstudanteByID(estudanteID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, domain.ErrorResponse{Error: "erro ao buscar estudante"})
			return
		}
		if estudante == nil {
			c.JSON(http.StatusNotFound, domain.ErrorResponse{Error: "estudante não encontrado"})
			return
		}
		if estudante.IDAcademia == nil || *estudante.IDAcademia != userID {
			c.JSON(http.StatusForbidden, domain.ErrorResponse{Error: "estudante não pertence a esta academia"})
			return
		}
	}

	// Buscar dados do estudante
	estudante, err := store.GetEstudanteByID(estudanteID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, domain.ErrorResponse{Error: "erro ao buscar estudante"})
		return
	}

	// Buscar notas
	notas, err := store.GetNotasByEstudante(estudanteID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, domain.ErrorResponse{Error: "erro ao buscar notas"})
		return
	}

	// Buscar faltas
	faltas, err := store.GetFaltasByEstudante(estudanteID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, domain.ErrorResponse{Error: "erro ao buscar faltas"})
		return
	}

	// Buscar inscrições
	inscricoes, err := store.GetInscricoesByEstudante(estudanteID)
	if err != nil {
		inscricoes = []domain.Inscricao{}
	}

	c.JSON(http.StatusOK, gin.H{
		"estudante":  estudante,
		"notas":      notas,
		"faltas":     faltas,
		"inscricoes": inscricoes,
	})
}

// GetEventosEstudante obtém todos os eventos de um estudante (auditoria completa)
// Demonstra como o Event Sourcing permite reconstruir toda a história
func GetEventosEstudante(c *gin.Context) {
	estudanteID, err := uuid.Parse(c.Param("estudanteId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, domain.ErrorResponse{Error: "ID de estudante inválido"})
		return
	}

	// Verificar permissões
	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	if userType == "estudante" && userID != estudanteID {
		c.JSON(http.StatusForbidden, domain.ErrorResponse{Error: "acesso negado"})
		return
	}

	// Buscar todos os eventos do estudante
	eventos, err := store.GetEventsByAggregate(estudanteID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, domain.ErrorResponse{Error: "erro ao buscar eventos"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"estudante_id": estudanteID,
		"eventos":      eventos,
		"total":        len(eventos),
		"message":      "Histórico completo de todos os eventos (Event Sourcing)",
	})
}

// GetMinhasInscricoes obtém as inscrições do estudante logado
func GetMinhasInscricoes(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	inscricoes, err := store.GetInscricoesByEstudante(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, domain.ErrorResponse{Error: "erro ao buscar inscrições"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"inscricoes": inscricoes,
		"total":      len(inscricoes),
	})
}

// GetMeuHistorico obtém o histórico completo do estudante logado
func GetMeuHistorico(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	// Buscar dados do estudante
	estudante, err := store.GetEstudanteByID(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, domain.ErrorResponse{Error: "erro ao buscar dados"})
		return
	}

	// Buscar notas
	notas, err := store.GetNotasByEstudante(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, domain.ErrorResponse{Error: "erro ao buscar notas"})
		return
	}

	// Buscar faltas
	faltas, err := store.GetFaltasByEstudante(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, domain.ErrorResponse{Error: "erro ao buscar faltas"})
		return
	}

	// Buscar inscrições
	inscricoes, err := store.GetInscricoesByEstudante(userID)
	if err != nil {
		inscricoes = []domain.Inscricao{}
	}

	c.JSON(http.StatusOK, gin.H{
		"estudante":  estudante,
		"notas":      notas,
		"faltas":     faltas,
		"inscricoes": inscricoes,
	})
}