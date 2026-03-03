package handlers

import (
	"fmt"
	"log"
	"net/http"
	"spuri/internal/middleware"
	"spuri/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type LoginRequest struct {
	Usuario string `json:"usuario" binding:"required"`
	Senha   string `json:"senha" binding:"required"`
	Type    string `json:"type" binding:"required"`
}

func Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if req.Type != "estudante" && req.Type != "academia" {
		utils.RespondWithValidationError(c, fmt.Errorf("tipo deve ser 'estudante' ou 'academia'"))
		return
	}

	estudanteProj := getEstudanteProjection(c)
	academiaProj := getAcademiaProjection(c)

	var userID uuid.UUID
	var userName string
	var senhaHash string
	var codigo string

	if req.Type == "academia" {
		academia, err := academiaProj.GetByCodigoOrEmail(req.Usuario)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		if academia == nil {
			utils.RespondWithUnauthorizedError(c)
			return
		}
		userID = academia.ID
		userName = academia.Nome
		senhaHash = academia.SenhaHash
		codigo = academia.CodigoAcademia
	} else {
		estudante, err := estudanteProj.GetByCodigo(req.Usuario)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		if estudante == nil {
			utils.RespondWithUnauthorizedError(c)
			return
		}
		userID = estudante.ID
		userName = estudante.Nome
		senhaHash = estudante.SenhaHash
		codigo = estudante.CodigoEstudante
	}

	if err := bcrypt.CompareHashAndPassword([]byte(senhaHash), []byte(req.Senha)); err != nil {
		utils.RespondWithUnauthorizedError(c)
		return
	}

	token, err := middleware.GenerateToken(userID, req.Type)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Login bem-sucedido: %s (%s)", userName, req.Type)

	c.JSON(http.StatusOK, gin.H{
		"token":  token,
		"codigo": codigo,
		"nome":   userName,
		"type":   req.Type,
	})
}