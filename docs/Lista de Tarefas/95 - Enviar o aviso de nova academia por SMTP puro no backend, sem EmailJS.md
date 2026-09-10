---
tarefa: 95
titulo: Enviar o aviso de "nova academia cadastrada" por SMTP puro no backend (sem EmailJS), lista de admins continua 100% automática
repo: fredypdp/spuri-backend
status: pronta para execução pelo Codex
orquestrado_e_pre_testado_por: Claude
depende_de: tarefa 92 (já concluída/mesclada em main — GetAdminsParaNotificarNovaAcademia, SendAcademiaCadastradaEmail, notificarAdminsSobreNovaAcademiaPendente). Não depende da tarefa 93 deste repositório (módulo de Comunicação SMS/GoSMSZiett) nem da tarefa 93 do spuripainel (aviso via NodeMailer) — nenhuma relação entre elas.
validado_contra: PostgreSQL 16 real + Go 1.24 (via apt), suíte completa existente + 8 testes novos (2 deles um handshake SMTP real de ponta a ponta contra um servidor mock local, não só lógica isolada), tudo reconferido de novo aplicando os diffs via `git apply` num clone limpo do main atual
---

# Como usar este documento

Este documento assume que a tarefa 92 já está em produção (confirmei isso puxando o `main` atual antes de escrever este documento — `notificarAdminsSobreNovaAcademiaPendente`, `GetAdminsParaNotificarNovaAcademia` e `SendAcademiaCadastradaEmail` já existem exatamente como a tarefa 92 descreveu). Esta tarefa **não desfaz nada da 92** — só troca **como** o email é enviado (SMTP em vez de EmailJS) trocando uma linha de chamada e adicionando código novo ao lado do que já existe.

Aplique os diffs na ordem desta seção. Não invente fallback para EmailJS nem tente unificar isso com o módulo de Comunicação SMS (`internal/services/comunicacao_gosms_client.go`/`comunicacao_ziett_client.go`, tarefa 93 deste mesmo repositório) — são coisas completamente não relacionadas (SMS × email), meras coincidências de numeração de tarefa. Ao final, rode `go build ./...`, `go vet ./...`, `go test ./...` (sem `SPURI_RUN_DB_INTEGRITY_TESTS` — não precisa de banco para nenhum teste desta tarefa, incluindo os testes end-to-end contra o servidor SMTP mock, que roda em processo, sem rede real) e confira o checklist da seção 4.

## 1. Por que esta tarefa existe

A tarefa 92 implementou o aviso via EmailJS, mas a conta EmailJS do projeto está no limite de templates do plano gratuito (só dá pra ter 2, já ocupados por verificação de email e recuperação de senha) — não dava pra criar o terceiro template dedicado. Isso levou a uma solução via frontend (tarefa 93 do `spuripainel`, NodeMailer) como contorno. Só que dá pra resolver isso **inteiramente dentro do backend**, sem depender de nenhum serviço de terceiro (nem EmailJS nem nada): Go tem SMTP na biblioteca padrão (`net/smtp`), então o email pode ser enviado direto por um provedor SMTP qualquer — inclusive Gmail com senha de app, gratuito, o mesmo mecanismo que o `spuripainel` já usa via NodeMailer.

A vantagem de fazer isso no backend em vez do frontend: **a lista de quem recebe já é 100% automática desde a tarefa 92** — `GetAdminsParaNotificarNovaAcademia()` já consulta o banco (`role IN ('adm','fpp')`, `status='ativo'`, `email_verificado=true`) toda vez, sem lista manual nenhuma. A versão do `spuripainel` (tarefa 93 de lá) precisou de uma lista estática por env var porque o frontend não tem acesso direto ao banco — o backend não tem essa limitação. Esta tarefa aproveita exatamente essa vantagem.

**Nenhuma dependência nova.** `net/smtp`, `crypto/tls` (usado internamente por `net/smtp`), `mime` e `html` já são bibliotecas padrão do Go — nada entra em `go.mod`. (Notei que `gopkg.in/gomail.v2` já está declarado em `go.mod` mas não é usado em nenhum lugar do código hoje — não usei essa biblioteca de propósito, `net/smtp` sozinho já resolve sem precisar baixar nada de módulo nenhum, o que também deixa o build mais simples de reproduzir offline. Não removi a declaração não utilizada do `go.mod` porque não é o escopo desta tarefa.)

## 2. O que muda, e o que não muda

- **Muda**: qual função `notificarAdminsSobreNovaAcademiaPendente` chama para enviar cada email — antes `EmailService.SendAcademiaCadastradaEmail` (EmailJS), agora `EmailService.SendAcademiaCadastradaEmailSMTP` (SMTP puro, novo).
- **Não muda**: `AdminProjection.GetAdminsParaNotificarNovaAcademia` (a busca dos admins continua idêntica — automática, sem lista manual), `notificarAdminsSobreNovaAcademiaPendente` (a lógica de loop/log continua igual, só a chamada interna muda), `RegisterAcademiaPublica`, a resposta HTTP do cadastro, e nenhuma migration.
- `EmailService.SendAcademiaCadastradaEmail` (EmailJS) **continua existindo no arquivo**, só deixa de ser chamada por este handler — não foi apagada, para o caso de o caminho via EmailJS ser retomado no futuro (ex.: se o plano for resolvido).
- Nenhuma mudança no `spuripainel`. As duas tarefas 93 (SMS neste repo, NodeMailer no `spuripainel`) permanecem como estão — esta tarefa não interage com nenhuma delas.

