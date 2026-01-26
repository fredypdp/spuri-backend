package services

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"gopkg.in/gomail.v2"
)

type EmailService struct {
	db          *sqlx.DB
	dialer      *gomail.Dialer
	fromEmail   string
	fromName    string
	frontendURL string
	enabled     bool
}

func NewEmailService(db *sqlx.DB) *EmailService {
	smtpHost := os.Getenv("SMTP_HOST")
	smtpPort := getEnvInt("SMTP_PORT", 587)
	smtpUser := os.Getenv("SMTP_USER")
	smtpPass := os.Getenv("SMTP_PASSWORD")

	enabled := smtpHost != "" && smtpUser != "" && smtpPass != ""

	if !enabled {
		log.Println("[EMAIL] ⚠️  Serviço DESABILITADO - configure SMTP_HOST, SMTP_USER, SMTP_PASSWORD")
		return &EmailService{
			db:          db,
			enabled:     false,
			frontendURL: getEnvOrDefault("FRONTEND_URL", "http://localhost:3000"),
		}
	}

	// Criar dialer com configurações otimizadas para Gmail
	d := gomail.NewDialer(smtpHost, smtpPort, smtpUser, smtpPass)
	
	// Configurações SSL/TLS otimizadas
	d.SSL = smtpPort == 465
	
	// Testar conexão na inicialização
	s := gomail.NewDialer(smtpHost, smtpPort, smtpUser, smtpPass)
	s.SSL = d.SSL
	
	testConn, closeFunc, err := s.Dial()
	if err != nil {
		log.Printf("[EMAIL] ❌ Erro ao conectar SMTP: %v", err)
		enabled = false
	} else {
		closeFunc()
		testConn.Close()
		log.Printf("[EMAIL] ✅ Conectado: %s:%d (SSL: %v)", smtpHost, smtpPort, d.SSL)
	}

	return &EmailService{
		db:          db,
		dialer:      d,
		fromEmail:   getEnvOrDefault("FROM_EMAIL", smtpUser),
		fromName:    getEnvOrDefault("FROM_NAME", "Spuri"),
		frontendURL: getEnvOrDefault("FRONTEND_URL", "http://localhost:3000"),
		enabled:     enabled,
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
	log.Printf("[EMAIL] Gerando token - UserID: %s, Type: %s, Tipo: %s", userID, userType, tipo)

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

	log.Printf("[EMAIL] ✅ Token salvo - Expira: %s", expiresAt.Format("2006-01-02 15:04:05"))
	return token, nil
}

func (s *EmailService) VerifyToken(token, tipo string) (*TokenInfo, error) {
	log.Printf("[EMAIL] Verificando token - Tipo: %s", tipo)

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

	// Marcar como usado
	updateQuery := fmt.Sprintf(`
		UPDATE auth_tokens 
		SET usado = TRUE, usado_em = CURRENT_TIMESTAMP 
		WHERE token = '%s'
	`, token)
	s.db.Exec(updateQuery)

	log.Printf("[EMAIL] ✅ Token válido e marcado como usado")
	return &info, nil
}

// SendEmail - Método principal para enviar emails
func (s *EmailService) SendEmail(to, subject, htmlBody string) error {
	if !s.enabled {
		log.Printf("[EMAIL] ⚠️  Serviço desabilitado - email não enviado")
		return fmt.Errorf("serviço de email desabilitado")
	}

	if to == "" {
		return fmt.Errorf("destinatário vazio")
	}

	log.Printf("[EMAIL] 📧 Preparando envio para: %s", to)
	log.Printf("[EMAIL] 📋 Assunto: %s", subject)

	// Criar mensagem
	m := gomail.NewMessage()
	m.SetHeader("From", m.FormatAddress(s.fromEmail, s.fromName))
	m.SetHeader("To", to)
	m.SetHeader("Subject", subject)
	m.SetBody("text/html", htmlBody)

	// Enviar com retry automático
	maxRetries := 3
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		log.Printf("[EMAIL] 🔄 Tentativa %d/%d...", attempt, maxRetries)

		// Criar nova conexão para cada tentativa
		d := gomail.NewDialer(s.dialer.Host, s.dialer.Port, s.dialer.Username, s.dialer.Password)
		d.SSL = s.dialer.SSL

		err := d.DialAndSend(m)
		if err == nil {
			log.Printf("[EMAIL] ✅ Email enviado com sucesso para: %s", to)
			return nil
		}

		lastErr = err
		log.Printf("[EMAIL] ❌ Tentativa %d falhou: %v", attempt, err)

		if attempt < maxRetries {
			waitTime := time.Duration(attempt*2) * time.Second
			log.Printf("[EMAIL] ⏳ Aguardando %v antes de retentar...", waitTime)
			time.Sleep(waitTime)
		}
	}

	log.Printf("[EMAIL] ❌ Falha após %d tentativas: %v", maxRetries, lastErr)
	return fmt.Errorf("falha ao enviar email após %d tentativas: %w", maxRetries, lastErr)
}

