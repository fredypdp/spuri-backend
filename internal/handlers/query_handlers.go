package handlers

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"spuri/internal/middleware"
	"spuri/internal/utils"
)

func getPaginationParams(c *gin.Context) (limit, offset int) {
	limitStr := c.Query("limit")
	offsetStr := c.Query("offset")

	limit = 50
	offset = 0

	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 200 {
			limit = l
		}
	}

	if offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	return limit, offset
}

// VerificarIntegridade verifica a cadeia de hashes do ledger de um estudante.
//
// H4-08 / H4-22: adicionada verificação de ownership por tipo de usuário:
//   - estudante: apenas o próprio estudante pode verificar sua integridade
//   - academia:  apenas a academia à qual o estudante pertence
//   - admin:     acesso irrestrito (qualquer role de admin)
func VerificarIntegridade(c *gin.Context) {
	codigoEstudante := c.Param("codigo")

	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByCodigo(codigoEstudante)
	if err != nil || estudante == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	switch userType {
	case "estudante":
		// Estudante só pode verificar sua própria integridade.
		if userID != estudante.ID {
			utils.RespondWithForbiddenError(c, "Você só pode verificar sua própria integridade")
			return
		}

	case "academia":
		// Academia só pode verificar estudantes que pertencem a ela.
		// H4-22: sem essa verificação, academia podia confirmar existência
		// e obter nome de estudantes de outras academias.
		academiaProj := getAcademiaProjection(c)
		academiaDTO, err := academiaProj.GetByID(userID)
		if err != nil || academiaDTO == nil {
			utils.RespondWithNotFoundError(c, "academia")
			return
		}
		if estudante.CodigoAcademia == nil || *estudante.CodigoAcademia != academiaDTO.CodigoAcademia {
			utils.RespondWithForbiddenError(c, "Estudante não pertence a esta academia")
			return
		}

	case "admin":
		// Admin tem acesso irrestrito — qualquer role de admin é aceito.
		// A autenticação do admin é garantida pelo JWT (user_type="admin").

	default:
		utils.RespondWithForbiddenError(c, "Tipo de usuário não autorizado")
		return
	}

	repository := getRepository(c)
	isValid, err := repository.VerifyIntegrity(estudante.ID)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"codigo_estudante": codigoEstudante,
		"nome":             estudante.Nome,
		"integro":          isValid,
		"message": func() string {
			if isValid {
				return "✅ Cadeia de hashes íntegra. Eventos não foram alterados."
			}
			return "⚠️ ATENÇÃO: Cadeia de hashes comprometida!"
		}(),
	})
}

func GetEstudantePorCodigoQuery(c *gin.Context) {
	// alias para compatibilidade — chama GetEstudantePorCodigo de profile_handlers
	GetEstudantePorCodigo(c)
}

// ListarEstudantes — delegado para admin_handlers
func ListarEstudantesQuery(c *gin.Context) {
	ListarEstudantes(c)
}

// getPaginationParamsWithDefaults é alias público para outros handlers
func getPaginationParamsWithDefaults(c *gin.Context, defaultLimit int) (limit, offset int) {
	limit, offset = getPaginationParams(c)
	if limit == 50 && defaultLimit > 0 {
		limit = defaultLimit
	}
	return
}

// RespondForbiddenIfNotOwner verifica se o userID autenticado corresponde ao estudante.
// Retorna true se o acesso foi bloqueado (handler deve retornar).
func respondForbiddenIfNotOwner(c *gin.Context, estudanteID uuid.UUID, msg string) bool {
	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)
	if userType == "estudante" && userID != estudanteID {
		utils.RespondWithForbiddenError(c, msg)
		return true
	}
	return false
}

// fmtOptional retorna o valor ou nil se vazio — helper para respostas JSON.
func fmtOptional(s *string) interface{} {
	if s == nil {
		return nil
	}
	return *s
}

// fmtRequiredStr converte string para interface{} para uso em maps.
func fmtRequiredStr(s string) interface{} {
	return s
}

// suppress unused import warning
var _ = fmt.Sprintf