## 3. Diffs a aplicar, nesta ordem

Todos os diffs foram extraídos com `git diff` a partir de uma cópia limpa do `main` atual (após o merge da tarefa 92 e do módulo de Comunicação SMS) e reconferidos aplicando-os com `git apply` contra um clone limpo antes de fechar este documento.

### Arquivo NOVO 1/5 — `internal/services/smtp_mailer.go`

Transporte SMTP genérico (config + construção de mensagem MIME), reaproveitável para qualquer email futuro que precise sair do backend sem passar pelo EmailJS. Crie com o conteúdo exato abaixo:

```go
// internal/services/smtp_mailer.go
//
// Envio de email via SMTP puro (net/smtp, biblioteca padrão do Go) —
// alternativa ao EmailJS que não depende de nenhum template hospedado por
// terceiro nem de limite de plano gratuito ("quantidade de templates"): o
// HTML de cada email é montado em código Go. Nenhuma dependência nova é
// necessária (sem gomail, sem SDK externo).
//
// Pensado para ser genérico o suficiente para qualquer email futuro que
// precise sair pelo backend sem passar pelo EmailJS — hoje só
// SendAcademiaCadastradaEmailSMTP (em email_service.go) usa isto.
package services

import (
	"bytes"
	"fmt"
	"mime"
	"net/smtp"
	"os"
)

// SMTPConfig agrupa a configuração de um provedor SMTP qualquer (Gmail com
// senha de app, ou qualquer outro). Mesmas variáveis de ambiente e mesmo
// conceito já usados pelo spuripainel (NodeMailer) para verificação de
// email e recuperação de senha — reaproveitar a mesma conta Gmail/senha de
// app nos dois lados é seguro, o Gmail não exige exclusividade por App
// Password.
type SMTPConfig struct {
	Host string
	Port string
	User string
	Pass string
	From string // remetente exibido; se EMAIL_FROM vazio, usa EMAIL_USER
}

// loadSMTPConfig lê a configuração SMTP do ambiente. O segundo retorno é
// false se EMAIL_HOST, EMAIL_USER ou EMAIL_PASS estiver ausente — mesma
// convenção de "desabilitado, só loga, nunca bloqueia" usada pelo resto
// deste pacote (ver EmailService.enabled).
func loadSMTPConfig() (SMTPConfig, bool) {
	cfg := SMTPConfig{
		Host: os.Getenv("EMAIL_HOST"),
		Port: getEnvOrDefault("EMAIL_PORT", "587"),
		User: os.Getenv("EMAIL_USER"),
		Pass: os.Getenv("EMAIL_PASS"),
	}
	cfg.From = getEnvOrDefault("EMAIL_FROM", cfg.User)
	if cfg.Host == "" || cfg.User == "" || cfg.Pass == "" {
		return cfg, false
	}
	return cfg, true
}

// sendSMTPEmail envia um email HTML+texto (multipart/alternative) via SMTP.
// net/smtp.SendMail já faz STARTTLS automaticamente quando o servidor
// oferece a extensão (caso do Gmail em smtp.gmail.com:587) antes de
// autenticar — não é necessário nenhum código de TLS manual aqui.
func sendSMTPEmail(cfg SMTPConfig, toEmail, toName, subject, textBody, htmlBody string) error {
	auth := smtp.PlainAuth("", cfg.User, cfg.Pass, cfg.Host)
	msg := buildMimeMessage(cfg.From, toEmail, toName, subject, textBody, htmlBody)
	addr := fmt.Sprintf("%s:%s", cfg.Host, cfg.Port)
	return smtp.SendMail(addr, auth, cfg.From, []string{toEmail}, msg)
}

// encodeHeaderWord codifica um valor de cabeçalho (nome de exibição,
// assunto) em RFC 2047 (UTF-8), necessário porque nomes/assuntos em
// português têm acentos e cabeçalhos de email só aceitam ASCII puro.
func encodeHeaderWord(s string) string {
	return mime.QEncoding.Encode("UTF-8", s)
}

// buildMimeMessage monta um email RFC 5322 com corpo multipart/alternative
// (texto puro + HTML). Boundary fixo: cada mensagem é montada e enviada
// isoladamente numa única conexão SMTP, não há risco de colisão entre
// mensagens concorrentes dentro do mesmo corpo.
func buildMimeMessage(from, toEmail, toName, subject, textBody, htmlBody string) []byte {
	const boundary = "spuri-smtp-boundary-7f3a"
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "From: %s <%s>\r\n", encodeHeaderWord("Spuri"), from)
	fmt.Fprintf(&buf, "To: %s <%s>\r\n", encodeHeaderWord(toName), toEmail)
	fmt.Fprintf(&buf, "Subject: %s\r\n", encodeHeaderWord(subject))
	buf.WriteString("MIME-Version: 1.0\r\n")
	fmt.Fprintf(&buf, "Content-Type: multipart/alternative; boundary=%s\r\n", boundary)
	buf.WriteString("\r\n")
	fmt.Fprintf(&buf, "--%s\r\n", boundary)
	buf.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
	buf.WriteString(textBody)
	buf.WriteString("\r\n\r\n")
	fmt.Fprintf(&buf, "--%s\r\n", boundary)
	buf.WriteString("Content-Type: text/html; charset=UTF-8\r\n\r\n")
	buf.WriteString(htmlBody)
	buf.WriteString("\r\n\r\n")
	fmt.Fprintf(&buf, "--%s--\r\n", boundary)
	return buf.Bytes()
}
```

