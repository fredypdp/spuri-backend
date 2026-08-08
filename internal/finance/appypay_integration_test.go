package finance

import (
	"context"
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
