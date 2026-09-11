---
tarefa: 97
titulo: Enviar o aviso de "nova academia cadastrada" via Brevo (API HTTP) em vez de SMTP puro — contorna bloqueio de porta SMTP na Render
repo: fredypdp/spuri-backend
status: pronta para execução pelo Codex
orquestrado_e_pre_testado_por: Claude
depende_de: tarefa 92 (GetAdminsParaNotificarNovaAcademia, notificarAdminsSobreNovaAcademiaPendente) e tarefa 95 (SendAcademiaCadastradaEmailSMTP, ambas já mescladas em main). Não depende da tarefa 93 deste repositório (módulo de Comunicação SMS) nem da tarefa 93 do spuripainel.
validado_contra: Go 1.24 (via apt), suíte completa existente + 9 testes novos (2 deles contra um servidor HTTP mock real via httptest, cobrindo sucesso e erro da API), tudo reconferido de novo aplicando os diffs via `git apply` num clone limpo do main atual
---

# Como usar este documento

A tarefa 95 (`internal/services/smtp_mailer.go`, `SendAcademiaCadastradaEmailSMTP`) já está em produção, mas em ambientes como a **Render (plano gratuito)** o envio falha com `connect: connection timed out` na porta 587 — confirmado no changelog oficial da Render: desde setembro de 2025, serviços web gratuitos bloqueiam tráfego de saída para as portas SMTP 25, 465 e 587. Não é uma questão de porta errada (465 também está bloqueada) nem de credencial errada (a conexão nem chega a autenticar).

Esta tarefa troca o transporte do aviso de "nova academia cadastrada" de SMTP para a **API HTTP do Brevo** (`https://api.brevo.com/v3/smtp/email`) — uma chamada HTTPS comum (porta 443), que nenhuma hospedagem de nuvem bloqueia (é a mesma porta usada por qualquer chamada de API externa, inclusive as que este projeto já faz para o EmailJS). Plano gratuito do Brevo: 300 emails/dia (no momento em que isto foi escrito — confirme o limite atual em brevo.com/pricing), exige só verificar um remetente (sem precisar de domínio próprio).

Aplique os diffs na ordem desta seção. Não invente fallback automático entre SMTP e Brevo, nem tente detectar automaticamente qual transporte usar — a decisão de qual usar é manual, por configuração de ambiente (documentada no `.env.example`), e o handler chama só um dos dois de cada vez. Ao final, rode `go build ./...`, `go vet ./...`, `go test ./...` (nenhum teste desta tarefa precisa de banco nem de rede real — o teste end-to-end sobe um servidor HTTP mock local via `httptest`) e confira o checklist da seção 4.

## 1. O que muda, e o que não muda

- **Muda**: qual função `notificarAdminsSobreNovaAcademiaPendente` chama — antes `SendAcademiaCadastradaEmailSMTP`, agora `SendAcademiaCadastradaEmailBrevo`.
- **Não muda**: `GetAdminsParaNotificarNovaAcademia` (busca de destinatários continua 100% automática, vinda do banco), a lógica de loop/log em `notificarAdminsSobreNovaAcademiaPendente`, `RegisterAcademiaPublica`, a resposta HTTP do cadastro, `renderAcademiaCadastradaHTML` (reaproveitado tal como está — o HTML do email é idêntico ao da versão SMTP).
- `SendAcademiaCadastradaEmail` (EmailJS) e `SendAcademiaCadastradaEmailSMTP` (SMTP puro) **continuam existindo no arquivo**, só deixam de ser chamadas por este handler — úteis se algum ambiente futuro não tiver o bloqueio de porta (VPS próprio, plano pago da Render) e preferir voltar a SMTP puro sem depender de um serviço de terceiro.
- Nenhuma migration, nenhuma mudança no `spuripainel`, nenhuma dependência nova em `go.mod` (é só `net/http`/`encoding/json`, já usados em todo o projeto).

## 2. Diffs a aplicar, nesta ordem

Todos os diffs foram extraídos com `git diff` a partir de uma cópia limpa do `main` atual e reconferidos aplicando-os com `git apply` contra um clone limpo antes de fechar este documento.

