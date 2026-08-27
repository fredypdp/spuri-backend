package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"spuri/internal/finance"
)

// TestIntegrationListarCobrancasAppyPayFiltraPorEscopoEEstado cobre o
// Problema 1 no nível HTTP: uma academia só vê as próprias cobranças, e os
// filtros de estado/tipo funcionam através da rota real.
func TestIntegrationListarCobrancasAppyPayFiltraPorEscopoEEstado(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := integrationFinanceClient(t)
	academiaA := "LSTA" + strings.ReplaceAll(uuid.NewString(), "-", "")[:6]
	academiaB := "LSTB" + strings.ReplaceAll(uuid.NewString(), "-", "")[:6]

	insert := func(academia, status, origemCampo, origemValor string) {
		payload := map[string]any{"status": status, "amount": 250.0, "currency": "AOA", "description": "teste", "payment_method": "REF"}
		if origemCampo != "" {
			payload[origemCampo] = origemValor
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		merchant := "LST" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
		if _, err := client.DB().Exec(`INSERT INTO financeiro_cobrancas (id,merchant_transaction_id,contexto_tipo,codigo_academia,payload) VALUES ($1,$2,'academia',$3,$4)`,
			uuid.New(), merchant, academia, raw); err != nil {
			t.Fatal(err)
		}
	}
	insert(academiaA, "criada", "", "")
	insert(academiaA, "Success", "codigo_estudante", "EST-LST-1")
	insert(academiaA, "cancelada", "codigo_solicitacao", "SOL-LST-1")
	insert(academiaB, "criada", "", "")

	previousService := FinanceiroService
	FinanceiroService = finance.NewService(client)
	t.Cleanup(func() { FinanceiroService = previousService })

	call := func(academia, query string) *httptest.ResponseRecorder {
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

	var body struct {
		Pagamentos []struct {
			Origem string `json:"origem"`
			Status string `json:"status"`
		} `json:"pagamentos"`
		TotalGeral int `json:"total_geral"`
	}

	all := call(academiaA, "")
	if all.Code != http.StatusOK {
		t.Fatalf("listagem sem filtro = %d: %s", all.Code, all.Body.String())
	}
	if err := json.Unmarshal(all.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.TotalGeral != 3 {
		t.Fatalf("academia A deveria ver 3 cobranças próprias, viu %d: %s", body.TotalGeral, all.Body.String())
	}
	// A cobrança "criada" foi inserida diretamente no banco (bypassando o
	// Service), simulando uma cobrança criada ANTES desta tarefa — o
	// status bruto histórico "criada" nunca deveria voltar ao chamador
	// como está: scanCobrancaResumo normaliza a leitura para o estado
	// canônico único aguardando_pagamento, mesmo para uma linha que nunca
	// passou pelo novo código de escrita.
	var achouCriadaComoAguardando bool
	for _, p := range body.Pagamentos {
		if p.Status == "criada" {
			t.Fatalf("status bruto histórico \"criada\" vazou para a API sem normalizar: %#v", p)
		}
		if p.Status == finance.EstadoCobrancaAguardandoPagamento && p.Origem == "avulsa" {
			achouCriadaComoAguardando = true
		}
	}
	if !achouCriadaComoAguardando {
		t.Fatalf("esperava a cobrança \"criada\" normalizada para %q: %s", finance.EstadoCobrancaAguardandoPagamento, all.Body.String())
	}

	// Filtrar por estado=aguardando_pagamento (o novo nome canônico) deve
	// encontrar essa MESMA cobrança histórica, mesmo o valor gravado no
	// banco ainda sendo o bruto "criada" — é a expansão de
	// estadosCobrancaEquivalentes que garante essa equivalência no SQL.
	porNovoEstado := call(academiaA, "estado=aguardando_pagamento")
	if err := json.Unmarshal(porNovoEstado.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.TotalGeral != 1 || len(body.Pagamentos) != 1 || body.Pagamentos[0].Status != finance.EstadoCobrancaAguardandoPagamento {
		t.Fatalf("filtro por estado=aguardando_pagamento deveria encontrar a cobrança histórica \"criada\": %s", porNovoEstado.Body.String())
	}

	filtrada := call(academiaA, "estado=Success")
	if err := json.Unmarshal(filtrada.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.TotalGeral != 1 || len(body.Pagamentos) != 1 || body.Pagamentos[0].Origem != "mensalidade" {
		t.Fatalf("filtro por estado=Success deveria devolver só a cobrança de mensalidade paga: %s", filtrada.Body.String())
	}

	outraAcademia := call(academiaB, "")
	if err := json.Unmarshal(outraAcademia.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.TotalGeral != 1 {
		t.Fatalf("academia B deveria ver só a própria cobrança, viu %d", body.TotalGeral)
	}
}

// TestIntegrationListarCobrancasAppyPayFiltroFailedIncluiFalhadaLocal
// reproduz, no nível HTTP, a mesma causa raiz de
// TestIntegrationConsultarCobrancasEstudanteFiltroEstadoFailedIncluiFalhadaLocal
// (financeiro_cobrancas_estudante_handlers_test.go) mas pelo lado de
// academia/admin: Fredy relatou que o mesmo erro (estado=Failed devolvendo
// vazio mesmo havendo cobranças falhadas) também acontece em GET
// /financeiro/cobrancas — ver estadosCobrancaEquivalentes (tarefa 69). A
// linha inserida com "falhada" simula uma cobrança criada antes do deploy
// desta tarefa (ledger imutável) — CreateCharge/CreateGPOQRCode já não
// gravam mais esse valor daqui pra frente.
func TestIntegrationListarCobrancasAppyPayFiltroFailedIncluiFalhadaLocal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := integrationFinanceClient(t)
	academia := "LSTF" + strings.ReplaceAll(uuid.NewString(), "-", "")[:6]

	insert := func(status string) {
		payload := map[string]any{"status": status, "amount": 250.0, "currency": "AOA", "description": "teste", "payment_method": "GPO"}
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		merchant := "LSF" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
		if _, err := client.DB().Exec(`INSERT INTO financeiro_cobrancas (id,merchant_transaction_id,contexto_tipo,codigo_academia,payload) VALUES ($1,$2,'academia',$3,$4)`,
			uuid.New(), merchant, academia, raw); err != nil {
			t.Fatal(err)
		}
	}
	insert("falhada")
	insert("Failed")
	insert("Success")

	previousService := FinanceiroService
	FinanceiroService = finance.NewService(client)
	t.Cleanup(func() { FinanceiroService = previousService })

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/financeiro/cobrancas?estado=Failed", nil)
	ctx.Set("dbClient", client)
	ctx.Set("user_id", uuid.New())
	ctx.Set("user_type", "academia")
	ctx.Set("codigo_academia", academia)
	ListarCobrancasAppyPay(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("academia filtrando estado=Failed = %d: %s", recorder.Code, recorder.Body.String())
	}
	var bodyFailed struct {
		Pagamentos []finance.PagamentoResumo `json:"pagamentos"`
		TotalGeral int                       `json:"total_geral"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &bodyFailed); err != nil {
		t.Fatal(err)
	}
	if bodyFailed.TotalGeral != 2 {
		t.Fatalf("estado=Failed deveria trazer as 2 cobranças falhadas (gravada como \"Failed\" e a histórica gravada como \"falhada\"), obteve %d: %s", bodyFailed.TotalGeral, recorder.Body.String())
	}
	for _, p := range bodyFailed.Pagamentos {
		if p.Status != "Failed" {
			t.Fatalf("esperava só status=\"Failed\" no resultado (normalizado), obteve %q: %s", p.Status, recorder.Body.String())
		}
	}
}

// TestIntegrationListarCobrancasAppyPayRejeitaAdminSemPermissaoFPP garante
// que a nova rota usa a mesma autorização das demais rotas de /financeiro:
// um admin sem a permissão "fpp" não pode listar cobranças.
func TestIntegrationListarCobrancasAppyPayRejeitaAdminSemPermissaoFPP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := integrationFinanceClient(t)
	adminID := uuid.New()
	if _, err := client.DB().Exec(`INSERT INTO projection_admins (id,nome,email,senha_hash,role,status,created_by) VALUES ($1,'gerente-lst',$2,'hash','gerente','ativo',$1)`, adminID, "gerente-lst-"+uuid.NewString()+"@example.test"); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/financeiro/cobrancas", nil)
	ctx.Set("dbClient", client)
	ctx.Set("user_id", adminID)
	ctx.Set("user_type", "admin")

	ListarCobrancasAppyPay(ctx)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("admin sem permissão fpp recebeu %d, queria 403: %s", recorder.Code, recorder.Body.String())
	}
}