func (s *EmailService) SendVerificationEmail(userID uuid.UUID, userType, email, nome string) error {
	log.Printf("[EMAIL] 📨 Iniciando envio de verificação - Email: %s, Nome: %s", email, nome)

	if !s.enabled {
		log.Println("[EMAIL] ⚠️  Serviço desabilitado")
		return fmt.Errorf("serviço de email desabilitado")
	}

	if email == "" {
		return fmt.Errorf("email vazio")
	}

	token, err := s.SaveToken(userID, userType, "verificacao_email", email, 24*time.Hour)
	if err != nil {
		return fmt.Errorf("erro ao gerar token: %w", err)
	}

	verifyURL := fmt.Sprintf("%s/verificar-email/%s", s.frontendURL, token)
	log.Printf("[EMAIL] 🔗 URL de verificação: %s", verifyURL)

	subject := "Verifique seu email - Spuri"
	body := s.buildVerificationEmailHTML(nome, verifyURL)

	return s.SendEmail(email, subject, body)
}

func (s *EmailService) SendPasswordResetEmail(userID uuid.UUID, userType, email, nome string) error {
	log.Printf("[EMAIL] 📨 Iniciando recuperação de senha - Email: %s, Nome: %s", email, nome)

	if !s.enabled {
		log.Println("[EMAIL] ⚠️  Serviço desabilitado")
		return fmt.Errorf("serviço de email desabilitado")
	}

	if email == "" {
		return fmt.Errorf("email vazio")
	}

	token, err := s.SaveToken(userID, userType, "recuperacao_senha", email, 1*time.Hour)
	if err != nil {
		return fmt.Errorf("erro ao gerar token: %w", err)
	}

	resetURL := fmt.Sprintf("%s/recuperar-senha/%s", s.frontendURL, token)
	log.Printf("[EMAIL] 🔗 URL de recuperação: %s", resetURL)

	subject := "Recuperação de Senha - Spuri"
	body := s.buildPasswordResetEmailHTML(nome, resetURL)

	return s.SendEmail(email, subject, body)
}

