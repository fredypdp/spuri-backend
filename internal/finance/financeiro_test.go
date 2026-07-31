package finance

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func cred(t *testing.T, cod string, ctx ContextoTipo) CredencialInput {
	t.Helper()
	t.Setenv(AppyPayAPIBaseURLEnv, "https://api")
	return CredencialInput{ContextoTipo: ctx, CodigoAcademia: cod, Ambiente: AmbienteTeste, AuthBaseURL: "https://login", ClientID: "client", ClientSecret: "super-secret", Resource: "res", WebhookSecret: "hook-secret", Applications: []ApplicationInput{{PaymentMethod: "REF", ApplicationID: "app", APIKey: "api-secret-key"}}}
}
func TestCredenciaisCriptografadasEMascaradas(t *testing.T) {
	s := NewService(nil)
	c, err := s.CriarCredencial(context.Background(), cred(t, "", ContextoSpuri), "u", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if c.ClientSecretEncrypted != "" {
		t.Fatal("segredo criptografado vazou na resposta")
	}
	if c.ClientSecretMask != "****cret" {
		t.Fatalf("mask=%q", c.ClientSecretMask)
	}
	stored := s.creds[c.ID]
	if stored.ClientSecretEncrypted == "" || strings.Contains(stored.ClientSecretEncrypted, "super-secret") {
		t.Fatal("segredo não foi criptografado em repouso")
	}
	if stored.Applications[0].APIKeyMask != "****-key" || strings.Contains(stored.Applications[0].APIKeyMask, "api-secret-key") {
		t.Fatal("apiKey não foi mascarada")
	}
}

func TestEventosCredenciaisAuditaveisComAutor(t *testing.T) {
	s := NewService(nil)
	c, err := s.CriarCredencial(context.Background(), cred(t, "", ContextoSpuri), "user-1", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.TestarCredencial(context.Background(), c.ID, "user-2", "admin", ""); err != nil {
		t.Fatal(err)
	}
	c, err = s.AlterarStatusCredencial(c.ID, StatusAtivo, "user-3", "admin", "", "ativar")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range c.Historico {
		if e.AutorID == "" {
			t.Fatalf("evento %s sem autor", e.Tipo)
		}
	}
}

func TestIsolamentoAcademiasEIdempotencia(t *testing.T) {
	s := NewService(nil)
	ca, _ := s.CriarCredencial(context.Background(), cred(t, "ACA", ContextoAcademia), "u", "admin")
	cb, _ := s.CriarCredencial(context.Background(), cred(t, "ACB", ContextoAcademia), "u", "admin")
	s.AlterarStatusCredencial(ca.ID, StatusAtivo, "u", "admin", "", "")
	s.AlterarStatusCredencial(cb.ID, StatusAtivo, "u", "admin", "", "")
	s.AlterarModalidade("academia", "ACA", true, "u", "admin", "")
	if _, err := s.ObterCredencial(cb.ID, "academia", "ACA"); err == nil {
		t.Fatal("academia A consultou credencial B")
	}
	in := GerarCobrancaInput{ContextoTipo: ContextoAcademia, CodigoAcademia: "ACA", PagadorTipo: "estudante", PagadorID: "E1", Valor: 1000, Moeda: "AOA", MetodoPagamento: "REF", ReferenciaExterna: "dom-1", Metadata: map[string]string{"codigo_academia_estudante": "ACA"}}
	c1, err := s.GerarCobrancaFinanceiraBase(context.Background(), in, "u")
	if err != nil {
		t.Fatal(err)
	}
	c2, err := s.GerarCobrancaFinanceiraBase(context.Background(), in, "u")
	if err != nil {
		t.Fatal(err)
	}
	if c1.ID != c2.ID {
		t.Fatal("idempotência falhou")
	}
	in.Metadata["codigo_academia_estudante"] = "ACB"
	in.ReferenciaExterna = "dom-2"
	if _, err := s.GerarCobrancaFinanceiraBase(context.Background(), in, "u"); err == nil {
		t.Fatal("cobrou estudante de outra academia")
	}
}
func TestModalidadeDesativadaImpedeAcademiaMasNaoSpuri(t *testing.T) {
	s := NewService(nil)
	ac, _ := s.CriarCredencial(context.Background(), cred(t, "ACA", ContextoAcademia), "u", "admin")
	sp, _ := s.CriarCredencial(context.Background(), cred(t, "", ContextoSpuri), "u", "admin")
	s.AlterarStatusCredencial(ac.ID, StatusAtivo, "u", "admin", "", "")
	s.AlterarStatusCredencial(sp.ID, StatusAtivo, "u", "admin", "", "")
	s.AlterarModalidade("academia", "ACA", true, "u", "admin", "")
	s.AlterarModalidade("global_academias", "", false, "u", "admin", "")
	_, err := s.GerarCobrancaFinanceiraBase(context.Background(), GerarCobrancaInput{ContextoTipo: ContextoAcademia, CodigoAcademia: "ACA", PagadorTipo: "estudante", PagadorID: "E", Valor: 1, MetodoPagamento: "REF", ReferenciaExterna: "a"}, "u")
	if err == nil {
		t.Fatal("modalidade global inativa permitiu cobrança")
	}
	_, err = s.GerarCobrancaFinanceiraBase(context.Background(), GerarCobrancaInput{ContextoTipo: ContextoSpuri, CodigoAcademia: "ACA", PagadorTipo: "academia", PagadorID: "ACA", Valor: 1, MetodoPagamento: "REF", ReferenciaExterna: "s"}, "u")
	if err != nil {
		t.Fatal(err)
	}
}

func TestMoedaSempreAOA(t *testing.T) {
	s := NewService(nil)
	sp, err := s.CriarCredencial(context.Background(), cred(t, "", ContextoSpuri), "u", "admin")
	if err != nil {
		t.Fatal(err)
	}
	s.AlterarStatusCredencial(sp.ID, StatusAtivo, "u", "admin", "", "")
	ch, err := s.GerarCobrancaFinanceiraBase(context.Background(), GerarCobrancaInput{ContextoTipo: ContextoSpuri, CodigoAcademia: "ACA", PagadorTipo: "academia", PagadorID: "ACA", Valor: 1, Moeda: "USD", MetodoPagamento: "REF", ReferenciaExterna: "moeda"}, "u")
	if err != nil {
		t.Fatal(err)
	}
	if ch.Moeda != "AOA" {
		t.Fatalf("moeda deve permanecer AOA, obtida %q", ch.Moeda)
	}
}

func TestWebhookDupENaoLiquidaSemConsulta(t *testing.T) {
	s := NewService(nil)
	sp, _ := s.CriarCredencial(context.Background(), cred(t, "", ContextoSpuri), "u", "admin")
	s.AlterarStatusCredencial(sp.ID, StatusAtivo, "u", "admin", "", "")
	ch, _ := s.GerarCobrancaFinanceiraBase(context.Background(), GerarCobrancaInput{ContextoTipo: ContextoSpuri, CodigoAcademia: "ACA", PagadorTipo: "academia", PagadorID: "ACA", Valor: 1, MetodoPagamento: "REF", ReferenciaExterna: "w"}, "u")
	ok, err := s.ProcessarWebhookFinanceiroBase(context.Background(), "evt1", ch.ID, "u")
	if err != nil || !ok {
		t.Fatalf("webhook err=%v ok=%v", err, ok)
	}
	ok, err = s.ProcessarWebhookFinanceiroBase(context.Background(), "evt1", ch.ID, "u")
	if err != nil || ok {
		t.Fatalf("duplicado não ignorado")
	}
	got, _ := s.ConsultarCobrancaFinanceiraBase(ch.ID)
	for _, e := range got.Historico {
		if e.AutorID == "" {
			t.Fatalf("evento %s sem autor", e.Tipo)
		}
	}
	if got.Status != CobrancaLiquidada {
		t.Fatalf("status=%s", got.Status)
	}
}

func TestAutorObrigatorioParaEventosFinanceiros(t *testing.T) {
	s := NewService(nil)
	if _, err := s.CriarCredencial(context.Background(), cred(t, "", ContextoSpuri), "", "admin"); err == nil {
		t.Fatal("criação de credencial sem autor deveria falhar")
	}
	c, err := s.CriarCredencial(context.Background(), cred(t, "", ContextoSpuri), "u", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AlterarStatusCredencial(c.ID, StatusAtivo, "", "admin", "", ""); err == nil {
		t.Fatal("alteração de status sem autor deveria falhar")
	}
	if _, err := s.ReconciliarFinanceiroBase(context.Background(), ""); err == nil {
		t.Fatal("reconciliação sem autor deveria falhar")
	}
}

type memoriaLedger struct{ events []LedgerEvent }

func (m *memoriaLedger) AppendFinanceEvent(ctx context.Context, event LedgerEvent, autorID, autorTipo, origem string) (int, error) {
	m.events = append(m.events, event)
	return len(m.events), nil
}
func (m *memoriaLedger) LoadFinanceEvents(ctx context.Context) ([]LedgerEvent, error) {
	return append([]LedgerEvent(nil), m.events...), nil
}

func TestCredencialGravaLedgerSemSegredoClaro(t *testing.T) {
	l := &memoriaLedger{}
	s := NewServiceWithDBAndLedger(nil, nil, l)
	out, err := s.CriarCredencial(context.Background(), cred(t, "", ContextoSpuri), "u", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if len(l.events) != 1 || l.events[0].EventType != "CredenciaisAppyPayCadastradas" {
		t.Fatalf("eventos=%v", l.events)
	}
	raw := strings.ToLower(func() string { b, _ := json.Marshal(l.events[0].Payload); return string(b) }())
	for _, secret := range []string{"super-secret", "hook-secret", "api-secret-key"} {
		if strings.Contains(raw, secret) {
			t.Fatalf("segredo %q vazou no ledger: %s", secret, raw)
		}
	}
	if out.ClientSecretEncrypted != "" || out.WebhookSecretEncrypted != "" {
		t.Fatal("ciphertext vazou na resposta")
	}
}

func TestReplayReconstróiModalidadeEIdempotencia(t *testing.T) {
	l := &memoriaLedger{}
	s := NewServiceWithDBAndLedger(nil, nil, l)
	sp, err := s.CriarCredencial(context.Background(), cred(t, "", ContextoSpuri), "u", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AlterarStatusCredencial(sp.ID, StatusAtivo, "u", "admin", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.AlterarModalidade("spuri", "", true, "u", "admin", ""); err != nil {
		t.Fatal(err)
	}
	in := GerarCobrancaInput{ContextoTipo: ContextoSpuri, PagadorTipo: "academia", PagadorID: "ACA", Valor: 10, MetodoPagamento: "REF", ReferenciaExterna: "idem"}
	ch, err := s.GerarCobrancaFinanceiraBase(context.Background(), in, "u")
	if err != nil {
		t.Fatal(err)
	}
	rebuilt := NewServiceWithDBAndLedger(nil, nil, l)
	got, err := rebuilt.GerarCobrancaFinanceiraBase(context.Background(), in, "u")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != ch.ID {
		t.Fatalf("replay não reconstruiu índice idempotente: %s != %s", got.ID, ch.ID)
	}
}

func TestEstudanteNaoAcessaCredenciaisFinanceiras(t *testing.T) {
	s := NewService(nil)
	ca, err := s.CriarCredencial(context.Background(), cred(t, "ACA", ContextoAcademia), "u", "admin")
	if err != nil {
		t.Fatal(err)
	}
	sp, err := s.CriarCredencial(context.Background(), cred(t, "", ContextoSpuri), "u", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if got := s.ListarCredenciais("estudante", "ACA"); len(got) != 0 {
		t.Fatalf("estudante listou credenciais: %d", len(got))
	}
	if _, err := s.ObterCredencial(ca.ID, "estudante", "ACA"); err == nil {
		t.Fatal("estudante obteve credencial de academia")
	}
	if err := s.TestarCredencial(context.Background(), sp.ID, "est", "estudante", "ACA"); err == nil {
		t.Fatal("estudante testou credencial financeira")
	}
}

func TestReplayPreservaHistoricoEMotivo(t *testing.T) {
	l := &memoriaLedger{}
	s := NewServiceWithDBAndLedger(nil, nil, l)
	sp, err := s.CriarCredencial(context.Background(), cred(t, "", ContextoSpuri), "u", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AlterarStatusCredencial(sp.ID, StatusAtivo, "u", "admin", "", "ativação operacional"); err != nil {
		t.Fatal(err)
	}
	rebuilt := NewServiceWithDBAndLedger(nil, nil, l)
	got, err := rebuilt.ObterCredencial(sp.ID, "admin", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Historico) != 2 || got.Historico[1].Motivo != "ativação operacional" {
		t.Fatalf("histórico/motivo não preservado no replay: %#v", got.Historico)
	}
}

func TestEncryptDecryptSegredoFinanceiro(t *testing.T) {
	ciphertext, err := encrypt("super-secret")
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := decrypt(ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	if plaintext != "super-secret" {
		t.Fatalf("decrypt retornou %q", plaintext)
	}
}

func TestValidateEncryptionConfigUsaENVProduction(t *testing.T) {
	t.Setenv("ENV", "production")
	t.Setenv("GO_ENV", "")
	t.Setenv("FINANCE_ENCRYPTION_KEY", "")
	if err := ValidateEncryptionConfig(); err == nil {
		t.Fatalf("esperava erro sem FINANCE_ENCRYPTION_KEY quando ENV=production")
	}

	t.Setenv("FINANCE_ENCRYPTION_KEY", "chave-de-teste")
	if err := ValidateEncryptionConfig(); err != nil {
		t.Fatalf("não esperava erro com FINANCE_ENCRYPTION_KEY configurada: %v", err)
	}
}