### Arquivo 2/5 — `internal/services/email_service.go` (novo método + import)

```diff
--- a/internal/services/email_service.go
+++ b/internal/services/email_service.go
@@ -8,6 +8,7 @@ import (
 	"encoding/hex"
 	"encoding/json"
 	"fmt"
+	"html"
 	"io"
 	"log"
 	"math/big"
@@ -444,6 +445,102 @@ func (s *EmailService) SendAcademiaCadastradaEmail(adminEmail, adminNome string,
 	return s.sendEmailViaEmailJS(adminEmail, adminNome, templateID, params)
 }
 
+// SendAcademiaCadastradaEmailSMTP avisa um administrador (role 'adm' ou
+// 'fpp', email verificado — ver AdminProjection.GetAdminsParaNotificarNovaAcademia)
+// que uma nova academia se autocadastrou via POST /academia/cadastro e está
+// pendente de análise/ativação no painel.
+//
+// Envia via SMTP puro (net/smtp, biblioteca padrão do Go — ver
+// smtp_mailer.go — nenhuma dependência nova): o HTML é montado em código,
+// então não existe limite de "quantidade de templates" de plano gratuito
+// para esbarrar, e a lista de quem recebe continua 100% automática/dinâmica
+// (nenhuma lista manual — vem de GetAdminsParaNotificarNovaAcademia).
+// Configuração em EMAIL_HOST/EMAIL_PORT/EMAIL_USER/EMAIL_PASS.
+//
+// Esta função é quem internal/handlers/academia_handlers.go chama para este
+// aviso. SendAcademiaCadastradaEmail (EmailJS, acima) continua existindo no
+// arquivo, só não é mais chamada por esse handler — ver decisão registrada
+// na tarefa correspondente.
+//
+// Assim como as demais notificações deste serviço, falha aqui NUNCA deve
+// bloquear o fluxo que a originou — é responsabilidade do chamador apenas
+// logar o erro e seguir.
+func (s *EmailService) SendAcademiaCadastradaEmailSMTP(adminEmail, adminNome string, info AcademiaCadastradaInfo) error {
+	cfg, ok := loadSMTPConfig()
+	if !ok {
+		log.Printf("[EMAIL-SMTP] ⚠️  EMAIL_HOST/EMAIL_USER/EMAIL_PASS não configurados — aviso de nova academia %s (%s) não enviado para admin %s",
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
+
+	if err := sendSMTPEmail(cfg, adminEmail, adminNome, subject, textBody, htmlBody); err != nil {
+		return fmt.Errorf("smtp: %w", err)
+	}
+	return nil
+}
+
+// renderAcademiaCadastradaHTML monta o HTML do aviso em código Go (sem
+// depender de nenhum template hospedado externamente). Todo campo de
+// `info`/`adminNome` é escapado com html.EscapeString antes de entrar no
+// corpo — defesa em profundidade, mesmo esses dados já vindo confiáveis
+// (persistidos pelo próprio backend em RegisterAcademiaPublica).
+func renderAcademiaCadastradaHTML(adminNome string, info AcademiaCadastradaInfo, painelURL string) string {
+	esc := html.EscapeString
+	const tpl = `<!DOCTYPE html>
+<html lang="pt-AO">
+<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0"></head>
+<body style="margin:0;padding:0;background-color:#F9FAFB;font-family:'Segoe UI',Arial,sans-serif;">
+  <table role="presentation" width="100%%" cellpadding="0" cellspacing="0"><tr><td align="center" style="padding:40px 16px;">
+    <table role="presentation" width="600" cellpadding="0" cellspacing="0" style="width:100%%;max-width:600px;background:#FFFFFF;border-radius:24px;border:1px solid #E4E7EC;">
+      <tr><td style="border-radius:24px 24px 0 0;height:5px;background:#465FFF;"></td></tr>
+      <tr><td style="padding:30px 40px 22px;text-align:center;border-bottom:1px solid #F2F4F7;">
+        <span style="font-size:18px;font-weight:700;color:#172741;">Spuri</span>
+      </td></tr>
+      <tr><td style="padding:40px;">
+        <p style="margin:0 0 6px;font-size:12px;font-weight:700;letter-spacing:0.08em;text-transform:uppercase;color:#465FFF;">Novo cadastro pendente</p>
+        <h1 style="margin:0 0 16px;font-size:24px;font-weight:700;color:#172741;">Uma instituição acabou de se cadastrar</h1>
+        <p style="margin:0;font-size:15px;line-height:1.65;color:#344054;">
+          Olá, <strong>%s</strong>! A instituição <strong>%s</strong> concluiu o autocadastro no Spuri e está com a conta <strong>inativa</strong>, aguardando a sua análise.
+        </p>
+        <table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="margin-top:24px;border-radius:16px;background:#F9FAFB;border:1px solid #F2F4F7;">
+          <tr><td style="padding:20px 22px;">
+            <p style="margin:0 0 6px;font-size:13px;color:#344054;"><strong>Código:</strong> %s</p>
+            <p style="margin:0 0 6px;font-size:13px;color:#344054;"><strong>NIF:</strong> %s</p>
+            <p style="margin:0 0 6px;font-size:13px;color:#344054;"><strong>Tipo:</strong> %s</p>
+            <p style="margin:0 0 6px;font-size:13px;color:#344054;"><strong>Nível:</strong> %s</p>
+            <p style="margin:0;font-size:13px;color:#344054;"><strong>Província:</strong> %s</p>
+          </td></tr>
+        </table>
+        <div style="text-align:center;margin-top:28px;">
+          <a href="%s" style="display:inline-block;padding:14px 34px;background:#465FFF;color:#FFFFFF;text-decoration:none;border-radius:12px;font-size:15px;font-weight:600;">Analisar no Painel</a>
+        </div>
+      </td></tr>
+      <tr><td style="background:#F9FAFB;padding:26px 40px;text-align:center;border-top:1px solid #F2F4F7;border-radius:0 0 24px 24px;">
+        <p style="margin:0;font-size:13px;font-weight:600;color:#172741;">Spuri</p>
+        <p style="margin:4px 0 0;font-size:12px;color:#667085;">Confiança e eficiência na gestão académica.</p>
+      </td></tr>
+    </table>
+  </td></tr></table>
+</body>
+</html>`
+	return fmt.Sprintf(tpl,
+		esc(adminNome), esc(info.Nome),
+		esc(info.CodigoAcademia), esc(info.NIF), esc(info.Type), esc(info.Nivel), esc(info.Provincia),
+		painelURL,
+	)
+}
+
 // GetDefaultPassword retorna a senha padrão para estudantes e academias.
 // O código do estudante/academia é conhecido pelo operador que criou o registro,
 // tornando este mecanismo aceitável para esses perfis.
```

