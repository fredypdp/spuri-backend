package services

import (
	"strings"
	"testing"
)

func TestLoadSMTPConfigMissingVarsDisabled(t *testing.T) {
	t.Setenv("EMAIL_HOST", "")
	t.Setenv("EMAIL_PORT", "")
	t.Setenv("EMAIL_USER", "")
	t.Setenv("EMAIL_PASS", "")
	t.Setenv("EMAIL_FROM", "")
	if _, ok := loadSMTPConfig(); ok {
		t.Fatal("esperava SMTP desabilitado")
	}
}
func TestLoadSMTPConfigDefaultsPortAndFrom(t *testing.T) {
	t.Setenv("EMAIL_HOST", "smtp.gmail.com")
	t.Setenv("EMAIL_PORT", "")
	t.Setenv("EMAIL_USER", "spuri@example.com")
	t.Setenv("EMAIL_PASS", "senha-de-app")
	t.Setenv("EMAIL_FROM", "")
	cfg, ok := loadSMTPConfig()
	if !ok || cfg.Port != "587" || cfg.From != cfg.User {
		t.Fatalf("configuração inesperada: %#v, ok=%v", cfg, ok)
	}
}
func TestSendAcademiaCadastradaEmailSMTPDisabledDoesNotError(t *testing.T) {
	t.Setenv("EMAIL_HOST", "")
	t.Setenv("EMAIL_USER", "")
	t.Setenv("EMAIL_PASS", "")
	if err := NewEmailService(nil).SendAcademiaCadastradaEmailSMTP("admin@example.com", "Admin Teste", AcademiaCadastradaInfo{Nome: "Academia Teste"}); err != nil {
		t.Fatal(err)
	}
}
func TestSendAcademiaCadastradaEmailSMTPEmptyRecipientErrors(t *testing.T) {
	t.Setenv("EMAIL_HOST", "smtp.gmail.com")
	t.Setenv("EMAIL_USER", "spuri@example.com")
	t.Setenv("EMAIL_PASS", "senha-de-app")
	if err := NewEmailService(nil).SendAcademiaCadastradaEmailSMTP("", "Admin", AcademiaCadastradaInfo{}); err == nil {
		t.Fatal("esperava erro")
	}
}
func TestBuildMimeMessageContainsExpectedParts(t *testing.T) {
	s := string(buildMimeMessage("remetente@spuri.co", "admin@example.com", "Jose Admin", "Assunto", "corpo em texto puro", "<p>corpo em <strong>html</strong></p>"))
	for _, want := range []string{"MIME-Version: 1.0", "Content-Type: multipart/alternative; boundary=spuri-smtp-boundary-7f3a", "Content-Type: text/plain; charset=UTF-8", "corpo em texto puro", "Content-Type: text/html; charset=UTF-8", "<strong>html</strong>", "admin@example.com", "remetente@spuri.co"} {
		if !strings.Contains(s, want) {
			t.Errorf("mensagem MIME nao contem %q", want)
		}
	}
	if !strings.Contains(s, "\r\n") {
		t.Error("mensagem MIME deveria usar CRLF")
	}
}
func TestBuildMimeMessageEncodesAccentedHeaders(t *testing.T) {
	s := string(buildMimeMessage("remetente@spuri.co", "admin@example.com", "José Admin", "Nova instituição cadastrada", "corpo", "<p>corpo</p>"))
	if strings.Contains(s, "José") || strings.Contains(s, "instituição") || !strings.Contains(s, "=?UTF-8?") {
		t.Error("cabeçalhos acentuados deveriam ser RFC 2047")
	}
}
func TestRenderAcademiaCadastradaHTMLEscapesFields(t *testing.T) {
	h := renderAcademiaCadastradaHTML("Admin <script>", AcademiaCadastradaInfo{Nome: "Academia & Cia <b>"}, "https://painel.exemplo.com/academias")
	if strings.Contains(h, "<script>") || strings.Contains(h, "Academia & Cia <b>") || !strings.Contains(h, "https://painel.exemplo.com/academias") {
		t.Error("HTML deveria escapar campos e conter o link")
	}
}
