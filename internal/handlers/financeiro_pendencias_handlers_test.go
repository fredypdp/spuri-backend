package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"spuri/internal/db"
	"spuri/internal/finance"
)

// seedAcademiaEscolarPrivadaComTurma cria a academia + turma mínimas
// necessárias para exercitar o escopo de mensalidade (turma_id, curso_id,
// ano_academico, ano_letivo) usado por PendenciasSemCobranca e pelos novos
// filtros de ListCobrancas/ListCobrancasEstudante — ver tarefa 58.
func seedAcademiaEscolarPrivadaComTurma(t *testing.T, client *db.Client, academia, codigoTurma, anoLetivo, anoAcademico, estudante string) {
	t.Helper()
	nif := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, uuid.NewString())
	if len(nif) < 10 {
		nif = nif + strings.Repeat("0", 10-len(nif))
	}
	nif = nif[:10]
	if _, err := client.DB().Exec(`INSERT INTO projection_academias
		(id,nivel,nome,nif,codigo_academia,senha_hash,provincia,endereco,nivel_escolar,status,cursos,anos_academicos,type,ano_letivo,created_at)
		VALUES ($1,'escola','Academia HTTP teste',$2,$3,'hash','LUA','endereco','fundamental','ativo','[]'::jsonb,$4::jsonb,'private',$5,CURRENT_TIMESTAMP)`,
		uuid.New(), nif, academia, `["`+anoAcademico+`"]`, anoLetivo); err != nil {
		t.Fatal(err)
	}
	historico := `{"` + anoLetivo + `":["` + estudante + `"]}`
	if _, err := client.DB().Exec(`INSERT INTO projection_turmas
		(id,codigo_turma,codigo_academia,nivel,curso_id,turno,estudantes,historico_estudantes_ano_letivo,status,created_at)
		VALUES ($1,$2,$3,$4,NULL,'manha','[]'::jsonb,$5::jsonb,'ativo',CURRENT_TIMESTAMP)`,
		uuid.New(), codigoTurma, academia, anoAcademico, historico); err != nil {
		t.Fatal(err)
	}
}

func seedMensalidadeConfigParaHTTP(t *testing.T, client *db.Client, academia, anoAcademico string, valor float64) {
	t.Helper()
	if _, err := client.DB().Exec(`INSERT INTO financeiro_mensalidade_configuracoes
		(event_id,aggregate_id,codigo_academia,nivel,ano_academico,curso_id,valor,mes_fim_cobranca,vigente_em)
		VALUES ($1,$2,$3,'fundamental',$4,NULL,$5,7,'2026-01-01')`,
		uuid.New(), uuid.New(), academia, anoAcademico, valor); err != nil {
		t.Fatal(err)
	}
}