### Arquivo 3/5 — `internal/handlers/academia_handlers.go` (troca a chamada)

```diff
--- a/internal/handlers/academia_handlers.go
+++ b/internal/handlers/academia_handlers.go
@@ -483,7 +483,7 @@ func notificarAdminsSobreNovaAcademiaPendente(c *gin.Context, req RegisterAcadem
 		Provincia:      codigoProvincia,
 	}
 	for _, admin := range admins {
-		if emailErr := emailSvc.SendAcademiaCadastradaEmail(admin.Email, admin.Nome, info); emailErr != nil {
+		if emailErr := emailSvc.SendAcademiaCadastradaEmailSMTP(admin.Email, admin.Nome, info); emailErr != nil {
 			log.Printf("[WARN] notificarAdminsSobreNovaAcademiaPendente: falha ao notificar admin %s sobre academia %s: %v", admin.Email, codigoAcademia, emailErr)
 		}
 	}
```

### Arquivo 4/5 — `internal/handlers/academia_notificacao_admins_test.go` (atualiza a asserção do teste já existente)

```diff
--- a/internal/handlers/academia_notificacao_admins_test.go
+++ b/internal/handlers/academia_notificacao_admins_test.go
@@ -19,7 +19,7 @@ func TestRegisterAcademiaPublicaNotificaAdminsRegistroAcademia(t *testing.T) {
 
 	mustContain(t, source, "func notificarAdminsSobreNovaAcademiaPendente(")
 	mustContain(t, source, "GetAdminsParaNotificarNovaAcademia()")
-	mustContain(t, source, "SendAcademiaCadastradaEmail(")
+	mustContain(t, source, "SendAcademiaCadastradaEmailSMTP(")
 
 	publicaBody := extractFuncBody(t, source, "func RegisterAcademiaPublica(")
 	if !strings.Contains(publicaBody, "notificarAdminsSobreNovaAcademiaPendente(") {
```

Este teste já existia (da tarefa 92) e checava a string `"SendAcademiaCadastradaEmail("` em qualquer lugar do arquivo — sem este ajuste, ele quebra sozinho depois do diff 3/5, porque essa string exata deixa de aparecer (`SendAcademiaCadastradaEmailSMTP(` não conta como o mesmo texto). Confirmei isso de propósito antes de escrever este documento: apliquei só o diff 3/5 sem este, o teste falhou exatamente com essa mensagem, e voltou a passar com este ajuste.

### Arquivo 5/5 — `.env.example` (documentação das novas variáveis)

