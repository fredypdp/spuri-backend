package finance

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"github.com/google/uuid"
	"spuri/internal/db"
)

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