// TestIntegrationListarCobrancasAppyPayComEscopoRetornaPendenciaSintetica
// cobre, no nível HTTP, o problema original da tarefa 58 (um estudante que
// nunca tentou nenhuma cobrança de mensalidade é invisível para a academia
// em GET /financeiro/cobrancas a menos que ela informe um filtro de
// escopo), já na forma unificada desta tarefa: quando ano_letivo é
// informado, a pendência aparece dentro de "pagamentos", com
// status="pendente" — o único sinal, desde esta tarefa, de que não existe
// nenhuma cobrança real por trás do item.
func TestIntegrationListarCobrancasAppyPayComEscopoRetornaPendenciaSintetica(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := integrationFinanceClient(t)
	academia := "PND" + strings.ReplaceAll(uuid.NewString(), "-", "")[:7]
	estudante := "ESTPND1"
	seedAcademiaEscolarPrivadaComTurma(t, client, academia, "T-HTTP-PND", "2026_2027", "7_ano_fundamental", estudante)
	seedMensalidadeConfigParaHTTP(t, client, academia, "7_ano_fundamental", 15000)

	previousService := FinanceiroService
	FinanceiroService = finance.NewService(client)
	t.Cleanup(func() { FinanceiroService = previousService })

	call := func(query string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/financeiro/cobrancas?"+query, nil)
		ctx.Set("dbClient", client)
		ctx.Set("user_id", uuid.New())
		ctx.Set("user_type", "academia")
		ctx.Set("codigo_academia", academia)
		ListarCobrancasAppyPay(ctx)
		return recorder
	}

	// Sem filtro de escopo: nenhuma cobrança foi criada ainda, e
	// pendências não são computadas (evita varredura sem limite) — a
	// lista unificada fica vazia.
	semEscopo := call("")
	if semEscopo.Code != http.StatusOK {
		t.Fatalf("sem escopo = %d: %s", semEscopo.Code, semEscopo.Body.String())
	}
	var bodySemEscopo struct {
		Pagamentos []finance.PagamentoResumo `json:"pagamentos"`
		TotalGeral int                       `json:"total_geral"`
	}
	if err := json.Unmarshal(semEscopo.Body.Bytes(), &bodySemEscopo); err != nil {
		t.Fatal(err)
	}
	if len(bodySemEscopo.Pagamentos) != 0 || bodySemEscopo.TotalGeral != 0 {
		t.Fatalf("sem filtro de escopo, esperava lista vazia (nenhuma cobrança real, pendências não computadas): %s", semEscopo.Body.String())
	}

	// Com ano_letivo: o estudante nunca tentou nenhuma cobrança, então
	// TODOS os meses pendentes dele devem vir em "pagamentos", com
	// status="pendente".
	comEscopo := call("ano_letivo=2026_2027")
	if comEscopo.Code != http.StatusOK {
		t.Fatalf("com escopo = %d: %s", comEscopo.Code, comEscopo.Body.String())
	}
	var body struct {
		Pagamentos []finance.PagamentoResumo `json:"pagamentos"`
	}
	if err := json.Unmarshal(comEscopo.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Pagamentos) == 0 {
		t.Fatalf("esperava pagamentos não vazio: %s", comEscopo.Body.String())
	}
	for _, p := range body.Pagamentos {
		if p.Status != finance.EstadoPendente {
			t.Fatalf("esperava só pendências sintéticas (status=%q) nesta academia (nenhuma cobrança real criada): %#v", finance.EstadoPendente, p)
		}
		if p.CodigoEstudante != estudante {
			t.Fatalf("pendência de outro estudante inesperada: %#v", p)
		}
		if p.AtualizadoEm != nil {
			t.Fatalf("pendência sintética não deveria ter atualizado_em: %#v", p)
		}
	}
}