```diff
--- a/.env.example
+++ b/.env.example
@@ -77,12 +77,35 @@ EMAILJS_TEMPLATE_ADMIN_WELCOME=template_xxxxxxxx
 # POST /academia/cadastro. Sem fallback: se vazio, o aviso apenas é logado
 # no servidor e nenhum email é enviado (ver SendAcademiaCadastradaEmail).
 EMAILJS_TEMPLATE_ACADEMIA_CADASTRADA=template_xxxxxxxx
+# Obs.: o aviso de "nova academia cadastrada" (POST /academia/cadastro) hoje
+# é enviado por SMTP puro (ver bloco EMAIL_HOST/EMAIL_PORT/EMAIL_USER/EMAIL_PASS
+# abaixo), não por EmailJS — EMAILJS_TEMPLATE_ACADEMIA_CADASTRADA acima fica
+# sem uso enquanto isso, mas continua documentada caso o caminho via EmailJS
+# volte a ser usado no futuro.
 EMAILJS_PUBLIC_KEY=xxxxxxxxxxxxxxxxxxxxxx
 EMAILJS_PRIVATE_KEY=xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
 
 # URL Frontend (para links nos emails).
 FRONTEND_URL=https://seu-frontend.vercel.app
 
+# =============================================================================
+# SMTP (envio de email direto pelo backend, sem EmailJS)
+# =============================================================================
+# Usado hoje só pelo aviso de "nova academia cadastrada" pendente de ativação
+# (ver internal/services/smtp_mailer.go + EmailService.SendAcademiaCadastradaEmailSMTP).
+# Sem limite de "templates" de plano gratuito: o HTML é montado em código.
+# Vazio (qualquer uma das 3 primeiras) = aviso apenas logado, nada é enviado,
+# o cadastro continua funcionando normalmente.
+# Pode reaproveitar a MESMA conta/senha de app já usada pelo spuripainel
+# (NodeMailer) para verificação de email e recuperação de senha — o Gmail
+# permite usar a mesma senha de app em mais de um cliente SMTP ao mesmo tempo.
+EMAIL_HOST=smtp.gmail.com
+EMAIL_PORT=587
+EMAIL_USER=
+EMAIL_PASS=
+# Opcional: remetente exibido, se diferente de EMAIL_USER.
+EMAIL_FROM=
+
 # =============================================================================
 # Armazenamento de Arquivos (Mega)
 # =============================================================================
```

### Arquivos de teste NOVOS 

Crie os dois arquivos abaixo com o conteúdo exato. Nenhum dos dois precisa de banco de dados nem de rede externa de verdade — o segundo sobe um servidor SMTP mínimo em `127.0.0.1` só para o teste, dentro do próprio processo `go test`.

**`internal/services/smtp_mailer_test.go`** (testes unitários puros: config, construção de MIME, escaping):

```go
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

	_, ok := loadSMTPConfig()
	if ok {
		t.Fatal("esperava loadSMTPConfig desabilitado sem EMAIL_HOST/EMAIL_USER/EMAIL_PASS")
	}
}

func TestLoadSMTPConfigDefaultsPortAndFrom(t *testing.T) {
	t.Setenv("EMAIL_HOST", "smtp.gmail.com")
	t.Setenv("EMAIL_PORT", "")
	t.Setenv("EMAIL_USER", "spuri@example.com")
	t.Setenv("EMAIL_PASS", "senha-de-app")
	t.Setenv("EMAIL_FROM", "")

	cfg, ok := loadSMTPConfig()
	if !ok {
		t.Fatal("esperava loadSMTPConfig habilitado com EMAIL_HOST/EMAIL_USER/EMAIL_PASS presentes")
	}
	if cfg.Port != "587" {
		t.Errorf("esperava porta padrão 587, obteve %q", cfg.Port)
	}
	if cfg.From != cfg.User {
		t.Errorf("esperava EMAIL_FROM vazio cair para EMAIL_USER (%q), obteve %q", cfg.User, cfg.From)
	}
}

func TestSendAcademiaCadastradaEmailSMTPDisabledDoesNotError(t *testing.T) {
	t.Setenv("EMAIL_HOST", "")
	t.Setenv("EMAIL_USER", "")
	t.Setenv("EMAIL_PASS", "")

	svc := NewEmailService(nil)
	err := svc.SendAcademiaCadastradaEmailSMTP("admin@example.com", "Admin Teste", AcademiaCadastradaInfo{
		Nome:           "Academia Teste",
		CodigoAcademia: "LDA2026A001",
		NIF:            "5417845812",
		Nivel:          "escola",
		Type:           "private",
		Provincia:      "LDA",
	})
	if err != nil {
		t.Fatalf("sem EMAIL_HOST/EMAIL_USER/EMAIL_PASS configurados deveria retornar nil (não bloqueia o cadastro), obteve: %v", err)
	}
}

func TestSendAcademiaCadastradaEmailSMTPEmptyRecipientErrors(t *testing.T) {
	t.Setenv("EMAIL_HOST", "smtp.gmail.com")
	t.Setenv("EMAIL_USER", "spuri@example.com")
	t.Setenv("EMAIL_PASS", "senha-de-app")

	svc := NewEmailService(nil)
	err := svc.SendAcademiaCadastradaEmailSMTP("", "Admin Teste", AcademiaCadastradaInfo{Nome: "Academia Teste"})
	if err == nil {
		t.Fatal("esperava erro para destinatário vazio")
	}
}

func TestBuildMimeMessageContainsExpectedParts(t *testing.T) {
	msg := buildMimeMessage("remetente@spuri.co", "admin@example.com", "Jose Admin", "Assunto com acentuacao",
		"corpo em texto puro", "<p>corpo em <strong>html</strong></p>")
	s := string(msg)

	checks := []string{
		"MIME-Version: 1.0",
		"Content-Type: multipart/alternative; boundary=spuri-smtp-boundary-7f3a",
		"Content-Type: text/plain; charset=UTF-8",
		"corpo em texto puro",
		"Content-Type: text/html; charset=UTF-8",
		"<strong>html</strong>",
		"admin@example.com",
		"remetente@spuri.co",
	}
	for _, want := range checks {
		if !strings.Contains(s, want) {
			t.Errorf("mensagem MIME nao contem %q", want)
		}
	}

	if !strings.Contains(s, "\r\n") {
		t.Error("mensagem MIME deveria usar terminadores de linha CRLF")
	}
}

func TestBuildMimeMessageEncodesAccentedHeaders(t *testing.T) {
	msg := buildMimeMessage("remetente@spuri.co", "admin@example.com", "José Admin", "Nova instituição cadastrada",
		"corpo", "<p>corpo</p>")
	s := string(msg)

	if strings.Contains(s, "José") || strings.Contains(s, "instituição") {
		t.Error("cabecalhos com acento deveriam sair codificados (RFC 2047), nao com UTF-8 cru no cabecalho")
	}
	if !strings.Contains(s, "=?UTF-8?") {
		t.Error("esperava pelo menos um cabecalho codificado em RFC 2047 (=?UTF-8?...?=)")
	}
}

func TestRenderAcademiaCadastradaHTMLEscapesFields(t *testing.T) {
	htmlOut := renderAcademiaCadastradaHTML("Admin <script>", AcademiaCadastradaInfo{
		Nome:           "Academia & Cia <b>",
		CodigoAcademia: "LDA2026A001",
		NIF:            "123",
		Type:           "private",
		Nivel:          "escola",
		Provincia:      "LDA",
	}, "https://painel.exemplo.com/academias")

	if strings.Contains(htmlOut, "<script>") {
		t.Error("nome do admin com tag HTML deveria vir escapado, nao literal no HTML")
	}
	if strings.Contains(htmlOut, "Academia & Cia <b>") {
		t.Error("nome da academia com caracteres especiais deveria vir escapado, nao literal no HTML")
	}
	if !strings.Contains(htmlOut, "https://painel.exemplo.com/academias") {
		t.Error("HTML deveria conter o link para o painel")
	}
}
```

