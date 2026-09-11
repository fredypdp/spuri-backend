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
	"spuri/internal/domain/aggregates"
	"spuri/internal/projections"
)

// setupDelecaoAcademiaTestRouter registra a rota de deleção da conta do
// estudante pela academia (Tarefa 98), autenticada como a academia userID.
func setupDelecaoAcademiaTestRouter(client *db.Client, academiaID uuid.UUID) *gin.Engine {
	repository := db.NewAggregateRepository(client)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("dbClient", client)
		c.Set("repository", repository)
		c.Set("user_id", academiaID)
		c.Set("user_type", "academia")
	})
	router.DELETE("/academia/estudante/:codigo/conta", DeletarContaEstudantePorAcademia)
	return router
}

func deletarContaEstudantePorAcademia(router *gin.Engine, codigoEstudante, motivo string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"motivo": motivo})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/academia/estudante/"+codigoEstudante+"/conta", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	return rec
}

// reintegrarEstudanteEmOutraAcademiaParaTeste simula uma revinculação (ou
// transferência) do estudante para uma academia DIFERENTE da que o
// cadastrou — o mesmo evento (EstudanteReintegrado) usado pela rota real
// POST /estudante/solicitacoes-status/revinculacao/:codigo_academia, que
// aceita qualquer codigo_academia (não necessariamente o original). Depois
// disto, estudanteDTO.CodigoAcademia aponta para a nova academia, mas o
// PRIMEIRO evento do ledger (EstudanteCriadoComVinculo) continua apontando
// para a academia original — é exatamente essa diferença que a Tarefa 98
// precisa respeitar.
func reintegrarEstudanteEmOutraAcademiaParaTeste(t *testing.T, client *db.Client, estID uuid.UUID, novaCodigoAcademia string) {
	t.Helper()
	repository := db.NewAggregateRepository(client)
	agg, err := repository.Load(estID, "Estudante")
	if err != nil {
		t.Fatalf("erro ao carregar estudante para reintegrar: %v", err)
	}
	estAgg := agg.(*aggregates.Estudante)
	if err := estAgg.Reintegrar(novaCodigoAcademia, "fundamental", nil, nil, nil, nil, uuid.New()); err != nil {
		t.Fatalf("erro ao reintegrar estudante de teste em outra academia: %v", err)
	}
	if err := repository.SaveWithAudit(estAgg, db.AuditContext{UserID: "integration-test", UserType: "sistema", IP: "127.0.0.1"}); err != nil {
		t.Fatalf("erro ao salvar reintegração: %v", err)
	}
	if err := projections.NewEstudanteProjection(client).Rebuild(); err != nil {
		t.Fatalf("erro ao reconstruir projeção de estudantes: %v", err)
	}
}

// TestDeletarContaEstudantePorAcademiaOKQuandoMesmaAcademiaCadastrou cobre o
// caminho feliz da Tarefa 98: a academia que cadastrou o estudante pode
// deletar a conta dele enquanto ele ainda está vinculado a ela (sem exigir
// desvinculação prévia).
func TestDeletarContaEstudantePorAcademiaOKQuandoMesmaAcademiaCadastrou(t *testing.T) {
	client := nivelEscolarTestClient(t)
	estID, codigoAcademia := criarEstudanteVinculadoParaTeste(t, client)

	academia, err := projections.NewAcademiaProjection(client).GetByCodigo(codigoAcademia)
	if err != nil || academia == nil {
		t.Fatalf("erro ao buscar academia de teste: %v", err)
	}
	est, err := projections.NewEstudanteProjection(client).GetByID(estID)
	if err != nil || est == nil {
		t.Fatalf("erro ao buscar estudante de teste: %v", err)
	}

	router := setupDelecaoAcademiaTestRouter(client, academia.ID)
	rec := deletarContaEstudantePorAcademia(router, est.CodigoEstudante, "estudante pediu remoção dos dados")

	if rec.Code != http.StatusOK {
		t.Fatalf("esperava 200, recebeu %d: %s", rec.Code, rec.Body.String())
	}
	if err := projections.NewEstudanteProjection(client).Rebuild(); err != nil {
		t.Fatalf("erro ao reconstruir projeção de estudantes após deleção: %v", err)
	}

	estAtualizado, err := projections.NewEstudanteProjection(client).GetByID(estID)
	if err != nil || estAtualizado == nil {
		t.Fatalf("erro ao buscar estudante após deleção: %v", err)
	}
	if estAtualizado.Status != "deletado" {
		t.Fatalf("Status = %q, want %q", estAtualizado.Status, "deletado")
	}
}

