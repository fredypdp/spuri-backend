package handlers

import (
	"net/http"
	"spuri/internal/genesisdb"
	"spuri/internal/middleware"
	"spuri/internal/projections"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// GetNotasEstudante busca notas de um estudante (Query - CQRS Read)
func GetNotasEstudante(c *gin.Context) {
	estudanteID, err := uuid.Parse(c.Param("estudanteId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID de estudante inválido"})
		return
	}

	// Verificar permissões
	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	if userType == "estudante" && userID != estudanteID {
		c.JSON(http.StatusForbidden, gin.H{"error": "acesso negado"})
		return
	}

	if userType == "academia" {
		// Verificar se estudante pertence a esta academia
		estudanteProj := getEstudanteProjection(c)
		estudante, err := estudanteProj.GetByID(estudanteID)
		if err != nil || estudante == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
			return
		}
		if estudante.IDAcademia == nil || *estudante.IDAcademia != userID {
			c.JSON(http.StatusForbidden, gin.H{"error": "estudante não pertence a esta academia"})
			return
		}
	}

	// Buscar notas da projeção (CQRS - Read Model)
	notasProj := getNotasProjection(c)
	notas, err := notasProj.GetByEstudante(estudanteID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar notas"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"estudante_id": estudanteID,
		"notas":        notas,
		"total":        len(notas),
	})
}

// GetFaltasEstudante busca faltas de um estudante (Query)
func GetFaltasEstudante(c *gin.Context) {
	estudanteID, err := uuid.Parse(c.Param("estudanteId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID de estudante inválido"})
		return
	}

	// Verificar permissões
	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	if userType == "estudante" && userID != estudanteID {
		c.JSON(http.StatusForbidden, gin.H{"error": "acesso negado"})
		return
	}

	if userType == "academia" {
		estudanteProj := getEstudanteProjection(c)
		estudante, err := estudanteProj.GetByID(estudanteID)
		if err != nil || estudante == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
			return
		}
		if estudante.IDAcademia == nil || *estudante.IDAcademia != userID {
			c.JSON(http.StatusForbidden, gin.H{"error": "estudante não pertence a esta academia"})
			return
		}
	}

	// Buscar faltas da projeção
	faltasProj := getFaltasProjection(c)
	faltas, err := faltasProj.GetByEstudante(estudanteID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar faltas"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"estudante_id": estudanteID,
		"faltas":       faltas,
		"total":        len(faltas),
	})
}

// GetHistoricoCompleto busca histórico completo do estudante (Query)
func GetHistoricoCompleto(c *gin.Context) {
	estudanteID, err := uuid.Parse(c.Param("estudanteId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID de estudante inválido"})
		return
	}

	// Verificar permissões
	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	if userType == "estudante" && userID != estudanteID {
		c.JSON(http.StatusForbidden, gin.H{"error": "acesso negado"})
		return
	}

	// Buscar dados das projeções
	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByID(estudanteID)
	if err != nil || estudante == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}

	// Verificar permissão para academia
	if userType == "academia" {
		if estudante.IDAcademia == nil || *estudante.IDAcademia != userID {
			c.JSON(http.StatusForbidden, gin.H{"error": "estudante não pertence a esta academia"})
			return
		}
	}

	// Buscar notas
	notasProj := getNotasProjection(c)
	notas, _ := notasProj.GetByEstudante(estudanteID)

	// Buscar faltas
	faltasProj := getFaltasProjection(c)
	faltas, _ := faltasProj.GetByEstudante(estudanteID)

	// Buscar inscrições
	inscProj := getInscricoesProjection(c)
	inscricoes, _ := inscProj.GetByEstudante(estudanteID)

	c.JSON(http.StatusOK, gin.H{
		"estudante":  estudante,
		"notas":      notas,
		"faltas":     faltas,
		"inscricoes": inscricoes,
	})
}

// GetEventosEstudante retorna todos os eventos do estudante (Event Sourcing)
func GetEventosEstudante(c *gin.Context) {
	estudanteID, err := uuid.Parse(c.Param("estudanteId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID de estudante inválido"})
		return
	}

	// Verificar permissões
	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	if userType == "estudante" && userID != estudanteID {
		c.JSON(http.StatusForbidden, gin.H{"error": "acesso negado"})
		return
	}

	// Buscar eventos do ledger
	repository := getRepository(c)
	eventos, err := repository.GetEventHistory(estudanteID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar eventos"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"estudante_id": estudanteID,
		"eventos":      eventos,
		"total":        len(eventos),
		"message":      "Histórico completo de eventos (Event Sourcing)",
	})
}

// VerificarIntegridade verifica integridade do ledger de um estudante
func VerificarIntegridade(c *gin.Context) {
	estudanteID, err := uuid.Parse(c.Param("estudanteId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID de estudante inválido"})
		return
	}

	// Verificar integridade da hash chain
	repository := getRepository(c)
	isValid, err := repository.VerifyIntegrity(estudanteID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao verificar integridade"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"estudante_id": estudanteID,
		"integro":      isValid,
		"message": func() string {
			if isValid {
				return "Cadeia de hashes íntegra. Eventos não foram alterados."
			}
			return "ATENÇÃO: Cadeia de hashes comprometida!"
		}(),
	})
}

// GetMinhasInscricoes retorna inscrições do estudante logado
func GetMinhasInscricoes(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	inscProj := getInscricoesProjection(c)
	inscricoes, err := inscProj.GetByEstudante(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar inscrições"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"inscricoes": inscricoes,
		"total":      len(inscricoes),
	})
}

// GetMeuHistorico retorna histórico completo do estudante logado
func GetMeuHistorico(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	// Buscar dados
	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByID(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar dados"})
		return
	}

	notasProj := getNotasProjection(c)
	notas, _ := notasProj.GetByEstudante(userID)

	faltasProj := getFaltasProjection(c)
	faltas, _ := faltasProj.GetByEstudante(userID)

	inscProj := getInscricoesProjection(c)
	inscricoes, _ := inscProj.GetByEstudante(userID)

	c.JSON(http.StatusOK, gin.H{
		"estudante":  estudante,
		"notas":      notas,
		"faltas":     faltas,
		"inscricoes": inscricoes,
	})
}

// ListarInscricoesPendentes lista inscrições pendentes de uma academia
func ListarInscricoesPendentes(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	inscProj := getInscricoesProjection(c)
	inscricoes, err := inscProj.GetByAcademia(userID, "espera")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar inscrições"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"inscricoes": inscricoes,
		"total":      len(inscricoes),
	})
}

// Helper functions para obter projeções

func getNotasProjection(c *gin.Context) *projections.NotasProjection {
	client := getGenesisClientFromContext(c)
	return projections.NewNotasProjection(client)
}

func getFaltasProjection(c *gin.Context) *projections.FaltasProjection {
	client := getGenesisClientFromContext(c)
	return projections.NewFaltasProjection(client)
}

func getInscricoesProjection(c *gin.Context) *projections.InscricoesProjection {
	client := getGenesisClientFromContext(c)
	return projections.NewInscricoesProjection(client)
}

func getGenesisClientFromContext(c *gin.Context) *genesisdb.Client {
	// Criar client (idealmente deveria ser injetado)
	config := genesisdb.DefaultConfig()
	client, _ := genesisdb.NewClient(config)
	return client
}