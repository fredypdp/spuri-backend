package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/handlers"
	"spuri/internal/middleware"
	"spuri/internal/projections"
)

type turmaVinculoFixture struct {
	client        *db.Client
	repository    *db.AggregateRepository
	router        *gin.Engine
	academia      *aggregates.Academia
	outraAcademia *aggregates.Academia
	turmaAtiva    *aggregates.Turma
	turmaInativa  *aggregates.Turma
	turmaOutra    *aggregates.Turma
	turmaAtiva2   *aggregates.Turma
	token         string
}

func setupTurmaVinculoIntegration(t *testing.T) *turmaVinculoFixture {
	t.Helper()
	if os.Getenv("SPURI_RUN_DB_INTEGRITY_TESTS") != "1" {
		t.Skip("set SPURI_RUN_DB_INTEGRITY_TESTS=1 with an isolated PostgreSQL database to run")
	}
	t.Setenv("ENV", "test")
	prev, _ := os.Getwd()
	_ = os.Chdir("../..")
	t.Cleanup(func() { _ = os.Chdir(prev) })
	client, err := db.NewClient(db.DefaultConfig())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.RunMigrations(); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	oldDB, oldRepo, oldPM := dbClient, repository, projManager
	dbClient = client
	repository = db.NewAggregateRepository(client)
	projManager = projections.NewManager(client)
	projManager.RegisterProjection("academias", projections.NewAcademiaProjection(client))
	projManager.RegisterProjection("estudantes", projections.NewEstudanteProjection(client))
	projManager.RegisterProjection("turmas", projections.NewTurmasProjection(client))
	t.Cleanup(func() { dbClient, repository, projManager = oldDB, oldRepo, oldPM })
	seq := time.Now().UnixNano()
	academia := criarAcademiaCorrecao(t, repository, seq, "V")
	outra := criarAcademiaCorrecao(t, repository, seq, "W")
	if err := academia.DefinirAnoLetivo("2025_2026", "escolar", academia.ID); err != nil {
		t.Fatalf("definir ano letivo academia: %v", err)
	}
	if err := repository.SaveWithAudit(academia, db.AuditContext{UserID: academia.ID.String(), UserType: "academia", IP: "127.0.0.1"}); err != nil {
		t.Fatalf("salvar ano letivo academia: %v", err)
	}
	if err := outra.DefinirAnoLetivo("2025_2026", "escolar", outra.ID); err != nil {
		t.Fatalf("definir ano letivo outra academia: %v", err)
	}
	if err := repository.SaveWithAudit(outra, db.AuditContext{UserID: outra.ID.String(), UserType: "academia", IP: "127.0.0.1"}); err != nil {
		t.Fatalf("salvar ano letivo outra academia: %v", err)
	}
	if err := projections.NewAcademiaProjection(client).Rebuild(); err != nil {
		t.Fatalf("rebuild academias: %v", err)
	}
	makeTurma := func(cod string, acad *aggregates.Academia, inactive bool) *aggregates.Turma {
		tr := aggregates.NewTurma()
		if err := tr.Criar(cod, acad.CodigoAcademia, "1_ano_fundamental", nil, "manha", acad.ID); err != nil {
			t.Fatalf("criar turma: %v", err)
		}
		if inactive {
			if err := tr.Desativar(acad.ID); err != nil {
				t.Fatalf("desativar turma: %v", err)
			}
		}
		if err := repository.SaveWithAudit(tr, db.AuditContext{UserID: acad.ID.String(), UserType: "academia", IP: "127.0.0.1"}); err != nil {
			t.Fatalf("salvar turma: %v", err)
		}
		return tr
	}
	ativa := makeTurma("1A", academia, false)
	inativa := makeTurma("1B", academia, true)
	outraTurma := makeTurma("1C", outra, false)
	ativa2 := makeTurma("1D", academia, false)
	if err := projections.NewTurmasProjection(client).Rebuild(); err != nil {
		t.Fatalf("rebuild turmas: %v", err)
	}
	token, err := middleware.GenerateToken(academia.ID, "academia")
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	return &turmaVinculoFixture{client: client, repository: repository, router: setupRouter(), academia: academia, outraAcademia: outra, turmaAtiva: ativa, turmaInativa: inativa, turmaOutra: outraTurma, turmaAtiva2: ativa2, token: token}
}

