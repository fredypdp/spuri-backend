package finance

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func cred(t *testing.T, cod string, ctx ContextoTipo) CredencialInput {
	t.Helper()
	t.Setenv(AppyPayAPIBaseURLEnv, "https://api")
	t.Setenv("FINANCE_ENCRYPTION_KEY", "chave-de-teste")
	return CredencialInput{ContextoTipo: ctx, CodigoAcademia: cod, Ambiente: AmbienteTeste, AuthBaseURL: "https://login", ClientID: "client", ClientSecret: "super-secret", Resource: "res", WebhookSecret: "hook-secret", Applications: []ApplicationInput{{PaymentMethod: "REF", ApplicationID: "app", APIKey: "api-secret-key"}}}
}
func TestCredenciaisCriptografadasEMascaradas(t *testing.T) {
	s := NewService(nil)
	c, err := s.CriarCredencial(context.Background(), cred(t, "", ContextoSpuri), "u", "fpp")
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
	c, err := s.CriarCredencial(context.Background(), cred(t, "", ContextoSpuri), "user-1", "fpp")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.TestarCredencial(context.Background(), c.ID, "user-2", "fpp", ""); err != nil {
		t.Fatal(err)
	}
	c, err = s.AlterarStatusCredencial(c.ID, StatusAtivo, "user-3", "fpp", "", "ativar")
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
	ca, _ := s.CriarCredencial(context.Background(), cred(t, "ACA", ContextoAcademia), "u", "fpp")
	cb, _ := s.CriarCredencial(context.Background(), cred(t, "ACB", ContextoAcademia), "u", "fpp")
	s.AlterarStatusCredencial(ca.ID, StatusAtivo, "u", "fpp", "", "")
	s.AlterarStatusCredencial(cb.ID, StatusAtivo, "u", "fpp", "", "")
	s.AlterarModalidade("academia", "ACA", true, "u", "fpp", "")
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

func TestGerarCobrancaConcorrenteReservaIdempotenciaAntesDoProvider(t *testing.T) {
	provider := &slowProvider{delay: 50 * time.Millisecond}
	s := NewService(provider)
	sp, err := s.CriarCredencial(context.Background(), cred(t, "", ContextoSpuri), "u", "fpp")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AlterarStatusCredencial(sp.ID, StatusAtivo, "u", "fpp", "", ""); err != nil {
		t.Fatal(err)
	}

	in := GerarCobrancaInput{ContextoTipo: ContextoSpuri, PagadorTipo: "academia", PagadorID: "ACA", Valor: 1000, MetodoPagamento: "REF", ReferenciaExterna: "concorrente-1"}
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make(chan CobrancaFinanceira, 2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			ch, err := s.GerarCobrancaFinanceiraBase(context.Background(), in, "u")
			results <- ch
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var ids []string
	for ch := range results {
		ids = append(ids, ch.ID.String())
	}
	if len(ids) != 2 || ids[0] != ids[1] {
		t.Fatalf("cobranças concorrentes não foram idempotentes: %v", ids)
	}
	if got := provider.calls.Load(); got != 1 {
		t.Fatalf("provider chamado %d vezes; esperado 1", got)
	}
}

func TestAcademiaNaoReatribuiContextoAoAtualizarCredencial(t *testing.T) {
	s := NewService(nil)
	ca, err := s.CriarCredencial(context.Background(), cred(t, "ACA", ContextoAcademia), "u", "fpp")
	if err != nil {
		t.Fatal(err)
	}
	in := cred(t, "ACB", ContextoAcademia)
	in.ClientID = "client-atualizado"
	got, err := s.AtualizarCredencial(context.Background(), ca.ID, in, "acad", "academia", "ACA")
	if err != nil {
		t.Fatal(err)
	}
	if got.ContextoTipo != ContextoAcademia || got.CodigoAcademia != "ACA" {
		t.Fatalf("academia reatribuiu credencial para contexto=%q codigo=%q", got.ContextoTipo, got.CodigoAcademia)
	}
	stored := s.creds[ca.ID]
	if stored.ContextoTipo != ContextoAcademia || stored.CodigoAcademia != "ACA" {
		t.Fatalf("credencial armazenada foi reatribuída para contexto=%q codigo=%q", stored.ContextoTipo, stored.CodigoAcademia)
	}
}

type slowProvider struct {
	delay time.Duration
	calls atomic.Int64
}

func (p *slowProvider) TestarCredencial(context.Context, CredencialAppyPay) error { return nil }
func (p *slowProvider) CriarCobranca(context.Context, CredencialAppyPay, CobrancaFinanceira, Application) (string, string, error) {
	p.calls.Add(1)
	time.Sleep(p.delay)
	return "appy_slow", "PENDING", nil
}
func (p *slowProvider) ConsultarCobranca(context.Context, CredencialAppyPay, CobrancaFinanceira) (string, error) {
	return "PAID", nil
}

func TestModalidadeDesativadaImpedeAcademiaMasNaoSpuri(t *testing.T) {
	s := NewService(nil)
	ac, _ := s.CriarCredencial(context.Background(), cred(t, "ACA", ContextoAcademia), "u", "fpp")
	sp, _ := s.CriarCredencial(context.Background(), cred(t, "", ContextoSpuri), "u", "fpp")
	s.AlterarStatusCredencial(ac.ID, StatusAtivo, "u", "fpp", "", "")
	s.AlterarStatusCredencial(sp.ID, StatusAtivo, "u", "fpp", "", "")
	s.AlterarModalidade("academia", "ACA", true, "u", "fpp", "")
	s.AlterarModalidade("global_academias", "", false, "u", "fpp", "")
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
	sp, err := s.CriarCredencial(context.Background(), cred(t, "", ContextoSpuri), "u", "fpp")
	if err != nil {
		t.Fatal(err)
	}
	s.AlterarStatusCredencial(sp.ID, StatusAtivo, "u", "fpp", "", "")
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
	sp, _ := s.CriarCredencial(context.Background(), cred(t, "", ContextoSpuri), "u", "fpp")
	s.AlterarStatusCredencial(sp.ID, StatusAtivo, "u", "fpp", "", "")
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
	if _, err := s.CriarCredencial(context.Background(), cred(t, "", ContextoSpuri), "", "fpp"); err == nil {
		t.Fatal("criação de credencial sem autor deveria falhar")
	}
	c, err := s.CriarCredencial(context.Background(), cred(t, "", ContextoSpuri), "u", "fpp")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AlterarStatusCredencial(c.ID, StatusAtivo, "", "fpp", "", ""); err == nil {
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
	out, err := s.CriarCredencial(context.Background(), cred(t, "", ContextoSpuri), "u", "fpp")
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
	sp, err := s.CriarCredencial(context.Background(), cred(t, "", ContextoSpuri), "u", "fpp")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AlterarStatusCredencial(sp.ID, StatusAtivo, "u", "fpp", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.AlterarModalidade("spuri", "", true, "u", "fpp", ""); err != nil {
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
	ca, err := s.CriarCredencial(context.Background(), cred(t, "ACA", ContextoAcademia), "u", "fpp")
	if err != nil {
		t.Fatal(err)
	}
	sp, err := s.CriarCredencial(context.Background(), cred(t, "", ContextoSpuri), "u", "fpp")
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
	sp, err := s.CriarCredencial(context.Background(), cred(t, "", ContextoSpuri), "u", "fpp")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AlterarStatusCredencial(sp.ID, StatusAtivo, "u", "fpp", "", "ativação operacional"); err != nil {
		t.Fatal(err)
	}
	rebuilt := NewServiceWithDBAndLedger(nil, nil, l)
	got, err := rebuilt.ObterCredencial(sp.ID, "fpp", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Historico) != 2 || got.Historico[1].Motivo != "ativação operacional" {
		t.Fatalf("histórico/motivo não preservado no replay: %#v", got.Historico)
	}
}

func TestEncryptDecryptSegredoFinanceiro(t *testing.T) {
	t.Setenv("FINANCE_ENCRYPTION_KEY", "chave-de-teste")
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

func TestOperacoesSensiveisExigemFPP(t *testing.T) {
	s := NewService(nil)
	if _, err := s.CriarCredencial(context.Background(), cred(t, "", ContextoSpuri), "u", "gerente"); err == nil {
		t.Fatal("gerente criou credencial financeira sensível")
	}
	c, err := s.CriarCredencial(context.Background(), cred(t, "", ContextoSpuri), "u", "fpp")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AlterarStatusCredencial(c.ID, StatusAtivo, "u", "adm", "", ""); err == nil {
		t.Fatal("adm ativou credencial financeira sensível")
	}
	if err := s.AlterarModalidade("spuri", "", false, "u", "gerente", ""); err == nil {
		t.Fatal("gerente alterou kill-switch financeiro")
	}
}

func TestSanitizeMapRemoveSegredosAninhados(t *testing.T) {
	got := sanitizeMap(map[string]any{
		"metadata": map[string]string{"token": "abc", "referencia": "ok"},
		"items":    []any{map[string]any{"api_key": "secret", "nome": "item"}},
	})
	metadata := got["metadata"].(map[string]string)
	if metadata["token"] != "" || metadata["token_redacted"] != "***" || metadata["referencia"] != "ok" {
		t.Fatalf("metadata não foi sanitizada: %#v", metadata)
	}
	item := got["items"].([]any)[0].(map[string]any)
	if item["api_key"] != nil || item["api_key_redacted"] != "***" || item["nome"] != "item" {
		t.Fatalf("slice aninhado não foi sanitizado: %#v", item)
	}
}

func TestValidateEncryptionConfigExigeChaveEmQualquerAmbiente(t *testing.T) {
	t.Setenv("ENV", "development")
	t.Setenv("FINANCE_ENCRYPTION_KEY", "")
	if err := ValidateEncryptionConfig(); err == nil {
		t.Fatalf("esperava erro sem FINANCE_ENCRYPTION_KEY")
	}

	t.Setenv("FINANCE_ENCRYPTION_KEY", "chave-de-teste")
	if err := ValidateEncryptionConfig(); err != nil {
		t.Fatalf("não esperava erro com FINANCE_ENCRYPTION_KEY configurada: %v", err)
	}
}
