package finance

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"spuri/internal/db"
)

func mensalidadeCodigo() string { return "MEN" + uuid.NewString()[:8] }

func seedMensalidadeAcademia(t *testing.T, client *db.Client, codigo, natureza, nivel, anoLetivo string) {
	t.Helper()
	anos := any(nil)
	nivelEscolar := "fundamental"
	if nivel == "superior" {
		nivelEscolar = ""
	} else if nivel == "medio" {
		nivelEscolar = "medio"
	} else {
		anos = `["6_ano_fundamental","7_ano_fundamental"]`
	}
	_, err := client.DB().Exec(`INSERT INTO projection_academias
		(id,nivel,nome,nif,codigo_academia,senha_hash,provincia,endereco,nivel_escolar,status,cursos,anos_academicos,type,ano_letivo,created_at)
		VALUES ($1,$2,$3,$4,$5,'hash','LUA','endereco',NULLIF($6,''),'ativo','[]'::jsonb,$7::jsonb,$8,$9,CURRENT_TIMESTAMP)`,
		uuid.New(), map[bool]string{true: "superior", false: "escola"}[nivel == "superior"], "Academia de teste", strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, uuid.NewString())[:10], codigo, nivelEscolar, anos, natureza, anoLetivo)
	if err != nil {
		t.Fatal(err)
	}
}

func seedMensalidadeCurso(t *testing.T, client *db.Client, codigoAcademia string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := client.DB().Exec(`INSERT INTO projection_cursos
		(id,nome,type,anos_academicos,codigo_academia,status,modelo)
		VALUES ($1,'Curso de teste','medio','["1_ano_medio","2_ano_medio","3_ano_medio"]'::jsonb,$2,'ativo','liceu')`, id, codigoAcademia)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func seedMensalidadeTurma(t *testing.T, client *db.Client, codigoAcademia, codigoTurma, ano, estudante string, curso *uuid.UUID) {
	t.Helper()
	cursoID := any(nil)
	if curso != nil {
		cursoID = *curso
	}
	historico := `{}`
	if ano != "" {
		historico = `{"` + ano + `":["` + estudante + `"]}`
	}
	_, err := client.DB().Exec(`INSERT INTO projection_turmas
		(id,codigo_turma,codigo_academia,nivel,curso_id,turno,estudantes,historico_estudantes_ano_letivo,status,created_at)
		VALUES ($1,$2,$3,$4,$5,'manha','[]'::jsonb,$6::jsonb,'ativo',CURRENT_TIMESTAMP)`,
		uuid.New(), codigoTurma, codigoAcademia, anoAcademicoDaTurma(ano, curso), cursoID, historico)
	if err != nil {
		t.Fatal(err)
	}
}

func anoAcademicoDaTurma(anoLetivo string, curso *uuid.UUID) string {
	if curso != nil {
		if anoLetivo == "2026_2027" {
			return "2_ano_medio"
		}
		return "1_ano_medio"
	}
	if anoLetivo == "2026_2027" {
		return "7_ano_fundamental"
	}
	return "6_ano_fundamental"
}

func seedMensalidadeConfiguracao(t *testing.T, client *db.Client, academia, nivel, ano string, curso *uuid.UUID, valor float64, fim int, vigente time.Time) {
	t.Helper()
	cursoID := any(nil)
	if curso != nil {
		cursoID = *curso
	}
	// sequencia é NOT NULL sem default (migração 116, tarefa 78) — em
	// produção vem sempre de spuri_ledger.id (a única fonte real de ordem
	// cronológica do sistema); aqui, para não inserir uma linha de ledger
	// só para popular este seed direto de teste, reserva-se um valor da
	// mesma sequence real (spuri_ledger_id_seq), que continua
	// monotonicamente crescente e nunca colide com sequencia já usada por
	// eventos reais no mesmo banco de teste.
	_, err := client.DB().Exec(`INSERT INTO financeiro_mensalidade_configuracoes
		(event_id,aggregate_id,codigo_academia,nivel,ano_academico,curso_id,valor,mes_fim_cobranca,vigente_em,sequencia)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,nextval('spuri_ledger_id_seq'))`, uuid.New(), uuid.New(), academia, nivel, ano, cursoID, valor, fim, vigente)
	if err != nil {
		t.Fatal(err)
	}
}

func mensalidadePorMes(t *testing.T, valores []MensalidadeMesView, academia, ano string, mes int) MensalidadeMesView {
	t.Helper()
	for _, valor := range valores {
		if valor.CodigoAcademia == academia && valor.AnoLetivo == ano && valor.Mes == mes {
			return valor
		}
	}
	t.Fatalf("mensalidade %s/%s/%d nao encontrada: %#v", academia, ano, mes, valores)
	return MensalidadeMesView{}
}

func TestIntegrationMensalidadeResolvePrecoHistorico(t *testing.T) {
	client := integrationClient(t)
	ctx := context.Background()
	academia := mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2025_2026")
	seedMensalidadeTurma(t, client, academia, "T-HIST", "2025_2026", "EST-HIST", nil)
	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "6_ano_fundamental", nil, 1000, 7, time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC))
	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "6_ano_fundamental", nil, 1500, 7, time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC))

	valores, err := NewService(client).ListMensalidades(ctx, "EST-HIST", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := mensalidadePorMes(t, valores, academia, "2025_2026", 9).Valor; got != 1000 {
		t.Fatalf("setembro historico = %.2f, queria 1000", got)
	}
	if got := mensalidadePorMes(t, valores, academia, "2025_2026", 1).Valor; got != 1500 {
		t.Fatalf("janeiro apos nova configuracao = %.2f, queria 1500", got)
	}
}