### Arquivo NOVO 1/6 — `internal/services/brevo_mailer.go`

```go
// internal/services/brevo_mailer.go
//
// Envio de email via a API HTTP do Brevo (https://api.brevo.com/v3/smtp/email)
// — alternativa ao SMTP puro (smtp_mailer.go) para hospedagens que bloqueiam
// tráfego de saída nas portas SMTP (25/465/587), como o plano gratuito da
// Render (confirmado no changelog oficial da Render: bloqueio de saída nas
// portas 25/465/587 em serviços web gratuitos). Como isto é uma chamada
// HTTPS comum (porta 443), nenhum provedor de nuvem bloqueia — é a mesma
// porta usada por qualquer chamada de API externa já feita neste projeto
// (EmailJS, Mega, etc.).
//
// Plano gratuito do Brevo: 300 emails/dia no momento em que isto foi escrito
// (conferir limite atual em https://www.brevo.com/pricing/, pode mudar).
// Exige verificar UM único remetente — não precisa de domínio próprio nem
// de configuração de DNS — em https://app.brevo.com/settings/senders. O
// mesmo email já usado em EMAIL_USER/EMAIL_FROM (SMTP) serve como
// remetente aqui também, só precisa ser verificado no painel do Brevo.
package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

var brevoHTTPClient = &http.Client{Timeout: 15 * time.Second}

// BrevoConfig agrupa a configuração de envio via API do Brevo.
type BrevoConfig struct {
	APIKey      string
	SenderEmail string // precisa estar verificado em app.brevo.com/settings/senders
	SenderName  string
}

// loadBrevoConfig lê a configuração do ambiente. Reaproveita EMAIL_FROM
// (com fallback para EMAIL_USER, a mesma cadeia já usada por
// loadSMTPConfig) como remetente — a mesma conta já configurada para SMTP
// funciona aqui, só precisa também ser verificada no Brevo. O segundo
// retorno é false se BREVO_API_KEY ou o remetente estiverem ausentes —
// mesma convenção de "desabilitado, só loga, nunca bloqueia" usada pelo
// resto deste pacote (ver EmailService.enabled, loadSMTPConfig).
func loadBrevoConfig() (BrevoConfig, bool) {
	sender := getEnvOrDefault("EMAIL_FROM", os.Getenv("EMAIL_USER"))
	cfg := BrevoConfig{
		APIKey:      os.Getenv("BREVO_API_KEY"),
		SenderEmail: sender,
		SenderName:  "Spuri",
	}
	if cfg.APIKey == "" || cfg.SenderEmail == "" {
		return cfg, false
	}
	return cfg, true
}

type brevoEmailAddress struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

type brevoSendRequest struct {
	Sender      brevoEmailAddress   `json:"sender"`
	To          []brevoEmailAddress `json:"to"`
	Subject     string              `json:"subject"`
	HTMLContent string              `json:"htmlContent"`
	TextContent string              `json:"textContent,omitempty"`
}

type brevoSendResponse struct {
	MessageID string `json:"messageId"`
}

type brevoErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// sendBrevoEmail envia um email via a API REST do Brevo
// (POST https://api.brevo.com/v3/smtp/email). Chamada HTTPS simples —
// nenhuma porta SMTP envolvida, então nenhum bloqueio de firewall de
// hospedagem baseado em porta se aplica aqui.
func sendBrevoEmail(cfg BrevoConfig, toEmail, toName, subject, textBody, htmlBody string) error {
	return sendBrevoEmailToURL(cfg, "https://api.brevo.com/v3/smtp/email", toEmail, toName, subject, textBody, htmlBody)
}

// sendBrevoEmailToURL existe separado de sendBrevoEmail só para permitir
// apontar para um servidor mock nos testes (ver brevo_mailer_e2e_test.go);
// em produção, sempre é chamada via sendBrevoEmail (URL fixa da Brevo).
func sendBrevoEmailToURL(cfg BrevoConfig, url, toEmail, toName, subject, textBody, htmlBody string) error {
	reqBody := brevoSendRequest{
		Sender:      brevoEmailAddress{Email: cfg.SenderEmail, Name: cfg.SenderName},
		To:          []brevoEmailAddress{{Email: toEmail, Name: toName}},
		Subject:     subject,
		HTMLContent: htmlBody,
		TextContent: textBody,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("erro ao serializar email: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("erro ao montar requisição: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("api-key", cfg.APIKey)

	resp, err := brevoHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("erro na requisição ao Brevo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var ok brevoSendResponse
		_ = json.NewDecoder(resp.Body).Decode(&ok) // messageId só é informativo, ignora erro de parse
		return nil
	}

	var apiErr brevoErrorResponse
	_ = json.NewDecoder(resp.Body).Decode(&apiErr)
	if apiErr.Message != "" {
		return fmt.Errorf("brevo respondeu %d: %s (%s)", resp.StatusCode, apiErr.Message, apiErr.Code)
	}
	return fmt.Errorf("brevo respondeu status %d", resp.StatusCode)
}
```

