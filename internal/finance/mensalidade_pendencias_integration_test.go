package finance

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"spuri/internal/db"
)

// seedFinanceiroMensalidadeCobranca insere diretamente a linha de vínculo
// cobrança<->mês que, em produção, é escrita por
// upsertMensalidadeCobrancas (internal/projections/financeiro_projection.go)
// a cada evento de cobrança de mensalidade. Os testes de integração deste
// pacote não passam pelo pipeline de eventos/projeção completo, então
// simulamos aqui só a linha que PendenciasSemCobranca e
// chargeIDsEscopoMensalidade efetivamente leem.
func seedFinanceiroMensalidadeCobranca(t *testing.T, client *db.Client, chargeID uuid.UUID, estudante, academia, anoLetivo string, mes int) {
	t.Helper()
	if _, err := client.DB().Exec(`INSERT INTO financeiro_mensalidade_cobrancas (charge_id,codigo_estudante,codigo_academia,ano_letivo,mes) VALUES ($1,$2,$3,$4,$5)`,
		chargeID, estudante, academia, anoLetivo, mes); err != nil {
		t.Fatal(err)
	}
}

// seedFinanceiroCobrancaMensalidade insere uma cobrança de mensalidade
// (financeiro_cobrancas) e o vínculo correspondente em
// financeiro_mensalidade_cobrancas, simulando uma tentativa de cobrança já
// registrada para o mês informado. Usada pelos testes de ListCobrancas
// (que filtram a listagem normal de cobranças por escopo/mês — inalterado
// por esta tarefa) e, nos testes de PendenciasSemCobranca, para comprovar
// que uma tentativa (mesmo com status "falhada") sozinha NÃO tira mais um
// mês de pendências_sem_cobranca — ver
// TestIntegrationPendenciasSemCobrancaIncluiMesesComTentativaFalhadaOuSemNenhuma.
func seedFinanceiroCobrancaMensalidade(t *testing.T, client *db.Client, academia, estudante, status, anoLetivo string, mes int, valor float64) uuid.UUID {
	t.Helper()
	id := uuid.New()
	payload, err := json.Marshal(map[string]any{
		"status": status, "amount": valor, "currency": "AOA", "description": "mensalidade",
		"payment_method": "REF", "codigo_estudante": estudante,
		"mensalidades": []MensalidadeSelecaoMes{{AnoLetivo: anoLetivo, Mes: mes}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.DB().Exec(`INSERT INTO financeiro_cobrancas (id,merchant_transaction_id,contexto_tipo,codigo_academia,payload) VALUES ($1,$2,'academia',$3,$4)`,
		id, integrationMerchant("PND"), academia, payload); err != nil {
		t.Fatal(err)
	}
	seedFinanceiroMensalidadeCobranca(t, client, id, estudante, academia, anoLetivo, mes)
	return id
}

// TestIntegrationPendenciasSemCobrancaIncluiMesesComTentativaFalhadaOuSemNenhuma
// cobre duas coisas juntas porque são a mesma regra de negócio vista de dois
// ângulos: um estudante que deve uma mensalidade continua aparecendo em
// pendências_sem_cobranca enquanto ela não for efetivamente PAGA (nem
// anulada) — não importa se ele nunca tentou nenhuma cobrança, ou se já
// tentou e a tentativa FALHOU.
//
// ESTPN01 nunca tentou nenhuma cobrança. ESTPN02 já tem uma cobrança
// FALHADA para setembro. Até 2026-08-23 este era o caso que
// PendenciasSemCobranca excluía (por engano — ver o comentário histórico em
// PendenciasSemCobranca): setembro do ESTPN02 desaparecia de toda visão
// agregada da academia mesmo continuando por pagar. Decisão de produto
// (Fredy, 2026-08-23): os dois devem aparecer igualmente — só uma cobrança
// bem-sucedida (ou uma anulação) tira um mês de pendências_sem_cobranca.
func TestIntegrationPendenciasSemCobrancaIncluiMesesComTentativaFalhadaOuSemNenhuma(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2026_2027")
	seedMensalidadeTurma(t, client, academia, "T-PND-A", "2026_2027", "ESTPN01", nil)
	seedMensalidadeTurma(t, client, academia, "T-PND-B", "2026_2027", "ESTPN02", nil)
	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "7_ano_fundamental", nil, 15000, 7, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	// ESTPN02 já tem uma tentativa de cobrança FALHADA para setembro — mas
	// ainda deve aparecer em pendências_sem_cobranca, porque continua sem
	// pagar.
	seedFinanceiroCobrancaMensalidade(t, client, academia, "ESTPN02", "falhada", "2026_2027", 9, 15000)

	res, err := service.PendenciasSemCobranca(ctx, academia, nil, nil, "", "2026_2027", nil)
	if err != nil {
		t.Fatal(err)
	}

	achouEst1Setembro, achouEst2Setembro := false, false
	for _, m := range res {
		if m.CodigoEstudante == "ESTPN01" && m.Mes == 9 {
			achouEst1Setembro = true
			if m.Estado != EstadoPendente {
				t.Fatalf("ESTPN01/setembro: esperava estado pendente, obteve %q", m.Estado)
			}
		}
		if m.CodigoEstudante == "ESTPN02" && m.Mes == 9 {
			achouEst2Setembro = true
			if m.Estado != EstadoPendente {
				t.Fatalf("ESTPN02/setembro: esperava estado pendente, obteve %q", m.Estado)
			}
		}
	}
	if !achouEst1Setembro {
		t.Fatalf("ESTPN01/setembro nunca teve nenhuma cobrança; deveria aparecer em pendências_sem_cobranca. resultado: %#v", res)
	}
	if !achouEst2Setembro {
		t.Fatalf("ESTPN02/setembro já tentou (falhou) mas continua sem pagar; deveria aparecer em pendências_sem_cobranca mesmo assim. resultado: %#v", res)
	}
}

// TestIntegrationPendenciasSemCobrancaExcluiMesesPagos cobre o lado oposto
// do teste acima: um mês com um evento "paga" registrado (a fonte correta e
// única de exclusão, vinda de financeiro_mensalidade_obrigacoes_eventos) NÃO
// deve aparecer em pendências_sem_cobranca — mesmo que o mesmo estudante
// tenha outros meses pendentes.
func TestIntegrationPendenciasSemCobrancaExcluiMesesPagos(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2026_2027")
	seedMensalidadeTurma(t, client, academia, "T-PAGO-A", "2026_2027", "ESTPAGO1", nil)
	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "7_ano_fundamental", nil, 15000, 7, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	if _, err := client.DB().Exec(`INSERT INTO financeiro_mensalidade_obrigacoes_eventos (event_id,aggregate_id,codigo_estudante,codigo_academia,ano_letivo,mes,tipo,ocorrido_em) VALUES ($1,$2,'ESTPAGO1',$3,'2026_2027',9,'paga',CURRENT_TIMESTAMP)`,
		uuid.New(), uuid.New(), academia); err != nil {
		t.Fatal(err)
	}

	res, err := service.PendenciasSemCobranca(ctx, academia, nil, nil, "", "2026_2027", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range res {
		if m.CodigoEstudante == "ESTPAGO1" && m.Mes == 9 {
			t.Fatalf("ESTPAGO1/setembro já foi PAGO; não deveria aparecer em pendências_sem_cobranca: %#v", m)
		}
	}
	outrosMeses := 0
	for _, m := range res {
		if m.CodigoEstudante == "ESTPAGO1" {
			outrosMeses++
		}
	}
	if outrosMeses == 0 {
		t.Fatal("ESTPAGO1 deveria continuar com outros meses pendentes além de setembro (que já foi pago)")
	}
}

// TestIntegrationPendenciasSemCobrancaExcluiMesesAnuladosEIncluiReativados
// cobre o outro caso de exclusão legítima (Estado == EstadoAnulado) e
// confirma que reativar volta a listar o mês — usando
// AnularObrigacoesMensalidade/ReativarObrigacoesMensalidade (o caminho de
// comando real, não INSERT direto), porque este teste também serve de
// regressão para essas duas operações continuarem consistentes com
// PendenciasSemCobranca depois da mudança de critério de exclusão.
func TestIntegrationPendenciasSemCobrancaExcluiMesesAnuladosEIncluiReativados(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2026_2027")
	seedMensalidadeTurma(t, client, academia, "T-ANUL-A", "2026_2027", "ESTANUL1", nil)
	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "7_ano_fundamental", nil, 15000, 7, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	in := ObrigacaoMensalidadeInput{CodigoEstudante: "ESTANUL1", CodigoAcademia: academia, AnoLetivo: "2026_2027", Meses: []int{9}}
	if err := service.AnularObrigacoesMensalidade(ctx, in, uuid.NewString(), "academia", "127.0.0.1"); err != nil {
		t.Fatal(err)
	}

	res, err := service.PendenciasSemCobranca(ctx, academia, nil, nil, "", "2026_2027", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range res {
		if m.CodigoEstudante == "ESTANUL1" && m.Mes == 9 {
			t.Fatalf("ESTANUL1/setembro foi ANULADO; não deveria aparecer em pendências_sem_cobranca: %#v", m)
		}
	}

	if err := service.ReativarObrigacoesMensalidade(ctx, in, uuid.NewString(), "academia", "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	resReativado, err := service.PendenciasSemCobranca(ctx, academia, nil, nil, "", "2026_2027", nil)
	if err != nil {
		t.Fatal(err)
	}
	achouReativado := false
	for _, m := range resReativado {
		if m.CodigoEstudante == "ESTANUL1" && m.Mes == 9 {
			achouReativado = true
			if m.Estado != EstadoPendente {
				t.Fatalf("esperava estado pendente após reativação, obteve %q", m.Estado)
			}
		}
	}
	if !achouReativado {
		t.Fatal("ESTANUL1/setembro foi reativado; deveria voltar a aparecer em pendências_sem_cobranca")
	}
}

// TestIntegrationPendenciasSemCobrancaEstudanteIncluiMesComTentativaFalhada
// cobre a mesma mudança de critério (ver
// TestIntegrationPendenciasSemCobrancaIncluiMesesComTentativaFalhadaOuSemNenhuma),
// só que no caminho por estudante único (PendenciasSemCobrancaEstudante,
// usada por ConsultarCobrancasEstudante) — os dois caminhos precisam
// continuar consistentes entre si.
func TestIntegrationPendenciasSemCobrancaEstudanteIncluiMesComTentativaFalhada(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2026_2027")
	seedMensalidadeTurma(t, client, academia, "T-PNDE-B", "2026_2027", "ESTPNE04", nil)
	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "7_ano_fundamental", nil, 15000, 7, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	seedFinanceiroCobrancaMensalidade(t, client, academia, "ESTPNE04", "falhada", "2026_2027", 9, 15000)

	res, err := service.PendenciasSemCobrancaEstudante(ctx, "ESTPNE04", &academia)
	if err != nil {
		t.Fatal(err)
	}
	achouSetembro := false
	for _, m := range res {
		if m.Mes == 9 {
			achouSetembro = true
		}
	}
	if !achouSetembro {
		t.Fatalf("ESTPNE04/setembro já tentou (falhou) mas continua sem pagar; deveria aparecer em pendências_sem_cobranca. resultado: %#v", res)
	}
}

// TestIntegrationPendenciasSemCobrancaExigeEscopo cobre a proteção contra
// varredura sem limite: sem nenhum filtro de escopo (turma_id, curso_id,
// ano_academico ou ano_letivo), PendenciasSemCobranca processaria a
// academia inteira a cada chamada. A função rejeita explicitamente essa
// chamada com erro de validação.
func TestIntegrationPendenciasSemCobrancaExigeEscopo(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	if _, err := service.PendenciasSemCobranca(ctx, academia, nil, nil, "", "", nil); err == nil {
		t.Fatal("esperava erro de validação sem nenhum filtro de escopo")
	}
	if _, err := service.PendenciasSemCobranca(ctx, "", nil, nil, "", "2026_2027", nil); err == nil {
		t.Fatal("esperava erro de validação sem codigo_academia")
	}
}

// TestIntegrationPendenciasSemCobrancaEstudanteNaoExigeEscopo cobre a versão
// por estudante: como já está inerentemente limitada a UM estudante, não
// exige nenhum filtro extra — usada por ConsultarCobrancasEstudante.
func TestIntegrationPendenciasSemCobrancaEstudanteNaoExigeEscopo(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2026_2027")
	seedMensalidadeTurma(t, client, academia, "T-PNDE-A", "2026_2027", "ESTPN03", nil)
	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "7_ano_fundamental", nil, 15000, 7, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	res, err := service.PendenciasSemCobrancaEstudante(ctx, "ESTPN03", &academia)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) == 0 {
		t.Fatal("esperava pendências sem cobrança para ESTPN03")
	}
	for _, m := range res {
		if m.CodigoEstudante != "ESTPN03" {
			t.Fatalf("resultado contém outro estudante: %#v", m)
		}
	}
}

// TestIntegrationListCobrancasFiltraPorEscopoMensalidade cobre o problema 2
// da tarefa 58: ListCobrancas passa a aceitar turma_id/curso_id/
// ano_academico/ano_letivo para restringir o resultado a cobranças de
// mensalidade vinculadas a esse escopo. Duas turmas da MESMA academia:
// filtrar por uma delas não deve trazer cobranças da outra.
func TestIntegrationListCobrancasFiltraPorEscopoMensalidade(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2026_2027")
	seedMensalidadeTurma(t, client, academia, "T-FLT-A", "2026_2027", "ESTFL01", nil)
	seedMensalidadeTurma(t, client, academia, "T-FLT-B", "2026_2027", "ESTFL02", nil)

	seedFinanceiroCobrancaMensalidade(t, client, academia, "ESTFL01", "Success", "2026_2027", 9, 15000)
	seedFinanceiroCobrancaMensalidade(t, client, academia, "ESTFL02", "Success", "2026_2027", 9, 16000)

	semFiltro, err := service.ListCobrancas(ctx, ContextoAcademia, academia, nil, nil, nil, nil, "", "", nil, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if semFiltro.Total != 2 {
		t.Fatalf("esperava 2 cobranças sem filtro de escopo, obteve %d", semFiltro.Total)
	}

	comFiltroAno, err := service.ListCobrancas(ctx, ContextoAcademia, academia, nil, nil, nil, nil, "7_ano_fundamental", "", nil, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if comFiltroAno.Total != 2 {
		t.Fatalf("as duas turmas são 7_ano_fundamental (mesmo ano_academico); esperava 2, obteve %d", comFiltroAno.Total)
	}

	comFiltroAnoLetivoInexistente, err := service.ListCobrancas(ctx, ContextoAcademia, academia, nil, nil, nil, nil, "", "2099_2100", nil, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if comFiltroAnoLetivoInexistente.Total != 0 {
		t.Fatalf("ano_letivo inexistente deveria devolver 0 cobranças, obteve %d", comFiltroAnoLetivoInexistente.Total)
	}
}

// TestIntegrationListCobrancasFiltraPorMes cobre a tarefa 60: mes restringe
// ainda mais um escopo já delimitado por ano_letivo (ou outro dos quatro
// filtros) a um único mês de calendário — necessário para o fluxo de
// drill-down do frontend (ano letivo -> mês -> lista) paginar corretamente
// sem precisar buscar o ano letivo inteiro para filtrar no cliente.
func TestIntegrationListCobrancasFiltraPorMes(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2026_2027")
	seedMensalidadeTurma(t, client, academia, "T-MES-A", "2026_2027", "ESTMS01", nil)

	seedFinanceiroCobrancaMensalidade(t, client, academia, "ESTMS01", "Success", "2026_2027", 9, 15000)
	seedFinanceiroCobrancaMensalidade(t, client, academia, "ESTMS01", "Success", "2026_2027", 10, 15000)

	mesNove := 9
	comMes, err := service.ListCobrancas(ctx, ContextoAcademia, academia, nil, nil, nil, nil, "", "2026_2027", &mesNove, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if comMes.Total != 1 {
		t.Fatalf("esperava 1 cobrança filtrando por mes=9, obteve %d", comMes.Total)
	}

	mesDez := 12
	comMesSemCobranca, err := service.ListCobrancas(ctx, ContextoAcademia, academia, nil, nil, nil, nil, "", "2026_2027", &mesDez, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if comMesSemCobranca.Total != 0 {
		t.Fatalf("dezembro não tem cobrança nenhuma; esperava 0, obteve %d", comMesSemCobranca.Total)
	}

	semMes, err := service.ListCobrancas(ctx, ContextoAcademia, academia, nil, nil, nil, nil, "", "2026_2027", nil, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if semMes.Total != 2 {
		t.Fatalf("sem filtro de mes, esperava as 2 cobranças (setembro e outubro), obteve %d", semMes.Total)
	}
}

// TestIntegrationPendenciasSemCobrancaFiltraPorMes cobre o mesmo filtro
// aplicado a PendenciasSemCobranca — o passo final do drill-down do
// frontend precisa das pendências de UM mês específico, não do ano letivo
// inteiro.
func TestIntegrationPendenciasSemCobrancaFiltraPorMes(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2026_2027")
	seedMensalidadeTurma(t, client, academia, "T-MESP-A", "2026_2027", "ESTMP01", nil)
	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "7_ano_fundamental", nil, 15000, 7, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	mesSetembro := 9
	res, err := service.PendenciasSemCobranca(ctx, academia, nil, nil, "", "2026_2027", &mesSetembro)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 {
		t.Fatalf("esperava exatamente 1 pendência (setembro), obteve %d: %#v", len(res), res)
	}
	if res[0].Mes != 9 {
		t.Fatalf("esperava mes=9, obteve %d", res[0].Mes)
	}

	semMes, err := service.PendenciasSemCobranca(ctx, academia, nil, nil, "", "2026_2027", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(semMes) <= 1 {
		t.Fatalf("sem filtro de mes, esperava mais de 1 pendência (todo o ano letivo), obteve %d", len(semMes))
	}
}

// TestIntegrationPendenciasSemCobrancaNaoDuplicaEstudanteEmDuasTurmasMesmoAno
// cobre um caso de borda da correção de performance de PendenciasSemCobranca
// (tarefa "GET /financeiro/cobrancas — lentidão de vários minutos com
// ano_letivo"): escopoMensalidadeEstudantes inclui turma_id na
// deduplicação (SELECT DISTINCT ... turma_id, ...), diferente de
// vinculosMensalidade (que dedupe por academia+ano_letivo+nivel+
// ano_academico+curso_id, SEM turma_id). Um estudante que aparece em DUAS
// turmas diferentes para a MESMA combinação (ex.: transferência de turma no
// meio do ano letivo histórico) produz duas linhas distintas em
// escopoMensalidadeEstudantes — PendenciasSemCobranca precisa deduplicar
// essas linhas antes de expandir os meses, ou listaria cada mês pendente
// duas vezes para esse estudante.
func TestIntegrationPendenciasSemCobrancaNaoDuplicaEstudanteEmDuasTurmasMesmoAno(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2026_2027")
	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "6_ano_fundamental", nil, 15000, 7, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC))
	seedMensalidadeTurma(t, client, academia, "T-DUP-A", "2020_2021", "ESTDUP01", nil)
	seedMensalidadeTurma(t, client, academia, "T-DUP-B", "2020_2021", "ESTDUP01", nil)

	mesSetembro := 9
	res, err := service.PendenciasSemCobranca(ctx, academia, nil, nil, "", "2020_2021", &mesSetembro)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, m := range res {
		if m.CodigoEstudante == "ESTDUP01" && m.Mes == 9 {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("esperava exatamente 1 pendência para ESTDUP01/setembro (estudante em 2 turmas do mesmo ano), obteve %d: %#v", count, res)
	}

	semMes, err := service.PendenciasSemCobranca(ctx, academia, nil, nil, "", "2020_2021", nil)
	if err != nil {
		t.Fatal(err)
	}
	porMes := map[int]int{}
	for _, m := range semMes {
		if m.CodigoEstudante == "ESTDUP01" {
			porMes[m.Mes]++
		}
	}
	if len(porMes) == 0 {
		t.Fatal("esperava pendências para ESTDUP01 no ano letivo inteiro")
	}
	for mes, qtd := range porMes {
		if qtd != 1 {
			t.Fatalf("mês %d apareceu %d vezes para ESTDUP01 (esperava exatamente 1)", mes, qtd)
		}
	}
}