func TestIntegrationMensalidadePrimeiraConfiguracaoRetroageSemReescreverHistorico(t *testing.T) {
	client := integrationClient(t)
	ctx := context.Background()
	academia := mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2025_2026")
	seedMensalidadeTurma(t, client, academia, "T-RETRO", "2025_2026", "EST-RETRO", nil)
	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "6_ano_fundamental", nil, 1000, 7, time.Date(2025, 11, 1, 0, 0, 0, 0, time.UTC))

	valores, err := NewService(client).ListMensalidades(ctx, "EST-RETRO", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := mensalidadePorMes(t, valores, academia, "2025_2026", 9).Valor; got != 1000 {
		t.Fatalf("primeira configuracao nao retroagiu para setembro: %.2f", got)
	}

	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "6_ano_fundamental", nil, 1500, 7, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	valores, err = NewService(client).ListMensalidades(ctx, "EST-RETRO", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := mensalidadePorMes(t, valores, academia, "2025_2026", 9).Valor; got != 1000 {
		t.Fatalf("setembro historico foi reescrito: %.2f", got)
	}
	if got := mensalidadePorMes(t, valores, academia, "2025_2026", 1).Valor; got != 1500 {
		t.Fatalf("janeiro apos reconfiguracao = %.2f, queria 1500", got)
	}
}

func TestIntegrationMensalidadeMantemAnoAcademicoHistorico(t *testing.T) {
	client := integrationClient(t)
	academia := mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2026_2027")
	seedMensalidadeTurma(t, client, academia, "T-6", "2025_2026", "EST-ANO", nil)
	seedMensalidadeTurma(t, client, academia, "T-7", "2026_2027", "EST-ANO", nil)
	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "6_ano_fundamental", nil, 600, 7, time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC))
	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "7_ano_fundamental", nil, 700, 7, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))
	valores, err := NewService(client).ListMensalidades(context.Background(), "EST-ANO", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := mensalidadePorMes(t, valores, academia, "2025_2026", 9).Valor; got != 600 {
		t.Fatalf("mes antigo = %.2f, queria valor do 6 ano", got)
	}
	if got := mensalidadePorMes(t, valores, academia, "2026_2027", 9).Valor; got != 700 {
		t.Fatalf("mes atual = %.2f, queria valor do 7 ano", got)
	}
}

