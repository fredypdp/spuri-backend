package handlers

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"spuri/internal/db"
	"spuri/internal/projections"
	"spuri/internal/storage"
)

// setupSolicitacaoEdicaoBITestRouter registra as rotas de criação (estudante)
// e decisão (academia) da solicitação de edição de bilhete_identidade,
// autenticadas como userID/userType fixos — mesmo padrão dos demais testes
// de integração HTTP deste pacote.
func setupSolicitacaoEdicaoBITestRouter(client *db.Client, userID uuid.UUID, userType string) *gin.Engine {
	repository := db.NewAggregateRepository(client)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("dbClient", client)
		c.Set("repository", repository)
		c.Set("user_id", userID)
		c.Set("user_type", userType)
		c.Set("storageProvider", storage.NewLocalProvider())
	})
	router.POST("/estudante/solicitacoes-edicao/bilhete-identidade", CriarSolicitacaoEdicaoDadoEstudanteHandler("bilhete_identidade"))
	router.PUT("/academia/solicitacoes-edicao-estudante/bilhete-identidade/:codigo/aprovar", DecidirSolicitacaoEdicaoDadoEstudanteHandler("bilhete_identidade", true))
	router.PUT("/academia/solicitacoes-edicao-estudante/bilhete-identidade/:codigo/reprovar", DecidirSolicitacaoEdicaoDadoEstudanteHandler("bilhete_identidade", false))
	return router
}

