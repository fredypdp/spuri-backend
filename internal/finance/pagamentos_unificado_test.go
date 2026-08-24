package finance

import (
	"testing"
)

// cobrancaListStub simula buscarCobrancas: um "banco" fixo de N cobranças
// (identificadas só pelo índice, via MerchantTransactionID), com
// LIMIT/OFFSET aplicados exatamente como uma consulta SQL real aplicaria —
// para testar a matemática de paginação de ListarPagamentosUnificado sem
// precisar de PostgreSQL. Também conta quantas vezes foi chamada, para
// confirmar que ListarPagamentosUnificado nunca busca mais do que o
// necessário.
func cobrancaListStub(totalReal int, chamadas *int) func(limit, offset int) (*CobrancaListResult, error) {
	return func(limit, offset int) (*CobrancaListResult, error) {
		*chamadas++
		out := []CobrancaResumo{}
		for i := offset; i < offset+limit && i < totalReal; i++ {
			out = append(out, CobrancaResumo{MerchantTransactionID: idxToMTID(i)})
		}
		return &CobrancaListResult{Cobrancas: out, Total: totalReal}, nil
	}
}

func idxToMTID(i int) string {
	digits := "0123456789"
	if i < 10 {
		return "COB-" + string(digits[i])
	}
	return "COB-" + string(digits[i/10]) + string(digits[i%10])
}

func pendenciasFake(n int) []MensalidadeMesView {
	out := make([]MensalidadeMesView, n)
	for i := range out {
		out[i] = MensalidadeMesView{CodigoEstudante: idxToMTID(i), CodigoAcademia: "ACA1", AnoLetivo: "2026_2027", Mes: (i % 12) + 1}
	}
	return out
}

// TestListarPagamentosUnificadoSemPendencias cobre o caso mais comum hoje
// (nenhum filtro de escopo em GET /financeiro/cobrancas): sem pendências,
// o resultado deve ser um passthrough exato das cobranças reais, com o
// limit/offset originais intactos — nenhuma mudança de comportamento em
// relação a antes desta unificação.
func TestListarPagamentosUnificadoSemPendencias(t *testing.T) {
	var chamadas int
	res, err := ListarPagamentosUnificado(nil, cobrancaListStub(120, &chamadas), 30, 60)
	if err != nil {
		t.Fatal(err)
	}
	if chamadas != 1 {
		t.Fatalf("esperava exatamente 1 chamada a buscarCobrancas, obteve %d", chamadas)
	}
	if len(res.Pagamentos) != 30 {
		t.Fatalf("esperava 30 itens, obteve %d", len(res.Pagamentos))
	}
	if res.Pagamentos[0].MerchantTransactionID != idxToMTID(60) {
		t.Fatalf("esperava a página começar em COB idx 60, começou em %s", res.Pagamentos[0].MerchantTransactionID)
	}
	if res.Total != 120 {
		t.Fatalf("esperava total=120, obteve %d", res.Total)
	}
	for _, p := range res.Pagamentos {
		if p.PendenciaSemCobranca {
			t.Fatalf("nenhum item deveria ter PendenciaSemCobranca=true: %#v", p)
		}
	}
}

// TestListarPagamentosUnificadoPaginaSoComPendencias cobre a página em que
// as pendências sozinhas já preenchem o limit inteiro — buscarCobrancas
// ainda precisa ser chamada (com limit=0) só para obter o total real de
// cobranças, mas sem trazer nenhuma linha extra.
func TestListarPagamentosUnificadoPaginaSoComPendencias(t *testing.T) {
	var chamadas int
	pendencias := pendenciasFake(50)
	res, err := ListarPagamentosUnificado(pendencias, cobrancaListStub(10, &chamadas), 30, 0)
	if err != nil {
		t.Fatal(err)
	}
	if chamadas != 1 {
		t.Fatalf("esperava 1 chamada a buscarCobrancas (só para o total), obteve %d", chamadas)
	}
	if len(res.Pagamentos) != 30 {
		t.Fatalf("esperava 30 itens (todos de pendências), obteve %d", len(res.Pagamentos))
	}
	for i, p := range res.Pagamentos {
		if !p.PendenciaSemCobranca {
			t.Fatalf("item %d deveria ser uma pendência sintética: %#v", i, p)
		}
	}
	if res.Total != 60 {
		t.Fatalf("esperava total=60 (50 pendências + 10 cobranças), obteve %d", res.Total)
	}
}

