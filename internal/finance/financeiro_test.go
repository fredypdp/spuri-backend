package finance

import (
	"context"
	"strings"
	"testing"
)

func cred(cod string, ctx ContextoTipo) CredencialInput {
	return CredencialInput{ContextoTipo: ctx, CodigoAcademia: cod, Ambiente: AmbienteTeste, AuthBaseURL: "https://login", APIBaseURL: "https://api", ClientID: "client", ClientSecret: "super-secret", Resource: "res", WebhookSecret: "hook-secret", Applications: []ApplicationInput{{PaymentMethod: "REF", ApplicationID: "app", APIKey: "api-secret-key"}}}
}
func TestCredenciaisCriptografadasEMascaradas(t *testing.T) {
	s := NewService(nil)
	c, err := s.CriarCredencial(context.Background(), cred("", ContextoSpuri), "u", "admin")
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
func TestIsolamentoAcademiasEIdempotencia(t *testing.T) {
	s := NewService(nil)
	ca, _ := s.CriarCredencial(context.Background(), cred("ACA", ContextoAcademia), "u", "admin")
	cb, _ := s.CriarCredencial(context.Background(), cred("ACB", ContextoAcademia), "u", "admin")
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
	ac, _ := s.CriarCredencial(context.Background(), cred("ACA", ContextoAcademia), "u", "admin")
	sp, _ := s.CriarCredencial(context.Background(), cred("", ContextoSpuri), "u", "admin")
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
func TestWebhookDupENaoLiquidaSemConsulta(t *testing.T) {
	s := NewService(nil)
	sp, _ := s.CriarCredencial(context.Background(), cred("", ContextoSpuri), "u", "admin")
	s.AlterarStatusCredencial(sp.ID, StatusAtivo, "u", "admin", "", "")
	ch, _ := s.GerarCobrancaFinanceiraBase(context.Background(), GerarCobrancaInput{ContextoTipo: ContextoSpuri, CodigoAcademia: "ACA", PagadorTipo: "academia", PagadorID: "ACA", Valor: 1, MetodoPagamento: "REF", ReferenciaExterna: "w"}, "u")
	ok, err := s.ProcessarWebhookFinanceiroBase(context.Background(), "evt1", ch.ID)
	if err != nil || !ok {
		t.Fatalf("webhook err=%v ok=%v", err, ok)
	}
	ok, err = s.ProcessarWebhookFinanceiroBase(context.Background(), "evt1", ch.ID)
	if err != nil || ok {
		t.Fatalf("duplicado não ignorado")
	}
	got, _ := s.ConsultarCobrancaFinanceiraBase(ch.ID)
	if got.Status != CobrancaLiquidada {
		t.Fatalf("status=%s", got.Status)
	}
}

func TestCredenciaisSpuriUsamVariaveisAmbienteAppyPay(t *testing.T) {
	t.Setenv("APPYPAY_AUTH_BASE_URL", "https://login.env")
	t.Setenv("APPYPAY_API_BASE_URL", "https://api.env")
	t.Setenv("APPYPAY_WEBAPI_BASE_URL", "https://web.env")
	t.Setenv("APPYPAY_SPURI_CLIENT_ID", "client-env")
	t.Setenv("APPYPAY_SPURI_CLIENT_SECRET", "secret-env")
	t.Setenv("APPYPAY_SPURI_RESOURCE", "resource-env")
	t.Setenv("APPYPAY_SPURI_WEBHOOK_SECRET", "hook-env")
	t.Setenv("APPYPAY_SPURI_APPLICATIONS_JSON", `[{"paymentMethod":"REF","applicationId":"app-env","apiKey":"api-env"}]`)
	s := NewService(nil)
	c, err := s.CriarCredencial(context.Background(), CredencialInput{ContextoTipo: ContextoSpuri, Ambiente: AmbienteTeste}, "u", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if c.APIBaseURL != "https://api.env" || c.AuthBaseURL != "https://login.env" || c.ClientID != "client-env" || c.Resource != "resource-env" {
		t.Fatalf("credencial não herdou ambiente AppyPay: %+v", c)
	}
	if c.ClientSecretMask != "****-env" || c.Applications[0].APIKeyMask != "****-env" {
		t.Fatalf("segredos Spuri não foram mascarados: %+v", c)
	}
}
