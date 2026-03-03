// ============================================================================
// ARQUIVO: internal/handlers/auth_handlers.go
// ============================================================================

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
	Senha   string `json:"senha"   binding:"required"`
	Type    string `json:"type"    binding:"required"`
}

// Login autentica estudante ou academia.
//
// FIX E-16 (timing attack): quando o usuário não existe, executamos bcrypt
// com um hash dummy para que o tempo de resposta seja idêntico ao caso em que
// o usuário existe mas a senha está errada. Sem isso, um atacante conseguia
// distinguir "usuário inexistente" de "senha errada" pelo tempo de resposta,
// permitindo enumerar usuários válidos.
//
// FIX E-17: academia inativa recebe 401 antes de emitir o JWT, mesmo que a
// senha esteja correta. O middleware ValidarStatusAcademia protege rotas após
// o login, mas emitir um token para academia inativa é desnecessário e cria
// uma janela de risco para rotas sem esse middleware.
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

	// Hash dummy para timing-safe: mesmo custo que um hash real.
	// Usado quando usuário não existe para evitar timing attack.
	// O valor "$2a$10$..." é um hash bcrypt válido que nunca vai coincidir.
	const dummyHash = "$2a$10$dummyhashvaluethatdoesnotmatch000000000000000000000000000"

	var userID uuid.UUID
	var userName string
	var senhaHash string
	var codigo string
	var userStatus string
	var userFound bool

	if req.Type == "academia" {
		academia, err := academiaProj.GetByCodigoOrEmail(req.Usuario)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		if academia != nil {
			userID = academia.ID
			userName = academia.Nome
			senhaHash = academia.SenhaHash
			codigo = academia.CodigoAcademia
			userStatus = academia.Status
			userFound = true
		}
	} else {
		estudante, err := estudanteProj.GetByCodigo(req.Usuario)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		if estudante != nil {
			userID = estudante.ID
			userName = estudante.Nome
			senhaHash = estudante.SenhaHash
			codigo = estudante.CodigoEstudante
			userStatus = estudante.Status
			userFound = true
		}
	}

	// FIX E-16: sempre executar bcrypt para equalizar o tempo de resposta,
	// independente de o usuário existir ou não.
	hashToCompare := senhaHash
	if !userFound {
		hashToCompare = dummyHash
	}

	bcryptErr := bcrypt.CompareHashAndPassword([]byte(hashToCompare), []byte(req.Senha))

	// Só após o bcrypt verificamos se o usuário foi encontrado.
	if !userFound || bcryptErr != nil {
		utils.RespondWithUnauthorizedError(c)
		return
	}

	// FIX E-17: verificar status da academia ANTES de emitir o JWT.
	// Academia inativa não deve receber token, mesmo com senha correta.
	if req.Type == "academia" && userStatus != "ativo" {
		log.Printf("[INFO] Login bloqueado: academia inativa - %s", codigo)
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "academia inativa. Entre em contato com o administrador.",
		})
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