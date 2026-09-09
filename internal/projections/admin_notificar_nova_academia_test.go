package projections

import (
	"os"
	"testing"

	"spuri/internal/db"
)

// TestGetAdminsParaNotificarNovaAcademiaFiltraPorPermissao valida, contra um
// PostgreSQL real e isolado, que a query usada para decidir quem recebe o
// aviso de "nova academia pendente de ativação" (POST /academia/cadastro)
// retorna exatamente os admins com role 'adm' ou 'fpp', status 'ativo' e
// email_verificado = true — o mesmo critério exigido para efetivamente
// ativar uma academia (PUT /dominis/academia/:codigo/ativar).
func TestGetAdminsParaNotificarNovaAcademiaFiltraPorPermissao(t *testing.T) {
	if os.Getenv("SPURI_RUN_DB_INTEGRITY_TESTS") != "1" {
		t.Skip("set SPURI_RUN_DB_INTEGRITY_TESTS=1 with an isolated PostgreSQL database to run")
	}

	prevDir, _ := os.Getwd()
	_ = os.Chdir("../..")
	t.Cleanup(func() { _ = os.Chdir(prevDir) })

	client, err := db.NewClient(db.DefaultConfig())
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	defer client.Close()
	if err := client.RunMigrations(); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	// Este teste assume um banco isolado/vazio, mesma exigência dos demais
	// testes gated por SPURI_RUN_DB_INTEGRITY_TESTS (ver README de testes).
	if _, err := client.DB().Exec(`TRUNCATE projection_admins CASCADE`); err != nil {
		t.Fatalf("truncate projection_admins: %v", err)
	}

	const bootstrapID = "00000000-0000-0000-0000-0000000000aa"
	if _, err := client.DB().Exec(`
		INSERT INTO projection_admins (id, nome, email, senha_hash, role, status, email_verificado, created_by, created_at, updated_at, version)
		VALUES ($1, 'FPP Bootstrap Verificado Ativo', 'fpp.bootstrap@example.com', 'x', 'fpp', 'ativo', true, NULL, now(), now(), 1)
	`, bootstrapID); err != nil {
		t.Fatalf("insert bootstrap fixture: %v", err)
	}

	insert := func(nome, email, role, status string, verificado bool) {
		t.Helper()
		if _, err := client.DB().Exec(`
			INSERT INTO projection_admins (id, nome, email, senha_hash, role, status, email_verificado, created_by, created_at, updated_at, version)
			VALUES (gen_random_uuid(), $1, $2, 'x', $3, $4, $5, $6, now(), now(), 1)
		`, nome, email, role, status, verificado, bootstrapID); err != nil {
			t.Fatalf("insert fixture %s: %v", email, err)
		}
	}

	insert("ADM Verificado Ativo", "adm.ok@example.com", "adm", "ativo", true)
	insert("Gerente Verificado Ativo", "gerente.ok@example.com", "gerente", "ativo", true)
	insert("ADM Nao Verificado", "adm.naoverificado@example.com", "adm", "ativo", false)
	insert("ADM Inativo Verificado", "adm.inativo@example.com", "adm", "inativo", true)
	insert("FPP Deletado Verificado", "fpp.deletado@example.com", "fpp", "deletado", true)

	proj := NewAdminProjection(client)
	admins, err := proj.GetAdminsParaNotificarNovaAcademia()
	if err != nil {
		t.Fatalf("GetAdminsParaNotificarNovaAcademia: %v", err)
	}

	got := map[string]bool{}
	for _, a := range admins {
		got[a.Email] = true
	}

	wantPresent := []string{"fpp.bootstrap@example.com", "adm.ok@example.com"}
	for _, email := range wantPresent {
		if !got[email] {
			t.Errorf("esperava %s na lista de notificação, não veio. Retornados: %+v", email, got)
		}
	}

	wantAbsent := []string{
		"gerente.ok@example.com",        // role sem permissão de ativar academia
		"adm.naoverificado@example.com", // email não verificado
		"adm.inativo@example.com",       // admin inativo (não consegue agir mesmo notificado)
		"fpp.deletado@example.com",      // admin deletado
	}
	for _, email := range wantAbsent {
		if got[email] {
			t.Errorf("%s não deveria estar na lista de notificação, mas está. Retornados: %+v", email, got)
		}
	}

	if len(admins) != len(wantPresent) {
		t.Errorf("esperava exatamente %d admins retornados, obteve %d: %+v", len(wantPresent), len(admins), admins)
	}
}
