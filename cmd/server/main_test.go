package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func assertStandardErrorEnvelope(t *testing.T, w *httptest.ResponseRecorder, expectedStatus int, expectedError string) {
	t.Helper()

	if w.Code != expectedStatus {
		t.Fatalf("expected status %d, got %d. body=%s", expectedStatus, w.Code, w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("expected JSON error envelope, got unparseable body %q: %v", w.Body.String(), err)
	}
	if body["error"] != expectedError {
		t.Fatalf("expected error=%q, got %v. body=%v", expectedError, body["error"], body)
	}
	if message, ok := body["message"].(string); !ok || message == "" {
		t.Fatalf("expected non-empty message in standard error envelope. body=%v", body)
	}
	if requestID, ok := body["request_id"].(string); !ok || requestID == "" {
		t.Fatalf("expected non-empty request_id in standard error envelope. body=%v", body)
	}
}

func TestAdminSistemaAnoLetivoRouteIsRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := setupRouter()

	req := httptest.NewRequest(http.MethodPost, "/admin/definir-ano-letivo-geral", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code == http.StatusNotFound {
		t.Fatalf("expected /admin/definir-ano-letivo-geral to be registered, got 404")
	}
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected registered route to require authentication with 401, got %d", w.Code)
	}
}

func TestDominisAcademiaRegisterUnauthorizedUsesStandardErrorEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := setupRouter()

	req := httptest.NewRequest(http.MethodPost, "/dominis/academia/register", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertStandardErrorEnvelope(t, w, http.StatusUnauthorized, "UNAUTHORIZED")
}

func TestGlobalAnoLetivoReadRoutesRequireOnlyAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := setupRouter()

	for _, path := range []string{"/ano-letivo", "/anos-letivos-lista"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code == http.StatusNotFound {
			t.Fatalf("expected %s to be registered, got 404", path)
		}
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected %s to require authentication with 401, got %d", path, w.Code)
		}
	}
}

func TestDocumentoRoutesRequireAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := setupRouter()
	for _, path := range []string{
		"/documentos/academias/ACA001/alvara/download",
		"/documentos/estudantes/EST001/bi_estudante/download",
		"/documentos/solicitacoes-matricula/SOL001/bi_estudante/download",
		"/estudante/documentos",
		"/estudante/documentos/bi_estudante/download",
		"/academia/documentos",
		"/academia/documentos/academia/alvara/download",
		"/academia/documentos/estudantes/EST001/bi_estudante/download",
		"/academia/documentos/solicitacoes-matricula/SOL001/bi_estudante/download",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code == http.StatusNotFound {
			t.Fatalf("expected %s to be registered, got 404", path)
		}
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected %s to require authentication with 401, got %d", path, w.Code)
		}
	}
}

func TestDefinirAnoLetivoSeguinteRouteIsRemoved(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := setupRouter()

	req := httptest.NewRequest(http.MethodPost, "/definir-ano-letivo-seguinte", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected /definir-ano-letivo-seguinte to be removed with 404, got %d", w.Code)
	}
}

func TestLegacyAdminSistemaAnoLetivoReadRoutesAreRemoved(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := setupRouter()

	for _, path := range []string{"/admin/sistema/ano-letivo", "/admin/sistema/anos-letivos-lista"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected legacy GET %s to be removed with 404, got %d", path, w.Code)
		}
	}
}

func TestLegacyAnoLetivoWriteAliasesAreRemoved(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := setupRouter()

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/admin/sistema/ano-letivo"},
		{http.MethodPost, "/academia/ano-letivo"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected legacy %s %s to be removed with 404, got %d", tc.method, tc.path, w.Code)
		}
	}
}

func TestLegacyDominisSistemaAnoLetivoRouteIsRemoved(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := setupRouter()

	req := httptest.NewRequest(http.MethodPost, "/dominis/sistema/ano-letivo", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected /dominis/sistema/ano-letivo to be removed with 404, got %d", w.Code)
	}
}

func TestAcademiaCursosPublicRouteAllowsMissingAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := setupRouter()

	req := httptest.NewRequest(http.MethodGet, "/academia/cursos", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code == http.StatusUnauthorized {
		t.Fatalf("expected /academia/cursos to allow requests without authentication, got 401")
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected public route to validate missing codigo_academia with 400, got %d", w.Code)
	}
}

func TestAcademiaCursoPublicRouteAllowsMissingAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := setupRouter()

	req := httptest.NewRequest(http.MethodGet, "/academia/curso/not-a-uuid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code == http.StatusUnauthorized {
		t.Fatalf("expected /academia/curso/:id to allow requests without authentication, got 401")
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected public route to validate invalid curso ID with 400, got %d", w.Code)
	}
}

