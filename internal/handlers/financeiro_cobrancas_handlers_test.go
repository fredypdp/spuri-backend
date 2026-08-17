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
		Cobrancas []struct {
			Origem string `json:"origem"`
			Status string `json:"status"`
		} `json:"cobrancas"`
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

	filtrada := call(academiaA, "estado=Success")
	if err := json.Unmarshal(filtrada.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.TotalGeral != 1 || len(body.Cobrancas) != 1 || body.Cobrancas[0].Origem != "mensalidade" {
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
