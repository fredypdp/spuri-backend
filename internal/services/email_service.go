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

func (s *EmailService) sendEmailViaEmailJS(to, nome, templateID string, params map[string]string) error {
	if !s.enabled {
		log.Printf("[EMAIL] ⚠️  Serviço desabilitado")
		return fmt.Errorf("serviço de email desabilitado")
	}

	if to == "" {
		return fmt.Errorf("destinatário vazio")
	}

	// Adicionar parâmetros padrão
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

	// Adicionar private key se disponível
	if s.privateKey != "" {
		emailReq.AccessToken = s.privateKey
	}

	jsonData, err := json.Marshal(emailReq)
	if err != nil {
		return fmt.Errorf("erro ao serializar email: %w", err)
	}

	log.Printf("[EMAIL] 🔍 Request: %s", string(jsonData))

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

		// Ler o corpo da resposta
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
		log.Printf("[EMAIL] 📥 Response [%d]: %s", resp.StatusCode, bodyStr)

		// EmailJS retorna "OK" em texto quando sucesso (status 200)
		if resp.StatusCode == 200 {
			if bodyStr == "OK" || bodyStr == "\"OK\"" {
				log.Printf("[EMAIL] ✅ Enviado com sucesso (%d/%d)", attempt, maxRetries)
				return nil
			}
		}

		// Se não for 200 ou não for "OK", tentar parsear como JSON de erro
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

func GetDefaultPassword(userType, codigo string) string {
	switch userType {
	case "estudante":
		return codigo // código do próprio estudante
	case "academia":
		return codigo // código da própria academia
	case "admin":
		// codigo contém o role quando chamado de ResetarSenha ou RegisterAdmin
		switch codigo {
		case "fpp":
			return "spurifpp"
		case "gerente":
			return "spurigerente"
		default: // "adm" e qualquer outro
			return "spuriadm"
		}
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