### Arquivo 2/6 — `internal/services/email_service.go` (novo método, ao lado de `SendAcademiaCadastradaEmailSMTP`)

```diff
--- a/internal/services/email_service.go
+++ b/internal/services/email_service.go
@@ -473,6 +473,51 @@ func (s *EmailService) SendAcademiaCadastradaEmailSMTP(adminEmail, adminNome str
 	return nil
 }
 
+// SendAcademiaCadastradaEmailBrevo avisa um administrador (role 'adm' ou
+// 'fpp', email verificado — ver AdminProjection.GetAdminsParaNotificarNovaAcademia)
+// que uma nova academia se autocadastrou via POST /academia/cadastro e está
+// pendente de análise/ativação no painel.
+//
+// Envia via a API HTTP do Brevo (ver brevo_mailer.go) em vez de SMTP puro
+// (SendAcademiaCadastradaEmailSMTP, acima): hospedagens como o plano
+// gratuito da Render bloqueiam tráfego de saída nas portas SMTP
+// (25/465/587), mas nunca bloqueiam HTTPS (443) — a mesma porta usada por
+// qualquer chamada de API externa. Reaproveita o mesmo HTML já usado no
+// caminho SMTP (renderAcademiaCadastradaHTML, abaixo).
+//
+// Esta função é quem internal/handlers/academia_handlers.go chama agora
+// para este aviso. SendAcademiaCadastradaEmail (EmailJS) e
+// SendAcademiaCadastradaEmailSMTP (SMTP puro) continuam existindo neste
+// arquivo, só não são mais chamadas por esse handler — ver decisão
+// registrada na tarefa correspondente.
+//
+// Assim como as demais notificações deste serviço, falha aqui NUNCA deve
+// bloquear o fluxo que a originou — é responsabilidade do chamador apenas
+// logar o erro e seguir.
+func (s *EmailService) SendAcademiaCadastradaEmailBrevo(adminEmail, adminNome string, info AcademiaCadastradaInfo) error {
+	cfg, ok := loadBrevoConfig()
+	if !ok {
+		log.Printf("[EMAIL-BREVO] ⚠️  BREVO_API_KEY/EMAIL_FROM/EMAIL_USER não configurados — aviso de nova academia %s (%s) não enviado para admin %s",
+			info.Nome, info.CodigoAcademia, adminEmail)
+		return nil // não bloqueia: mesmo modo degradado do resto deste serviço
+	}
+	if adminEmail == "" {
+		return fmt.Errorf("email do admin vazio")
+	}
+
+	painelURL := fmt.Sprintf("%s/academias", s.frontendURL)
+	subject := fmt.Sprintf("Nova instituição cadastrada: %s", info.Nome)
+	textBody := fmt.Sprintf(
+		"Olá %s!\n\nA instituição %s (código %s, NIF %s, %s, %s, província %s) concluiu o autocadastro no Spuri e está inativa, aguardando análise.\n\nAcesse o painel para revisar: %s\n",
+		adminNome, info.Nome, info.CodigoAcademia, info.NIF, info.Type, info.Nivel, info.Provincia, painelURL,
+	)
+	htmlBody := renderAcademiaCadastradaHTML(adminNome, info, painelURL)
+	if err := sendBrevoEmail(cfg, adminEmail, adminNome, subject, textBody, htmlBody); err != nil {
+		return fmt.Errorf("brevo: %w", err)
+	}
+	return nil
+}
+
 // renderAcademiaCadastradaHTML monta e escapa o HTML do aviso de cadastro.
 func renderAcademiaCadastradaHTML(adminNome string, info AcademiaCadastradaInfo, painelURL string) string {
 	esc := html.EscapeString
```

