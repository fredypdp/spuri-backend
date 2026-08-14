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
	"sync"
	"testing"

	"github.com/google/uuid"
	"spuri/internal/db"
)

type appyPayMockTransport struct {
	status string
	mu     sync.Mutex
	nextID int
}

func (t *appyPayMockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body := `{"id":"provider-charge","status":"Pending"}`
	switch {
	case strings.Contains(req.URL.Path, "/oauth2/token"):
		body = `{"access_token":"test-token","expires_in":3600}`
	case req.Method == http.MethodGet:
		body = `{"id":"provider-charge","status":"` + t.status + `"}`
	case strings.HasSuffix(req.URL.Path, "/qr-codes"):
		body = `{"id":"` + t.providerID("qr") + `","status":"Pending","qrCodeArr":"base64-qr"}`
	case req.Method == http.MethodPost:
		body = `{"id":"` + t.providerID("charge") + `","status":"Pending"}`
	}
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
}

func (t *appyPayMockTransport) providerID(kind string) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.nextID++
	return fmt.Sprintf("provider-%s-%d", kind, t.nextID)
}

func integrationMerchant(prefix string) string {
	return prefix + strings.ReplaceAll(uuid.NewString(), "-", "")[:15-len(prefix)]
}

func configureIntegrationCredential(t *testing.T, service *Service, contexto, academia string) {
	t.Helper()
	_, err := service.ConfigureCredential(context.Background(), nil, CredentialInput{
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
	accepted, err := service.AcceptWebhook(context.Background(), "REF", "evt-"+uuid.NewString(), WebhookOwner{CredentialID: uuid.New(), ContextoTipo: ContextoAcademia, CodigoAcademia: academia}, map[string]any{"id": charge.Charge.ProviderChargeID, "status": "Success"})
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

func TestIntegrationWebhookAuthConfigurableHeaderAndResourceFreeCredentials(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()
	t.Setenv("ENV", "test")
	t.Setenv("FINANCE_ENCRYPTION_KEY", "test-only-secret-material-at-least-32")
	suffix := uuid.NewString()[:8]

	customAcademy := "INT" + uuid.NewString()[:8]
	custom, err := service.ConfigureCredential(ctx, nil, CredentialInput{
		ContextoTipo:      ContextoAcademia,
		CodigoAcademia:    customAcademy,
		ClientID:          "client-custom",
		ClientSecret:      "secret-custom",
		GPOPaymentMethod:  "GPO_CUSTOM",
		REFPaymentMethod:  "REF_CUSTOM",
		WebhookSecret:     "custom-webhook-secret-" + suffix,
		WebhookHeaderName: "X-Spuri-Webhook-Secret",
	}, "integration-test", "sistema", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if custom.WebhookHeaderName != "X-Spuri-Webhook-Secret" {
		t.Fatalf("nome de cabeçalho não persistido: %q", custom.WebhookHeaderName)
	}
	if _, err = service.loadCredential(ctx, ContextoAcademia, customAcademy); err != nil {
		t.Fatalf("credencial sem resource no cofre não recarregou: %v", err)
	}
	customHeaders := http.Header{}
	customHeaders.Set("X-Spuri-Webhook-Secret", "custom-webhook-secret-"+suffix)
	owner, err := service.AuthenticateWebhook(ctx, customHeaders)
	if err != nil || owner.CredentialID != custom.ID {
		t.Fatalf("cabeçalho customizado não autenticou: owner=%#v err=%v", owner, err)
	}
	wrongHeaders := http.Header{}
	wrongHeaders.Set("X-API-Key", "custom-webhook-secret-"+suffix)
	if _, err = service.AuthenticateWebhook(ctx, wrongHeaders); err == nil {
		t.Fatal("X-API-Key autenticou credencial configurada para cabeçalho customizado")
	}

	legacyID := uuid.New()
	legacyAcademy := "INT" + uuid.NewString()[:8]
	legacyPayload, err := json.Marshal(map[string]any{
		"credential_id":       legacyID.String(),
		"contexto_tipo":       ContextoAcademia,
		"codigo_academia":     legacyAcademy,
		"ambiente":            AmbienteTeste,
		"webhook_header_name": "X-API-Key",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.DB().ExecContext(ctx, `INSERT INTO financeiro_credenciais_appypay (id,contexto_tipo,codigo_academia,ambiente,payload) VALUES ($1,$2,$3,$4,$5::jsonb)`, legacyID, ContextoAcademia, legacyAcademy, AmbienteTeste, legacyPayload); err != nil {
		t.Fatal(err)
	}
	if err = service.saveSecrets(ctx, legacyID, map[string]string{"client_id": "legacy-client", "client_secret": "legacy-secret", "gpo_method": "GPO_LEGACY", "ref_method": "REF_LEGACY", "webhook_secret": "legacy-webhook-secret-" + suffix}); err != nil {
		t.Fatal(err)
	}
	legacyHeaders := http.Header{}
	legacyHeaders.Set("X-API-Key", "legacy-webhook-secret-"+suffix)
	owner, err = service.AuthenticateWebhook(ctx, legacyHeaders)
	if err != nil || owner.CredentialID != legacyID {
		t.Fatalf("fallback X-API-Key para credencial legada falhou: owner=%#v err=%v", owner, err)
	}

	noWebhookAcademy := "INT" + uuid.NewString()[:8]
	if _, err = service.ConfigureCredential(ctx, nil, CredentialInput{
		ContextoTipo:     ContextoAcademia,
		CodigoAcademia:   noWebhookAcademy,
		ClientID:         "client-no-webhook",
		ClientSecret:     "secret-no-webhook",
		GPOPaymentMethod: "GPO_NOWEBHOOK",
		REFPaymentMethod: "REF_NOWEBHOOK",
	}, "integration-test", "sistema", "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	noWebhookHeaders := http.Header{}
	noWebhookHeaders.Set("X-API-Key", "")
	if _, err = service.AuthenticateWebhook(ctx, noWebhookHeaders); err == nil {
		t.Fatal("credencial sem webhook_secret configurado não deveria autenticar nada")
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