// TestListarPagamentosUnificadoPaginaMista cobre a página de transição:
// parte do limit preenchida por pendências, o resto por cobranças reais —
// o caso central que esta tarefa existe para resolver.
func TestListarPagamentosUnificadoPaginaMista(t *testing.T) {
	var chamadas int
	pendencias := pendenciasFake(25)
	res, err := ListarPagamentosUnificado(pendencias, cobrancaListStub(100, &chamadas), 30, 0)
	if err != nil {
		t.Fatal(err)
	}
	if chamadas != 1 {
		t.Fatalf("esperava 1 chamada a buscarCobrancas, obteve %d", chamadas)
	}
	if len(res.Pagamentos) != 30 {
		t.Fatalf("esperava 30 itens (25 pendências + 5 cobranças), obteve %d", len(res.Pagamentos))
	}
	for i := 0; i < 25; i++ {
		if !res.Pagamentos[i].PendenciaSemCobranca {
			t.Fatalf("item %d deveria ser pendência", i)
		}
	}
	for i := 25; i < 30; i++ {
		if res.Pagamentos[i].PendenciaSemCobranca {
			t.Fatalf("item %d deveria ser cobrança real", i)
		}
	}
	// As 5 cobranças reais na página mista devem ser exatamente as 5
	// primeiras (offset=0 do lado das cobranças, porque as pendências
	// não consomem nenhum offset delas).
	if res.Pagamentos[25].MerchantTransactionID != idxToMTID(0) {
		t.Fatalf("esperava a 1a cobranca real (idx 0), obteve %s", res.Pagamentos[25].MerchantTransactionID)
	}
	if res.Pagamentos[29].MerchantTransactionID != idxToMTID(4) {
		t.Fatalf("esperava a 5a cobranca real (idx 4), obteve %s", res.Pagamentos[29].MerchantTransactionID)
	}
	if res.Total != 125 {
		t.Fatalf("esperava total=125 (25+100), obteve %d", res.Total)
	}
}

// TestListarPagamentosUnificadoPaginaSoComCobrancasAposPendencias cobre a
// página seguinte à página mista: offset já passou de totalPendencias, e o
// offset repassado a buscarCobrancas precisa estar corretamente ajustado
// para continuar exatamente de onde a página anterior parou do lado das
// cobranças — sem pular nem repetir nenhuma cobrança real.
func TestListarPagamentosUnificadoPaginaSoComCobrancasAposPendencias(t *testing.T) {
	var chamadas int
	pendencias := pendenciasFake(25)
	// Página 2 (offset=30): a página 1 já consumiu as 25 pendências +
	// cobranças[0:5]; a página 2 deve continuar em cobranças[5:35].
	res, err := ListarPagamentosUnificado(pendencias, cobrancaListStub(100, &chamadas), 30, 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Pagamentos) != 30 {
		t.Fatalf("esperava 30 itens, obteve %d", len(res.Pagamentos))
	}
	for _, p := range res.Pagamentos {
		if p.PendenciaSemCobranca {
			t.Fatalf("nenhum item desta página deveria ser pendência: %#v", p)
		}
	}
	if res.Pagamentos[0].MerchantTransactionID != idxToMTID(5) {
		t.Fatalf("esperava continuar em cobranca idx 5 (sem pular nem repetir), obteve %s", res.Pagamentos[0].MerchantTransactionID)
	}
	if res.Pagamentos[29].MerchantTransactionID != idxToMTID(34) {
		t.Fatalf("esperava terminar em cobranca idx 34, obteve %s", res.Pagamentos[29].MerchantTransactionID)
	}
}

