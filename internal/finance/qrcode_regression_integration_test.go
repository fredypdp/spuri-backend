package finance

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestIntegrationPagamentoMensalidadeGPOQRDevolveQRCodeArr reproduz o
// Problema 2 documentado em
// "docs/Lista de Tarefas/Problemas de Backend - Modulo de Pagamentos.md" de
// ponta a ponta: antes da correção, esta chamada devolvia
// view.Charge.QRCodeArr == "" mesmo com a AppyPay (simulada aqui pelo mock
// transport) devolvendo qrCodeArr normalmente.
func TestIntegrationPagamentoMensalidadeGPOQRDevolveQRCodeArr(t *testing.T) {
	t.Setenv("APPYPAY_RESOURCE", "integration-resource")
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	estudante := "EST-QR-" + uuid.NewString()[:8]
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2025_2026")
	seedMensalidadeTurma(t, client, academia, "T-QR", "2025_2026", estudante, nil)
	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "6_ano_fundamental", nil, 1000, 7, time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC))
	if _, err := client.DB().Exec(`UPDATE financeiro_mensalidade_configuracoes SET metodos_pagamento='{GPO_QR}' WHERE codigo_academia=$1`, academia); err != nil {
		t.Fatal(err)
	}

	configureIntegrationCredential(t, service, ContextoAcademia, academia)
	service.SetHTTPClient(&http.Client{Transport: &appyPayMockTransport{status: "Pending"}})

	pendentes, err := service.ListMensalidades(ctx, estudante, &academia)
	if err != nil {
		t.Fatal(err)
	}
	if len(pendentes) == 0 {
		t.Fatal("esperava pelo menos uma mensalidade pendente")
	}
	alvo := pendentes[0]

	view, err := service.IniciarPagamentoMensalidades(ctx, MensalidadePagamentoInput{
		CodigoEstudante: estudante, CodigoAcademia: academia,
		Meses:           []MensalidadeSelecaoMes{{AnoLetivo: alvo.AnoLetivo, Mes: alvo.Mes}},
		MetodoPagamento: "GPO_QR",
	}, estudante, "estudante", "127.0.0.1")
	if err != nil {
		t.Fatalf("IniciarPagamentoMensalidades falhou: %v", err)
	}
	if view.Charge.QRCodeArr == "" {
		t.Fatalf("qrCodeArr não chegou na resposta de pagamento de mensalidade GPO_QR: %#v", view.Charge)
	}
}

// TestIntegrationPagamentoMatriculaGPOQRDevolveQRCodeArr é o equivalente de
// TestIntegrationPagamentoMensalidadeGPOQRDevolveQRCodeArr para o fluxo de
// matrícula.
func TestIntegrationPagamentoMatriculaGPOQRDevolveQRCodeArr(t *testing.T) {
	t.Setenv("APPYPAY_RESOURCE", "integration-resource")
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	codigo := seedMatriculaPendente(t, client, academia, 750)
	if _, err := client.DB().Exec(`UPDATE projection_solicitacoes_matricula SET metodos_pagamento_matricula='{GPO_QR}' WHERE codigo_solicitacao=$1`, codigo); err != nil {
		t.Fatal(err)
	}

	configureIntegrationCredential(t, service, ContextoAcademia, academia)
	service.SetHTTPClient(&http.Client{Transport: &appyPayMockTransport{status: "Pending"}})

	view, err := service.IniciarPagamentoMatricula(ctx, MatriculaPagamentoInput{CodigoSolicitacao: codigo, MetodoPagamento: "GPO_QR"}, "127.0.0.1")
	if err != nil {
		t.Fatalf("IniciarPagamentoMatricula falhou: %v", err)
	}
	if view.Charge.QRCodeArr == "" {
		t.Fatalf("qrCodeArr não chegou na resposta de pagamento de matrícula GPO_QR: %#v", view.Charge)
	}
}
