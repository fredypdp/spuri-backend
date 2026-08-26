package finance

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// capturingAppyPayTransport é um mock isolado (não compartilhado com
// appyPayMockTransport de appypay_integration_test.go) que grava o corpo
// de cada requisição POST /charges ou /qr-codes enviada à AppyPay, para
// permitir inspecionar exatamente o payload que gerarCobranca monta —
// não apenas se a chamada teve sucesso.
type capturingAppyPayTransport struct {
	mu        sync.Mutex
	lastBody  map[string]any
	lastPath  string
	qrCodeArr string
}

func (t *capturingAppyPayTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if strings.Contains(req.URL.Path, "/oauth2/token") {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"access_token":"test-token","expires_in":3600}`)), Request: req}, nil
	}
	if req.Method == http.MethodPost {
		raw, _ := io.ReadAll(req.Body)
		var parsed map[string]any
		_ = json.Unmarshal(raw, &parsed)
		t.mu.Lock()
		t.lastBody = parsed
		t.lastPath = req.URL.Path
		t.mu.Unlock()

		if strings.HasSuffix(req.URL.Path, "/qr-codes") {
			body := `{"id":"provider-qr-` + uuid.NewString() + `","status":"Pending","qrCodeArr":"` + t.qrCodeArr + `"}`
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
		}
		body := `{"id":"provider-charge-` + uuid.NewString() + `","status":"Pending"}`
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
	}
	// GET (consulta) não é usado por estes testes, mas devolve algo
	// coerente para não quebrar caso algum caminho inesperado o chame.
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"provider","status":"Pending"}`)), Request: req}, nil
}

func (t *capturingAppyPayTransport) paymentInfo() map[string]any {
	t.mu.Lock()
	defer t.mu.Unlock()
	pi, _ := t.lastBody["paymentInfo"].(map[string]any)
	return pi
}

// TestIntegrationGerarCobrancaMensalidadeGPOEnviaPhoneNumberNormalizado
// prova que, após a extração de gerarCobranca (internal/finance/cobranca_geracao.go),
// o fluxo de mensalidade continua enviando paymentInfo.phoneNumber já sem
// espaços à AppyPay quando o método é GPO — exatamente como antes da
// refatoração.
func TestIntegrationGerarCobrancaMensalidadeGPOEnviaPhoneNumberNormalizado(t *testing.T) {
	t.Setenv("APPYPAY_RESOURCE", "integration-resource")
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	estudante := "EST-GPO-" + uuid.NewString()[:8]
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2025_2026")
	seedMensalidadeTurma(t, client, academia, "T-GPO", "2025_2026", estudante, nil)
	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "6_ano_fundamental", nil, 1000, 7, time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC))
	if _, err := client.DB().Exec(`UPDATE financeiro_mensalidade_configuracoes SET metodos_pagamento='{GPO}' WHERE codigo_academia=$1`, academia); err != nil {
		t.Fatal(err)
	}
	configureIntegrationCredential(t, service, ContextoAcademia, academia)
	transport := &capturingAppyPayTransport{}
	service.SetHTTPClient(&http.Client{Transport: transport})

	pendentes, err := service.ListMensalidades(ctx, estudante, &academia)
	if err != nil {
		t.Fatal(err)
	}
	if len(pendentes) == 0 {
		t.Fatal("esperava pelo menos uma mensalidade pendente")
	}
	alvo := pendentes[0]

	_, err = service.IniciarPagamentoMensalidades(ctx, MensalidadePagamentoInput{
		CodigoEstudante: estudante, CodigoAcademia: academia,
		Meses:           []MensalidadeSelecaoMes{{AnoLetivo: alvo.AnoLetivo, Mes: alvo.Mes}},
		MetodoPagamento: "GPO", Telefone: "  923000000  ",
	}, estudante, "estudante", "127.0.0.1")
	if err != nil {
		t.Fatalf("IniciarPagamentoMensalidades falhou: %v", err)
	}
	if transport.lastPath == "" || !strings.HasSuffix(transport.lastPath, "/charges") {
		t.Fatalf("esperava POST .../charges, obteve path=%q", transport.lastPath)
	}
	pi := transport.paymentInfo()
	if pi == nil || pi["phoneNumber"] != "923000000" {
		t.Fatalf("esperava paymentInfo.phoneNumber=923000000 (sem espaços), obteve %#v", pi)
	}
}

