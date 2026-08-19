package finance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"spuri/internal/db"
)

func seedMensalidadeConfiguracaoRemocao(t *testing.T, client *db.Client, academia, nivel, ano string, curso *uuid.UUID, removidoEm time.Time) {
	t.Helper()
	cursoID := any(nil)
	if curso != nil {
		cursoID = *curso
	}
	_, err := client.DB().Exec(`INSERT INTO financeiro_mensalidade_configuracoes_remocoes
		(event_id,aggregate_id,codigo_academia,nivel,ano_academico,curso_id,removido_em)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`, uuid.New(), uuid.New(), academia, nivel, ano, cursoID, removidoEm)
	if err != nil {
		t.Fatal(err)
	}
}

// Testa resolveConfiguracao com uma linha do tempo totalmente controlada via
// SQL (independente do relógio real), replicando o mesmo padrão do teste
// pré-existente TestIntegrationMensalidadePrimeiraConfiguracaoRetroageSemReescreverHistorico.
func TestIntegrationResolveConfiguracaoComRemocaoPreservaHistoricoDePreco(t *testing.T) {
	client := integrationClient(t)
	academia := mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2026_2027")
	service := NewService(client)

	// T1 = configuração original (5000), vigente desde o início do ano.
	// T2 = referência histórica que já foi cobrada com essa configuração.
	// T3 = remoção da configuração (depois de T2 já ter sido cobrado).
	// T4 = referência "atual", depois da remoção.
	// T5 = reconfiguração posterior à remoção.
	t1 := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	t2Historica := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	t3Remocao := time.Date(2027, 1, 15, 0, 0, 0, 0, time.UTC)
	t4Atual := time.Date(2027, 2, 1, 0, 0, 0, 0, time.UTC)

	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "7_ano_fundamental", nil, 5000, 7, t1)

	cfg, err := service.resolveConfiguracao(context.Background(), academia, NivelFundamental, "7_ano_fundamental", nil, t2Historica)
	if err != nil || cfg.Valor != 5000 {
		t.Fatalf("esperava 5000 para referência histórica antes de qualquer remoção, obteve valor=%v err=%v", cfg.Valor, err)
	}

	seedMensalidadeConfiguracaoRemocao(t, client, academia, NivelFundamental, "7_ano_fundamental", nil, t3Remocao)

	// T2 (mês já cobrado ANTES da remoção) continua resolvendo para 5000.
	cfgHistorica, err := service.resolveConfiguracao(context.Background(), academia, NivelFundamental, "7_ano_fundamental", nil, t2Historica)
	if err != nil {
		t.Fatalf("resolveConfiguracao para T2 (antes da remoção) não deveria falhar: %v", err)
	}
	if cfgHistorica.Valor != 5000 {
		t.Fatalf("preço histórico alterado pela remoção: esperado 5000, obteve %v", cfgHistorica.Valor)
	}

	// T4 (depois da remoção) não tem mais configuração ativa.
	if _, err := service.resolveConfiguracao(context.Background(), academia, NivelFundamental, "7_ano_fundamental", nil, t4Atual); !errors.Is(err, ErrNotFound) {
		t.Fatalf("esperava ErrNotFound para referência pós-remoção, obteve: %v", err)
	}

	// Reconfiguração em T5 (depois da remoção) volta a valer a partir de
	// T5, sem alterar nada antes disso e sem preencher retroativamente o
	// intervalo T3-T5.
	t5Reconfig := time.Date(2027, 3, 1, 0, 0, 0, 0, time.UTC)
	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "7_ano_fundamental", nil, 6000, 7, t5Reconfig)

	cfgHistoricaAindaIntacta, err := service.resolveConfiguracao(context.Background(), academia, NivelFundamental, "7_ano_fundamental", nil, t2Historica)
	if err != nil || cfgHistoricaAindaIntacta.Valor != 5000 {
		t.Fatalf("reconfiguração alterou o histórico: esperado 5000 para T2, obteve valor=%v err=%v", cfgHistoricaAindaIntacta.Valor, err)
	}
	cfgPosReconfig, err := service.resolveConfiguracao(context.Background(), academia, NivelFundamental, "7_ano_fundamental", nil, time.Date(2027, 4, 1, 0, 0, 0, 0, time.UTC))
	if err != nil || cfgPosReconfig.Valor != 6000 {
		t.Fatalf("esperava 6000 depois da reconfiguração pós-remoção, obteve valor=%v err=%v", cfgPosReconfig.Valor, err)
	}
	if _, err := service.resolveConfiguracao(context.Background(), academia, NivelFundamental, "7_ano_fundamental", nil, t4Atual); !errors.Is(err, ErrNotFound) {
		t.Fatalf("reconfiguração em T5 não deveria preencher retroativamente o intervalo removido (T3-T5), obteve: %v", err)
	}
}