// TestIntegrationListarCobrancasAppyPayFiltroEstadoExcluiPendenciasSinteticas
// reproduz, no nível HTTP, o bug relatado em produção: uma academia
// consultou GET /financeiro/cobrancas?...&estado=Failed&tipo=mensalidade&
// ano_letivo=...&mes=... e recebeu de volta todos os pagamentos do mês
// (não só os Failed), porque a computação de pendências sintéticas nunca
// olhava para o filtro de estado antes desta tarefa — toda pendência é
// sempre status="pendente", nunca "Failed", mas entrava na lista do mesmo
// jeito. Com o mesmo escopo usado no relato original (ano_letivo e mes),
// nenhum estudante tentou nenhuma cobrança ainda: o resultado filtrado por
// estado=Failed deve vir vazio, e não "todos os pagamentos".
func TestIntegrationListarCobrancasAppyPayFiltroEstadoExcluiPendenciasSinteticas(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := integrationFinanceClient(t)
	academia := "BUG" + strings.ReplaceAll(uuid.NewString(), "-", "")[:7]
	estudante := "ESTBUG1"
	seedAcademiaEscolarPrivadaComTurma(t, client, academia, "T-HTTP-BUG", "2026_2027", "7_ano_fundamental", estudante)
	seedMensalidadeConfigParaHTTP(t, client, academia, "7_ano_fundamental", 15000)

	previousService := FinanceiroService
	FinanceiroService = finance.NewService(client)
	t.Cleanup(func() { FinanceiroService = previousService })

	call := func(query string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/financeiro/cobrancas?"+query, nil)
		ctx.Set("dbClient", client)
		ctx.Set("user_id", uuid.New())
		ctx.Set("user_type", "academia")
		ctx.Set("codigo_academia", academia)
		ListarCobrancasAppyPay(ctx)
		return recorder
	}

	// Confirma primeiro que, sem o filtro de estado, a pendência sintética
	// aparece normalmente (mesma cobertura do teste anterior) — isolando
	// que a mudança de comportamento vem exclusivamente do filtro estado.
	semFiltro := call("ano_letivo=2026_2027&mes=3")
	var bodySemFiltro struct {
		Pagamentos []finance.PagamentoResumo `json:"pagamentos"`
		TotalGeral int                       `json:"total_geral"`
	}
	if err := json.Unmarshal(semFiltro.Body.Bytes(), &bodySemFiltro); err != nil {
		t.Fatal(err)
	}
	if bodySemFiltro.TotalGeral == 0 {
		t.Fatalf("sem filtro de estado, esperava ver a pendência sintética de março: %s", semFiltro.Body.String())
	}

	// Reproduz exatamente a query relatada (adaptada ao escopo deste
	// teste): estado=Failed nunca deveria trazer pendências sintéticas,
	// já que nenhuma delas tem esse status.
	comFiltro := call("estado=Failed&tipo=mensalidade&ano_letivo=2026_2027&mes=3&limit=30&offset=0")
	if comFiltro.Code != http.StatusOK {
		t.Fatalf("com filtro estado=Failed = %d: %s", comFiltro.Code, comFiltro.Body.String())
	}
	var bodyComFiltro struct {
		Pagamentos []finance.PagamentoResumo `json:"pagamentos"`
		TotalGeral int                       `json:"total_geral"`
	}
	if err := json.Unmarshal(comFiltro.Body.Bytes(), &bodyComFiltro); err != nil {
		t.Fatal(err)
	}
	if bodyComFiltro.TotalGeral != 0 || len(bodyComFiltro.Pagamentos) != 0 {
		t.Fatalf("estado=Failed não deveria trazer nenhuma pendência sintética (nenhuma cobrança real existe nesta academia): %s", comFiltro.Body.String())
	}

	// tipo=matricula (excluindo mensalidade) também deve excluir as
	// pendências, já que toda pendência sintética é sempre mensalidade.
	comTipoMatricula := call("tipo=matricula&ano_letivo=2026_2027&mes=3")
	var bodyTipoMatricula struct {
		TotalGeral int `json:"total_geral"`
	}
	if err := json.Unmarshal(comTipoMatricula.Body.Bytes(), &bodyTipoMatricula); err != nil {
		t.Fatal(err)
	}
	if bodyTipoMatricula.TotalGeral != 0 {
		t.Fatalf("tipo=matricula não deveria trazer pendências sintéticas de mensalidade: %s", comTipoMatricula.Body.String())
	}

	// E, de volta, estado=pendente (o próprio valor das pendências)
	// continua trazendo-as normalmente.
	comEstadoPendente := call("estado=pendente&ano_letivo=2026_2027&mes=3")
	var bodyEstadoPendente struct {
		TotalGeral int `json:"total_geral"`
	}
	if err := json.Unmarshal(comEstadoPendente.Body.Bytes(), &bodyEstadoPendente); err != nil {
		t.Fatal(err)
	}
	if bodyEstadoPendente.TotalGeral == 0 {
		t.Fatalf("estado=pendente deveria continuar trazendo a pendência sintética: %s", comEstadoPendente.Body.String())
	}
}

