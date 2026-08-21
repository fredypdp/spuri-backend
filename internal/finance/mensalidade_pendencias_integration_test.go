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
// registrada para o mês informado — o caso que PendenciasSemCobranca deve
// EXCLUIR do resultado (a cobrança pode ter falhado; o que importa é que
// já existiu tentativa).
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

// TestIntegrationPendenciasSemCobrancaExcluiQuandoJaExisteTentativa cobre o
// problema 1 da tarefa 58: um estudante que deve uma mensalidade mas nunca
// gerou (nem tentou gerar) nenhuma cobrança fica hoje totalmente invisível
// para a academia em qualquer consulta de pagamentos — só ele mesmo vê a
// própria dívida, via GET /financeiro/mensalidades/estudante/:codigo.
//
// ESTPN01 nunca tentou nenhuma cobrança: TODOS os seus meses pendentes
// devem aparecer em PendenciasSemCobranca. ESTPN02 já tem uma cobrança
// falhada para setembro: aquele mês específico NÃO deve aparecer (já está
// visível de outra forma, na listagem normal de cobranças), mas os demais
// meses dele, sim.
func TestIntegrationPendenciasSemCobrancaExcluiQuandoJaExisteTentativa(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2026_2027")
	seedMensalidadeTurma(t, client, academia, "T-PND-A", "2026_2027", "ESTPN01", nil)
	seedMensalidadeTurma(t, client, academia, "T-PND-B", "2026_2027", "ESTPN02", nil)
	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "7_ano_fundamental", nil, 15000, 7, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	// ESTPN02 já tem uma tentativa de cobrança (falhada) para setembro —
	// não deve aparecer como "pendência sem cobrança" para esse mês.
	seedFinanceiroCobrancaMensalidade(t, client, academia, "ESTPN02", "falhada", "2026_2027", 9, 15000)

	res, err := service.PendenciasSemCobranca(ctx, academia, nil, nil, "", "2026_2027")
	if err != nil {
		t.Fatal(err)
	}

	achouEst1Setembro := false
	for _, m := range res {
		if m.CodigoEstudante == "ESTPN02" && m.Mes == 9 {
			t.Fatalf("ESTPN02/setembro já tem cobrança (falhada); não deveria aparecer como pendência sem cobrança: %#v", m)
		}
		if m.CodigoEstudante == "ESTPN01" && m.Mes == 9 {
			achouEst1Setembro = true
			if m.Estado != EstadoPendente {
				t.Fatalf("esperava estado pendente, obteve %q", m.Estado)
			}
		}
	}
	if !achouEst1Setembro {
		t.Fatalf("ESTPN01/setembro nunca teve nenhuma cobrança; deveria aparecer como pendência sem cobrança. resultado: %#v", res)
	}

	// ESTPN02 continua tendo os OUTROS meses (out..jul) como pendência
	// sem cobrança — só setembro está coberto pela tentativa já existente.
	outrosMesesEst2 := 0
	for _, m := range res {
		if m.CodigoEstudante == "ESTPN02" {
			outrosMesesEst2++
		}
	}
	if outrosMesesEst2 == 0 {
		t.Fatalf("ESTPN02 deveria ter outros meses pendentes sem cobrança além de setembro")
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
	if _, err := service.PendenciasSemCobranca(ctx, academia, nil, nil, "", ""); err == nil {
		t.Fatal("esperava erro de validação sem nenhum filtro de escopo")
	}
	if _, err := service.PendenciasSemCobranca(ctx, "", nil, nil, "", "2026_2027"); err == nil {
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

	semFiltro, err := service.ListCobrancas(ctx, ContextoAcademia, academia, nil, nil, nil, nil, "", "", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if semFiltro.Total != 2 {
		t.Fatalf("esperava 2 cobranças sem filtro de escopo, obteve %d", semFiltro.Total)
	}

	comFiltroAno, err := service.ListCobrancas(ctx, ContextoAcademia, academia, nil, nil, nil, nil, "7_ano_fundamental", "", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if comFiltroAno.Total != 2 {
		t.Fatalf("as duas turmas são 7_ano_fundamental (mesmo ano_academico); esperava 2, obteve %d", comFiltroAno.Total)
	}

	comFiltroAnoLetivoInexistente, err := service.ListCobrancas(ctx, ContextoAcademia, academia, nil, nil, nil, nil, "", "2099_2100", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if comFiltroAnoLetivoInexistente.Total != 0 {
		t.Fatalf("ano_letivo inexistente deveria devolver 0 cobranças, obteve %d", comFiltroAnoLetivoInexistente.Total)
	}
}
