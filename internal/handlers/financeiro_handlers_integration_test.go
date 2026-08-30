package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/finance"
	"spuri/internal/projections"
)

type handlerAppyPayMockTransport struct{}

func (handlerAppyPayMockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body := `{"id":"provider-charge-handler","status":"Pending"}`
	switch {
	case strings.Contains(req.URL.Path, "/oauth2/token"):
		body = `{"access_token":"test-token","expires_in":3600}`
	case req.Method == http.MethodGet:
		// Formato real de GET /charges/{id} (seção "Get a charge" da
		// documentação AppyPay): o status vem dentro de "payment.status",
		// nunca num campo solto "status" na raiz — diferente do corpo
		// simplificado usado acima para a criação da cobrança (que este
		// teste não usa para decidir nada). AcceptWebhook confirma um
		// webhook de sucesso com exatamente este GET antes de aplicar um
		// efeito irreversível (ver liveChargeStatus em
		// internal/finance/appypay.go); devolver aqui o mesmo resultado que
		// o webhook do teste relata (Success) reflete o cenário sendo
		// testado — a AppyPay já confirmou o pagamento.
		body = `{"payment":{"id":"provider-charge-handler","status":"Success","transactionEvents":[{"responseStatus":{"successful":true,"status":"Success","code":100,"source":"REF"}}]}}`
	}
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
}

func seedAcademiaParaMatriculaWebhook(t *testing.T, client *db.Client, codigo string) {
	t.Helper()
	_, err := client.DB().Exec(`INSERT INTO projection_academias
		(id,nivel,nome,nif,codigo_academia,senha_hash,provincia,endereco,nivel_escolar,status,cursos,anos_academicos,type,ano_letivo,created_at)
		VALUES ($1,'escola','Academia webhook',$2,$3,'hash','LUA','endereco','fundamental','ativo','[]'::jsonb,'["1_ano_fundamental"]'::jsonb,'private','2026_2027',CURRENT_TIMESTAMP)`,
		uuid.New(), strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, uuid.NewString())[:10], codigo)
	if err != nil {
		t.Fatal(err)
	}
}

func geraDigitos(n int) string {
	digitos := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, uuid.NewString())
	for len(digitos) < n {
		digitos += "0"
	}
	return digitos[:n]
}

func seedSolicitacaoMatriculaPendenteComLedger(t *testing.T, client *db.Client, academia string, valor float64) (codigo, codigoEstudante string) {
	t.Helper()
	codigo = "SOL" + strings.ReplaceAll(uuid.NewString(), "-", "")[:8]
	codigoEstudante = "EST" + codigo[3:7]
	email := strings.ToLower(codigo) + "@example.test"
	telefone := geraDigitos(9)
	telefoneResp := geraDigitos(9)
	bi := "BI-" + codigo
	biResp := "BI-RESP-" + codigo
	ano := "1_ano_fundamental"
	docs := map[string]aggregates.DocumentoMatricula{
		"bi_estudante":   {Tipo: "bi_estudante", Path: "docs/bi-estudante.pdf"},
		"bi_encarregado": {Tipo: "bi_encarregado", Path: "docs/bi-encarregado.pdf"},
	}
	sol := aggregates.NewSolicitacaoMatricula()
	if err := sol.Criar(codigo, academia, "Estudante Webhook", "feminino", time.Date(2017, 1, 2, 0, 0, 0, 0, time.UTC), &email, &telefone, &telefoneResp, &bi, &biResp, &ano, nil, nil, nil, nil, docs, []string{}); err != nil {
		t.Fatal(err)
	}
	if err := sol.Aprovar(uuid.New(), codigoEstudante); err != nil {
		t.Fatal(err)
	}
	if err := sol.MarcarPendentePagamentoMatricula(valor, []string{"REF"}); err != nil {
		t.Fatal(err)
	}
	if err := db.NewAggregateRepository(client).SaveWithAudit(sol, db.AuditContext{UserID: "integration-test", UserType: "sistema", IP: "127.0.0.1"}); err != nil {
		t.Fatal(err)
	}
	if err := projections.NewSolicitacaoMatriculaProjection(client).Rebuild(); err != nil {
		t.Fatal(err)
	}
	return codigo, codigoEstudante
}

