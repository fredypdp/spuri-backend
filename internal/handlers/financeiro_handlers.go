package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"spuri/internal/finance"
	"spuri/internal/middleware"
	"spuri/internal/utils"
)

// FinanceiroService is initialised with the application database in initDB.
var FinanceiroService = finance.NewService(nil)

func financeActor(c *gin.Context) (uuid.UUID, string, string, bool) {
	id, ok := middleware.GetUserID(c)
	if !ok {
		return uuid.Nil, "", "", false
	}
	typeName, ok := middleware.GetUserType(c)
	if !ok {
		return uuid.Nil, "", "", false
	}
	return id, typeName, c.GetString("codigo_academia"), true
}
func financeAdminAllowed(c *gin.Context) bool {
	_, t, _, ok := financeActor(c)
	if !ok || t != "admin" {
		return false
	}
	return verificarPermissaoAdmin(c, "fpp") == nil
}
func authorizeFinanceScope(c *gin.Context, context *string, academy *string) bool {
	_, t, own, ok := financeActor(c)
	if !ok {
		return false
	}
	if t == "academia" {
		if *context != "" && *context != finance.ContextoAcademia {
			return false
		}
		if *academy != "" && *academy != own {
			return false
		}
		*context = finance.ContextoAcademia
		*academy = own
		return true
	}
	return t == "admin" && financeAdminAllowed(c)
}
func financeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, finance.ErrNotFound):
		utils.RespondWithNotFoundError(c, "recurso financeiro")
	case errors.Is(err, finance.ErrConflict):
		utils.RespondWithConflictError(c, "operação financeira equivalente em processamento")
	case errors.Is(err, finance.ErrUpstream):
		utils.RespondWithServiceUnavailable(c, err)
	default:
		utils.RespondWithValidationError(c, err)
	}
}

func ConfigurarCredencialAppyPay(c *gin.Context) {
	var in finance.CredentialInput
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.RespondWithValidationError(c, errors.New("payload inválido"))
		return
	}
	if !authorizeFinanceScope(c, &in.ContextoTipo, &in.CodigoAcademia) {
		utils.RespondWithForbiddenError(c, "sem permissão para configurar estas credenciais")
		return
	}
	id, t, _, ok := financeActor(c)
	if !ok {
		utils.RespondWithUnauthorizedError(c)
		return
	}
	out, err := FinanceiroService.ConfigureCredential(c.Request.Context(), nil, in, id.String(), t, c.ClientIP())
	if err != nil {
		financeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, out)
}
func AtualizarCredencialAppyPay(c *gin.Context) {
	idParam, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, errors.New("id inválido"))
		return
	}
	var in finance.CredentialInput
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.RespondWithValidationError(c, errors.New("payload inválido"))
		return
	}
	if !authorizeFinanceScope(c, &in.ContextoTipo, &in.CodigoAcademia) {
		utils.RespondWithForbiddenError(c, "sem permissão para configurar estas credenciais")
		return
	}
	id, t, _, ok := financeActor(c)
	if !ok {
		utils.RespondWithUnauthorizedError(c)
		return
	}
	out, err := FinanceiroService.ConfigureCredential(c.Request.Context(), &idParam, in, id.String(), t, c.ClientIP())
	if err != nil {
		financeError(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}
func ListarCredenciaisAppyPay(c *gin.Context) {
	contexto := c.Query("contexto_tipo")
	academia := c.Query("codigo_academia")
	if !authorizeFinanceScope(c, &contexto, &academia) {
		utils.RespondWithForbiddenError(c, "sem permissão para consultar credenciais financeiras")
		return
	}
	out, err := FinanceiroService.ListCredentials(c.Request.Context(), contexto, academia)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}
func CriarCobrancaAppyPay(c *gin.Context) {
	var in finance.ChargeRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.RespondWithValidationError(c, errors.New("payload inválido"))
		return
	}
	if !authorizeFinanceScope(c, &in.ContextoTipo, &in.CodigoAcademia) {
		utils.RespondWithForbiddenError(c, "sem permissão para este contexto financeiro")
		return
	}
	id, t, _, ok := financeActor(c)
	if !ok {
		utils.RespondWithUnauthorizedError(c)
		return
	}
	out, err := FinanceiroService.CreateCharge(c.Request.Context(), in, id.String(), t, c.ClientIP())
	if err != nil {
		financeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, out)
}
func GerarQRCodeAppyPay(c *gin.Context) {
	var in finance.QRCodeRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.RespondWithValidationError(c, errors.New("payload inválido"))
		return
	}
	if !authorizeFinanceScope(c, &in.ContextoTipo, &in.CodigoAcademia) {
		utils.RespondWithForbiddenError(c, "sem permissão para este contexto financeiro")
		return
	}
	id, t, _, ok := financeActor(c)
	if !ok {
		utils.RespondWithUnauthorizedError(c)
		return
	}
	out, err := FinanceiroService.CreateGPOQRCode(c.Request.Context(), in, id.String(), t, c.ClientIP())
	if err != nil {
		financeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, out)
}
func ConsultarCobrancaAppyPay(c *gin.Context) {
	contexto := c.Query("contexto_tipo")
	academia := c.Query("codigo_academia")
	if !authorizeFinanceScope(c, &contexto, &academia) {
		utils.RespondWithForbiddenError(c, "sem permissão para este contexto financeiro")
		return
	}
	id, t, _, ok := financeActor(c)
	if !ok {
		utils.RespondWithUnauthorizedError(c)
		return
	}
	out, err := FinanceiroService.ConsultCharge(c.Request.Context(), contexto, academia, c.Param("id"), id.String(), t, c.ClientIP())
	if err != nil {
		financeError(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}
func ReceberWebhookAppyPay(metodo string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, password, _ := c.Request.BasicAuth()
		owner, err := FinanceiroService.AuthenticateWebhook(c.Request.Context(), user, password, c.GetHeader("X-API-Key"))
		if err != nil {
			c.Status(http.StatusUnauthorized)
			return
		}
		var payload map[string]any
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		eventID := webhookID(payload)
		if eventID == "" {
			c.Status(http.StatusBadRequest)
			return
		}
		if _, err := FinanceiroService.AcceptWebhook(c.Request.Context(), metodo, eventID, owner, payload); err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusOK)
	}
}
func webhookID(payload map[string]any) string {
	for _, k := range []string{"id", "merchantTransactionId", "merchant_transaction_id"} {
		if v, ok := payload[k].(string); ok && strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
