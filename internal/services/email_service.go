// internal/services/email_service.go
package services

import (
	"bytes"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type EmailService struct {
	db             *sqlx.DB
	serviceID      string
	templateVerify string
	templateReset  string
	publicKey      string
	privateKey     string
	frontendURL    string
	enabled        bool
	httpClient     *http.Client
}

// EmailJS API Request
type EmailJSRequest struct {
	ServiceID      string            `json:"service_id"`
	TemplateID     string            `json:"template_id"`
	UserID         string            `json:"user_id"`
	AccessToken    string            `json:"accessToken,omitempty"`
	TemplateParams map[string]string `json:"template_params"`
}

func NewEmailService(db *sqlx.DB) *EmailService {
	serviceID := os.Getenv("EMAILJS_SERVICE_ID")
	templateVerify := os.Getenv("EMAILJS_TEMPLATE_VERIFICATION")
	templateReset := os.Getenv("EMAILJS_TEMPLATE_RESET")
	publicKey := os.Getenv("EMAILJS_PUBLIC_KEY")
	privateKey := os.Getenv("EMAILJS_PRIVATE_KEY")

	enabled := serviceID != "" && templateVerify != "" && templateReset != "" && publicKey != ""

	if !enabled {
		log.Println("[EMAIL] ⚠️  DESABILITADO - configure EMAILJS_* vars")
		return &EmailService{
			db:          db,
			enabled:     false,
			frontendURL: getEnvOrDefault("FRONTEND_URL", "http://localhost:3000"),
		}
	}

	log.Printf("[EMAIL] ✅ EmailJS configurado - ServiceID: %s", serviceID)

	return &EmailService{
		db:             db,
		serviceID:      serviceID,
		templateVerify: templateVerify,
		templateReset:  templateReset,
		publicKey:      publicKey,
		privateKey:     privateKey,
		frontendURL:    getEnvOrDefault("FRONTEND_URL", "http://localhost:3000"),
		enabled:        enabled,
		httpClient:     &http.Client{Timeout: 30 * time.Second},
	}
}

func (s *EmailService) IsEnabled() bool {
	return s.enabled
}

// GenerateToken gera um token aleatório seguro de 32 bytes (64 chars hex).
func GenerateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// GenerateSecurePassword gera uma senha temporária criptograficamente segura
// com 14 caracteres, garantindo ao menos uma maiúscula, minúscula, dígito e símbolo.
//
// FIX E4-AA-04: substitui senhas hardcoded ("spuriadm", "spurifpp", "spurigerente")
// que eram idênticas para todos os admins do mesmo role, públicas no repositório,
// e tornavam todas as contas recém-criadas e pós-reset trivialmente comprometíveis.
// Agora cada criação/reset produz uma senha única, desconhecida até mesmo pelo código.
func GenerateSecurePassword() (string, error) {
	const (
		upper   = "ABCDEFGHJKLMNPQRSTUVWXYZ" // sem I, O para evitar confusão visual
		lower   = "abcdefghjkmnpqrstuvwxyz"  // sem i, l, o
		digits  = "23456789"                  // sem 0, 1
		symbols = "@#$%&*!"
		all     = upper + lower + digits + symbols
		length  = 14
	)

	password := make([]byte, length)

	// Garantir ao menos um de cada classe nos primeiros 4 slots.
	charsets := []string{upper, lower, digits, symbols}
	for i, charset := range charsets {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", fmt.Errorf("erro ao gerar senha: %w", err)
		}
		password[i] = charset[n.Int64()]
	}

	// Preencher restante com caracteres do conjunto completo.
	for i := len(charsets); i < length; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(all))))
		if err != nil {
			return "", fmt.Errorf("erro ao gerar senha: %w", err)
		}
		password[i] = all[n.Int64()]
	}

	// Embaralhar para não fixar padrão nas primeiras posições.
	for i := length - 1; i > 0; i-- {
		j, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return "", fmt.Errorf("erro ao embaralhar senha: %w", err)
		}
		password[i], password[j.Int64()] = password[j.Int64()], password[i]
	}

	return string(password), nil
}