func TestAcademiaAnosAcademicosRoutesExposeOnlyGetPostDelete(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := setupRouter()

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/academia/anos-academicos"},
		{http.MethodPost, "/academia/anos-academicos"},
		{http.MethodDelete, "/academia/anos-academicos"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code == http.StatusNotFound {
			t.Fatalf("expected %s %s to be registered, got 404", tc.method, tc.path)
		}
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected registered %s %s to require authentication with 401, got %d", tc.method, tc.path, w.Code)
		}
	}

	req := httptest.NewRequest(http.MethodPatch, "/academia/anos-academicos", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected PATCH /academia/anos-academicos to be removed with 404, got %d", w.Code)
	}
}

func TestNotasAndFaltasExposeApenasRotasSuportadas(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := setupRouter()

	registered := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/academia/notas-aluno"},
		{http.MethodPost, "/academia/notas-aluno/async"},
		{http.MethodPost, "/academia/faltas-aluno"},
		{http.MethodPost, "/academia/faltas-aluno/async"},
		{http.MethodPatch, "/academia/notas-aluno/00000000-0000-0000-0000-000000000000"},
		{http.MethodPatch, "/academia/faltas-aluno/00000000-0000-0000-0000-000000000000"},
		{http.MethodGet, "/notas"},
		{http.MethodGet, "/faltas"},
		{http.MethodGet, "/notas-estudante/ABC1234"},
		{http.MethodGet, "/faltas-estudante/ABC1234"},
	}
	for _, tc := range registered {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code == http.StatusNotFound {
			t.Fatalf("expected %s %s to be registered, got 404", tc.method, tc.path)
		}
	}

	removed := []struct {
		method string
		path   string
	}{
		{http.MethodPut, "/academia/notas-aluno"},
		{http.MethodPatch, "/academia/notas-aluno"},
		{http.MethodDelete, "/academia/notas-aluno"},
		{http.MethodPut, "/academia/atualizar-nota"},
		{http.MethodPatch, "/academia/atualizar-nota"},
		{http.MethodDelete, "/academia/nota/00000000-0000-0000-0000-000000000000"},
		{http.MethodPut, "/academia/atualizar-nota/async"},
		{http.MethodDelete, "/academia/nota/async"},
		{http.MethodPut, "/academia/faltas-aluno"},
		{http.MethodPatch, "/academia/faltas-aluno"},
		{http.MethodDelete, "/academia/faltas-aluno"},
		{http.MethodPut, "/academia/atualizar-falta"},
		{http.MethodPatch, "/academia/atualizar-falta"},
		{http.MethodDelete, "/academia/falta/00000000-0000-0000-0000-000000000000"},
		{http.MethodPut, "/academia/atualizar-falta/async"},
		{http.MethodDelete, "/academia/falta/async"},
	}
	for _, tc := range removed {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected %s %s to be removed with 404, got %d", tc.method, tc.path, w.Code)
		}
	}
}

func TestInitStorageDoesNotFallbackToLocalWhenMegaConfigurationFails(t *testing.T) {
	t.Setenv("STORAGE_PROVIDER", "mega")
	t.Setenv("MEGA_EMAIL", "")
	t.Setenv("MEGA_PASSWORD", "")
	t.Setenv("ENV", "development")

	if err := initStorage(); err == nil {
		t.Fatal("initStorage() error = nil, want Mega configuration error instead of local fallback")
	}
}

func TestLegacyStudentProgressionAndInterruptionRoutesAreRemoved(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := setupRouter()

	for _, path := range []string{
		"/academia/estudante/EST001/matricula/fundamental",
		"/academia/estudante/EST001/matricula/medio",
		"/academia/estudante/EST001/matricula/superior",
		"/academia/estudante/EST001/interrupcao/fundamental",
		"/academia/estudante/EST001/interrupcao/medio",
		"/academia/estudante/EST001/trancamento/superior",
	} {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected legacy POST %s to be removed with 404, got %d", path, w.Code)
		}
	}
}

func TestLegacyPaymentRoutesAreRemoved(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := setupRouter()

	const idPlaceholder = "00000000-0000-0000-0000-000000000000"

	removed := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/finan" + "ceiro/app" + "ypay/credenciais/" + idPlaceholder},
		{http.MethodPost, "/finan" + "ceiro/app" + "ypay/credenciais/" + idPlaceholder + "/testar"},
		{http.MethodPost, "/finan" + "ceiro/app" + "ypay/credenciais/" + idPlaceholder + "/ativar"},
		{http.MethodPost, "/finan" + "ceiro/app" + "ypay/credenciais/" + idPlaceholder + "/desativar"},
		{http.MethodPost, "/finan" + "ceiro/modalidade-pagamento"},
	}
	for _, tc := range removed {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected %s %s to be removed with 404, got %d", tc.method, tc.path, w.Code)
		}
	}

	// The base module deliberately restores only its documented configuration
	// collection routes; authentication must protect them before validation.
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/financeiro/appypay/credenciais"},
		{http.MethodPut, "/financeiro/appypay/credenciais/" + idPlaceholder},
		{http.MethodGet, "/financeiro/appypay/credenciais"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected protected %s %s to return 401 without a token, got %d", tc.method, tc.path, w.Code)
		}
	}
}