func TestIntegrationMensalidadeMantemCursoHistorico(t *testing.T) {
	client := integrationClient(t)
	academia := mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, academia, "private", "medio", "2026_2027")
	cursoAntigo := seedMensalidadeCurso(t, client, academia)
	cursoAtual := seedMensalidadeCurso(t, client, academia)
	seedMensalidadeTurma(t, client, academia, "T-CURSO-1", "2025_2026", "EST-CURSO", &cursoAntigo)
	seedMensalidadeTurma(t, client, academia, "T-CURSO-2", "2026_2027", "EST-CURSO", &cursoAtual)
	seedMensalidadeConfiguracao(t, client, academia, NivelMedio, "1_ano_medio", &cursoAntigo, 800, 7, time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC))
	seedMensalidadeConfiguracao(t, client, academia, NivelMedio, "2_ano_medio", &cursoAtual, 900, 7, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))
	valores, err := NewService(client).ListMensalidades(context.Background(), "EST-CURSO", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := mensalidadePorMes(t, valores, academia, "2025_2026", 9).Valor; got != 800 {
		t.Fatalf("curso antigo = %.2f, queria 800", got)
	}
}

func TestIntegrationMensalidadeMantemAcademiaHistoricaAposTransferencia(t *testing.T) {
	client := integrationClient(t)
	primeira, segunda := mensalidadeCodigo(), mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, primeira, "private", "fundamental", "2025_2026")
	seedMensalidadeAcademia(t, client, segunda, "private", "fundamental", "2026_2027")
	seedMensalidadeTurma(t, client, primeira, "T-ORIGEM", "2025_2026", "EST-TRANS", nil)
	seedMensalidadeTurma(t, client, segunda, "T-DESTINO", "2026_2027", "EST-TRANS", nil)
	seedMensalidadeConfiguracao(t, client, primeira, NivelFundamental, "6_ano_fundamental", nil, 1100, 7, time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC))
	seedMensalidadeConfiguracao(t, client, segunda, NivelFundamental, "7_ano_fundamental", nil, 2200, 7, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))
	valores, err := NewService(client).ListMensalidades(context.Background(), "EST-TRANS", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := mensalidadePorMes(t, valores, primeira, "2025_2026", 9).Valor; got != 1100 {
		t.Fatalf("pendencia da academia anterior = %.2f, queria 1100", got)
	}
}

func TestIntegrationMensalidadeValidaGranularidade(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	publica, privada := mensalidadeCodigo(), mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, publica, "public", "fundamental", "2026_2027")
	seedMensalidadeAcademia(t, client, privada, "private", "medio", "2026_2027")
	curso := seedMensalidadeCurso(t, client, privada)
	if err := service.validateConfiguracaoMensalidade(context.Background(), &MensalidadeConfiguracaoInput{CodigoAcademia: publica, Nivel: NivelFundamental, AnoAcademico: "6_ano_fundamental", Valor: 10, MesFimCobranca: 6, ModoVigencia: ModoVigenciaAPartirDaAtualizacao}); err == nil {
		t.Fatal("academia publica aceitou configuracao")
	}
	if err := service.validateConfiguracaoMensalidade(context.Background(), &MensalidadeConfiguracaoInput{CodigoAcademia: privada, Nivel: NivelMedio, AnoAcademico: "1_ano_medio", Valor: 10, MesFimCobranca: 6, ModoVigencia: ModoVigenciaAPartirDaAtualizacao}); err == nil {
		t.Fatal("curso obrigatorio no medio foi aceite sem curso_id")
	}
	cursoTexto := curso.String()
	if err := service.validateConfiguracaoMensalidade(context.Background(), &MensalidadeConfiguracaoInput{CodigoAcademia: privada, Nivel: NivelMedio, AnoAcademico: "1_ano_medio", CursoID: &cursoTexto, Valor: 10, MesFimCobranca: 6, ModoVigencia: ModoVigenciaAPartirDaAtualizacao}); err != nil {
		t.Fatalf("curso medio oferecido foi rejeitado: %v", err)
	}
	if err := service.validateConfiguracaoMensalidade(context.Background(), &MensalidadeConfiguracaoInput{CodigoAcademia: privada, Nivel: NivelMedio, AnoAcademico: "9_ano_medio", CursoID: &cursoTexto, Valor: 10, MesFimCobranca: 6, ModoVigencia: ModoVigenciaAPartirDaAtualizacao}); err == nil {
		t.Fatal("ano nao oferecido foi aceite")
	}
}