**`internal/services/smtp_mailer_e2e_test.go`** (servidor SMTP mock real, em processo, provando o handshake completo — este é o teste mais importante desta tarefa):

```go
package services

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// mockSMTPServer é um servidor SMTP mínimo (só o suficiente para
// smtp.SendMail completar o fluxo real: EHLO, AUTH PLAIN, MAIL FROM,
// RCPT TO, DATA, QUIT) rodando em localhost, para provar que
// sendSMTPEmail/buildMimeMessage produzem uma troca de protocolo válida de
// verdade — não só que a lógica isolada "parece certa".
type mockSMTPServer struct {
	listener net.Listener
	mu       sync.Mutex
	rawData  string // conteúdo bruto recebido no comando DATA da última mensagem
	authSeen bool
}

func startMockSMTPServer(t *testing.T) *mockSMTPServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start mock smtp server: %v", err)
	}
	srv := &mockSMTPServer{listener: ln}
	go srv.serve(t)
	t.Cleanup(func() { _ = ln.Close() })
	return srv
}

func (s *mockSMTPServer) addr() string {
	return s.listener.Addr().String()
}

func (s *mockSMTPServer) serve(t *testing.T) {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return // listener fechado (t.Cleanup) — encerra a goroutine
		}
		go s.handleConn(t, conn)
	}
}

func (s *mockSMTPServer) handleConn(t *testing.T, conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	write := func(format string, args ...interface{}) {
		fmt.Fprintf(conn, format+"\r\n", args...)
	}

	write("220 mock.local ESMTP ready")

	inData := false
	var dataBuf strings.Builder

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")

		if inData {
			if line == "." {
				inData = false
				s.mu.Lock()
				s.rawData = dataBuf.String()
				s.mu.Unlock()
				write("250 OK: queued")
				continue
			}
			dataBuf.WriteString(line)
			dataBuf.WriteString("\r\n")
			continue
		}

		upper := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
			write("250-mock.local greets you")
			write("250 AUTH PLAIN")
		case strings.HasPrefix(upper, "AUTH PLAIN"):
			s.mu.Lock()
			s.authSeen = true
			s.mu.Unlock()
			write("235 Authentication successful")
		case strings.HasPrefix(upper, "MAIL FROM"):
			write("250 OK")
		case strings.HasPrefix(upper, "RCPT TO"):
			write("250 OK")
		case upper == "DATA":
			inData = true
			write("354 Send message content; end with <CRLF>.<CRLF>")
		case upper == "QUIT":
			write("221 Bye")
			return
		default:
			write("500 unrecognized command")
		}
	}
}

func (s *mockSMTPServer) getRawData() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rawData
}

func (s *mockSMTPServer) getAuthSeen() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.authSeen
}

// TestSendSMTPEmailAgainstRealServer prova, contra um servidor SMTP real
// (local), que sendSMTPEmail/buildMimeMessage produzem uma sessão válida de
// ponta a ponta: net/smtp.SendMail autentica (PlainAuth aceita sem TLS
// porque o host é 127.0.0.1 — ver isLocalhost em net/smtp), envia MAIL
// FROM/RCPT TO/DATA, e o corpo recebido pelo servidor contém exatamente o
// texto e o HTML esperados.
func TestSendSMTPEmailAgainstRealServer(t *testing.T) {
	srv := startMockSMTPServer(t)
	host, port, err := net.SplitHostPort(srv.addr())
	if err != nil {
		t.Fatalf("split host/port: %v", err)
	}

	cfg := SMTPConfig{Host: host, Port: port, User: "spuri@example.com", Pass: "senha-de-app", From: "spuri@example.com"}

	err = sendSMTPEmail(cfg, "admin@example.com", "Admin Teste", "Nova instituição cadastrada: Academia Teste",
		"Ola Admin Teste! A instituicao Academia Teste esta pendente de analise.",
		"<p>Ola <strong>Admin Teste</strong>! A instituicao <strong>Academia Teste</strong> esta pendente de analise.</p>")
	if err != nil {
		t.Fatalf("sendSMTPEmail contra servidor mock falhou: %v", err)
	}

	// Pequena espera para garantir que a goroutine do servidor já processou
	// o DATA e atualizado o estado antes de checarmos.
	deadline := time.Now().Add(2 * time.Second)
	for srv.getRawData() == "" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	if !srv.getAuthSeen() {
		t.Error("servidor mock nunca recebeu AUTH PLAIN — autenticação não ocorreu")
	}

	raw := srv.getRawData()
	if raw == "" {
		t.Fatal("servidor mock não recebeu nenhum conteúdo DATA")
	}
	for _, want := range []string{
		"Content-Type: multipart/alternative",
		"admin@example.com",
		"pendente de analise",
		"<strong>Academia Teste</strong>",
	} {
		if !strings.Contains(raw, want) {
			t.Errorf("conteúdo DATA recebido pelo servidor não contém %q\n--- recebido ---\n%s", want, raw)
		}
	}
}

// TestSendAcademiaCadastradaEmailSMTPAgainstRealServer é o mesmo teste,
// mas passando pelo caminho completo público
// (EmailService.SendAcademiaCadastradaEmailSMTP), do jeito que
// internal/handlers/academia_handlers.go realmente chama.
func TestSendAcademiaCadastradaEmailSMTPAgainstRealServer(t *testing.T) {
	srv := startMockSMTPServer(t)
	host, port, err := net.SplitHostPort(srv.addr())
	if err != nil {
		t.Fatalf("split host/port: %v", err)
	}

	t.Setenv("EMAIL_HOST", host)
	t.Setenv("EMAIL_PORT", port)
	t.Setenv("EMAIL_USER", "spuri@example.com")
	t.Setenv("EMAIL_PASS", "senha-de-app")
	t.Setenv("FRONTEND_URL", "https://painel.exemplo.com")

	svc := NewEmailService(nil)
	sendErr := svc.SendAcademiaCadastradaEmailSMTP("admin@example.com", "Admin Teste", AcademiaCadastradaInfo{
		Nome:           "Academia Teste",
		CodigoAcademia: "LDA2026A001",
		NIF:            "5417845812",
		Nivel:          "escola",
		Type:           "private",
		Provincia:      "LDA",
	})
	if sendErr != nil {
		t.Fatalf("SendAcademiaCadastradaEmailSMTP contra servidor mock falhou: %v", sendErr)
	}

	deadline := time.Now().Add(2 * time.Second)
	for srv.getRawData() == "" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	raw := srv.getRawData()
	if raw == "" {
		t.Fatal("servidor mock não recebeu nenhum conteúdo DATA")
	}
	for _, want := range []string{
		"LDA2026A001",
		"5417845812",
		"https://painel.exemplo.com/academias",
	} {
		if !strings.Contains(raw, want) {
			t.Errorf("conteúdo DATA recebido pelo servidor não contém %q", want)
		}
	}
}
```

