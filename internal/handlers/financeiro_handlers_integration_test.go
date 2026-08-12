package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"spuri/internal/db"
	"spuri/internal/finance"
)

func integrationFinanceClient(t *testing.T) *db.Client {
	t.Helper()
	if os.Getenv("RUN_POSTGRES_INTEGRATION") != "1" {
		t.Skip("teste de integração requer RUN_POSTGRES_INTEGRATION=1 e PostgreSQL")
	}
	previousDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chdir("../.."); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previousDir) })
	client, err := db.NewClient(db.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err = client.RunMigrations(); err != nil {
		t.Fatal(err)
	}
	return client
}

func TestIntegrationFinanceRejectsAcademyChargeOutsideScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := integrationFinanceClient(t)
	merchant := "INTSCOPE" + uuid.NewString()[:6]
	chargeID := uuid.New()
	payload, _ := json.Marshal(map[string]any{"status": "criada"})
	if _, err := client.DB().Exec(`INSERT INTO financeiro_cobrancas (id,merchant_transaction_id,contexto_tipo,codigo_academia,payload) VALUES ($1,$2,'academia','ACA1',$3)`, chargeID, merchant, payload); err != nil {
		t.Fatal(err)
	}

	previousService := FinanceiroService
	FinanceiroService = finance.NewService(client)
	t.Cleanup(func() { FinanceiroService = previousService })
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/financeiro/appypay/cobrancas/"+merchant, nil)
	ctx.Params = gin.Params{{Key: "id", Value: merchant}}
	ctx.Set("dbClient", client)
	ctx.Set("user_id", uuid.New())
	ctx.Set("user_type", "academia")
	ctx.Set("codigo_academia", "ACA2")

	ConsultarCobrancaAppyPay(ctx)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("academia externa recebeu status %d, quer 404: %s", recorder.Code, recorder.Body.String())
	}
}

func TestIntegrationFinanceRejectsNonFPPAdmins(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := integrationFinanceClient(t)
	for _, role := range []string{"gerente", "adm"} {
		t.Run(role, func(t *testing.T) {
			adminID := uuid.New()
			if _, err := client.DB().Exec(`INSERT INTO projection_admins (id,nome,email,senha_hash,role,status) VALUES ($1,$2,$3,'hash',$4,'ativo')`, adminID, role, role+"-"+uuid.NewString()+"@example.test", role); err != nil {
				t.Fatal(err)
			}
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/financeiro/appypay/cobrancas", bytes.NewBufferString(`{}`))
			ctx.Request.Header.Set("Content-Type", "application/json")
			ctx.Set("dbClient", client)
			ctx.Set("user_id", adminID)
			ctx.Set("user_type", "admin")

			CriarCobrancaAppyPay(ctx)
			if recorder.Code != http.StatusForbidden {
				t.Fatalf("admin %s recebeu status %d, quer 403: %s", role, recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestIntegrationFinanceFPPAdminCannotCancelAcademyCharge(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := integrationFinanceClient(t)
	merchant := "INTCANCEL" + uuid.NewString()[:6]
	chargeID := uuid.New()
	payload, _ := json.Marshal(map[string]any{"status": "criada"})
	if _, err := client.DB().Exec(`INSERT INTO financeiro_cobrancas (id,merchant_transaction_id,contexto_tipo,codigo_academia,payload) VALUES ($1,$2,'academia','ACA-CANCEL',$3)`, chargeID, merchant, payload); err != nil {
		t.Fatal(err)
	}
	adminID := uuid.New()
	if _, err := client.DB().Exec(`INSERT INTO projection_admins (id,nome,email,senha_hash,role,status) VALUES ($1,'fpp-cancel',$2,'hash','fpp','ativo')`, adminID, "fpp-cancel-"+uuid.NewString()+"@example.test"); err != nil {
		t.Fatal(err)
	}
	previousService := FinanceiroService
	FinanceiroService = finance.NewService(client)
	t.Cleanup(func() { FinanceiroService = previousService })
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/financeiro/appypay/cobrancas/"+merchant+"/cancelar", bytes.NewBufferString(`{}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{{Key: "id", Value: merchant}}
	ctx.Set("dbClient", client)
	ctx.Set("user_id", adminID)
	ctx.Set("user_type", "admin")

	CancelarCobrancaAppyPay(ctx)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("admin FPP recebeu status %d, quer 404: %s", recorder.Code, recorder.Body.String())
	}
}
