package services

import (
	"crypto/rand"
	"crypto/tls"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"net/smtp"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type EmailService struct {
	db         *sqlx.DB
	smtpHost   string
	smtpPort   string
	smtpUser   string
	smtpPass   string
	fromEmail  string
	fromName   string
	baseURL    string
	enabled    bool
	useSSL     bool
}

func NewEmailService(db *sqlx.DB) *EmailService {
	smtpHost := os.Getenv("SMTP_HOST")
	smtpUser := os.Getenv("SMTP_USER")
	smtpPass := os.Getenv("SMTP_PASSWORD")
	smtpPort := getEnvOrDefault("SMTP_PORT", "587")
	useSSL := smtpPort == "465"
	
	enabled := smtpHost != "" && smtpUser != "" && smtpPass != ""
	
	if !enabled {
		log.Println("[EMAIL] Serviço DESABILITADO - variáveis SMTP não configuradas")
	} else {
		log.Printf("[EMAIL] Serviço HABILITADO (Host: %s:%s, SSL: %v)", smtpHost, smtpPort, useSSL)
	}

	return &EmailService{
		db:        db,
		smtpHost:  smtpHost,
		smtpPort:  smtpPort,
		smtpUser:  smtpUser,
		smtpPass:  smtpPass,
		fromEmail: getEnvOrDefault("FROM_EMAIL", smtpUser),
		fromName:  getEnvOrDefault("FROM_NAME", "Spuri"),
		baseURL:   getEnvOrDefault("BASE_URL", "http://localhost:8080"),
		enabled:   enabled,
		useSSL:    useSSL,
	}
}

func (s *EmailService) IsEnabled() bool {
	return s.enabled
}

func GenerateToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func (s *EmailService) SaveToken(userID uuid.UUID, userType, tipo, email string, expiresIn time.Duration) (string, error) {
	token, err := GenerateToken()
	if err != nil {
		return "", fmt.Errorf("erro ao gerar token: %w", err)
	}

	expiresAt := time.Now().Add(expiresIn)

	query := fmt.Sprintf(`
		INSERT INTO auth_tokens (user_id, user_type, token, tipo, email, expires_at)
		VALUES ('%s', '%s', '%s', '%s', '%s', '%s')
	`, userID.String(), userType, token, tipo, email, expiresAt.Format("2006-01-02 15:04:05"))

	_, err = s.db.Exec(query)
	if err != nil {
		return "", fmt.Errorf("erro ao salvar token: %w", err)
	}

	return token, nil
}

func (s *EmailService) VerifyToken(token, tipo string) (*TokenInfo, error) {
	query := fmt.Sprintf(`
		SELECT user_id, user_type, email, usado, expires_at
		FROM auth_tokens
		WHERE token = '%s' AND tipo = '%s'
	`, token, tipo)

	var info TokenInfo
	
	err := s.db.QueryRow(query).Scan(
		&info.UserID,
		&info.UserType,
		&info.Email,
		&info.Usado,
		&info.ExpiresAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("token inválido ou expirado")
		}
		return nil, fmt.Errorf("erro ao verificar token: %w", err)
	}

	if info.Usado {
		return nil, fmt.Errorf("token já foi usado")
	}

	if time.Now().After(info.ExpiresAt) {
		return nil, fmt.Errorf("token expirado")
	}

	updateQuery := fmt.Sprintf(`
		UPDATE auth_tokens 
		SET usado = TRUE, usado_em = CURRENT_TIMESTAMP 
		WHERE token = '%s'
	`, token)
	s.db.Exec(updateQuery)

	return &info, nil
}

func (s *EmailService) SendEmail(to, subject, body string) error {
	if !s.enabled {
		return fmt.Errorf("serviço de email desabilitado")
	}

	if to == "" {
		return fmt.Errorf("destinatário vazio")
	}

	log.Printf("[EMAIL] Tentando enviar para %s via %s:%s (SSL: %v)", 
		to, s.smtpHost, s.smtpPort, s.useSSL)

	msg := fmt.Sprintf("From: %s <%s>\r\n"+
		"To: %s\r\n"+
		"Subject: %s\r\n"+
		"MIME-Version: 1.0\r\n"+
		"Content-Type: text/html; charset=UTF-8\r\n"+
		"\r\n"+
		"%s", s.fromName, s.fromEmail, to, subject, body)

	addr := fmt.Sprintf("%s:%s", s.smtpHost, s.smtpPort)

	if s.useSSL {
		// Porta 465 - SSL direto
		return s.sendMailSSL(addr, to, msg)
	} else {
		// Porta 587 - STARTTLS
		return s.sendMailTLS(addr, to, msg)
	}
}

func (s *EmailService) sendMailTLS(addr, to, msg string) error {
	auth := smtp.PlainAuth("", s.smtpUser, s.smtpPass, s.smtpHost)
	
	err := smtp.SendMail(addr, auth, s.fromEmail, []string{to}, []byte(msg))
	if err != nil {
		log.Printf("[EMAIL] ERRO STARTTLS: %v", err)
		return fmt.Errorf("falha ao enviar email (STARTTLS): %w", err)
	}

	log.Printf("[EMAIL] ✓ Email enviado com sucesso via STARTTLS")
	return nil
}