// SaveToken persiste um token de autenticação (verificação ou recuperação).
//
// FIX SVC-01: prepared statement — sem interpolação de string.
func (s *EmailService) SaveToken(userID uuid.UUID, userType, tipo, email string, expiresIn time.Duration) (string, error) {
	token, err := GenerateToken()
	if err != nil {
		return "", fmt.Errorf("erro ao gerar token: %w", err)
	}

	expiresAt := time.Now().Add(expiresIn)

	_, err = s.db.Exec(`
		INSERT INTO auth_tokens (token, user_id, user_type, tipo, email, expires_at, usado)
		VALUES ($1, $2, $3, $4, $5, $6, FALSE)
	`, token, userID, userType, tipo, email, expiresAt)
	if err != nil {
		return "", fmt.Errorf("erro ao salvar token: %w", err)
	}

	return token, nil
}

// VerifyToken valida e consome um token de autenticação.
//
// FIX SVC-03: erro do UPDATE não é silenciado — token não reutilizável.
func (s *EmailService) VerifyToken(token, tipo string) (*TokenInfo, error) {
	var info TokenInfo
	var idStr string

	err := s.db.QueryRow(`
		SELECT user_id, user_type, email, usado, expires_at
		FROM auth_tokens
		WHERE token = $1 AND tipo = $2
	`, token, tipo).Scan(&idStr, &info.UserType, &info.Email, &info.Usado, &info.ExpiresAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("token inválido ou não encontrado")
	}
	if err != nil {
		return nil, fmt.Errorf("erro ao verificar token: %w", err)
	}

	if info.Usado {
		return nil, fmt.Errorf("token já foi utilizado")
	}

	if time.Now().After(info.ExpiresAt) {
		return nil, fmt.Errorf("token expirado")
	}

	info.UserID, _ = uuid.Parse(idStr)

	// FIX SVC-03: marcar como usado; erro aqui impede reuso.
	_, err = s.db.Exec(`
		UPDATE auth_tokens
		SET usado = TRUE, usado_em = CURRENT_TIMESTAMP
		WHERE token = $1
	`, token)
	if err != nil {
		return nil, fmt.Errorf("erro ao marcar token como usado: %w", err)
	}

	return &info, nil
}

func (s *EmailService) sendEmailViaEmailJS(to, nome, templateID string, params map[string]string) error {
	if !s.enabled {
		log.Printf("[EMAIL] ⚠️  Serviço desabilitado — email não enviado para: %s", to)
		return fmt.Errorf("serviço de email desabilitado")
	}

	if to == "" {
		return fmt.Errorf("destinatário vazio")
	}

	params["to_email"] = to
	params["to_name"] = nome
	params["from_name"] = "Spuri Sistema Acadêmico"

	log.Printf("[EMAIL] 📧 Enviando para: %s via EmailJS", to)

	emailReq := EmailJSRequest{
		ServiceID:      s.serviceID,
		TemplateID:     templateID,
		UserID:         s.publicKey,
		TemplateParams: params,
	}

	if s.privateKey != "" {
		emailReq.AccessToken = s.privateKey
	}

	jsonData, err := json.Marshal(emailReq)
	if err != nil {
		return fmt.Errorf("erro ao serializar email: %w", err)
	}

	maxRetries := 2
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		resp, err := s.httpClient.Post(
			"https://api.emailjs.com/api/v1.0/email/send",
			"application/json",
			bytes.NewBuffer(jsonData),
		)

		if err != nil {
			lastErr = fmt.Errorf("erro na requisição: %w", err)
			log.Printf("[EMAIL] ❌ Tentativa %d falhou: %v", attempt, lastErr)
			if attempt < maxRetries {
				time.Sleep(time.Duration(attempt*2) * time.Second)
			}
			continue
		}

		defer resp.Body.Close()

		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			lastErr = fmt.Errorf("erro ao ler resposta: %w", err)
			log.Printf("[EMAIL] ❌ Tentativa %d falhou: %v", attempt, lastErr)
			if attempt < maxRetries {
				time.Sleep(time.Duration(attempt*2) * time.Second)
			}
			continue
		}

		bodyStr := string(bodyBytes)

		if resp.StatusCode == 200 && (bodyStr == "OK" || bodyStr == "\"OK\"") {
			log.Printf("[EMAIL] ✅ Enviado com sucesso (%d/%d)", attempt, maxRetries)
			return nil
		}

		var errorMsg string
		var jsonError map[string]interface{}
		if err := json.Unmarshal(bodyBytes, &jsonError); err == nil {
			if msg, ok := jsonError["message"].(string); ok {
				errorMsg = msg
			} else if text, ok := jsonError["text"].(string); ok {
				errorMsg = text
			} else {
				errorMsg = bodyStr
			}
		} else {
			errorMsg = bodyStr
		}

		lastErr = fmt.Errorf("EmailJS erro [%d]: %s", resp.StatusCode, errorMsg)
		log.Printf("[EMAIL] ❌ Tentativa %d falhou: %v", attempt, lastErr)

		if attempt < maxRetries {
			time.Sleep(time.Duration(attempt*2) * time.Second)
		}
	}

	return fmt.Errorf("falha após %d tentativas: %w", maxRetries, lastErr)
}