Nenhum import novo é necessário neste arquivo — `fmt`, `log` já são importados.

### Arquivo 3/6 — `internal/handlers/academia_handlers.go` (troca a chamada)

```diff
--- a/internal/handlers/academia_handlers.go
+++ b/internal/handlers/academia_handlers.go
@@ -483,7 +483,7 @@ func notificarAdminsSobreNovaAcademiaPendente(c *gin.Context, req RegisterAcadem
 		Provincia:      codigoProvincia,
 	}
 	for _, admin := range admins {
-		if emailErr := emailSvc.SendAcademiaCadastradaEmailSMTP(admin.Email, admin.Nome, info); emailErr != nil {
+		if emailErr := emailSvc.SendAcademiaCadastradaEmailBrevo(admin.Email, admin.Nome, info); emailErr != nil {
 			log.Printf("[WARN] notificarAdminsSobreNovaAcademiaPendente: falha ao notificar admin %s sobre academia %s: %v", admin.Email, codigoAcademia, emailErr)
 		}
 	}
```

### Arquivo 4/6 — `internal/handlers/academia_notificacao_admins_test.go` (atualiza a asserção do teste já existente)

```diff
--- a/internal/handlers/academia_notificacao_admins_test.go
+++ b/internal/handlers/academia_notificacao_admins_test.go
@@ -19,7 +19,7 @@ func TestRegisterAcademiaPublicaNotificaAdminsRegistroAcademia(t *testing.T) {
 
 	mustContain(t, source, "func notificarAdminsSobreNovaAcademiaPendente(")
 	mustContain(t, source, "GetAdminsParaNotificarNovaAcademia()")
-	mustContain(t, source, "SendAcademiaCadastradaEmailSMTP(")
+	mustContain(t, source, "SendAcademiaCadastradaEmailBrevo(")
 
 	publicaBody := extractFuncBody(t, source, "func RegisterAcademiaPublica(")
 	if !strings.Contains(publicaBody, "notificarAdminsSobreNovaAcademiaPendente(") {
```

Confirmei de propósito, antes de escrever este documento, que sem este ajuste o teste quebra sozinho depois do diff 3/6 (a string exata `"SendAcademiaCadastradaEmailSMTP("` deixa de aparecer no arquivo).

### Arquivo 5/6 — `.env.example` (documentação da nova variável + nota no bloco SMTP)

