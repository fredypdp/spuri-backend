package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestIntegrationFPPAdminNaoPodeAnularOuReativarMensalidade(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for nome, handler := range map[string]func(*gin.Context){
		"anular":   AnularObrigacoesMensalidade,
		"reativar": ReativarObrigacoesMensalidade,
	} {
		t.Run(nome, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/financeiro/mensalidades/"+nome, bytes.NewBufferString(`{"codigo_estudante":"EST","ano_letivo":"2025_2026","meses":[9]}`))
			ctx.Request.Header.Set("Content-Type", "application/json")
			ctx.Set("user_id", uuid.New())
			ctx.Set("user_type", "admin")

			handler(ctx)
			if recorder.Code != http.StatusForbidden {
				t.Fatalf("admin fpp recebeu status %d, queria 403: %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}
