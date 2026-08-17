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

// seedEstudanteParaCobrancas cria a linha mínima válida de
// projection_estudantes necessária para ConsultarCobrancasEstudante
// encontrar o estudante pelo código e devolve o id gerado (usado como
// user_id do ator "estudante" nos testes deste arquivo).
func seedEstudanteParaCobrancas(t *testing.T, client *db.Client, codigoEstudante, academia string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := client.DB().Exec(`INSERT INTO projection_estudantes (id,nome,codigo_estudante,senha_hash,telefone,codigo_academia,status,created_at) VALUES ($1,'Estudante de teste',$2,'hash',$3,$4,'ativo',CURRENT_TIMESTAMP)`,
		id, codigoEstudante, "923"+codigoEstudante, academia); err != nil {
		t.Fatal(err)
	}
	return id
}

// TestIntegrationConsultarCobrancasEstudanteEstudanteVeTodosOsEstados cobre
// o requisito principal desta tarefa: o próprio estudante consulta o
// histórico completo, sem filtro de estado por padrão.
func TestIntegrationConsultarCobrancasEstudanteEstudanteVeTodosOsEstados(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := integrationFinanceClient(t)
	academia := "COBEST" + strings.ReplaceAll(uuid.NewString(), "-", "")[:4]
	codigoEstudante := "ESTCOB1"
	estudanteID := seedEstudanteParaCobrancas(t, client, codigoEstudante, academia)

	insert := func(status string) {
		payload := map[string]any{"status": status, "amount": 300.0, "currency": "AOA", "description": "teste", "payment_method": "REF", "codigo_estudante": codigoEstudante}
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		merchant := "COB" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
		if _, err := client.DB().Exec(`INSERT INTO financeiro_cobrancas (id,merchant_transaction_id,contexto_tipo,codigo_academia,payload) VALUES ($1,$2,'academia',$3,$4)`,
			uuid.New(), merchant, academia, raw); err != nil {
			t.Fatal(err)
		}
	}
	insert("Success")
	insert("Failed")

	previousService := FinanceiroService
	FinanceiroService = finance.NewService(client)
	t.Cleanup(func() { FinanceiroService = previousService })

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/financeiro/cobrancas/estudante/"+codigoEstudante, nil)
	ctx.Params = gin.Params{{Key: "codigo", Value: codigoEstudante}}
	ctx.Set("dbClient", client)
	ctx.Set("user_id", estudanteID)
	ctx.Set("user_type", "estudante")

	ConsultarCobrancasEstudante(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("estudante consultando o próprio histórico = %d: %s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		TotalGeral int `json:"total_geral"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.TotalGeral != 2 {
		t.Fatalf("estudante deveria ver as 2 cobranças (todos os estados), obteve %d: %s", body.TotalGeral, recorder.Body.String())
	}
}

// TestIntegrationConsultarCobrancasEstudanteRejeitaOutroEstudante garante
// que um estudante não consegue consultar o histórico de outro, mesmo
// sabendo o código dele.
func TestIntegrationConsultarCobrancasEstudanteRejeitaOutroEstudante(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := integrationFinanceClient(t)
	academia := "COBEST" + strings.ReplaceAll(uuid.NewString(), "-", "")[:4]
	codigoEstudante := "ESTCOB2"
	seedEstudanteParaCobrancas(t, client, codigoEstudante, academia)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/financeiro/cobrancas/estudante/"+codigoEstudante, nil)
	ctx.Params = gin.Params{{Key: "codigo", Value: codigoEstudante}}
	ctx.Set("dbClient", client)
	ctx.Set("user_id", uuid.New()) // outro estudante, não o dono do código
	ctx.Set("user_type", "estudante")

	ConsultarCobrancasEstudante(ctx)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("outro estudante recebeu %d, queria 403: %s", recorder.Code, recorder.Body.String())
	}
}

// TestIntegrationConsultarCobrancasEstudanteAcademiaSemVinculoEProibida
// garante que uma academia sem vínculo (atual ou histórico) com o estudante
// recebe 403, mesmo sabendo o código dele.
func TestIntegrationConsultarCobrancasEstudanteAcademiaSemVinculoEProibida(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := integrationFinanceClient(t)
	academiaDona := "COBEST" + strings.ReplaceAll(uuid.NewString(), "-", "")[:4]
	outraAcademia := "COBEST" + strings.ReplaceAll(uuid.NewString(), "-", "")[:4]
	codigoEstudante := "ESTCOB3"
	seedEstudanteParaCobrancas(t, client, codigoEstudante, academiaDona)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/financeiro/cobrancas/estudante/"+codigoEstudante, nil)
	ctx.Params = gin.Params{{Key: "codigo", Value: codigoEstudante}}
	ctx.Set("dbClient", client)
	ctx.Set("user_id", uuid.New())
	ctx.Set("user_type", "academia")
	ctx.Set("codigo_academia", outraAcademia)

	ConsultarCobrancasEstudante(ctx)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("academia sem vínculo recebeu %d, queria 403: %s", recorder.Code, recorder.Body.String())
	}
}
