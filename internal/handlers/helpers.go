package handlers

import (
	"log"
	"spuri/internal/db"
	"spuri/internal/projections"

	"github.com/gin-gonic/gin"
)

// getRepository obtém o repositório do contexto
func getRepository(c *gin.Context) *db.AggregateRepository {
	repo, _ := c.Get("repository")
	return repo.(*db.AggregateRepository)
}

// getDbClient obtém o cliente Banco de dados do contexto
func getDbClient(c *gin.Context) *db.Client {
	client, exists := c.Get("dbClient")
	if !exists {
		// Fallback: criar novo (não ideal mas evita crash)
		log.Printf("⚠️ Cliente Banco de dados não encontrado no contexto, criando novo")
		config := db.DefaultConfig()
		newClient, _ := db.NewClient(config)
		return newClient
	}
	return client.(*db.Client)
}

// getDbClientFromContext é um alias para getDbClient
// (usado em admin_handlers.go)
func getDbClientFromContext(c *gin.Context) *db.Client {
	return getDbClient(c)
}

// getEstudanteProjection obtém a projeção de estudantes
func getEstudanteProjection(c *gin.Context) *projections.EstudanteProjection {
	client := getDbClient(c)
	return projections.NewEstudanteProjection(client)
}

// getAcademiaProjection obtém a projeção de academias
func getAcademiaProjection(c *gin.Context) *projections.AcademiaProjection {
	client := getDbClient(c)
	return projections.NewAcademiaProjection(client)
}

// getNotasProjection obtém a projeção de notas
func getNotasProjection(c *gin.Context) *projections.NotasProjection {
	client := getDbClient(c)
	return projections.NewNotasProjection(client)
}

// getFaltasProjection obtém a projeção de faltas
func getFaltasProjection(c *gin.Context) *projections.FaltasProjection {
	client := getDbClient(c)
	return projections.NewFaltasProjection(client)
}

// getInscricoesProjection obtém a projeção de inscrições
func getInscricoesProjection(c *gin.Context) *projections.InscricoesProjection {
	client := getDbClient(c)
	return projections.NewInscricoesProjection(client)
}
