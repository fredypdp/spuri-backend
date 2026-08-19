package finance

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestIntegrationRemoveCredentialRespeitaEventSourcing(t *testing.T) {
	client := integrationClient(t)
	academia := mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2026_2027")
	service := NewService(client)

	view, _, err := service.ConfigureCredential(context.Background(), nil, CredentialInput{
		ContextoTipo: ContextoAcademia, CodigoAcademia: academia,
		ClientID: "client-" + uuid.NewString(), ClientSecret: "secret-" + uuid.NewString(),
		GPOPaymentMethod: "GPO_QR_TEST", REFPaymentMethod: "REF_TEST",
	}, uuid.NewString(), "academia", "127.0.0.1")
	if err != nil {
		t.Fatalf("ConfigureCredential falhou: %v", err)
	}

	var segredosAntes int
	if err := client.DB().QueryRow(`SELECT COUNT(*) FROM financeiro_segredos_appypay WHERE credential_id=$1`, view.ID).Scan(&segredosAntes); err != nil {
		t.Fatal(err)
	}
	if segredosAntes == 0 {
		t.Fatal("esperava segredos gravados após ConfigureCredential")
	}

	var eventosConfigurados int
	if err := client.DB().QueryRow(`SELECT COUNT(*) FROM spuri_ledger WHERE aggregate_id=$1 AND event_type='CredenciaisAppyPayConfiguradas'`, view.ID).Scan(&eventosConfigurados); err != nil {
		t.Fatal(err)
	}
	if eventosConfigurados != 1 {
		t.Fatalf("esperava 1 evento CredenciaisAppyPayConfiguradas, obteve %d", eventosConfigurados)
	}

	if err := service.RemoveCredential(context.Background(), ContextoAcademia, academia, uuid.NewString(), "academia", "127.0.0.1"); err != nil {
		t.Fatalf("RemoveCredential não deveria falhar: %v", err)
	}

	// O evento de configuração original PERMANECE no ledger, imutável —
	// event sourcing nunca apaga fatos passados.
	if err := client.DB().QueryRow(`SELECT COUNT(*) FROM spuri_ledger WHERE aggregate_id=$1 AND event_type='CredenciaisAppyPayConfiguradas'`, view.ID).Scan(&eventosConfigurados); err != nil {
		t.Fatal(err)
	}
	if eventosConfigurados != 1 {
		t.Fatalf("o evento CredenciaisAppyPayConfiguradas não deveria desaparecer do ledger, obteve contagem %d", eventosConfigurados)
	}
	var eventosRemovidos int
	if err := client.DB().QueryRow(`SELECT COUNT(*) FROM spuri_ledger WHERE aggregate_id=$1 AND event_type='CredenciaisAppyPayRemovidas'`, view.ID).Scan(&eventosRemovidos); err != nil {
		t.Fatal(err)
	}
	if eventosRemovidos != 1 {
		t.Fatalf("esperava 1 evento CredenciaisAppyPayRemovidas gravado no ledger, obteve %d", eventosRemovidos)
	}

	// A projeção (estado de leitura) não mostra mais a credencial como ativa.
	if _, err := service.findCredentialID(context.Background(), ContextoAcademia, academia, AmbienteAtual()); err == nil {
		t.Fatal("findCredentialID deveria falhar após remoção, mas encontrou a credencial")
	}

	// loadCredential (usado por CreateCharge/CreateGPOQRCode) também bloqueia.
	if _, err := service.loadCredential(context.Background(), ContextoAcademia, academia); !errors.Is(err, ErrNotFound) {
		t.Fatalf("loadCredential deveria falhar com ErrNotFound após remoção, obteve: %v", err)
	}

	// O cofre de segredos foi limpo (não fica material sensível órfão).
	var segredosDepois int
	if err := client.DB().QueryRow(`SELECT COUNT(*) FROM financeiro_segredos_appypay WHERE credential_id=$1`, view.ID).Scan(&segredosDepois); err != nil {
		t.Fatal(err)
	}
	if segredosDepois != 0 {
		t.Fatalf("esperava 0 segredos após remoção, obteve %d", segredosDepois)
	}

	// Remover de novo (idempotência de erro) deve falhar com ErrNotFound,
	// nunca gravar um segundo evento de remoção para algo já removido.
	if err := service.RemoveCredential(context.Background(), ContextoAcademia, academia, uuid.NewString(), "academia", "127.0.0.1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("segunda remoção deveria falhar com ErrNotFound, obteve: %v", err)
	}

	// Reconfigurar depois de remover funciona normalmente (novo evento,
	// nova versão — o histórico de configuração+remoção anterior continua
	// intacto no ledger).
	view2, _, err := service.ConfigureCredential(context.Background(), nil, CredentialInput{
		ContextoTipo: ContextoAcademia, CodigoAcademia: academia,
		ClientID: "client2-" + uuid.NewString(), ClientSecret: "secret2-" + uuid.NewString(),
		GPOPaymentMethod: "GPO_QR_TEST", REFPaymentMethod: "REF_TEST",
	}, uuid.NewString(), "academia", "127.0.0.1")
	if err != nil {
		t.Fatalf("reconfiguração após remoção não deveria falhar: %v", err)
	}
	if _, err := service.loadCredential(context.Background(), ContextoAcademia, academia); err != nil {
		t.Fatalf("loadCredential deveria funcionar após reconfiguração, obteve: %v", err)
	}
	_ = view2

	// Rebuild() a partir do ledger reproduz o MESMO estado final (a
	// credencial ativa é a reconfigurada, não a removida) — prova de que a
	// projeção é 100% derivável do ledger, sem estado escondido.
	proj := service.projection
	if err := proj.Rebuild(); err != nil {
		t.Fatalf("Rebuild não deveria falhar: %v", err)
	}
	if _, err := service.loadCredential(context.Background(), ContextoAcademia, academia); err != nil {
		t.Fatalf("após Rebuild, loadCredential deveria continuar funcionando (última config vale), obteve: %v", err)
	}
}