func seedSolicitacaoMatriculaParaBusca(t *testing.T, client *db.Client) (codigo, telefone, email string) {
	t.Helper()
	codigo = "SOL" + strings.ReplaceAll(uuid.NewString(), "-", "")[:8]
	telefone, email = "244"+codigo[3:], strings.ToLower(codigo)+"@example.test"
	_, err := client.DB().Exec(`INSERT INTO projection_solicitacoes_matricula
		(id,codigo_solicitacao,codigo_academia,nome,genero,data_nascimento,email,telefone,telefone_encarregado,bilhete_identidade,bilhete_identidade_encarregado,ano_escolar_fundamental,status,documentos,codigo_estudante_gerado,valor_matricula,metodos_pagamento_matricula,created_at,updated_at)
		VALUES ($1,$2,'ACA-BUSCA','Estudante buscável','feminino','2012-01-02',$3,$4,'923000000',$5,$6,'6_ano_fundamental','aprovada_pendente_pagamento_matricula','{}'::jsonb,'EST0001',1250,ARRAY['REF'],CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, uuid.New(), codigo, email, telefone, "BI-"+codigo, "BI-RESP-"+codigo)
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

func TestIntegrationReceberWebhookAppyPayEfetivaVinculoMatricula(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := integrationFinanceClient(t)
	t.Setenv("ENV", "test")
	t.Setenv("APPYPAY_RESOURCE", "integration-resource")
	t.Setenv("FINANCE_ENCRYPTION_KEY", "test-only-secret-material-at-least-32")

	academia := "WH" + strings.ReplaceAll(uuid.NewString(), "-", "")[:8]
	seedAcademiaParaMatriculaWebhook(t, client, academia)
	codigo, codigoEstudante := seedSolicitacaoMatriculaPendenteComLedger(t, client, academia, 750)

	service := finance.NewService(client)
	service.SetHTTPClient(&http.Client{Transport: handlerAppyPayMockTransport{}})
	_, webhookSecret, err := service.ConfigureCredential(context.Background(), nil, finance.CredentialInput{
		ContextoTipo:     finance.ContextoAcademia,
		CodigoAcademia:   academia,
		ClientID:         "integration-client",
		ClientSecret:     "integration-secret",
		GPOPaymentMethod: "GPO_INTEGRATION",
		REFPaymentMethod: "REF_INTEGRATION",
	}, "integration-test", "sistema", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	charge, err := service.IniciarPagamentoMatricula(context.Background(), finance.MatriculaPagamentoInput{CodigoSolicitacao: codigo, MetodoPagamento: "REF"}, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}

	previousService := FinanceiroService
	FinanceiroService = service
	t.Cleanup(func() { FinanceiroService = previousService })
	repository := db.NewAggregateRepository(client)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("dbClient", client)
		c.Set("repository", repository)
	})
	router.POST("/financeiro/appypay/webhooks/ref", ReceberWebhookAppyPay("REF"))

	eventID := charge.Charge.ProviderChargeID
	// Formato real de um webhook da AppyPay (ver seção "Merchant Webhooks" de
	// docs/Parceiros e integrações/AppyPay Documentação.md): o status vem
	// dentro de "responseStatus", nunca em um campo solto "status"/"state"
	// na raiz do payload.
	payload, _ := json.Marshal(map[string]any{
		"id":                    eventID,
		"merchantTransactionId": charge.Charge.MerchantTransactionID,
		"amount":                750,
		"responseStatus": map[string]any{
			"successful": true,
			"status":     "Success",
			"code":       100,
			"message":    "Transaction Approved",
			"source":     "REF",
		},
	})
	postWebhook := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/financeiro/appypay/webhooks/ref", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(finance.WebhookHeaderName, webhookSecret)
		router.ServeHTTP(rec, req)
		return rec
	}

	first := postWebhook()
	if first.Code != http.StatusOK {
		t.Fatalf("webhook retornou %d: %s", first.Code, first.Body.String())
	}
	if err := projections.NewSolicitacaoMatriculaProjection(client).Rebuild(); err != nil {
		t.Fatal(err)
	}
	if err := projections.NewEstudanteProjection(client).Rebuild(); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := client.DB().QueryRow(`SELECT status FROM projection_solicitacoes_matricula WHERE codigo_solicitacao=$1`, codigo).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "aprovada" {
		t.Fatalf("status da solicitação = %q, queria aprovada", status)
	}
	var estudantes int
	if err := client.DB().QueryRow(`SELECT COUNT(*) FROM projection_estudantes WHERE codigo_estudante=$1 AND codigo_academia=$2 AND ano_escolar_fundamental='1_ano_fundamental'`, codigoEstudante, academia).Scan(&estudantes); err != nil {
		t.Fatal(err)
	}
	if estudantes != 1 {
		t.Fatalf("estudantes criados = %d, queria 1", estudantes)
	}
	second := postWebhook()
	if second.Code != http.StatusOK {
		t.Fatalf("webhook idempotente retornou %d: %s", second.Code, second.Body.String())
	}
	if err := projections.NewEstudanteProjection(client).Rebuild(); err != nil {
		t.Fatal(err)
	}
	if err := client.DB().QueryRow(`SELECT COUNT(*) FROM projection_estudantes WHERE codigo_estudante=$1`, codigoEstudante).Scan(&estudantes); err != nil {
		t.Fatal(err)
	}
	if estudantes != 1 {
		t.Fatalf("webhook duplicou estudante: %d", estudantes)
	}
	var vinculacoes int
	if err := client.DB().QueryRow(`SELECT COUNT(*) FROM spuri_ledger WHERE aggregate_type='SolicitacaoMatricula' AND aggregate_id=(SELECT id FROM projection_solicitacoes_matricula WHERE codigo_solicitacao=$1) AND event_type='SolicitacaoMatriculaVinculada'`, codigo).Scan(&vinculacoes); err != nil {
		t.Fatal(err)
	}
	if vinculacoes != 1 {
		t.Fatalf("eventos de vinculação = %d, queria 1", vinculacoes)
	}
}
