package finance

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestIntegrationConfigureMensalidadeGravaNoLedgerEProjectaCorretamente
// cobre o caminho de escrita completo de ConfigureMensalidade: o evento deve
// ser aceite pelo ledger (whitelist de tipos de evento) e a projeção de
// leitura financeiro_mensalidade_configuracoes deve refletir imediatamente o
// que foi configurado, incluindo o campo metodos_pagamento (TEXT[]).
func TestIntegrationConfigureMensalidadeGravaNoLedgerEProjectaCorretamente(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2025_2026")
	configureIntegrationCredential(t, service, ContextoAcademia, academia)

	view, err := service.ConfigureMensalidade(ctx, MensalidadeConfiguracaoInput{
		CodigoAcademia: academia, Nivel: NivelFundamental, AnoAcademico: "6_ano_fundamental",
		Valor: 1000, MesFimCobranca: 7, MetodosPagamento: []string{"GPO", "REF"}, ModoVigencia: ModoVigenciaAPartirDaAtualizacao,
	}, "admin-teste", "academia", "127.0.0.1")
	if err != nil {
		t.Fatalf("ConfigureMensalidade retornou erro: %v", err)
	}
	if view.Valor != 1000 {
		t.Fatalf("valor inesperado na view retornada: %v", view.Valor)
	}

	var ledgerCount int
	if err := client.DB().QueryRow(`SELECT COUNT(*) FROM spuri_ledger WHERE aggregate_type='Financeiro' AND event_type='MensalidadeConfigurada'`).Scan(&ledgerCount); err != nil {
		t.Fatal(err)
	}
	if ledgerCount != 1 {
		t.Fatalf("esperava 1 evento MensalidadeConfigurada no ledger, obteve %d", ledgerCount)
	}

	configs, err := service.ListMensalidadeConfiguracoes(ctx, academia)
	if err != nil {
		t.Fatal(err)
	}
	if len(configs) != 1 {
		t.Fatalf("esperava 1 configuração visível em financeiro_mensalidade_configuracoes, obteve %d", len(configs))
	}
	if len(configs[0].MetodosPagamento) != 2 {
		t.Fatalf("esperava 2 métodos de pagamento persistidos, obteve %v", configs[0].MetodosPagamento)
	}
}

// TestIntegrationConfigureMatriculaGravaNoLedgerEProjectaCorretamente cobre
// o mesmo caminho de escrita para ConfigureMatricula, cujo evento
// MatriculaConfigurada estava totalmente ausente da whitelist do ledger.
func TestIntegrationConfigureMatriculaGravaNoLedgerEProjectaCorretamente(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2025_2026")
	configureIntegrationCredential(t, service, ContextoAcademia, academia)

	view, err := service.ConfigureMatricula(ctx, MatriculaConfiguracaoInput{
		CodigoAcademia: academia, Nivel: NivelFundamental, AnoAcademico: "6_ano_fundamental",
		Valor: 5000, MetodosPagamento: []string{"GPO"}, ModoVigencia: ModoVigenciaAPartirDaAtualizacao,
	}, "admin-teste", "academia", "127.0.0.1")
	if err != nil {
		t.Fatalf("ConfigureMatricula retornou erro: %v", err)
	}
	if view.Valor != 5000 {
		t.Fatalf("valor inesperado na view retornada: %v", view.Valor)
	}

	var ledgerCount int
	if err := client.DB().QueryRow(`SELECT COUNT(*) FROM spuri_ledger WHERE aggregate_type='Financeiro' AND event_type='MatriculaConfigurada'`).Scan(&ledgerCount); err != nil {
		t.Fatal(err)
	}
	if ledgerCount != 1 {
		t.Fatalf("esperava 1 evento MatriculaConfigurada no ledger, obteve %d", ledgerCount)
	}

	configs, err := service.ListMatriculaConfiguracoes(ctx, academia)
	if err != nil {
		t.Fatal(err)
	}
	if len(configs) != 1 {
		t.Fatalf("esperava 1 configuração visível em financeiro_matricula_configuracoes, obteve %d", len(configs))
	}
	if len(configs[0].MetodosPagamento) != 1 || configs[0].MetodosPagamento[0] != "GPO" {
		t.Fatalf("métodos de pagamento persistidos incorretamente: %v", configs[0].MetodosPagamento)
	}
}

