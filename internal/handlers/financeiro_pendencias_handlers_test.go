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

// TestIntegrationListarCobrancasAppyPayComEscopoRetornaPendenciasSemCobranca
// cobre, no nível HTTP, o problema 1 da tarefa 58: um estudante que nunca
// tentou nenhuma cobrança de mensalidade é invisível para a academia em
// GET /financeiro/cobrancas — a menos que ela informe um filtro de escopo
// (aqui, ano_letivo), caso em que a resposta passa a incluir
// pendencias_sem_cobranca com os meses que faltam.
func TestIntegrationListarCobrancasAppyPayComEscopoRetornaPendenciasSemCobranca(t *testing.T) {
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
	// pendencias_sem_cobranca não é computado (evita varredura sem limite).
	semEscopo := call("")
	if semEscopo.Code != http.StatusOK {
		t.Fatalf("sem escopo = %d: %s", semEscopo.Code, semEscopo.Body.String())
	}
	var bodySemEscopo map[string]json.RawMessage
	if err := json.Unmarshal(semEscopo.Body.Bytes(), &bodySemEscopo); err != nil {
		t.Fatal(err)
	}
	if _, ok := bodySemEscopo["pendencias_sem_cobranca"]; ok {
		t.Fatalf("sem filtro de escopo, pendencias_sem_cobranca não deveria aparecer na resposta: %s", semEscopo.Body.String())
	}

	// Com ano_letivo: o estudante nunca tentou nenhuma cobrança, então
	// TODOS os meses pendentes dele devem vir em pendencias_sem_cobranca.
	comEscopo := call("ano_letivo=2026_2027")
	if comEscopo.Code != http.StatusOK {
		t.Fatalf("com escopo = %d: %s", comEscopo.Code, comEscopo.Body.String())
	}
	var body struct {
		Cobrancas             []any                        `json:"cobrancas"`
		PendenciasSemCobranca []finance.MensalidadeMesView `json:"pendencias_sem_cobranca"`
	}
	if err := json.Unmarshal(comEscopo.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.PendenciasSemCobranca) == 0 {
		t.Fatalf("esperava pendencias_sem_cobranca não vazio: %s", comEscopo.Body.String())
	}
	for _, p := range body.PendenciasSemCobranca {
		if p.CodigoEstudante != estudante {
			t.Fatalf("pendência de outro estudante inesperada: %#v", p)
		}
		if p.Estado != finance.EstadoPendente {
			t.Fatalf("esperava estado pendente, obteve %q", p.Estado)
		}
	}
}

// TestIntegrationConsultarCobrancasEstudanteIncluiPendenciasSemCobranca
// cobre, no nível HTTP, a versão por estudante (sempre calculada, sem
// exigir filtro de escopo): a própria academia, consultando o histórico de
// UM estudante específico, já enxerga os meses que ele deve e nunca tentou
// pagar — sem precisar de nenhum parâmetro extra.
func TestIntegrationConsultarCobrancasEstudanteIncluiPendenciasSemCobranca(t *testing.T) {
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
		PendenciasSemCobranca []finance.MensalidadeMesView `json:"pendencias_sem_cobranca"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.PendenciasSemCobranca) == 0 {
		t.Fatalf("esperava pendencias_sem_cobranca não vazio: %s", recorder.Body.String())
	}
}

// TestIntegrationListarCobrancasAppyPayFiltraPorMes cobre, no nível HTTP, o
// filtro mes (tarefa 60): combinado com ano_letivo, restringe tanto
// cobrancas quanto pendencias_sem_cobranca a um único mês de calendário —
// é este par de parâmetros que o passo final do drill-down do frontend usa.
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
		PendenciasSemCobranca []finance.MensalidadeMesView `json:"pendencias_sem_cobranca"`
	}
	if err := json.Unmarshal(comMesSetembro.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.PendenciasSemCobranca) != 1 {
		t.Fatalf("esperava exatamente 1 pendência filtrando por mes=9, obteve %d: %s", len(body.PendenciasSemCobranca), comMesSetembro.Body.String())
	}
	if body.PendenciasSemCobranca[0].Mes != 9 {
		t.Fatalf("esperava mes=9, obteve %d", body.PendenciasSemCobranca[0].Mes)
	}
}