// Testa o fluxo de COMANDO completo (Service.RemoveMensalidadeConfiguracao),
// incluindo efeitos colaterais na listagem, métodos de pagamento, ledger e
// Rebuild().
func TestIntegrationRemoveMensalidadeConfiguracaoFluxoDeComando(t *testing.T) {
	client := integrationClient(t)
	academia := mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2026_2027")
	service := NewService(client)

	if _, _, err := service.ConfigureCredential(context.Background(), nil, CredentialInput{
		ContextoTipo: ContextoAcademia, CodigoAcademia: academia,
		ClientID: "client-" + uuid.NewString(), ClientSecret: "secret-" + uuid.NewString(),
		GPOPaymentMethod: "GPO_QR_TEST", REFPaymentMethod: "REF_TEST",
	}, uuid.NewString(), "academia", "127.0.0.1"); err != nil {
		t.Fatalf("ConfigureCredential não deveria falhar: %v", err)
	}
	if _, err := service.ConfigureMensalidade(context.Background(), MensalidadeConfiguracaoInput{
		CodigoAcademia: academia, Nivel: NivelFundamental, AnoAcademico: "7_ano_fundamental",
		Valor: 5000, MesFimCobranca: 7, MetodosPagamento: []string{"GPO_QR"},
	}, uuid.NewString(), "academia", "127.0.0.1"); err != nil {
		t.Fatalf("ConfigureMensalidade falhou: %v", err)
	}

	if err := service.RemoveMensalidadeConfiguracao(context.Background(), academia, NivelFundamental, "7_ano_fundamental", nil, uuid.NewString(), "academia", "127.0.0.1"); err != nil {
		t.Fatalf("RemoveMensalidadeConfiguracao não deveria falhar: %v", err)
	}

	configs, err := service.ListMensalidadeConfiguracoes(context.Background(), academia)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range configs {
		if c.Nivel == NivelFundamental && c.AnoAcademico == "7_ano_fundamental" {
			t.Fatalf("configuração removida ainda aparece em ListMensalidadeConfiguracoes: %#v", c)
		}
	}

	metodos, err := service.metodosPagamentoMensalidade(context.Background(), academia)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range metodos {
		if m == "GPO_QR" {
			t.Fatalf("método de pagamento GPO_QR ainda habilitado após remoção da única configuração")
		}
	}

	aggID := mensalidadeAggregateID(academia)
	var configurados, removidos int
	if err := client.DB().QueryRow(`SELECT COUNT(*) FROM spuri_ledger WHERE aggregate_id=$1 AND event_type='MensalidadeConfigurada'`, aggID).Scan(&configurados); err != nil {
		t.Fatal(err)
	}
	if configurados != 1 {
		t.Fatalf("esperava 1 evento MensalidadeConfigurada preservado, obteve %d", configurados)
	}
	if err := client.DB().QueryRow(`SELECT COUNT(*) FROM spuri_ledger WHERE aggregate_id=$1 AND event_type='MensalidadeConfiguracaoRemovida'`, aggID).Scan(&removidos); err != nil {
		t.Fatal(err)
	}
	if removidos != 1 {
		t.Fatalf("esperava 1 evento MensalidadeConfiguracaoRemovida, obteve %d", removidos)
	}

	if err := service.RemoveMensalidadeConfiguracao(context.Background(), academia, NivelFundamental, "7_ano_fundamental", nil, uuid.NewString(), "academia", "127.0.0.1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("segunda remoção deveria falhar com ErrNotFound, obteve: %v", err)
	}

	if _, err := service.ConfigureMensalidade(context.Background(), MensalidadeConfiguracaoInput{
		CodigoAcademia: academia, Nivel: NivelFundamental, AnoAcademico: "7_ano_fundamental",
		Valor: 6000, MesFimCobranca: 7, MetodosPagamento: []string{"GPO_QR"},
	}, uuid.NewString(), "academia", "127.0.0.1"); err != nil {
		t.Fatalf("reconfiguração após remoção não deveria falhar: %v", err)
	}
	configsPos, err := service.ListMensalidadeConfiguracoes(context.Background(), academia)
	if err != nil {
		t.Fatal(err)
	}
	achou := false
	for _, c := range configsPos {
		if c.Nivel == NivelFundamental && c.AnoAcademico == "7_ano_fundamental" {
			achou = true
			if c.Valor != 6000 {
				t.Fatalf("esperava valor 6000 após reconfiguração, obteve %v", c.Valor)
			}
		}
	}
	if !achou {
		t.Fatal("configuração reconfigurada não aparece em ListMensalidadeConfiguracoes")
	}

	if err := service.projection.Rebuild(); err != nil {
		t.Fatalf("Rebuild não deveria falhar: %v", err)
	}
	configsRebuild, err := service.ListMensalidadeConfiguracoes(context.Background(), academia)
	if err != nil {
		t.Fatal(err)
	}
	achouRebuild := false
	for _, c := range configsRebuild {
		if c.Nivel == NivelFundamental && c.AnoAcademico == "7_ano_fundamental" && c.Valor == 6000 {
			achouRebuild = true
		}
	}
	if !achouRebuild {
		t.Fatal("após Rebuild, esperava encontrar a configuração reconfigurada (6000) em ListMensalidadeConfiguracoes")
	}
}

