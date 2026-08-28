package finance

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// TestIntegrationListaUnificadaMostraPendenciaEFalhaJuntasSemFiltros reproduz
// o relato de Fredy após a tarefa 72: consultando GET /financeiro/cobrancas
// sem filtro de estado, para um estudante com uma tentativa de mensalidade
// falhada, o esperado é que a lista unificada devolva OS DOIS itens lado a
// lado — a pendência sintética daquele mês (ainda pagável, com Estado ==
// EstadoPendente) e a própria cobrança falhada (histórico da tentativa) —
// em vez de a pendência desaparecer só porque existe uma cobrança real
// vinculada a ela.
//
// Passa pelo fluxo real: cria a cobrança via IniciarPagamentoMensalidades
// (mock da AppyPay), deixa-a falhar de verdade via ConsultCharge, e só
// depois monta a lista unificada exatamente como ListarCobrancasAppyPay
// (handler HTTP) faz: PendenciasSemCobranca -> FiltrarPendenciasComCobrancaRealVinculada
// -> ListarPagamentosUnificado.
func TestIntegrationListaUnificadaMostraPendenciaEFalhaJuntasSemFiltros(t *testing.T) {
	t.Setenv("APPYPAY_RESOURCE", "integration-resource")
	t.Setenv("FINANCE_ENCRYPTION_KEY", "test-only-secret-material-at-least-32")
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	estudante := "SJS-" + mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2025_2026")
	seedMensalidadeTurma(t, client, academia, "T-UNIF73", "2025_2026", estudante, nil)
	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "6_ano_fundamental", nil, 10000, 7, time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC))
	if _, err := client.DB().Exec(`UPDATE financeiro_mensalidade_configuracoes SET metodos_pagamento='{GPO}' WHERE codigo_academia=$1`, academia); err != nil {
		t.Fatal(err)
	}
	configureIntegrationCredential(t, service, ContextoAcademia, academia)

	pendentes, err := service.ListMensalidades(ctx, estudante, &academia)
	if err != nil {
		t.Fatal(err)
	}
	if len(pendentes) == 0 {
		t.Fatal("esperava mensalidade pendente")
	}
	alvo := pendentes[0]
	meses := []MensalidadeSelecaoMes{{AnoLetivo: alvo.AnoLetivo, Mes: alvo.Mes}}

	transport := &appyPayMockTransport{status: "Pending"}
	service.SetHTTPClient(&http.Client{Transport: transport})
	primeira, err := service.IniciarPagamentoMensalidades(ctx, MensalidadePagamentoInput{
		CodigoEstudante: estudante, CodigoAcademia: academia, Meses: meses,
		MetodoPagamento: "GPO", Telefone: "923000000",
	}, estudante, "estudante", "127.0.0.1")
	if err != nil {
		t.Fatalf("1a tentativa deveria ser aceite: %v", err)
	}
	transport.status, transport.code, transport.message = "Failed", 246, "Internal provider error"
	if _, err := service.ConsultCharge(ctx, ContextoAcademia, academia, primeira.Charge.ID.String(), estudante, "estudante", "127.0.0.1"); err != nil {
		t.Fatalf("ConsultCharge falhou: %v", err)
	}

	mesAlvo := alvo.Mes

	// Exatamente o pipeline de ListarCobrancasAppyPay, sem filtro de
	// estado nem de origem (== "consultado tudo sem filtros").
	pendencias, err := service.PendenciasSemCobranca(ctx, academia, nil, nil, "", alvo.AnoLetivo, &mesAlvo)
	if err != nil {
		t.Fatal(err)
	}
	pendencias, err = service.FiltrarPendenciasComCobrancaRealVinculada(ctx, pendencias)
	if err != nil {
		t.Fatal(err)
	}
	res, err := ListarPagamentosUnificado(pendencias, func(l, o int) (*CobrancaListResult, error) {
		return service.ListCobrancas(ctx, ContextoAcademia, academia, nil, nil, nil, nil, "", alvo.AnoLetivo, &mesAlvo, 30, 0)
	}, 30, 0)
	if err != nil {
		t.Fatal(err)
	}

	var temPendente, temFalha bool
	for _, p := range res.Pagamentos {
		if p.CodigoEstudante != estudante {
			continue
		}
		if p.Status == EstadoPendente {
			temPendente = true
		}
		if p.Status == "Failed" {
			temFalha = true
		}
	}
	if !temPendente {
		t.Fatalf("BUG: a pendência sintética de %s/%d não apareceu na lista unificada sem filtros — a academia não veria a opção de o estudante ainda pagar: %#v", alvo.AnoLetivo, mesAlvo, res.Pagamentos)
	}
	if !temFalha {
		t.Fatalf("BUG: a cobrança falhada de %s/%d não apareceu na lista unificada sem filtros: %#v", alvo.AnoLetivo, mesAlvo, res.Pagamentos)
	}
	t.Logf("OK: pendência e cobrança falhada aparecem juntas na lista sem filtros, %d itens no total para o mês", res.Total)
}