func montarMultipartCadastroEstudante(t *testing.T, campos map[string]string, comArquivos bool) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	for k, v := range campos {
		if err := w.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	if comArquivos {
		for _, f := range []string{"bi_encarregado", "cedula_estudante"} {
			header := make(textproto.MIMEHeader)
			header.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, f, f+".pdf"))
			header.Set("Content-Type", "application/pdf")
			part, err := w.CreatePart(header)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := part.Write([]byte("%PDF-1.4\n%%EOF")); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return body, w.FormDataContentType()
}

func geraDigitosTurmaVinculo(n int) string {
	digitos := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, uuid.NewString())
	for len(digitos) < n {
		digitos += "0"
	}
	return digitos[:n]
}
func camposCadastro(nome string) map[string]string {
	return map[string]string{"nome": nome, "genero": "masculino", "data_nascimento": "2014-01-01", "ano_escolar_fundamental": "1_ano_fundamental", "telefone_encarregado": "9" + geraDigitosTurmaVinculo(8), "bilhete_identidade_encarregado": "BI" + strings.ReplaceAll(nome, " ", "")}
}

func postCadastro(t *testing.T, fx *turmaVinculoFixture, campos map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	b, ct := montarMultipartCadastroEstudante(t, campos, true)
	req := httptest.NewRequest(http.MethodPost, "/academia/estudante/register", b)
	req.Header.Set("Authorization", "Bearer "+fx.token)
	req.Header.Set("Content-Type", ct)
	w := httptest.NewRecorder()
	fx.router.ServeHTTP(w, req)
	return w
}
func decodeMap(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("json %v: %s", err, b)
	}
	return m
}
func dataMap(m map[string]any) map[string]any {
	if d, ok := m["data"].(map[string]any); ok {
		return d
	}
	return m
}
func codigoEstudante(m map[string]any) string {
	d := dataMap(m)
	for _, k := range []string{"codigo_estudante", "codigo"} {
		if v, ok := d[k].(string); ok {
			return v
		}
	}
	return ""
}
func estudanteCount(t *testing.T, fx *turmaVinculoFixture, nome string) int {
	t.Helper()
	var n int
	if err := fx.client.DB().QueryRow(`SELECT count(*) FROM projection_estudantes WHERE nome=$1`, nome).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestTurmaVinculo01CadastroIndividualSemCodigoTurmaMantemRespostaSemCamposDeVinculo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fx := setupTurmaVinculoIntegration(t)
	w := postCadastro(t, fx, camposCadastro("Aluno sem turma"))
	if w.Code != 201 {
		t.Fatalf("status=%d %s", w.Code, w.Body.String())
	}
	d := dataMap(decodeMap(t, w.Body.Bytes()))
	if _, ok := d["turma_vinculada"]; ok {
		t.Fatal("turma_vinculada inesperado")
	}
	if _, ok := d["turma_aviso"]; ok {
		t.Fatal("turma_aviso inesperado")
	}
}
func TestTurmaVinculo02CadastroIndividualComCodigoTurmaValidoVinculaEstudante(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fx := setupTurmaVinculoIntegration(t)
	c := camposCadastro("Aluno vinculado")
	c["codigo_turma"] = fx.turmaAtiva.CodigoTurma
	w := postCadastro(t, fx, c)
	if w.Code != 201 {
		t.Fatalf("status=%d %s", w.Code, w.Body.String())
	}
	m := decodeMap(t, w.Body.Bytes())
	if dataMap(m)["turma_vinculada"] != true {
		t.Fatalf("sem vinculo: %s", w.Body.String())
	}
	_ = projections.NewTurmasProjection(fx.client).Rebuild()
	dto, _ := projections.NewTurmasProjection(fx.client).GetByCodigoTurma(fx.turmaAtiva.CodigoTurma, fx.academia.CodigoAcademia)
	cod := codigoEstudante(m)
	found := false
	for _, e := range dto.Estudantes {
		if e == cod {
			found = true
		}
	}
	if !found {
		t.Fatalf("estudante %s não vinculado", cod)
	}
}
func TestTurmaVinculo03CadastroIndividualComCodigoTurmaInexistenteRetorna404ENaoCriaEstudante(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fx := setupTurmaVinculoIntegration(t)
	c := camposCadastro("Aluno turma inexistente")
	c["codigo_turma"] = "TURMA_INEXISTENTE"
	w := postCadastro(t, fx, c)
	if w.Code != 404 {
		t.Fatalf("status=%d %s", w.Code, w.Body.String())
	}
	if estudanteCount(t, fx, c["nome"]) != 0 {
		t.Fatal("estudante criado")
	}
}
func TestTurmaVinculo04CadastroIndividualComCodigoTurmaDeOutraAcademiaRetorna404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fx := setupTurmaVinculoIntegration(t)
	c := camposCadastro("Aluno turma outra academia")
	c["codigo_turma"] = fx.turmaOutra.CodigoTurma
	w := postCadastro(t, fx, c)
	if w.Code != 404 {
		t.Fatalf("status=%d %s", w.Code, w.Body.String())
	}
	if estudanteCount(t, fx, c["nome"]) != 0 {
		t.Fatal("estudante criado")
	}
}
func TestTurmaVinculo05CadastroIndividualComCodigoTurmaInativaRetorna400ENaoCriaEstudante(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fx := setupTurmaVinculoIntegration(t)
	c := camposCadastro("Aluno turma inativa")
	c["codigo_turma"] = fx.turmaInativa.CodigoTurma
	w := postCadastro(t, fx, c)
	if w.Code != 400 {
		t.Fatalf("status=%d %s", w.Code, w.Body.String())
	}
	if estudanteCount(t, fx, c["nome"]) != 0 {
		t.Fatal("estudante criado")
	}
}
func TestTurmaVinculo06CadastroIndividualComTurmaIncompativelRetorna400ENaoCriaEstudante(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fx := setupTurmaVinculoIntegration(t)
	c := camposCadastro("Aluno turma incompativel")
	c["codigo_turma"] = fx.turmaAtiva.CodigoTurma
	c["ano_escolar_fundamental"] = "2_ano_fundamental"
	w := postCadastro(t, fx, c)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "incompat") {
		t.Fatalf("status=%d %s", w.Code, w.Body.String())
	}
	if estudanteCount(t, fx, c["nome"]) != 0 {
		t.Fatal("estudante criado")
	}
}
func TestTurmaVinculo07CadastroComVinculoNaoDependeDeReleituraDaProjecaoDeEstudantes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fx := setupTurmaVinculoIntegration(t)
	c := camposCadastro("Aluno sem rebuild estudante")
	c["codigo_turma"] = fx.turmaAtiva.CodigoTurma
	w := postCadastro(t, fx, c)
	if w.Code != 201 || dataMap(decodeMap(t, w.Body.Bytes()))["turma_vinculada"] != true {
		t.Fatalf("status=%d %s", w.Code, w.Body.String())
	}
}