// Templates HTML
func (s *EmailService) buildVerificationEmailHTML(nome, verifyURL string) string {
	return fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
</head>
<body style="font-family: Arial, sans-serif; line-height: 1.6; color: #333; margin: 0; padding: 0;">
	<div style="max-width: 600px; margin: 0 auto; padding: 20px;">
		<div style="background: linear-gradient(135deg, #2563eb 0%%, #1e40af 100%%); padding: 30px; text-align: center; border-radius: 10px 10px 0 0;">
			<h1 style="color: white; margin: 0; font-size: 28px;">Bem-vindo ao Spuri!</h1>
		</div>
		
		<div style="background: #ffffff; padding: 30px; border: 1px solid #e5e7eb; border-top: none; border-radius: 0 0 10px 10px;">
			<p style="font-size: 18px; margin-bottom: 20px;">Olá <strong>%s</strong>,</p>
			
			<p style="margin-bottom: 20px;">
				Para completar seu cadastro e começar a usar o Spuri, 
				precisamos verificar seu endereço de email.
			</p>
			
			<div style="text-align: center; margin: 30px 0;">
				<a href="%s" 
				   style="background-color: #2563eb; 
				          color: white; 
				          padding: 14px 40px; 
				          text-decoration: none; 
				          border-radius: 8px; 
				          display: inline-block;
				          font-weight: bold;
				          font-size: 16px;">
					Verificar Email
				</a>
			</div>
			
			<div style="background-color: #f3f4f6; padding: 15px; border-radius: 8px; margin-top: 25px;">
				<p style="margin: 0; font-size: 14px; color: #6b7280;">
					<strong>⏰ Este link expira em 24 horas.</strong><br>
					Se você não criou esta conta, pode ignorar este email com segurança.
				</p>
			</div>
			
			<div style="margin-top: 30px; padding-top: 20px; border-top: 1px solid #e5e7eb;">
				<p style="font-size: 12px; color: #9ca3af; margin: 0;">
					Se o botão não funcionar, copie e cole este link no seu navegador:<br>
					<a href="%s" style="color: #2563eb; word-break: break-all;">%s</a>
				</p>
			</div>
		</div>
		
		<div style="text-align: center; margin-top: 20px; color: #9ca3af; font-size: 12px;">
			<p>© 2026 Spuri - Sistema de Gestão Acadêmica</p>
		</div>
	</div>
</body>
</html>
	`, nome, verifyURL, verifyURL, verifyURL)
}

func (s *EmailService) buildPasswordResetEmailHTML(nome, resetURL string) string {
	return fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
</head>
<body style="font-family: Arial, sans-serif; line-height: 1.6; color: #333; margin: 0; padding: 0;">
	<div style="max-width: 600px; margin: 0 auto; padding: 20px;">
		<div style="background: linear-gradient(135deg, #dc2626 0%%, #991b1b 100%%); padding: 30px; text-align: center; border-radius: 10px 10px 0 0;">
			<h1 style="color: white; margin: 0; font-size: 28px;">Recuperação de Senha</h1>
		</div>
		
		<div style="background: #ffffff; padding: 30px; border: 1px solid #e5e7eb; border-top: none; border-radius: 0 0 10px 10px;">
			<p style="font-size: 18px; margin-bottom: 20px;">Olá <strong>%s</strong>,</p>
			
			<p style="margin-bottom: 20px;">
				Recebemos uma solicitação para redefinir a senha da sua conta Spuri.
			</p>
			
			<p style="margin-bottom: 20px;">
				Se você não fez esta solicitação, pode ignorar este email com segurança. 
				Sua senha atual permanecerá inalterada.
			</p>
			
			<div style="text-align: center; margin: 30px 0;">
				<a href="%s" 
				   style="background-color: #dc2626; 
				          color: white; 
				          padding: 14px 40px; 
				          text-decoration: none; 
				          border-radius: 8px; 
				          display: inline-block;
				          font-weight: bold;
				          font-size: 16px;">
					Redefinir Senha
				</a>
			</div>
			
			<div style="background-color: #fef2f2; padding: 15px; border-radius: 8px; margin-top: 25px; border-left: 4px solid #dc2626;">
				<p style="margin: 0; font-size: 14px; color: #991b1b;">
					<strong>⏰ Este link expira em 1 hora.</strong><br>
					Por segurança, você precisará solicitar um novo link após este período.
				</p>
			</div>
			
			<div style="margin-top: 30px; padding-top: 20px; border-top: 1px solid #e5e7eb;">
				<p style="font-size: 12px; color: #9ca3af; margin: 0;">
					Se o botão não funcionar, copie e cole este link no seu navegador:<br>
					<a href="%s" style="color: #dc2626; word-break: break-all;">%s</a>
				</p>
			</div>
		</div>
		
		<div style="text-align: center; margin-top: 20px; color: #9ca3af; font-size: 12px;">
			<p>© 2026 Spuri - Sistema de Gestão Acadêmica</p>
		</div>
	</div>
</body>
</html>
	`, nome, resetURL, resetURL, resetURL)
}

// Funções auxiliares
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

func getEnvInt(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	var result int
	fmt.Sscanf(value, "%d", &result)
	if result == 0 {
		return defaultValue
	}
	return result
}

type TokenInfo struct {
	UserID    uuid.UUID `db:"user_id"`
	UserType  string    `db:"user_type"`
	Email     string    `db:"email"`
	Usado     bool      `db:"usado"`
	ExpiresAt time.Time `db:"expires_at"`
}