package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"spuri/internal/middleware"
	"spuri/internal/utils"
)

// GetMinhasNotas retorna as notas do estudante autenticado.
// Rota: GET /estudante/minhas-notas
// Protegido por AuthMiddleware + RequireEstudante.
//
// H4-06: substitui o acesso do estudante a /notas-estudante/:codigo.
// O código do estudante é obtido do JWT — sem parâmetro de URL.
func GetMinhasNotas(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByID(userID)
	if err != nil || estudante == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	notasProj := getNotasProjection(c)
	notas, err := notasProj.GetByEstudante(estudante.CodigoEstudante)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"codigo_estudante": estudante.CodigoEstudante,
		"nome":             estudante.Nome,
		"notas":            notas,
		"total":            len(notas),
	})
}

// GetMinhasFaltas retorna as faltas do estudante autenticado.
// Rota: GET /estudante/minhas-faltas
// Protegido por AuthMiddleware + RequireEstudante.
//
// H4-06: substitui o acesso do estudante a /faltas-estudante/:codigo.
// O código do estudante é obtido do JWT — sem parâmetro de URL.
func GetMinhasFaltas(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByID(userID)
	if err != nil || estudante == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	faltasProj := getFaltasProjection(c)
	faltas, err := faltasProj.GetByEstudante(estudante.CodigoEstudante)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"codigo_estudante": estudante.CodigoEstudante,
		"nome":             estudante.Nome,
		"faltas":           faltas,
		"total":            len(faltas),
	})
}