// TestIntegrationPagamentoMensalidadeConfirmadoPelaAppyPayMarcaComoPago é o
// teste mais importante deste ficheiro: reproduz o fluxo real de um
// estudante a pagar a propina (mensalidade) - criação da cobrança, seguida
// da AppyPay confirmando o pagamento como "Success" - e garante que essa
// confirmação fica de facto registada no ledger (evento
// MensalidadesCobrancaConfirmada) e refletida como "pago" na leitura.
func TestIntegrationPagamentoMensalidadeConfirmadoPelaAppyPayMarcaComoPago(t *testing.T) {
	t.Setenv("APPYPAY_RESOURCE", "integration-resource")
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	estudante := "EST-PAGA-" + uuid.NewString()[:8]
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2025_2026")
	seedMensalidadeTurma(t, client, academia, "T-PAGA", "2025_2026", estudante, nil)
	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "6_ano_fundamental", nil, 1000, 7, time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC))
	if _, err := client.DB().Exec(`UPDATE financeiro_mensalidade_configuracoes SET metodos_pagamento='{GPO}' WHERE codigo_academia=$1`, academia); err != nil {
		t.Fatal(err)
	}

	configureIntegrationCredential(t, service, ContextoAcademia, academia)
	transport := &appyPayMockTransport{status: "Pending"}
	service.SetHTTPClient(&http.Client{Transport: transport})

	pendentesAntes, err := service.ListMensalidades(ctx, estudante, &academia)
	if err != nil {
		t.Fatal(err)
	}
	if len(pendentesAntes) == 0 {
		t.Fatal("esperava pelo menos uma mensalidade pendente antes do pagamento")
	}
	alvo := pendentesAntes[0]
	if alvo.Estado != EstadoPendente {
		t.Fatalf("primeira mensalidade da lista não está pendente: %#v", alvo)
	}

	view, err := service.IniciarPagamentoMensalidades(ctx, MensalidadePagamentoInput{
		CodigoEstudante: estudante, CodigoAcademia: academia,
		Meses:           []MensalidadeSelecaoMes{{AnoLetivo: alvo.AnoLetivo, Mes: alvo.Mes}},
		MetodoPagamento: "GPO", Telefone: "923000000",
	}, estudante, "estudante", "127.0.0.1")
	if err != nil {
		t.Fatalf("IniciarPagamentoMensalidades falhou: %v", err)
	}

	// A AppyPay confirma o pagamento de forma assíncrona; o Spuri descobre
	// isso por consulta (ou webhook, mesmo caminho interno). Simula a
	// confirmação exatamente como um pagamento GPO/REF real acontece.
	transport.status = "Success"
	consulted, err := service.ConsultCharge(ctx, ContextoAcademia, academia, view.Charge.ID.String(), estudante, "estudante", "127.0.0.1")
	if err != nil {
		t.Fatalf("ConsultCharge falhou: %v", err)
	}
	if !isSuccessfulChargeStatus(consulted.Status) {
		t.Fatalf("mock não retornou Success na consulta: status=%s", consulted.Status)
	}

	var ledgerCountConfirmada int
	if err := client.DB().QueryRow(`SELECT COUNT(*) FROM spuri_ledger WHERE aggregate_type='Financeiro' AND event_type='MensalidadesCobrancaConfirmada'`).Scan(&ledgerCountConfirmada); err != nil {
		t.Fatal(err)
	}
	if ledgerCountConfirmada != 1 {
		t.Fatalf("esperava 1 evento MensalidadesCobrancaConfirmada no ledger após pagamento Success, obteve %d", ledgerCountConfirmada)
	}

	var obrigacoesPagas int
	if err := client.DB().QueryRow(`SELECT COUNT(*) FROM financeiro_mensalidade_obrigacoes_eventos WHERE codigo_estudante=$1 AND codigo_academia=$2 AND ano_letivo=$3 AND mes=$4 AND tipo='paga'`, estudante, academia, alvo.AnoLetivo, alvo.Mes).Scan(&obrigacoesPagas); err != nil {
		t.Fatal(err)
	}
	if obrigacoesPagas != 1 {
		t.Fatalf("esperava 1 linha 'paga' na obrigação de mensalidade, obteve %d", obrigacoesPagas)
	}

	pendentesDepois, err := service.ListMensalidades(ctx, estudante, &academia)
	if err != nil {
		t.Fatal(err)
	}
	alvoDepois := mensalidadePorMes(t, pendentesDepois, academia, alvo.AnoLetivo, alvo.Mes)
	if alvoDepois.Estado != EstadoPago {
		t.Fatalf("mensalidade paga continua com estado=%q após pagamento confirmado como Success pela AppyPay (deveria ser %q)", alvoDepois.Estado, EstadoPago)
	}
}