## 4. Checklist de aceitação

- [ ] `internal/services/smtp_mailer.go` existe com `SMTPConfig`, `loadSMTPConfig`, `sendSMTPEmail`, `buildMimeMessage`, `encodeHeaderWord`.
- [ ] `EmailService.SendAcademiaCadastradaEmailSMTP` existe em `email_service.go`, usa `loadSMTPConfig`/`sendSMTPEmail`, nunca bloqueia (retorna `nil` quando SMTP não está configurado, igual ao padrão já usado pelo resto do serviço).
- [ ] `notificarAdminsSobreNovaAcademiaPendente` (em `academia_handlers.go`) chama `SendAcademiaCadastradaEmailSMTP`, não mais `SendAcademiaCadastradaEmail`.
- [ ] `SendAcademiaCadastradaEmail` (EmailJS) **continua existindo** no arquivo — não foi apagada.
- [ ] `GetAdminsParaNotificarNovaAcademia` não foi tocada — lista de destinatários continua 100% automática, vinda do banco, sem lista manual em lugar nenhum.
- [ ] `EMAIL_HOST`, `EMAIL_PORT`, `EMAIL_USER`, `EMAIL_PASS`, `EMAIL_FROM` documentadas em `.env.example`.
- [ ] Nenhuma dependência nova em `go.mod`/`go.sum`.
- [ ] Nenhuma migration nova.
- [ ] Nenhuma mudança no `spuripainel`, nem no módulo de Comunicação SMS deste repositório.
- [ ] `go build ./...` e `go vet ./...` limpos.
- [ ] `gofmt -l` não lista nenhum dos arquivos tocados/criados.
- [ ] `go test ./...` (sem `SPURI_RUN_DB_INTEGRITY_TESTS` — nenhum teste desta tarefa precisa de banco) limpo, incluindo os 8 testes novos: 6 unitários em `smtp_mailer_test.go` e 2 end-to-end (servidor SMTP mock real) em `smtp_mailer_e2e_test.go`.

## 5. Validação já feita (evidência real)