func jobCadastro(t *testing.T, fx *turmaVinculoFixture, item map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(item)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/jobs/item", bytes.NewReader(b))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user_id", fx.academia.ID)
	c.Set("user_type", "academia")
	c.Set("dbClient", fx.client)
	c.Set("repository", fx.repository)
	c.Set("projManager", projManager)
	handlers.RegisterEstudantePorAcademiaJobItem(c)
	return w
}
func TestTurmaVinculo08CadastroEmMassaTrataItensComESemCodigoTurmaDeFormaIndependente(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fx := setupTurmaVinculoIntegration(t)

	item0 := camposCadastro("Aluno job Um")
	w0 := jobCadastro(t, fx, item0)
	if w0.Code < 200 || w0.Code > 299 {
		t.Fatalf("item sem turma: status=%d %s", w0.Code, w0.Body.String())
	}
	if v, ok := dataMap(decodeMap(t, w0.Body.Bytes()))["turma_vinculada"]; ok {
		t.Fatalf("item sem turma não deveria ter turma_vinculada no payload: %v", v)
	}

	item1 := camposCadastro("Aluno job Dois")
	item1["codigo_turma"] = fx.turmaAtiva.CodigoTurma
	w1 := jobCadastro(t, fx, item1)
	if w1.Code < 200 || w1.Code > 299 || dataMap(decodeMap(t, w1.Body.Bytes()))["turma_vinculada"] != true {
		t.Fatalf("item com turma válida: status=%d %s", w1.Code, w1.Body.String())
	}

	item2 := camposCadastro("Aluno job Tres")
	item2["codigo_turma"] = "TURMA_INEXISTENTE"
	w2 := jobCadastro(t, fx, item2)
	if w2.Code != 404 {
		t.Fatalf("item com turma inexistente deveria falhar com 404: status=%d %s", w2.Code, w2.Body.String())
	}
	if estudanteCount(t, fx, item2["nome"]) != 0 {
		t.Fatal("item com turma inexistente não deveria criar estudante")
	}
}
func TestTurmaVinculo09FalhaPosCriacaoGeraTurmaAvisoSemAbortarCadastro(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fx := setupTurmaVinculoIntegration(t)
	// A turma existe, está ativa e é compatível — passa na pré-checagem de
	// registerEstudantePorAcademiaComRequestModo. Mas a academia usada aqui
	// nunca teve DefinirAnoLetivo chamado, então vincularEstudanteATurma
	// falha depois de o estudante já ter sido persistido (resolverAnoLetivoAcademia
	// erro), gerando turma_vinculada=false + turma_aviso sem abortar o cadastro.
	academiaSemAnoLetivo := criarAcademiaCorrecao(t, fx.repository, time.Now().UnixNano(), "Z")
	if err := projections.NewAcademiaProjection(fx.client).Rebuild(); err != nil {
		t.Fatalf("rebuild academias: %v", err)
	}
	turma := aggregates.NewTurma()
	if err := turma.Criar("9A", academiaSemAnoLetivo.CodigoAcademia, "1_ano_fundamental", nil, "manha", academiaSemAnoLetivo.ID); err != nil {
		t.Fatalf("criar turma sem ano letivo: %v", err)
	}
	if err := fx.repository.SaveWithAudit(turma, db.AuditContext{UserID: academiaSemAnoLetivo.ID.String(), UserType: "academia", IP: "127.0.0.1"}); err != nil {
		t.Fatalf("salvar turma sem ano letivo: %v", err)
	}
	if err := projections.NewTurmasProjection(fx.client).Rebuild(); err != nil {
		t.Fatalf("rebuild turmas: %v", err)
	}
	tokenSemAnoLetivo, err := middleware.GenerateToken(academiaSemAnoLetivo.ID, "academia")
	if err != nil {
		t.Fatalf("token: %v", err)
	}

	item := camposCadastro("Aluno job aviso")
	item["codigo_turma"] = "9A"
	b, _ := json.Marshal(item)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/jobs/item", bytes.NewReader(b))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user_id", academiaSemAnoLetivo.ID)
	c.Set("user_type", "academia")
	c.Set("dbClient", fx.client)
	c.Set("repository", fx.repository)
	c.Set("projManager", projManager)
	_ = tokenSemAnoLetivo
	handlers.RegisterEstudantePorAcademiaJobItem(c)

	if w.Code < 200 || w.Code > 299 || !strings.Contains(w.Body.String(), "turma_aviso") {
		t.Fatalf("status=%d %s", w.Code, w.Body.String())
	}
	_ = projections.NewEstudanteProjection(fx.client).Rebuild()
	if estudanteCount(t, fx, item["nome"]) != 1 {
		t.Fatal("estudante deveria ter sido criado apesar da falha de vínculo")
	}
}
func TestTurmaVinculo10ConflitoOtimistaNoVinculoTemRetryOuFalhaLimpaSemCorromperTurma(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fx := setupTurmaVinculoIntegration(t)
	codes := make([]string, 2)
	for i := range codes {
		c := camposCadastro(fmt.Sprintf("Aluno concorrente %d", i))
		w := postCadastro(t, fx, c)
		if w.Code != 201 {
			t.Fatalf("cadastro %d: %d %s", i, w.Code, w.Body.String())
		}
		codes[i] = codigoEstudante(decodeMap(t, w.Body.Bytes()))
	}
	if err := projections.NewEstudanteProjection(fx.client).Rebuild(); err != nil {
		t.Fatalf("rebuild estudantes: %v", err)
	}
	var wg sync.WaitGroup
	for _, cod := range codes {
		wg.Add(1)
		go func(cod string) {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "/academia/turma/"+fx.turmaAtiva.CodigoTurma+"/estudante", strings.NewReader(`{"codigo_estudante":"`+cod+`"}`))
			req.Header.Set("Authorization", "Bearer "+fx.token)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			fx.router.ServeHTTP(w, req)
		}(cod)
	}
	wg.Wait()
	_ = projections.NewTurmasProjection(fx.client).Rebuild()
	dto, _ := projections.NewTurmasProjection(fx.client).GetByCodigoTurma(fx.turmaAtiva.CodigoTurma, fx.academia.CodigoAcademia)
	if len(dto.Estudantes) < 2 {
		t.Fatalf("vinculos perdidos: %+v", dto.Estudantes)
	}
}
func TestTurmaVinculo11AdicionarEstudanteATurmaRotaManualPreservaStatusERegraDuplicidade(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fx := setupTurmaVinculoIntegration(t)
	c := camposCadastro("Aluno rota manual")
	w := postCadastro(t, fx, c)
	if w.Code != 201 {
		t.Fatalf("cadastro: %d %s", w.Code, w.Body.String())
	}
	cod := codigoEstudante(decodeMap(t, w.Body.Bytes()))
	if err := projections.NewEstudanteProjection(fx.client).Rebuild(); err != nil {
		t.Fatalf("rebuild estudantes: %v", err)
	}
	call := func(turma string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/academia/turma/"+turma+"/estudante", strings.NewReader(`{"codigo_estudante":"`+cod+`"}`))
		req.Header.Set("Authorization", "Bearer "+fx.token)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		fx.router.ServeHTTP(rr, req)
		return rr
	}
	if rr := call(fx.turmaAtiva.CodigoTurma); rr.Code != 200 {
		t.Fatalf("ativo: %d %s", rr.Code, rr.Body.String())
	}
	if err := projections.NewTurmasProjection(fx.client).Rebuild(); err != nil {
		t.Fatalf("rebuild turmas: %v", err)
	}
	if rr := call("TURMA_INEXISTENTE"); rr.Code != 404 {
		t.Fatalf("inexistente: %d %s", rr.Code, rr.Body.String())
	}
	if rr := call(fx.turmaAtiva2.CodigoTurma); rr.Code != 400 || !strings.Contains(rr.Body.String(), "já pertence") {
		t.Fatalf("duplicado: %d %s", rr.Code, rr.Body.String())
	}
}

var _ io.Reader
