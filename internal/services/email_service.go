// ============================================================================
// ARQUIVO: internal/services/email_service.go
// 🔒 CORRIGIDO: Tratamento robusto de erros em emails
// ============================================================================

package services

import (
	"crypto/rand"
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
	enabled    bool // 🔒 NOVO: Flag para habilitar/desabilitar emails
}

func NewEmailService(db *sqlx.DB) *EmailService {
	// Verificar se configurações de email estão presentes
	smtpHost := os.Getenv("SMTP_HOST")
	smtpUser := os.Getenv("SMTP_USER")
	smtpPass := os.Getenv("SMTP_PASSWORD")
	
	enabled := smtpHost != "" && smtpUser != "" && smtpPass != ""
	
	if !enabled {
		log.Println("⚠️ Serviço de email DESABILITADO - variáveis SMTP não configuradas")
	} else {
		log.Println("✅ Serviço de email HABILITADO")
	}

	return &EmailService{
		db:        db,
		smtpHost:  smtpHost,
		smtpPort:  getEnvOrDefault("SMTP_PORT", "587"),
		smtpUser:  smtpUser,
		smtpPass:  smtpPass,
		fromEmail: getEnvOrDefault("FROM_EMAIL", smtpUser),
		fromName:  getEnvOrDefault("FROM_NAME", "Spuri"),
		baseURL:   getEnvOrDefault("BASE_URL", "http://localhost:8080"),
		enabled:   enabled,
	}
}

// IsEnabled retorna se o serviço de email está habilitado
func (s *EmailService) IsEnabled() bool {
	return s.enabled
}

// GenerateToken gera token aleatório seguro
func GenerateToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// SaveToken salva token no banco
func (s *EmailService) SaveToken(userID uuid.UUID, userType, tipo, email string, expiresIn time.Duration) (string, error) {
	token, err := GenerateToken()
	if err != nil {
		return "", fmt.Errorf("erro ao gerar token: %w", err)
	}

	expiresAt := time.Now().Add(expiresIn)

	query := `
		INSERT INTO auth_tokens (user_id, user_type, token, tipo, email, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`

	_, err = s.db.Exec(query, userID, userType, token, tipo, email, expiresAt)
	if err != nil {
		return "", fmt.Errorf("erro ao salvar token: %w", err)
	}

	return token, nil
}

// VerifyToken verifica e marca token como usado
func (s *EmailService) VerifyToken(token, tipo string) (*TokenInfo, error) {
	query := `
		SELECT user_id, user_type, email, usado, expires_at
		FROM auth_tokens
		WHERE token = $1 AND tipo = $2
	`

	var info TokenInfo
	err := s.db.QueryRow(query, token, tipo).Scan(
		&info.UserID, &info.UserType, &info.Email, &info.Usado, &info.ExpiresAt,
	)
	if err != nil {
		return nil, fmt.Errorf("token inválido ou expirado")
	}

	if info.Usado {
		return nil, fmt.Errorf("token já foi usado")
	}

	if time.Now().After(info.ExpiresAt) {
		return nil, fmt.Errorf("token expirado")
	}

	// Marcar como usado
	updateQuery := `
		UPDATE auth_tokens 
		SET usado = TRUE, usado_em = CURRENT_TIMESTAMP 
		WHERE token = $1
	`
	s.db.Exec(updateQuery, token)

	return &info, nil
}

// SendEmail envia email usando Google SMTP
// 🔒 CORRIGIDO: Retorna erro detalhado em vez de panic
func (s *EmailService) SendEmail(to, subject, body string) error {
	if !s.enabled {
		return fmt.Errorf("serviço de email desabilitado - configure SMTP_HOST, SMTP_USER e SMTP_PASSWORD")
	}

	if to == "" {
		return fmt.Errorf("destinatário vazio")
	}

	auth := smtp.PlainAuth("", s.smtpUser, s.smtpPass, s.smtpHost)

	msg := fmt.Sprintf("From: %s <%s>\r\n"+
		"To: %s\r\n"+
		"Subject: %s\r\n"+
		"MIME-Version: 1.0\r\n"+
		"Content-Type: text/html; charset=UTF-8\r\n"+
		"\r\n"+
		"%s", s.fromName, s.fromEmail, to, subject, body)

	addr := fmt.Sprintf("%s:%s", s.smtpHost, s.smtpPort)
	
	err := smtp.SendMail(addr, auth, s.fromEmail, []string{to}, []byte(msg))
	if err != nil {
		log.Printf("❌ [EMAIL] Erro ao enviar para %s: %v", to, err)
		return fmt.Errorf("falha ao enviar email: %w", err)
	}

	log.Printf("✅ [EMAIL] Enviado com sucesso para %s", to)
	return nil
}

// SendVerificationEmail envia email de verificação
// 🔒 CORRIGIDO: Retorna erro em vez de apenas logar
func (s *EmailService) SendVerificationEmail(userID uuid.UUID, userType, email, nome string) error {
	if !s.enabled {
		log.Println("⚠️ [EMAIL] Serviço desabilitado - pulando envio de verificação")
		return fmt.Errorf("serviço de email desabilitado")
	}

	if email == "" {
		return fmt.Errorf("email vazio - não é possível enviar verificação")
	}

	token, err := s.SaveToken(userID, userType, "verificacao_email", email, 24*time.Hour)
	if err != nil {
		return fmt.Errorf("erro ao gerar token de verificação: %w", err)
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

// SendPasswordResetEmail envia email de recuperação de senha
// 🔒 CORRIGIDO: Retorna erro em vez de apenas logar
func (s *EmailService) SendPasswordResetEmail(userID uuid.UUID, userType, email, nome string) error {
	if !s.enabled {
		log.Println("⚠️ [EMAIL] Serviço desabilitado - pulando envio de recuperação")
		return fmt.Errorf("serviço de email desabilitado")
	}

	if email == "" {
		return fmt.Errorf("email vazio - não é possível enviar recuperação")
	}

	token, err := s.SaveToken(userID, userType, "recuperacao_senha", email, 1*time.Hour)
	if err != nil {
		return fmt.Errorf("erro ao gerar token de recuperação: %w", err)
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

// GetDefaultPassword retorna senha padrão por tipo
func GetDefaultPassword(userType, codigo string) string {
	switch userType {
	case "estudante":
		return codigo // código do estudante
	case "academia":
		return codigo // código da academia
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

// 🔒 NOVO: Helper para pegar variável de ambiente com valor padrão
func getEnvOrDefault(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

type TokenInfo struct {
	UserID    uuid.UUID
	UserType  string
	Email     string
	Usado     bool
	ExpiresAt time.Time
}