- **Ambiente**: Go 1.24.4 + PostgreSQL 16 (via apt), no `main` atual (já com a tarefa 92 e o módulo de Comunicação SMS mesclados — confirmei puxando o repositório antes de escrever este documento).
- **Build/vet/gofmt**: limpos, antes e depois, tanto na minha cópia de trabalho quanto reaplicando os diffs desta seção via `git apply` num clone limpo do `main` atual (com os 3 arquivos novos copiados por cima) — build, vet e gofmt saíram limpos de novo, de forma independente.
- **Suíte completa, sem banco**: `go test ./...` limpo, nos dois ambientes (cópia de trabalho e clone limpo reaplicado).
- **Suíte completa, com banco real, banco recriado do zero** (`DROP DATABASE`+`CREATE DATABASE`+ migrations + `SPURI_RUN_DB_INTEGRITY_TESTS=1`): limpo, nos dois ambientes — esta tarefa não mexe em nada que dependa de banco, mas rodei mesmo assim para garantir que nada mais quebrou.
- **Revert-and-confirm no teste de wiring já existente** (`TestRegisterAcademiaPublicaNotificaAdminsRegistroAcademia`, da tarefa 92): reverti só a chamada em `academia_handlers.go` de volta para `SendAcademiaCadastradaEmail` (sem o `SMTP` no final) sem atualizar a asserção do teste → falhou exatamente como esperado ("source does not contain \"SendAcademiaCadastradaEmailSMTP(\""). Restaurei e reconfirmei verde, com `go build ./...` limpo depois.
- **Teste de ponta a ponta contra um servidor SMTP real** (não é só "a lógica parece certa" — é o protocolo de verdade): subi um servidor SMTP mínimo em `127.0.0.1` dentro do próprio processo de teste, e `sendSMTPEmail`/`SendAcademiaCadastradaEmailSMTP` completaram o fluxo real — `EHLO`, `AUTH PLAIN` (aceito sem TLS porque `net/smtp.PlainAuth` reconhece `127.0.0.1` como localhost, dispensando a exigência de conexão segura só para esse caso), `MAIL FROM`, `RCPT TO`, `DATA` — e o conteúdo bruto que o servidor recebeu contém exatamente o texto, o HTML e os dados da academia (código, NIF, link do painel) esperados.
- **Escaping HTML e codificação de cabeçalho testados isoladamente**: nome de admin/academia com `<script>`/`&`/`<b>` sai escapado no HTML (não literal); nomes e assuntos com acento (`José`, `instituição`) saem codificados em RFC 2047 no cabeçalho, nunca como UTF-8 cru.

**O que não pôde ser testado neste ambiente**: entrega real por um provedor SMTP de verdade (Gmail) — meu sandbox não tem acesso de rede a `smtp.gmail.com`. O teste de ponta a ponta contra o servidor mock cobre 100% do código que este projeto escreve (construção da mensagem, autenticação, sequência de comandos); a única coisa que fica de fora é a infraestrutura de rede/TLS do provedor real em si, que é código do próprio Go (`net/smtp`, `crypto/tls`), não deste projeto. Recomendo, depois de configurar `EMAIL_HOST/EMAIL_USER/EMAIL_PASS` com uma conta Gmail real (senha de app) em homologação, fazer um autocadastro de teste e conferir a caixa de entrada de um admin de teste.

## 6. Sobre "gratuito": limites do Gmail SMTP

Gmail (pessoal) permite até ~500 emails/dia por conta via SMTP com senha de app — bem acima do volume esperado deste aviso (um email por autocadastro de academia, multiplicado pelo número de admins ativos com permissão de ativação, tipicamente poucas unidades). Se o volume crescer muito no futuro, alternativas com camada gratuita maior existem (Brevo/Sendinblue: 300/dia; Amazon SES: muito barato mas precisa de verificação de domínio) — nenhuma mudança de código seria necessária além de trocar `EMAIL_HOST`/`EMAIL_PORT`/usuário/senha, já que `net/smtp` é um cliente SMTP padrão, não específico do Gmail.

## 7. Nota sobre numeração desta tarefa

Esta tarefa foi originalmente escrita como "94", mas por volta do mesmo período outra tarefa completamente não relacionada ("Bloquear type, validar dependências ao trocar nivel_escolar e autoatualização de BI sem academia") também recebeu o número 94, foi executada primeiro e já está arquivada em `docs/Tarefas feitas/94 - ...md`. Renumerei esta para **95** antes de entregar, e reconferi de novo — build, vet, gofmt e suíte completa (com e sem banco) — aplicando os diffs desta seção 3 contra o `main` real já com aquela tarefa 94 mesclada (commit `d5c1fa3`). Nenhum dos arquivos que aquela tarefa tocou (`nivel_escolar_handlers.go`, `bilhete_identidade_sem_academia_handlers.go`, `academia_projection.go`, `estudante_projection.go`, etc.) se sobrepõe aos arquivos desta tarefa (`academia_handlers.go`, `admin_projection.go` não é tocado por esta tarefa, `email_service.go`) — são mudanças totalmente independentes.

## 8. Ao terminar

Mova este arquivo para `docs/Tarefas feitas/95 - Enviar o aviso de nova academia por SMTP puro no backend, sem EmailJS.md` depois que o checklist da seção 4 estiver 100% marcado.
