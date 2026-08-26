package finance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"spuri/internal/db"
)

type appyPayMockTransport struct {
	status string
}

func (t *appyPayMockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body := `{"id":"provider-charge","status":"Pending"}`
	switch {
	case strings.Contains(req.URL.Path, "/oauth2/token"):
		body = `{"access_token":"test-token","expires_in":3600}`
	case req.Method == http.MethodGet:
		providerID := strings.TrimPrefix(req.URL.EscapedPath(), "/v2.0/charges/")
		if providerID == req.URL.EscapedPath() || providerID == "" {
			providerID = req.URL.Query().Get("merchantTransactionId")
		}
		body = `{"id":"` + providerID + `","status":"` + t.status + `"}`
	case strings.HasSuffix(req.URL.Path, "/qr-codes"):
		body = `{"id":"` + t.providerID("qr") + `","status":"Pending","qrCodeArr":"base64-qr"}`
	case req.Method == http.MethodPost:
		body = `{"id":"` + t.providerID("charge") + `","status":"Pending"}`
	}
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
}

func (t *appyPayMockTransport) providerID(kind string) string {
	return fmt.Sprintf("provider-%s-%s", kind, uuid.NewString())
}

func integrationMerchant(prefix string) string {
	return prefix + strings.ReplaceAll(uuid.NewString(), "-", "")[:15-len(prefix)]
}