func TestIntegrationMensalidadeValidaMesFim(t *testing.T) {
	client := integrationClient(t)
	academia := mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2026_2027")
	service := NewService(client)
	for _, fim := range []int{6, 7} {
		if err := service.validateConfiguracaoMensalidade(context.Background(), &MensalidadeConfiguracaoInput{CodigoAcademia: academia, Nivel: NivelFundamental, AnoAcademico: "6_ano_fundamental", Valor: 10, MesFimCobranca: fim, ModoVigencia: ModoVigenciaAPartirDaAtualizacao}); err != nil {
			t.Fatalf("mes_fim %d foi rejeitado: %v", fim, err)
		}
	}
	for _, fim := range []int{5, 8} {
		if err := service.validateConfiguracaoMensalidade(context.Background(), &MensalidadeConfiguracaoInput{CodigoAcademia: academia, Nivel: NivelFundamental, AnoAcademico: "6_ano_fundamental", Valor: 10, MesFimCobranca: fim, ModoVigencia: ModoVigenciaAPartirDaAtualizacao}); err == nil {
			t.Fatalf("mes_fim %d foi aceite", fim)
		}
	}
}

func TestIntegrationMensalidadeMesInicioEValidadePorAno(t *testing.T) {
	client := integrationClient(t)
	academia := mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2026_2027")
	seedMensalidadeTurma(t, client, academia, "T-INICIO-ANT", "2025_2026", "EST-INICIO", nil)
	seedMensalidadeTurma(t, client, academia, "T-INICIO-ATU", "2026_2027", "EST-INICIO", nil)
	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "6_ano_fundamental", nil, 100, 7, time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC))
	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "7_ano_fundamental", nil, 100, 7, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))
	_, err := client.DB().Exec(`INSERT INTO financeiro_mensalidade_inicio_cobranca (event_id,aggregate_id,codigo_academia,ano_letivo,mes_inicio,definido_em) VALUES ($1,$2,$3,'2025_2026',1,CURRENT_TIMESTAMP)`, uuid.New(), uuid.New(), academia)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(client)
	if err := service.validateMesInicioCobranca(context.Background(), &MesInicioCobrancaInput{CodigoAcademia: academia, AnoLetivo: "2026_2027", MesInicio: 8}); err == nil {
		t.Fatal("mes_inicio anterior ao periodo natural foi aceite")
	}
	if err := service.validateMesInicioCobranca(context.Background(), &MesInicioCobrancaInput{CodigoAcademia: academia, AnoLetivo: "2026_2027", MesInicio: 10}); err != nil {
		t.Fatalf("mes_inicio de outubro deve ser aceito dentro da ordem do ano letivo: %v", err)
	}
	valores, err := service.ListMensalidades(context.Background(), "EST-INICIO", nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = mensalidadePorMes(t, valores, academia, "2025_2026", 1)
	_ = mensalidadePorMes(t, valores, academia, "2026_2027", 9)
	meses2026 := 0
	for _, valor := range valores {
		if valor.CodigoAcademia == academia && valor.AnoLetivo == "2026_2027" {
			meses2026++
		}
	}
	if meses2026 != 11 {
		t.Fatalf("mensalidades de 2026_2027 = %d, queria 11", meses2026)
	}
}

