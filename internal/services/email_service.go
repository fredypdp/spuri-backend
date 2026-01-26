package services

import (
	"crypto/rand"
	"crypto/tls"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"net"
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
		log.Println("[EMAIL] ⚠️  DESABILITADO - configure SMTP_HOST, SMTP_USER, SMTP_PASSWORD")
		return &EmailService{
			db:          db,
			enabled:     false,
			frontendURL: getEnvOrDefault("FRONTEND_URL", "http://localhost:3000"),
		}
	}

	log.Printf("[EMAIL] 🔧 Configurando - Host: %s, Port: %d", smtpHost, smtpPort)

	// Tentar detectar melhor porta
	detectedPort := detectBestPort(smtpHost, smtpPort)
	if detectedPort != smtpPort {
		log.Printf("[EMAIL] 🔄 Porta %d bloqueada, usando %d", smtpPort, detectedPort)
		smtpPort = detectedPort
	}

	// Criar dialer
	d := gomail.NewDialer(smtpHost, smtpPort, smtpUser, smtpPass)
	d.SSL = smtpPort == 465
	d.TLSConfig = &tls.Config{
		InsecureSkipVerify: false,
		ServerName:         smtpHost,
	}
	d.Timeout = time.Duration(getEnvInt("SMTP_TIMEOUT", 15)) * time.Second

	// Teste de conectividade
	log.Printf("[EMAIL] 🔐 Testando autenticação (timeout: %v)...", d.Timeout)
	testConn, err := d.Dial()
	if err != nil {
		log.Printf("[EMAIL] ❌ Falha: %v", err)
		log.Printf("[EMAIL] 💡 Sugestões:")
		log.Printf("   1. Porta %d bloqueada? Tente SMTP_PORT=465", smtpPort)
		log.Printf("   2. Firewall bloqueando SMTP? Configure regras de saída")
		log.Printf("   3. Gmail: use 'Senha de App' (não a senha normal)")
		log.Printf("   4. Considere usar relay SMTP (SendGrid, Mailgun, etc)")
		enabled = false
	} else {
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

// detectBestPort tenta detectar qual porta está disponível
func detectBestPort(host string, preferredPort int) int {
	ports := []int{preferredPort, 465, 587, 2525}
	timeout := 3 * time.Second

	for _, port := range ports {
		address := fmt.Sprintf("%s:%d", host, port)
		conn, err := net.DialTimeout("tcp", address, timeout)
		if err == nil {
			conn.Close()
			log.Printf("[EMAIL] ✅ Porta %d acessível", port)
			return port
		}
		log.Printf("[EMAIL] ⏭️  Porta %d bloqueada/timeout", port)
	}

	log.Printf("[EMAIL] ⚠️  Nenhuma porta SMTP acessível, usando %d (vai falhar)", preferredPort)
	return preferredPort
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

	log.Printf("[EMAIL] ✅ Token salvo - Expira: %s", expiresAt.Format("2006-01-02 15:04:05"))
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

func (s *EmailService) SendEmail(to, subject, htmlBody string) error {
	if !s.enabled {
		log.Printf("[EMAIL] ⚠️  Serviço desabilitado")
		return fmt.Errorf("serviço de email desabilitado")
	}

	if to == "" {
		return fmt.Errorf("destinatário vazio")
	}

	log.Printf("[EMAIL] 📧 Enviando para: %s - Assunto: %s", to, subject)

	m := gomail.NewMessage()
	m.SetHeader("From", m.FormatAddress(s.fromEmail, s.fromName))
	m.SetHeader("To", to)
	m.SetHeader("Subject", subject)
	m.SetBody("text/html", htmlBody)

	maxRetries := 2
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := s.dialer.DialAndSend(m)
		if err == nil {
			log.Printf("[EMAIL] ✅ Enviado (%d/%d)", attempt, maxRetries)
			return nil
		}

		lastErr = err
		log.Printf("[EMAIL] ❌ Tentativa %d falhou: %v", attempt, err)

		if attempt < maxRetries {
			time.Sleep(time.Duration(attempt*2) * time.Second)
		}
	}

	return fmt.Errorf("falha após %d tentativas: %w", maxRetries, lastErr)
}

func (s *EmailService) SendVerificationEmail(userID uuid.UUID, userType, email, nome string) error {
	if !s.enabled {
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
	subject := "Verifique seu email - Spuri"
	body := s.buildVerificationEmailHTML(nome, verifyURL)

	return s.SendEmail(email, subject, body)
}

func (s *EmailService) SendPasswordResetEmail(userID uuid.UUID, userType, email, nome string) error {
	if !s.enabled {
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
	subject := "Recuperação de Senha - Spuri"
	body := s.buildPasswordResetEmailHTML(nome, resetURL)

	return s.SendEmail(email, subject, body)
}

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
					Se você não criou esta conta, ignore este email.
				</p>
			</div>
			
			<div style="margin-top: 30px; padding-top: 20px; border-top: 1px solid #e5e7eb;">
				<p style="font-size: 12px; color: #9ca3af; margin: 0;">
					Se o botão não funcionar, copie e cole este link:<br>
					<a href="%s" style="color: #2563eb; word-break: break-all;">%s</a>
				</p>
			</div>
		</div>
		
		<div style="text-align: center; margin-top: 20px; color: #9ca3af; font-size: 12px;">
			<p>© 2026 Spuri</p>
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
				Recebemos uma solicitação para redefinir sua senha.
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
					<strong>⏰ Expira em 1 hora.</strong>
				</p>
			</div>
			
			<div style="margin-top: 30px; padding-top: 20px; border-top: 1px solid #e5e7eb;">
				<p style="font-size: 12px; color: #9ca3af; margin: 0;">
					Se o botão não funcionar:<br>
					<a href="%s" style="color: #dc2626; word-break: break-all;">%s</a>
				</p>
			</div>
		</div>
		
		<div style="text-align: center; margin-top: 20px; color: #9ca3af; font-size: 12px;">
			<p>© 2026 Spuri</p>
		</div>
	</div>
</body>
</html>
	`, nome, resetURL, resetURL, resetURL)
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