// TestIntegrationConsultarCobrancasEstudanteIncluiPendenciaSintetica cobre,
// no nível HTTP, a versão por estudante (sempre calculada, sem exigir
// filtro de escopo): a própria academia, consultando o histórico de UM
// estudante específico, já enxerga dentro de "pagamentos" os meses que ele
// deve e nunca tentou pagar, marcados com status="pendente".
func TestIntegrationConsultarCobrancasEstudanteIncluiPendenciaSintetica(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := integrationFinanceClient(t)
	academia := "PNDE" + strings.ReplaceAll(uuid.NewString(), "-", "")[:6]
	estudante := "ESTPND2"
	seedAcademiaEscolarPrivadaComTurma(t, client, academia, "T-HTTP-PNDE", "2026_2027", "7_ano_fundamental", estudante)
	seedMensalidadeConfigParaHTTP(t, client, academia, "7_ano_fundamental", 15000)
	seedEstudanteParaCobrancas(t, client, estudante, academia)

	previousService := FinanceiroService
	FinanceiroService = finance.NewService(client)
	t.Cleanup(func() { FinanceiroService = previousService })

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/financeiro/cobrancas/estudante/"+estudante, nil)
	ctx.Params = gin.Params{{Key: "codigo", Value: estudante}}
	ctx.Set("dbClient", client)
	ctx.Set("user_id", uuid.New())
	ctx.Set("user_type", "academia")
	ctx.Set("codigo_academia", academia)

	ConsultarCobrancasEstudante(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("academia consultando estudante vinculado = %d: %s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Pagamentos []finance.PagamentoResumo `json:"pagamentos"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	achouPendencia := false
	for _, p := range body.Pagamentos {
		if p.Status == finance.EstadoPendente {
			achouPendencia = true
		}
	}
	if !achouPendencia {
		t.Fatalf("esperava ao menos 1 pendência sintética em pagamentos: %s", recorder.Body.String())
	}
}

// TestIntegrationListarCobrancasAppyPayFiltraPorMes cobre, no nível HTTP, o
// filtro mes (tarefa 60): combinado com ano_letivo, restringe a lista
// unificada "pagamentos" a um único mês de calendário — é este par de
// parâmetros que o passo final do drill-down do frontend usa.
func TestIntegrationListarCobrancasAppyPayFiltraPorMes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := integrationFinanceClient(t)
	academia := "MES" + strings.ReplaceAll(uuid.NewString(), "-", "")[:7]
	estudante := "ESTHMS1"
	seedAcademiaEscolarPrivadaComTurma(t, client, academia, "T-HTTP-MES", "2026_2027", "7_ano_fundamental", estudante)
	seedMensalidadeConfigParaHTTP(t, client, academia, "7_ano_fundamental", 15000)

	previousService := FinanceiroService
	FinanceiroService = finance.NewService(client)
	t.Cleanup(func() { FinanceiroService = previousService })

	call := func(query string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/financeiro/cobrancas?"+query, nil)
		ctx.Set("dbClient", client)
		ctx.Set("user_id", uuid.New())
		ctx.Set("user_type", "academia")
		ctx.Set("codigo_academia", academia)
		ListarCobrancasAppyPay(ctx)
		return recorder
	}

	comMesInvalido := call("ano_letivo=2026_2027&mes=13")
	if comMesInvalido.Code != http.StatusBadRequest {
		t.Fatalf("mes=13 deveria ser rejeitado com 400, obteve %d: %s", comMesInvalido.Code, comMesInvalido.Body.String())
	}

	comMesSetembro := call("ano_letivo=2026_2027&mes=9")
	if comMesSetembro.Code != http.StatusOK {
		t.Fatalf("mes=9 = %d: %s", comMesSetembro.Code, comMesSetembro.Body.String())
	}
	var body struct {
		Pagamentos []finance.PagamentoResumo `json:"pagamentos"`
	}
	if err := json.Unmarshal(comMesSetembro.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Pagamentos) != 1 {
		t.Fatalf("esperava exatamente 1 pagamento filtrando por mes=9, obteve %d: %s", len(body.Pagamentos), comMesSetembro.Body.String())
	}
	if len(body.Pagamentos[0].Mensalidades) != 1 || body.Pagamentos[0].Mensalidades[0].Mes != 9 {
		t.Fatalf("esperava mes=9 em mensalidades[0], obteve %#v", body.Pagamentos[0].Mensalidades)
	}
}