func TestIntegrationMensalidadeAnularEReativar(t *testing.T) {
	client := integrationClient(t)
	academia := mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2025_2026")
	seedMensalidadeTurma(t, client, academia, "T-ANULAR", "2025_2026", "EST-ANULAR", nil)
	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "6_ano_fundamental", nil, 100, 7, time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC))
	service := NewService(client)
	in := ObrigacaoMensalidadeInput{CodigoEstudante: "EST-ANULAR", CodigoAcademia: academia, AnoLetivo: "2025_2026", Meses: []int{9}}
	if err := service.AnularObrigacoesMensalidade(context.Background(), in, uuid.NewString(), "academia", "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	if err := service.ReativarObrigacoesMensalidade(context.Background(), in, uuid.NewString(), "academia", "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	if err := service.ReativarObrigacoesMensalidade(context.Background(), in, uuid.NewString(), "academia", "127.0.0.1"); err == nil {
		t.Fatal("reativacao de mensalidade nao anulada foi aceite")
	}
	_, err := client.DB().Exec(`INSERT INTO financeiro_mensalidade_obrigacoes_eventos (event_id,aggregate_id,codigo_estudante,codigo_academia,ano_letivo,mes,tipo,ocorrido_em) VALUES ($1,$2,'EST-ANULAR',$3,'2025_2026',9,'paga',CURRENT_TIMESTAMP)`, uuid.New(), uuid.New(), academia)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ReativarObrigacoesMensalidade(context.Background(), in, uuid.NewString(), "academia", "127.0.0.1"); err == nil {
		t.Fatal("reativacao de mensalidade paga foi aceite")
	}
}

// TestIntegrationMensalidadeComCobrancaFalhadaNaAppyPayPermiteNovaTentativa
// cobre um bug real de produção: mensalidadeTemCobrancaAberta (usada por
// IniciarPagamentoMensalidades para bloquear uma segunda tentativa
// enquanto já existe uma cobrança "em aberto" para o mesmo mês) só
// reconhecia os estados terminais locais ("cancelada", "falhada") e
// "Success" — uma cobrança com o estado bruto "Failed" devolvido pela
// própria AppyPay (recusa no processador, ver docs/Parceiros e
// integrações/AppyPay Documentação.md) nunca entrava nessa lista e por
// isso ficava "presa" como em aberto para sempre, bloqueando
// indefinidamente qualquer nova tentativa de pagamento do mesmo mês —
// mesmo a cobrança anterior já tendo definitivamente falhado no provedor.
func TestIntegrationMensalidadeComCobrancaFalhadaNaAppyPayPermiteNovaTentativa(t *testing.T) {
	t.Setenv("APPYPAY_RESOURCE", "integration-resource")
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	estudante := "EST-RETRY-" + uuid.NewString()[:8]
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2025_2026")
	seedMensalidadeTurma(t, client, academia, "T-RETRY", "2025_2026", estudante, nil)
	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "6_ano_fundamental", nil, 1000, 7, time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC))
	if _, err := client.DB().Exec(`UPDATE financeiro_mensalidade_configuracoes SET metodos_pagamento='{GPO}' WHERE codigo_academia=$1`, academia); err != nil {
		t.Fatal(err)
	}
	configureIntegrationCredential(t, service, ContextoAcademia, academia)
	transport := &appyPayMockTransport{status: "Pending"}
	service.SetHTTPClient(&http.Client{Transport: transport})

	pendentes, err := service.ListMensalidades(ctx, estudante, &academia)
	if err != nil {
		t.Fatal(err)
	}
	if len(pendentes) == 0 {
		t.Fatal("esperava pelo menos uma mensalidade pendente")
	}
	alvo := pendentes[0]
	meses := []MensalidadeSelecaoMes{{AnoLetivo: alvo.AnoLetivo, Mes: alvo.Mes}}

	// 1a tentativa: cria a cobrança (POST /charges do mock sempre devolve
	// "Pending" — vira EstadoCobrancaAguardandoPagamento após a
	// normalização desta tarefa).
	primeira, err := service.IniciarPagamentoMensalidades(ctx, MensalidadePagamentoInput{
		CodigoEstudante: estudante, CodigoAcademia: academia, Meses: meses,
		MetodoPagamento: "GPO", Telefone: "923000000",
	}, estudante, "estudante", "127.0.0.1")
	if err != nil {
		t.Fatalf("1a tentativa de pagamento falhou: %v", err)
	}
	if primeira.Charge.Status != EstadoCobrancaAguardandoPagamento {
		t.Fatalf("esperava status=%q logo após criar a cobrança, obteve %q", EstadoCobrancaAguardandoPagamento, primeira.Charge.Status)
	}

	// Uma 2a tentativa imediata (sem a AppyPay ter resolvido a 1a) deve
	// continuar bloqueada — comportamento que já existia antes desta
	// tarefa e continua correto: a cobrança está aberta de verdade.
	if _, err := service.IniciarPagamentoMensalidades(ctx, MensalidadePagamentoInput{
		CodigoEstudante: estudante, CodigoAcademia: academia, Meses: meses,
		MetodoPagamento: "GPO", Telefone: "923000000",
	}, estudante, "estudante", "127.0.0.1"); err == nil {
		t.Fatal("esperava bloqueio de 2a tentativa enquanto a 1a cobrança ainda está aguardando pagamento")
	}

	// A AppyPay resolve a 1a cobrança como Failed (recusada no
	// processador) — o Spuri descobre isso numa consulta, exatamente como
	// aconteceria via webhook ou verificação manual.
	transport.status = "Failed"
	consultada, err := service.ConsultCharge(ctx, ContextoAcademia, academia, primeira.Charge.ID.String(), estudante, "estudante", "127.0.0.1")
	if err != nil {
		t.Fatalf("ConsultCharge falhou: %v", err)
	}
	if consultada.Status != "Failed" {
		t.Fatalf("esperava status=Failed após a consulta, obteve %q", consultada.Status)
	}

	// A 2a tentativa agora deve ser aceite: a cobrança anterior já está
	// definitivamente resolvida (Failed), não "em aberto" — este é
	// exatamente o bug corrigido nesta tarefa.
	segunda, err := service.IniciarPagamentoMensalidades(ctx, MensalidadePagamentoInput{
		CodigoEstudante: estudante, CodigoAcademia: academia, Meses: meses,
		MetodoPagamento: "GPO", Telefone: "923000000",
	}, estudante, "estudante", "127.0.0.1")
	if err != nil {
		t.Fatalf("esperava que a 2a tentativa fosse aceite após a 1a cobrança ter falhado no provedor, obteve erro: %v", err)
	}
	if segunda.Charge.ID == primeira.Charge.ID {
		t.Fatal("a 2a tentativa deveria ter criado uma cobrança nova, não reutilizado a 1a")
	}
}

