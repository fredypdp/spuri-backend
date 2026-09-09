package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/projections"
	"spuri/internal/storage"
)

func setupBilheteIdentidadeSemAcademiaTestRouter(client *db.Client, userID uuid.UUID) *gin.Engine {
	repository := db.NewAggregateRepository(client)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("dbClient", client)
		c.Set("repository", repository)
		c.Set("user_id", userID)
		c.Set("user_type", "estudante")
		c.Set("storageProvider", storage.NewLocalProvider())
	})
	router.PUT("/estudante/bilhete-identidade", AtualizarBilheteIdentidadeSemAcademia)
	router.POST("/estudante/solicitacoes-edicao/bilhete-identidade", CriarSolicitacaoEdicaoDadoEstudanteHandler("bilhete_identidade"))
	return router
}

// criarEstudanteVinculadoParaTeste cria (via evento real, não SQL direto) um
// estudante fundamental vinculado a uma academia recém-criada, e devolve o
// ID do estudante e o código da academia.
func criarEstudanteVinculadoParaTeste(t *testing.T, client *db.Client) (uuid.UUID, string) {
	t.Helper()
	codigoAcademia := "IT" + uuid.New().String()[:8]
	academiaID := criarAcademiaEscolarParaTeste(t, client, codigoAcademia, "fundamental", []string{"1_ano_fundamental"})

	estID := uuid.New()
	agg := &aggregates.Estudante{}
	agg.SetID(estID)
	codigoEstudante := "E" + uuid.New().String()[:6]
	anoEscolar := "1_ano_fundamental"
	telefoneEncarregado := fmt.Sprintf("9%08d", time.Now().UnixNano()%100000000)
	bilheteResp := "999999999999ZZ"
	documentos := map[string]aggregates.DocumentoMatricula{
		"bi_encarregado":   {Path: "teste/bi_encarregado.pdf"},
		"cedula_estudante": {Path: "teste/cedula_estudante.pdf"},
	}
	if err := agg.CriarComVinculo(
		"Estudante Teste "+codigoEstudante, codigoEstudante, "hash",
		nil, nil, &telefoneEncarregado, nil, &bilheteResp,
		"masculino", time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC),
		&anoEscolar, nil, nil, nil, nil,
		&academiaID, codigoAcademia, documentos,
	); err != nil {
		t.Fatalf("erro ao criar estudante de teste: %v", err)
	}
	repository := db.NewAggregateRepository(client)
	if err := repository.SaveWithAudit(agg, db.AuditContext{UserID: "integration-test", UserType: "sistema", IP: "127.0.0.1"}); err != nil {
		t.Fatalf("erro ao salvar estudante de teste: %v", err)
	}
	if err := projections.NewEstudanteProjection(client).Rebuild(); err != nil {
		t.Fatalf("erro ao reconstruir projeção de estudantes: %v", err)
	}
	return estID, codigoAcademia
}

func desvincularEstudanteDeTeste(t *testing.T, client *db.Client, estID uuid.UUID, codigoAcademia string) {
	t.Helper()
	repository := db.NewAggregateRepository(client)
	agg, err := repository.Load(estID, "Estudante")
	if err != nil {
		t.Fatalf("erro ao carregar estudante para desvincular: %v", err)
	}
	estAgg := agg.(*aggregates.Estudante)
	if err := estAgg.DesvincularDaAcademia(codigoAcademia, "fim de teste de integração", uuid.New()); err != nil {
		t.Fatalf("erro ao desvincular estudante de teste: %v", err)
	}
	if err := repository.SaveWithAudit(estAgg, db.AuditContext{UserID: "integration-test", UserType: "sistema", IP: "127.0.0.1"}); err != nil {
		t.Fatalf("erro ao salvar desvinculação: %v", err)
	}
	if err := projections.NewEstudanteProjection(client).Rebuild(); err != nil {
		t.Fatalf("erro ao reconstruir projeção de estudantes: %v", err)
	}
}

// TestEstudanteVinculadoNaoPodeUsarRotaDireta cobre o caminho "com
// academia": a rota direta deve recusar e apontar para a solicitação.
func TestEstudanteVinculadoNaoPodeUsarRotaDireta(t *testing.T) {
	client := nivelEscolarTestClient(t)
	estID, _ := criarEstudanteVinculadoParaTeste(t, client)
	router := setupBilheteIdentidadeSemAcademiaTestRouter(client, estID)

	body, _ := json.Marshal(map[string]any{"bilhete_identidade": generateBITest()})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/estudante/bilhete-identidade", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("esperava 400 (estudante vinculado não pode usar rota direta), recebeu %d: %s", rec.Code, rec.Body.String())
	}
}