// SendVerificationEmail envia email de verificação de endereço.
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

	params := map[string]string{
		"user_name":  nome,
		"verify_url": verifyURL,
		"expiry":     "24 horas",
	}

	return s.sendEmailViaEmailJS(email, nome, s.templateVerify, params)
}

// SendPasswordResetEmail envia link de recuperação de senha.
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

	params := map[string]string{
		"user_name": nome,
		"reset_url": resetURL,
		"expiry":    "1 hora",
	}

	return s.sendEmailViaEmailJS(email, nome, s.templateReset, params)
}

// SendAdminWelcomeEmail envia a senha temporária ao admin recém-criado.
//
// FIX E4-AA-03: este é o único canal pelo qual a senha chega ao admin —
// a resposta HTTP da criação nunca inclui a senha, tornando a afirmação
// "enviada por email" factualmente verdadeira.
//
// Quando o serviço de email está desabilitado (desenvolvimento), a senha é
// registada no log do servidor — visível apenas a operadores com acesso ao
// sistema — para não bloquear o fluxo em ambientes sem SMTP configurado.
func (s *EmailService) SendAdminWelcomeEmail(email, nome, senhaTemporaria, role string) error {
	if !s.enabled {
		log.Printf("[EMAIL] ⚠️  Serviço desabilitado — senha temporária do admin %s (%s) registada no log do servidor.",
			email, role)
		log.Printf("[EMAIL] 🔑 Senha temporária para %s: %s", email, senhaTemporaria)
		return nil // não é erro: funciona em modo degradado
	}

	if email == "" {
		return fmt.Errorf("email vazio")
	}

	templateAdmin := os.Getenv("EMAILJS_TEMPLATE_ADMIN_WELCOME")
	if templateAdmin == "" {
		// Fallback: reutiliza o template de reset de senha se não houver template dedicado.
		templateAdmin = s.templateReset
		log.Printf("[EMAIL] ⚠️  EMAILJS_TEMPLATE_ADMIN_WELCOME não configurado — usando templateReset como fallback")
	}

	params := map[string]string{
		"user_name":     nome,
		"user_role":     role,
		"temp_password": senhaTemporaria,
		"login_url":     fmt.Sprintf("%s/admin/login", s.frontendURL),
	}

	return s.sendEmailViaEmailJS(email, nome, templateAdmin, params)
}

// GetDefaultPassword retorna a senha padrão para estudantes e academias.
// O código do estudante/academia é conhecido pelo operador que criou o registro,
// tornando este mecanismo aceitável para esses perfis.
//
// FIX E4-AA-04: admins foram REMOVIDOS desta função.
// Admins nunca devem ter senha derivada de constante pública do código-fonte.
// Use GenerateSecurePassword() + SendAdminWelcomeEmail() para admins.
func GetDefaultPassword(userType, codigo string) string {
	switch userType {
	case "estudante":
		return codigo
	case "academia":
		return codigo
	default:
		// Qualquer outra chamada é uso incorreto — logar para diagnóstico.
		log.Printf("[WARN] GetDefaultPassword chamado para userType=%q — use GenerateSecurePassword() para admins", userType)
		return codigo
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// TokenInfo representa as informações de um token de autenticação.
type TokenInfo struct {
	UserID    uuid.UUID `db:"user_id"`
	UserType  string    `db:"user_type"`
	Email     string    `db:"email"`
	Usado     bool      `db:"usado"`
	ExpiresAt time.Time `db:"expires_at"`
}