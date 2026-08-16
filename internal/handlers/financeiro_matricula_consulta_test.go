package handlers

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"spuri/internal/db"
	"spuri/internal/finance"
	"spuri/internal/projections"
)

type matriculaConsultaMockTransport struct{ status string }

func (t *matriculaConsultaMockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body := `{"id":"provider-charge-consulta","status":"Pending"}`
	switch {
	case strings.Contains(req.URL.Path, "/oauth2/token"):
		body = `{"access_token":"test-token","expires_in":3600}`
	case req.Method == http.MethodGet:
		body = `{"id":"provider-charge-consulta","status":"` + t.status + `"}`
	}
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
}

// TestIntegrationConsultarCobrancaAppyPayNaoEfetivaMatriculaAposSuccess
// reproduz o fluxo de polling: a academia (ou o candidato) consulta o status
// de uma cobrança de matrícula GPO/REF através do endpoint genérico
// GET /financeiro/appypay/cobrancas/:id (o mesmo endpoint e o mesmo caminho
// de código, finance.Service.ConsultCharge, usados pelo fluxo de
// mensalidades e exercitados pelo teste
// TestIntegrationPagamentoMensalidadeConfirmadoPelaAppyPayMarcaComoPago da
// tarefa 42). A cobrança é criada como "Pending" (comportamento real de
// GPO/REF: a criação nunca retorna "Success" de forma síncrona) e só depois
// passa a "Success" na consulta. Diferente do webhook (que já efetiva o
// vínculo corretamente, ver
// TestIntegrationReceberWebhookAppyPayEfetivaVinculoMatricula), este
// caminho nunca chama efetivarVinculoMatriculaPaga: confirmMensalidadeCharge
// é chamado incondicionalmente a partir de ConsultCharge, mas é um no-op
// silencioso para qualquer cobrança cujo payload não contenha
// "mensalidades" (como é o caso de toda cobrança de matrícula). O
// resultado: a cobrança fica "Success" no read model, mas o estudante nunca
// é criado e a solicitação nunca sai de
// "aprovada_pendente_pagamento_matricula".
func TestIntegrationConsultarCobrancaAppyPayNaoEfetivaMatriculaAposSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := integrationFinanceClient(t)
	t.Setenv("ENV", "test")
	t.Setenv("APPYPAY_RESOURCE", "integration-resource")
	t.Setenv("FINANCE_ENCRYPTION_KEY", "test-only-secret-material-at-least-32")

	academia := "WC" + strings.ReplaceAll(uuid.NewString(), "-", "")[:8]
	seedAcademiaParaMatriculaWebhook(t, client, academia)
	codigo, codigoEstudante := seedSolicitacaoMatriculaPendenteComLedger(t, client, academia, 750)

	transport := &matriculaConsultaMockTransport{status: "Pending"}
	service := finance.NewService(client)
	service.SetHTTPClient(&http.Client{Transport: transport})
	if _, _, err := service.ConfigureCredential(context.Background(), nil, finance.CredentialInput{
		ContextoTipo: finance.ContextoAcademia, CodigoAcademia: academia,
		ClientID: "integration-client", ClientSecret: "integration-secret",
		GPOPaymentMethod: "GPO_INTEGRATION", REFPaymentMethod: "REF_INTEGRATION",
	}, "integration-test", "sistema", "127.0.0.1"); err != nil {
		t.Fatal(err)
	}

	charge, err := service.IniciarPagamentoMatricula(context.Background(), finance.MatriculaPagamentoInput{CodigoSolicitacao: codigo, MetodoPagamento: "REF"}, "127.0.0.1")
	if err != nil {
		t.Fatalf("IniciarPagamentoMatricula falhou: %v", err)
	}
	if strings.EqualFold(charge.Charge.Status, "success") {
		t.Fatalf("cobrança REF retornou success na criação, cenário não reproduz o fluxo real (deveria ser Pending): %q", charge.Charge.Status)
	}

	previousService := FinanceiroService
	FinanceiroService = service
	t.Cleanup(func() { FinanceiroService = previousService })

	// AppyPay confirma o pagamento de forma assíncrona (pagamento na
	// referência bancária). A academia/candidato descobre isso consultando o
	// status da cobrança pelo endpoint genérico, exatamente como o teste da
	// tarefa 42 fez para mensalidades via ConsultCharge.
	transport.status = "Success"
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/financeiro/appypay/cobrancas/"+charge.Charge.ID.String(), nil)
	ctx.Params = gin.Params{{Key: "id", Value: charge.Charge.ID.String()}}
	ctx.Set("dbClient", client)
	ctx.Set("repository", db.NewAggregateRepository(client))
	ctx.Set("user_id", uuid.New())
	ctx.Set("user_type", "academia")
	ctx.Set("codigo_academia", academia)

	ConsultarCobrancaAppyPay(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("ConsultarCobrancaAppyPay retornou %d: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(strings.ToLower(recorder.Body.String()), "success") {
		t.Fatalf("consulta não refletiu o status Success da AppyPay: %s", recorder.Body.String())
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
	var estudantes int
	if err := client.DB().QueryRow(`SELECT COUNT(*) FROM projection_estudantes WHERE codigo_estudante=$1`, codigoEstudante).Scan(&estudantes); err != nil {
		t.Fatal(err)
	}

	// A AppyPay confirmou o pagamento (a consulta acima devolveu "Success" e
	// isso já está persistido em financeiro_cobrancas): a solicitação deve
	// estar "aprovada" e o estudante deve existir, exatamente como acontece
	// quando a confirmação chega por webhook
	// (TestIntegrationReceberWebhookAppyPayEfetivaVinculoMatricula).
	if status != "aprovada" {
		t.Fatalf("status da solicitação = %q, esperado \"aprovada\" após a consulta confirmar o pagamento", status)
	}
	if estudantes != 1 {
		t.Fatalf("estudantes criados = %d, esperado 1 após a consulta confirmar o pagamento", estudantes)
	}
}