```diff
--- a/.env.example
+++ b/.env.example
@@ -87,9 +87,11 @@ FRONTEND_URL=https://seu-frontend.vercel.app
 # =============================================================================
 # SMTP (envio de email direto pelo backend, sem EmailJS)
 # =============================================================================
-# Usado pelo aviso de "nova academia cadastrada" pendente de ativação.
-# Se EMAIL_HOST, EMAIL_USER ou EMAIL_PASS estiver vazio, o aviso somente é
-# registrado no servidor e o cadastro continua funcionando normalmente.
+# Obs.: o aviso de "nova academia cadastrada" hoje é enviado via Brevo (API
+# HTTP, ver bloco abaixo), não por aqui — hospedagens como o plano gratuito
+# da Render bloqueiam as portas SMTP de saída (25/465/587). Este bloco
+# continua documentado e o código continua existindo (SendAcademiaCadastradaEmailSMTP)
+# caso seu ambiente não tenha esse bloqueio (ex.: VPS próprio, Render pago).
 EMAIL_HOST=smtp.gmail.com
 EMAIL_PORT=587
 EMAIL_USER=
@@ -97,6 +99,25 @@ EMAIL_PASS=
 # Opcional: remetente exibido, se diferente de EMAIL_USER.
 EMAIL_FROM=
 
+# =============================================================================
+# Brevo (envio de email via API HTTP — usado no lugar de SMTP quando a
+# hospedagem bloqueia as portas SMTP de saída, ex.: plano gratuito da Render)
+# =============================================================================
+# Aviso de "nova academia cadastrada" hoje é enviado por aqui
+# (ver internal/services/brevo_mailer.go + SendAcademiaCadastradaEmailBrevo).
+# Chamada HTTPS comum (porta 443) — nenhuma hospedagem de nuvem bloqueia isso,
+# ao contrário das portas SMTP (25/465/587), bloqueadas por padrão em vários
+# planos gratuitos (confirmado no changelog oficial da Render).
+# 1. Crie uma conta gratuita em https://www.brevo.com (plano free: 300
+#    emails/dia no momento em que isto foi escrito — conferir limite atual).
+# 2. Verifique UM remetente (não precisa de domínio próprio) em
+#    https://app.brevo.com/settings/senders — pode ser o MESMO email já
+#    configurado acima em EMAIL_USER/EMAIL_FROM.
+# 3. Gere uma API key em https://app.brevo.com/settings/keys/api e cole
+#    abaixo.
+# Vazio = aviso apenas logado, nada é enviado, cadastro continua funcionando.
+BREVO_API_KEY=
+
 # =============================================================================
 # Armazenamento de Arquivos (Mega)
 # =============================================================================
```

### Arquivos de teste NOVOS 6/6

Crie os dois arquivos abaixo com o conteúdo exato. Nenhum precisa de banco nem de rede externa de verdade.

**`internal/services/brevo_mailer_test.go`** (testes unitários puros: config):

```go
package services

import "testing"

func TestLoadBrevoConfigMissingAPIKeyDisabled(t *testing.T) {
	t.Setenv("BREVO_API_KEY", "")
	t.Setenv("EMAIL_FROM", "spuri@example.com")
	t.Setenv("EMAIL_USER", "spuri@example.com")

	_, ok := loadBrevoConfig()
	if ok {
		t.Fatal("esperava loadBrevoConfig desabilitado sem BREVO_API_KEY")
	}
}

func TestLoadBrevoConfigMissingSenderDisabled(t *testing.T) {
	t.Setenv("BREVO_API_KEY", "xkeysib-teste")
	t.Setenv("EMAIL_FROM", "")
	t.Setenv("EMAIL_USER", "")

	_, ok := loadBrevoConfig()
	if ok {
		t.Fatal("esperava loadBrevoConfig desabilitado sem EMAIL_FROM/EMAIL_USER")
	}
}

func TestLoadBrevoConfigFallsBackToEmailUser(t *testing.T) {
	t.Setenv("BREVO_API_KEY", "xkeysib-teste")
	t.Setenv("EMAIL_FROM", "")
	t.Setenv("EMAIL_USER", "spuriartipan@gmail.com")

	cfg, ok := loadBrevoConfig()
	if !ok {
		t.Fatal("esperava loadBrevoConfig habilitado com EMAIL_USER presente mesmo sem EMAIL_FROM")
	}
	if cfg.SenderEmail != "spuriartipan@gmail.com" {
		t.Errorf("esperava remetente cair para EMAIL_USER, obteve %q", cfg.SenderEmail)
	}
}

func TestLoadBrevoConfigPrefersEmailFrom(t *testing.T) {
	t.Setenv("BREVO_API_KEY", "xkeysib-teste")
	t.Setenv("EMAIL_FROM", "avisos@spuri.co")
	t.Setenv("EMAIL_USER", "spuriartipan@gmail.com")

	cfg, ok := loadBrevoConfig()
	if !ok {
		t.Fatal("esperava loadBrevoConfig habilitado")
	}
	if cfg.SenderEmail != "avisos@spuri.co" {
		t.Errorf("esperava EMAIL_FROM ter prioridade sobre EMAIL_USER, obteve %q", cfg.SenderEmail)
	}
}

func TestSendAcademiaCadastradaEmailBrevoDisabledDoesNotError(t *testing.T) {
	t.Setenv("BREVO_API_KEY", "")
	t.Setenv("EMAIL_FROM", "")
	t.Setenv("EMAIL_USER", "")

	svc := NewEmailService(nil)
	err := svc.SendAcademiaCadastradaEmailBrevo("admin@example.com", "Admin Teste", AcademiaCadastradaInfo{
		Nome:           "Academia Teste",
		CodigoAcademia: "LDA2026A001",
		NIF:            "5417845812",
		Nivel:          "escola",
		Type:           "private",
		Provincia:      "LDA",
	})
	if err != nil {
		t.Fatalf("sem BREVO_API_KEY/remetente configurados deveria retornar nil (não bloqueia o cadastro), obteve: %v", err)
	}
}

func TestSendAcademiaCadastradaEmailBrevoEmptyRecipientErrors(t *testing.T) {
	t.Setenv("BREVO_API_KEY", "xkeysib-teste")
	t.Setenv("EMAIL_FROM", "avisos@spuri.co")
	t.Setenv("EMAIL_USER", "")

	svc := NewEmailService(nil)
	err := svc.SendAcademiaCadastradaEmailBrevo("", "Admin Teste", AcademiaCadastradaInfo{Nome: "Academia Teste"})
	if err == nil {
		t.Fatal("esperava erro para destinatário vazio")
	}
}
```

