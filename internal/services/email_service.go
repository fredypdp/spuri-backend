package services

import (
	"crypto/rand"
	"crypto/tls"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"net"
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
	frontendURL string
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
		db:          db,
		smtpHost:    smtpHost,
		smtpPort:    smtpPort,
		smtpUser:    smtpUser,
		smtpPass:    smtpPass,
		fromEmail:   getEnvOrDefault("FROM_EMAIL", smtpUser),
		fromName:    getEnvOrDefault("FROM_NAME", "Spuri"),
		baseURL:     getEnvOrDefault("BASE_URL", "http://localhost:8080"),
		frontendURL: getEnvOrDefault("FRONTEND_URL", "http://localhost:3000"),
		enabled:     enabled,
		useSSL:      useSSL,
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
	log.Printf("[EMAIL][DEBUG] SaveToken - UserID: %s, Type: %s, Tipo: %s, Email: %s", userID, userType, tipo, email)
	
	token, err := GenerateToken()
	if err != nil {
		log.Printf("[EMAIL][DEBUG] Erro ao gerar token: %v", err)
		return "", fmt.Errorf("erro ao gerar token: %w", err)
	}

	expiresAt := time.Now().Add(expiresIn)
	log.Printf("[EMAIL][DEBUG] Token gerado, expira em: %s", expiresAt.Format("2006-01-02 15:04:05"))

	query := fmt.Sprintf(`
		INSERT INTO auth_tokens (user_id, user_type, token, tipo, email, expires_at)
		VALUES ('%s', '%s', '%s', '%s', '%s', '%s')
	`, userID.String(), userType, token, tipo, email, expiresAt.Format("2006-01-02 15:04:05"))

	log.Printf("[EMAIL][DEBUG] Executando query: %s", query)
	
	_, err = s.db.Exec(query)
	if err != nil {
		log.Printf("[EMAIL][DEBUG] Erro ao executar query: %v", err)
		return "", fmt.Errorf("erro ao salvar token: %w", err)
	}

	log.Printf("[EMAIL][DEBUG] Token salvo com sucesso")
	return token, nil
}

func (s *EmailService) VerifyToken(token, tipo string) (*TokenInfo, error) {
	log.Printf("[EMAIL][DEBUG] VerifyToken - Token: %s, Tipo: %s", token, tipo)
	
	query := fmt.Sprintf(`
		SELECT user_id, user_type, email, usado, expires_at
		FROM auth_tokens
		WHERE token = '%s' AND tipo = '%s'
	`, token, tipo)

	log.Printf("[EMAIL][DEBUG] Executando query: %s", query)
	
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
			log.Printf("[EMAIL][DEBUG] Token não encontrado")
			return nil, fmt.Errorf("token inválido ou expirado")
		}
		log.Printf("[EMAIL][DEBUG] Erro ao verificar token: %v", err)
		return nil, fmt.Errorf("erro ao verificar token: %w", err)
	}

	log.Printf("[EMAIL][DEBUG] Token encontrado - UserID: %s, Usado: %v, Expira: %s", 
		info.UserID, info.Usado, info.ExpiresAt.Format("2006-01-02 15:04:05"))

	if info.Usado {
		log.Printf("[EMAIL][DEBUG] Token já foi usado")
		return nil, fmt.Errorf("token já foi usado")
	}

	if time.Now().After(info.ExpiresAt) {
		log.Printf("[EMAIL][DEBUG] Token expirado")
		return nil, fmt.Errorf("token expirado")
	}

	updateQuery := fmt.Sprintf(`
		UPDATE auth_tokens 
		SET usado = TRUE, usado_em = CURRENT_TIMESTAMP 
		WHERE token = '%s'
	`, token)
	
	log.Printf("[EMAIL][DEBUG] Marcando token como usado: %s", updateQuery)
	s.db.Exec(updateQuery)

	log.Printf("[EMAIL][DEBUG] Token verificado e marcado como usado com sucesso")
	return &info, nil
}

