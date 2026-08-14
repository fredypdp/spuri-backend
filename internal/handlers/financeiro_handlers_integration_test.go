package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"spuri/internal/db"
	"spuri/internal/finance"
)

func seedSolicitacaoMatriculaParaBusca(t *testing.T, client *db.Client) (codigo, telefone, email string) {
	t.Helper()
	codigo = "SOL" + strings.ReplaceAll(uuid.NewString(), "-", "")[:8]
	telefone, email = "244"+codigo[3:], strings.ToLower(codigo)+"@example.test"
	_, err := client.DB().Exec(`INSERT INTO projection_solicitacoes_matricula
		(id,codigo_solicitacao,codigo_academia,nome,genero,data_nascimento,email,telefone,telefone_encarregado,bilhete_identidade,bilhete_identidade_encarregado,ano_escolar_fundamental,status,documentos,codigo_estudante_gerado,valor_matricula,metodos_pagamento_matricula,created_at,updated_at)
		VALUES ($1,$2,'ACA-BUSCA','Estudante buscável','feminino','2012-01-02',$3,$4,'923000000','BI-' || $2,'BI-RESP-' || $2,'6_ano_fundamental','aprovada_pendente_pagamento_matricula','{}'::jsonb,'EST0001',1250,ARRAY['REF'],CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, uuid.New(), codigo, email, telefone)
	if err != nil {
		t.Fatal(err)
	}
	return codigo, telefone, email
}

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

func TestIntegrationBuscaPublicaMatriculaExigeDoisCamposENaoExibePagamento(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := integrationFinanceClient(t)
	codigo, telefone, email := seedSolicitacaoMatriculaParaBusca(t, client)

	buscar := func(query string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/solicitacoes-matricula/buscar?"+query, nil)
		ctx.Set("dbClient", client)
		BuscarSolicitacoesMatricula(ctx)
		return recorder
	}
	twoFields := buscar("telefone=" + telefone + "&email=" + email)
	if twoFields.Code != http.StatusOK || !strings.Contains(twoFields.Body.String(), codigo) {
		t.Fatalf("busca com dois campos = %d: %s", twoFields.Code, twoFields.Body.String())
	}
	if strings.Contains(twoFields.Body.String(), "valor_matricula") || strings.Contains(twoFields.Body.String(), "metodos_pagamento") {
		t.Fatalf("busca pública expôs dados de pagamento: %s", twoFields.Body.String())
	}
	for _, query := range []string{"telefone=" + telefone, "telefone=" + telefone + "&email=outro@example.test"} {
		recorder := buscar(query)
		if recorder.Code != http.StatusOK || strings.Contains(recorder.Body.String(), codigo) {
			t.Fatalf("busca indevida para %q: %d %s", query, recorder.Code, recorder.Body.String())
		}
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