**`internal/services/brevo_mailer_e2e_test.go`** (servidor HTTP mock real via `httptest`, provando a requisição/resposta completa — estes são os testes mais importantes desta tarefa):

```go
package services

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestSendBrevoEmailToURLAgainstRealServer prova, contra um servidor HTTP
// real (local), que sendBrevoEmailToURL monta a requisição exatamente como
// a API do Brevo espera: método POST, cabeçalhos api-key/Content-Type, e
// corpo JSON com sender/to/subject/htmlContent/textContent corretos — e
// que trata a resposta de sucesso (200 + messageId) sem erro.
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

	err := sendBrevoEmailToURL(cfg, server.URL, "admin@example.com", "Admin Teste",
		"Nova instituição cadastrada: Academia Teste",
		"corpo em texto puro", "<p>corpo em <strong>html</strong></p>")
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

	if gotBody.Sender.Email != "spuriartipan@gmail.com" {
		t.Errorf("sender.email incorreto: %q", gotBody.Sender.Email)
	}
	if len(gotBody.To) != 1 || gotBody.To[0].Email != "admin@example.com" {
		t.Errorf("to incorreto: %+v", gotBody.To)
	}
	if gotBody.Subject != "Nova instituição cadastrada: Academia Teste" {
		t.Errorf("subject incorreto: %q", gotBody.Subject)
	}
	if !strings.Contains(gotBody.HTMLContent, "<strong>html</strong>") {
		t.Errorf("htmlContent não contém o HTML esperado: %q", gotBody.HTMLContent)
	}
	if gotBody.TextContent != "corpo em texto puro" {
		t.Errorf("textContent incorreto: %q", gotBody.TextContent)
	}
}

// TestSendBrevoEmailToURLPropagatesAPIError prova que uma resposta de erro
// do Brevo (ex.: remetente não verificado, API key inválida) vira um erro
// Go com a mensagem da API, em vez de ser tratada como sucesso.
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
		t.Errorf("esperava a mensagem de erro da API do Brevo no erro Go, obteve: %v", err)
	}
}

// TestSendAcademiaCadastradaEmailBrevoAgainstRealServer é o mesmo teste do
// primeiro caso acima, mas passando pelo caminho público completo
// (EmailService.SendAcademiaCadastradaEmailBrevo) — não dá para apontar
// para o servidor mock por env var (a URL da Brevo é fixa por design, ver
// sendBrevoEmail), então este teste confirma só a parte que dá pra
// verificar sem mudar a produção: com BREVO_API_KEY/EMAIL_FROM ausentes,
// retorna nil sem tentar rede (já coberto em brevo_mailer_test.go); a
// integração ponta-a-ponta contra a Brevo real está coberta pelos dois
// testes acima, que exercitam sendBrevoEmailToURL — a mesma função que
// sendBrevoEmail (usada em produção) apenas encaminha para a URL fixa.
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
	err := sendBrevoEmailToURL(cfg, server.URL, "admin@example.com", "Admin Teste",
		"Nova instituição cadastrada: Academia Teste",
		"Ola Admin Teste! A instituicao Academia Teste esta pendente de analise. Acesse: https://painel.exemplo.com/academias",
		renderAcademiaCadastradaHTML("Admin Teste", AcademiaCadastradaInfo{
			Nome:           "Academia Teste",
			CodigoAcademia: "LDA2026A001",
			NIF:            "5417845812",
			Nivel:          "escola",
			Type:           "private",
			Provincia:      "LDA",
		}, "https://painel.exemplo.com/academias"))
	if err != nil {
		t.Fatalf("sendBrevoEmailToURL com HTML real (renderAcademiaCadastradaHTML) falhou: %v", err)
	}

	for _, want := range []string{"LDA2026A001", "5417845812", "https://painel.exemplo.com/academias"} {
		if !strings.Contains(gotBody.HTMLContent, want) {
			t.Errorf("HTML enviado não contém %q", want)
		}
	}
}
```