// TestListarPagamentosUnificadoLimiteExatoNoFinalDasPendencias cobre o
// caso de borda em que totalPendencias é um múltiplo exato de limit: a
// página do limite exato deve vir 100% de pendências (limiteCobrancas=0,
// sem buscar nenhuma cobrança extra), e a página seguinte deve começar do
// offset 0 das cobranças reais, sem pular nenhuma.
func TestListarPagamentosUnificadoLimiteExatoNoFinalDasPendencias(t *testing.T) {
	var chamadas int
	pendencias := pendenciasFake(30)

	pagina1, err := ListarPagamentosUnificado(pendencias, cobrancaListStub(50, &chamadas), 30, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(pagina1.Pagamentos) != 30 {
		t.Fatalf("pagina1: esperava 30 itens, obteve %d", len(pagina1.Pagamentos))
	}
	for _, p := range pagina1.Pagamentos {
		if !p.PendenciaSemCobranca {
			t.Fatalf("pagina1: todos os itens deveriam ser pendências: %#v", p)
		}
	}

	pagina2, err := ListarPagamentosUnificado(pendencias, cobrancaListStub(50, &chamadas), 30, 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(pagina2.Pagamentos) != 30 {
		t.Fatalf("pagina2: esperava 30 itens, obteve %d", len(pagina2.Pagamentos))
	}
	for _, p := range pagina2.Pagamentos {
		if p.PendenciaSemCobranca {
			t.Fatalf("pagina2: nenhum item deveria ser pendência: %#v", p)
		}
	}
	if pagina2.Pagamentos[0].MerchantTransactionID != idxToMTID(0) {
		t.Fatalf("pagina2: esperava comecar na cobranca idx 0, obteve %s", pagina2.Pagamentos[0].MerchantTransactionID)
	}
}

// TestListarPagamentosUnificadoUltimaPaginaParcial cobre a última página,
// menor que limit dos dois lados (pendências e cobranças esgotam antes de
// preencher a página inteira) — o resultado deve conter só os itens que
// realmente existem, sem preencher com itens vazios/zerados.
func TestListarPagamentosUnificadoUltimaPaginaParcial(t *testing.T) {
	var chamadas int
	pendencias := pendenciasFake(5)
	res, err := ListarPagamentosUnificado(pendencias, cobrancaListStub(3, &chamadas), 30, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Pagamentos) != 8 {
		t.Fatalf("esperava 8 itens (5 pendências + 3 cobranças, nada de preenchimento), obteve %d", len(res.Pagamentos))
	}
	if res.Total != 8 {
		t.Fatalf("esperava total=8, obteve %d", res.Total)
	}
}

// TestPendenciaParaPagamentoResumoIDDeterministico confirma que a mesma
// pendência sempre produz o mesmo id entre chamadas (importante para o
// frontend usar como key de lista sem piscar a cada nova requisição), e
// que pendências diferentes produzem ids diferentes.
func TestPendenciaParaPagamentoResumoIDDeterministico(t *testing.T) {
	m := MensalidadeMesView{CodigoEstudante: "EST001", CodigoAcademia: "ACA1", AnoLetivo: "2026_2027", Mes: 9, Valor: 15000}
	a := pendenciaParaPagamentoResumo(m)
	b := pendenciaParaPagamentoResumo(m)
	if a.ID != b.ID {
		t.Fatalf("a mesma pendência produziu ids diferentes entre chamadas: %s vs %s", a.ID, b.ID)
	}
	if !a.PendenciaSemCobranca {
		t.Fatal("esperava PendenciaSemCobranca=true")
	}
	if a.Status != EstadoPendente {
		t.Fatalf("esperava status=%q, obteve %q", EstadoPendente, a.Status)
	}
	if a.AtualizadoEm != nil {
		t.Fatalf("esperava AtualizadoEm nil para uma pendência sintética, obteve %v", a.AtualizadoEm)
	}
	if len(a.Mensalidades) != 1 || a.Mensalidades[0].AnoLetivo != "2026_2027" || a.Mensalidades[0].Mes != 9 {
		t.Fatalf("mensalidades não preservou ano_letivo/mes corretamente: %#v", a.Mensalidades)
	}

	outra := m
	outra.Mes = 10
	c := pendenciaParaPagamentoResumo(outra)
	if c.ID == a.ID {
		t.Fatal("pendências diferentes (mês diferente) produziram o mesmo id")
	}
}
