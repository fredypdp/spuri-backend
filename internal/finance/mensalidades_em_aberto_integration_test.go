package finance

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// TestIntegrationMensalidadesEmAbertoAcademiaVeDividaMesmoAposCobrancaFalhada
// reproduz o relato de Fredy: uma cobrança de mensalidade falha (a AppyPay
// responde e recusa — código 246/"erro_interno_provedor", cenário real
// coberto pela tarefa 70), e do lado da academia (GET /financeiro/cobrancas
// contexto_tipo=academia) a linha aparecia só com status "Failed", sem
// nenhum sinal de que o estudante ainda devia aquele mês — mesmo a
// pendência sintética correspondente tendo sido removida de propósito pela
// deduplicação (FiltrarPendenciasComCobrancaRealVinculada, tarefa 64).
//
// Cobre três momentos:
//  1. Logo após a falha: MensalidadesEmAberto deve conter o mês (ainda
//     pendente).
//  2. Depois de uma nova tentativa bem-sucedida para o MESMO mês: a cobrança
//     ANTIGA (falhada) consultada de novo deve deixar de mostrar o mês em
//     MensalidadesEmAberto — prova que o campo reflete o estado atual da
//     obrigação, não um "carimbo" fixo da hora em que a cobrança falhou.
func TestIntegrationMensalidadesEmAbertoAcademiaVeDividaMesmoAposCobrancaFalhada(t *testing.T) {
	t.Setenv("APPYPAY_RESOURCE", "integration-resource")
	t.Setenv("FINANCE_ENCRYPTION_KEY", "test-only-secret-material-at-least-32")
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	estudante := "SJS-" + mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2025_2026")
	seedMensalidadeTurma(t, client, academia, "T-EMABERTO", "2025_2026", estudante, nil)
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

	// 1a tentativa: cria a cobrança (POST /charges do mock sempre devolve
	// "Pending" — vira aguardando_pagamento). Depois a AppyPay resolve como
	// Failed (saldo insuficiente) e o Spuri descobre isso numa consulta —
	// mesmo padrão de TestIntegrationMensalidadeComCobrancaFalhadaNaAppyPayPermiteNovaTentativa.
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
	acha := func(res *CobrancaListResult) *CobrancaResumo {
		for i := range res.Cobrancas {
			c := &res.Cobrancas[i]
			if c.CodigoEstudante == estudante && c.Origem == "mensalidade" {
				return c
			}
		}
		return nil
	}

	resAcademia, err := service.ListCobrancas(ctx, ContextoAcademia, academia, nil, nil, nil, nil, "", alvo.AnoLetivo, &mesAlvo, 30, 0)
	if err != nil {
		t.Fatal(err)
	}
	cobranca := acha(resAcademia)
	if cobranca == nil {
		t.Fatal("esperava encontrar a cobrança falhada do lado da academia")
	}
	if cobranca.Status != "Failed" {
		t.Fatalf("Status = %q, queria Failed", cobranca.Status)
	}
	if len(cobranca.MensalidadesEmAberto) != 1 || cobranca.MensalidadesEmAberto[0].Mes != mesAlvo {
		t.Fatalf("BUG: cobrança Failed não sinalizou o mês %d como em aberto — MensalidadesEmAberto=%+v (a academia veria 'Failed' sem saber que o estudante ainda deve)", mesAlvo, cobranca.MensalidadesEmAberto)
	}
	t.Logf("OK: cobrança Failed sinaliza corretamente MensalidadesEmAberto=%+v", cobranca.MensalidadesEmAberto)

	// Do lado do estudante, a mesma cobrança deve mostrar exatamente a
	// mesma coisa (simetria entre os dois papéis).
	resEstudante, err := service.ListCobrancasEstudante(ctx, estudante, &academia, nil, nil, nil, nil, "", alvo.AnoLetivo, nil, 30, 0)
	if err != nil {
		t.Fatal(err)
	}
	cobrancaEst := acha(resEstudante)
	if cobrancaEst == nil || len(cobrancaEst.MensalidadesEmAberto) != 1 {
		t.Fatalf("BUG: lado do estudante não sinalizou o mesmo mês em aberto: %+v", cobrancaEst)
	}

	// 2a tentativa, agora bem-sucedida, para o MESMO mês (mesmo padrão:
	// cria como Pending, depois o Spuri descobre Success numa consulta).
	transport.status, transport.code, transport.message = "Pending", 0, ""
	segunda, err := service.IniciarPagamentoMensalidades(ctx, MensalidadePagamentoInput{
		CodigoEstudante: estudante, CodigoAcademia: academia, Meses: meses,
		MetodoPagamento: "GPO", Telefone: "923000000",
	}, estudante, "estudante", "127.0.0.1")
	if err != nil {
		t.Fatalf("2a tentativa deveria ser aceite (a 1a já está Failed, não bloqueia): %v", err)
	}
	transport.status = "Success"
	consultadaSegunda, err := service.ConsultCharge(ctx, ContextoAcademia, academia, segunda.Charge.ID.String(), estudante, "estudante", "127.0.0.1")
	if err != nil {
		t.Fatalf("ConsultCharge da 2a tentativa falhou: %v", err)
	}
	if consultadaSegunda.Status != "Success" {
		t.Fatalf("2a tentativa Status = %q, queria Success", consultadaSegunda.Status)
	}

	// Reconsultando a cobrança ANTIGA (falhada): o mês já foi pago por
	// outra cobrança, então MensalidadesEmAberto deve esvaziar — o campo
	// reflete o estado ATUAL da obrigação, não um carimbo da hora da falha.
	resAcademiaDepois, err := service.ListCobrancas(ctx, ContextoAcademia, academia, []string{"Failed"}, nil, nil, nil, "", alvo.AnoLetivo, &mesAlvo, 30, 0)
	if err != nil {
		t.Fatal(err)
	}
	var antiga *CobrancaResumo
	for i := range resAcademiaDepois.Cobrancas {
		if resAcademiaDepois.Cobrancas[i].ID == cobranca.ID {
			antiga = &resAcademiaDepois.Cobrancas[i]
		}
	}
	if antiga == nil {
		t.Fatal("esperava reencontrar a cobrança falhada antiga filtrando por estado=Failed")
	}
	if len(antiga.MensalidadesEmAberto) != 0 {
		t.Fatalf("BUG: cobrança falhada antiga ainda mostra o mês em aberto depois de pago por outra cobrança: %+v", antiga.MensalidadesEmAberto)
	}
	t.Logf("OK: após pagamento bem-sucedido em nova tentativa, a cobrança antiga deixa de sinalizar o mês como em aberto")
}