## 3. Configuração necessária fora do código (deploy)

1. Criar uma conta gratuita em [brevo.com](https://www.brevo.com).
2. Verificar UM remetente em `https://app.brevo.com/settings/senders` — não precisa de domínio próprio. Pode ser o **mesmo email já usado em `EMAIL_USER`** (o mesmo que já foi configurado para SMTP na tarefa 95).
3. Gerar uma API key em `https://app.brevo.com/settings/keys/api`.
4. Definir `BREVO_API_KEY` no ambiente de produção (Render). `EMAIL_FROM`/`EMAIL_USER` já configurados continuam sendo usados como remetente — nenhuma variável nova além de `BREVO_API_KEY` é necessária se `EMAIL_USER` já estiver definido.
5. **Sem esse remetente verificado no Brevo**, a API retorna erro (capturado e logado, não quebra o cadastro) — o email simplesmente não sai até o remetente ser verificado.

## 4. Checklist de aceitação

- [ ] `internal/services/brevo_mailer.go` existe com `BrevoConfig`, `loadBrevoConfig`, `sendBrevoEmail`, `sendBrevoEmailToURL`.
- [ ] `EmailService.SendAcademiaCadastradaEmailBrevo` existe em `email_service.go`, reaproveita `renderAcademiaCadastradaHTML` já existente, nunca bloqueia (retorna `nil` quando Brevo não está configurado).
- [ ] `notificarAdminsSobreNovaAcademiaPendente` chama `SendAcademiaCadastradaEmailBrevo`, não mais `SendAcademiaCadastradaEmailSMTP`.
- [ ] `SendAcademiaCadastradaEmailSMTP` (tarefa 95) e `SendAcademiaCadastradaEmail` (EmailJS, tarefa 92) **continuam existindo** — não foram apagadas.
- [ ] `GetAdminsParaNotificarNovaAcademia` não foi tocada — lista de destinatários continua 100% automática.
- [ ] `BREVO_API_KEY` documentada em `.env.example`, com instruções de como obter.
- [ ] Nenhuma dependência nova em `go.mod`/`go.sum`.
- [ ] Nenhuma migration nova, nenhuma mudança no `spuripainel`.
- [ ] `go build ./...` e `go vet ./...` limpos.
- [ ] `gofmt -l` não lista nenhum dos arquivos tocados/criados.
- [ ] `go test ./...` limpo, incluindo os 9 testes novos (6 unitários + 3 contra servidor HTTP mock via `httptest`, sem precisar de banco nem de rede real).

## 5. Validação já feita (evidência real)

- **Diagnóstico da causa raiz**: confirmado via changelog oficial da Render ("Free web services will no longer allow outbound traffic to SMTP ports", vigente desde 26/09/2025) que bloqueia as portas 25, 465 e 587 em serviços web gratuitos — casa exatamente com o erro relatado (`connect: connection timed out` na porta 587), que é a assinatura de bloqueio de firewall (não erro de credencial).
- **Build/vet/gofmt**: limpos, na cópia de trabalho e, de novo, reaplicando os diffs desta seção via `git apply` num clone limpo do `main` atual (com os 3 arquivos novos copiados por cima) — validação independente.
- **Suíte completa, sem banco e com banco real** (banco recriado do zero, `SPURI_RUN_DB_INTEGRITY_TESTS=1`): limpa nos dois ambientes — esta tarefa não depende de banco, mas rodei mesmo assim para garantir que nada mais quebrou.
- **Revert-and-confirm no teste de wiring já existente**: reverti a chamada de volta para `SendAcademiaCadastradaEmailSMTP` sem atualizar a asserção do teste → falhou exatamente como esperado. Restaurei e reconfirmei verde, com `go build ./...` limpo depois.
- **Teste de ponta a ponta contra um servidor HTTP real** (não é só "a lógica parece certa" — é a requisição HTTP de verdade): usando `httptest.NewServer`, confirmei que `sendBrevoEmailToURL` monta a requisição exatamente como a API do Brevo documenta — método POST, header `api-key` com a chave configurada, `Content-Type: application/json`, corpo JSON com `sender.email`, `to[0].email`, `subject`, `htmlContent` e `textContent` corretos — e que uma resposta de sucesso (200/201 + `messageId`) é tratada sem erro.
- **Teste de propagação de erro da API**: uma resposta 401 com `{"code":"unauthorized","message":"Key not found"}` (formato real de erro do Brevo) vira um erro Go contendo a mensagem da API, não é tratada como sucesso silenciosamente.
- **Teste com o HTML real de produção**: rodei `sendBrevoEmailToURL` com o `htmlBody` vindo de `renderAcademiaCadastradaHTML` (a mesma função já usada pela versão SMTP) e confirmei que o código da academia, o NIF e o link do painel chegam corretos no corpo da requisição capturada pelo servidor mock.

**O que não pôde ser testado neste ambiente**: uma chamada real a `api.brevo.com` (meu sandbox não tem acesso de rede a esse domínio) e a verificação de remetente/entrega de fato numa caixa de entrada. O teste contra o servidor mock cobre 100% do código que este projeto escreve (montagem da requisição, headers, parsing da resposta e de erros); a única coisa que fica de fora é a infraestrutura do Brevo em si. Recomendo, depois de configurar `BREVO_API_KEY` e verificar o remetente, fazer um autocadastro de teste e conferir a caixa de entrada de um admin de teste — e checar os logs (`[EMAIL-BREVO]` para o caso desabilitado, ou o erro da API via `[WARN] notificarAdminsSobreNovaAcademiaPendente` se a API rejeitar por remetente não verificado).

## 6. Nota sobre numeração desta tarefa

Esta tarefa foi originalmente escrita como "96", mas por volta do mesmo período outra tarefa completamente não relacionada ("Remover vazamento de mensagens técnicas em respostas de sucesso") também recebeu o número 96 e já foi mesclada em `main` — inclusive tocando `internal/handlers/academia_handlers.go` (mudou o texto do aviso de alvará não enviado em `RegisterAcademiaPublica`, em outra parte da mesma função). Renumerei esta para **97** antes de entregar, e reconferi de novo — build, vet, gofmt e suíte completa (com e sem banco) — aplicando os diffs desta seção 2 contra o `main` real já com aquela tarefa 96 mesclada. O diff de `academia_handlers.go` aplica com um pequeno deslocamento de linha (esperado, por causa da mudança anterior no arquivo), sem nenhum conflito de conteúdo — confirmei que a mensagem de aviso alterada pela outra tarefa continua intacta depois de aplicar esta.

## 7. Ao terminar

Mova este arquivo para `docs/Tarefas feitas/97 - Enviar o aviso de nova academia via Brevo (API HTTP) em vez de SMTP puro.md` depois que o checklist da seção 4 estiver 100% marcado.