// TestDeletarContaEstudantePorAcademiaForbiddenQuandoAcademiaSoRecebeuPorRevinculacao
// é o teste central da Tarefa 98: uma academia que recebeu o estudante por
// revinculação (ou transferência) — e por isso está vinculada a ele agora —
// mas NÃO foi quem o cadastrou originalmente, não pode deletar a conta dele.
// Sem a checagem via histórico do ledger, esta academia passaria pela
// checagem ingênua "codigo_academia atual == academia autenticada" e
// conseguiria deletar indevidamente.
func TestDeletarContaEstudantePorAcademiaForbiddenQuandoAcademiaSoRecebeuPorRevinculacao(t *testing.T) {
	client := nivelEscolarTestClient(t)
	estID, codigoAcademiaOriginal := criarEstudanteVinculadoParaTeste(t, client)
	desvincularEstudanteDeTeste(t, client, estID, codigoAcademiaOriginal)

	codigoAcademiaNova := "IT" + uuid.New().String()[:8]
	academiaNovaID := criarAcademiaEscolarParaTeste(t, client, codigoAcademiaNova, "fundamental", []string{"1_ano_fundamental"})
	reintegrarEstudanteEmOutraAcademiaParaTeste(t, client, estID, codigoAcademiaNova)

	est, err := projections.NewEstudanteProjection(client).GetByID(estID)
	if err != nil || est == nil {
		t.Fatalf("erro ao buscar estudante de teste: %v", err)
	}
	if est.CodigoAcademia == nil || *est.CodigoAcademia != codigoAcademiaNova {
		t.Fatalf("pré-condição do teste falhou: estudante deveria estar vinculado à nova academia, CodigoAcademia=%v", est.CodigoAcademia)
	}

	router := setupDelecaoAcademiaTestRouter(client, academiaNovaID)
	rec := deletarContaEstudantePorAcademia(router, est.CodigoEstudante, "tentativa indevida")

	if rec.Code != http.StatusForbidden {
		t.Fatalf("esperava 403 (academia não foi quem cadastrou o estudante), recebeu %d: %s", rec.Code, rec.Body.String())
	}

	estAposTentativa, err := projections.NewEstudanteProjection(client).GetByID(estID)
	if err != nil || estAposTentativa == nil {
		t.Fatalf("erro ao buscar estudante após tentativa: %v", err)
	}
	if estAposTentativa.Status == "deletado" {
		t.Fatal("estudante foi deletado indevidamente por academia que não o cadastrou")
	}
}

// TestDeletarContaEstudantePorAcademiaForbiddenQuandoEstudanteNaoPertenceAAcademia
// cobre o caso mais simples: uma academia totalmente alheia (nunca teve
// vínculo com o estudante) não pode deletar a conta dele.
func TestDeletarContaEstudantePorAcademiaForbiddenQuandoEstudanteNaoPertenceAAcademia(t *testing.T) {
	client := nivelEscolarTestClient(t)
	estID, _ := criarEstudanteVinculadoParaTeste(t, client)
	est, err := projections.NewEstudanteProjection(client).GetByID(estID)
	if err != nil || est == nil {
		t.Fatalf("erro ao buscar estudante de teste: %v", err)
	}

	codigoAcademiaAlheia := "IT" + uuid.New().String()[:8]
	academiaAlheiaID := criarAcademiaEscolarParaTeste(t, client, codigoAcademiaAlheia, "fundamental", []string{"1_ano_fundamental"})
	if err := projections.NewEstudanteProjection(client).Rebuild(); err != nil {
		t.Fatalf("erro ao reconstruir projeção de estudantes: %v", err)
	}

	router := setupDelecaoAcademiaTestRouter(client, academiaAlheiaID)
	rec := deletarContaEstudantePorAcademia(router, est.CodigoEstudante, "tentativa indevida")

	if rec.Code != http.StatusForbidden {
		t.Fatalf("esperava 403, recebeu %d: %s", rec.Code, rec.Body.String())
	}
}

// TestDeletarContaEstudantePorAcademiaFalhaQuandoDesvinculado cobre a outra
// metade da regra: mesmo sendo a academia que cadastrou o estudante, ela só
// pode deletar enquanto ele está VINCULADO — depois de desvinculado, a via é
// exclusivamente a autodeleção do próprio estudante (DeletarContaEstudante).
func TestDeletarContaEstudantePorAcademiaFalhaQuandoDesvinculado(t *testing.T) {
	client := nivelEscolarTestClient(t)
	estID, codigoAcademia := criarEstudanteVinculadoParaTeste(t, client)
	desvincularEstudanteDeTeste(t, client, estID, codigoAcademia)

	academia, err := projections.NewAcademiaProjection(client).GetByCodigo(codigoAcademia)
	if err != nil || academia == nil {
		t.Fatalf("erro ao buscar academia de teste: %v", err)
	}
	est, err := projections.NewEstudanteProjection(client).GetByID(estID)
	if err != nil || est == nil {
		t.Fatalf("erro ao buscar estudante de teste: %v", err)
	}

	router := setupDelecaoAcademiaTestRouter(client, academia.ID)
	rec := deletarContaEstudantePorAcademia(router, est.CodigoEstudante, "tentativa após desvinculação")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("esperava 400 (estudante já desvinculado), recebeu %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDeletarContaEstudantePorAcademiaMotivoObrigatorio(t *testing.T) {
	client := nivelEscolarTestClient(t)
	estID, codigoAcademia := criarEstudanteVinculadoParaTeste(t, client)
	academia, err := projections.NewAcademiaProjection(client).GetByCodigo(codigoAcademia)
	if err != nil || academia == nil {
		t.Fatalf("erro ao buscar academia de teste: %v", err)
	}
	est, err := projections.NewEstudanteProjection(client).GetByID(estID)
	if err != nil || est == nil {
		t.Fatalf("erro ao buscar estudante de teste: %v", err)
	}

	router := setupDelecaoAcademiaTestRouter(client, academia.ID)
	rec := deletarContaEstudantePorAcademia(router, est.CodigoEstudante, "")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("esperava 400 (motivo obrigatório), recebeu %d: %s", rec.Code, rec.Body.String())
	}
}