func TestIntegrationMensalidadeConsultaRespeitaAcademia(t *testing.T) {
	client := integrationClient(t)
	primeira, segunda := mensalidadeCodigo(), mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, primeira, "private", "fundamental", "2025_2026")
	seedMensalidadeAcademia(t, client, segunda, "private", "fundamental", "2025_2026")
	seedMensalidadeTurma(t, client, primeira, "T-CONS-1", "2025_2026", "EST-CONS", nil)
	seedMensalidadeTurma(t, client, segunda, "T-CONS-2", "2025_2026", "EST-CONS", nil)
	seedMensalidadeConfiguracao(t, client, primeira, NivelFundamental, "6_ano_fundamental", nil, 100, 7, time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC))
	seedMensalidadeConfiguracao(t, client, segunda, NivelFundamental, "6_ano_fundamental", nil, 200, 7, time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC))
	service := NewService(client)
	somentePrimeira, err := service.ListMensalidades(context.Background(), "EST-CONS", &primeira)
	if err != nil {
		t.Fatal(err)
	}
	for _, valor := range somentePrimeira {
		if valor.CodigoAcademia != primeira {
			t.Fatalf("consulta limitada retornou academia externa: %#v", valor)
		}
	}
	todas, err := service.ListMensalidades(context.Background(), "EST-CONS", nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = mensalidadePorMes(t, todas, primeira, "2025_2026", 9)
	_ = mensalidadePorMes(t, todas, segunda, "2025_2026", 9)
}