func criarSolicitacaoEdicaoBIParaTeste(t *testing.T, client *db.Client, router *gin.Engine, novoValor string, conteudoPDF []byte) string {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if err := w.WriteField("novo_valor", novoValor); err != nil {
		t.Fatalf("erro ao escrever campo novo_valor: %v", err)
	}
	part, err := createPDFFormFile(w, "documento", "bi.pdf")
	if err != nil {
		t.Fatalf("erro ao criar campo de arquivo: %v", err)
	}
	if _, err := part.Write(conteudoPDF); err != nil {
		t.Fatalf("erro ao escrever PDF de teste: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("erro ao fechar multipart writer: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/estudante/solicitacoes-edicao/bilhete-identidade", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("esperava 201 ao criar solicitação de edição de BI, recebeu %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		CodigoSolicitacao string `json:"codigo_solicitacao"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("erro ao decodificar resposta de criação: %v", err)
	}
	// A criação grava no ledger de forma síncrona (SaveWithAudit), mas a
	// projeção de solicitações só é atualizada pelo projManager assíncrono
	// em produção — nos testes, sem esse worker rodando, precisamos
	// reconstruir explicitamente antes que a academia consiga encontrar a
	// solicitação pelo código (mesmo padrão usado para Estudante/Academia
	// em todo este pacote de testes).
	if err := projections.NewSolicitacaoEdicaoDadoEstudanteProjection(client).Rebuild(); err != nil {
		t.Fatalf("erro ao reconstruir projeção de solicitações de edição: %v", err)
	}
	return resp.CodigoSolicitacao
}

func aprovarSolicitacaoEdicaoBI(t *testing.T, client *db.Client, router *gin.Engine, codigoSolicitacao string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/academia/solicitacoes-edicao-estudante/bilhete-identidade/"+codigoSolicitacao+"/aprovar", nil)
	router.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		// Idem: reflete a decisão (status -> aprovada) na projeção antes que
		// o teste prossiga, senão ExistePendente ainda veria a solicitação
		// anterior como pendente numa segunda tentativa de edição.
		if err := projections.NewSolicitacaoEdicaoDadoEstudanteProjection(client).Rebuild(); err != nil {
			t.Fatalf("erro ao reconstruir projeção de solicitações de edição após decisão: %v", err)
		}
	}
	return rec
}

// syncEstudanteProjectionAposEvento processa, via EstudanteProjection.Handle,
// apenas o ÚLTIMO evento do estudante estID — o mesmo que o projManager
// assíncrono processaria em produção depois de um SaveWithAudit. Evitamos
// deliberadamente EstudanteProjection.Rebuild() aqui: Rebuild() é um replay
// GLOBAL de todo o ledger (de qualquer teste que já rodou neste processo) e
// falha se alguma academia referenciada por QUALQUER estudante de QUALQUER
// outro teste de integração deste pacote ainda não estiver projetada — algo
// fora do nosso controle e sem relação com o que este teste verifica.
// Processar só o evento novo é mais preciso (é exatamente o que
// aconteceria em produção) e imune a esse tipo de poluição cruzada entre
// testes que compartilham o mesmo banco.
func syncEstudanteProjectionAposEvento(t *testing.T, client *db.Client, estID uuid.UUID) {
	t.Helper()
	repository := db.NewAggregateRepository(client)
	historico, err := repository.GetEventHistory(estID)
	if err != nil || len(historico) == 0 {
		t.Fatalf("erro ao buscar histórico do estudante para sincronizar projeção: %v", err)
	}
	ultimo := historico[len(historico)-1]
	if err := projections.NewEstudanteProjection(client).Handle(ultimo); err != nil {
		t.Fatalf("erro ao processar último evento (%s) na projeção de estudantes: %v", ultimo.EventType, err)
	}
}

// TestAprovarSolicitacaoEdicaoBIPromoveDocumentoQuandoNaoHaviaNenhum cobre o
// caso central da Tarefa 98: o estudante criado por criarEstudanteVinculadoParaTeste
// NÃO tem nenhum documento em Documentos["bi_estudante"] (só bi_encarregado e
// cedula_estudante). Depois de uma solicitação de edição de BI aprovada, o
// documento anexado à solicitação deve se tornar o documento oficial — "deve
// substituir o atual (mesmo se não tiver um)", como pedido.
func TestAprovarSolicitacaoEdicaoBIPromoveDocumentoQuandoNaoHaviaNenhum(t *testing.T) {
	client := nivelEscolarTestClient(t)
	estID, codigoAcademia := criarEstudanteVinculadoParaTeste(t, client)
	est, err := projections.NewEstudanteProjection(client).GetByID(estID)
	if err != nil || est == nil {
		t.Fatalf("erro ao buscar estudante de teste: %v", err)
	}
	if _, ok := est.Documentos["bi_estudante"]; ok {
		t.Fatal("pré-condição do teste falhou: estudante já tinha documento bi_estudante")
	}
	academia, err := projections.NewAcademiaProjection(client).GetByCodigo(codigoAcademia)
	if err != nil || academia == nil {
		t.Fatalf("erro ao buscar academia de teste: %v", err)
	}

	routerEstudante := setupSolicitacaoEdicaoBITestRouter(client, estID, "estudante")
	routerAcademia := setupSolicitacaoEdicaoBITestRouter(client, academia.ID, "academia")

	novoBI := generateBITest()
	codigoSolicitacao := criarSolicitacaoEdicaoBIParaTeste(t, client, routerEstudante, novoBI, minimalPDFBytesForTest())

	rec := aprovarSolicitacaoEdicaoBI(t, client, routerAcademia, codigoSolicitacao)
	if rec.Code != http.StatusOK {
		t.Fatalf("esperava 200 ao aprovar solicitação, recebeu %d: %s", rec.Code, rec.Body.String())
	}

	syncEstudanteProjectionAposEvento(t, client, estID)
	estAtualizado, err := projections.NewEstudanteProjection(client).GetByID(estID)
	if err != nil || estAtualizado == nil {
		t.Fatalf("erro ao buscar estudante após aprovação: %v", err)
	}
	if estAtualizado.BilheteIdentidade == nil || *estAtualizado.BilheteIdentidade != novoBI {
		t.Fatalf("BilheteIdentidade = %v, want %q", estAtualizado.BilheteIdentidade, novoBI)
	}
	doc, ok := estAtualizado.Documentos["bi_estudante"]
	if !ok {
		t.Fatal("Documentos[\"bi_estudante\"] ausente após aprovação — o documento da solicitação não foi promovido a oficial")
	}
	if doc.Path == "" {
		t.Fatal("Documentos[\"bi_estudante\"].Path está vazio")
	}

	// O arquivo tem de existir de fato no caminho definitivo (não só o
	// ponteiro na projeção) — senão um download real falharia.
	provider := storage.NewLocalProvider()
	rc, err := provider.Read(doc.Path)
	if err != nil {
		t.Fatalf("documento oficial do BI não está legível em %q: %v", doc.Path, err)
	}
	_ = rc.Close()
}

// TestAprovarSolicitacaoEdicaoBISubstituiDocumentoAnterior cobre a segunda
// metade da regra: quando JÁ existe um documento oficial de BI, uma nova
// solicitação aprovada substitui (não acumula) — e o documento antigo deixa
// de ser acessível no caminho anterior.
func TestAprovarSolicitacaoEdicaoBISubstituiDocumentoAnterior(t *testing.T) {
	client := nivelEscolarTestClient(t)
	estID, codigoAcademia := criarEstudanteVinculadoParaTeste(t, client)
	academia, err := projections.NewAcademiaProjection(client).GetByCodigo(codigoAcademia)
	if err != nil || academia == nil {
		t.Fatalf("erro ao buscar academia de teste: %v", err)
	}
	routerEstudante := setupSolicitacaoEdicaoBITestRouter(client, estID, "estudante")
	routerAcademia := setupSolicitacaoEdicaoBITestRouter(client, academia.ID, "academia")

	// Ciclo 1: estabelece o primeiro documento oficial.
	bi1 := generateBITest()
	cod1 := criarSolicitacaoEdicaoBIParaTeste(t, client, routerEstudante, bi1, minimalPDFBytesForTest())
	if rec := aprovarSolicitacaoEdicaoBI(t, client, routerAcademia, cod1); rec.Code != http.StatusOK {
		t.Fatalf("esperava 200 no primeiro ciclo, recebeu %d: %s", rec.Code, rec.Body.String())
	}
	syncEstudanteProjectionAposEvento(t, client, estID)
	estApos1, err := projections.NewEstudanteProjection(client).GetByID(estID)
	if err != nil || estApos1 == nil {
		t.Fatalf("erro ao buscar estudante após ciclo 1: %v", err)
	}
	doc1, ok := estApos1.Documentos["bi_estudante"]
	if !ok || doc1.Path == "" {
		t.Fatal("documento do primeiro ciclo não foi promovido corretamente")
	}

	// Ciclo 2: um segundo PDF diferente (conteúdo maior, para garantir um
	// arquivo distinto) substitui o primeiro.
	bi2 := generateBITest()
	pdf2 := append(minimalPDFBytesForTest(), []byte("\n% segundo documento de teste")...)
	cod2 := criarSolicitacaoEdicaoBIParaTeste(t, client, routerEstudante, bi2, pdf2)
	if rec := aprovarSolicitacaoEdicaoBI(t, client, routerAcademia, cod2); rec.Code != http.StatusOK {
		t.Fatalf("esperava 200 no segundo ciclo, recebeu %d: %s", rec.Code, rec.Body.String())
	}
	syncEstudanteProjectionAposEvento(t, client, estID)
	estApos2, err := projections.NewEstudanteProjection(client).GetByID(estID)
	if err != nil || estApos2 == nil {
		t.Fatalf("erro ao buscar estudante após ciclo 2: %v", err)
	}
	if estApos2.BilheteIdentidade == nil || *estApos2.BilheteIdentidade != bi2 {
		t.Fatalf("BilheteIdentidade = %v, want %q (segundo ciclo)", estApos2.BilheteIdentidade, bi2)
	}
	doc2, ok := estApos2.Documentos["bi_estudante"]
	if !ok || doc2.Path == "" {
		t.Fatal("documento do segundo ciclo não foi promovido corretamente")
	}
	if doc2.Path == doc1.Path {
		t.Fatal("o documento do segundo ciclo deveria ter um caminho diferente do primeiro (substituição, não reaproveitamento)")
	}

	provider := storage.NewLocalProvider()
	if rc, err := provider.Read(doc2.Path); err != nil {
		t.Fatalf("documento oficial atual não está legível em %q: %v", doc2.Path, err)
	} else {
		_ = rc.Close()
	}
	if _, err := provider.Read(doc1.Path); err == nil {
		t.Fatal("documento antigo ainda está acessível após substituição — deveria ter sido removido")
	}
}