func TestIntegrationRemoveMesInicioCobrancaVoltaAoPadraoNatural(t *testing.T) {
	client := integrationClient(t)
	academia := mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2026_2027")
	service := NewService(client)

	mesAntes, err := service.mesInicioEfetivo(context.Background(), academia, "2026_2027", NivelFundamental)
	if err != nil {
		t.Fatal(err)
	}
	if mesAntes != 9 {
		t.Fatalf("mês natural padrão esperado = 9 (setembro), obteve %d", mesAntes)
	}

	if err := service.DefinirMesInicioCobranca(context.Background(), MesInicioCobrancaInput{
		CodigoAcademia: academia, AnoLetivo: "2026_2027", MesInicio: 11,
	}, uuid.NewString(), "academia", "127.0.0.1"); err != nil {
		t.Fatalf("DefinirMesInicioCobranca falhou: %v", err)
	}
	mesDefinido, err := service.mesInicioEfetivo(context.Background(), academia, "2026_2027", NivelFundamental)
	if err != nil {
		t.Fatal(err)
	}
	if mesDefinido != 11 {
		t.Fatalf("esperava mes_inicio=11 após definir, obteve %d", mesDefinido)
	}

	if err := service.RemoveMesInicioCobranca(context.Background(), academia, "2026_2027", uuid.NewString(), "academia", "127.0.0.1"); err != nil {
		t.Fatalf("RemoveMesInicioCobranca não deveria falhar: %v", err)
	}

	mesDepois, err := service.mesInicioEfetivo(context.Background(), academia, "2026_2027", NivelFundamental)
	if err != nil {
		t.Fatal(err)
	}
	if mesDepois != 9 {
		t.Fatalf("após remoção, esperava voltar ao mês natural (9), obteve %d", mesDepois)
	}

	aggID := mensalidadeAggregateID(academia)
	var definidos, removidos int
	if err := client.DB().QueryRow(`SELECT COUNT(*) FROM spuri_ledger WHERE aggregate_id=$1 AND event_type='MesInicioCobrancaDefinido'`, aggID).Scan(&definidos); err != nil {
		t.Fatal(err)
	}
	if definidos != 1 {
		t.Fatalf("esperava 1 evento MesInicioCobrancaDefinido preservado, obteve %d", definidos)
	}
	if err := client.DB().QueryRow(`SELECT COUNT(*) FROM spuri_ledger WHERE aggregate_id=$1 AND event_type='MesInicioCobrancaRemovido'`, aggID).Scan(&removidos); err != nil {
		t.Fatal(err)
	}
	if removidos != 1 {
		t.Fatalf("esperava 1 evento MesInicioCobrancaRemovido, obteve %d", removidos)
	}

	if err := service.RemoveMesInicioCobranca(context.Background(), academia, "2026_2027", uuid.NewString(), "academia", "127.0.0.1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("segunda remoção deveria falhar com ErrNotFound, obteve: %v", err)
	}

	if err := service.DefinirMesInicioCobranca(context.Background(), MesInicioCobrancaInput{
		CodigoAcademia: academia, AnoLetivo: "2026_2027", MesInicio: 1,
	}, uuid.NewString(), "academia", "127.0.0.1"); err != nil {
		t.Fatalf("redefinição após remoção não deveria falhar: %v", err)
	}
	mesFinal, err := service.mesInicioEfetivo(context.Background(), academia, "2026_2027", NivelFundamental)
	if err != nil {
		t.Fatal(err)
	}
	if mesFinal != 1 {
		t.Fatalf("esperava mes_inicio=1 após redefinição, obteve %d", mesFinal)
	}
}
