package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"spuri/internal/db"
	"spuri/internal/finance"
)

func seedAcademiaParaRemocaoHandlers(t *testing.T, client *db.Client, codigo string) {
	t.Helper()
	_, err := client.DB().Exec(`INSERT INTO projection_academias
		(id,nivel,nome,nif,codigo_academia,senha_hash,provincia,endereco,nivel_escolar,status,cursos,anos_academicos,type,ano_letivo,created_at)
		VALUES ($1,'escola','Academia remoção',$2,$3,'hash','LUA','endereco','fundamental','ativo','[]'::jsonb,'["7_ano_fundamental"]'::jsonb,'private','2026_2027',CURRENT_TIMESTAMP)`,
		uuid.New(), geraDigitos(10), codigo)
	if err != nil {
		t.Fatal(err)
	}
}

func newDeleteJSONContext(method, path string, body any) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	buf, _ := json.Marshal(body)
	ctx.Request = httptest.NewRequest(method, path, bytes.NewReader(buf))
	ctx.Request.Header.Set("Content-Type", "application/json")
	return ctx, recorder
}

// Cobre, para os 4 endpoints de remoção novos, o mesmo contrato de
// autorização que já vale para os endpoints de configuração equivalentes:
// a academia só remove a PRÓPRIA configuração; tentar remover algo que não
// existe retorna 404 (ErrNotFound -> financeError); removendo o que existe
// retorna 204 e a segunda tentativa (idempotência de erro) volta a dar 404.
func TestIntegrationHandlersRemocaoFinanceiraRespeitamEscopoDaAcademia(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := integrationFinanceClient(t)
	academiaDona := "REM" + uuid.NewString()[:6]
	academiaOutra := "OUT" + uuid.NewString()[:6]
	seedAcademiaParaRemocaoHandlers(t, client, academiaDona)
	seedAcademiaParaRemocaoHandlers(t, client, academiaOutra)

	previousService := FinanceiroService
	FinanceiroService = finance.NewService(client)
	t.Cleanup(func() { FinanceiroService = previousService })

	userID := uuid.New()
	setActor := func(ctx *gin.Context, academia string) {
		ctx.Set("dbClient", client)
		ctx.Set("user_id", userID)
		ctx.Set("user_type", "academia")
		ctx.Set("codigo_academia", academia)
	}

	// 1) Credenciais: nada configurado ainda -> 404.
	ctx, rec := newDeleteJSONContext(http.MethodDelete, "/financeiro/appypay/credenciais", map[string]any{
		"contexto_tipo": finance.ContextoAcademia, "codigo_academia": academiaDona,
	})
	setActor(ctx, academiaDona)
	RemoverCredencialAppyPay(ctx)
	if ctx.Writer.Status() != http.StatusNotFound {
		t.Fatalf("RemoverCredencialAppyPay sem credencial: esperava 404, obteve %d: %s", ctx.Writer.Status(), rec.Body.String())
	}

	// Configura via o Service diretamente (mais rápido que passar pelo
	// handler de criação) e então remove via o HANDLER, que é o que este
	// teste precisa validar.
	if _, _, err := FinanceiroService.ConfigureCredential(ctx.Request.Context(), nil, finance.CredentialInput{
		ContextoTipo: finance.ContextoAcademia, CodigoAcademia: academiaDona,
		ClientID: "client-" + uuid.NewString(), ClientSecret: "secret-" + uuid.NewString(),
		GPOPaymentMethod: "GPO_QR_TEST", REFPaymentMethod: "REF_TEST",
	}, uuid.NewString(), "academia", "127.0.0.1"); err != nil {
		t.Fatalf("ConfigureCredential falhou: %v", err)
	}

	// Outra academia não pode remover: authorizeFinanceScope reescreve o
	// escopo para o dela própria, então a busca por credencial nesse
	// escopo (vazio) falha com 404 — nunca afeta a credencial da dona.
	ctxOutra, _ := newDeleteJSONContext(http.MethodDelete, "/financeiro/appypay/credenciais", map[string]any{
		"contexto_tipo": finance.ContextoAcademia, "codigo_academia": academiaDona,
	})
	setActor(ctxOutra, academiaOutra)
	RemoverCredencialAppyPay(ctxOutra)
	if ctxOutra.Writer.Status() == http.StatusNoContent {
		t.Fatal("academia sem vínculo conseguiu remover credencial de outra academia")
	}

	ctx2, rec2 := newDeleteJSONContext(http.MethodDelete, "/financeiro/appypay/credenciais", map[string]any{
		"contexto_tipo": finance.ContextoAcademia, "codigo_academia": academiaDona,
	})
	setActor(ctx2, academiaDona)
	RemoverCredencialAppyPay(ctx2)
	if ctx2.Writer.Status() != http.StatusNoContent {
		t.Fatalf("RemoverCredencialAppyPay pela dona: esperava 204, obteve %d: %s", ctx2.Writer.Status(), rec2.Body.String())
	}
	ctx3, _ := newDeleteJSONContext(http.MethodDelete, "/financeiro/appypay/credenciais", map[string]any{
		"contexto_tipo": finance.ContextoAcademia, "codigo_academia": academiaDona,
	})
	setActor(ctx3, academiaDona)
	RemoverCredencialAppyPay(ctx3)
	if ctx3.Writer.Status() != http.StatusNotFound {
		t.Fatalf("segunda remoção de credencial: esperava 404, obteve %d", ctx3.Writer.Status())
	}

	// 2) Configuração de mensalidade.
	if _, _, err := FinanceiroService.ConfigureCredential(ctx.Request.Context(), nil, finance.CredentialInput{
		ContextoTipo: finance.ContextoAcademia, CodigoAcademia: academiaDona,
		ClientID: "client2-" + uuid.NewString(), ClientSecret: "secret2-" + uuid.NewString(),
		GPOPaymentMethod: "GPO_QR_TEST", REFPaymentMethod: "REF_TEST",
	}, uuid.NewString(), "academia", "127.0.0.1"); err != nil {
		t.Fatalf("ConfigureCredential (2) falhou: %v", err)
	}
	if _, err := FinanceiroService.ConfigureMensalidade(ctx.Request.Context(), finance.MensalidadeConfiguracaoInput{
		CodigoAcademia: academiaDona, Nivel: finance.NivelFundamental, AnoAcademico: "7_ano_fundamental",
		Valor: 5000, MesFimCobranca: 7, MetodosPagamento: []string{"GPO_QR"}, ModoVigencia: finance.ModoVigenciaAPartirDaAtualizacao,
	}, uuid.NewString(), "academia", "127.0.0.1"); err != nil {
		t.Fatalf("ConfigureMensalidade falhou: %v", err)
	}
	ctxMens, recMens := newDeleteJSONContext(http.MethodDelete, "/financeiro/mensalidades/configuracoes", map[string]any{
		"codigo_academia": academiaDona, "nivel": finance.NivelFundamental, "ano_academico": "7_ano_fundamental",
	})
	setActor(ctxMens, academiaDona)
	RemoverConfiguracaoMensalidade(ctxMens)
	if ctxMens.Writer.Status() != http.StatusNoContent {
		t.Fatalf("RemoverConfiguracaoMensalidade: esperava 204, obteve %d: %s", ctxMens.Writer.Status(), recMens.Body.String())
	}

	// 3) Mês de início de cobrança.
	if err := FinanceiroService.DefinirMesInicioCobranca(ctx.Request.Context(), finance.MesInicioCobrancaInput{
		CodigoAcademia: academiaDona, AnoLetivo: "2026_2027", MesInicio: 11,
	}, uuid.NewString(), "academia", "127.0.0.1"); err != nil {
		t.Fatalf("DefinirMesInicioCobranca falhou: %v", err)
	}
	ctxMes, recMes := newDeleteJSONContext(http.MethodDelete, "/financeiro/mensalidades/inicio-cobranca", map[string]any{
		"codigo_academia": academiaDona, "ano_letivo": "2026_2027",
	})
	setActor(ctxMes, academiaDona)
	RemoverMesInicioCobranca(ctxMes)
	if ctxMes.Writer.Status() != http.StatusNoContent {
		t.Fatalf("RemoverMesInicioCobranca: esperava 204, obteve %d: %s", ctxMes.Writer.Status(), recMes.Body.String())
	}

	// 4) Configuração de matrícula.
	if _, err := FinanceiroService.ConfigureMatricula(ctx.Request.Context(), finance.MatriculaConfiguracaoInput{
		CodigoAcademia: academiaDona, Nivel: finance.NivelFundamental, AnoAcademico: "7_ano_fundamental",
		Valor: 15000, MetodosPagamento: []string{"GPO_QR"}, ModoVigencia: finance.ModoVigenciaAPartirDaAtualizacao,
	}, uuid.NewString(), "academia", "127.0.0.1"); err != nil {
		t.Fatalf("ConfigureMatricula falhou: %v", err)
	}
	ctxMat, recMat := newDeleteJSONContext(http.MethodDelete, "/financeiro/matriculas/configuracoes", map[string]any{
		"codigo_academia": academiaDona, "nivel": finance.NivelFundamental, "ano_academico": "7_ano_fundamental",
	})
	setActor(ctxMat, academiaDona)
	RemoverConfiguracaoMatricula(ctxMat)
	if ctxMat.Writer.Status() != http.StatusNoContent {
		t.Fatalf("RemoverConfiguracaoMatricula: esperava 204, obteve %d: %s", ctxMat.Writer.Status(), recMat.Body.String())
	}
}
