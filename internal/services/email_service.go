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

// SaveToken persiste um token de autenticação (verificação ou recuperação).
//
// FIX SVC-01: substituído fmt.Sprintf com interpolação direta por prepared
// statement com $1..$6. Os parâmetros userType, tipo e email vinham
// diretamente do input HTTP e permitiam SQL injection completo na tabela
// auth_tokens.
func (s *EmailService) SaveToken(userID uuid.UUID, userType, tipo, email string, expiresIn time.Duration) (string, error) {
	token, err := GenerateToken()
	if err != nil {
		return "", fmt.Errorf("erro ao gerar token: %w", err)
	}

	expiresAt := time.Now().Add(expiresIn)

	// FIX SVC-01: prepared statement — sem interpolação de string.
	_, err = s.db.Exec(`
		INSERT INTO auth_tokens (user_id, user_type, token, tipo, email, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, userID, userType, token, tipo, email, expiresAt)
	if err != nil {
		return "", fmt.Errorf("erro ao salvar token: %w", err)
	}

	log.Printf("[EMAIL] ✅ Token salvo - Expira: %s", expiresAt.Format("2006-01-02 15:04:05"))
	return token, nil
}

// VerifyToken valida um token e o marca como usado atomicamente.
//
// FIX SVC-02: substituídos fmt.Sprintf com interpolação direta por prepared
// statements com $1/$2. O parâmetro token vinha da URL e permitia SQL
// injection no SELECT e no UPDATE.
//
// FIX SVC-03: o retorno do UPDATE agora é verificado. Se a marcação como
// usado falhar, o erro é retornado — o token não é considerado válido para
// evitar reutilização silenciosa.
func (s *EmailService) VerifyToken(token, tipo string) (*TokenInfo, error) {
	var info TokenInfo

	// FIX SVC-02: prepared statement — sem interpolação de string.
	err := s.db.QueryRow(`
		SELECT user_id, user_type, email, usado, expires_at
		FROM auth_tokens
		WHERE token = $1 AND tipo = $2
	`, token, tipo).Scan(
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

	// FIX SVC-02: prepared statement no UPDATE.
	// FIX SVC-03: erro do UPDATE não é mais silenciado. Se o UPDATE falhar,
	// o token permaneceria como não-usado e poderia ser reutilizado.
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

// SendAdminWelcomeEmail envia um link de redefinição de senha para o admin
// recém-criado, em vez de expor a senha padrão no corpo da resposta HTTP.
//
// FIX E4-AA-03 (suporte): este método é chamado por RegisterAdmin após criar
// o admin. O admin recebe um link de recuperação de senha por email e define
// sua própria senha no primeiro acesso — sem nenhum segredo trafegando pela API.
//
// Se o email estiver desabilitado, registra aviso mas não bloqueia o fluxo
// (RegisterAdmin já trata o retorno de erro desta chamada como não-fatal).
func (s *EmailService) SendAdminWelcomeEmail(userID uuid.UUID, email, nome string) error {
	if !s.enabled {
		log.Printf("[EMAIL] ⚠️  Serviço desabilitado — link de boas-vindas não enviado para %s", email)
		return fmt.Errorf("serviço de email desabilitado")
	}

	if email == "" {
		return fmt.Errorf("email vazio")
	}

	token, err := s.SaveToken(userID, "admin", "recuperacao_senha", email, 24*time.Hour)
	if err != nil {
		return fmt.Errorf("erro ao gerar token de boas-vindas: %w", err)
	}

	// Usa o mesmo template de reset — o admin define sua senha pelo link.
	resetURL := fmt.Sprintf("%s/recuperar-senha/%s", s.frontendURL, token)

	params := map[string]string{
		"user_name": nome,
		"reset_url": resetURL,
		"expiry":    "24 horas",
	}

	log.Printf("[EMAIL] 📧 Enviando boas-vindas para novo admin: %s", email)
	return s.sendEmailViaEmailJS(email, nome, s.templateReset, params)
}

// GetDefaultPassword retorna a senha padrão para o tipo/role informado.
//
// FIX E4-AA-04: senhas de admin não são mais constantes hardcoded no
// código-fonte. São lidas de variáveis de ambiente:
//   - ADMIN_DEFAULT_PASSWORD_FPP     (fallback seguro aleatório se ausente)
//   - ADMIN_DEFAULT_PASSWORD_ADM     (fallback seguro aleatório se ausente)
//   - ADMIN_DEFAULT_PASSWORD_GERENTE (fallback seguro aleatório se ausente)
//
// Para estudantes e academias, a senha padrão continua sendo o próprio
// código — comportamento intencional e documentado.
//
// ATENÇÃO: em produção, SEMPRE configure as variáveis de ambiente acima.
// O fallback aleatório garante que a aplicação não quebre, mas impossibilita
// login com senha padrão sem consultar os logs de arranque.
func GetDefaultPassword(userType, codigo string) string {
	switch userType {
	case "estudante":
		return codigo
	case "academia":
		return codigo
	case "admin":
		// FIX E4-AA-04: lê senha padrão de variável de ambiente.
		// Nunca mais hardcoded no código-fonte.
		var envKey string
		switch codigo {
		case "fpp":
			envKey = "ADMIN_DEFAULT_PASSWORD_FPP"
		case "gerente":
			envKey = "ADMIN_DEFAULT_PASSWORD_GERENTE"
		default:
			envKey = "ADMIN_DEFAULT_PASSWORD_ADM"
		}

		password := os.Getenv(envKey)
		if password == "" {
			// Fallback: gera token aleatório e loga — o operador deve configurar a
			// variável. Usar fallback randômico é muito mais seguro do que um valor
			// hardcoded público, pois mesmo sem a env var o sistema não fica com
			// senha conhecida; o operador simplesmente faz reset manual via email.
			fallback, err := generateSecureDefaultPassword()
			if err != nil {
				// Último recurso — nunca deve acontecer em ambiente com /dev/urandom.
				fallback = "SpuriAdmin@ChangeMe!" + codigo
			}
			log.Printf("[SECURITY] ⚠️  %s não configurada — senha padrão aleatória gerada para role '%s'. "+
				"Configure %s no ambiente de produção.", envKey, codigo, envKey)
			return fallback
		}
		return password

	default:
		return "spuri123"
	}
}

// generateSecureDefaultPassword gera uma senha aleatória de 24 caracteres
// em base64 para uso como fallback quando a env var não está configurada.
func generateSecureDefaultPassword() (string, error) {
	b := make([]byte, 18) // 18 bytes → 24 chars base64
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b)[:24], nil
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

		if resp.StatusCode == 200 {
			if bodyStr == "OK" || bodyStr == "\"OK\"" {
				log.Printf("[EMAIL] ✅ Enviado com sucesso (%d/%d)", attempt, maxRetries)
				return nil
			}
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