// TestIntegrationListMensalidadesOrdemCronologicaAnoLetivo garante que os
// meses dentro de um mesmo ano_letivo são devolvidos em ordem cronológica
// real (setembro..dezembro do 1º ano civil, depois janeiro..julho do 2º),
// e não pela ordenação numérica crua do número do mês.
func TestIntegrationListMensalidadesOrdemCronologicaAnoLetivo(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	estudante := "EST-ORDEM-" + uuid.NewString()[:8]
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2025_2026")
	seedMensalidadeTurma(t, client, academia, "T-ORDEM", "2025_2026", estudante, nil)
	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "6_ano_fundamental", nil, 1000, 7, time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC))

	valores, err := service.ListMensalidades(ctx, estudante, &academia)
	if err != nil {
		t.Fatal(err)
	}
	var meses []int
	for _, v := range valores {
		if v.AnoLetivo == "2025_2026" {
			meses = append(meses, v.Mes)
		}
	}
	esperado := []int{9, 10, 11, 12, 1, 2, 3, 4, 5, 6, 7}
	if len(meses) != len(esperado) {
		t.Fatalf("esperava %d meses, obteve %d (%v)", len(esperado), len(meses), meses)
	}
	for i := range esperado {
		if meses[i] != esperado[i] {
			t.Fatalf("ordem cronológica incorreta.\n  esperado (set->jul): %v\n  obtido:              %v", esperado, meses)
		}
	}
}

// TestIntegrationRebuildFinanceiroReconstroiConfiguracoesEcobrancasMensalidade
// garante que um Rebuild completo da projeção financeiro reconstrói TODAS as
// tabelas derivadas do ledger, incluindo financeiro_matricula_configuracoes
// e financeiro_mensalidade_cobrancas, que antes sobreviviam intactas (não
// eram limpas) a um Rebuild.
func TestIntegrationRebuildFinanceiroReconstroiConfiguracoesEcobrancasMensalidade(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2025_2026")
	configureIntegrationCredential(t, service, ContextoAcademia, academia)

	if _, err := service.ConfigureMatricula(ctx, MatriculaConfiguracaoInput{
		CodigoAcademia: academia, Nivel: NivelFundamental, AnoAcademico: "6_ano_fundamental",
		Valor: 5000, MetodosPagamento: []string{"GPO"}, ModoVigencia: ModoVigenciaAPartirDaAtualizacao,
	}, "admin-teste", "academia", "127.0.0.1"); err != nil {
		t.Fatalf("ConfigureMatricula falhou: %v", err)
	}

	// Simula corrupção/drift do read model: apaga a linha manualmente sem
	// tocar no ledger (fonte da verdade).
	if _, err := client.DB().Exec(`DELETE FROM financeiro_matricula_configuracoes WHERE codigo_academia=$1`, academia); err != nil {
		t.Fatal(err)
	}

	if err := service.projection.Rebuild(); err != nil {
		t.Fatalf("Rebuild() falhou: %v", err)
	}

	configs, err := service.ListMatriculaConfiguracoes(ctx, academia)
	if err != nil {
		t.Fatal(err)
	}
	if len(configs) != 1 {
		t.Fatalf("após Rebuild(), esperava financeiro_matricula_configuracoes reconstruída a partir do ledger (1 linha), obteve %d", len(configs))
	}
}
