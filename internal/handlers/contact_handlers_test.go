package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRejectDedicatedContactFieldsRejectsEmailAndPreservesAtomicity(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPut, "/academia/dados", strings.NewReader(`{"nome":"Academia Nova","email":"novo@example.com"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	if !rejectDedicatedContactFields(c) {
		t.Fatalf("esperava rejeição de email em rota genérica")
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status esperado 400, obtido %d", w.Code)
	}
	body := w.Body.String()
	for _, expected := range []string{"email", "campo_nao_permitido", "PUT /me/email"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("resposta não orienta rota dedicada para email; faltou %q em %s", expected, body)
		}
	}
}

func TestRejectDedicatedContactFieldsRejectsTelefoneAndPreservesAtomicity(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPut, "/estudante/dados-pessoais", strings.NewReader(`{"nome":"Aluno Novo","telefone":"923456789"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	if !rejectDedicatedContactFields(c) {
		t.Fatalf("esperava rejeição de telefone em rota genérica")
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status esperado 400, obtido %d", w.Code)
	}
	body := w.Body.String()
	for _, expected := range []string{"telefone", "campo_nao_permitido", "PUT /me/telefone"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("resposta não orienta rota dedicada para telefone; faltou %q em %s", expected, body)
		}
	}
}