// TestIntegrationGerarCobrancaMatriculaGPOEnviaPhoneNumberNormalizado é o
// equivalente do teste acima para o fluxo de matrícula — prova que os dois
// únicos chamadores de gerarCobranca continuam se comportando de forma
// idêntica para o mesmo método de pagamento.
func TestIntegrationGerarCobrancaMatriculaGPOEnviaPhoneNumberNormalizado(t *testing.T) {
	t.Setenv("APPYPAY_RESOURCE", "integration-resource")
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	codigo := seedMatriculaPendente(t, client, academia, 750)
	if _, err := client.DB().Exec(`UPDATE projection_solicitacoes_matricula SET metodos_pagamento_matricula='{GPO}' WHERE codigo_solicitacao=$1`, codigo); err != nil {
		t.Fatal(err)
	}
	configureIntegrationCredential(t, service, ContextoAcademia, academia)
	transport := &capturingAppyPayTransport{}
	service.SetHTTPClient(&http.Client{Transport: transport})

	_, err := service.IniciarPagamentoMatricula(ctx, MatriculaPagamentoInput{
		CodigoSolicitacao: codigo, MetodoPagamento: "GPO", Telefone: "  912345678  ",
	}, "127.0.0.1")
	if err != nil {
		t.Fatalf("IniciarPagamentoMatricula falhou: %v", err)
	}
	if transport.lastPath == "" || !strings.HasSuffix(transport.lastPath, "/charges") {
		t.Fatalf("esperava POST .../charges, obteve path=%q", transport.lastPath)
	}
	pi := transport.paymentInfo()
	if pi == nil || pi["phoneNumber"] != "912345678" {
		t.Fatalf("esperava paymentInfo.phoneNumber=912345678 (sem espaços), obteve %#v", pi)
	}
}

// TestIntegrationGerarCobrancaREFNaoEnviaPhoneNumber prova que o método REF
// (matrícula e mensalidade) nunca envia paymentInfo.phoneNumber — o campo
// só existe para GPO. Cobre os dois chamadores de gerarCobranca no mesmo
// teste para deixar explícito que é o mesmo comportamento nos dois fluxos.
func TestIntegrationGerarCobrancaREFNaoEnviaPhoneNumber(t *testing.T) {
	t.Setenv("APPYPAY_RESOURCE", "integration-resource")
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	t.Run("mensalidade", func(t *testing.T) {
		academia := mensalidadeCodigo()
		estudante := "EST-REF-" + uuid.NewString()[:8]
		seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2025_2026")
		seedMensalidadeTurma(t, client, academia, "T-REF", "2025_2026", estudante, nil)
		seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "6_ano_fundamental", nil, 1000, 7, time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC))
		if _, err := client.DB().Exec(`UPDATE financeiro_mensalidade_configuracoes SET metodos_pagamento='{REF}' WHERE codigo_academia=$1`, academia); err != nil {
			t.Fatal(err)
		}
		configureIntegrationCredential(t, service, ContextoAcademia, academia)
		transport := &capturingAppyPayTransport{}
		service.SetHTTPClient(&http.Client{Transport: transport})

		pendentes, err := service.ListMensalidades(ctx, estudante, &academia)
		if err != nil {
			t.Fatal(err)
		}
		if len(pendentes) == 0 {
			t.Fatal("esperava pelo menos uma mensalidade pendente")
		}
		alvo := pendentes[0]

		_, err = service.IniciarPagamentoMensalidades(ctx, MensalidadePagamentoInput{
			CodigoEstudante: estudante, CodigoAcademia: academia,
			Meses:           []MensalidadeSelecaoMes{{AnoLetivo: alvo.AnoLetivo, Mes: alvo.Mes}},
			MetodoPagamento: "REF",
		}, estudante, "estudante", "127.0.0.1")
		if err != nil {
			t.Fatalf("IniciarPagamentoMensalidades falhou: %v", err)
		}
		pi := transport.paymentInfo()
		if _, ok := pi["phoneNumber"]; ok {
			t.Fatalf("REF não deveria enviar phoneNumber, obteve paymentInfo=%#v", pi)
		}
	})

	t.Run("matricula", func(t *testing.T) {
		academia := mensalidadeCodigo()
		codigo := seedMatriculaPendente(t, client, academia, 750)
		configureIntegrationCredential(t, service, ContextoAcademia, academia)
		transport := &capturingAppyPayTransport{}
		service.SetHTTPClient(&http.Client{Transport: transport})

		_, err := service.IniciarPagamentoMatricula(ctx, MatriculaPagamentoInput{
			CodigoSolicitacao: codigo, MetodoPagamento: "REF",
		}, "127.0.0.1")
		if err != nil {
			t.Fatalf("IniciarPagamentoMatricula falhou: %v", err)
		}
		pi := transport.paymentInfo()
		if _, ok := pi["phoneNumber"]; ok {
			t.Fatalf("REF não deveria enviar phoneNumber, obteve paymentInfo=%#v", pi)
		}
	})
}
