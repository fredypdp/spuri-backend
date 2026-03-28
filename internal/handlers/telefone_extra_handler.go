package handlers

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/projections"
	"spuri/internal/utils"
)

// getTelefonesExtraProjection é o helper de projeção para este handler.
func getTelefonesExtraProjection(c *gin.Context) *projections.TelefonesExtraProjection {
	return projections.NewTelefonesExtraProjection(getDbClient(c))
}

// ============================================================================
// POST /adicionar-telefone-extra
// ============================================================================

// AdicionarTelefoneExtra cadastra um número de telefone extra para o usuário autenticado.
//
// Regras aplicadas:
//  1. Número normalizado (remove espaços, hífens e parênteses) e validado no aggregate.
//  2. Se o número já está verificado por qualquer usuário → 409 Conflict.
//  3. Se o usuário já cadastrou este número → 409 Conflict.
//  4. O aggregate TelefoneExtra é criado e salvo no ledger.
//  5. O evento TelefoneExtraAdicionado é processado pela projeção.
//
// Disponível para: estudante, academia, admin (qualquer usuário autenticado).
// Rota: POST /adicionar-telefone-extra
func AdicionarTelefoneExtra(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		utils.RespondWithUnauthorizedError(c)
		return
	}
	userType, ok := middleware.GetUserType(c)
	if !ok {
		utils.RespondWithUnauthorizedError(c)
		return
	}

	var req struct {
		NumeroTelefone string `json:"numero_telefone" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("numero_telefone é obrigatório"))
		return
	}

	// Normalizar antes de consultar a projeção — a mesma normalização que o aggregate aplica.
	normalized := normalizarNumeroTelefone(req.NumeroTelefone)

	telefonesProj := getTelefonesExtraProjection(c)

	// Verificação 1: número já verificado por outro usuário → ninguém mais pode cadastrá-lo.
	jaVerificado, err := telefonesProj.NumeroJaVerificado(normalized)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	if jaVerificado {
		utils.RespondWithConflictError(c,
			"este número de telefone já está verificado por outro usuário e não pode ser cadastrado novamente",
		)
		return
	}

	// Verificação 2: usuário já cadastrou este número.
	jaCadastrou, err := telefonesProj.UsuarioJaCadastrou(userID, userType, normalized)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	if jaCadastrou {
		utils.RespondWithConflictError(c, "você já cadastrou este número de telefone")
		return
	}

	// Criar aggregate e executar comando.
	tel := aggregates.NewTelefoneExtra()
	if err := tel.Adicionar(userID, userType, normalized); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	// Persistir no ledger.
	repository := getRepository(c)
	audit := db.AuditContext{
		UserID:   userID.String(),
		UserType: userType,
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(tel, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":         "telefone extra adicionado com sucesso",
		"id":              tel.ID,
		"numero_telefone": tel.NumeroTelefone,
		"verificado":      false,
	})
}

// normalizarNumeroTelefone remove espaços, hífens e parênteses do número.
// Deve produzir o mesmo resultado que aggregates.normalizarTelefone (privada).
func normalizarNumeroTelefone(numero string) string {
	s := numero
	result := make([]rune, 0, len(s))
	for i, r := range s {
		if i == 0 && r == '+' {
			result = append(result, r)
			continue
		}
		if r >= '0' && r <= '9' {
			result = append(result, r)
		}
	}
	return string(result)
}