// TestEstudanteVinculadoAindaConsegueSolicitarComAprovacao cobre o caso
// positivo da correção: Status='ativo' continua autorizado a usar o fluxo
// de solicitação (regressão do fix da seção 5.4).
func TestEstudanteVinculadoAindaConsegueSolicitarComAprovacao(t *testing.T) {
	client := nivelEscolarTestClient(t)
	estID, _ := criarEstudanteVinculadoParaTeste(t, client)
	router := setupBilheteIdentidadeSemAcademiaTestRouter(client, estID)

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("novo_valor", generateBITest())
	fw, _ := createPDFFormFile(w, "documento", "doc.pdf")
	_, _ = fw.Write(minimalPDFBytesForTest())
	_ = w.Close()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/estudante/solicitacoes-edicao/bilhete-identidade", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("esperava 201 (estudante vinculado pode solicitar), recebeu %d: %s", rec.Code, rec.Body.String())
	}
}

// TestEstudanteDesvinculadoPodeUsarRotaDiretaMasNaoSolicitacao é o teste
// central da correção: reproduz o bug real (CodigoAcademia permanece
// preenchido após desvincular) e confirma que, depois do fix, um estudante
// desvinculado (Status='inativo') consegue autoatualizar o BI pela rota
// direta e é corretamente barrado da rota de solicitação (que exigiria
// aprovação de uma academia da qual ele já não faz parte).
func TestEstudanteDesvinculadoPodeUsarRotaDiretaMasNaoSolicitacao(t *testing.T) {
	client := nivelEscolarTestClient(t)
	estID, codigoAcademia := criarEstudanteVinculadoParaTeste(t, client)
	desvincularEstudanteDeTeste(t, client, estID, codigoAcademia)

	var codigoAcademiaPosDesvinculo *string
	if err := client.DB().QueryRow(`SELECT codigo_academia FROM projection_estudantes WHERE id = $1`, estID).Scan(&codigoAcademiaPosDesvinculo); err != nil {
		t.Fatalf("erro ao verificar codigo_academia pós-desvínculo: %v", err)
	}
	if codigoAcademiaPosDesvinculo == nil {
		t.Fatalf("premissa do bug não reproduzida: codigo_academia deveria continuar preenchido após desvincular")
	}

	router := setupBilheteIdentidadeSemAcademiaTestRouter(client, estID)

	// Rota direta deve funcionar.
	body, _ := json.Marshal(map[string]any{"bilhete_identidade": generateBITest()})
	recDireta := httptest.NewRecorder()
	reqDireta := httptest.NewRequest(http.MethodPut, "/estudante/bilhete-identidade", bytes.NewReader(body))
	reqDireta.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recDireta, reqDireta)
	if recDireta.Code != http.StatusOK {
		t.Fatalf("esperava 200 na rota direta para estudante desvinculado, recebeu %d: %s", recDireta.Code, recDireta.Body.String())
	}

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("novo_valor", generateBITest())
	fw, _ := createPDFFormFile(w, "documento", "doc.pdf")
	_, _ = fw.Write(minimalPDFBytesForTest())
	_ = w.Close()

	recSolicitacao := httptest.NewRecorder()
	reqSolicitacao := httptest.NewRequest(http.MethodPost, "/estudante/solicitacoes-edicao/bilhete-identidade", &buf)
	reqSolicitacao.Header.Set("Content-Type", w.FormDataContentType())
	router.ServeHTTP(recSolicitacao, reqSolicitacao)
	if recSolicitacao.Code != http.StatusBadRequest {
		t.Fatalf("esperava 400 na rota de solicitação para estudante desvinculado (sem academia para aprovar), recebeu %d: %s", recSolicitacao.Code, recSolicitacao.Body.String())
	}
}

// minimalPDFBytesForTest gera um PDF mínimo válido o bastante para passar
// por readAndValidatePDF nos testes de integração deste arquivo.
func minimalPDFBytesForTest() []byte {
	return []byte("%PDF-1.4\n1 0 obj<<>>endobj\ntrailer<<>>\n%%EOF")
}

// createPDFFormFile é como multipart.Writer.CreateFormFile, mas com
// Content-Type "application/pdf" (CreateFormFile hardcoda
// application/octet-stream, e readAndValidatePDF exige application/pdf).
func createPDFFormFile(w *multipart.Writer, fieldName, fileName string) (io.Writer, error) {
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="`+fieldName+`"; filename="`+fileName+`"`)
	h.Set("Content-Type", "application/pdf")
	return w.CreatePart(h)
}

// generateBITest gera um bilhete de identidade único e válido (12 números +
// 2 letras) para evitar colisão de unicidade entre estudantes de teste
// quando vários testes deste arquivo rodam na mesma execução/banco.
func generateBITest() string {
	n := time.Now().UnixNano() % 1000000000000
	return fmt.Sprintf("%012dAB", n)
}
