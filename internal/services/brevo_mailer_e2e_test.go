package services

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSendBrevoEmailToURLAgainstRealServer(t *testing.T) {
	var gotMethod, gotAPIKey, gotContentType string
	var gotBody brevoSendRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotAPIKey = r.Header.Get("api-key")
		gotContentType = r.Header.Get("Content-Type")

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("erro lendo corpo recebido: %v", err)
		}
		if err := json.Unmarshal(body, &gotBody); err != nil {
			t.Fatalf("corpo recebido não é JSON válido: %v\n%s", err, body)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"messageId":"<teste@relay.brevo.com>"}`))
	}))
	defer server.Close()

	cfg := BrevoConfig{APIKey: "xkeysib-teste-123", SenderEmail: "spuriartipan@gmail.com", SenderName: "Spuri"}
	err := sendBrevoEmailToURL(cfg, server.URL, "admin@example.com", "Admin Teste", "Nova instituição cadastrada: Academia Teste", "corpo em texto puro", "<p>corpo em <strong>html</strong></p>")
	if err != nil {
		t.Fatalf("sendBrevoEmailToURL contra servidor mock falhou: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("esperava método POST, obteve %s", gotMethod)
	}
	if gotAPIKey != "xkeysib-teste-123" {
		t.Errorf("esperava header api-key com a chave configurada, obteve %q", gotAPIKey)
	}
	if !strings.Contains(gotContentType, "application/json") {
		t.Errorf("esperava Content-Type application/json, obteve %q", gotContentType)
	}
	if gotBody.Sender.Email != "spuriartipan@gmail.com" || len(gotBody.To) != 1 || gotBody.To[0].Email != "admin@example.com" {
		t.Errorf("remetente ou destinatário incorreto: %+v", gotBody)
	}
	if gotBody.Subject != "Nova instituição cadastrada: Academia Teste" || !strings.Contains(gotBody.HTMLContent, "<strong>html</strong>") || gotBody.TextContent != "corpo em texto puro" {
		t.Errorf("conteúdo incorreto: %+v", gotBody)
	}
}

func TestSendBrevoEmailToURLPropagatesAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":"unauthorized","message":"Key not found"}`))
	}))
	defer server.Close()

	cfg := BrevoConfig{APIKey: "chave-invalida", SenderEmail: "spuriartipan@gmail.com", SenderName: "Spuri"}
	err := sendBrevoEmailToURL(cfg, server.URL, "admin@example.com", "Admin Teste", "Assunto", "texto", "<p>html</p>")
	if err == nil {
		t.Fatal("esperava erro para resposta 401 do Brevo")
	}
	if !strings.Contains(err.Error(), "Key not found") {
		t.Errorf("esperava mensagem da API no erro Go, obteve: %v", err)
	}
}

func TestSendAcademiaCadastradaEmailBrevoAgainstRealServer(t *testing.T) {
	var gotBody brevoSendRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"messageId":"<teste@relay.brevo.com>"}`))
	}))
	defer server.Close()

	cfg := BrevoConfig{APIKey: "xkeysib-teste", SenderEmail: "spuriartipan@gmail.com", SenderName: "Spuri"}
	info := AcademiaCadastradaInfo{Nome: "Academia Teste", CodigoAcademia: "LDA2026A001", NIF: "5417845812", Nivel: "escola", Type: "private", Provincia: "LDA"}
	err := sendBrevoEmailToURL(cfg, server.URL, "admin@example.com", "Admin Teste", "Nova instituição cadastrada: Academia Teste", "texto", renderAcademiaCadastradaHTML("Admin Teste", info, "https://painel.exemplo.com/academias"))
	if err != nil {
		t.Fatalf("sendBrevoEmailToURL com HTML real falhou: %v", err)
	}
	for _, want := range []string{"LDA2026A001", "5417845812", "https://painel.exemplo.com/academias"} {
		if !strings.Contains(gotBody.HTMLContent, want) {
			t.Errorf("HTML enviado não contém %q", want)
		}
	}
}
