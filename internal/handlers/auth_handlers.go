// ============================================================================
// ARQUIVO: internal/handlers/auth_handlers.go
//
// CORREÇÕES APLICADAS:
//   FIX-C4  — Login: estudante com status != "ativo" agora recebe 401.
//              Antes: apenas academia tinha verificação de status.
//              Agora: ambos são verificados antes de emitir o JWT.
//   FIX-E16 — Timing attack: bcrypt executado mesmo quando usuário não existe.
//   FIX-E17 — Academia inativa bloqueada antes de emitir token.
//   H4-03   — RegisterEstudantePorAcademia REMOVIDA deste arquivo.
//              A versão legada com senha "spuri123" hardcoded não existe mais.
//              A versão corrigida está em estudante_handlers.go (FIX-S1/S2).
//   H4-04   — Consequência direta de H4-03: evento de criação por academia agora
//              contém AuditContext correto (academia ID + IP).
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
// FIX-E16 (timing attack): quando o usuário não existe, executamos bcrypt
// com um hash dummy para que o tempo de resposta seja idêntico ao caso em que
// o usuário existe mas a senha está errada.
//
// FIX-E17: academia inativa recebe 401 antes de emitir o JWT.
//
// FIX-C4: estudante com status != "ativo" agora recebe 401.
// Estudantes auto-cadastrados nascem com status "inativo" e não podem fazer
// login até serem ativados. Estudantes criados por academia já nascem "ativo".
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

	// FIX-E16: sempre executar bcrypt para equalizar o tempo de resposta.
	hashToCompare := senhaHash
	if !userFound {
		hashToCompare = dummyHash
	}

	bcryptErr := bcrypt.CompareHashAndPassword([]byte(hashToCompare), []byte(req.Senha))

	if !userFound || bcryptErr != nil {
		utils.RespondWithUnauthorizedError(c)
		return
	}

	// FIX-E17: academia inativa bloqueada antes de emitir token.
	if req.Type == "academia" && userStatus != "ativo" {
		log.Printf("[INFO] Login bloqueado: academia inativa - %s", codigo)
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "academia inativa. Entre em contato com o administrador.",
		})
		return
	}

	// FIX-C4: estudante inativo bloqueado antes de emitir token.
	// Estudantes auto-cadastrados têm status "inativo" por padrão.
	// Apenas estudantes criados por academia (CriarComVinculo) nascem "ativo".
	if req.Type == "estudante" && userStatus != "ativo" {
		log.Printf("[INFO] Login bloqueado: estudante inativo - %s", codigo)
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "conta inativa. Entre em contato com sua academia.",
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