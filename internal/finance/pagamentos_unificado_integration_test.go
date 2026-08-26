package finance

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestIntegrationFiltrarPendenciasComCobrancaRealVinculadaRemoveApenasOsVinculados
// cobre a razão de existir FiltrarPendenciasComCobrancaRealVinculada: desde
// a tarefa 63, PendenciasSemCobranca inclui corretamente qualquer mês
// ainda não pago, mesmo com uma tentativa de cobrança falhada — mas isso
// significa que, ao montar a lista unificada, esse mês apareceria duas
// vezes (uma como a cobrança real falhada, outra como pendência sintética
// redundante) se não for filtrado antes.
func TestIntegrationFiltrarPendenciasComCobrancaRealVinculadaRemoveApenasOsVinculados(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2026_2027")
	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "7_ano_fundamental", nil, 15000, 7, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	// ESTVINC1 tem uma cobrança falhada para setembro — continua contando
	// como pendente (tarefa 63), mas não deve gerar pendência sintética
	// duplicada para o mesmo mês.
	seedMensalidadeTurma(t, client, academia, "T-VINC-A", "2026_2027", "ESTVINC1", nil)
	// ESTVINC2 nunca teve nenhuma tentativa — continua aparecendo como
	// pendência sintética normalmente.
	seedMensalidadeTurma(t, client, academia, "T-VINC-B", "2026_2027", "ESTVINC2", nil)

	chargeID := uuid.New()
	payload, err := json.Marshal(map[string]any{
		"status": "falhada", "amount": 15000, "currency": "AOA", "description": "Propinas: 1 mensalidade(s)",
		"payment_method": "GPO_QR", "codigo_estudante": "ESTVINC1",
		"mensalidades": []MensalidadeSelecaoMes{{AnoLetivo: "2026_2027", Mes: 9}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.DB().Exec(`INSERT INTO financeiro_cobrancas (id,merchant_transaction_id,contexto_tipo,codigo_academia,payload) VALUES ($1,$2,'academia',$3,$4)`,
		chargeID, integrationMerchant("VINC"), academia, payload); err != nil {
		t.Fatal(err)
	}
	if _, err := client.DB().Exec(`INSERT INTO financeiro_mensalidade_cobrancas (charge_id,codigo_estudante,codigo_academia,ano_letivo,mes) VALUES ($1,'ESTVINC1',$2,'2026_2027',9)`,
		chargeID, academia); err != nil {
		t.Fatal(err)
	}

	mesSetembro := 9
	pendencias, err := service.PendenciasSemCobranca(ctx, academia, nil, nil, "", "2026_2027", &mesSetembro)
	if err != nil {
		t.Fatal(err)
	}
	achouEst1AntesDoFiltro := false
	for _, p := range pendencias {
		if p.CodigoEstudante == "ESTVINC1" {
			achouEst1AntesDoFiltro = true
		}
	}
	if !achouEst1AntesDoFiltro {
		t.Fatal("pré-condição do teste: ESTVINC1/setembro deveria aparecer em PendenciasSemCobranca (tarefa 63), sem isso o teste não prova nada")
	}

	filtradas, err := service.FiltrarPendenciasComCobrancaRealVinculada(ctx, pendencias)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range filtradas {
		if p.CodigoEstudante == "ESTVINC1" && p.Mes == 9 {
			t.Fatalf("ESTVINC1/setembro já tem cobrança real vinculada (falhada); não deveria sobrar em pendências após o filtro: %#v", p)
		}
	}
	achouEst2 := false
	for _, p := range filtradas {
		if p.CodigoEstudante == "ESTVINC2" && p.Mes == 9 {
			achouEst2 = true
		}
	}
	if !achouEst2 {
		t.Fatal("ESTVINC2/setembro nunca teve nenhuma cobrança; deveria continuar em pendências após o filtro")
	}
}

// TestIntegrationListarCobrancasHandlerFluxoUnificado reproduz, com
// PostgreSQL real, exatamente o que ListarCobrancasAppyPay faz: resolve
// pendências, filtra as vinculadas, e unifica com as cobranças reais numa
// única lista paginada — confirmando que as peças (PendenciasSemCobranca,
// FiltrarPendenciasComCobrancaRealVinculada, ListCobrancas,
// ListarPagamentosUnificado) se encaixam corretamente contra dados reais,
// não só contra os stubs de pagamentos_unificado_test.go.
func TestIntegrationListarCobrancasHandlerFluxoUnificado(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2026_2027")
	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "7_ano_fundamental", nil, 15000, 7, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	// 3 estudantes sem NENHUMA tentativa (viram pendência sintética).
	for i, cod := range []string{"ESTFLX01", "ESTFLX02", "ESTFLX03"} {
		seedMensalidadeTurma(t, client, academia, "T-FLX-"+string(rune('A'+i)), "2026_2027", cod, nil)
	}
	// 1 estudante com cobrança PAGA para setembro — não deve aparecer em
	// nenhuma das duas fontes (nem pendência, nem em "falhada").
	seedMensalidadeTurma(t, client, academia, "T-FLX-PG", "2026_2027", "ESTFLXPG", nil)
	if _, err := client.DB().Exec(`INSERT INTO financeiro_mensalidade_obrigacoes_eventos (event_id,aggregate_id,codigo_estudante,codigo_academia,ano_letivo,mes,tipo,ocorrido_em) VALUES ($1,$2,'ESTFLXPG',$3,'2026_2027',9,'paga',CURRENT_TIMESTAMP)`,
		uuid.New(), uuid.New(), academia); err != nil {
		t.Fatal(err)
	}
	// 1 estudante com cobrança FALHADA para setembro — deve aparecer como
	// cobrança real (status falhada), NÃO como pendência sintética.
	seedMensalidadeTurma(t, client, academia, "T-FLX-FL", "2026_2027", "ESTFLXFL", nil)
	chargeID := uuid.New()
	payload, err := json.Marshal(map[string]any{
		"status": "falhada", "amount": 15000, "currency": "AOA", "description": "Propinas: 1 mensalidade(s)",
		"payment_method": "GPO_QR", "codigo_estudante": "ESTFLXFL",
		"mensalidades": []MensalidadeSelecaoMes{{AnoLetivo: "2026_2027", Mes: 9}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.DB().Exec(`INSERT INTO financeiro_cobrancas (id,merchant_transaction_id,contexto_tipo,codigo_academia,payload) VALUES ($1,$2,'academia',$3,$4)`,
		chargeID, integrationMerchant("FLX"), academia, payload); err != nil {
		t.Fatal(err)
	}
	if _, err := client.DB().Exec(`INSERT INTO financeiro_mensalidade_cobrancas (charge_id,codigo_estudante,codigo_academia,ano_letivo,mes) VALUES ($1,'ESTFLXFL',$2,'2026_2027',9)`,
		chargeID, academia); err != nil {
		t.Fatal(err)
	}

	mesSetembro := 9
	pendencias, err := service.PendenciasSemCobranca(ctx, academia, nil, nil, "", "2026_2027", &mesSetembro)
	if err != nil {
		t.Fatal(err)
	}
	pendencias, err = service.FiltrarPendenciasComCobrancaRealVinculada(ctx, pendencias)
	if err != nil {
		t.Fatal(err)
	}

	buscarCobrancas := func(limit, offset int) (*CobrancaListResult, error) {
		return service.ListCobrancas(ctx, ContextoAcademia, academia, nil, nil, nil, nil, "", "2026_2027", &mesSetembro, limit, offset)
	}

	res, err := ListarPagamentosUnificado(pendencias, buscarCobrancas, 30, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Total esperado para setembro: 3 pendências sintéticas (FLX01-03) +
	// 1 cobrança real falhada (FLXFL) = 4. ESTFLXPG (pago) não entra em
	// nenhuma das duas fontes.
	if res.Total != 4 {
		t.Fatalf("esperava total=4 (3 pendências + 1 cobrança falhada), obteve %d: %#v", res.Total, res.Pagamentos)
	}
	if len(res.Pagamentos) != 4 {
		t.Fatalf("esperava 4 itens na página, obteve %d", len(res.Pagamentos))
	}

	var pendentesSinteticas, cobrancasReais int
	estudantesVistos := map[string]bool{}
	for _, p := range res.Pagamentos {
		estudantesVistos[p.CodigoEstudante] = true
		if p.Status == EstadoPendente {
			pendentesSinteticas++
			if p.AtualizadoEm != nil {
				t.Fatalf("pendência sintética deveria ter AtualizadoEm nil, obteve %v", p.AtualizadoEm)
			}
		} else {
			cobrancasReais++
			if p.CodigoEstudante != "ESTFLXFL" {
				t.Fatalf("única cobrança real esperada era de ESTFLXFL, veio de %s", p.CodigoEstudante)
			}
			if p.Status != "falhada" {
				t.Fatalf("esperava status=falhada na cobrança real, obteve %q", p.Status)
			}
			if p.AtualizadoEm == nil {
				t.Fatal("cobrança real deveria ter AtualizadoEm preenchido")
			}
		}
	}
	if pendentesSinteticas != 3 {
		t.Fatalf("esperava 3 pendências sintéticas, obteve %d", pendentesSinteticas)
	}
	if cobrancasReais != 1 {
		t.Fatalf("esperava 1 cobrança real, obteve %d", cobrancasReais)
	}
	if estudantesVistos["ESTFLXPG"] {
		t.Fatal("ESTFLXPG (pago) não deveria aparecer em nenhuma das duas fontes")
	}
	if estudantesVistos["ESTVINC1"] {
		t.Fatal("estudante inesperado na lista")
	}

	// Ordem: pendências primeiro, cobranças reais depois.
	for i := 0; i < 3; i++ {
		if res.Pagamentos[i].Status != EstadoPendente {
			t.Fatalf("item %d deveria ser pendência (pendências vêm primeiro)", i)
		}
	}
	if res.Pagamentos[3].Status == EstadoPendente {
		t.Fatal("item 3 deveria ser a cobrança real (por último)")
	}
}
