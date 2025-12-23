package handlers

import (
	"log"
	"spuri/internal/genesisdb"
	"spuri/internal/projections"

	"github.com/gin-gonic/gin"
)

// getRepository obtém o repositório do contexto
func getRepository(c *gin.Context) *genesisdb.AggregateRepository {
	repo, _ := c.Get("repository")
	return repo.(*genesisdb.AggregateRepository)
}

// getGenesisClient obtém o cliente GenesisDB do contexto
func getGenesisClient(c *gin.Context) *genesisdb.Client {
	client, exists := c.Get("genesisClient")
	if !exists {
		// Fallback: criar novo (não ideal mas evita crash)
		log.Printf("⚠️ Cliente GenesisDB não encontrado no contexto, criando novo")
		config := genesisdb.DefaultConfig()
		newClient, _ := genesisdb.NewClient(config)
		return newClient
	}
	return client.(*genesisdb.Client)
}

// getGenesisClientFromContext é um alias para getGenesisClient
// (usado em admin_handlers.go)
func getGenesisClientFromContext(c *gin.Context) *genesisdb.Client {
	return getGenesisClient(c)
}

// getEstudanteProjection obtém a projeção de estudantes
func getEstudanteProjection(c *gin.Context) *projections.EstudanteProjection {
	client := getGenesisClient(c)
	return projections.NewEstudanteProjection(client)
}

// getAcademiaProjection obtém a projeção de academias
func getAcademiaProjection(c *gin.Context) *projections.AcademiaProjection {
	client := getGenesisClient(c)
	return projections.NewAcademiaProjection(client)
}

// getNotasProjection obtém a projeção de notas
func getNotasProjection(c *gin.Context) *projections.NotasProjection {
	client := getGenesisClient(c)
	return projections.NewNotasProjection(client)
}

// getFaltasProjection obtém a projeção de faltas
func getFaltasProjection(c *gin.Context) *projections.FaltasProjection {
	client := getGenesisClient(c)
	return projections.NewFaltasProjection(client)
}

// getInscricoesProjection obtém a projeção de inscrições
func getInscricoesProjection(c *gin.Context) *projections.InscricoesProjection {
	client := getGenesisClient(c)
	return projections.NewInscricoesProjection(client)
}