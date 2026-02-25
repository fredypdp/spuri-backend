package handlers

import (
	"log"
	"spuri/internal/db"
	"spuri/internal/projections"

	"github.com/gin-gonic/gin"
)

func getRepository(c *gin.Context) *db.AggregateRepository {
	repo, _ := c.Get("repository")
	return repo.(*db.AggregateRepository)
}

func getDbClient(c *gin.Context) *db.Client {
	client, exists := c.Get("dbClient")
	if !exists {
		log.Printf("⚠️ Cliente BD não encontrado no contexto, criando novo")
		config := db.DefaultConfig()
		newClient, _ := db.NewClient(config)
		return newClient
	}
	return client.(*db.Client)
}

func getDbClientFromContext(c *gin.Context) *db.Client {
	return getDbClient(c)
}

func getEstudanteProjection(c *gin.Context) *projections.EstudanteProjection {
	client := getDbClient(c)
	return projections.NewEstudanteProjection(client)
}

func getAcademiaProjection(c *gin.Context) *projections.AcademiaProjection {
	client := getDbClient(c)
	return projections.NewAcademiaProjection(client)
}

func getNotasProjection(c *gin.Context) *projections.NotasProjection {
	client := getDbClient(c)
	return projections.NewNotasProjection(client)
}

func getFaltasProjection(c *gin.Context) *projections.FaltasProjection {
	client := getDbClient(c)
	return projections.NewFaltasProjection(client)
}

func getInscricoesProjection(c *gin.Context) *projections.InscricoesProjection {
	client := getDbClient(c)
	return projections.NewInscricoesProjection(client)
}

func getCursosProjection(c *gin.Context) *projections.CursosProjection {
	client := getDbClient(c)
	return projections.NewCursosProjection(client)
}

func getMateriasProjection(c *gin.Context) *projections.MateriasProjection {
	client := getDbClient(c)
	return projections.NewMateriasProjection(client)
}

func getAprovacaoAnoProjection(c *gin.Context) *projections.AprovacaoAnoProjection {
	client := getDbClient(c)
	return projections.NewAprovacaoAnoProjection(client)
}

func getProjManager(c *gin.Context) *projections.Manager {
    raw, _ := c.Get("projManager")
    return raw.(*projections.Manager)
}

func getCategoriasNotaProjection(c *gin.Context) *projections.CategoriasNotaProjection {
	client := getDbClient(c)
	return projections.NewCategoriasNotaProjection(client)
}