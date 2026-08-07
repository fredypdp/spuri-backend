package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"spuri/internal/db"
	"spuri/internal/financeiro"
	"spuri/internal/middleware"
	"spuri/internal/utils"
)

func financeiroService(c *gin.Context) *financeiro.Service {
	client := getDbClient(c)
	if client == nil {
		return nil
	}
	return financeiro.NewService(client)
}
func auditFinanceiro(c *gin.Context) db.AuditContext {
	id, _ := middleware.GetUserID(c)
	typ, _ := middleware.GetUserType(c)
	return db.AuditContext{UserID: id.String(), UserType: typ, IP: c.ClientIP()}
}
func academiaFinanceiroScope(c *gin.Context) (financeiro.Scope, bool) {
	ac, ok := currentAcademiaDTO(c)
	if !ok {
		return financeiro.Scope{}, false
	}
	return financeiro.AcademiaScope(ac.CodigoAcademia), true
}

func ConfigurarCredenciaisAppyPayAcademia(c *gin.Context) {
	scope, ok := academiaFinanceiroScope(c)
	if !ok {
		return
	}
	configurarCredenciaisAppyPay(c, scope)
}
func ConsultarCredenciaisAppyPayAcademia(c *gin.Context) {
	scope, ok := academiaFinanceiroScope(c)
	if !ok {
		return
	}
	consultarCredenciaisAppyPay(c, scope)
}
func ConfigurarCredenciaisAppyPaySpuri(c *gin.Context) {
	configurarCredenciaisAppyPay(c, financeiro.SpuriScope())
}
func ConsultarCredenciaisAppyPaySpuri(c *gin.Context) {
	consultarCredenciaisAppyPay(c, financeiro.SpuriScope())
}
func configurarCredenciaisAppyPay(c *gin.Context, scope financeiro.Scope) {
	var req financeiro.CredentialInput
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	out, err := financeiroService(c).ConfigureCredentials(c.Request.Context(), scope, req, auditFinanceiro(c))
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}
func consultarCredenciaisAppyPay(c *gin.Context, scope financeiro.Scope) {
	out, err := financeiroService(c).GetCredential(c.Request.Context(), scope)
	if err != nil {
		if strings.Contains(err.Error(), "não configuradas") {
			utils.RespondWithNotFoundError(c, "credenciais AppyPay")
		} else {
			utils.RespondWithInternalError(c, err)
		}
		return
	}
	c.JSON(http.StatusOK, out)
}

func CriarCobrancaAppyPayAcademia(c *gin.Context) {
	scope, ok := academiaFinanceiroScope(c)
	if !ok {
		return
	}
	criarCobrancaAppyPay(c, scope)
}
func CriarCobrancaAppyPaySpuri(c *gin.Context) { criarCobrancaAppyPay(c, financeiro.SpuriScope()) }
func criarCobrancaAppyPay(c *gin.Context, scope financeiro.Scope) {
	var req financeiro.ChargeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	out, err := financeiroService(c).CreateCharge(c.Request.Context(), scope, req, auditFinanceiro(c))
	if err != nil {
		utils.RespondWithError(c, http.StatusBadGateway, "APPYPAY_ERROR", err)
		return
	}
	c.JSON(http.StatusCreated, out)
}
func CriarQRCodeAppyPayAcademia(c *gin.Context) {
	scope, ok := academiaFinanceiroScope(c)
	if !ok {
		return
	}
	criarQRCodeAppyPay(c, scope)
}
func CriarQRCodeAppyPaySpuri(c *gin.Context) { criarQRCodeAppyPay(c, financeiro.SpuriScope()) }
func criarQRCodeAppyPay(c *gin.Context, scope financeiro.Scope) {
	var req financeiro.QRCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	out, err := financeiroService(c).CreateGPOQRCode(c.Request.Context(), scope, req, auditFinanceiro(c))
	if err != nil {
		utils.RespondWithError(c, http.StatusBadGateway, "APPYPAY_ERROR", err)
		return
	}
	c.JSON(http.StatusCreated, out)
}
func ConsultarCobrancaAppyPayAcademia(c *gin.Context) {
	scope, ok := academiaFinanceiroScope(c)
	if !ok {
		return
	}
	consultarCobrancaAppyPay(c, scope)
}
func ConsultarCobrancaAppyPaySpuri(c *gin.Context) {
	consultarCobrancaAppyPay(c, financeiro.SpuriScope())
}
func consultarCobrancaAppyPay(c *gin.Context, scope financeiro.Scope) {
	out, err := financeiroService(c).GetCharge(c.Request.Context(), scope, c.Param("id"), auditFinanceiro(c))
	if err != nil {
		utils.RespondWithError(c, http.StatusBadGateway, "APPYPAY_ERROR", err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func WebhookAppyPayGPO(c *gin.Context) { receberWebhookAppyPay(c, "GPO") }
func WebhookAppyPayREF(c *gin.Context) { receberWebhookAppyPay(c, "REF") }
func receberWebhookAppyPay(c *gin.Context, method string) {
	var body map[string]any
	if err := c.ShouldBindJSON(&body); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	duplicate, err := financeiroService(c).ReceiveWebhook(c.Request.Context(), method, c.Request.Header, body)
	if err != nil {
		utils.RespondWithError(c, http.StatusUnauthorized, "UNAUTHORIZED", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"accepted": true, "duplicate": duplicate})
}