func (s *EmailService) sendMailSSL(addr, to, msg string) error {
	auth := smtp.PlainAuth("", s.smtpUser, s.smtpPass, s.smtpHost)

	tlsConfig := &tls.Config{
		InsecureSkipVerify: false,
		ServerName:         s.smtpHost,
	}

	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		log.Printf("[EMAIL] ERRO SSL Dial: %v", err)
		return fmt.Errorf("falha ao conectar SSL: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, s.smtpHost)
	if err != nil {
		log.Printf("[EMAIL] ERRO SSL Client: %v", err)
		return fmt.Errorf("falha ao criar cliente SMTP: %w", err)
	}
	defer client.Quit()

	if err = client.Auth(auth); err != nil {
		log.Printf("[EMAIL] ERRO Autenticação: %v", err)
		return fmt.Errorf("falha na autenticação: %w", err)
	}

	if err = client.Mail(s.fromEmail); err != nil {
		return fmt.Errorf("falha ao definir remetente: %w", err)
	}

	if err = client.Rcpt(to); err != nil {
		return fmt.Errorf("falha ao definir destinatário: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("falha ao abrir data writer: %w", err)
	}

	_, err = w.Write([]byte(msg))
	if err != nil {
		return fmt.Errorf("falha ao escrever mensagem: %w", err)
	}

	err = w.Close()
	if err != nil {
		return fmt.Errorf("falha ao fechar data writer: %w", err)
	}

	log.Printf("[EMAIL] ✓ Email enviado com sucesso via SSL")
	return nil
}

func (s *EmailService) SendVerificationEmail(userID uuid.UUID, userType, email, nome string) error {
	if !s.enabled {
		log.Println("[EMAIL] Serviço desabilitado - pulando verificação")
		return fmt.Errorf("serviço de email desabilitado")
	}

	if email == "" {
		return fmt.Errorf("email vazio")
	}

	token, err := s.SaveToken(userID, userType, "verificacao_email", email, 24*time.Hour)
	if err != nil {
		log.Printf("[EMAIL] Erro ao salvar token: %v", err)
		return fmt.Errorf("erro ao gerar token: %w", err)
	}

	verifyURL := fmt.Sprintf("%s/verificar-email/%s", s.baseURL, token)

	subject := "Verifique seu email - Spuri"
	body := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<body style="font-family: Arial, sans-serif; line-height: 1.6; color: #333;">
	<div style="max-width: 600px; margin: 0 auto; padding: 20px;">
		<h2 style="color: #2563eb;">Bem-vindo ao Spuri, %s!</h2>
		<p>Para completar seu cadastro, verifique seu email clicando no botão abaixo:</p>
		<div style="text-align: center; margin: 30px 0;">
			<a href="%s" style="background-color: #2563eb; color: white; padding: 12px 30px; text-decoration: none; border-radius: 5px; display: inline-block;">
				Verificar Email
			</a>
		</div>
		<p style="color: #666; font-size: 14px;">
			Este link expira em 24 horas.<br>
			Se você não criou esta conta, ignore este email.
		</p>
		<p style="color: #666; font-size: 12px; margin-top: 30px; border-top: 1px solid #eee; padding-top: 20px;">
			Link alternativo: <a href="%s">%s</a>
		</p>
	</div>
</body>
</html>
	`, nome, verifyURL, verifyURL, verifyURL)

	return s.SendEmail(email, subject, body)
}

func (s *EmailService) SendPasswordResetEmail(userID uuid.UUID, userType, email, nome string) error {
	if !s.enabled {
		log.Println("[EMAIL] Serviço desabilitado - pulando recuperação")
		return fmt.Errorf("serviço de email desabilitado")
	}

	if email == "" {
		return fmt.Errorf("email vazio")
	}

	token, err := s.SaveToken(userID, userType, "recuperacao_senha", email, 1*time.Hour)
	if err != nil {
		return fmt.Errorf("erro ao gerar token: %w", err)
	}

	resetURL := fmt.Sprintf("%s/recuperar-senha/%s", s.baseURL, token)

	subject := "Recuperação de Senha - Spuri"
	body := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<body style="font-family: Arial, sans-serif; line-height: 1.6; color: #333;">
	<div style="max-width: 600px; margin: 0 auto; padding: 20px;">
		<h2 style="color: #dc2626;">Recuperação de Senha</h2>
		<p>Olá %s,</p>
		<p>Recebemos uma solicitação para redefinir sua senha. Clique no botão abaixo para criar uma nova senha:</p>
		<div style="text-align: center; margin: 30px 0;">
			<a href="%s" style="background-color: #dc2626; color: white; padding: 12px 30px; text-decoration: none; border-radius: 5px; display: inline-block;">
				Redefinir Senha
			</a>
		</div>
		<p style="color: #666; font-size: 14px;">
			Este link expira em 1 hora.<br>
			Se você não solicitou esta redefinição, ignore este email - sua senha permanecerá a mesma.
		</p>
		<p style="color: #666; font-size: 12px; margin-top: 30px; border-top: 1px solid #eee; padding-top: 20px;">
			Link alternativo: <a href="%s">%s</a>
		</p>
	</div>
</body>
</html>
	`, nome, resetURL, resetURL, resetURL)

	return s.SendEmail(email, subject, body)
}

func GetDefaultPassword(userType, codigo string) string {
	switch userType {
	case "estudante":
		return codigo
	case "academia":
		return codigo
	case "admin":
		return "spuriadm"
	case "gerente":
		return "spurigerente"
	case "fpp":
		return "spurifpp"
	default:
		return "spuri123"
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

type TokenInfo struct {
	UserID    uuid.UUID `db:"user_id"`
	UserType  string    `db:"user_type"`
	Email     string    `db:"email"`
	Usado     bool      `db:"usado"`
	ExpiresAt time.Time `db:"expires_at"`
}