func (s *EmailService) SendEmail(to, subject, body string) error {
	log.Printf("[EMAIL][DEBUG] SendEmail iniciado - Para: %s, Assunto: %s", to, subject)
	
	if !s.enabled {
		log.Printf("[EMAIL][DEBUG] Serviço desabilitado, abortando")
		return fmt.Errorf("serviço de email desabilitado")
	}

	if to == "" {
		log.Printf("[EMAIL][DEBUG] Destinatário vazio, abortando")
		return fmt.Errorf("destinatário vazio")
	}

	log.Printf("[EMAIL][DEBUG] Tentando enviar para %s via %s:%s (SSL: %v)", 
		to, s.smtpHost, s.smtpPort, s.useSSL)

	// Formato idêntico ao test_smtp.go que funcionou
	msg := fmt.Sprintf("From: %s\r\n"+
		"To: %s\r\n"+
		"Subject: %s\r\n"+
		"MIME-Version: 1.0\r\n"+
		"Content-Type: text/html; charset=UTF-8\r\n"+
		"\r\n"+
		"%s", s.fromEmail, to, subject, body)

	log.Printf("[EMAIL][DEBUG] Mensagem formatada (%d bytes)", len(msg))

	addr := fmt.Sprintf("%s:%s", s.smtpHost, s.smtpPort)

	if s.useSSL {
		log.Printf("[EMAIL][DEBUG] Usando método SSL (porta 465)")
		return s.sendMailSSL(addr, to, msg)
	}
	
	log.Printf("[EMAIL][DEBUG] Usando método STARTTLS (porta 587)")
	return s.sendMailSTARTTLS(addr, to, msg)
}

func (s *EmailService) sendMailSTARTTLS(addr, to, msg string) error {
	log.Printf("[EMAIL][DEBUG] sendMailSTARTTLS iniciado - Addr: %s", addr)
	
	auth := smtp.PlainAuth("", s.smtpUser, s.smtpPass, s.smtpHost)
	log.Printf("[EMAIL][DEBUG] Auth criado para user: %s", s.smtpUser)
	
	log.Printf("[EMAIL][DEBUG] Tentando conectar TCP com timeout de 15s...")
	conn, err := net.DialTimeout("tcp", addr, 15*time.Second)
	if err != nil {
		log.Printf("[EMAIL][DEBUG] ERRO ao conectar TCP: %v", err)
		return fmt.Errorf("falha ao conectar: %w", err)
	}
	defer conn.Close()
	log.Printf("[EMAIL][DEBUG] Conexão TCP estabelecida")

	log.Printf("[EMAIL][DEBUG] Criando cliente SMTP...")
	client, err := smtp.NewClient(conn, s.smtpHost)
	if err != nil {
		log.Printf("[EMAIL][DEBUG] ERRO ao criar client SMTP: %v", err)
		return fmt.Errorf("falha ao criar cliente SMTP: %w", err)
	}
	defer client.Quit()
	log.Printf("[EMAIL][DEBUG] Cliente SMTP criado")

	// STARTTLS (igual ao test_smtp.go)
	if ok, _ := client.Extension("STARTTLS"); ok {
		log.Printf("[EMAIL][DEBUG] STARTTLS disponível, iniciando...")
		config := &tls.Config{
			ServerName: s.smtpHost,
		}
		if err = client.StartTLS(config); err != nil {
			log.Printf("[EMAIL][DEBUG] ERRO ao iniciar STARTTLS: %v", err)
			return fmt.Errorf("falha ao iniciar TLS: %w", err)
		}
		log.Printf("[EMAIL][DEBUG] STARTTLS iniciado com sucesso")
	} else {
		log.Printf("[EMAIL][DEBUG] STARTTLS não disponível, continuando sem TLS")
	}

	log.Printf("[EMAIL][DEBUG] Tentando autenticação...")
	if err = client.Auth(auth); err != nil {
		log.Printf("[EMAIL][DEBUG] ERRO na autenticação: %v", err)
		return fmt.Errorf("falha na autenticação: %w", err)
	}
	log.Printf("[EMAIL][DEBUG] Autenticação bem-sucedida")

	log.Printf("[EMAIL][DEBUG] Definindo remetente: %s", s.fromEmail)
	if err = client.Mail(s.fromEmail); err != nil {
		log.Printf("[EMAIL][DEBUG] ERRO ao definir remetente: %v", err)
		return fmt.Errorf("falha ao definir remetente: %w", err)
	}

	log.Printf("[EMAIL][DEBUG] Definindo destinatário: %s", to)
	if err = client.Rcpt(to); err != nil {
		log.Printf("[EMAIL][DEBUG] ERRO ao definir destinatário: %v", err)
		return fmt.Errorf("falha ao definir destinatário: %w", err)
	}

	log.Printf("[EMAIL][DEBUG] Abrindo data writer...")
	w, err := client.Data()
	if err != nil {
		log.Printf("[EMAIL][DEBUG] ERRO ao abrir data writer: %v", err)
		return fmt.Errorf("falha ao abrir data writer: %w", err)
	}

	log.Printf("[EMAIL][DEBUG] Escrevendo mensagem...")
	if _, err = w.Write([]byte(msg)); err != nil {
		log.Printf("[EMAIL][DEBUG] ERRO ao escrever mensagem: %v", err)
		return fmt.Errorf("falha ao escrever mensagem: %w", err)
	}

	log.Printf("[EMAIL][DEBUG] Fechando data writer...")
	if err = w.Close(); err != nil {
		log.Printf("[EMAIL][DEBUG] ERRO ao fechar data writer: %v", err)
		return fmt.Errorf("falha ao fechar data writer: %w", err)
	}

	log.Printf("[EMAIL] ✓ Email enviado com sucesso via STARTTLS")
	return nil
}

