// Envio de email via SMTP puro (net/smtp, biblioteca padrão do Go).
package services

import (
	"bytes"
	"fmt"
	"mime"
	"net/smtp"
	"os"
)

// SMTPConfig agrupa a configuração de um provedor SMTP qualquer.
type SMTPConfig struct {
	Host string
	Port string
	User string
	Pass string
	From string // remetente exibido; se EMAIL_FROM vazio, usa EMAIL_USER
}

// loadSMTPConfig lê a configuração SMTP do ambiente.
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
func sendSMTPEmail(cfg SMTPConfig, toEmail, toName, subject, textBody, htmlBody string) error {
	auth := smtp.PlainAuth("", cfg.User, cfg.Pass, cfg.Host)
	msg := buildMimeMessage(cfg.From, toEmail, toName, subject, textBody, htmlBody)
	addr := fmt.Sprintf("%s:%s", cfg.Host, cfg.Port)
	return smtp.SendMail(addr, auth, cfg.From, []string{toEmail}, msg)
}

// encodeHeaderWord codifica cabeçalhos em RFC 2047 (UTF-8).
func encodeHeaderWord(s string) string {
	return mime.QEncoding.Encode("UTF-8", s)
}

// buildMimeMessage monta um email RFC 5322 multipart/alternative.
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
