package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"spuri/internal/finance"
)

var FinanceiroService = finance.NewService(nil)

func user(c *gin.Context) (string, string, string) {
	id, _ := c.Get("user_id")
	t, _ := c.Get("user_type")
	cod := c.GetString("codigo_academia")
	return toS(id), toS(t), cod
}
func toS(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case uuid.UUID:
		return x.String()
	case interface{ String() string }:
		return x.String()
	default:
		return ""
	}
}

func CriarCredencialAppyPay(c *gin.Context) {
	var in finance.CredencialInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "payload inválido"})
		return
	}
	uid, ut, _ := user(c)
	out, err := FinanceiroService.CriarCredencial(c.Request.Context(), in, uid, ut)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, out)
}
func AtualizarCredencialAppyPay(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(400, gin.H{"error": "id inválido"})
		return
	}
	var in finance.CredencialInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(400, gin.H{"error": "payload inválido"})
		return
	}
	uid, ut, cod := user(c)
	out, err := FinanceiroService.AtualizarCredencial(c.Request.Context(), id, in, uid, ut, cod)
	if err != nil {
		c.JSON(403, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, out)
}
func ListarCredenciaisAppyPay(c *gin.Context) {
	_, ut, cod := user(c)
	c.JSON(200, FinanceiroService.ListarCredenciais(ut, cod))
}
func ObterCredencialAppyPay(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(400, gin.H{"error": "id inválido"})
		return
	}
	_, ut, cod := user(c)
	out, err := FinanceiroService.ObterCredencial(id, ut, cod)
	if err != nil {
		c.JSON(404, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, out)
}
func TestarCredencialAppyPay(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(400, gin.H{"error": "id inválido"})
		return
	}
	uid, ut, cod := user(c)
	if err := FinanceiroService.TestarCredencial(c.Request.Context(), id, uid, ut, cod); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "validada"})
}
func AtivarCredencialAppyPay(c *gin.Context)    { alterarStatusCred(c, finance.StatusAtivo) }
func DesativarCredencialAppyPay(c *gin.Context) { alterarStatusCred(c, finance.StatusInativo) }
func alterarStatusCred(c *gin.Context, st finance.StatusCredencial) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(400, gin.H{"error": "id inválido"})
		return
	}
	var body struct {
		Motivo string `json:"motivo"`
	}
	_ = c.ShouldBindJSON(&body)
	uid, ut, cod := user(c)
	out, err := FinanceiroService.AlterarStatusCredencial(id, st, uid, ut, cod, body.Motivo)
	if err != nil {
		c.JSON(403, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, out)
}
func AlterarModalidadePagamento(c *gin.Context) {
	var body struct {
		Escopo         string `json:"escopo"`
		CodigoAcademia string `json:"codigo_academia"`
		Ativa          bool   `json:"ativa"`
		Motivo         string `json:"motivo"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": "payload inválido"})
		return
	}
	uid, ut, _ := user(c)
	if err := FinanceiroService.AlterarModalidade(body.Escopo, body.CodigoAcademia, body.Ativa, uid, ut, body.Motivo); err != nil {
		c.JSON(403, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "alterada"})
}