func (s *EmailService) sendMailSSL(addr, to, msg string) error {
	log.Printf("[EMAIL][DEBUG] sendMailSSL iniciado - Addr: %s", addr)
	
	auth := smtp.PlainAuth("", s.smtpUser, s.smtpPass, s.smtpHost)
	log.Printf("[EMAIL][DEBUG] Auth criado para user: %s", s.smtpUser)

	tlsConfig := &tls.Config{
		ServerName: s.smtpHost,
	}
	log.Printf("[EMAIL][DEBUG] TLS Config criado para servidor: %s", s.smtpHost)

	log.Printf("[EMAIL][DEBUG] Tentando conexão TLS/SSL com timeout de 15s...")
	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		log.Printf("[EMAIL][DEBUG] ERRO na conexão SSL: %v", err)
		return fmt.Errorf("falha ao conectar SSL: %w", err)
	}
	defer conn.Close()
	log.Printf("[EMAIL][DEBUG] Conexão SSL estabelecida")

	log.Printf("[EMAIL][DEBUG] Criando cliente SMTP sobre SSL...")
	client, err := smtp.NewClient(conn, s.smtpHost)
	if err != nil {
		log.Printf("[EMAIL][DEBUG] ERRO ao criar client SSL: %v", err)
		return fmt.Errorf("falha ao criar cliente SMTP: %w", err)
	}
	defer client.Quit()
	log.Printf("[EMAIL][DEBUG] Cliente SMTP SSL criado")

	log.Printf("[EMAIL][DEBUG] Tentando autenticação...")
	if err = client.Auth(auth); err != nil {
		log.Printf("[EMAIL][DEBUG] ERRO na autenticação: %v", err)
		return fmt.Errorf("falha na autenticação: %w", err)
	}
	log.Printf("[EMAIL][DEBUG] Autenticação bem-sucedida")

	log.Printf("[EMAIL][DEBUG] Definindo remetente: %s", s.fromEmail)
	if err = client.Mail(s.fromEmail); err != nil {
		log.Printf("[EMAIL][DEBUG] ERRO ao definir remetente: %v", err)
		return fmt.Errorf("falha ao definir remetente: %w", err)
	}

	log.Printf("[EMAIL][DEBUG] Definindo destinatário: %s", to)
	if err = client.Rcpt(to); err != nil {
		log.Printf("[EMAIL][DEBUG] ERRO ao definir destinatário: %v", err)
		return fmt.Errorf("falha ao definir destinatário: %w", err)
	}

	log.Printf("[EMAIL][DEBUG] Abrindo data writer...")
	w, err := client.Data()
	if err != nil {
		log.Printf("[EMAIL][DEBUG] ERRO ao abrir data writer: %v", err)
		return fmt.Errorf("falha ao abrir data writer: %w", err)
	}

	log.Printf("[EMAIL][DEBUG] Escrevendo mensagem...")
	if _, err = w.Write([]byte(msg)); err != nil {
		log.Printf("[EMAIL][DEBUG] ERRO ao escrever mensagem: %v", err)
		return fmt.Errorf("falha ao escrever mensagem: %w", err)
	}

	log.Printf("[EMAIL][DEBUG] Fechando data writer...")
	if err = w.Close(); err != nil {
		log.Printf("[EMAIL][DEBUG] ERRO ao fechar data writer: %v", err)
		return fmt.Errorf("falha ao fechar data writer: %w", err)
	}

	log.Printf("[EMAIL] ✓ Email enviado com sucesso via SSL")
	return nil
}

