package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAdminSistemaAnoLetivoRouteIsRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := setupRouter()

	req := httptest.NewRequest(http.MethodPost, "/admin/sistema/ano-letivo", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code == http.StatusNotFound {
		t.Fatalf("expected /admin/sistema/ano-letivo to be registered, got 404")
	}
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected registered route to require authentication with 401, got %d", w.Code)
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
