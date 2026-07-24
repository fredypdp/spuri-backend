package handlers

import (
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRejectRemovedJSONFieldsFindsNestedBatchStudentFields(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/academia/estudante/register/async", strings.NewReader(`{"com_arquivo":false,"estudantes":[{"telefone_responsavel":"924000000"}]}`))
	c.Request.Header.Set("Content-Type", "application/json")

	if !rejectRemovedJSONFields(c) {
		t.Fatalf("esperava rejeição do campo removido no JSON aninhado do lote")
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status esperado 400, obtido %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "telefone_responsavel") || !strings.Contains(body, "telefone_encarregado") || !strings.Contains(body, "campo_removido") {
		t.Fatalf("resposta não orienta troca do campo removido: %s", body)
	}
}

func TestFindRemovedJSONFieldStringFindsMultipartStudentPayloadFields(t *testing.T) {
	t.Parallel()

	oldField, newField, ok := findRemovedJSONFieldString(`[{"codigo_temporario":"tmp-1","bilhete_identidade_responsavel":"009876543LA089"}]`)
	if !ok {
		t.Fatalf("esperava encontrar campo removido dentro do JSON textual de estudantes")
	}
	if oldField != "bilhete_identidade_responsavel" || newField != "bilhete_identidade_encarregado" {
		t.Fatalf("mapeamento inesperado: %s -> %s", oldField, newField)
	}
}

func TestRejectRemovedMultipartFieldsFindsCompletionUploadField(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/academia/estudante/EST001/documentos", nil)
	c.Request.MultipartForm = &multipart.Form{
		File: map[string][]*multipart.FileHeader{"bi_responsavel": {{Filename: "bi.pdf"}}},
	}

	if !rejectRemovedMultipartFields(c) {
		t.Fatalf("esperava rejeição do campo de arquivo removido")
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status esperado 400, obtido %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "bi_responsavel") || !strings.Contains(body, "bi_encarregado") {
		t.Fatalf("resposta não orienta troca do arquivo removido: %s", body)
	}
}

func TestRejectStudentPersonalImmutableFieldsRejectsDataNascimento(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPut, "/estudante/dados-pessoais", strings.NewReader(`{"nome":"Aluno Novo","data_nascimento":"2010-05-20"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	if !rejectStudentPersonalImmutableFields(c) {
		t.Fatalf("esperava rejeição de data_nascimento em dados pessoais")
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status esperado 400, obtido %d", w.Code)
	}
	body := w.Body.String()
	for _, expected := range []string{"data_nascimento", "campo_imutavel", "não pode ser alterada"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("resposta não contém %q: %s", expected, body)
		}
	}
}