func (s *EmailService) SendVerificationEmail(userID uuid.UUID, userType, email, nome string) error {
	log.Printf("[EMAIL][INICIO] SendVerificationEmail - UserID: %s, Type: %s, Email: %s, Nome: %s", 
		userID, userType, email, nome)
	
	if !s.enabled {
		log.Println("[EMAIL][ERRO] Serviço desabilitado - pulando verificação")
		return fmt.Errorf("serviço de email desabilitado")
	}

	if email == "" {
		log.Printf("[EMAIL][ERRO] Email vazio, abortando")
		return fmt.Errorf("email vazio")
	}

	log.Printf("[EMAIL][DEBUG] Gerando token de verificação...")
	token, err := s.SaveToken(userID, userType, "verificacao_email", email, 24*time.Hour)
	if err != nil {
		log.Printf("[EMAIL][ERRO] Erro ao salvar token: %v", err)
		return fmt.Errorf("erro ao gerar token: %w", err)
	}

	verifyURL := fmt.Sprintf("%s/verificar-email/%s", s.frontendURL, token)
	log.Printf("[EMAIL][DEBUG] URL de verificação: %s", verifyURL)

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

	log.Printf("[EMAIL][DEBUG] Chamando SendEmail para verificação...")
	err = s.SendEmail(email, subject, body)
	if err != nil {
		log.Printf("[EMAIL][ERRO] SendEmail FALHOU: %v", err)
		return err
	}
	
	log.Printf("[EMAIL][SUCESSO] Email de verificação enviado!")
	return nil
}

func (s *EmailService) SendPasswordResetEmail(userID uuid.UUID, userType, email, nome string) error {
	log.Printf("[EMAIL][DEBUG] SendPasswordResetEmail - UserID: %s, Type: %s, Email: %s, Nome: %s", 
		userID, userType, email, nome)
	
	if !s.enabled {
		log.Println("[EMAIL][DEBUG] Serviço desabilitado - pulando recuperação")
		return fmt.Errorf("serviço de email desabilitado")
	}

	if email == "" {
		log.Printf("[EMAIL][DEBUG] Email vazio, abortando")
		return fmt.Errorf("email vazio")
	}

	log.Printf("[EMAIL][DEBUG] Gerando token de recuperação...")
	token, err := s.SaveToken(userID, userType, "recuperacao_senha", email, 1*time.Hour)
	if err != nil {
		log.Printf("[EMAIL][DEBUG] Erro ao gerar token: %v", err)
		return fmt.Errorf("erro ao gerar token: %w", err)
	}

	resetURL := fmt.Sprintf("%s/recuperar-senha/%s", s.frontendURL, token)
	log.Printf("[EMAIL][DEBUG] URL de recuperação: %s", resetURL)

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

	log.Printf("[EMAIL][DEBUG] Chamando SendEmail para recuperação...")
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