func configureIntegrationCredential(t *testing.T, service *Service, contexto, academia string) {
	t.Helper()
	_, _, err := service.ConfigureCredential(context.Background(), nil, CredentialInput{
		ContextoTipo:     contexto,
		CodigoAcademia:   academia,
		ClientID:         "integration-client",
		ClientSecret:     "integration-secret",
		GPOPaymentMethod: "GPO_INTEGRATION",
		REFPaymentMethod: "REF_INTEGRATION",
	}, "integration-test", "sistema", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
}

func integrationClient(t *testing.T) *db.Client {
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

func seedMatriculaPendente(t *testing.T, client *db.Client, academia string, valor float64) string {
	t.Helper()
	codigo := "SOL" + strings.ReplaceAll(uuid.NewString(), "-", "")[:8]
	_, err := client.DB().Exec(`INSERT INTO projection_solicitacoes_matricula
		(id,codigo_solicitacao,codigo_academia,nome,genero,data_nascimento,email,telefone,telefone_encarregado,bilhete_identidade,bilhete_identidade_encarregado,ano_escolar_fundamental,status,documentos,codigo_estudante_gerado,valor_matricula,metodos_pagamento_matricula,created_at,updated_at)
		VALUES ($1,$2,$3,'Estudante de integração','feminino','2012-01-02',$4,$5,$6,$7,$8,'6_ano_fundamental','aprovada_pendente_pagamento_matricula','{}'::jsonb,$9,$10,ARRAY['REF'],CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
		uuid.New(), codigo, academia, codigo+"@example.test", "244"+codigo[3:], "923"+codigo[3:], "BI-"+codigo, "BI-RESP-"+codigo, "EST"+codigo[3:7], valor)
	if err != nil {
		t.Fatal(err)
	}
	return codigo
}

func TestIntegrationAcceptWebhookIsIdempotent(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	eventID := "evt-" + uuid.NewString()
	owner := WebhookOwner{CredentialID: uuid.New(), ContextoTipo: ContextoAcademia, CodigoAcademia: "INTWEBHOOK"}
	payload := map[string]any{"id": eventID, "status": "Paid"}

	accepted, err := service.AcceptWebhook(context.Background(), "GPO", eventID, owner, payload)
	if err != nil || !accepted {
		t.Fatalf("primeiro webhook = accepted %t, err %v", accepted, err)
	}
	accepted, err = service.AcceptWebhook(context.Background(), "GPO", eventID, owner, payload)
	if err != nil || accepted {
		t.Fatalf("webhook repetido = accepted %t, err %v", accepted, err)
	}

	var received, ledger int
	if err = client.DB().QueryRow(`SELECT COUNT(*) FROM financeiro_webhooks_recebidos WHERE event_id=$1`, eventID).Scan(&received); err != nil {
		t.Fatal(err)
	}
	if err = client.DB().QueryRow(`SELECT COUNT(*) FROM spuri_ledger WHERE aggregate_type='Financeiro' AND event_type='WebhookAppyPayRecebido' AND payload->>'event_id'=$1`, eventID).Scan(&ledger); err != nil {
		t.Fatal(err)
	}
	if received != 1 || ledger != 1 {
		t.Fatalf("efeito duplicado: recebidos=%d ledger=%d", received, ledger)
	}
}

func TestIntegrationMatriculaPagamentoFixaValorImpedeDuplicidadeECancelaEmCascata(t *testing.T) {
	client := integrationClient(t)
	t.Setenv("ENV", "test")
	t.Setenv("APPYPAY_RESOURCE", "integration-resource")
	t.Setenv("FINANCE_ENCRYPTION_KEY", "test-only-secret-material-at-least-32")
	service := NewService(client)
	service.httpClient = &http.Client{Transport: &appyPayMockTransport{status: "Pending"}}
	academia := "MAT" + uuid.NewString()[:8]
	configureIntegrationCredential(t, service, ContextoAcademia, academia)
	codigo := seedMatriculaPendente(t, client, academia, 1250.50)

	var estudantes int
	if err := client.DB().QueryRow(`SELECT COUNT(*) FROM projection_estudantes WHERE codigo_estudante=$1`, "EST"+codigo[3:7]).Scan(&estudantes); err != nil {
		t.Fatal(err)
	}
	if estudantes != 0 {
		t.Fatal("solicitacao pendente criou estudante antes do pagamento")
	}
	primeira, err := service.IniciarPagamentoMatricula(context.Background(), MatriculaPagamentoInput{CodigoSolicitacao: codigo, MetodoPagamento: "REF"}, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	var valor float64
	if err = client.DB().QueryRow(`SELECT (payload->>'amount')::float8 FROM financeiro_cobrancas WHERE id=$1`, primeira.Charge.ID).Scan(&valor); err != nil {
		t.Fatal(err)
	}
	if valor != 1250.50 {
		t.Fatalf("valor da cobrança = %.2f, queria o valor fixado 1250.50", valor)
	}
	if _, err = service.IniciarPagamentoMatricula(context.Background(), MatriculaPagamentoInput{CodigoSolicitacao: codigo, MetodoPagamento: "REF"}, "127.0.0.1"); err == nil {
		t.Fatal("segunda cobrança de matrícula aberta foi aceita")
	}
	if err = service.CancelarCobrancaMatriculaAberta(context.Background(), codigo, "solicitação cancelada", uuid.NewString(), "academia", "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	var status string
	if err = client.DB().QueryRow(`SELECT payload->>'status' FROM financeiro_cobrancas WHERE id=$1`, primeira.Charge.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "cancelada" {
		t.Fatalf("cobrança após cancelamento = %q", status)
	}
}

func TestIntegrationMatriculaWebhookTardioMantemCancelamentoERegistraConflito(t *testing.T) {
	client := integrationClient(t)
	t.Setenv("ENV", "test")
	t.Setenv("APPYPAY_RESOURCE", "integration-resource")
	t.Setenv("FINANCE_ENCRYPTION_KEY", "test-only-secret-material-at-least-32")
	service := NewService(client)
	service.httpClient = &http.Client{Transport: &appyPayMockTransport{status: "Pending"}}
	academia := "MAT" + uuid.NewString()[:8]
	configureIntegrationCredential(t, service, ContextoAcademia, academia)
	codigo := seedMatriculaPendente(t, client, academia, 900)
	charge, err := service.IniciarPagamentoMatricula(context.Background(), MatriculaPagamentoInput{CodigoSolicitacao: codigo, MetodoPagamento: "REF"}, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if err = service.CancelarCobrancaMatriculaAberta(context.Background(), codigo, "solicitação cancelada", uuid.NewString(), "academia", "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	accepted, err := service.AcceptWebhook(context.Background(), "REF", charge.Charge.ProviderChargeID, WebhookOwner{CredentialID: uuid.New(), ContextoTipo: ContextoAcademia, CodigoAcademia: academia}, map[string]any{"id": charge.Charge.ProviderChargeID, "status": "Success"})
	if err != nil || !accepted {
		t.Fatalf("webhook tardio = accepted %t, err %v", accepted, err)
	}
	var status, codigoNoEvento string
	if err = client.DB().QueryRow(`SELECT payload->>'status',payload->>'codigo_solicitacao' FROM financeiro_cobrancas WHERE id=$1`, charge.Charge.ID).Scan(&status, &codigoNoEvento); err != nil {
		t.Fatal(err)
	}
	if status != "cancelada" || codigoNoEvento != codigo {
		t.Fatalf("webhook tardio alterou cobrança: status=%q codigo=%q", status, codigoNoEvento)
	}
	var conflitos int
	if err = client.DB().QueryRow(`SELECT COUNT(*) FROM spuri_ledger WHERE aggregate_type='Financeiro' AND aggregate_id=$1 AND event_type='CobrancaAppyPayConflitoPosCancelamento' AND payload->>'codigo_solicitacao'=$2`, charge.Charge.ID, codigo).Scan(&conflitos); err != nil {
		t.Fatal(err)
	}
	if conflitos != 1 {
		t.Fatalf("conflitos pós-cancelamento = %d, queria 1", conflitos)
	}
}

// TestIntegrationAcceptWebhookReflecteEstadoNaoSucesso cobre a
// generalização de AcceptWebhook feita nesta tarefa: antes, só um webhook
// de sucesso atualizava financeiro_cobrancas — um webhook avisando que uma
// referência REF expirou (ou que um GPO foi recusado) era gravado em
// WebhookAppyPayRecebido mas nunca refletia na cobrança, que ficava presa
// em aguardando_pagamento até alguém consultá-la manualmente. Cobre
// também a guarda que impede um segundo webhook terminal de sobrescrever
// um estado terminal já registrado.
func TestIntegrationAcceptWebhookReflecteEstadoNaoSucesso(t *testing.T) {
	client := integrationClient(t)
	t.Setenv("ENV", "test")
	t.Setenv("APPYPAY_RESOURCE", "integration-resource")
	t.Setenv("FINANCE_ENCRYPTION_KEY", "test-only-secret-material-at-least-32")
	service := NewService(client)
	service.httpClient = &http.Client{Transport: &appyPayMockTransport{status: "Pending"}}
	academia := "MAT" + uuid.NewString()[:8]
	configureIntegrationCredential(t, service, ContextoAcademia, academia)
	codigo := seedMatriculaPendente(t, client, academia, 900)
	charge, err := service.IniciarPagamentoMatricula(context.Background(), MatriculaPagamentoInput{CodigoSolicitacao: codigo, MetodoPagamento: "REF"}, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if charge.Charge.Status != EstadoCobrancaAguardandoPagamento {
		t.Fatalf("esperava status=%q logo após criar a cobrança, obteve %q", EstadoCobrancaAguardandoPagamento, charge.Charge.Status)
	}

	owner := WebhookOwner{CredentialID: uuid.New(), ContextoTipo: ContextoAcademia, CodigoAcademia: academia}
	accepted, err := service.AcceptWebhook(context.Background(), "REF", charge.Charge.ProviderChargeID, owner, map[string]any{"id": charge.Charge.ProviderChargeID, "status": "Expired"})
	if err != nil || !accepted {
		t.Fatalf("webhook Expired = accepted %t, err %v", accepted, err)
	}
	statusAtual := func() string {
		var status string
		if err := client.DB().QueryRow(`SELECT payload->>'status' FROM financeiro_cobrancas WHERE id=$1`, charge.Charge.ID).Scan(&status); err != nil {
			t.Fatal(err)
		}
		return status
	}
	if got := statusAtual(); got != "Expired" {
		t.Fatalf("esperava status=Expired refletido na cobrança após o webhook, obteve %q", got)
	}

	// Um segundo webhook tardio (ex.: reentrega), com um estado terminal
	// DIFERENTE, não deve sobrescrever o Expired já registrado — só um
	// eventID diferente (aqui o id interno da cobrança, que loadCharge
	// também reconhece) passa pela deduplicação de
	// financeiro_webhooks_recebidos para realmente exercer a guarda.
	accepted2, err := service.AcceptWebhook(context.Background(), "REF", charge.Charge.ID.String(), owner, map[string]any{"id": charge.Charge.ProviderChargeID, "status": "Failed"})
	if err != nil || !accepted2 {
		t.Fatalf("segundo webhook = accepted %t, err %v", accepted2, err)
	}
	if got := statusAtual(); got != "Expired" {
		t.Fatalf("um segundo webhook terminal não deveria sobrescrever Expired, obteve %q", got)
	}

	var confirmacoes int
	if err = client.DB().QueryRow(`SELECT COUNT(*) FROM spuri_ledger WHERE aggregate_type='Financeiro' AND aggregate_id=$1 AND event_type='MensalidadesCobrancaConfirmada'`, charge.Charge.ID).Scan(&confirmacoes); err != nil {
		t.Fatal(err)
	}
	if confirmacoes != 0 {
		t.Fatalf("um webhook não-sucesso nunca deveria confirmar pagamento, obteve %d confirmações", confirmacoes)
	}
}

func TestIntegrationWebhookSecretGeneratedOnceGlobalHeaderAndRotation(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()
	t.Setenv("ENV", "test")
	t.Setenv("FINANCE_ENCRYPTION_KEY", "test-only-secret-material-at-least-32")

	academia := "INT" + uuid.NewString()[:8]
	created, firstSecret, err := service.ConfigureCredential(ctx, nil, CredentialInput{
		ContextoTipo:     ContextoAcademia,
		CodigoAcademia:   academia,
		ClientID:         "client-webhook",
		ClientSecret:     "secret-webhook",
		GPOPaymentMethod: "GPO_WEBHOOK",
		REFPaymentMethod: "REF_WEBHOOK",
	}, "integration-test", "sistema", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if firstSecret == "" {
		t.Fatal("nenhum segredo de webhook foi gerado na criação da credencial")
	}
	if created.WebhookHeaderName != WebhookHeaderName {
		t.Fatalf("view não expõe a constante global de cabeçalho: %q", created.WebhookHeaderName)
	}

	stored, err := service.WebhookSecret(ctx, created.ID)
	if err != nil || stored != firstSecret {
		t.Fatalf("WebhookSecret() = %q, %v; queria %q", stored, err, firstSecret)
	}

	contexto, resolvedAcademia, err := service.CredentialScope(ctx, created.ID)
	if err != nil || contexto != ContextoAcademia || resolvedAcademia != academia {
		t.Fatalf("CredentialScope() = %q, %q, %v", contexto, resolvedAcademia, err)
	}

	okHeaders := http.Header{}
	okHeaders.Set(WebhookHeaderName, firstSecret)
	owner, err := service.AuthenticateWebhook(ctx, okHeaders)
	if err != nil || owner.CredentialID != created.ID {
		t.Fatalf("segredo correto não autenticou: owner=%#v err=%v", owner, err)
	}

	wrongValueHeaders := http.Header{}
	wrongValueHeaders.Set(WebhookHeaderName, "valor-errado")
	if _, err = service.AuthenticateWebhook(ctx, wrongValueHeaders); err == nil {
		t.Fatal("valor de segredo errado autenticou")
	}

	wrongHeaderNameHeaders := http.Header{}
	wrongHeaderNameHeaders.Set("X-API-Key", firstSecret)
	if _, err = service.AuthenticateWebhook(ctx, wrongHeaderNameHeaders); err == nil {
		t.Fatal("cabeçalho fora do nome global autenticou")
	}

	if _, err = service.AuthenticateWebhook(ctx, http.Header{}); err == nil {
		t.Fatal("requisição sem nenhum cabeçalho autenticou")
	}

	updated, secondSecret, err := service.ConfigureCredential(ctx, &created.ID, CredentialInput{
		ContextoTipo:     ContextoAcademia,
		CodigoAcademia:   academia,
		ClientID:         "client-webhook-atualizado",
		ClientSecret:     "secret-webhook-atualizado",
		GPOPaymentMethod: "GPO_WEBHOOK",
		REFPaymentMethod: "REF_WEBHOOK",
	}, "integration-test", "sistema", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if secondSecret != "" {
		t.Fatalf("atualização de credencial já existente regenerou o segredo: %q", secondSecret)
	}
	if stored, err = service.WebhookSecret(ctx, updated.ID); err != nil || stored != firstSecret {
		t.Fatalf("atualização alterou o segredo existente: %q, %v", stored, err)
	}

	rotated, err := service.RotateWebhookSecret(ctx, created.ID, "integration-test", "sistema", "127.0.0.1")
	if err != nil || rotated == "" || rotated == firstSecret {
		t.Fatalf("rotação inválida: %q, %v", rotated, err)
	}
	if stored, err = service.WebhookSecret(ctx, created.ID); err != nil || stored != rotated {
		t.Fatalf("segredo pós-rotação = %q, %v; queria %q", stored, err, rotated)
	}
	oldHeaders := http.Header{}
	oldHeaders.Set(WebhookHeaderName, firstSecret)
	if _, err = service.AuthenticateWebhook(ctx, oldHeaders); err == nil {
		t.Fatal("segredo antigo continuou autenticando após rotação")
	}
	newHeaders := http.Header{}
	newHeaders.Set(WebhookHeaderName, rotated)
	if owner, err = service.AuthenticateWebhook(ctx, newHeaders); err != nil || owner.CredentialID != created.ID {
		t.Fatalf("segredo novo pós-rotação não autenticou: owner=%#v err=%v", owner, err)
	}

	var rotationEvents int
	if err = client.DB().QueryRow(`SELECT COUNT(*) FROM spuri_ledger WHERE aggregate_id=$1 AND event_type='SegredoWebhookAppyPayRotacionado'`, created.ID).Scan(&rotationEvents); err != nil {
		t.Fatal(err)
	}
	if rotationEvents != 1 {
		t.Fatalf("eventos de rotação registrados = %d, queria 1", rotationEvents)
	}
}

func TestIntegrationCancelChargeAndLateSuccessConflict(t *testing.T) {
	client := integrationClient(t)
	t.Setenv("ENV", "test")
	t.Setenv("APPYPAY_RESOURCE", "integration-resource")
	t.Setenv("FINANCE_ENCRYPTION_KEY", "test-only-secret-material-at-least-32")
	ctx := context.Background()
	mock := &appyPayMockTransport{status: "Pending"}
	service := NewService(client)
	service.httpClient = &http.Client{Transport: mock}
	configureIntegrationCredential(t, service, ContextoSpuri, "")
	configureIntegrationCredential(t, service, ContextoAcademia, "CANCELACA1")
	configureIntegrationCredential(t, service, ContextoAcademia, "CANCELACA2")

	newCharge := func(contexto, academia, merchant string) ChargeResult {
		t.Helper()
		out, err := service.CreateCharge(ctx, ChargeRequest{ContextoTipo: contexto, CodigoAcademia: academia, Amount: 10, Description: "Cobrança de teste", MerchantTransactionID: merchant, PaymentMethod: "REF"}, "integration-test", "admin", "127.0.0.1")
		if err != nil {
			t.Fatal(err)
		}
		return out
	}

	spuri := newCharge(ContextoSpuri, "", integrationMerchant("S"))
	cancelled, err := service.CancelCharge(ctx, ContextoSpuri, "", spuri.MerchantTransactionID, "emitida em duplicado", "fpp-test", "admin", "127.0.0.1")
	if err != nil || cancelled.Status != "cancelada" {
		t.Fatalf("cancelamento Spuri = %#v, %v", cancelled, err)
	}

	academy := newCharge(ContextoAcademia, "CANCELACA1", integrationMerchant("A"))
	if _, err = service.CancelCharge(ctx, ContextoAcademia, "CANCELACA1", academy.MerchantTransactionID, "anulada", "academy-test", "academia", "127.0.0.1"); err != nil {
		t.Fatalf("academia dona não cancelou a cobrança: %v", err)
	}
	foreign := newCharge(ContextoAcademia, "CANCELACA2", integrationMerchant("B"))
	if _, err = service.CancelCharge(ctx, ContextoAcademia, "CANCELACA1", foreign.MerchantTransactionID, "", "academy-test", "academia", "127.0.0.1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("academia externa recebeu %v, queria ErrNotFound", err)
	}
	if _, err = service.CancelCharge(ctx, ContextoAcademia, "CANCELACA2", foreign.MerchantTransactionID, "", "fpp-test", "admin", "127.0.0.1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("admin cancelou cobrança de academia: %v", err)
	}

	if _, err = service.CancelCharge(ctx, ContextoSpuri, "", spuri.MerchantTransactionID, "", "fpp-test", "admin", "127.0.0.1"); err == nil {
		t.Fatal("cobrança já cancelada foi cancelada novamente")
	}

	paid := newCharge(ContextoSpuri, "", integrationMerchant("P"))
	mock.status = "Success"
	result, err := service.CancelCharge(ctx, ContextoSpuri, "", paid.MerchantTransactionID, "", "fpp-test", "admin", "127.0.0.1")
	if err == nil || result.Status != "Success" {
		t.Fatalf("cancelamento de cobrança paga = %#v, %v", result, err)
	}
	var canceledEvents int
	if err = client.DB().QueryRow(`SELECT COUNT(*) FROM spuri_ledger WHERE aggregate_id=$1 AND event_type='CobrancaAppyPayCancelada'`, paid.ID).Scan(&canceledEvents); err != nil {
		t.Fatal(err)
	}
	if canceledEvents != 0 {
		t.Fatal("cobrança paga gravou evento de cancelamento")
	}

	mock.status = "Success"
	conflict, err := service.ConsultCharge(ctx, ContextoSpuri, "", spuri.MerchantTransactionID, "fpp-test", "admin", "127.0.0.1")
	if err != nil || conflict.Status != "cancelada" {
		t.Fatalf("sucesso tardio alterou cobrança cancelada: %#v, %v", conflict, err)
	}
	var conflicts int
	if err = client.DB().QueryRow(`SELECT COUNT(*) FROM spuri_ledger WHERE aggregate_id=$1 AND event_type='CobrancaAppyPayConflitoPosCancelamento'`, spuri.ID).Scan(&conflicts); err != nil {
		t.Fatal(err)
	}
	if conflicts != 1 {
		t.Fatalf("conflitos pós-cancelamento = %d, queria 1", conflicts)
	}

	mock.status = "Pending"
	qr, err := service.CreateGPOQRCode(ctx, QRCodeRequest{ContextoTipo: ContextoAcademia, CodigoAcademia: "CANCELACA1", Amount: 10, Description: "QR de teste", MerchantTransactionID: integrationMerchant("Q")}, "academy-test", "academia", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.CancelCharge(ctx, ContextoAcademia, "CANCELACA1", qr.MerchantTransactionID, "QR substituído", "academy-test", "academia", "127.0.0.1"); err != nil {
		t.Fatalf("QR Code não foi cancelado: %v", err)
	}

	failedID := uuid.New()
	failedMerchant := integrationMerchant("F")
	failedPayload, err := json.Marshal(map[string]any{"charge_id": failedID.String(), "contexto_tipo": ContextoSpuri, "merchant_transaction_id": failedMerchant, "status": "falhada"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.DB().ExecContext(ctx, `INSERT INTO financeiro_cobrancas (id,merchant_transaction_id,contexto_tipo,payload) VALUES ($1,$2,$3,$4)`, failedID, failedMerchant, ContextoSpuri, failedPayload); err != nil {
		t.Fatal(err)
	}
	if _, err = service.CancelCharge(ctx, ContextoSpuri, "", failedMerchant, "", "fpp-test", "admin", "127.0.0.1"); err == nil {
		t.Fatal("cobrança falhada foi cancelada")
	}
}
